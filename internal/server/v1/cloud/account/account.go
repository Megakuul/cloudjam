package account

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/oltp"
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
	oltp      *dynamitedb.Bucket
}

func New(logger *slog.Logger, scheduler *scheduler.Scheduler, oltp *dynamitedb.Bucket) *Server {
	return &Server{
		logger:    logger,
		scheduler: scheduler,
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
		AccountID: dynamitedb.KeyPrefix(""),
		Scope:     dynamitedb.In(auth.Scopes(ctx)...),
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

	provider, err := dynamitedb.Get(ctx, s.oltp, &oltp.Provider{
		ProviderID: dynamitedb.Key(req.Msg.Init.ProviderId),
		Scope:      dynamitedb.In(auth.Scopes(ctx)...),
	})
	if err != nil {
		if errors.Is(err, dynamitedb.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("provideprovider does not exist"))
		}
		l.Error(fmt.Sprintf("failed to fetch provider: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to fetch provider"))
	}

	err = dynamitedb.Create(ctx, s.oltp, &oltp.Account{
		ProviderID:  dynamitedb.Key(req.Msg.Init.ProviderId),
		AccountID:   dynamitedb.Key(req.Msg.Init.Id),
		Name:        dynamitedb.Set(req.Msg.Init.Name),
		Description: dynamitedb.Set(req.Msg.Init.Description),
		State:       dynamitedb.Set(cloud.AccountState_Provisioning),
		Scope:       dynamitedb.Set(provider.Scope.Value()),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to create account: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create account"))
	}
	s.scheduler.ProvisionAccount(req.Msg.Init.ProviderId, req.Msg.Init.Id)
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
	err = dynamitedb.Update(ctx, s.oltp, &oltp.Account{
		ProviderID:  targetAccount.ProviderID,
		AccountID:   targetAccount.AccountID,
		ETag:        targetAccount.ETag,
		Name:        dynamitedb.Set(req.Msg.Mod.Name),
		Description: dynamitedb.Set(req.Msg.Mod.Description),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to update account: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update account"))
	}
	return &connect.Response[account.UpdateResponse]{Msg: &account.UpdateResponse{}}, nil
}

func (s *Server) Evict(ctx context.Context, req *connect.Request[account.EvictRequest]) (*connect.Response[account.EvictResponse], error) {
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
	return &connect.Response[account.EvictResponse]{Msg: &account.EvictResponse{}}, nil
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
	if time.Now().Before(targetAccount.BoundUntil.Value()) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("account is currently used by a challenge"))
	}
	s.scheduler.EvictAccount(targetAccount.ProviderID.Value(), targetAccount.AccountID.Value())
	return &connect.Response[account.FixResponse]{Msg: &account.FixResponse{}}, nil
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[account.DeleteRequest]) (*connect.Response[account.DeleteResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	targetAccount, err := dynamitedb.Get(ctx, s.oltp, &oltp.Account{
		ProviderID: dynamitedb.Key(req.Msg.ProviderId),
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
	if time.Now().Before(targetAccount.BoundUntil.Value()) {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("account is currently used by a challenge"))
	}

	// with force remove the account just from here (this will leak the account on the provider however).
	if req.Msg.Force {
		err = dynamitedb.Delete(ctx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(targetAccount.ProviderID.Value()),
			AccountID:  dynamitedb.Key(targetAccount.AccountID.Value()),
			ETag:       targetAccount.ETag,
		})
		if err != nil {
			l.Error(fmt.Sprintf("failed to force delete account: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to force delete account"))
		}
	} else {
		err = dynamitedb.Update(ctx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(targetAccount.ProviderID.Value()),
			AccountID:  dynamitedb.Key(targetAccount.AccountID.Value()),
			ETag:       targetAccount.ETag,
			State:      dynamitedb.Set(cloud.AccountState_Disabled),
		})
		if err != nil {
			l.Error(fmt.Sprintf("failed to disable account: %v", err))
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to disable account"))
		}
		s.scheduler.DeleteAccount(targetAccount.ProviderID.Value(), targetAccount.AccountID.Value())
	}
	return &connect.Response[account.DeleteResponse]{Msg: &account.DeleteResponse{}}, nil
}
