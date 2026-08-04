//go:build wasip1

package aws

import (
	"encoding/json"
	"fmt"

	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
	"github.com/extism/go-pdk"
)

// Host functions provided by the cloudjam host (see internal/challenge). Each
// takes an extism memory offset holding the JSON-encoded input struct and
// returns an offset holding the JSON-encoded output struct.
//
// The import names must be string literals — //go:wasmimport cannot reference a
// constant — so the init below asserts they still match the pkg/challenge/api
// contract and fails loudly at plugin startup if either side drifts.

//go:wasmimport extism:host/user init
func hostInit(uint64) uint64

//go:wasmimport extism:host/user report
func hostReport(uint64) uint64

//go:wasmimport extism:host/user read_score
func hostReadScore(uint64) uint64

//go:wasmimport extism:host/user update_score
func hostUpdateScore(uint64) uint64

//go:wasmimport extism:host/user create_resource
func hostCreateResource(uint64) uint64

// NOTE: the read call is registered under "register_resource" — the name in
// api.ReadResource. It reads like a copy/paste slip, but host and guest
// agree on it, so this import tracks the constant rather than the intent.
//
//go:wasmimport extism:host/user register_resource
func hostReadResource(uint64) uint64

//go:wasmimport extism:host/user update_resource
func hostUpdateResource(uint64) uint64

//go:wasmimport extism:host/user delete_resource
func hostDeleteResource(uint64) uint64

//go:wasmimport extism:host/user list_resource
func hostListResource(uint64) uint64

func init() {
	for constant, imported := range map[string]string{
		api.Init:           "init",
		api.Report:         "report",
		api.ReadScore:      "read_score",
		api.UpdateScore:    "update_score",
		api.CreateResource: "create_resource",
		api.ReadResource:   "register_resource",
		api.UpdateResource: "update_resource",
		api.DeleteResource: "delete_resource",
		api.ListResource:   "list_resource",
	} {
		if constant != imported {
			panic(fmt.Sprintf("aws: host function %q drifted from wasm import %q; "+
				"update pkg/challenge/aws/host_wasip1.go", constant, imported))
		}
	}
}

// pdkHost shuttles JSON across the extism boundary.
type pdkHost struct{}

func newHost() transport { return pdkHost{} }

// call marshals in, invokes the host function, and decodes the response.
func call[In, Out any](fn func(uint64) uint64, in In) (Out, error) {
	var out Out

	data, err := json.Marshal(in)
	if err != nil {
		return out, fmt.Errorf("marshal host request: %w", err)
	}
	mem := pdk.AllocateBytes(data)
	defer mem.Free()

	offset := fn(mem.Offset())
	if offset == 0 {
		// No response payload. Legitimate for the calls whose output struct is
		// empty, so this is not an error.
		return out, nil
	}
	rmem := pdk.FindMemory(offset)
	raw := rmem.ReadBytes()
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("decode host response: %w", err)
	}
	return out, nil
}

func (pdkHost) Init(in api.InitInput) (api.InitOutput, error) {
	return call[api.InitInput, api.InitOutput](hostInit, in)
}

func (pdkHost) Report(in api.ReportInput) (api.ReportOutput, error) {
	return call[api.ReportInput, api.ReportOutput](hostReport, in)
}

func (pdkHost) ReadScore(in api.ReadScoreInput) (api.ReadScoreOutput, error) {
	return call[api.ReadScoreInput, api.ReadScoreOutput](hostReadScore, in)
}

func (pdkHost) UpdateScore(in api.UpdateScoreInput) (api.UpdateScoreOutput, error) {
	return call[api.UpdateScoreInput, api.UpdateScoreOutput](hostUpdateScore, in)
}

func (pdkHost) CreateResource(in api.CreateResourceInput) (api.CreateResourceOutput, error) {
	return call[api.CreateResourceInput, api.CreateResourceOutput](hostCreateResource, in)
}

func (pdkHost) ReadResource(in api.ReadResourceInput) (api.ReadResourceOutput, error) {
	return call[api.ReadResourceInput, api.ReadResourceOutput](hostReadResource, in)
}

func (pdkHost) UpdateResource(in api.UpdateResourceInput) (api.UpdateResourceOutput, error) {
	return call[api.UpdateResourceInput, api.UpdateResourceOutput](hostUpdateResource, in)
}

func (pdkHost) DeleteResource(in api.DeleteResourceInput) (api.DeleteResourceOutput, error) {
	return call[api.DeleteResourceInput, api.DeleteResourceOutput](hostDeleteResource, in)
}

func (pdkHost) ListResource(in api.ListResourceInput) (api.ListResourceOutput, error) {
	return call[api.ListResourceInput, api.ListResourceOutput](hostListResource, in)
}

func (pdkHost) Log(msg string)     { pdk.Log(pdk.LogInfo, msg) }
func (pdkHost) SetError(err error) { pdk.SetError(err) }
func (pdkHost) Output(msg string)  { pdk.OutputString(msg) }
