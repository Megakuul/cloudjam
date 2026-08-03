package challenge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"codeberg.org/megakuul/cloudjam/internal/oltp"
	"codeberg.org/megakuul/cloudjam/pkg/challenge"
	extism "github.com/extism/go-sdk"
	"github.com/megakuul/dynamitedb"
	"github.com/megakuul/lake"
)

type Challenge struct {
	logger *slog.Logger
	oltp   *dynamitedb.Bucket
	olap   *lake.Bucket

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
		createHostFunction(challenge.Init, c.Init, report),
		createHostFunction(challenge.ReadScore, c.ReadScore, report),
		createHostFunction(challenge.UpdateScore, c.UpdateScore, report),
		createHostFunction(challenge.CreateResource, c.CreateResource, report),
		createHostFunction(challenge.ReadResource, c.ReadResource, report),
		createHostFunction(challenge.UpdateResource, c.UpdateResource, report),
		createHostFunction(challenge.DeleteResource, c.DeleteResource, report),
		createHostFunction(challenge.ListResource, c.ListResource, report),
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

func (c *Challenge) Init(ctx context.Context, input *challenge.InitInput) (*challenge.InitOutput, error) {
	err := dynamitedb.Update(ctx, c.oltp, &oltp.Challenge{
		GameID:      dynamitedb.Key(c.gameID),
		ChallengeID: dynamitedb.Key(c.challengeID),
		Title:       dynamitedb.Set(input.Title),
		Description: dynamitedb.Set(input.Description),
		Clues:       dynamitedb.Set(input.Clues),
	})
	return &challenge.InitOutput{}, err
}

func (c *Challenge) ReadScore(ctx context.Context, input *challenge.ReadScoreInput) (*challenge.ReadScoreOutput, error) {
	return nil, nil
}

func (c *Challenge) UpdateScore(ctx context.Context, input *challenge.UpdateScoreInput) (*challenge.UpdateScoreOutput, error) {
	err := dynamitedb.Update(ctx, c.oltp, &oltp.Player{
		GameID:   dynamitedb.Key(c.gameID),
		PlayerID: dynamitedb.Key(c.playerID),
		// PlayerScore: dynamitedb.Increment(input.Increment),
	})
	return &challenge.UpdateScoreOutput{}, err
}

func (c *Challenge) CreateResource(ctx context.Context, input *challenge.CreateResourceInput) (*challenge.CreateResourceOutput, error) {
	return nil, nil
}

func (c *Challenge) ReadResource(ctx context.Context, input *challenge.ReadResourceInput) (*challenge.ReadResourceOutput, error) {
	return nil, nil
}

func (c *Challenge) UpdateResource(ctx context.Context, input *challenge.UpdateResourceInput) (*challenge.UpdateResourceOutput, error) {
	return nil, nil
}

func (c *Challenge) DeleteResource(ctx context.Context, input *challenge.DeleteResourceInput) (*challenge.DeleteResourceOutput, error) {
	return nil, nil
}

func (c *Challenge) ListResource(ctx context.Context, input *challenge.ListResourceInput) (*challenge.ListResourceOutput, error) {
	return nil, nil
}
