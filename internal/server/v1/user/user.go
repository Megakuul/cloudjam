package user

import (
	"context"
	"log/slog"

	"buf.build/go/protovalidate"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/user"
	"connectrpc.com/connect"
)

type Server struct {
	validator protovalidate.Validator
}

func New() *Server {
	validator, err := protovalidate.New()
	if err != nil {
		slog.Error("failed to create validator", "error", err)
		panic(err)
	}
	return &Server{
		validator: validator,
	}
}

func (s *Server) CreateUser(ctx context.Context, req *connect.Request[user.CreateUserRequest]) (*connect.Response[user.CreateUserResponse], error) {
	if err := s.validator.Validate(req.Msg); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) GetUser(ctx context.Context, req *connect.Request[user.GetUserRequest]) (*connect.Response[user.GetUserResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) UpdateUser(ctx context.Context, req *connect.Request[user.UpdateUserRequest]) (*connect.Response[user.UpdateUserResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) DeleteUser(ctx context.Context, req *connect.Request[user.DeleteUserRequest]) (*connect.Response[user.DeleteUserResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) ListUsers(ctx context.Context, req *connect.Request[user.ListUsersRequest]) (*connect.Response[user.ListUsersResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
