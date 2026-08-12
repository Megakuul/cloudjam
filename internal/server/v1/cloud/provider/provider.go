package provider

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/internal/provider/cache"
	"codeberg.org/megakuul/cloudjam/internal/scheduler"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud/provider"
	"connectrpc.com/connect"
	"github.com/megakuul/dynamitedb"
)

type Server struct {
	logger    *slog.Logger
	scheduler *scheduler.Scheduler
	providers *cache.Cache
	oltp      *dynamitedb.Bucket
}

func New(logger *slog.Logger, scheduler *scheduler.Scheduler, providers *cache.Cache, oltp *dynamitedb.Bucket) *Server {
	return &Server{
		logger:    logger,
		scheduler: scheduler,
		providers: providers,
		oltp:      oltp,
	}
}

func (s *Server) Get(ctx context.Context, req *connect.Request[provider.GetRequest]) (*connect.Response[provider.GetResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	providerMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Provider{
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
		Id:          providerMeta.ProviderID.Value(),
		Type:        cloud.ProviderType(providerMeta.Type.Value()),
		Name:        providerMeta.Name.Value(),
		Description: providerMeta.Description.Value(),
		Email:       providerMeta.Email.Value(),
		Regions:     providerMeta.Regions.Value(),
		Credentials: "",
		Scope:       providerMeta.Scope.Value(),
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
	providers, err := dynamitedb.Query(ctx, s.oltp, &oltp.Provider{
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
			Id:          provider.ProviderID.Value(),
			Type:        cloud.ProviderType(provider.Type.Value()),
			Name:        provider.Name.Value(),
			Description: provider.Description.Value(),
			Regions:     provider.Regions.Value(),
			Email:       provider.Email.Value(),
			Credentials: "",
			Scope:       provider.Scope.Value(),
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

	err := dynamitedb.Create(ctx, s.oltp, &oltp.Provider{
		ProviderID:  dynamitedb.Key(req.Msg.Init.Id),
		Name:        dynamitedb.Set(req.Msg.Init.Name),
		Type:        dynamitedb.Set(req.Msg.Init.Type),
		Description: dynamitedb.Set(req.Msg.Init.Description),
		Credentials: dynamitedb.Set(req.Msg.Init.Credentials),
		Email:       dynamitedb.Set(req.Msg.Init.Email),
		Regions:     dynamitedb.Set(req.Msg.Init.Regions),
		Scope:       dynamitedb.Set(req.Msg.Init.Scope),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("provider does already exist"))
		}
		l.Error(fmt.Sprintf("failed to create provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create provider"))
	}
	providerMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Provider{
		ProviderID: dynamitedb.Key(req.Msg.Init.Id),
		Scope:      dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to load provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load provider"))
	}

	_, err = s.providers.Load(ctx, providerMeta)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to initialize provider: %v", err))
	}

	return &connect.Response[provider.CreateResponse]{Msg: &provider.CreateResponse{}}, nil
}

func (s *Server) Update(ctx context.Context, req *connect.Request[provider.UpdateRequest]) (*connect.Response[provider.UpdateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	providerMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Provider{
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

	err = dynamitedb.Update(ctx, s.oltp, &oltp.Provider{
		ProviderID:  dynamitedb.Key(providerMeta.ProviderID.Value()),
		ETag:        providerMeta.ETag,
		Name:        dynamitedb.Set(req.Msg.Mod.Name),
		Description: dynamitedb.Set(req.Msg.Mod.Description),
		Credentials: dynamitedb.Set(req.Msg.Mod.Credentials),
		Regions:     dynamitedb.Set(req.Msg.Mod.Regions),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to update provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update provider"))
	}

	s.providers.Bust(providerMeta)
	_, err = s.providers.Load(ctx, providerMeta)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to initialize provider: %v", err))
	}

	return &connect.Response[provider.UpdateResponse]{Msg: &provider.UpdateResponse{}}, nil
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[provider.DeleteRequest]) (*connect.Response[provider.DeleteResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	accounts, err := dynamitedb.Query(ctx, s.oltp, &oltp.Account{
		ProviderID: dynamitedb.Key(req.Msg.Id),
	}, dynamitedb.WithLimit(1))
	if err != nil {
		l.Error(fmt.Sprintf("failed to fetch provider accounts: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch provider accounts"))
	}
	if len(accounts) > 0 {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cannot delete provider with associated accounts; delete all accounts first"))
	}
	err = dynamitedb.Delete(ctx, s.oltp, &oltp.Provider{
		ProviderID: dynamitedb.Key(req.Msg.Id),
		Scope:      dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrFilterMismatch) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("provider cannot be deleted"))
		}
		l.Error(fmt.Sprintf("failed to delete provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete provider"))
	}
	return &connect.Response[provider.DeleteResponse]{Msg: &provider.DeleteResponse{}}, nil
}
