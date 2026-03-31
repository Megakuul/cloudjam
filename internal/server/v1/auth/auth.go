package auth

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/model/creds"
	"codeberg.org/megakuul/cloudjam/internal/model/user"
	"codeberg.org/megakuul/cloudjam/internal/token"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/auth"
	"connectrpc.com/connect"
	"github.com/alexedwards/argon2id"
	"gocloud.dev/docstore"
	"gocloud.dev/gcerrors"
)

type Server struct {
	logger *slog.Logger
	coll   *docstore.Collection
	issuer *token.Issuer
}

func New(logger *slog.Logger, coll *docstore.Collection, issuer *token.Issuer) *Server {
	return &Server{
		coll:   coll,
		issuer: issuer,
	}
}

func (s *Server) Register(ctx context.Context, req *connect.Request[auth.RegisterRequest]) (*connect.Response[auth.RegisterResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)
	codeHash, err := argon2id.CreateHash(req.Msg.Code, argon2id.DefaultParams)
	if err != nil {
		l.Error(fmt.Sprintf("failed to construct argon2id hash: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to construct argon2id hash"))
	}
	authCreds := &creds.Data{
		PK: creds.Key.New(req.Msg.Email), SK: creds.SortData.New(""),
		Code:   codeHash,
		Active: false,
	}
	if err = s.coll.Get(ctx, authCreds); err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			l.Info("invalid registration attempt detected", "ip", req.Peer().Addr)
			return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("incorrect email or invitation code"))
		}
		l.Error(fmt.Sprintf("failed to fetch user invitation: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user invitation"))
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

	err = s.coll.Actions().AtomicWrites().Create(user.Data{
		PK:        user.Key.New(authCreds.UserId),
		SK:        user.SortData.New(""),
		Username:  req.Msg.Username,
		Email:     req.Msg.Email,
		CreatedAt: time.Now(),
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
	hash, err := argon2id.CreateHash(req.Msg.Password, argon2id.DefaultParams)
	if err != nil {
		l.Error(fmt.Sprintf("failed to construct argon2id hash: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to construct argon2id hash"))
	}
	authCreds := &creds.Data{
		PK: creds.Key.New(req.Msg.Email), SK: creds.SortData.New(""),
		Password: hash,
		Active:   true,
	}
	if err = s.coll.Get(ctx, authCreds); err != nil {
		if gcerrors.Code(err) == gcerrors.NotFound {
			l.Info("invalid login attempt detected", "ip", req.Peer().Addr)
			return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("incorrect email or password"))
		}
		l.Error(fmt.Sprintf("failed to fetch user credentials: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user credentials"))
	}
	linkedUser := &user.Data{PK: user.Key.New(authCreds.UserId), SK: user.SortData.New("")}
	if err = s.coll.Get(ctx, linkedUser); err != nil {
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
