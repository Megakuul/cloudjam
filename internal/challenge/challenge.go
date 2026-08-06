package challenge

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/internal/provider"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
	extism "github.com/extism/go-sdk"
	"github.com/google/uuid"
	"github.com/megakuul/dynamitedb"
	"github.com/megakuul/lake"
)

type Challenge struct {
	logger *slog.Logger
	oltp   *dynamitedb.Bucket
	olap   *lake.Bucket

	assets    provider.AssetController
	resources provider.ResourceController

	providerID     string
	definitionID   string
	definitionName string

	gameID      string
	challengeID string
	playerID    string
	playerName  string
	playerPubID string
	scope       string
}

func (c *Challenge) Start(ctx context.Context) error {
	if err := dynamitedb.Create(ctx, c.oltp, &oltp.Challenge{
		GameID:         dynamitedb.Key(c.gameID),
		ChallengeID:    dynamitedb.Key(c.challengeID),
		DefinitionID:   dynamitedb.Set(c.definitionID),
		DefinitionName: dynamitedb.Set(c.definitionName),
		Scope:          dynamitedb.Set(c.scope),
	}); err != nil {
		return err
	}
	if err := dynamitedb.Create(ctx, c.oltp, &oltp.Player{
		GameID:   dynamitedb.Key(c.gameID),
		PlayerID: dynamitedb.Key(c.playerID),
		Username: dynamitedb.Set(c.playerName),
		PubID:    dynamitedb.Set(c.playerPubID),
		// PlayerScore: dynamitedb.Set(0.0),
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
	manifests := extism.Manifest{
		Wasm: []extism.Wasm{
			extism.WasmData{},
		},
	}
	config := extism.PluginConfig{}
	plugin, err := extism.NewCompiledPlugin(ctx, manifests, config, []extism.HostFunction{
		RegisterInOutHost(api.ReportName, c.Report, report),
		RegisterInOutHost(api.LogName, c.Log, report),
		RegisterInOutHost(api.CreateMetaName, c.CreateMeta, report),
		RegisterInOutHost(api.UpdateMetaName, c.UpdateMeta, report),
		RegisterInOutHost(api.ReadScoreName, c.ReadScore, report),
		RegisterInOutHost(api.UpdateScoreName, c.UpdateScore, report),
		RegisterOutHost(api.CreateAssetName, c.CreateAsset, report),
		RegisterInOutHost(api.UpdateAssetName, c.UpdateAsset, report),
		RegisterInOutHost(api.CreateResourceName, c.CreateResource, report),
		RegisterInOutHost(api.ReadResourceName, c.ReadResource, report),
		RegisterInOutHost(api.UpdateResourceName, c.UpdateResource, report),
		RegisterInOutHost(api.DeleteResourceName, c.DeleteResource, report),
		RegisterInOutHost(api.ListResourceName, c.ListResource, report),
	})
	if err != nil {
		return nil
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
	player, err := dynamitedb.Get(ctx, c.oltp, &oltp.Player{
		GameID:   dynamitedb.Key(c.gameID),
		PlayerID: dynamitedb.Key(c.playerID),
	})
	if err != nil {
		return nil, err
	}
	return &api.ReadScoreOutput{
		Score: player.Score.Value(),
	}, nil
}

func (c *Challenge) UpdateScore(ctx context.Context, input *api.UpdateScoreInput) (*api.UpdateScoreOutput, error) {
	err := dynamitedb.Update(ctx, c.oltp, &oltp.Player{
		GameID:   dynamitedb.Key(c.gameID),
		PlayerID: dynamitedb.Key(c.playerID),
		Score:    dynamitedb.Increment(input.Increment),
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
