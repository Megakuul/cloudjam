package challenge

import (
	"context"
	"encoding/json"
	"fmt"

	extism "github.com/extism/go-sdk"
)

// RegisterHost registers a host function to the guest that preserves input and output structures (useful for large binary transfer).
// Exported to allow external systems like jamctl to also use the same bridge logic.
func RegisterHost[In, Out ~[]byte](name string, callback func(context.Context, In) (Out, error), report func(error)) extism.HostFunction {
	transformer := func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) (uint64, error) {
		if len(stack) < 1 {
			return 0, fmt.Errorf("invalid input")
		}
		rawInput, err := p.ReadBytes(stack[0])
		if err != nil {
			return 0, err
		}
		output, err := callback(ctx, In(rawInput))
		if err != nil {
			return 0, fmt.Errorf("%s: %w", name, err)
		}
		return p.WriteBytes(output)
	}
	return createHostFunction(name, transformer, report)
}

// RegisterInHost registers a host function to the guest that encodes input but preserves output (useful for large guest binary download).
func RegisterInHost[In any, Out ~[]byte](name string, callback func(context.Context, *In) (Out, error), report func(error)) extism.HostFunction {
	transformer := func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) (uint64, error) {
		if len(stack) < 1 {
			return 0, fmt.Errorf("invalid input")
		}
		rawInput, err := p.ReadBytes(stack[0])
		if err != nil {
			return 0, err
		}
		var input In
		if err = json.Unmarshal(rawInput, &input); err != nil {
			return 0, err
		}
		output, err := callback(ctx, &input)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", name, err)
		}
		return p.WriteBytes(output)
	}
	return createHostFunction(name, transformer, report)
}

// RegisterOutHost registers a host function to the guest that preserves input but decodes output (useful for large guest binary upload).
func RegisterOutHost[In ~[]byte, Out any](name string, callback func(context.Context, In) (*Out, error), report func(error)) extism.HostFunction {
	transformer := func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) (uint64, error) {
		if len(stack) < 1 {
			return 0, fmt.Errorf("invalid input")
		}
		rawInput, err := p.ReadBytes(stack[0])
		if err != nil {
			return 0, err
		}
		output, err := callback(ctx, In(rawInput))
		if err != nil {
			return 0, fmt.Errorf("%s: %w", name, err)
		}
		rawOutput, err := json.Marshal(output)
		if err != nil {
			return 0, err
		}
		return p.WriteBytes(rawOutput)
	}
	return createHostFunction(name, transformer, report)
}

// RegisterInOutHost registers a host function to the guest that serializes input and output structures (useful for normal data transfer).
func RegisterInOutHost[In, Out any](name string, callback func(context.Context, *In) (*Out, error), report func(error)) extism.HostFunction {
	transformer := func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) (uint64, error) {
		if len(stack) < 1 {
			return 0, fmt.Errorf("invalid input")
		}
		rawInput, err := p.ReadBytes(stack[0])
		if err != nil {
			return 0, err
		}
		var input In
		if err = json.Unmarshal(rawInput, &input); err != nil {
			return 0, err
		}
		output, err := callback(ctx, &input)
		if err != nil {
			return 0, fmt.Errorf("%s: %w", name, err)
		}
		rawOutput, err := json.Marshal(output)
		if err != nil {
			return 0, err
		}
		return p.WriteBytes(rawOutput)
	}
	return createHostFunction(name, transformer, report)
}

func createHostFunction(
	name string,
	transformer func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) (uint64, error),
	report func(error),
) extism.HostFunction {
	return extism.NewHostFunctionWithStack(name, func(ctx context.Context, p *extism.CurrentPlugin, stack []uint64) {
		offset, err := transformer(ctx, p, stack)
		if err != nil {
			report(err)
		}
		stack[0] = offset
	},
		[]extism.ValueType{extism.ValueTypePTR},
		[]extism.ValueType{extism.ValueTypePTR},
	)
}
