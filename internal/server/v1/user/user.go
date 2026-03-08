package user

import (
	context "context"
	connect "connectrpc.com/connect"
	"github.com/megakuul/zen/pkg/api/v1/user"
)

type Server struct {}

func New() *Server {
	return &Server{}
}

func (s *Server) CreateUser(ctx context.Context, req *connect.Request[user.CreateUserRequest]) (*connect.Response[user.CreateUserResponse], error) {
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
