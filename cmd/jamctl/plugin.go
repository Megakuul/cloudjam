package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/provider"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
	extism "github.com/extism/go-sdk"
	"github.com/tetratelabs/wazero"
)

// host implements the plugin api. internal/challenge implements the same calls
// against dynamitedb and a real game; here the score is a float and a log line.
type host struct {
	score     float64
	resources provider.ResourceController
}

// runPlugin compiles the plugin, installs the plugin api as host functions and
// calls _start. A plugin is a wasip1 command module whose main is the challenge
// loop, so cancelling ctx is the normal way to end a run.
func runPlugin(ctx context.Context, source string, resources provider.ResourceController, timeout time.Duration) error {
	slog.Info("compiling plugin", "source", source)
	module, err := compile(ctx, source, "")
	if err != nil {
		return err
	}
	if module != source {
		defer os.Remove(module)
	}

	h := &host{resources: resources}
	plugin, err := extism.NewCompiledPlugin(ctx,
		extism.Manifest{Wasm: []extism.Wasm{extism.WasmFile{Path: module}}},
		extism.PluginConfig{
			EnableWasi: true,
			// Without this a cancelled context cannot interrupt the guest, and a
			// challenge loop would keep running after ctrl-c.
			RuntimeConfig: wazero.NewRuntimeConfig().WithCloseOnContextDone(true),
		},
		[]extism.HostFunction{
			hostFunction(api.InitName, h.init),
			hostFunction(api.ReportName, h.report),
			hostFunction(api.ReadScoreName, h.readScore),
			hostFunction(api.UpdateScoreName, h.updateScore),
			hostFunction(api.CreateResourceName, h.createResource),
			hostFunction(api.ReadResourceName, h.readResource),
			hostFunction(api.UpdateResourceName, h.updateResource),
			hostFunction(api.DeleteResourceName, h.deleteResource),
			hostFunction(api.ListResourceName, h.listResource),
		},
	)
	if err != nil {
		return fmt.Errorf("load plugin: %w", err)
	}
	defer plugin.Close(context.WithoutCancel(ctx))

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

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	_, _, err = instance.CallWithContext(ctx, "_start", nil)
	slog.Info("plugin finished", "score", h.score)
	if err != nil && ctx.Err() == nil {
		return err
	}
	// A cancelled context means ctrl-c or the timeout, which is how a challenge
	// normally ends. wazero reports that as a trap, so ctx is the only signal.
	return nil
}

func (h *host) init(ctx context.Context, in *api.InitInput) (*api.InitOutput, error) {
	slog.Info("challenge registered", "title", in.Title)
	if in.Description != "" {
		slog.Info(in.Description)
	}
	for id, clue := range in.Clues {
		slog.Info(clue, "clue", id)
	}
	return &api.InitOutput{}, nil
}

func (h *host) report(ctx context.Context, in *api.ReportInput) (*api.ReportOutput, error) {
	slog.Warn(in.Error, "source", "plugin")
	return &api.ReportOutput{}, nil
}

func (h *host) readScore(ctx context.Context, in *api.ReadScoreInput) (*api.ReadScoreOutput, error) {
	return &api.ReadScoreOutput{Score: h.score}, nil
}

func (h *host) updateScore(ctx context.Context, in *api.UpdateScoreInput) (*api.UpdateScoreOutput, error) {
	h.score += in.Increment
	slog.Info(fmt.Sprintf("%+g points — %s", in.Increment, in.Reason), "score", h.score)
	return &api.UpdateScoreOutput{}, nil
}

func (h *host) createResource(ctx context.Context, in *api.CreateResourceInput) (*api.CreateResourceOutput, error) {
	slog.Info("creating resource", "type", in.Type)
	identifier, err := h.resources.Create(ctx, in.Type, in.Desired)
	if err != nil {
		return nil, err
	}
	slog.Info("created resource", "type", in.Type, "identifier", identifier)
	return &api.CreateResourceOutput{Identifier: identifier}, nil
}

func (h *host) readResource(ctx context.Context, in *api.ReadResourceInput) (*api.ReadResourceOutput, error) {
	state, err := h.resources.Read(ctx, in.Type, in.Identifier)
	return &api.ReadResourceOutput{State: state}, err
}

func (h *host) updateResource(ctx context.Context, in *api.UpdateResourceInput) (*api.UpdateResourceOutput, error) {
	slog.Info("updating resource", "type", in.Type, "identifier", in.Identifier)
	return &api.UpdateResourceOutput{}, h.resources.Update(ctx, in.Type, in.Identifier, in.Patch)
}

func (h *host) deleteResource(ctx context.Context, in *api.DeleteResourceInput) (*api.DeleteResourceOutput, error) {
	slog.Info("deleting resource", "type", in.Type, "identifier", in.Identifier)
	return &api.DeleteResourceOutput{}, h.resources.Delete(ctx, in.Type, in.Identifier)
}

func (h *host) listResource(ctx context.Context, in *api.ListResourceInput) (*api.ListResourceOutput, error) {
	resources, err := h.resources.List(ctx, in.Type)
	return &api.ListResourceOutput{Resources: resources}, err
}

// hostFunction adapts a handler to the plugin abi: one pointer in, one pointer
// out, json on both sides.
//
// A failed call returns the null pointer, which the guest sdk reads as an empty
// response — the abi has no way to hand an error back — so the error is logged
// here or it is lost.
func hostFunction[In, Out any](name string, handler func(context.Context, *In) (*Out, error)) extism.HostFunction {
	call := func(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) (uint64, error) {
		raw, err := plugin.ReadBytes(stack[0])
		if err != nil {
			return 0, err
		}
		var in In
		if err := json.Unmarshal(raw, &in); err != nil {
			return 0, err
		}
		out, err := handler(ctx, &in)
		if err != nil {
			return 0, err
		}
		encoded, err := json.Marshal(out)
		if err != nil {
			return 0, err
		}
		return plugin.WriteBytes(encoded)
	}

	return extism.NewHostFunctionWithStack(name,
		func(ctx context.Context, plugin *extism.CurrentPlugin, stack []uint64) {
			offset, err := call(ctx, plugin, stack)
			if err != nil {
				slog.Error(fmt.Sprintf("%s: %v", name, err))
			}
			stack[0] = offset
		},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}
