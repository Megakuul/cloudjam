package oltp

import (
	"time"

	"github.com/megakuul/dynamitedb"
)

type Game struct {
	GameID      dynamitedb.KeyField             `pk:"game_id"`
	Name        dynamitedb.DataField[string]    `cbor:"name,omitempty"`
	Description dynamitedb.DataField[string]    `cbor:"description,omitempty"`
	From        dynamitedb.DataField[time.Time] `cbor:"from,omitempty"`
	To          dynamitedb.DataField[time.Time] `cbor:"to,omitempty"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}

type Player struct {
	GameID   dynamitedb.KeyField          `pk:"game_id"`
	PlayerID dynamitedb.KeyField          `sk:"player_id"`
	Username dynamitedb.DataField[string] `cbor:"username,omitempty"`
	PubID    dynamitedb.DataField[string] `cbor:"pub_id,omitempty"`
}

type Team struct {
	GameID  dynamitedb.KeyField            `pk:"game_id"`
	TeamID  dynamitedb.KeyField            `sk:"team_id"`
	Name    dynamitedb.DataField[string]   `cbor:"name,omitempty"`
	Score   dynamitedb.DataField[float64]  `cbor:"score,omitempty"`
	Players dynamitedb.DataField[[]Player] `cbor:"players,omitempty"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}

type ScoreEvent struct {
	Timestamp time.Time `cbor:"timestamp"`
	Text      string    `cbor:"text"`
	Change    float64   `cbor:"change"`
}

type Challenge struct {
	GameID         dynamitedb.KeyField          `pk:"game_id"`
	ChallengeID    dynamitedb.KeyField          `sk:"challenge_id"`
	TeamID         dynamitedb.DataField[string] `cbor:"team_id,omitempty"`
	DefinitionID   dynamitedb.DataField[string] `cbor:"definition_id,omitempty"`
	DefinitionName dynamitedb.DataField[string] `cbor:"definition_name,omitempty"`

	Title       dynamitedb.DataField[string]            `cbor:"title,omitempty"`
	Description dynamitedb.DataField[[]string]          `cbor:"description,omitempty"`
	Clues       dynamitedb.DataField[map[string]string] `cbor:"clues,omitempty"`
	Assets      dynamitedb.DataField[map[string]string] `cbor:"assets,omitempty"`
	Errors      dynamitedb.DataField[[]string]          `cbor:"errors,omitempty"`
	ScoreEvents dynamitedb.DataField[[]ScoreEvent]      `cbor:"score_events,omitempty"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}
