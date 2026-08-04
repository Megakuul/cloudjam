// package api contains the plugin API function names and serialization structs.
// Usually you do not need this, it is wrapped by the challenge pkg that provides a high level sdk.
package api

const Init = "init"

type InitInput struct {
	Title       string            `json:"title,omitempty"`
	Description string            `json:"description,omitempty"`
	Clues       map[string]string `json:"clues,omitempty"`
}

type InitOutput struct{}

const Report = "report"

type ReportInput struct {
	Error string `json:"error,omitempty"`
}

type ReportOutput struct{}

const ReadScore = "read_score"

type ReadScoreInput struct{}

type ReadScoreOutput struct {
	Score float64 `json:"score,omitempty"`
}

const UpdateScore = "update_score"

type UpdateScoreInput struct {
	Reason    string  `json:"reason,omitempty"`
	Increment float64 `json:"increme,omitempty"`
}

type UpdateScoreOutput struct{}

const CreateResource = "create_resource"

type CreateResourceInput struct {
	Type    string `json:"type,omitempty"`
	Desired string `json:"desired,omitempty"`
	Wait    bool   `json:"wait,omitempty"`
}

type CreateResourceOutput struct{}

const ReadResource = "register_resource"

type ReadResourceInput struct {
	Type       string `json:"type,omitempty"`
	Identifier string `json:"identifier,omitempty"`
}

type ReadResourceOutput struct {
	State string `json:"state,omitempty"`
}

const UpdateResource = "update_resource"

type UpdateResourceInput struct {
	Type       string `json:"type,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	Patch      string `json:"patch,omitempty"`
	Wait       bool   `json:"wait,omitempty"`
}

type UpdateResourceOutput struct{}

const DeleteResource = "delete_resource"

type DeleteResourceInput struct {
	Type       string `json:"type,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	Wait       bool   `json:"wait,omitempty"`
}

type DeleteResourceOutput struct{}

const ListResource = "list_resource"

type ListResourceInput struct {
	Type string `json:"type,omitempty"`
}

type ListResourceOutput struct {
	Resources map[string]string `json:"resources,omitempty"`
}
