package challenge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/internal/provider"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
	extism "github.com/extism/go-sdk"
	"github.com/megakuul/dynamitedb"
	"github.com/megakuul/lake"
)

type Challenge struct {
	logger *slog.Logger
	oltp   *dynamitedb.Bucket
	olap   *lake.Bucket

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
		createHostFunction(api.InitName, c.Init, report),
		createHostFunction(api.ReportName, c.Report, report),
		createHostFunction(api.ReadScoreName, c.ReadScore, report),
		createHostFunction(api.UpdateScoreName, c.UpdateScore, report),
		createHostFunction(api.CreateResourceName, c.CreateResource, report),
		createHostFunction(api.ReadResourceName, c.ReadResource, report),
		createHostFunction(api.UpdateResourceName, c.UpdateResource, report),
		createHostFunction(api.DeleteResourceName, c.DeleteResource, report),
		createHostFunction(api.ListResourceName, c.ListResource, report),
	})
	if err != nil {
		return nil
	}
	defer plugin.Close(ctx)

	return nil
}

func createHostFunction[Input, Output any](name string, callback func(context.Context, *Input) (*Output, error), report func(error)) extism.HostFunction {
	transformer := func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) error {
		if len(stack) < 1 {
			return fmt.Errorf("invalid input")
		}
		rawInput, err := p.ReadBytes(stack[0])
		if err != nil {
			return err
		}
		var input Input
		if err = json.Unmarshal(rawInput, &input); err != nil {
			return err
		}
		output, err := callback(ctx, &input)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		rawOutput, err := json.Marshal(output)
		if err != nil {
			return err
		}
		_, err = p.WriteBytes(rawOutput)
		if err != nil {
			return err
		}
		return nil
	}
	return extism.NewHostFunctionWithStack(name, func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
		if err := transformer(ctx, p, stack); err != nil {
			report(err)
		}
	},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}

func (c *Challenge) Init(ctx context.Context, input *api.InitInput) (*api.InitOutput, error) {
	err := dynamitedb.Update(ctx, c.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(c.gameID),
		ChallengeID: dynamitedb.Key(c.challengeID),
		Title:       dynamitedb.Set(input.Title),
		Description: dynamitedb.Set(input.Description),
		Clues:       dynamitedb.Set(input.Clues),
	})
	return &api.InitOutput{}, err
}

func (c *Challenge) Report(ctx context.Context, input *api.ReportInput) (*api.ReportOutput, error) {
	err := dynamitedb.Update(ctx, c.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(c.gameID),
		ChallengeID: dynamitedb.Key(c.challengeID),
		Errors:      dynamitedb.Append(input.Error),
	})
	return &api.ReportOutput{}, err
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
