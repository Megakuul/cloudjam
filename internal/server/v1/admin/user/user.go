package user

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"

	"codeberg.org/megakuul/cloudjam/internal/model/creds"
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
	})
	if err != nil {
		if gcerrors.Code(err) == gcerrors.AlreadyExists {
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
