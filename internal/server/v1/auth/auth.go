package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/alexedwards/argon2id"
	"gocloud.dev/docstore"
	"gocloud.dev/gcerrors"

	"codeberg.org/megakuul/cloudjam/internal/model/creds"
	"codeberg.org/megakuul/cloudjam/internal/model/user"
	"codeberg.org/megakuul/cloudjam/internal/token"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/auth"
)

type Server struct {
	logger *slog.Logger
	coll   *docstore.Collection
	issuer *token.Issuer
}

func New(logger *slog.Logger, coll *docstore.Collection, issuer *token.Issuer) *Server {
	return &Server{
		logger: logger,
		coll:   coll,
		issuer: issuer,
	}
}

func (s *Server) Register(ctx context.Context, req *connect.Request[auth.RegisterRequest]) (*connect.Response[auth.RegisterResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)
	authCredsIter := s.coll.Query().
		Where("pk", "=", creds.Key.New(req.Msg.Email)).
		Where("sk", "=", creds.SortData.New("")).
		Where("active", "=", false).
		Get(ctx)
	defer authCredsIter.Stop()
	authCreds := &creds.Data{}
	if err := authCredsIter.Next(ctx, authCreds); err != nil {
		if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
			l.Info("invalid registration attempt detected (invalid user)", "ip", req.Peer().Addr, "email", req.Msg.Email)
			return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("incorrect email or invitation code"))
		}
		l.Error(fmt.Sprintf("failed to fetch user invitation: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user invitation"))
	}
	if match, err := argon2id.ComparePasswordAndHash(req.Msg.Code, authCreds.Code); err != nil || !match {
		l.Info("invalid registration attempt detected (incorrect password)", "ip", req.Peer().Addr, "email", req.Msg.Email)
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("incorrect email or invitation code"))
	}
	if authCreds.CodeExpiration.Before(time.Now()) {
		return nil, connect.NewError(connect.CodeOutOfRange, fmt.Errorf("invitation already expired"))
	}

	passwordHash, err := argon2id.CreateHash(req.Msg.Password, argon2id.DefaultParams)
	if err != nil {
		l.Error(fmt.Sprintf("failed to construct argon2id hash: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to construct argon2id hash"))
	}
	authCreds.Active = true
	authCreds.Code = ""
	authCreds.CodeExpiration = time.Time{}
	authCreds.Password = passwordHash

	linkedUserIter := s.coll.Query().
		Where("pk", "=", user.Key.New(authCreds.UserId)).
		Where("sk", "=", user.SortData.New("")).
		Get(ctx)
	defer linkedUserIter.Stop()
	linkedUser := &user.Data{}
	if err := linkedUserIter.Next(ctx, linkedUser); err != nil {
		l.Error(fmt.Sprintf("failed to retrieve user linked by credentials: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
	}

	err = s.coll.Actions().AtomicWrites().Update(linkedUser, docstore.Mods{
		"username":   req.Msg.Username,
		"email":      req.Msg.Email,
		"created_at": time.Now(),
	}).Put(authCreds).Do(ctx)
	if err != nil {
		l.Error(fmt.Sprintf("failed to create user and disable invitation: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("user creation failed"))
	}

	return &connect.Response[auth.RegisterResponse]{
		Msg: &auth.RegisterResponse{},
	}, nil
}

func (s *Server) Login(ctx context.Context, req *connect.Request[auth.LoginRequest]) (*connect.Response[auth.LoginResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)
	authCredsIter := s.coll.Query().
		Where("pk", "=", creds.Key.New(req.Msg.Email)).
		Where("sk", "=", creds.SortData.New("")).
		Where("active", "=", true).
		Get(ctx)
	defer authCredsIter.Stop()
	authCreds := &creds.Data{}
	if err := authCredsIter.Next(ctx, authCreds); err != nil {
		if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
			l.Info("invalid login attempt detected (invalid user)", "ip", req.Peer().Addr, "email", req.Msg.Email)
			return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("incorrect email or password"))
		}
		l.Error(fmt.Sprintf("failed to fetch user credentials: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user credentials"))
	}
	if match, err := argon2id.ComparePasswordAndHash(req.Msg.Password, authCreds.Password); err != nil || !match {
		l.Info("invalid login attempt detected (incorrect password)", "ip", req.Peer().Addr, "email", req.Msg.Email)
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("incorrect email or password"))
	}
	linkedUserIter := s.coll.Query().
		Where("pk", "=", user.Key.New(authCreds.UserId)).
		Where("sk", "=", user.SortData.New("")).
		Get(ctx)
	defer linkedUserIter.Stop()
	linkedUser := &user.Data{}
	if err := linkedUserIter.Next(ctx, linkedUser); err != nil {
		l.Error(fmt.Sprintf("failed to retrieve user linked by credentials: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
	}

	token, err := s.issuer.Issue(ctx, linkedUser.PK.ID(user.Key), authCreds.PK.ID(creds.Key))
	if err != nil {
		l.Error(fmt.Sprintf("failed to issue token: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to issue token"))
	}

	return &connect.Response[auth.LoginResponse]{
		Msg: &auth.LoginResponse{Token: token},
	}, nil
}
