package oltp

import (
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/api/v1/play"
	"github.com/megakuul/dynamitedb"
)

type Game struct {
	GameID      dynamitedb.KeyField             `pk:"game_id" cbor:"-"`
	ETag        dynamitedb.ETagField            `etag:"true" cbor:"-"`
	Name        dynamitedb.DataField[string]    `cbor:"name,omitempty"`
	Description dynamitedb.DataField[string]    `cbor:"description,omitempty"`
	From        dynamitedb.DataField[time.Time] `cbor:"from,omitempty"`
	To          dynamitedb.DataField[time.Time] `cbor:"to,omitempty"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}

type Team struct {
	GameID  dynamitedb.KeyField                           `pk:"game_id" cbor:"-"`
	TeamID  dynamitedb.KeyField                           `sk:"team_id" cbor:"-"`
	ETag    dynamitedb.ETagField                          `etag:"true" cbor:"-"`
	Name    dynamitedb.DataField[string]                  `cbor:"name,omitempty"`
	Score   dynamitedb.DataField[float64]                 `cbor:"score,omitempty"`
	Players dynamitedb.DataField[map[string]*play.Player] `cbor:"players,omitempty"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}

type Challenge struct {
	GameID       dynamitedb.KeyField          `pk:"game_id" cbor:"-"`
	ChallengeID  dynamitedb.KeyField          `sk:"challenge_id" cbor:"-"`
	ETag         dynamitedb.ETagField         `etag:"true" cbor:"-"`
	TeamID       dynamitedb.DataField[string] `cbor:"team_id,omitempty"`
	ProviderID   dynamitedb.DataField[string] `cbor:"provider_id,omitempty"`
	DefinitionID dynamitedb.DataField[string] `cbor:"definition_id,omitempty"`
	AccountID    dynamitedb.DataField[string] `cbor:"account_id,omitempty"`

	Title          dynamitedb.DataField[string]             `cbor:"title,omitempty"`
	Description    dynamitedb.DataField[[]string]           `cbor:"description,omitempty"`
	Ready          dynamitedb.DataField[bool]               `cbor:"ready"`
	CluePrices     dynamitedb.DataField[map[string]float64] `cbor:"clue_prices,omitempty"`
	Clues          dynamitedb.DataField[map[string]string]  `cbor:"clues,omitempty"`
	UncoveredClues dynamitedb.DataField[map[string]bool]    `cbor:"uncovered_clues,omitempty"`
	Assets         dynamitedb.DataField[map[string]string]  `cbor:"assets,omitempty"`
	Error          dynamitedb.DataField[string]             `cbor:"error,omitempty"`
	ScoreEvents    dynamitedb.DataField[[]*play.ScoreEvent] `cbor:"score_events,omitempty"`

	Scope dynamitedb.DataField[string] `cbor:"scope,omitempty"`
}
