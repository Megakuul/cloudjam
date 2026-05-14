package user

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/model/creds"
	"codeberg.org/megakuul/cloudjam/internal/model/role"
	usermodel "codeberg.org/megakuul/cloudjam/internal/model/user"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/user"
	"connectrpc.com/connect"
	"github.com/alexedwards/argon2id"
	"github.com/google/uuid"
	"gocloud.dev/docstore"
	"gocloud.dev/gcerrors"
)

type Server struct {
	logger *slog.Logger
	coll   *docstore.Collection
}

func New(logger *slog.Logger, coll *docstore.Collection) *Server {
	return &Server{
		logger: logger,
		coll:   coll,
	}
}

func (s *Server) Get(ctx context.Context, req *connect.Request[user.GetRequest]) (*connect.Response[user.GetResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	var query *docstore.Query
	if req.Msg.Id == "" || req.Msg.Id == auth.Claims(ctx).Subject {
		if !slices.Contains(auth.Scopes(ctx), role.ScopeSelf) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("no self management access"))
		}
		query = s.coll.Query().
			Where("pk", "=", usermodel.Key.New(auth.Claims(ctx).Subject)).
			Where("sk", "=", usermodel.SortData.New(""))
	} else {
		// if the requested user is not the requesting user perform scope check.
		query = s.coll.Query().
			Where("pk", "=", usermodel.Key.New(req.Msg.Id)).
			Where("sk", "=", usermodel.SortData.New("")).
			Where("scope", "in", auth.Scopes(ctx))
	}

	userIter := query.Get(ctx)
	defer userIter.Stop()
	userData := &usermodel.Data{}
	if err := userIter.Next(ctx, userData); err != nil {
		if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch user: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
	}

	return &connect.Response[user.GetResponse]{Msg: &user.GetResponse{User: &admin.User{
		Id:          userData.PK.ID(usermodel.Key),
		Username:    userData.Username,
		Description: userData.Description,
		Email:       userData.Email,
		Score:       userData.Score,
		MaxScore:    userData.MaxScore,
		Streak:      int64(userData.Streak),
		MaxStreak:   int64(userData.MaxStreak),
		Privileged:  userData.Privileged,
		Role:        userData.Role,
		CreatedAt:   userData.CreatedAt.Unix(),
	}}}, nil
}

func (s *Server) List(ctx context.Context, req *connect.Request[user.ListRequest]) (*connect.Response[user.ListResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	userIter := s.coll.Query().Limit(int(req.Msg.Limit)).Offset(int(req.Msg.Offset)).
		Where("scope", "in", auth.Scopes(ctx)).
		Get(ctx)
	defer userIter.Stop()

	users := []*admin.User{}
	for {
		userData := usermodel.Data{}
		if err := userIter.Next(ctx, &userData); err != nil {
			if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
				break
			}
			l.Error(fmt.Sprintf("failed to iterate user: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to iterate user"))
		}
		users = append(users, &admin.User{
			Id:          userData.PK.ID(usermodel.Key),
			Username:    userData.Username,
			Description: userData.Description,
			Email:       userData.Email,
			Score:       userData.Score,
			MaxScore:    userData.MaxScore,
			Streak:      int64(userData.Streak),
			MaxStreak:   int64(userData.MaxStreak),
			Privileged:  userData.Privileged,
			Role:        userData.Role,
			CreatedAt:   userData.CreatedAt.Unix(),
		})
	}

	return &connect.Response[user.ListResponse]{Msg: &user.ListResponse{
		Users: users,
	}}, nil
}

func (s *Server) Create(ctx context.Context, req *connect.Request[user.CreateRequest]) (*connect.Response[user.CreateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	if !slices.Contains(auth.Scopes(ctx), role.Scope(req.Msg.Init.Scope)) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("can't attach a scope you don't possess"))
	}

	code := rand.Text()
	codeHash, err := argon2id.CreateHash(code, argon2id.DefaultParams)
	if err != nil {
		l.Error(fmt.Sprintf("failed to construct argon2id hash for code: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to construct invitation code"))
	}
	userId := uuid.NewString()
	err = s.coll.Actions().AtomicWrites().
		Create(&creds.Data{
			PK:             creds.Key.New(req.Msg.Init.Email),
			SK:             creds.SortData.New(""),
			Active:         false,
			UserId:         userId,
			Code:           codeHash,
			CodeExpiration: req.Msg.Expires.AsTime(),
			Scope:          role.Scope(req.Msg.Init.Scope),
		}).
		Create(&usermodel.Data{
			PK:           usermodel.Key.New(userId),
			SK:           usermodel.SortData.New(""),
			Email:        req.Msg.Init.Email,
			Username:     req.Msg.Init.Username,
			Description:  req.Msg.Init.Description,
			Organization: req.Msg.Init.Organization,
			Score:        0,
			MaxScore:     0,
			Streak:       0,
			MaxStreak:    0,
			Privileged:   false,
			CreatedAt:    time.Now(),
			Scope:        role.Scope(req.Msg.Init.Scope),
		}).
		Do(ctx)
	if err != nil {
		if errors.Is(err, gcerrors.ErrAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("user does already exist"))
		}
		l.Error(fmt.Sprintf("failed to create user invitation: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create user invitation"))
	}
	return &connect.Response[user.CreateResponse]{
		Msg: &user.CreateResponse{
			Code: code,
		},
	}, nil
}

func (s *Server) Update(ctx context.Context, req *connect.Request[user.UpdateRequest]) (*connect.Response[user.UpdateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	// if the requested user is not the requesting user perform scope check and ensure the user is not privileged.
	if req.Msg.Mod.Id != auth.Claims(ctx).Subject {
		userIter := s.coll.Query().
			Where("pk", "=", usermodel.Key.New(req.Msg.Mod.Id)).
			Where("sk", "=", usermodel.SortData.New("")).
			Where("scope", "in", auth.Scopes(ctx)).Get(ctx)
		defer userIter.Stop()
		userData := &usermodel.Data{}
		if err := userIter.Next(ctx, userData); err != nil {
			if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user does not exist"))
			}
			l.Error(fmt.Sprintf("failed to fetch user: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
		}
		if userData.Privileged {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("privileged users cannot be modified"))
		}
		err := s.coll.Update(ctx, userData, docstore.Mods{
			"organization": req.Msg.Mod.Organization,
		})
		if err != nil {
			l.Error(fmt.Sprintf("failed to update user: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update user"))
		}
	} else {
		if !slices.Contains(auth.Scopes(ctx), role.ScopeSelf) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("no self management access"))
		}
		userIter := s.coll.Query().
			Where("pk", "=", usermodel.Key.New(auth.Claims(ctx).Subject)).
			Where("sk", "=", usermodel.SortData.New("")).
			Get(ctx)
		defer userIter.Stop()
		userData := &usermodel.Data{}
		if err := userIter.Next(ctx, userData); err != nil {
			if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user does not exist"))
			}
			l.Error(fmt.Sprintf("failed to fetch user: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
		}
		err := s.coll.Update(ctx, userData, docstore.Mods{
			"username":    req.Msg.Mod.Username,
			"description": req.Msg.Mod.Description,
		})
		if err != nil {
			l.Error(fmt.Sprintf("failed to update user: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update user"))
		}
	}
	return &connect.Response[user.UpdateResponse]{Msg: &user.UpdateResponse{}}, nil
}

func (s *Server) ResetPassword(ctx context.Context, req *connect.Request[user.ResetPasswordRequest]) (*connect.Response[user.ResetPasswordResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	code := rand.Text()
	codeHash, err := argon2id.CreateHash(code, argon2id.DefaultParams)
	if err != nil {
		l.Error(fmt.Sprintf("failed to construct argon2id hash for code: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to construct invitation code"))
	}

	var credsQuery *docstore.Query
	if req.Msg.Email == auth.Claims(ctx).Email {
		if !slices.Contains(auth.Scopes(ctx), role.ScopeSelf) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("no self management access"))
		}
		credsQuery = s.coll.Query().
			Where("pk", "=", creds.Key.New(auth.Claims(ctx).Email)).
			Where("sk", "=", usermodel.SortData.New("")).
			Where("user_id", "=", auth.Claims(ctx).Subject)
	} else {
		credsQuery = s.coll.Query().
			Where("pk", "=", creds.Key.New(req.Msg.Email)).
			Where("sk", "=", usermodel.SortData.New("")).
			Where("scope", "in", auth.Scopes(ctx))
	}

	credsIter := credsQuery.Get(ctx)
	defer credsIter.Stop()
	credsData := &creds.Data{}
	if err := credsIter.Next(ctx, credsData); err != nil {
		if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch user credentials: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user credentials"))
	}

	err = s.coll.Update(ctx, credsData, docstore.Mods{
		"code":            codeHash,
		"code_expiration": req.Msg.Expires.AsTime(),
	})
	if err != nil {
		if errors.Is(err, gcerrors.ErrAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("user does already exist"))
		}
		l.Error(fmt.Sprintf("failed to create user invitation: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create user invitation"))
	}
	return &connect.Response[user.ResetPasswordResponse]{
		Msg: &user.ResetPasswordResponse{
			Code: code,
		},
	}, nil
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[user.DeleteRequest]) (*connect.Response[user.DeleteResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	userIter := s.coll.Query().
		Where("pk", "=", usermodel.Key.New(req.Msg.Id)).
		Where("sk", "=", usermodel.SortData.New("")).
		Where("scope", "in", auth.Scopes(ctx)).Get(ctx)
	defer userIter.Stop()
	userData := &usermodel.Data{}
	if err := userIter.Next(ctx, userData); err != nil {
		if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch user: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
	}
	if userData.Privileged {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("privileged users cannot be deleted"))
	}
	err := s.coll.Delete(ctx, userData)
	if err != nil {
		l.Error(fmt.Sprintf("failed to update user: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update user"))
	}
	return &connect.Response[user.DeleteResponse]{Msg: &user.DeleteResponse{}}, nil
}
