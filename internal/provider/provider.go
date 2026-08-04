package provider

import (
	"context"
	"log/slog"
	"time"
)

type Leak struct {
	// Severity classifies the finding.
	Severity slog.Level
	// Region locates the finding (empty for account global findings).
	Region string
	// Resource identifies the affected resource or heuristic.
	Resource string
	// Message describes what was found (or verified).
	Message string
	// Burn is a human readable estimated burn rate (e.g. "~$400/month", empty if none).
	Burn string
}

type Cost struct {
	Service string
	Date    time.Time
	Amount  float64 // specified in good old fashioned us dollars
}

type Provider interface {
	// Provision rolls out a new account to the provider.
	Provision(ctx context.Context, name string) (string, error)
	// Prepare prepares the account for a challenge with necessary guards and configurations.
	Prepare(ctx context.Context, id string) error
	// Nuke erases all contents of an account. This may also raze guardrails / configs, therefore Prepare() must be called before reusing.
	Nuke(ctx context.Context, id string) error
	// Credentials generates shortlived credentials that a end-user will use to connect to the specified account.
	// The format is a generic string that may be json formatted so that the frontend can interpret it (must be human readable as fallback).
	Credentials(ctx context.Context, id string) (string, error)
	//
	Cost(ctx context.Context, id string) ([]Cost, error)
	// Check performs various heuristics on the account to check if anything (security, cost, etc.) is leaking.
	Check(ctx context.Context, id string) ([]Leak, error)
}
