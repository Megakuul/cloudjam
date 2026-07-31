package oltp

import "github.com/megakuul/dynamitedb"

type Provider struct {
	ProviderID      dynamitedb.KeyField          `pk:"provider_id"`
	Type            dynamitedb.DataField[int]    `json:"type,omitempty"`
	Name            dynamitedb.DataField[string] `json:"name,omitempty"`
	Description     dynamitedb.DataField[string] `json:"description,omitempty"`
	Credentials     dynamitedb.DataField[string] `json:"credentials"` // credentials for provider root entity that can create accounts
	DesiredAccounts dynamitedb.DataField[int]    `json:"desired_accounts"`

	Scope dynamitedb.DataField[string] `json:"scope,omitempty"`
}

type Account struct {
	ProviderID   dynamitedb.KeyField          `pk:"provider_id"`
	AccountID    dynamitedb.KeyField          `sk:"account_id"`
	Name         dynamitedb.DataField[string] `json:"name,omitempty"`
	Description  dynamitedb.DataField[string] `json:"description,omitempty"`
	Credentials  dynamitedb.DataField[string] `json:"credentials"`
	State        dynamitedb.DataField[int]    `json:"state"`
	DesiredState dynamitedb.DataField[int]    `json:"desired_state"`

	Scope dynamitedb.DataField[string] `json:"scope,omitempty"`
}

type ChallengeDefinition struct {
	ProviderID  dynamitedb.KeyField          `pk:"provider_id"`
	ChallengeID dynamitedb.KeyField          `sk:"challenge_id"`
	Name        dynamitedb.DataField[string] `json:"name,omitempty"`
	Description dynamitedb.DataField[string] `json:"description,omitempty"`
	Version     dynamitedb.DataField[string] `json:"version,omitempty"`
	WASM        dynamitedb.DataField[string] `json:"wasm,omitempty"`

	Scope dynamitedb.DataField[string] `json:"scope,omitempty"`
}
