package rbac

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"

	"codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/model/creds"
	"codeberg.org/megakuul/cloudjam/internal/model/role"
	"codeberg.org/megakuul/cloudjam/internal/model/user"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/rbac"
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

func (s *Server) ConfigureRole(ctx context.Context, req *connect.Request[rbac.ConfigureRoleRequest]) (*connect.Response[rbac.ConfigureRoleResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	roleIter := s.coll.Query().
		Where("pk", "=", user.Key.New(req.Msg.Mod.Id)).
		Where("sk", "=", user.SortData.New("")).
		Where("scope", "in", auth.Scopes(ctx)).Get(ctx)
	defer roleIter.Stop()
	roleData := &role.Data{}
	if err := roleIter.Next(ctx, roleData); err != nil {
		if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch user: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch role"))
	}

	if roleData.Builtin {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("builtin roles cannot be modified"))
	}

	permissions := map[role.Scope][]string{}
	// prevent scope privilege escalation (allow only builtin ScopeAdmin to self extend)
	for scope, permission := range req.Msg.Mod.Permissions {
		if slices.Contains(auth.Scopes(ctx), role.Scope(scope)) || slices.Contains(auth.Scopes(ctx), role.ScopeAdmin) {
			permissions[role.Scope(scope)] = permission.ActionExprs
			continue
		}
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("can't grant a scope you don't possess ('%s')", scope))
	}

	err := s.coll.Update(ctx, roleData, docstore.Mods{
		"permissions": permissions,
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to attach role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to attach role"))
	}
	return &connect.Response[rbac.ConfigureRoleResponse]{Msg: &rbac.ConfigureRoleResponse{}}, nil
}

func (s *Server) AttachRole(ctx context.Context, req *connect.Request[rbac.AttachRoleRequest]) (*connect.Response[rbac.AttachRoleResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	userIter := s.coll.Query().
		Where("pk", "=", user.Key.New(req.Msg.Id)).
		Where("sk", "=", user.SortData.New("")).
		Where("scope", "in", auth.Scopes(ctx)).Get(ctx)
	defer userIter.Stop()
	userData := &user.Data{}
	if err := userIter.Next(ctx, userData); err != nil {
		if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch user: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
	}
	// load role to ensure A) it exists and B) the requestor has access to its scope.
	roleIter := s.coll.Query().
		Where("pk", "=", role.Key.New(req.Msg.Role)).
		Where("sk", "=", role.SortData.New("")).
		Where("scope", "in", auth.Scopes(ctx)).Get(ctx)
	defer roleIter.Stop()
	roleData := &role.Data{}
	if err := roleIter.Next(ctx, roleData); err != nil {
		if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch role"))
	}

	if userData.Privileged {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("privileged users cannot be modified"))
	}

	err := s.coll.Update(ctx, userData, docstore.Mods{
		"role": req.Msg.Role,
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to attach role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to attach role"))
	}
	return &connect.Response[rbac.AttachRoleResponse]{Msg: &rbac.AttachRoleResponse{}}, nil
}

func (s *Server) AttachScope(ctx context.Context, req *connect.Request[rbac.AttachScopeRequest]) (*connect.Response[rbac.AttachScopeResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	if !slices.Contains(auth.Scopes(ctx), role.Scope(req.Msg.Scope)) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("can't attach a scope you don't possess"))
	}

	switch req.Msg.Resource {
	case rbac.Resource_CredsData:
		credsIter := s.coll.Query().
			Where("pk", "=", creds.Key.New(req.Msg.Id)).
			Where("sk", "=", creds.SortData.New("")).
			Where("scope", "in", auth.Scopes(ctx)).Get(ctx)
		defer credsIter.Stop()
		credsData := &creds.Data{}
		if err := credsIter.Next(ctx, credsData); err != nil {
			if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("creds does not exist"))
			}
			l.Error(fmt.Sprintf("failed to fetch creds: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch creds"))
		}
		if err := s.coll.Update(ctx, credsData, docstore.Mods{"scope": req.Msg.Scope}); err != nil {
			l.Error(fmt.Sprintf("failed to attach scope: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scope creds"))
		}
	case rbac.Resource_RoleData:
		roleIter := s.coll.Query().
			Where("pk", "=", role.Key.New(req.Msg.Id)).
			Where("sk", "=", role.SortData.New("")).
			Where("scope", "in", auth.Scopes(ctx)).Get(ctx)
		defer roleIter.Stop()
		roleData := &role.Data{}
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
		if err := s.coll.Update(ctx, roleData, docstore.Mods{"scope": req.Msg.Scope}); err != nil {
			l.Error(fmt.Sprintf("failed to attach scope: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scope role"))
		}
	case rbac.Resource_UserData:
		userIter := s.coll.Query().
			Where("pk", "=", user.Key.New(req.Msg.Id)).
			Where("sk", "=", user.SortData.New("")).
			Where("scope", "in", auth.Scopes(ctx)).Get(ctx)
		defer userIter.Stop()
		userData := &user.Data{}
		if err := userIter.Next(ctx, userData); err != nil {
			if errors.Is(err, gcerrors.ErrNotFound) || errors.Is(err, io.EOF) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user does not exist"))
			}
			l.Error(fmt.Sprintf("failed to fetch user: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
		}
		if userData.Privileged {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("privileged users cannot be modified"))
		}
		if err := s.coll.Update(ctx, userData, docstore.Mods{"scope": req.Msg.Scope}); err != nil {
			l.Error(fmt.Sprintf("failed to attach scope: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scope role"))
		}
	}
	return &connect.Response[rbac.AttachScopeResponse]{Msg: &rbac.AttachScopeResponse{}}, nil
}
