package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"codeberg.org/megakuul/cloudjam/pkg/challenge"
	extism "github.com/extism/go-sdk"
	"github.com/megakuul/dynamitedb"
	"github.com/megakuul/lake"
)

// Challenge represents the current state of an account according to teh provider.
type Challenge struct {
	otlp dynamitedb.Bucket
	olap lake.Bucket

	challengeID string
	userID      string
}

func (c *Challenge) Start(ctx context.Context) error {
	manifests := extism.Manifest{
		Wasm: []extism.Wasm{
			extism.WasmData{},
		},
	}
	config := extism.PluginConfig{}
	plugin, err := extism.NewCompiledPlugin(ctx, manifests, config, []extism.HostFunction{
		extism.NewHostFunctionWithStack("", func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {}),
	})
	if err != nil {
		return nil
	}
	_, _, err = plugin.CallWithContext(ctx, "init", []byte{})
	if err != nil {
		return fmt.Errorf("challenge initialization: %w", err)
	}

	return nil
}

func (c *Challenge) Init(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) error {
	if len(stack) < 1 {
		return fmt.Errorf("invalid input")
	}
	rawInput, err := p.ReadBytes(stack[0])
	if err != nil {
		return err
	}
	input := &challenge.InitInput{}
	if err = json.Unmarshal(rawInput, input); err != nil {
		return err
	}
	return nil
}

func (c *Challenge) EmitPoints() string {
	return ""
}
