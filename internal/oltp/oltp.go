// Package oltp contains the dynamite oltp database schemas / models.
package oltp

import (
	"time"

	"github.com/megakuul/dynamitedb"
)

type User struct {
	UserID       dynamitedb.KeyField             `pk:"user_id" json:"-"`
	PubId        dynamitedb.DataField[string]    `json:"pub_id,omitempty"`
	Username     dynamitedb.DataField[string]    `json:"username,omitempty"`
	Description  dynamitedb.DataField[string]    `json:"description,omitempty"`
	Organization dynamitedb.DataField[string]    `json:"organization,omitempty"`
	Email        dynamitedb.DataField[string]    `json:"email,omitempty"`
	CreatedAt    dynamitedb.DataField[time.Time] `json:"created_at,omitempty"`
	Score        dynamitedb.DataField[float64]   `json:"score,omitempty"`
	MaxScore     dynamitedb.DataField[float64]   `json:"max_score,omitempty"`
	Streak       dynamitedb.DataField[int]       `json:"streak,omitempty"`
	MaxStreak    dynamitedb.DataField[int]       `json:"max_streak,omitempty"`
	Privileged   dynamitedb.DataField[bool]      `json:"privileged,omitempty"`
	Role         dynamitedb.DataField[string]    `json:"role,omitempty"`

	Scope dynamitedb.DataField[string] `json:"scope,omitempty"`
}

type Creds struct {
	Email          dynamitedb.KeyField             `pk:"email" json:"-"`
	Active         dynamitedb.DataField[bool]      `json:"active,omitempty"`
	UserId         dynamitedb.DataField[string]    `json:"user_id,omitempty"`
	Password       dynamitedb.DataField[string]    `json:"password,omitempty"`
	Code           dynamitedb.DataField[string]    `json:"code,omitempty"`
	CodeExpiration dynamitedb.DataField[time.Time] `json:"code_expiration,omitempty"`

	Scope dynamitedb.DataField[string] `json:"scope,omitempty"`
}

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
