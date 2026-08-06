// package api contains the plugin API function names and serialization structs.
// Usually you do not need this, it is wrapped by the challenge pkg that provides a high level sdk.
package api

import (
	"log/slog"
)

const ReportName = "report"

type ReportInput struct {
	Error string `json:"error,omitempty"`
}

type ReportOutput struct{}

const LogName = "log"

type LogInput struct {
	Severity slog.Level `json:"severity,omitempty"`
	Message  string     `json:"error,omitempty"`
}

type LogOutput struct{}

const CreateMetaName = "create_meta"

type CreateMetaInput struct {
	Title        string            `json:"title,omitempty"`
	Descriptions []string          `json:"descriptions,omitempty"`
	Clues        map[string]string `json:"clues,omitempty"`
	Assets       map[string]string `json:"assets,omitempty"`
}

type CreateMetaOutput struct{}

const UpdateMetaName = "update_meta"

type UpdateMetaInput struct {
	AdditionalDescriptions []string          `json:"additional_descriptions,omitempty"`
	AdditionalClues        map[string]string `json:"additional_clues,omitempty"`
	AdditionalAssets       map[string]string `json:"additional_assets,omitempty"`
}

type UpdateMetaOutput struct{}

const ReadScoreName = "read_score"

type ReadScoreInput struct{}

type ReadScoreOutput struct {
	Score float64 `json:"score,omitempty"`
}

const UpdateScoreName = "update_score"

type UpdateScoreInput struct {
	Reason    string  `json:"reason,omitempty"`
	Increment float64 `json:"increme,omitempty"`
}

type UpdateScoreOutput struct{}

const CreateAssetName = "create_asset"

type CreateAssetInput []byte

type CreateAssetOutput struct {
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
}

const UpdateAssetName = "update_asset"

type UpdateAssetInput struct {
	OldName string `json:"old_name,omitempty"`
	NewName string `json:"new_name,omitempty"`
}

type UpdateAssetOutput struct {
	NewURL string `json:"new_url,omitempty"`
}

const CreateResourceName = "create_resource"

type CreateResourceInput struct {
	Type    string `json:"type,omitempty"`
	Desired string `json:"desired,omitempty"`
}

type CreateResourceOutput struct {
	Identifier string `json:"identifier,omitempty"`
}

const ReadResourceName = "read_resource"

type ReadResourceInput struct {
	Type       string `json:"type,omitempty"`
	Identifier string `json:"identifier,omitempty"`
}

type ReadResourceOutput struct {
	State string `json:"state,omitempty"`
}

const UpdateResourceName = "update_resource"

type UpdateResourceInput struct {
	Type       string `json:"type,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	Patch      string `json:"patch,omitempty"`
}

type UpdateResourceOutput struct{}

const DeleteResourceName = "delete_resource"

type DeleteResourceInput struct {
	Type       string `json:"type,omitempty"`
	Identifier string `json:"identifier,omitempty"`
}

type DeleteResourceOutput struct{}

const ListResourceName = "list_resource"

type ListResourceInput struct {
	Type string `json:"type,omitempty"`
}

type ListResourceOutput struct {
	Resources map[string]string `json:"resources,omitempty"`
}
