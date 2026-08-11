package challenge

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/internal/provider/cache"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud"
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/play"
	"connectrpc.com/connect"
	extism "github.com/extism/go-sdk"
	"github.com/klauspost/compress/zstd"
	"github.com/megakuul/dynamitedb"
	"github.com/megakuul/lake"
)

type Runner struct {
	rootCtx context.Context
	wg      sync.WaitGroup

	logger *slog.Logger
	oltp   *dynamitedb.Bucket
	olap   *lake.Bucket

	providerCache *cache.Cache
	pluginCache   *Cache
}

func NewRunner(rootCtx context.Context) *Runner {
	return &Runner{
		rootCtx: rootCtx,
	}
}

func (r *Runner) loadPluginBinary(ctx context.Context, providerID, definitionID string) (extism.Wasm, error) {
	definitionBinary, err := dynamitedb.Get(ctx, r.oltp, &oltp.DefinitionBinary{
		ProviderID:   dynamitedb.Key(providerID),
		DefinitionID: dynamitedb.Key(definitionID),
	})
	if err != nil {
		return nil, err
	}
	wasmData := extism.WasmData{}
	switch cloud.CompressionMode(definitionBinary.Compression.Value()) {
	case cloud.CompressionMode_Zstd:
		wasmData.Data, err = zstd.DecodeTo(nil, definitionBinary.WASM.Value())
		if err != nil {
			return nil, fmt.Errorf("failed to zstd decode plugin: %w", err)
		}
	default:
		return nil, fmt.Errorf("unknown compression algorithm in challenge definition (%d)", definitionBinary.Compression.Value())
	}
	return wasmData, nil
}

func (r *Runner) Launch(ctx context.Context, challenge *oltp.Challenge) error {
	provider, err := r.providerCache.Load(ctx, challenge.DefinitionProviderID.Value())
	if err != nil {
		return err
	}
	definition, err := dynamitedb.Get(ctx, r.oltp, &oltp.Definition{
		ProviderID:   dynamitedb.Key(challenge.DefinitionProviderID.Value()),
		DefinitionID: dynamitedb.Key(challenge.DefinitionID.Value()),
		Scope:        dynamitedb.Eq(challenge.Scope.Value()),
	})
	if err != nil {
		return err
	}

	err = dynamitedb.Update(ctx, r.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(challenge.GameID.Value()),
		ChallengeID: dynamitedb.Key(challenge.ChallengeID.Value()),
		State:       dynamitedb.Set(play.ChallengeState_Running),
		Ends:        dynamitedb.Set(time.Now().Add(challenge.Duration.Value())),
	})
	if err != nil {
		return err
	}

	accounts, err := dynamitedb.Query(ctx, r.oltp, &oltp.Account{
		ProviderID: dynamitedb.Key(definition.ProviderID.Value()),
		AccountID:  dynamitedb.KeyPrefix(""),
		State:      dynamitedb.Eq(cloud.AccountState_Ready),
		BoundUntil: dynamitedb.Before(time.Now()),
		Scope:      dynamitedb.Eq(challenge.Scope.Value()),
	}, dynamitedb.WithLimit(1))
	if err != nil {
		return fmt.Errorf("failed to enumerate accounts: %w", err)
	}
	if len(accounts) < 1 {
		return connect.NewError(connect.CodeResourceExhausted, fmt.Errorf("no capacity: not enough accounts on challenge provider"))
	}
	account := accounts[0]
	err = dynamitedb.Update(ctx, r.oltp, &oltp.Account{
		ProviderID: dynamitedb.Key(account.ProviderID.Value()),
		AccountID:  dynamitedb.Key(account.AccountID.Value()),
		ETag:       account.ETag,
		State:      dynamitedb.Set(cloud.AccountState_Running),
		BoundUntil: dynamitedb.Set(time.Now().Add(challenge.Duration.Value())),
	})
	if err != nil {
		return fmt.Errorf("claiming account: %w", err)
	}

	access, err := provider.Access(ctx, account.AccountID.Value(), challenge.Duration.Value())
	if err != nil {
		return fmt.Errorf("creating access controller: %w", err)
	}
	assets, err := provider.Assets(ctx, account.AccountID.Value(), challenge.Duration.Value())
	if err != nil {
		return fmt.Errorf("creating asset controller: %w", err)
	}
	resources, err := provider.Resources(ctx, account.AccountID.Value(), challenge.Duration.Value())
	if err != nil {
		return fmt.Errorf("creating resource controller: %w", err)
	}

	r.wg.Go(func() {
		challengeCtx, challengeCancel := context.WithTimeout(r.rootCtx, challenge.Duration.Value())
		defer challengeCancel()

		challengeInstance := New(
			challenge.GameID.Value(), challenge.ChallengeID.Value(), challenge.TeamID.Value(),
			r.oltp, r.olap, access, assets, resources,
		)
		plugin, err := r.pluginCache.Load(challengeCtx, definition.Hash.Value(), func(ctx context.Context) (extism.Wasm, error) {
			return r.loadPluginBinary(challengeCtx, definition.DefinitionID.Value(), definition.ProviderID.Value())
		}, challengeInstance.registerHost(challengeCtx)...)
		if err != nil {
			r.logger.Error(fmt.Sprintf("loading plugin: %v", err))
			return
		}
		defer plugin.Close(r.rootCtx)

		_, _, err = plugin.CallWithContext(ctx, "_start", nil)
		if err != nil && ctx.Err() == nil {
			r.logger.Error(fmt.Sprintf("start plugin: %v", err))
			return
		}
		err = dynamitedb.Update(challengeCtx, r.oltp, &oltp.Challenge{
			GameID:      dynamitedb.Key(challenge.GameID.Value()),
			ChallengeID: dynamitedb.Key(challenge.ChallengeID.Value()),
			State:       dynamitedb.Set(play.ChallengeState_Finished),
		})
		if err != nil {
			r.logger.Error(fmt.Sprintf("failed to report challenge status: %v", err))
			return
		}
	})
	return nil
}

func (r *Runner) Wait() {
	r.wg.Wait()
}
