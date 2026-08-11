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
	ID       string `cbor:"id,omitempty"`
	Username string `cbor:"username,omitempty"`
	PubID    string `cbor:"pub_id,omitempty"`
}

type Team struct {
	GameID  dynamitedb.KeyField                     `pk:"game_id"`
	TeamID  dynamitedb.KeyField                     `sk:"team_id"`
	Name    dynamitedb.DataField[string]            `cbor:"name,omitempty"`
	Score   dynamitedb.DataField[float64]           `cbor:"score,omitempty"`
	Players dynamitedb.DataField[map[string]Player] `cbor:"players,omitempty"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}

type ScoreEvent struct {
	Timestamp time.Time `cbor:"timestamp"`
	Text      string    `cbor:"text"`
	Change    float64   `cbor:"change"`
}

type Challenge struct {
	GameID               dynamitedb.KeyField          `pk:"game_id"`
	ChallengeID          dynamitedb.KeyField          `sk:"challenge_id"`
	TeamID               dynamitedb.DataField[string] `cbor:"team_id,omitempty"`
	DefinitionProviderID dynamitedb.DataField[string] `cbor:"definition_provider_id,omitempty"`
	DefinitionID         dynamitedb.DataField[string] `cbor:"definition_id,omitempty"`

	Title       dynamitedb.DataField[string]            `cbor:"title,omitempty"`
	Description dynamitedb.DataField[[]string]          `cbor:"description,omitempty"`
	Clues       dynamitedb.DataField[map[string]string] `cbor:"clues,omitempty"`
	Assets      dynamitedb.DataField[map[string]string] `cbor:"assets,omitempty"`
	Errors      dynamitedb.DataField[[]string]          `cbor:"errors,omitempty"`
	ScoreEvents dynamitedb.DataField[[]ScoreEvent]      `cbor:"score_events,omitempty"`
	Starts      dynamitedb.DataField[time.Duration]     `cbor:"starts,omitempty"`
	Ends        dynamitedb.DataField[time.Time]         `cbor:"ends,omitempty"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}
