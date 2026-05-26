package role

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/model"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/role"
	"connectrpc.com/connect"
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

func (s *Server) Get(ctx context.Context, req *connect.Request[role.GetRequest]) (*connect.Response[role.GetResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	foundRole, err := dynamitedb.Get(ctx, s.bucket, &model.Role{
		RoleID: dynamitedb.Key(req.Msg.Id),
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch role"))
	}

	permissions := map[string]string{}
	for scope, permission := range foundRole.Permissions.Value() {
		permissions[string(scope)] = permission
	}
	return &connect.Response[role.GetResponse]{Msg: &role.GetResponse{Role: &admin.Role{
		Id:          foundRole.RoleID.Value(),
		Name:        foundRole.Name.Value(),
		Description: foundRole.Description.Value(),
		Builtin:     foundRole.Builtin.Value(),
		Permissions: permissions,
		Scope:       foundRole.Scope.Value(),
	}}}, nil
}

func (s *Server) List(ctx context.Context, req *connect.Request[role.ListRequest]) (*connect.Response[role.ListResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	opts := []dynamitedb.Option{dynamitedb.WithLimit(int(req.Msg.Limit))}
	if req.Msg.StartAfter != "" {
		opts = append(opts, dynamitedb.WithStartAfter(&model.Role{
			RoleID: dynamitedb.Key(req.Msg.StartAfter),
		}))
	}
	roles, err := dynamitedb.Query(ctx, s.bucket, &model.Role{
		RoleID: dynamitedb.KeyPrefix(""),
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	}, opts...)
	if err != nil {
		l.Error(fmt.Sprintf("failed to iterate roles: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to iterate roles"))
	}

	rolesOutput := []*admin.Role{}
	for _, role := range roles {
		rolesOutput = append(rolesOutput, &admin.Role{
			Id:          role.RoleID.Value(),
			Name:        role.Name.Value(),
			Builtin:     role.Builtin.Value(),
			Permissions: role.Permissions.Value(),
			Scope:       role.Scope.Value(),
		})
	}

	return &connect.Response[role.ListResponse]{Msg: &role.ListResponse{
		Roles: rolesOutput,
	}}, nil
}

func (s *Server) Create(ctx context.Context, req *connect.Request[role.CreateRequest]) (*connect.Response[role.CreateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	if !slices.Contains(auth.Scopes(ctx), req.Msg.Init.Scope) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("can't attach a scope you don't possess"))
	}

	err := dynamitedb.Create(ctx, s.bucket, &model.Role{
		RoleID:      dynamitedb.Key(req.Msg.Init.Id),
		Name:        dynamitedb.Set(req.Msg.Init.Name),
		Description: dynamitedb.Set(req.Msg.Init.Description),
		Builtin:     dynamitedb.Set(req.Msg.Init.Builtin),
		Scope:       dynamitedb.Set(req.Msg.Init.Scope),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("role does already exist"))
		}
		l.Error(fmt.Sprintf("failed to create role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create role"))
	}
	return &connect.Response[role.CreateResponse]{Msg: &role.CreateResponse{}}, nil
}

func (s *Server) Update(ctx context.Context, req *connect.Request[role.UpdateRequest]) (*connect.Response[role.UpdateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	targetRole, err := dynamitedb.Get(ctx, s.bucket, &model.Role{
		RoleID: dynamitedb.Key(req.Msg.Mod.Id),
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch role"))
	}
	if targetRole.Builtin.Value() {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("builtin roles cannot be modified"))
	}
	err = dynamitedb.Update(ctx, s.bucket, &model.Role{
		RoleID:      targetRole.RoleID,
		Name:        dynamitedb.Set(req.Msg.Mod.Name),
		Description: dynamitedb.Set(req.Msg.Mod.Description),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to update role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update role"))
	}
	return &connect.Response[role.UpdateResponse]{Msg: &role.UpdateResponse{}}, nil
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[role.DeleteRequest]) (*connect.Response[role.DeleteResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	err := dynamitedb.Delete(ctx, s.bucket, &model.Role{
		RoleID:  dynamitedb.Key(req.Msg.Id),
		Scope:   dynamitedb.In(auth.Scopes(ctx)...),
		Builtin: dynamitedb.Eq(false),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrFilterMismatch) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("builtin or out of scope roles cannot be deleted"))
		}
		l.Error(fmt.Sprintf("failed to fetch role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch role"))
	}
	return &connect.Response[role.DeleteResponse]{Msg: &role.DeleteResponse{}}, nil
}
