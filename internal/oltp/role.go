package oltp

import "github.com/megakuul/dynamitedb"

const (
	ScopeAdmin string = "admin"
	ScopeSelf  string = "self"
)

type Role struct {
	RoleID      dynamitedb.KeyField                     `pk:"role_id"`
	Name        dynamitedb.DataField[string]            `cbor:"name,omitempty"`
	Description dynamitedb.DataField[string]            `cbor:"description,omitempty"`
	Builtin     dynamitedb.DataField[bool]              `cbor:"builtin,omitempty"`
	Permissions dynamitedb.DataField[map[string]string] `cbor:"permissions,omitempty"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}
