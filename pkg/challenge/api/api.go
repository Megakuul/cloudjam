// package api contains the plugin API function names and serialization structs.
// Usually you do not need this, it is wrapped by the challenge pkg that provides a high level sdk.
package api

const InitName = "init"

//go:wasmimport extism:host/user init
func hostInit(uint64) uint64

type InitInput struct {
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Clues       map[string]string `json:"clues,omitempty"`
}

type InitOutput struct{}

func Init(in InitInput) (InitOutput, error) {
	return call[InitInput, InitOutput](hostInit, in)
}

const ReportName = "report"

//go:wasmimport extism:host/user report
func hostReport(uint64) uint64

type ReportInput struct {
	Error string `json:"error,omitempty"`
}

type ReportOutput struct{}

func Report(in ReportInput) (ReportOutput, error) {
	return call[ReportInput, ReportOutput](hostReport, in)
}

const ReadScoreName = "read_score"

//go:wasmimport extism:host/user read_score
func hostReadScore(uint64) uint64

type ReadScoreInput struct{}

type ReadScoreOutput struct {
	Score float64 `json:"score,omitempty"`
}

func ReadScore(in ReadScoreInput) (ReadScoreOutput, error) {
	return call[ReadScoreInput, ReadScoreOutput](hostReadScore, in)
}

const UpdateScoreName = "update_score"

//go:wasmimport extism:host/user update_score
func hostUpdateScore(uint64) uint64

type UpdateScoreInput struct {
	Reason    string  `json:"reason,omitempty"`
	Increment float64 `json:"increme,omitempty"`
}

type UpdateScoreOutput struct{}

func UpdateScore(in UpdateScoreInput) (UpdateScoreOutput, error) {
	return call[UpdateScoreInput, UpdateScoreOutput](hostUpdateScore, in)
}

const CreateResourceName = "create_resource"

//go:wasmimport extism:host/user create_resource
func hostCreateResource(uint64) uint64

type CreateResourceInput struct {
	Type    string `json:"type,omitempty"`
	Desired string `json:"desired,omitempty"`
}

type CreateResourceOutput struct {
	Identifier string `json:"identifier,omitempty"`
}

func CreateResource(in CreateResourceInput) (CreateResourceOutput, error) {
	return call[CreateResourceInput, CreateResourceOutput](hostCreateResource, in)
}

const ReadResourceName = "read_resource"

//go:wasmimport extism:host/user read_resource
func hostReadResource(uint64) uint64

type ReadResourceInput struct {
	Type       string `json:"type,omitempty"`
	Identifier string `json:"identifier,omitempty"`
}

type ReadResourceOutput struct {
	State string `json:"state,omitempty"`
}

func ReadResource(in ReadResourceInput) (ReadResourceOutput, error) {
	return call[ReadResourceInput, ReadResourceOutput](hostReadResource, in)
}

const UpdateResourceName = "update_resource"

//go:wasmimport extism:host/user update_resource
func hostUpdateResource(uint64) uint64

type UpdateResourceInput struct {
	Type       string `json:"type,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	Patch      string `json:"patch,omitempty"`
}

type UpdateResourceOutput struct{}

func UpdateResource(in UpdateResourceInput) (UpdateResourceOutput, error) {
	return call[UpdateResourceInput, UpdateResourceOutput](hostUpdateResource, in)
}

const DeleteResourceName = "delete_resource"

//go:wasmimport extism:host/user delete_resource
func hostDeleteResource(uint64) uint64

type DeleteResourceInput struct {
	Type       string `json:"type,omitempty"`
	Identifier string `json:"identifier,omitempty"`
}

type DeleteResourceOutput struct{}

func DeleteResource(in DeleteResourceInput) (DeleteResourceOutput, error) {
	return call[DeleteResourceInput, DeleteResourceOutput](hostDeleteResource, in)
}

const ListResourceName = "list_resource"

//go:wasmimport extism:host/user list_resource
func hostListResource(uint64) uint64

type ListResourceInput struct {
	Type string `json:"type,omitempty"`
}

type ListResourceOutput struct {
	Resources map[string]string `json:"resources,omitempty"`
}

func ListResource(in ListResourceInput) (ListResourceOutput, error) {
	return call[ListResourceInput, ListResourceOutput](hostListResource, in)
}
