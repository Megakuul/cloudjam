package user

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"

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

	query := s.coll.Query().
		Where("pk", "=", usermodel.Key.New(auth.Claims(ctx).Subject)).
		Where("sk", "=", usermodel.SortData.New(""))
	// if the requested user is not the requesting user perform scope check.
	if req.Msg.Id != "" && req.Msg.Id != auth.Claims(ctx).Subject {
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
	code := rand.Text()
	codeHash, err := argon2id.CreateHash(code, argon2id.DefaultParams)
	if err != nil {
		l.Error(fmt.Sprintf("failed to construct argon2id hash for code: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to construct invitation code"))
	}
	err = s.coll.Create(ctx, &creds.Data{
		PK:             creds.Key.New(req.Msg.Email),
		SK:             creds.SortData.New(""),
		Active:         false,
		UserId:         uuid.NewString(),
		Code:           codeHash,
		CodeExpiration: req.Msg.Expires.AsTime(),
		Scope:          role.ScopeAdmin,
	})
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
		if !slices.Contains(auth.Scopes(ctx), role.Scope(req.Msg.Mod.Scope)) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("cannot move user to unauthorized scope"))
		}
		err := s.coll.Update(ctx, userData, docstore.Mods{
			"scope": req.Msg.Mod.Scope,
		})
		if err != nil {
			l.Error(fmt.Sprintf("failed to update user: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update user"))
		}
	} else {
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

func (s *Server) AttachRole(ctx context.Context, req *connect.Request[user.AttachRoleRequest]) (*connect.Response[user.AttachRoleResponse], error) {
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
	// load role to ensure A) it exists and B) the requestor has access to its scope.
	roleIter := s.coll.Query().
		Where("pk", "=", role.Key.New(req.Msg.Role)).
		Where("sk", "=", role.SortData.New("")).
		Where("scope", "in", auth.Scopes(ctx)).Get(ctx)
	defer roleIter.Stop()
	roleData := &role.Data{}
	if err := roleIter.Next(ctx, roleData); err != nil {
		if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch role"))
	}

	if userData.Privileged {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("privileged users cannot be modified"))
	}

	err := s.coll.Update(ctx, userData, docstore.Mods{
		"role": req.Msg.Role,
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to attach role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to attach role"))
	}
	return &connect.Response[user.AttachRoleResponse]{Msg: &user.AttachRoleResponse{}}, nil
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[user.DeleteRequest]) (*connect.Response[user.DeleteResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
