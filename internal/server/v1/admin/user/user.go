package user

import (
	"context"

	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/user"
	"connectrpc.com/connect"
	"gocloud.dev/docstore"
)

type Server struct {
	coll *docstore.Collection
}

func New(coll *docstore.Collection) *Server {
	return &Server{
		coll: coll,
	}
}

func (s *Server) Create(ctx context.Context, req *connect.Request[user.CreateRequest]) (*connect.Response[user.CreateResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) Get(ctx context.Context, req *connect.Request[user.GetRequest]) (*connect.Response[user.GetResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) Update(ctx context.Context, req *connect.Request[user.UpdateRequest]) (*connect.Response[user.UpdateResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[user.DeleteRequest]) (*connect.Response[user.DeleteResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}

func (s *Server) List(ctx context.Context, req *connect.Request[user.ListRequest]) (*connect.Response[user.ListResponse], error) {
	return nil, connect.NewError(connect.CodeUnimplemented, nil)
}
