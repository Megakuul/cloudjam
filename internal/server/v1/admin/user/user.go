package user

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/model"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/user"
	"connectrpc.com/connect"
	"github.com/alexedwards/argon2id"
	"github.com/google/uuid"
	"github.com/megakuul/dynamitedb"
)

type Server struct {
	logger *slog.Logger
	bucket *dynamitedb.Bucket
}

func New(logger *slog.Logger, bucket *dynamitedb.Bucket) *Server {
	return &Server{
		logger: logger,
		bucket: bucket,
	}
}

func (s *Server) Get(ctx context.Context, req *connect.Request[user.GetRequest]) (*connect.Response[user.GetResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	userFilter := &model.User{}
	if req.Msg.Id == "" || req.Msg.Id == auth.Claims(ctx).Subject {
		if !slices.Contains(auth.Scopes(ctx), model.ScopeSelf) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("no self management access"))
		}
		userFilter.UserID = dynamitedb.Key(auth.Claims(ctx).Subject)
	} else {
		// if the requested user is not the requesting user perform scope check.
		userFilter.UserID = dynamitedb.Key(req.Msg.Id)
		userFilter.Scope = dynamitedb.In(auth.Scopes(ctx)...)
	}

	foundUser, err := dynamitedb.Get(ctx, s.bucket, userFilter)
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch user: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
	}

	return &connect.Response[user.GetResponse]{Msg: &user.GetResponse{User: &admin.User{
		Id:           foundUser.UserID.Value(),
		PubId:        foundUser.PubId.Value(),
		Username:     foundUser.Username.Value(),
		Description:  foundUser.Description.Value(),
		Organization: foundUser.Organization.Value(),
		Email:        foundUser.Email.Value(),
		Score:        foundUser.Score.Value(),
		MaxScore:     foundUser.MaxScore.Value(),
		Streak:       int64(foundUser.Streak.Value()),
		MaxStreak:    int64(foundUser.MaxStreak.Value()),
		Privileged:   foundUser.Privileged.Value(),
		Role:         foundUser.Role.Value(),
		Scope:        foundUser.Scope.Value(),
		CreatedAt:    foundUser.CreatedAt.Value().Unix(),
	}}}, nil
}

func (s *Server) List(ctx context.Context, req *connect.Request[user.ListRequest]) (*connect.Response[user.ListResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	opts := []dynamitedb.Option{dynamitedb.WithLimit(int(req.Msg.Limit))}
	if req.Msg.StartAfter != "" {
		opts = append(opts, dynamitedb.WithStartAfter(&model.User{
			UserID: dynamitedb.Key(req.Msg.StartAfter),
		}))
	}
	users, err := dynamitedb.Query(ctx, s.bucket, &model.User{
		UserID: dynamitedb.KeyPrefix(""), // scan and yes this is expensive...
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	}, opts...)
	if err != nil {
		l.Error(fmt.Sprintf("failed to iterate user: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to iterate user"))
	}

	usersOutput := []*admin.User{}
	for _, user := range users {
		usersOutput = append(usersOutput, &admin.User{
			Id:           user.UserID.Value(),
			PubId:        user.PubId.Value(),
			Username:     user.Username.Value(),
			Description:  user.Description.Value(),
			Organization: user.Organization.Value(),
			Email:        user.Email.Value(),
			Score:        user.Score.Value(),
			MaxScore:     user.MaxScore.Value(),
			Streak:       int64(user.Streak.Value()),
			MaxStreak:    int64(user.MaxStreak.Value()),
			Privileged:   user.Privileged.Value(),
			Role:         user.Role.Value(),
			CreatedAt:    user.CreatedAt.Value().Unix(),
		})
	}

	return &connect.Response[user.ListResponse]{Msg: &user.ListResponse{
		Users: usersOutput,
	}}, nil
}

func (s *Server) Create(ctx context.Context, req *connect.Request[user.CreateRequest]) (*connect.Response[user.CreateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	if !slices.Contains(auth.Scopes(ctx), req.Msg.Init.Scope) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("can't attach a scope you don't possess"))
	}

	code := rand.Text()
	codeHash, err := argon2id.CreateHash(code, argon2id.DefaultParams)
	if err != nil {
		l.Error(fmt.Sprintf("failed to construct argon2id hash for code: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to construct invitation code"))
	}
	userId := uuid.NewString()

	err = dynamitedb.Create(ctx, s.bucket, &model.Creds{
		Email:          dynamitedb.Key(req.Msg.Init.Email),
		Active:         dynamitedb.Set(false),
		UserId:         dynamitedb.Set(userId),
		Code:           dynamitedb.Set(codeHash),
		CodeExpiration: dynamitedb.Set(req.Msg.Expires.AsTime()),
		Scope:          dynamitedb.Set(req.Msg.Init.Scope),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("user does already exist"))
		}
		l.Error(fmt.Sprintf("failed to create user invitation: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create user invitation"))
	}

	err = dynamitedb.Create(ctx, s.bucket, &model.User{
		UserID:       dynamitedb.Key(userId),
		PubId:        dynamitedb.Set(uuid.NewString()),
		Email:        dynamitedb.Set(req.Msg.Init.Email),
		Username:     dynamitedb.Set(req.Msg.Init.Username),
		Description:  dynamitedb.Set(req.Msg.Init.Description),
		Organization: dynamitedb.Set(req.Msg.Init.Organization),
		Score:        dynamitedb.Set(0.0),
		MaxScore:     dynamitedb.Set(0.0),
		Streak:       dynamitedb.Set(0),
		MaxStreak:    dynamitedb.Set(0),
		Privileged:   dynamitedb.Set(false),
		CreatedAt:    dynamitedb.Set(time.Now()),
		Scope:        dynamitedb.Set(req.Msg.Init.Scope),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to create user: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create user"))
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
		targetUser, err := dynamitedb.Get(ctx, s.bucket, &model.User{
			UserID: dynamitedb.Key(req.Msg.Mod.Id),
			Scope:  dynamitedb.In(auth.Scopes(ctx)...),
		})
		if err != nil {
			if errors.Is(err, dynamitedb.ErrNotFound) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user does not exist"))
			}
			l.Error(fmt.Sprintf("failed to fetch user: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
		}
		if targetUser.Privileged.Value() {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("privileged users cannot be modified"))
		}
		err = dynamitedb.Update(ctx, s.bucket, &model.User{
			UserID:       dynamitedb.Key(req.Msg.Mod.Id),
			Organization: dynamitedb.Set(req.Msg.Mod.Organization),
		})
		if err != nil {
			l.Error(fmt.Sprintf("failed to update user: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update user"))
		}
	} else {
		if !slices.Contains(auth.Scopes(ctx), model.ScopeSelf) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("no self management access"))
		}
		err := dynamitedb.Update(ctx, s.bucket, &model.User{
			UserID:      dynamitedb.Key(auth.Claims(ctx).Subject),
			Username:    dynamitedb.Set(req.Msg.Mod.Username),
			Description: dynamitedb.Set(req.Msg.Mod.Description),
		})
		if err != nil {
			if errors.Is(err, dynamitedb.ErrNotFound) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user does not exist"))
			}
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

	credsFilter := &model.Creds{}
	if req.Msg.Email == auth.Claims(ctx).Email {
		if !slices.Contains(auth.Scopes(ctx), model.ScopeSelf) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("no self management access"))
		}
		credsFilter.Email = dynamitedb.Key(auth.Claims(ctx).Email)
		credsFilter.UserId = dynamitedb.Eq(auth.Claims(ctx).Subject)
	} else {
		credsFilter.Email = dynamitedb.Key(req.Msg.Email)
		credsFilter.Scope = dynamitedb.In(auth.Scopes(ctx)...)
	}

	creds, err := dynamitedb.Get(ctx, s.bucket, credsFilter)
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch user credentials: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user credentials"))
	}

	err = dynamitedb.Update(ctx, s.bucket, &model.Creds{
		Email:          creds.Email,
		Code:           dynamitedb.Set(codeHash),
		CodeExpiration: dynamitedb.Set(req.Msg.Expires.AsTime()),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to reset user password: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to reset user password"))
	}
	return &connect.Response[user.ResetPasswordResponse]{
		Msg: &user.ResetPasswordResponse{
			Code: code,
		},
	}, nil
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[user.DeleteRequest]) (*connect.Response[user.DeleteResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	err := dynamitedb.Delete(ctx, s.bucket, &model.User{
		UserID:     dynamitedb.Key(req.Msg.Id),
		Scope:      dynamitedb.In(auth.Scopes(ctx)...),
		Privileged: dynamitedb.Eq(false),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrFilterMismatch) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("privileged or out of scope users cannot be deleted"))
		}
		l.Error(fmt.Sprintf("failed to fetch user: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
	}
	return &connect.Response[user.DeleteResponse]{Msg: &user.DeleteResponse{}}, nil
}
