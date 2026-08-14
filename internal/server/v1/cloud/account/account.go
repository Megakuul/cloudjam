package account

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/internal/provider/cache"
	"codeberg.org/megakuul/cloudjam/internal/scheduler"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud/account"
	"connectrpc.com/connect"
	"github.com/megakuul/dynamitedb"
	"google.golang.org/protobuf/types/known/timestamppb"
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

func (s *Server) Get(ctx context.Context, req *connect.Request[account.GetRequest]) (*connect.Response[account.GetResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	foundAccount, err := dynamitedb.Get(ctx, s.oltp, &oltp.Account{
		AccountID: dynamitedb.Key(req.Msg.Id),
		Scope:     dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("account does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch account: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch account"))
	}

	return &connect.Response[account.GetResponse]{Msg: &account.GetResponse{Account: &cloud.Account{
		ProviderId:  foundAccount.ProviderID.Value(),
		Id:          foundAccount.AccountID.Value(),
		TargetId:    foundAccount.TargetID.Value(),
		Name:        foundAccount.Name.Value(),
		Description: foundAccount.Description.Value(),
		State:       cloud.AccountState(foundAccount.State.Value()),
		BoundUntil:  timestamppb.New(foundAccount.BoundUntil.Value()),
		Error:       foundAccount.Error.Value(),

		Scope: foundAccount.Scope.Value(),
	}}}, nil
}

func (s *Server) List(ctx context.Context, req *connect.Request[account.ListRequest]) (*connect.Response[account.ListResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	opts := []dynamitedb.Option{dynamitedb.WithLimit(int(req.Msg.Limit))}
	if req.Msg.StartAfter != "" {
		opts = append(opts, dynamitedb.WithStartAfter(&oltp.Account{
			AccountID: dynamitedb.Key(req.Msg.StartAfter),
		}))
	}
	accounts, err := dynamitedb.Query(ctx, s.oltp, &oltp.Account{
		ProviderID: dynamitedb.Key(req.Msg.ProviderId),
		AccountID:  dynamitedb.KeyPrefix(""),
		Scope:      dynamitedb.In(auth.Scopes(ctx)...),
	}, opts...)
	if err != nil {
		l.Error(fmt.Sprintf("failed to iterate accounts: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to iterate accounts"))
	}

	accountsOutput := []*cloud.Account{}
	for _, account := range accounts {
		accountsOutput = append(accountsOutput, &cloud.Account{
			ProviderId:  account.ProviderID.Value(),
			Id:          account.AccountID.Value(),
			TargetId:    account.TargetID.Value(),
			Name:        account.Name.Value(),
			Description: account.Description.Value(),
			State:       cloud.AccountState(account.State.Value()),
			BoundUntil:  timestamppb.New(account.BoundUntil.Value()),
			Error:       account.Error.Value(),

			Scope: account.Scope.Value(),
		})
	}

	return &connect.Response[account.ListResponse]{Msg: &account.ListResponse{
		Accounts: accountsOutput,
	}}, nil
}

func (s *Server) Create(ctx context.Context, req *connect.Request[account.CreateRequest]) (*connect.Response[account.CreateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	providerMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Provider{
		ProviderID: dynamitedb.Key(req.Msg.Init.ProviderId),
		Scope:      dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("provider does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch provider"))
	}

	err = dynamitedb.Create(ctx, s.oltp, &oltp.Account{
		ProviderID:  dynamitedb.Key(providerMeta.ProviderID.Value()),
		AccountID:   dynamitedb.Key(req.Msg.Init.Id),
		Name:        dynamitedb.Set(req.Msg.Init.Name),
		Description: dynamitedb.Set(req.Msg.Init.Description),
		State:       dynamitedb.Set(cloud.AccountState_NotCreated),
		Scope:       dynamitedb.Set(providerMeta.Scope.Value()),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to create account: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create account"))
	}

	accountMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Account{
		ProviderID: dynamitedb.Key(providerMeta.ProviderID.Value()),
		AccountID:  dynamitedb.Key(req.Msg.Init.Id),
		State:      dynamitedb.Eq(cloud.AccountState_NotCreated),
		Scope:      dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to load new account: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load new account"))
	}

	provider, err := s.providers.Load(ctx, providerMeta)
	if err != nil {
		l.Error(fmt.Sprintf("failed to load provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load provider"))
	}

	s.scheduler.Schedule(func(ctx context.Context) error {
		err = dynamitedb.Update(ctx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(accountMeta.ProviderID.Value()),
			AccountID:  dynamitedb.Key(accountMeta.AccountID.Value()),
			ETag:       accountMeta.ETag,
			State:      dynamitedb.Set(cloud.AccountState_Provisioning),
		})
		if err != nil {
			return fmt.Errorf("locking account state: %w", err)
		}
		id, err := provider.Provision(ctx, accountMeta.Name.Value())
		if err != nil {
			return fmt.Errorf("provision account: %w", err)
		}
		if err = dynamitedb.Update(ctx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(accountMeta.ProviderID.Value()),
			AccountID:  dynamitedb.Key(accountMeta.AccountID.Value()),
			TargetID:   dynamitedb.Set(id),
			State:      dynamitedb.Set(cloud.AccountState_Preparing),
		}); err != nil {
			return err
		}
		err = provider.Prepare(ctx, id)
		if err != nil {
			return fmt.Errorf("prepare account: %w", err)
		}
		if err = dynamitedb.Update(ctx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(accountMeta.ProviderID.Value()),
			AccountID:  dynamitedb.Key(accountMeta.AccountID.Value()),
			State:      dynamitedb.Set(cloud.AccountState_Ready),
		}); err != nil {
			return err
		}
		return nil
	}, func(ctx context.Context, err error) error {
		l.Warn(fmt.Sprintf("failed to create account: %v", err))
		return dynamitedb.Update(ctx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(accountMeta.ProviderID.Value()),
			AccountID:  dynamitedb.Key(accountMeta.AccountID.Value()),
			State:      dynamitedb.Set(cloud.AccountState_Corrupted),
			Error:      dynamitedb.Set(err.Error()),
		})
	})
	return &connect.Response[account.CreateResponse]{Msg: &account.CreateResponse{}}, nil
}

func (s *Server) Update(ctx context.Context, req *connect.Request[account.UpdateRequest]) (*connect.Response[account.UpdateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	targetAccount, err := dynamitedb.Get(ctx, s.oltp, &oltp.Account{
		ProviderID: dynamitedb.Key(req.Msg.Mod.ProviderId),
		AccountID:  dynamitedb.Key(req.Msg.Mod.Id),
		Scope:      dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("account does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch account: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch account"))
	}
	if targetAccount.State.Value() != cloud.AccountState_Ready {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("account must be in ready state to accept updates"))
	}
	err = dynamitedb.Update(ctx, s.oltp, &oltp.Account{
		ProviderID:  targetAccount.ProviderID,
		AccountID:   targetAccount.AccountID,
		ETag:        targetAccount.ETag,
		Description: dynamitedb.Set(req.Msg.Mod.Description),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to update account: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update account"))
	}
	return &connect.Response[account.UpdateResponse]{Msg: &account.UpdateResponse{}}, nil
}

func (s *Server) Fix(ctx context.Context, req *connect.Request[account.FixRequest]) (*connect.Response[account.FixResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	targetAccount, err := dynamitedb.Get(ctx, s.oltp, &oltp.Account{
		ProviderID: dynamitedb.Key(req.Msg.ProviderId),
		AccountID:  dynamitedb.Key(req.Msg.Id),
		Scope:      dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("account does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch account: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch account"))
	}
	err = dynamitedb.Update(ctx, s.oltp, &oltp.Account{
		ProviderID: targetAccount.ProviderID,
		AccountID:  targetAccount.AccountID,
		ETag:       targetAccount.ETag,
		State:      dynamitedb.Set(cloud.AccountState_Ready),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to hard fix account: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to hard fix account"))
	}
	return &connect.Response[account.FixResponse]{Msg: &account.FixResponse{}}, nil
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[account.DeleteRequest]) (*connect.Response[account.DeleteResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	providerMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Provider{
		ProviderID: dynamitedb.Key(req.Msg.ProviderId),
		Scope:      dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("provider does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch provider"))
	}
	accountMeta, err := dynamitedb.Get(ctx, s.oltp, &oltp.Account{
		ProviderID: dynamitedb.Key(providerMeta.ProviderID.Value()),
		AccountID:  dynamitedb.Key(req.Msg.Id),

		Scope: dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("account does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch account: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch account"))
	}
	if accountMeta.State.Value() != cloud.AccountState_Ready {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("account must be in ready state to accept updates"))
	}

	// with force remove the account just from here (this will leak the account on the provider however).
	if req.Msg.Force {
		err = dynamitedb.Delete(ctx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(accountMeta.ProviderID.Value()),
			AccountID:  dynamitedb.Key(accountMeta.AccountID.Value()),
			ETag:       accountMeta.ETag,
		})
		if err != nil {
			l.Error(fmt.Sprintf("failed to force delete account: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to force delete account"))
		}
	} else {
		provider, err := s.providers.Load(ctx, providerMeta)
		if err != nil {
			l.Error(fmt.Sprintf("failed to load provider: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load provider"))
		}
		s.scheduler.Schedule(func(ctx context.Context) error {
			err = dynamitedb.Update(ctx, s.oltp, &oltp.Account{
				ProviderID: dynamitedb.Key(accountMeta.ProviderID.Value()),
				AccountID:  dynamitedb.Key(accountMeta.AccountID.Value()),
				ETag:       accountMeta.ETag,
				State:      dynamitedb.Set(cloud.AccountState_Deleting),
			})
			if err != nil {
				return fmt.Errorf("locking account state: %w", err)
			}
			err = provider.Delete(ctx, accountMeta.TargetID.Value())
			if err != nil {
				return fmt.Errorf("deleting account (%q): %w", accountMeta.AccountID.Value(), err)
			}
			if err = dynamitedb.Delete(ctx, s.oltp, &oltp.Account{
				ProviderID: dynamitedb.Key(accountMeta.ProviderID.Value()),
				AccountID:  dynamitedb.Key(accountMeta.AccountID.Value()),
			}); err != nil {
				return err
			}
			return nil
		}, func(ctx context.Context, err error) error {
			l.Warn(fmt.Sprintf("failed to delete account (%q): %v", accountMeta.AccountID.Value(), err))
			return dynamitedb.Update(ctx, s.oltp, &oltp.Account{
				ProviderID: dynamitedb.Key(accountMeta.ProviderID.Value()),
				AccountID:  dynamitedb.Key(accountMeta.AccountID.Value()),
				State:      dynamitedb.Set(cloud.AccountState_Corrupted),
				Error:      dynamitedb.Set(err.Error()),
			})
		})
	}
	return &connect.Response[account.DeleteResponse]{Msg: &account.DeleteResponse{}}, nil
}
