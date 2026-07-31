package oltp

import (
	"time"

	"github.com/megakuul/dynamitedb"
)

type Game struct {
	GameID      dynamitedb.KeyField             `pk:"game_id"`
	Name        dynamitedb.DataField[string]    `json:"name,omitempty"`
	Description dynamitedb.DataField[string]    `json:"description,omitempty"`
	From        dynamitedb.DataField[time.Time] `json:"from,omitempty"`
	To          dynamitedb.DataField[time.Time] `json:"to,omitempty"`

	Scope dynamitedb.DataField[string] `json:"scope,omitempty"`
}

type Player struct {
	GameID      dynamitedb.KeyField           `pk:"game_id"`
	PlayerID    dynamitedb.KeyField           `sk:"player_id"`
	Username    dynamitedb.DataField[string]  `json:"username,omitempty"`
	PubID       dynamitedb.DataField[string]  `json:"pub_id,omitempty"`
	PlayerScore dynamitedb.DataField[float64] `json:"player_score,omitempty"`
	GameScore   dynamitedb.DataField[float64] `json:"game_score,omitempty"`

	Scope dynamitedb.DataField[string] `json:"scope,omitempty"`
}

type Challenge struct {
	GameID         dynamitedb.KeyField          `pk:"game_id"`
	ChallengeID    dynamitedb.KeyField          `sk:"challenge_id"`
	DefinitionID   dynamitedb.DataField[string] `json:"definition_id,omitempty"`
	DefinitionName dynamitedb.DataField[string] `json:"definition_name,omitempty"`

	Title       dynamitedb.DataField[string]   `json:"title,omitempty"`
	Description dynamitedb.DataField[string]   `json:"description,omitempty"`
	Errors      dynamitedb.DataField[[]string] `json:"errors,omitempty"`

	Scope dynamitedb.DataField[string] `json:"scope,omitempty"`
}
