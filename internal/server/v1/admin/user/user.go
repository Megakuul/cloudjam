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
		Scopes:         []role.Scope{role.ScopeAdmin},
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

func (s *Server) Get(ctx context.Context, req *connect.Request[user.GetRequest]) (*connect.Response[user.GetResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	query := s.coll.Query().
		Where("pk", "=", usermodel.Key.New(auth.Claims(ctx).Subject)).
		Where("sk", "=", usermodel.SortData.New(""))

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

	// if the requested user is not the requesting user perform scope check.
	if req.Msg.Id != "" && req.Msg.Id != auth.Claims(ctx).Subject {
		if !slices.Contains(userData.Scopes, auth.Scope(ctx)) {
			l.Info("invalid user access (incorrect scope)", "ip", req.Peer().Addr, "email", auth.Claims(ctx).Email)
			// I'm considering returning 404 here but the ux is just much better with 403 (+ it is uuidv4 you cannot enumerate)
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied (outside of scope)"))
		}
	}

	return &connect.Response[user.GetResponse]{Msg: &user.GetResponse{User: &admin.User{
		Id:          userData.PK.ID(usermodel.Key),
		Username:    userData.Username,
		Description: userData.Description,
		Email:       userData.Email,
		Score:       userData.Score,
		Streak:      int64(userData.Streak),
		MaxStreak:   int64(userData.MaxStreak),
		Privileged:  userData.Privileged,
		Role:        userData.Role,
		CreatedAt:   userData.CreatedAt.Unix(),
	}}}, nil
}

func (s *Server) List(ctx context.Context, req *connect.Request[user.ListRequest]) (*connect.Response[user.ListResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	userIter := s.coll.Query().Get(ctx)
	defer userIter.Stop()

	users := []admin.User{}
	for {
		userData := usermodel.Data{}
		if err := userIter.Next(ctx, &userData); err != nil {
			if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
				break
			}
			l.Error(fmt.Sprintf("failed to iterate user: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to iterate user"))
		}

		// if the requested user is not the requesting user perform scope check.
		if req.Msg.Id != "" && req.Msg.Id != auth.Claims(ctx).Subject {
			if !slices.Contains(userData.Scopes, auth.Scope(ctx)) {
				l.Info("invalid user access (incorrect scope)", "ip", req.Peer().Addr, "email", auth.Claims(ctx).Email)
				// I'm considering returning 404 here but the ux is just much better with 403 (+ it is uuidv4 you cannot enumerate)
				return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied (outside of scope)"))
			}
		}

	}
	if err := userIter.Next(ctx, userData); err != nil {
		if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch user: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
	}

	// if the requested user is not the requesting user perform scope check.
	if req.Msg.Id != "" && req.Msg.Id != auth.Claims(ctx).Subject {
		if !slices.Contains(userData.Scopes, auth.Scope(ctx)) {
			l.Info("invalid user access (incorrect scope)", "ip", req.Peer().Addr, "email", auth.Claims(ctx).Email)
			// I'm considering returning 404 here but the ux is just much better with 403 (+ it is uuidv4 you cannot enumerate)
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("permission denied (outside of scope)"))
		}
	}

	return &connect.Response[user.GetResponse]{Msg: &user.GetResponse{User: &admin.User{
		Id:          userData.PK.ID(usermodel.Key),
		Username:    userData.Username,
		Description: userData.Description,
		Email:       userData.Email,
		Score:       userData.Score,
		Streak:      int64(userData.Streak),
		MaxStreak:   int64(userData.MaxStreak),
		Privileged:  userData.Privileged,
		Role:        userData.Role,
		CreatedAt:   userData.CreatedAt.Unix(),
	}}}, nil
}

func (s *Server) Update(ctx context.Context, req *connect.Request[user.UpdateRequest]) (*connect.Response[user.UpdateResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[user.DeleteRequest]) (*connect.Response[user.DeleteResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
