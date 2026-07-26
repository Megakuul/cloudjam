package account

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"codeberg.org/megakuul/cloudjam/internal/auth"
	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud/account"
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

func (s *Server) Get(ctx context.Context, req *connect.Request[account.GetRequest]) (*connect.Response[account.GetResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	foundAccount, err := dynamitedb.Get(ctx, s.bucket, &oltp.Account{
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
		Provider:     foundAccount.ProviderID.Value(),
		Id:           foundAccount.AccountID.Value(),
		Name:         foundAccount.Name.Value(),
		Description:  foundAccount.Description.Value(),
		Credentials:  "",
		State:        cloud.AccountState(foundAccount.State.Value()),
		DesiredState: cloud.AccountState(foundAccount.DesiredState.Value()),
		Scope:        foundAccount.Scope.Value(),
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
	accounts, err := dynamitedb.Query(ctx, s.bucket, &oltp.Account{
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
			Provider:     account.ProviderID.Value(),
			Id:           account.AccountID.Value(),
			Name:         account.Name.Value(),
			Description:  account.Description.Value(),
			Credentials:  "",
			State:        cloud.AccountState(account.State.Value()),
			DesiredState: cloud.AccountState(account.DesiredState.Value()),
			Scope:        account.Scope.Value(),
		})
	}

	return &connect.Response[account.ListResponse]{Msg: &account.ListResponse{
		Accounts: accountsOutput,
	}}, nil
}

func (s *Server) Update(ctx context.Context, req *connect.Request[account.UpdateRequest]) (*connect.Response[account.UpdateResponse], error) {
	l := s.logger.With("proc", req.Spec().Procedure)

	targetAccount, err := dynamitedb.Get(ctx, s.bucket, &oltp.Account{
		ProviderID: dynamitedb.Key(req.Msg.Mod.Provider),
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
	err = dynamitedb.Update(ctx, s.bucket, &oltp.Account{
		ProviderID:   targetAccount.ProviderID,
		AccountID:    targetAccount.AccountID,
		DesiredState: dynamitedb.Set(int(req.Msg.Mod.DesiredState)),
	})
	if err != nil {
		l.Error(fmt.Sprintf("failed to update account: %v", err))
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update account"))
	}
	return &connect.Response[account.UpdateResponse]{Msg: &account.UpdateResponse{}}, nil
}
