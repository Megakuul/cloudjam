package oltp

import (
	"time"

	"github.com/megakuul/dynamitedb"
)

type User struct {
	UserID       dynamitedb.KeyField             `pk:"user_id" json:"-"`
	PubID        dynamitedb.DataField[string]    `json:"pub_id,omitempty"`
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
