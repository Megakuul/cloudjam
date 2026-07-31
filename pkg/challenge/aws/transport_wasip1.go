//go:build wasip1

package aws

import (
	"encoding/json"
	"fmt"

	"codeberg.org/megakuul/cloudjam/pkg/challenge"
	"github.com/extism/go-pdk"
)

// Host functions provided by the WASM host behind pkg/challenge. Each takes an
// extism memory offset holding the JSON-encoded input struct and returns an
// offset holding the JSON-encoded output struct (0 when there is no output).
//
// The import names are string literals here (the //go:wasmimport directive
// cannot reference a constant); the init below asserts they still match the
// pkg/challenge contract so any drift fails loudly at plugin startup.

//go:wasmimport extism:host/user init
func hostInit(uint64) uint64

//go:wasmimport extism:host/user read_score
func hostReadScore(uint64) uint64

//go:wasmimport extism:host/user update_score
func hostUpdateScore(uint64) uint64

//go:wasmimport extism:host/user create_resource
func hostCreateResource(uint64) uint64

//go:wasmimport extism:host/user register_resource
func hostReadResource(uint64) uint64

//go:wasmimport extism:host/user update_resource
func hostUpdateResource(uint64) uint64

//go:wasmimport extism:host/user delete_resource
func hostDeleteResource(uint64) uint64

func init() {
	for got, want := range map[string]string{
		challenge.Init:           "init",
		challenge.ReadScore:      "read_score",
		challenge.UpdateScore:    "update_score",
		challenge.CreateResource: "create_resource",
		challenge.ReadResource:   "register_resource",
		challenge.UpdateResource: "update_resource",
		challenge.DeleteResource: "delete_resource",
	} {
		if got != want {
			panic(fmt.Sprintf("aws: host function name %q drifted from wasm import %q; "+
				"update pkg/challenge/aws/transport_wasip1.go", got, want))
		}
	}
}

// pdkHost implements host by marshalling to/from JSON and shuttling the bytes
// across the extism boundary.
type pdkHost struct{}

func newHost() host { return pdkHost{} }

// invoke marshals in, calls the host function, and returns the raw output bytes.
func invoke(fn func(uint64) uint64, in any) ([]byte, error) {
	data, err := json.Marshal(in)
	if err != nil {
		return nil, fmt.Errorf("marshal host request: %w", err)
	}
	mem := pdk.AllocateBytes(data)
	defer mem.Free()

	out := fn(mem.Offset())
	if out == 0 {
		return nil, nil
	}
	rmem := pdk.FindMemory(out)
	return rmem.ReadBytes(), nil
}

// call is invoke plus output decoding into a typed struct.
func call(fn func(uint64) uint64, in any, out any) error {
	raw, err := invoke(fn, in)
	if err != nil {
		return err
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode host response: %w", err)
	}
	return nil
}

func (pdkHost) RegisterMeta(m challenge.InitInput) error {
	return call(hostInit, m, nil)
}

func (pdkHost) ReadScore() (float64, error) {
	var out challenge.ReadScoreOutput
	if err := call(hostReadScore, challenge.ReadScoreInput{}, &out); err != nil {
		return 0, err
	}
	return out.Score, nil
}

func (pdkHost) UpdateScore(reason string, increment float64) error {
	return call(hostUpdateScore, challenge.UpdateScoreInput{Reason: reason, Increment: increment}, nil)
}

func (pdkHost) CreateResource(typeName, desired string) error {
	return call(hostCreateResource, challenge.CreateResourceInput{Type: typeName, Desired: desired}, nil)
}

func (pdkHost) ReadResource(typeName, identifier string) (string, error) {
	var out challenge.ReadResourceOutput
	if err := call(hostReadResource, challenge.ReadResourceInput{Type: typeName, Identifier: identifier}, &out); err != nil {
		return "", err
	}
	return out.State, nil
}

func (pdkHost) UpdateResource(typeName, identifier, patch string) error {
	return call(hostUpdateResource, challenge.UpdateResourceInput{Type: typeName, Identifier: identifier, Patch: patch}, nil)
}

func (pdkHost) DeleteResource(typeName, identifier string) error {
	return call(hostDeleteResource, challenge.DeleteResourceInput{Type: typeName, Identifier: identifier}, nil)
}

// Completed / MarkCompleted use extism vars, which persist across host calls to
// the same plugin instance, so an already-awarded objective stays awarded.
func (pdkHost) Completed(id string) (bool, error) {
	return pdk.GetVar("done:"+id) != nil, nil
}

func (pdkHost) MarkCompleted(id string) error {
	pdk.SetVar("done:"+id, []byte{1})
	return nil
}

func (pdkHost) Log(msg string) { pdk.Log(pdk.LogInfo, msg) }

// init is the entry point the host calls first: it registers metadata, runs the
// challenge Setup, and performs an initial objective sweep.
//
//go:wasmexport init
func exportInit() int32 { return dispatch(phaseInit) }

// evaluate re-checks objectives and awards any newly reached goals. The host
// calls it whenever it wants to re-score the account.
//
//go:wasmexport evaluate
func exportEvaluate() int32 { return dispatch(phaseEvaluate) }

func dispatch(p phase) int32 {
	if registered == nil {
		pdk.SetErrorString("aws: no challenge registered; call aws.Serve in the plugin's init()")
		return 1
	}
	if err := registered.validate(); err != nil {
		pdk.SetError(err)
		return 1
	}
	if err := registered.run(newHost(), p); err != nil {
		pdk.SetError(err)
		return 1
	}
	pdk.OutputString("ok")
	return 0
}
