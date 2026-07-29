package oltp

import "github.com/megakuul/dynamitedb"

const (
	ScopeAdmin string = "admin"
	ScopeSelf  string = "self"
)

type Role struct {
	RoleID      dynamitedb.KeyField                     `pk:"role_id"`
	Name        dynamitedb.DataField[string]            `json:"name,omitempty"`
	Description dynamitedb.DataField[string]            `json:"description,omitempty"`
	Builtin     dynamitedb.DataField[bool]              `json:"builtin,omitempty"`
	Permissions dynamitedb.DataField[map[string]string] `json:"permissions,omitempty"`

	Scope dynamitedb.DataField[string] `json:"scope,omitempty"`
}
