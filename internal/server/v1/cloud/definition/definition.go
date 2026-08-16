package definition

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"log/slog"
	"slices"

	"codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/internal/sortid"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud/definition"
	"connectrpc.com/connect"
	"github.com/megakuul/dynamitedb"
)

type Server struct {
	logger *slog.Logger
	oltp   *dynamitedb.Bucket
}

func New(logger *slog.Logger, oltp *dynamitedb.Bucket) *Server {
	return &Server{
		logger: logger,
		oltp:   oltp,
	}
}

func (s *Server) Get(ctx context.Context, req *connect.Request[definition.GetRequest]) (*connect.Response[definition.GetResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	definitionMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Definition{
		ProviderID:   dynamitedb.Key(req.Msg.ProviderId),
		DefinitionID: dynamitedb.Key(req.Msg.Id),
		Scope:        dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("definition does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch definition: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch definition"))
	}

	return &connect.Response[definition.GetResponse]{Msg: &definition.GetResponse{Definition: &cloud.Definition{
		ProviderId:  definitionMeta.ProviderID.Value(),
		Id:          definitionMeta.DefinitionID.Value(),
		Name:        definitionMeta.Name.Value(),
		Description: definitionMeta.Description.Value(),
		Version:     definitionMeta.Version.Value(),
		Hash:        definitionMeta.Hash.Value(),

		Scope: definitionMeta.Scope.Value(),
	}}}, nil
}

func (s *Server) List(ctx context.Context, req *connect.Request[definition.ListRequest]) (*connect.Response[definition.ListResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	opts := []dynamitedb.Option{dynamitedb.WithLimit(int(req.Msg.Limit))}
	if req.Msg.StartAfter != "" {
		opts = append(opts, dynamitedb.WithStartAfter(&oltp.Definition{
			DefinitionID: dynamitedb.Key(req.Msg.StartAfter),
		}))
	}
	definitions, err := dynamitedb.Query(ctx, s.oltp, &oltp.Definition{
		ProviderID:   dynamitedb.Key(req.Msg.ProviderId),
		DefinitionID: dynamitedb.KeyPrefix(""),
		Scope:        dynamitedb.In(auth.Scopes(ctx)...),
	}, opts...)
	if err != nil {
		l.Error(fmt.Sprintf("failed to iterate definitions: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to iterate definitions"))
	}

	definitionsOutput := []*cloud.Definition{}
	for _, definition := range definitions {
		definitionsOutput = append(definitionsOutput, &cloud.Definition{
			ProviderId:  definition.ProviderID.Value(),
			Id:          definition.DefinitionID.Value(),
			Name:        definition.Name.Value(),
			Description: definition.Description.Value(),
			Version:     definition.Version.Value(),
			Hash:        definition.Hash.Value(),

			Scope: definition.Scope.Value(),
		})
	}

	return &connect.Response[definition.ListResponse]{Msg: &definition.ListResponse{
		Definitions: definitionsOutput,
	}}, nil
}

func (s *Server) Create(ctx context.Context, req *connect.Request[definition.CreateRequest]) (*connect.Response[definition.CreateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	if !slices.Contains(auth.Scopes(ctx), req.Msg.Init.Scope) {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("can't attach a scope you don't possess"))
	}

	h := sha256.New()
	_, err := h.Write(req.Msg.Binary)
	if err != nil {
		l.Error(fmt.Sprintf("failed to create definition binary hash: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create definition binary hash"))
	}

	definitionID := sortid.New().String()
	err = dynamitedb.Create(ctx, s.oltp, &oltp.DefinitionBinary{
		ProviderID:   dynamitedb.Key(req.Msg.Init.ProviderId),
		DefinitionID: dynamitedb.Key(definitionID),
		Compression:  dynamitedb.Set(req.Msg.Compression),
		WASM:         dynamitedb.Set(req.Msg.Binary),

		Scope: dynamitedb.Set(req.Msg.Init.Scope),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("definition binary does already exist"))
		}
		l.Error(fmt.Sprintf("failed to create definition binary: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create definition binary"))
	}

	err = dynamitedb.Create(ctx, s.oltp, &oltp.Definition{
		ProviderID:   dynamitedb.Key(req.Msg.Init.ProviderId),
		DefinitionID: dynamitedb.Key(definitionID),
		Name:         dynamitedb.Set(req.Msg.Init.Name),
		Description:  dynamitedb.Set(req.Msg.Init.Description),
		Version:      dynamitedb.Set(req.Msg.Init.Version),
		Hash:         dynamitedb.Set(h.Sum(nil)),

		Scope: dynamitedb.Set(req.Msg.Init.Scope),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrAlreadyExists) {
			return nil, connect.NewError(connect.CodeAlreadyExists, fmt.Errorf("definition does already exist"))
		}
		l.Error(fmt.Sprintf("failed to create definition: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create definition"))
	}

	return &connect.Response[definition.CreateResponse]{Msg: &definition.CreateResponse{
		Id: definitionID,
	}}, nil
}

func (s *Server) Update(ctx context.Context, req *connect.Request[definition.UpdateRequest]) (*connect.Response[definition.UpdateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	newHash := []byte{}
	if len(req.Msg.Binary) > 0 {
		definitionBinary, err := dynamitedb.Get(ctx, s.oltp, &oltp.DefinitionBinary{
			ProviderID:   dynamitedb.Key(req.Msg.Mod.ProviderId),
			DefinitionID: dynamitedb.Key(req.Msg.Mod.Id),
			Scope:        dynamitedb.In(auth.Scopes(ctx)...),
		})
		if err != nil {
			if errors.Is(err, dynamitedb.ErrNotFound) {
				return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("definition binary does not exist"))
			}
			l.Error(fmt.Sprintf("failed to fetch definition binary: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch definition binary"))
		}

		h := sha256.New()
		_, err = h.Write(req.Msg.Binary)
		if err != nil {
			l.Error(fmt.Sprintf("failed to create definition binary hash: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create definition binary hash"))
		}
		newHash = h.Sum(nil)

		err = dynamitedb.Update(ctx, s.oltp, &oltp.DefinitionBinary{
			ProviderID:   dynamitedb.Key(definitionBinary.ProviderID.Value()),
			DefinitionID: dynamitedb.Key(definitionBinary.DefinitionID.Value()),
			ETag:         definitionBinary.ETag,
			Compression:  dynamitedb.Set(req.Msg.Compression),
			WASM:         dynamitedb.Set(req.Msg.Binary),
		})
		if err != nil {
			l.Error(fmt.Sprintf("failed to update definition binary: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update definition binary"))
		}
	}
	definitionMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Definition{
		ProviderID:   dynamitedb.Key(req.Msg.Mod.ProviderId),
		DefinitionID: dynamitedb.Key(req.Msg.Mod.Id),
		Scope:        dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("definition does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch definition: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch definition"))
	}

	err = dynamitedb.Update(ctx, s.oltp, &oltp.Definition{
		ProviderID:   dynamitedb.Key(definitionMeta.ProviderID.Value()),
		DefinitionID: dynamitedb.Key(definitionMeta.DefinitionID.Value()),
		ETag:         definitionMeta.ETag,
		Name:         dynamitedb.Set(req.Msg.Mod.Name),
		Description:  dynamitedb.Set(req.Msg.Mod.Description),
		Hash: dynamitedb.CustomUpdate(func(original []byte) []byte {
			if len(newHash) > 0 {
				return newHash
			}
			return original
		}),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to update definition: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update definition"))
	}

	return &connect.Response[definition.UpdateResponse]{Msg: &definition.UpdateResponse{}}, nil
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[definition.DeleteRequest]) (*connect.Response[definition.DeleteResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	err := dynamitedb.Delete(ctx, s.oltp, &oltp.DefinitionBinary{
		ProviderID:   dynamitedb.Key(req.Msg.ProviderId),
		DefinitionID: dynamitedb.Key(req.Msg.Id),
		Scope:        dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrFilterMismatch) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("definition binary cannot be deleted"))
		}
		l.Error(fmt.Sprintf("failed to delete definition binary: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete definition binary"))
	}

	err = dynamitedb.Delete(ctx, s.oltp, &oltp.Definition{
		ProviderID:   dynamitedb.Key(req.Msg.ProviderId),
		DefinitionID: dynamitedb.Key(req.Msg.Id),
		Scope:        dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrFilterMismatch) {
			return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("definition binary cannot be deleted"))
		}
		l.Error(fmt.Sprintf("failed to delete definition: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete definition"))
	}
	return &connect.Response[definition.DeleteResponse]{Msg: &definition.DeleteResponse{}}, nil
}
