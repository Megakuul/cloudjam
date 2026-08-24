package api

import (
	"encoding/json"
	"fmt"

	"github.com/extism/go-pdk"
)

//go:wasmimport extism:host/user cancel
func hostCancel(uint64) uint64

func Cancel(in CancelInput) (CancelOutput, error) {
	return callInOut[CancelInput, CancelOutput](func(i uint64) uint64 { return hostCancel(i) }, in)
}

//go:wasmimport extism:host/user log
func hostLog(uint64) uint64

func Log(in LogInput) (LogOutput, error) {
	return callInOut[LogInput, LogOutput](func(i uint64) uint64 { return hostLog(i) }, in)
}

//go:wasmimport extism:host/user create_meta
func hostCreateMeta(uint64) uint64

func CreateMeta(in CreateMetaInput) (CreateMetaOutput, error) {
	return callInOut[CreateMetaInput, CreateMetaOutput](func(i uint64) uint64 { return hostCreateMeta(i) }, in)
}

//go:wasmimport extism:host/user update_meta
func hostUpdateMeta(uint64) uint64

func UpdateMeta(in UpdateMetaInput) (UpdateMetaOutput, error) {
	return callInOut[UpdateMetaInput, UpdateMetaOutput](func(i uint64) uint64 { return hostUpdateMeta(i) }, in)
}

//go:wasmimport extism:host/user read_score
func hostReadScore(uint64) uint64

func ReadScore(in ReadScoreInput) (ReadScoreOutput, error) {
	return callInOut[ReadScoreInput, ReadScoreOutput](func(i uint64) uint64 { return hostReadScore(i) }, in)
}

//go:wasmimport extism:host/user update_score
func hostUpdateScore(uint64) uint64

func UpdateScore(in UpdateScoreInput) (UpdateScoreOutput, error) {
	return callInOut[UpdateScoreInput, UpdateScoreOutput](func(i uint64) uint64 { return hostUpdateScore(i) }, in)
}

//go:wasmimport extism:host/user create_asset
func hostCreateAsset(uint64) uint64

func CreateAsset(in CreateAssetInput) (CreateAssetOutput, error) {
	return callOut[CreateAssetInput, CreateAssetOutput](func(i uint64) uint64 { return hostCreateAsset(i) }, in)
}

//go:wasmimport extism:host/user update_asset
func hostUpdateAsset(uint64) uint64

func UpdateAsset(in UpdateAssetInput) (UpdateAssetOutput, error) {
	return callInOut[UpdateAssetInput, UpdateAssetOutput](func(i uint64) uint64 { return hostUpdateAsset(i) }, in)
}

//go:wasmimport extism:host/user create_permission
func hostCreatePermission(uint64) uint64

func CreatePermission(in CreatePermissionInput) (CreatePermissionOutput, error) {
	return callInOut[CreatePermissionInput, CreatePermissionOutput](func(i uint64) uint64 { return hostCreatePermission(i) }, in)
}

//go:wasmimport extism:host/user update_permission
func hostUpdatePermission(uint64) uint64

func UpdatePermission(in UpdatePermissionInput) (UpdatePermissionOutput, error) {
	return callInOut[UpdatePermissionInput, UpdatePermissionOutput](func(i uint64) uint64 { return hostUpdatePermission(i) }, in)
}

//go:wasmimport extism:host/user create_guardrail
func hostCreateGuardrail(uint64) uint64

func CreateGuardrail(in CreateGuardrailInput) (CreateGuardrailOutput, error) {
	return callInOut[CreateGuardrailInput, CreateGuardrailOutput](func(i uint64) uint64 { return hostCreateGuardrail(i) }, in)
}

//go:wasmimport extism:host/user update_guardrail
func hostUpdateGuardrail(uint64) uint64

func UpdateGuardrail(in UpdateGuardrailInput) (UpdateGuardrailOutput, error) {
	return callInOut[UpdateGuardrailInput, UpdateGuardrailOutput](func(i uint64) uint64 { return hostUpdateGuardrail(i) }, in)
}

//go:wasmimport extism:host/user create_resource
func hostCreateResource(uint64) uint64

func CreateResource(in CreateResourceInput) (CreateResourceOutput, error) {
	return callInOut[CreateResourceInput, CreateResourceOutput](func(i uint64) uint64 { return hostCreateResource(i) }, in)
}

//go:wasmimport extism:host/user read_resource
func hostReadResource(uint64) uint64

func ReadResource(in ReadResourceInput) (ReadResourceOutput, error) {
	return callInOut[ReadResourceInput, ReadResourceOutput](func(i uint64) uint64 { return hostReadResource(i) }, in)
}

//go:wasmimport extism:host/user update_resource
func hostUpdateResource(uint64) uint64

func UpdateResource(in UpdateResourceInput) (UpdateResourceOutput, error) {
	return callInOut[UpdateResourceInput, UpdateResourceOutput](func(i uint64) uint64 { return hostUpdateResource(i) }, in)
}

//go:wasmimport extism:host/user delete_resource
func hostDeleteResource(uint64) uint64

func DeleteResource(in DeleteResourceInput) (DeleteResourceOutput, error) {
	return callInOut[DeleteResourceInput, DeleteResourceOutput](func(i uint64) uint64 { return hostDeleteResource(i) }, in)
}

//go:wasmimport extism:host/user list_resource
func hostListResource(uint64) uint64

func ListResource(in ListResourceInput) (ListResourceOutput, error) {
	return callInOut[ListResourceInput, ListResourceOutput](func(i uint64) uint64 { return hostListResource(i) }, in)
}

// CallInOut calls a host function and json decodes the request / response.
func callInOut[In, Out any](fn func(uint64) uint64, in In) (Out, error) {
	var out Out

	data, err := json.Marshal(in)
	if err != nil {
		return out, fmt.Errorf("marshal host request: %w", err)
	}
	raw, err := call(fn, data)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("decode host response: %w", err)
	}
	return out, nil
}

// callOut calls a host function and json decodes the response.
// useful to efficiently move large datablocks to the host without json base64 encoding.
func callOut[In ~[]byte, Out any](fn func(uint64) uint64, in In) (Out, error) {
	var out Out

	raw, err := call(fn, in)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return out, fmt.Errorf("decode host response: %w", err)
	}
	return out, nil
}

// call calls a host function (allocates the input memory block, invokes the host function and returns the response memory block).
func call(fn func(uint64) uint64, in []byte) ([]byte, error) {
	mem := pdk.AllocateBytes(in)
	defer mem.Free()

	offset := fn(mem.Offset())
	if offset == 0 {
		// No response payload. Legitimate for the calls whose output struct is empty, so this is not an error.
		return nil, nil
	}
	rmem := pdk.FindMemory(offset)
	raw := rmem.ReadBytes()
	if len(raw) == 0 {
		return nil, nil
	}
	return raw, nil
}
