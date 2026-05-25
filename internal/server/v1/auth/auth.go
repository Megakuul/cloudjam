package auth

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/alexedwards/argon2id"
	"github.com/megakuul/dynamitedb"

	"codeberg.org/megakuul/cloudjam/internal/model"
	"codeberg.org/megakuul/cloudjam/internal/token"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/auth"
)

type Server struct {
	logger *slog.Logger
	bucket *dynamitedb.Bucket
	issuer *token.Issuer
}

func New(logger *slog.Logger, bucket *dynamitedb.Bucket, issuer *token.Issuer) *Server {
	return &Server{
		logger: logger,
		bucket: bucket,
		issuer: issuer,
	}
}

func (s *Server) Register(ctx context.Context, req *connect.Request[auth.RegisterRequest]) (*connect.Response[auth.RegisterResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)
	creds, err := dynamitedb.Get(ctx, s.bucket, &model.Creds{
		Email:  dynamitedb.Key(req.Msg.Email),
		Active: dynamitedb.Eq(false),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			l.Info("invalid registration attempt detected (invalid user)", "ip", req.Peer().Addr, "email", req.Msg.Email)
			return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("incorrect email or invitation code"))
		}
		l.Error(fmt.Sprintf("failed to fetch user invitation: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user invitation"))
	}
	if match, err := argon2id.ComparePasswordAndHash(req.Msg.Code, creds.Code.Value()); err != nil || !match {
		l.Info("invalid registration attempt detected (incorrect invitation code)", "ip", req.Peer().Addr, "email", req.Msg.Email)
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("incorrect email or invitation code"))
	}
	if creds.CodeExpiration.Value().Before(time.Now()) {
		return nil, connect.NewError(connect.CodeOutOfRange, fmt.Errorf("invitation already expired"))
	}

	passwordHash, err := argon2id.CreateHash(req.Msg.Password, argon2id.DefaultParams)
	if err != nil {
		l.Error(fmt.Sprintf("failed to construct argon2id hash: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to construct argon2id hash"))
	}

	linkedUser, err := dynamitedb.Get(ctx, s.bucket, &model.User{
		UserID: dynamitedb.Key(creds.UserId.Value()),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to retrieve user linked by credentials: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
	}

	err = dynamitedb.Update(ctx, s.bucket, &model.User{
		UserID:   linkedUser.UserID,
		Username: dynamitedb.Set(req.Msg.Username),
		Email:    dynamitedb.Set(req.Msg.Email),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to update user: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("user registration failed"))
	}

	err = dynamitedb.Update(ctx, s.bucket, &model.Creds{
		Email:          creds.Email,
		Active:         dynamitedb.Set(true),
		Code:           dynamitedb.Set(""),
		CodeExpiration: dynamitedb.Set(time.Time{}),
		Password:       dynamitedb.Set(passwordHash),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to unlock user and disable invitation: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("user registration failed"))
	}

	return &connect.Response[auth.RegisterResponse]{
		Msg: &auth.RegisterResponse{},
	}, nil
}

func (s *Server) Login(ctx context.Context, req *connect.Request[auth.LoginRequest]) (*connect.Response[auth.LoginResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)
	creds, err := dynamitedb.Get(ctx, s.bucket, &model.Creds{
		Email:  dynamitedb.Key(req.Msg.Email),
		Active: dynamitedb.Eq(true),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			l.Info("invalid login attempt detected (invalid user)", "ip", req.Peer().Addr, "email", req.Msg.Email)
			return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("incorrect email or password"))
		}
		l.Error(fmt.Sprintf("failed to fetch user credentials: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user credentials"))
	}
	if match, err := argon2id.ComparePasswordAndHash(req.Msg.Password, creds.Password.Value()); err != nil || !match {
		l.Info("invalid login attempt detected (incorrect password)", "ip", req.Peer().Addr, "email", req.Msg.Email)
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("incorrect email or password"))
	}

	linkedUser, err := dynamitedb.Get(ctx, s.bucket, &model.User{
		UserID: dynamitedb.Key(creds.UserId.Value()),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to retrieve user linked by credentials: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
	}

	token, err := s.issuer.Issue(ctx, linkedUser.UserID.Value(), creds.Email.Value(), linkedUser.PubId.Value())
	if err != nil {
		l.Error(fmt.Sprintf("failed to issue token: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to issue token"))
	}

	return &connect.Response[auth.LoginResponse]{
		Msg: &auth.LoginResponse{Token: token},
	}, nil
}
