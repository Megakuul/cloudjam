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
	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
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

func (r *Runner) Launch(ctx context.Context, gameID, challengeID string) error {
	challenge, err := dynamitedb.Get(ctx, r.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(gameID),
		ChallengeID: dynamitedb.Key(challengeID),
	})
	if err != nil {
		return fmt.Errorf("loading challenge: %w", err)
	}
	provider, err := r.providerCache.Load(ctx, challenge.DefinitionProviderID.Value())
	if err != nil {
		return err
	}
	definition, err := dynamitedb.Get(ctx, r.oltp, &oltp.Definition{
		ProviderID:   dynamitedb.Key(challenge.DefinitionProviderID.Value()),
		DefinitionID: dynamitedb.Key(challenge.DefinitionID.Value()),
	})
	if err != nil {
		return err
	}

	err = dynamitedb.Update(ctx, r.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(gameID),
		ChallengeID: dynamitedb.Key(challengeID),
		State:       dynamitedb.Set(play.ChallengeState_Running),
		Ends:        dynamitedb.Set(time.Now().Add(challenge.Duration.Value())),
	})
	if err != nil {
		return err
	}

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		challengeCtx, challengeCancel := context.WithTimeout(r.rootCtx, challenge.Duration.Value())
		defer challengeCancel()
		err = dynamitedb.Update(challengeCtx, r.oltp, &oltp.Challenge{
			GameID:      dynamitedb.Key(gameID),
			ChallengeID: dynamitedb.Key(challengeID),
			State:       dynamitedb.Set(play.ChallengeState_Finished),
		})
		if err != nil {
			r.logger.Error(fmt.Sprintf("failed to report challenge status: %w", err))
		}

		r.pluginCache.Load(ctx, definition.Hash.Value(), func(ctx context.Context) (extism.Wasm, error) {
			definitionBinary, err := dynamitedb.Get(ctx, r.oltp, &oltp.DefinitionBinary{
				ProviderID:   dynamitedb.Key(challenge.DefinitionProviderID.Value()),
				DefinitionID: dynamitedb.Key(challenge.DefinitionID.Value()),
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
		},
			RegisterInOutHost(api.ReportName, c.Report, report),
			RegisterInOutHost(api.LogName, c.Log, report),
			RegisterInOutHost(api.CreateMetaName, c.CreateMeta, report),
			RegisterInOutHost(api.UpdateMetaName, c.UpdateMeta, report),
			RegisterInOutHost(api.ReadScoreName, c.ReadScore, report),
			RegisterInOutHost(api.UpdateScoreName, c.UpdateScore, report),
			RegisterOutHost(api.CreateAssetName, c.CreateAsset, report),
			RegisterInOutHost(api.UpdateAssetName, c.UpdateAsset, report),
			RegisterInOutHost(api.CreatePermissionName, c.CreatePermission, report),
			RegisterInOutHost(api.UpdatePermissionName, c.UpdatePermission, report),
			RegisterInOutHost(api.CreateGuardrailName, c.CreateGuardrail, report),
			RegisterInOutHost(api.UpdateGuardrailName, c.UpdateGuardrail, report),
			RegisterInOutHost(api.CreateResourceName, c.CreateResource, report),
			RegisterInOutHost(api.ReadResourceName, c.ReadResource, report),
			RegisterInOutHost(api.UpdateResourceName, c.UpdateResource, report),
			RegisterInOutHost(api.DeleteResourceName, c.DeleteResource, report),
			RegisterInOutHost(api.ListResourceName, c.ListResource, report),
		)
		challenge.Start(r.rootCtx)
	}()
	return nil
}

func (r *Runner) Wait() {
	r.wg.Wait()
}
