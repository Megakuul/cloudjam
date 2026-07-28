package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/internal/sandbox"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud/provider"
	"connectrpc.com/connect"
	"github.com/megakuul/dynamitedb"
)

type Server struct {
	logger *slog.Logger
	bucket *dynamitedb.Bucket
	boxer  sandbox.Provider
}

func New(logger *slog.Logger, bucket *dynamitedb.Bucket) *Server {
	return &Server{
		logger: logger,
		bucket: bucket,
	}
}

func (s *Server) Get(ctx context.Context, req *connect.Request[provider.GetRequest]) (*connect.Response[provider.GetResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	foundProvider, err := dynamitedb.Get(ctx, s.bucket, &oltp.Provider{
		ProviderID: dynamitedb.Key(req.Msg.Id),
		Scope:      dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("provider does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch provider"))
	}

	return &connect.Response[provider.GetResponse]{Msg: &provider.GetResponse{Provider: &cloud.Provider{
		Id:              foundProvider.ProviderID.Value(),
		Type:            cloud.ProviderType(foundProvider.Type.Value()),
		Name:            foundProvider.Name.Value(),
		Description:     foundProvider.Description.Value(),
		Credentials:     "",
		DesiredAccounts: int64(foundProvider.DesiredAccounts.Value()),
		Scope:           foundProvider.Scope.Value(),
	}}}, nil
}

func (s *Server) List(ctx context.Context, req *connect.Request[provider.ListRequest]) (*connect.Response[provider.ListResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	opts := []dynamitedb.Option{dynamitedb.WithLimit(int(req.Msg.Limit))}
	if req.Msg.StartAfter != "" {
		opts = append(opts, dynamitedb.WithStartAfter(&oltp.Provider{
			ProviderID: dynamitedb.Key(req.Msg.StartAfter),
		}))
	}
	providers, err := dynamitedb.Query(ctx, s.bucket, &oltp.Provider{
		ProviderID: dynamitedb.KeyPrefix(""),
		Scope:      dynamitedb.In(auth.Scopes(ctx)...),
	}, opts...)
	if err != nil {
		l.Error(fmt.Sprintf("failed to iterate providers: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to iterate providers"))
	}

	providersOutput := []*cloud.Provider{}
	for _, provider := range providers {
		providersOutput = append(providersOutput, &cloud.Provider{
			Id:              provider.ProviderID.Value(),
			Type:            cloud.ProviderType(provider.Type.Value()),
			Name:            provider.Name.Value(),
			Description:     provider.Description.Value(),
			Credentials:     "",
			DesiredAccounts: int64(provider.DesiredAccounts.Value()),
			Scope:           provider.Scope.Value(),
		})
	}

	return &connect.Response[provider.ListResponse]{Msg: &provider.ListResponse{
		Providers: providersOutput,
	}}, nil
}

func (s *Server) Create(ctx context.Context, req *connect.Request[provider.CreateRequest]) (*connect.Response[provider.CreateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	if !slices.Contains(auth.Scopes(ctx), req.Msg.Init.Scope) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("can't attach a scope you don't possess"))
	}

	// TODO first test provider credentials
	l.Warn("TODO: test provider credentials")

	err := dynamitedb.Create(ctx, s.bucket, &oltp.Provider{
		ProviderID:      dynamitedb.Key(req.Msg.Init.Id),
		Name:            dynamitedb.Set(req.Msg.Init.Name),
		Type:            dynamitedb.Set(int(req.Msg.Init.Type)),
		Description:     dynamitedb.Set(req.Msg.Init.Description),
		Credentials:     dynamitedb.Set(req.Msg.Init.Credentials),
		DesiredAccounts: dynamitedb.Set(int(req.Msg.Init.DesiredAccounts)),
		Scope:           dynamitedb.Set(req.Msg.Init.Scope),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("provider does already exist"))
		}
		l.Error(fmt.Sprintf("failed to create provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create provider"))
	}
	return &connect.Response[provider.CreateResponse]{Msg: &provider.CreateResponse{}}, nil
}

func (s *Server) Update(ctx context.Context, req *connect.Request[provider.UpdateRequest]) (*connect.Response[provider.UpdateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	targetProvider, err := dynamitedb.Get(ctx, s.bucket, &oltp.Provider{
		ProviderID: dynamitedb.Key(req.Msg.Mod.Id),
		Scope:      dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("provider does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch provider"))
	}

	// TODO first test provider credentials
	l.Warn("TODO: test provider credentials")

	err = dynamitedb.Update(ctx, s.bucket, &oltp.Provider{
		ProviderID:      targetProvider.ProviderID,
		Type:            dynamitedb.Set(int(req.Msg.Mod.Type)),
		Name:            dynamitedb.Set(req.Msg.Mod.Name),
		Description:     dynamitedb.Set(req.Msg.Mod.Description),
		Credentials:     dynamitedb.Set(req.Msg.Mod.Credentials),
		DesiredAccounts: dynamitedb.Set(int(req.Msg.Mod.DesiredAccounts)),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to update provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update provider"))
	}
	return &connect.Response[provider.UpdateResponse]{Msg: &provider.UpdateResponse{}}, nil
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[provider.DeleteRequest]) (*connect.Response[provider.DeleteResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	// TODO first teardown infrastructure
	l.Warn("TODO: implement infrastructure teardown")

	err := dynamitedb.Delete(ctx, s.bucket, &oltp.Provider{
		ProviderID: dynamitedb.Key(req.Msg.Id),
		Scope:      dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrFilterMismatch) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("builtin or out of scope providers cannot be deleted"))
		}
		l.Error(fmt.Sprintf("failed to fetch provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch provider"))
	}
	return &connect.Response[provider.DeleteResponse]{Msg: &provider.DeleteResponse{}}, nil
}
