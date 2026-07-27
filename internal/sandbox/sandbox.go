package sandbox

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
	// Service is the provider service that generated the cost (empty for daily totals).
	Service string
	// Date is the day of the data point in ISO format (empty for service totals).
	Date time.Time
	// Amount is the cost amount in the bill currency.
	Amount float64
}

// Account represents the current state of an account according to teh provider.
type Account struct {
	// ID is the provider specific account identifier.
	ID string
	// Name is the provider specific human readable account name.
	Name string
}

type Provider interface {
	Provision(ctx context.Context, id string) (*Account, error)
	Get(ctx context.Context, id string) (*Account, error)
	List(ctx context.Context) ([]*Account, error)
	// Credentials generates shortlived credentials that a end-user will use to connect to the specified account.
	// The credentials format is a generic string that may be json formatted so that the frontend can interpret it.
	Credentials(ctx context.Context, id string) (string, error)
	Cost(ctx context.Context, id string) ([]*Cost, error)
	Check(ctx context.Context, id string) ([]*Leak, error)
}
