package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/internal/provider/cache"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud"
	"github.com/megakuul/dynamitedb"
	"github.com/megakuul/lake"
)

type Scheduler struct {
	rootCtx context.Context
	wg      sync.WaitGroup

	logger *slog.Logger
	oltp   *dynamitedb.Bucket
	olap   *lake.Bucket

	providerCache *cache.Cache
}

func New(rootCtx context.Context) *Scheduler {
	return &Scheduler{
		rootCtx: rootCtx,
	}
}

func (s *Scheduler) launchBackground(fn func() error, report func(error) error) {
	s.wg.Go(func() {
		if err := fn(); err != nil {
			if rErr := report(err); rErr != nil {
				s.logger.Error(fmt.Sprintf("failed to report an error (%v): %v", rErr, err))
			}
		}
	})
}

func (s *Scheduler) ProvisionAccount(providerID, accountID string) {
	s.launchBackground(func() error {
		account, err := dynamitedb.Get(s.rootCtx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(providerID),
			AccountID:  dynamitedb.Key(accountID),
		})
		if err != nil {
			return err
		}
		if account.State.Value() != cloud.AccountState_NotCreated {
			return nil // account already provisioning (making this partly idempotent).
		}
		provider, err := s.providerCache.Load(s.rootCtx, providerID)
		if err != nil {
			return err
		}
		err = dynamitedb.Update(s.rootCtx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(providerID),
			AccountID:  dynamitedb.Key(accountID),
			ETag:       account.ETag,
			State:      dynamitedb.Set(cloud.AccountState_Provisioning),
		})
		if err != nil {
			return fmt.Errorf("locking account state: %w", err)
		}
		id, err := provider.Provision(s.rootCtx, account.Name.Value())
		if err != nil {
			return fmt.Errorf("provision account: %w", err)
		}
		if err = dynamitedb.Update(s.rootCtx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(providerID),
			AccountID:  dynamitedb.Key(accountID),
			TargetID:   dynamitedb.Set(id),
		}); err != nil {
			return err
		}
		err = provider.Prepare(s.rootCtx, id)
		if err != nil {
			return fmt.Errorf("prepare account: %w", err)
		}
		if err = dynamitedb.Update(s.rootCtx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(providerID),
			AccountID:  dynamitedb.Key(accountID),
			State:      dynamitedb.Set(cloud.AccountState_Ready),
		}); err != nil {
			return err
		}
		return nil
	}, func(err error) error {
		s.logger.Warn(fmt.Sprintf("failed to create account: %v", err))
		return dynamitedb.Update(s.rootCtx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(providerID),
			AccountID:  dynamitedb.Key(accountID),
			State:      dynamitedb.Set(cloud.AccountState_Corrupted),
			Error:      dynamitedb.Set(err.Error()),
		})
	})
}

func (s *Scheduler) EvictAccount(providerID, accountID string) {
	s.launchBackground(func() error {
		account, err := dynamitedb.Get(s.rootCtx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(providerID),
			AccountID:  dynamitedb.Key(accountID),
		})
		if err != nil {
			return err
		}
		switch account.State.Value() {
		case cloud.AccountState_Evicting, cloud.AccountState_Ready:
			return nil // account already evicting or evicted (making this partly idempotent).
		case cloud.AccountState_Running:
		default:
			return fmt.Errorf("account eviction scheduled despite not being in running state")
		}
		if time.Now().Before(account.BoundUntil.Value()) {
			return fmt.Errorf("account eviction scheduled despite being used by a challenge")
		}
		provider, err := s.providerCache.Load(s.rootCtx, providerID)
		if err != nil {
			return err
		}
		err = dynamitedb.Update(s.rootCtx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(providerID),
			AccountID:  dynamitedb.Key(accountID),
			ETag:       account.ETag,
			State:      dynamitedb.Set(cloud.AccountState_Provisioning),
		})
		if err != nil {
			return fmt.Errorf("locking account state: %w", err)
		}
		id, err := provider.Provision(s.rootCtx, account.Name.Value())
		if err != nil {
			return fmt.Errorf("provision account: %w", err)
		}
		if err = dynamitedb.Update(s.rootCtx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(providerID),
			AccountID:  dynamitedb.Key(accountID),
			TargetID:   dynamitedb.Set(id),
		}); err != nil {
			return err
		}
		err = provider.Prepare(s.rootCtx, id)
		if err != nil {
			return fmt.Errorf("prepare account: %w", err)
		}
		if err = dynamitedb.Update(s.rootCtx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(providerID),
			AccountID:  dynamitedb.Key(accountID),
			State:      dynamitedb.Set(cloud.AccountState_Ready),
		}); err != nil {
			return err
		}
		return nil
	}, func(err error) error {
		s.logger.Warn(fmt.Sprintf("failed to evict account: %v", err))
		return dynamitedb.Update(s.rootCtx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(providerID),
			AccountID:  dynamitedb.Key(accountID),
			State:      dynamitedb.Set(cloud.AccountState_Corrupted),
			Error:      dynamitedb.Set(err.Error()),
		})
	})
}

func (s *Scheduler) DeleteAccount(providerID, accountID string) {
	s.launchBackground(func() error {
		account, err := dynamitedb.Get(s.rootCtx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(providerID),
			AccountID:  dynamitedb.Key(accountID),
		})
		if err != nil {
			return err
		}
		switch account.State.Value() {
		case cloud.AccountState_Deleting:
			return nil // account already deleting (making this partly idempotent).
		case cloud.AccountState_Disabled:
			// disabled must be given to ensure the account is not bound on a challenge or used in the same moment.
		default:
			return fmt.Errorf("account deletion scheduled despite not being in disabled state")
		}
		provider, err := s.providerCache.Load(s.rootCtx, providerID)
		if err != nil {
			return err
		}
		err = dynamitedb.Update(s.rootCtx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(providerID),
			AccountID:  dynamitedb.Key(accountID),
			ETag:       account.ETag,
			State:      dynamitedb.Set(cloud.AccountState_Deleting),
		})
		if err != nil {
			return fmt.Errorf("locking account state: %w", err)
		}
		err = provider.Delete(s.rootCtx, account.TargetID.Value())
		if err != nil {
			return fmt.Errorf("deleting account: %w", err)
		}
		if err = dynamitedb.Delete(s.rootCtx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(providerID),
			AccountID:  dynamitedb.Key(accountID),
		}); err != nil {
			return err
		}
		return nil
	}, func(err error) error {
		s.logger.Warn(fmt.Sprintf("failed to delete account: %v", err))
		return dynamitedb.Update(s.rootCtx, s.oltp, &oltp.Account{
			ProviderID: dynamitedb.Key(providerID),
			AccountID:  dynamitedb.Key(accountID),
			State:      dynamitedb.Set(cloud.AccountState_Corrupted),
			Error:      dynamitedb.Set(err.Error()),
		})
	})
}

func (s *Scheduler) Wait() {
	s.wg.Wait()
}
