package oltp

import "github.com/megakuul/dynamitedb"

type Challenge struct {
	ProviderID  dynamitedb.KeyField          `pk:"provider_id"`
	ChallengeID dynamitedb.KeyField          `sk:"challenge_id"`
	Name        dynamitedb.DataField[string] `json:"name,omitempty"`
	Description dynamitedb.DataField[string] `json:"description,omitempty"`
	Version     dynamitedb.DataField[string] `json:"version,omitempty"`
	WASM        dynamitedb.DataField[string] `json:"wasm,omitempty"`

	Scope dynamitedb.DataField[string] `json:"scope,omitempty"`
}
