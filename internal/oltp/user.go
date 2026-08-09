package oltp

import (
	"time"

	"github.com/megakuul/dynamitedb"
)

type User struct {
	UserID       dynamitedb.KeyField             `pk:"user_id" cbor:"-"`
	PubID        dynamitedb.DataField[string]    `cbor:"pub_id,omitempty"`
	Username     dynamitedb.DataField[string]    `cbor:"username,omitempty"`
	Description  dynamitedb.DataField[string]    `cbor:"description,omitempty"`
	Organization dynamitedb.DataField[string]    `cbor:"organization,omitempty"`
	Email        dynamitedb.DataField[string]    `cbor:"email,omitempty"`
	CreatedAt    dynamitedb.DataField[time.Time] `cbor:"created_at,omitempty"`
	Score        dynamitedb.DataField[float64]   `cbor:"score,omitempty"`
	MaxScore     dynamitedb.DataField[float64]   `cbor:"max_score,omitempty"`
	Streak       dynamitedb.DataField[int]       `cbor:"streak,omitempty"`
	MaxStreak    dynamitedb.DataField[int]       `cbor:"max_streak,omitempty"`
	Privileged   dynamitedb.DataField[bool]      `cbor:"privileged,omitempty"`
	Role         dynamitedb.DataField[string]    `cbor:"role,omitempty"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}

type Creds struct {
	Email          dynamitedb.KeyField             `pk:"email" cbor:"-"`
	Active         dynamitedb.DataField[bool]      `cbor:"active,omitempty"`
	UserId         dynamitedb.DataField[string]    `cbor:"user_id,omitempty"`
	Password       dynamitedb.DataField[string]    `cbor:"password,omitempty"`
	Code           dynamitedb.DataField[string]    `cbor:"code,omitempty"`
	CodeExpiration dynamitedb.DataField[time.Time] `cbor:"code_expiration,omitempty"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}
