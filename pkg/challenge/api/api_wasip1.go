package api

import (
	"encoding/json"
	"fmt"

	"github.com/extism/go-pdk"
)

//go:wasmimport extism:host/user report
func hostReport(uint64) uint64

func Report(in ReportInput) (ReportOutput, error) {
	return callInOut[ReportInput, ReportOutput](hostReport, in)
}

//go:wasmimport extism:host/user log
func hostLog(uint64) uint64

func Log(in LogInput) (LogOutput, error) {
	return callInOut[LogInput, LogOutput](hostLog, in)
}

//go:wasmimport extism:host/user create_meta
func hostCreateMeta(uint64) uint64

func CreateMeta(in CreateMetaInput) (CreateMetaOutput, error) {
	return callInOut[CreateMetaInput, CreateMetaOutput](hostCreateMeta, in)
}

//go:wasmimport extism:host/user update_meta
func hostUpdateMeta(uint64) uint64

func UpdateMeta(in UpdateMetaInput) (UpdateMetaOutput, error) {
	return callInOut[UpdateMetaInput, UpdateMetaOutput](hostUpdateMeta, in)
}

//go:wasmimport extism:host/user read_score
func hostReadScore(uint64) uint64

func ReadScore(in ReadScoreInput) (ReadScoreOutput, error) {
	return callInOut[ReadScoreInput, ReadScoreOutput](hostReadScore, in)
}

//go:wasmimport extism:host/user update_score
func hostUpdateScore(uint64) uint64

func UpdateScore(in UpdateScoreInput) (UpdateScoreOutput, error) {
	return callInOut[UpdateScoreInput, UpdateScoreOutput](hostUpdateScore, in)
}

//go:wasmimport extism:host/user create_asset
func hostCreateAsset(uint64) uint64

func CreateAsset(in CreateAssetInput) (CreateAssetOutput, error) {
	return callOut[CreateAssetInput, CreateAssetOutput](hostCreateAsset, in)
}

//go:wasmimport extism:host/user update_asset
func hostUpdateAsset(uint64) uint64

func UpdateAsset(in UpdateAssetInput) (UpdateAssetOutput, error) {
	return callInOut[UpdateAssetInput, UpdateAssetOutput](hostUpdateAsset, in)
}

//go:wasmimport extism:host/user create_permission
func hostCreatePermission(uint64) uint64

func CreatePermission(in CreatePermissionInput) (CreatePermissionOutput, error) {
	return callInOut[CreatePermissionInput, CreatePermissionOutput](hostCreatePermission, in)
}

//go:wasmimport extism:host/user update_permission
func hostUpdatePermission(uint64) uint64

func UpdatePermission(in UpdatePermissionInput) (UpdatePermissionOutput, error) {
	return callInOut[UpdatePermissionInput, UpdatePermissionOutput](hostUpdatePermission, in)
}

//go:wasmimport extism:host/user create_guardrail
func hostCreateGuardrail(uint64) uint64

func CreateGuardrail(in CreateGuardrailInput) (CreateGuardrailOutput, error) {
	return callInOut[CreateGuardrailInput, CreateGuardrailOutput](hostCreateGuardrail, in)
}

//go:wasmimport extism:host/user update_guardrail
func hostUpdateGuardrail(uint64) uint64

func UpdateGuardrail(in UpdateGuardrailInput) (UpdateGuardrailOutput, error) {
	return callInOut[UpdateGuardrailInput, UpdateGuardrailOutput](hostUpdateGuardrail, in)
}

//go:wasmimport extism:host/user create_resource
func hostCreateResource(uint64) uint64

func CreateResource(in CreateResourceInput) (CreateResourceOutput, error) {
	return callInOut[CreateResourceInput, CreateResourceOutput](hostCreateResource, in)
}

//go:wasmimport extism:host/user read_resource
func hostReadResource(uint64) uint64

func ReadResource(in ReadResourceInput) (ReadResourceOutput, error) {
	return callInOut[ReadResourceInput, ReadResourceOutput](hostReadResource, in)
}

//go:wasmimport extism:host/user update_resource
func hostUpdateResource(uint64) uint64

func UpdateResource(in UpdateResourceInput) (UpdateResourceOutput, error) {
	return callInOut[UpdateResourceInput, UpdateResourceOutput](hostUpdateResource, in)
}

//go:wasmimport extism:host/user delete_resource
func hostDeleteResource(uint64) uint64

func DeleteResource(in DeleteResourceInput) (DeleteResourceOutput, error) {
	return callInOut[DeleteResourceInput, DeleteResourceOutput](hostDeleteResource, in)
}

//go:wasmimport extism:host/user list_resource
func hostListResource(uint64) uint64

func ListResource(in ListResourceInput) (ListResourceOutput, error) {
	return callInOut[ListResourceInput, ListResourceOutput](hostListResource, in)
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
