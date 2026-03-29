package auth

import (
	"context"

	"codeberg.org/megakuul/cloudjam/internal/token"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/auth"
	"connectrpc.com/connect"
	"gocloud.dev/docstore"
)

type Server struct {
	coll   *docstore.Collection
	issuer *token.Issuer
}

func New(coll *docstore.Collection, issuer *token.Issuer) *Server {
	return &Server{
		coll:   coll,
		issuer: issuer,
	}
}

func (s *Server) Login(ctx context.Context, req *connect.Request[auth.LoginRequest]) (*connect.Response[auth.LoginResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) Logout(ctx context.Context, req *connect.Request[auth.LogoutRequest]) (*connect.Response[auth.LogoutResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
