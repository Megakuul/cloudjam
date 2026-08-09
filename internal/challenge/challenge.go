package challenge

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"os"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/internal/provider"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
	extism "github.com/extism/go-sdk"
	"github.com/google/uuid"
	"github.com/klauspost/compress/zstd"
	"github.com/megakuul/dynamitedb"
	"github.com/megakuul/lake"
	"github.com/tetratelabs/wazero"
)

type Challenge struct {
	logger *slog.Logger
	oltp   *dynamitedb.Bucket
	olap   *lake.Bucket

	access    provider.AccessController
	assets    provider.AssetController
	resources provider.ResourceController

	providerID     string
	definitionID   string
	definitionName string

	gameID      string
	challengeID string
	teamID      string
	teamName    string
	playerPubID string
	scope       string
}

func (c *Challenge) Start(ctx context.Context) error {
	definitionBinary, err := dynamitedb.Get(ctx, c.oltp, &oltp.DefinitionBinary{
		ProviderID:   dynamitedb.Key(c.providerID),
		DefinitionID: dynamitedb.Key(c.definitionID),
	})
	if err != nil {
		return err
	}
	if err := dynamitedb.Create(ctx, c.oltp, &oltp.Challenge{
		GameID:         dynamitedb.Key(c.gameID),
		ChallengeID:    dynamitedb.Key(c.challengeID),
		DefinitionID:   dynamitedb.Set(c.definitionID),
		DefinitionName: dynamitedb.Set(c.definitionName),
		Scope:          dynamitedb.Set(c.scope),
	}); err != nil {
		return err
	}
	if err := dynamitedb.Create(ctx, c.oltp, &oltp.Team{
		GameID: dynamitedb.Key(c.gameID),
		TeamID: dynamitedb.Key(c.teamID),
		Name:   dynamitedb.Set(c.teamName),
		// Players: ,
		Scope: dynamitedb.Set(c.scope),
	}); err != nil {
		return err
	}
	report := func(err error) {
		c.logger.Error(err.Error())
		if dErr := dynamitedb.Update(ctx, c.oltp, &oltp.Challenge{
			GameID:      dynamitedb.Key(c.gameID),
			ChallengeID: dynamitedb.Key(c.challengeID),
			Errors:      dynamitedb.Append(err.Error()),
		}); dErr != nil {
			c.logger.Warn(dErr.Error())
		}
	}

	wasmData := extism.WasmData{}
	switch definitionBinary.Compression.Value() {
	case oltp.CompressionZstd:
		wasmData.Data, err = zstd.DecodeTo(nil, definitionBinary.WASM.Value())
		if err != nil {
			return fmt.Errorf("failed to zstd decode plugin: %w", err)
		}
	default:
		return fmt.Errorf("unknown compression algorithm in challenge definition (%d)", definitionBinary.Compression.Value())
	}
	plugin, err := extism.NewCompiledPlugin(ctx,
		extism.Manifest{Wasm: []extism.Wasm{wasmData}},
		extism.PluginConfig{
			EnableWasi: true,
			// Without this a cancelled context cannot interrupt the guest, and a
			// challenge loop would keep running after ctrl-c.
			RuntimeConfig: wazero.NewRuntimeConfig().WithCloseOnContextDone(true),
		},
		[]extism.HostFunction{
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
		})
	if err != nil {
		return nil
	}
	instance, err := plugin.Instance(ctx, extism.PluginInstanceConfig{
		// The defaults are a frozen clock and a sleep that returns immediately,
		// which would collapse every challenge interval into one hot spin.
		ModuleConfig: wazero.NewModuleConfig().
			WithSysWalltime().
			WithSysNanotime().
			WithSysNanosleep().
			WithRandSource(rand.Reader).
			WithStdout(os.Stdout).
			WithStderr(os.Stderr),
	})
	if err != nil {
		return fmt.Errorf("instantiate plugin: %w", err)
	}
	defer instance.Close(context.WithoutCancel(ctx))

	_, _, err = instance.CallWithContext(ctx, "_start", nil)
	if err != nil && ctx.Err() == nil {
		return err
	}
	defer plugin.Close(ctx)

	return nil
}

func (c *Challenge) Report(ctx context.Context, input *api.ReportInput) (*api.ReportOutput, error) {
	err := dynamitedb.Update(ctx, c.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(c.gameID),
		ChallengeID: dynamitedb.Key(c.challengeID),
		Errors:      dynamitedb.Append(input.Error),
	})
	return &api.ReportOutput{}, err
}

func (c *Challenge) Log(ctx context.Context, input *api.LogInput) (*api.LogOutput, error) {
	c.logger.Log(ctx, input.Severity, input.Message)
	return &api.LogOutput{}, nil
}

func (c *Challenge) CreateMeta(ctx context.Context, input *api.CreateMetaInput) (*api.CreateMetaOutput, error) {
	err := dynamitedb.Update(ctx, c.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(c.gameID),
		ChallengeID: dynamitedb.Key(c.challengeID),
		Title:       dynamitedb.Set(input.Title),
		Description: dynamitedb.Set(input.Descriptions),
		Clues:       dynamitedb.Set(input.Clues),
		Assets:      dynamitedb.Set(input.Assets),
	})
	return &api.CreateMetaOutput{}, err
}

func (c *Challenge) UpdateMeta(ctx context.Context, input *api.UpdateMetaInput) (*api.UpdateMetaOutput, error) {
	err := dynamitedb.Update(ctx, c.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(c.gameID),
		ChallengeID: dynamitedb.Key(c.challengeID),
		Description: dynamitedb.Append(input.AdditionalDescriptions...),
		Clues:       dynamitedb.Emplace(input.AdditionalClues),
		Assets:      dynamitedb.Emplace(input.AdditionalAssets),
	})
	return &api.UpdateMetaOutput{}, err
}

func (c *Challenge) ReadScore(ctx context.Context, input *api.ReadScoreInput) (*api.ReadScoreOutput, error) {
	team, err := dynamitedb.Get(ctx, c.oltp, &oltp.Team{
		GameID: dynamitedb.Key(c.gameID),
		TeamID: dynamitedb.Key(c.teamID),
	})
	if err != nil {
		return nil, err
	}
	return &api.ReadScoreOutput{
		Score: team.Score.Value(),
	}, nil
}

func (c *Challenge) UpdateScore(ctx context.Context, input *api.UpdateScoreInput) (*api.UpdateScoreOutput, error) {
	err := dynamitedb.Update(ctx, c.oltp, &oltp.Team{
		GameID: dynamitedb.Key(c.gameID),
		TeamID: dynamitedb.Key(c.teamID),
		Score:  dynamitedb.Increment(input.Increment),
	})
	if err != nil {
		return nil, err
	}
	err = dynamitedb.Update(ctx, c.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(c.gameID),
		ChallengeID: dynamitedb.Key(c.challengeID),
		ScoreEvents: dynamitedb.Append(oltp.ScoreEvent{
			Timestamp: time.Now(),
			Text:      input.Reason,
			Change:    input.Increment,
		}),
	})
	return &api.UpdateScoreOutput{}, err
}

func (c *Challenge) CreateAsset(ctx context.Context, input api.CreateAssetInput) (*api.CreateAssetOutput, error) {
	if len(input) > 50_000_000 {
		return nil, fmt.Errorf("assets larger then 50 MB are not supported")
	}
	name := uuid.NewString()
	url, err := c.assets.Create(ctx, name, bytes.NewReader(input))
	if err != nil {
		return nil, err
	}
	return &api.CreateAssetOutput{
		Name: name,
		URL:  url,
	}, nil
}

func (c *Challenge) UpdateAsset(ctx context.Context, input *api.UpdateAssetInput) (*api.UpdateAssetOutput, error) {
	url, err := c.assets.Update(ctx, input.OldName, input.NewName)
	if err != nil {
		return nil, err
	}
	return &api.UpdateAssetOutput{
		NewURL: url,
	}, nil
}

func (c *Challenge) CreatePermission(ctx context.Context, input *api.CreatePermissionInput) (*api.CreatePermissionOutput, error) {
	err := c.access.CreatePermission(ctx, input.Permission)
	return &api.CreatePermissionOutput{}, err
}

func (c *Challenge) UpdatePermission(ctx context.Context, input *api.UpdatePermissionInput) (*api.UpdatePermissionOutput, error) {
	err := c.access.UpdatePermission(ctx, input.Permission)
	return &api.UpdatePermissionOutput{}, err
}

func (c *Challenge) CreateGuardrail(ctx context.Context, input *api.CreateGuardrailInput) (*api.CreateGuardrailOutput, error) {
	err := c.access.CreateGuardrail(ctx, input.Guardrail)
	return &api.CreateGuardrailOutput{}, err
}

func (c *Challenge) UpdateGuardrail(ctx context.Context, input *api.UpdateGuardrailInput) (*api.UpdateGuardrailOutput, error) {
	err := c.access.UpdateGuardrail(ctx, input.Guardrail)
	return &api.UpdateGuardrailOutput{}, err
}

func (c *Challenge) CreateResource(ctx context.Context, input *api.CreateResourceInput) (*api.CreateResourceOutput, error) {
	id, err := c.resources.Create(ctx, input.Type, input.Desired)
	return &api.CreateResourceOutput{Identifier: id}, err
}

func (c *Challenge) ReadResource(ctx context.Context, input *api.ReadResourceInput) (*api.ReadResourceOutput, error) {
	state, err := c.resources.Read(ctx, input.Type, input.Identifier)
	return &api.ReadResourceOutput{State: state}, err
}

func (c *Challenge) UpdateResource(ctx context.Context, input *api.UpdateResourceInput) (*api.UpdateResourceOutput, error) {
	err := c.resources.Update(ctx, input.Type, input.Identifier, input.Patch)
	return &api.UpdateResourceOutput{}, err
}

func (c *Challenge) DeleteResource(ctx context.Context, input *api.DeleteResourceInput) (*api.DeleteResourceOutput, error) {
	err := c.resources.Delete(ctx, input.Type, input.Identifier)
	return &api.DeleteResourceOutput{}, err
}

func (c *Challenge) ListResource(ctx context.Context, input *api.ListResourceInput) (*api.ListResourceOutput, error) {
	resources, err := c.resources.List(ctx, input.Type)
	return &api.ListResourceOutput{Resources: resources}, err
}
