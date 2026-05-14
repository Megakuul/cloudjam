package role

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"

	"codeberg.org/megakuul/cloudjam/internal/auth"
	rolemodel "codeberg.org/megakuul/cloudjam/internal/model/role"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/role"
	"connectrpc.com/connect"
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

func (s *Server) Get(ctx context.Context, req *connect.Request[role.GetRequest]) (*connect.Response[role.GetResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	query := s.coll.Query().
		Where("pk", "=", rolemodel.Key.New(req.Msg.Id)).
		Where("sk", "=", rolemodel.SortData.New("")).
		Where("scope", "in", auth.Scopes(ctx))
	roleIter := query.Get(ctx)
	defer roleIter.Stop()
	roleData := &rolemodel.Data{}
	if err := roleIter.Next(ctx, roleData); err != nil {
		if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch role"))
	}

	permissions := map[string]*admin.Permission{}
	for scope, exprs := range roleData.Permissions {
		permissions[string(scope)] = &admin.Permission{ActionExprs: exprs}
	}
	return &connect.Response[role.GetResponse]{Msg: &role.GetResponse{Role: &admin.Role{
		Id:          roleData.PK.ID(rolemodel.Key),
		Name:        roleData.Name,
		Description: roleData.Description,
		Builtin:     roleData.Builtin,
		Permissions: permissions,
		Scope:       string(roleData.Scope),
	}}}, nil
}

func (s *Server) List(ctx context.Context, req *connect.Request[role.ListRequest]) (*connect.Response[role.ListResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	roleIter := s.coll.Query().Limit(int(req.Msg.Limit)).Offset(int(req.Msg.Offset)).
		Where("scope", "in", auth.Scopes(ctx)).
		Get(ctx)
	defer roleIter.Stop()

	roles := []*admin.Role{}
	for {
		roleData := rolemodel.Data{}
		if err := roleIter.Next(ctx, &roleData); err != nil {
			if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
				break
			}
			l.Error(fmt.Sprintf("failed to iterate role: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to iterate role"))
		}
		permissions := map[string]*admin.Permission{}
		for scope, exprs := range roleData.Permissions {
			permissions[string(scope)] = &admin.Permission{ActionExprs: exprs}
		}
		roles = append(roles, &admin.Role{
			Id:          roleData.PK.ID(rolemodel.Key),
			Name:        roleData.Name,
			Description: roleData.Description,
			Builtin:     roleData.Builtin,
			Permissions: permissions,
			Scope:       string(roleData.Scope),
		})
	}

	return &connect.Response[role.ListResponse]{Msg: &role.ListResponse{
		Roles: roles,
	}}, nil
}

func (s *Server) Create(ctx context.Context, req *connect.Request[role.CreateRequest]) (*connect.Response[role.CreateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	if !slices.Contains(auth.Scopes(ctx), rolemodel.Scope(req.Msg.Init.Scope)) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("can't attach a scope you don't possess"))
	}

	err := s.coll.Create(ctx, &rolemodel.Data{
		PK:          rolemodel.Key.New(req.Msg.Init.Id),
		SK:          rolemodel.SortData.New(""),
		Name:        req.Msg.Init.Name,
		Description: req.Msg.Init.Description,
		Builtin:     false,
		Scope:       rolemodel.Scope(req.Msg.Init.Scope),
	})
	if err != nil {
		if errors.Is(err, gcerrors.ErrAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("role does already exist"))
		}
		l.Error(fmt.Sprintf("failed to create role invitation: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create role invitation"))
	}
	return &connect.Response[role.CreateResponse]{Msg: &role.CreateResponse{}}, nil
}

func (s *Server) Update(ctx context.Context, req *connect.Request[role.UpdateRequest]) (*connect.Response[role.UpdateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	roleIter := s.coll.Query().
		Where("pk", "=", rolemodel.Key.New(req.Msg.Mod.Id)).
		Where("sk", "=", rolemodel.SortData.New("")).
		Where("scope", "in", auth.Scopes(ctx)).Get(ctx)
	defer roleIter.Stop()
	roleData := &rolemodel.Data{}
	if err := roleIter.Next(ctx, roleData); err != nil {
		if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch role"))
	}
	if roleData.Builtin {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("builtin roles cannot be modified"))
	}
	err := s.coll.Update(ctx, roleData, docstore.Mods{
		"name":        req.Msg.Mod.Name,
		"description": req.Msg.Mod.Description,
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to update role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update role"))
	}
	return &connect.Response[role.UpdateResponse]{Msg: &role.UpdateResponse{}}, nil
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[role.DeleteRequest]) (*connect.Response[role.DeleteResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	roleIter := s.coll.Query().
		Where("pk", "=", rolemodel.Key.New(req.Msg.Id)).
		Where("sk", "=", rolemodel.SortData.New("")).
		Where("scope", "in", auth.Scopes(ctx)).Get(ctx)
	defer roleIter.Stop()
	roleData := &rolemodel.Data{}
	if err := roleIter.Next(ctx, roleData); err != nil {
		if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch role"))
	}
	if roleData.Builtin {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("builtin roles cannot be deleted"))
	}
	err := s.coll.Delete(ctx, roleData)
	if err != nil {
		l.Error(fmt.Sprintf("failed to update role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update role"))
	}
	return &connect.Response[role.DeleteResponse]{Msg: &role.DeleteResponse{}}, nil
}
