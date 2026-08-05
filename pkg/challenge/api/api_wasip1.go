// The guest half of the plugin API: the wasm imports the host installs, and the
// thin functions that call them. It compiles only inside a plugin (GOOS=wasip1),
// which is what lets the host import this package for the structs in api.go
// without dragging in the pdk — that package has no bodies off wasm.
package api

import (
	"encoding/json"
	"fmt"

	"github.com/extism/go-pdk"
)

//go:wasmimport extism:host/user init
func hostInit(uint64) uint64

func Init(in InitInput) (InitOutput, error) {
	return call[InitInput, InitOutput](hostInit, in)
}

//go:wasmimport extism:host/user report
func hostReport(uint64) uint64

func Report(in ReportInput) (ReportOutput, error) {
	return call[ReportInput, ReportOutput](hostReport, in)
}

//go:wasmimport extism:host/user read_score
func hostReadScore(uint64) uint64

func ReadScore(in ReadScoreInput) (ReadScoreOutput, error) {
	return call[ReadScoreInput, ReadScoreOutput](hostReadScore, in)
}

//go:wasmimport extism:host/user update_score
func hostUpdateScore(uint64) uint64

func UpdateScore(in UpdateScoreInput) (UpdateScoreOutput, error) {
	return call[UpdateScoreInput, UpdateScoreOutput](hostUpdateScore, in)
}

//go:wasmimport extism:host/user create_resource
func hostCreateResource(uint64) uint64

func CreateResource(in CreateResourceInput) (CreateResourceOutput, error) {
	return call[CreateResourceInput, CreateResourceOutput](hostCreateResource, in)
}

//go:wasmimport extism:host/user read_resource
func hostReadResource(uint64) uint64

func ReadResource(in ReadResourceInput) (ReadResourceOutput, error) {
	return call[ReadResourceInput, ReadResourceOutput](hostReadResource, in)
}

//go:wasmimport extism:host/user update_resource
func hostUpdateResource(uint64) uint64

func UpdateResource(in UpdateResourceInput) (UpdateResourceOutput, error) {
	return call[UpdateResourceInput, UpdateResourceOutput](hostUpdateResource, in)
}

//go:wasmimport extism:host/user delete_resource
func hostDeleteResource(uint64) uint64

func DeleteResource(in DeleteResourceInput) (DeleteResourceOutput, error) {
	return call[DeleteResourceInput, DeleteResourceOutput](hostDeleteResource, in)
}

//go:wasmimport extism:host/user list_resource
func hostListResource(uint64) uint64

func ListResource(in ListResourceInput) (ListResourceOutput, error) {
	return call[ListResourceInput, ListResourceOutput](hostListResource, in)
}

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
