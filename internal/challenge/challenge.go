package provider

import (
	"context"
	"fmt"

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
	plugin, err := extism.NewPlugin(ctx, manifests, config, []extism.HostFunction{})
	if err != nil {
		return nil
	}
	_, _, err = plugin.CallWithContext(ctx, "init", []byte{})
	if err != nil {
		return fmt.Errorf("challenge initialization: %w", err)
	}

	return nil
}

func (c *Challenge) EmitPoints() string {
	return ""
}
