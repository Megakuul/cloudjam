package oltp

import "github.com/megakuul/dynamitedb"

type Provider struct {
	ProviderID      dynamitedb.KeyField            `pk:"provider_id"`
	Type            dynamitedb.DataField[int]      `cbor:"type,omitempty"`
	Name            dynamitedb.DataField[string]   `cbor:"name,omitempty"`
	Description     dynamitedb.DataField[string]   `cbor:"description,omitempty"`
	Email           dynamitedb.DataField[string]   `cbor:"email,omitempty"`
	Regions         dynamitedb.DataField[[]string] `cbor:"regions,omitempty"`
	Credentials     dynamitedb.DataField[string]   `cbor:"credentials"` // credentials for provider root entity that can create accounts
	DesiredAccounts dynamitedb.DataField[int]      `cbor:"desired_accounts"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}

type Account struct {
	ProviderID   dynamitedb.KeyField          `pk:"provider_id"`
	AccountID    dynamitedb.KeyField          `sk:"account_id"`
	Name         dynamitedb.DataField[string] `cbor:"name,omitempty"`
	Description  dynamitedb.DataField[string] `cbor:"description,omitempty"`
	Credentials  dynamitedb.DataField[string] `cbor:"credentials"`
	State        dynamitedb.DataField[int]    `cbor:"state"`
	DesiredState dynamitedb.DataField[int]    `cbor:"desired_state"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}

type Definition struct {
	ProviderID   dynamitedb.KeyField          `pk:"provider_id"`
	DefinitionID dynamitedb.KeyField          `sk:"definition_id"`
	Name         dynamitedb.DataField[string] `cbor:"name,omitempty"`
	Description  dynamitedb.DataField[string] `cbor:"description,omitempty"`
	Version      dynamitedb.DataField[string] `cbor:"version,omitempty"`
	Checksum     dynamitedb.DataField[string] `cbor:"checksum,omitempty"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}

type CompressionMode int

const (
	CompressionZstd = iota
)

// separated from definition to avoid loading the WASM binary on every definition lookup.
type DefinitionBinary struct {
	ProviderID   dynamitedb.KeyField                   `pk:"provider_id"`
	DefinitionID dynamitedb.KeyField                   `sk:"definition_binary_id"`
	Compression  dynamitedb.DataField[CompressionMode] `cbor:"compression,omitempty"`
	WASM         dynamitedb.DataField[[]byte]          `cbor:"wasm,omitempty"`
}
