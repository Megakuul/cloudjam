package rbac

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/admin/rbac"
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

func (s *Server) ConfigureRole(ctx context.Context, req *connect.Request[rbac.ConfigureRoleRequest]) (*connect.Response[rbac.ConfigureRoleResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	role, err := dynamitedb.Get(ctx, s.bucket, &oltp.Role{
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
	if role.Builtin.Value() {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("builtin roles cannot be modified"))
	}

	permissions := map[string]string{}
	// prevent scope privilege escalation (allow only builtin ScopeAdmin to self extend)
	for scope, permission := range req.Msg.Mod.Permissions {
		if slices.Contains(auth.Scopes(ctx), scope) || slices.Contains(auth.Scopes(ctx), oltp.ScopeAdmin) {
			permissions[scope] = permission
			continue
		}
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("can't grant a scope you don't possess ('%s')", scope))
	}

	err = dynamitedb.Update(ctx, s.bucket, &oltp.Role{
		RoleID:      role.RoleID,
		Permissions: dynamitedb.Set(permissions),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to configure role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to configure role"))
	}
	return &connect.Response[rbac.ConfigureRoleResponse]{Msg: &rbac.ConfigureRoleResponse{}}, nil
}

func (s *Server) AttachRole(ctx context.Context, req *connect.Request[rbac.AttachRoleRequest]) (*connect.Response[rbac.AttachRoleResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	user, err := dynamitedb.Get(ctx, s.bucket, &oltp.User{
		UserID: dynamitedb.Key(req.Msg.Id),
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch user: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
	}
	// load role to ensure A) it exists and B) the requestor has access to its scope.
	_, err = dynamitedb.Get(ctx, s.bucket, &oltp.Role{
		RoleID: dynamitedb.Key(req.Msg.Role),
		Scope:  dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("role does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch role"))
	}

	if user.Privileged.Value() {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("privileged users cannot be modified"))
	}

	err = dynamitedb.Update(ctx, s.bucket, &oltp.User{
		UserID: user.UserID,
		Role:   dynamitedb.Set(req.Msg.Role),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to attach role: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to attach role"))
	}
	return &connect.Response[rbac.AttachRoleResponse]{Msg: &rbac.AttachRoleResponse{}}, nil
}

func (s *Server) AttachScope(ctx context.Context, req *connect.Request[rbac.AttachScopeRequest]) (*connect.Response[rbac.AttachScopeResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	if !slices.Contains(auth.Scopes(ctx), req.Msg.Scope) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("can't attach a scope you don't possess"))
	}

	switch req.Msg.Resource {
	case rbac.Resource_CredsData:
		creds, err := dynamitedb.Get(ctx, s.bucket, &oltp.Creds{
			Email: dynamitedb.Key(req.Msg.Id),
			Scope: dynamitedb.In(auth.Scopes(ctx)...),
		})
		if err != nil {
			if errors.Is(err, dynamitedb.ErrNotFound) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("creds does not exist"))
			}
			l.Error(fmt.Sprintf("failed to fetch creds: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch creds"))
		}
		err = dynamitedb.Update(ctx, s.bucket, &oltp.Creds{
			Email: creds.Email,
			Scope: dynamitedb.Set(req.Msg.Scope),
		})
		if err != nil {
			l.Error(fmt.Sprintf("failed to attach scope: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scope creds"))
		}
	case rbac.Resource_RoleData:
		role, err := dynamitedb.Get(ctx, s.bucket, &oltp.Role{
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
		err = dynamitedb.Update(ctx, s.bucket, &oltp.Role{
			RoleID: role.RoleID,
			Scope:  dynamitedb.Set(req.Msg.Scope),
		})
		if err != nil {
			l.Error(fmt.Sprintf("failed to attach scope: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scope role"))
		}
	case rbac.Resource_UserData:
		user, err := dynamitedb.Get(ctx, s.bucket, &oltp.User{
			UserID: dynamitedb.Key(req.Msg.Id),
			Scope:  dynamitedb.In(auth.Scopes(ctx)...),
		})
		if err != nil {
			if errors.Is(err, dynamitedb.ErrNotFound) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("user does not exist"))
			}
			l.Error(fmt.Sprintf("failed to fetch user: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch user"))
		}
		if user.Privileged.Value() {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("privileged users cannot be modified"))
		}
		err = dynamitedb.Update(ctx, s.bucket, &oltp.User{
			UserID: user.UserID,
			Scope:  dynamitedb.Set(req.Msg.Scope),
		})
		if err != nil {
			l.Error(fmt.Sprintf("failed to attach scope: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to scope user"))
		}
	}
	return &connect.Response[rbac.AttachScopeResponse]{Msg: &rbac.AttachScopeResponse{}}, nil
}
