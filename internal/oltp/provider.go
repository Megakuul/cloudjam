package oltp

import (
	"codeberg.org/megakuul/cloudjam/pkg/api/v1/cloud"
	"github.com/megakuul/dynamitedb"
)

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
	ProviderID  dynamitedb.KeyField                      `pk:"provider_id"`
	AccountID   dynamitedb.KeyField                      `sk:"account_id"`
	Name        dynamitedb.DataField[string]             `cbor:"name,omitempty"`
	Description dynamitedb.DataField[string]             `cbor:"description,omitempty"`
	Credentials dynamitedb.DataField[string]             `cbor:"credentials"`
	State       dynamitedb.DataField[cloud.AccountState] `cbor:"state"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}

type Definition struct {
	ProviderID   dynamitedb.KeyField          `pk:"provider_id"`
	DefinitionID dynamitedb.KeyField          `sk:"definition_id"`
	Name         dynamitedb.DataField[string] `cbor:"name,omitempty"`
	Description  dynamitedb.DataField[string] `cbor:"description,omitempty"`
	Version      dynamitedb.DataField[string] `cbor:"version,omitempty"`
	Hash         dynamitedb.DataField[string] `cbor:"hash,omitempty"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}

// separated from definition to avoid loading the WASM binary on every definition lookup.
type DefinitionBinary struct {
	ProviderID   dynamitedb.KeyField                         `pk:"provider_id"`
	DefinitionID dynamitedb.KeyField                         `sk:"definition_binary_id"`
	Compression  dynamitedb.DataField[cloud.CompressionMode] `cbor:"compression,omitempty"`
	WASM         dynamitedb.DataField[[]byte]                `cbor:"wasm,omitempty"`
}
