// sandbox provides pooling and lifecycle management for disposable competitor
// environments (cloud accounts or bare machines) used in ctf challenges.
//
// Environments are registered manually and then cycle through the pool:
//
//	bootstrapping -> ready -> used -> cleaning -> ready
//	       |                             |
//	       `--------> dirty <------------´
//
// "dirty" marks environments where bootstrapping or cleanup failed (or where
// leftovers were detected after cleanup); those require operator attention and
// can be re-released to retry the cleanup.
//
// Provider implementations are defined as subpackages (aws, ...).
package sandbox

import (
	"context"
	"time"
)

// State describes the lifecycle position of a pooled account.
type State string

const (
	// StateBootstrapping marks accounts currently being prepared (guardrails, roles, ...).
	StateBootstrapping State = "bootstrapping"
	// StateReady marks clean and guarded accounts available for acquisition.
	StateReady State = "ready"
	// StateUsed marks accounts leased to a competitor.
	StateUsed State = "used"
	// StateCleaning marks accounts currently being wiped.
	StateCleaning State = "cleaning"
	// StateDirty marks accounts where bootstrap or cleanup failed; requires operator attention.
	StateDirty State = "dirty"
)

// Account represents one pooled account.
type Account struct {
	// ID is the provider specific account identifier.
	ID string
	// Name is the provider specific human readable account name.
	Name string
	// State is the current lifecycle state.
	State State
	// Owner identifies who acquired the account (empty unless used).
	Owner string
	// Updated is the time of the last state transition.
	Updated time.Time
}

// Credentials hold short-lived administrator credentials for one account.
type Credentials struct {
	// AccessKey is the provider specific key id (or username).
	AccessKey string
	// SecretKey is the provider specific secret.
	SecretKey string
	// Token is an optional session token.
	Token string
	// Expires is the time the credentials become invalid.
	Expires time.Time
}

// Severity classifies a check finding.
type Severity string

const (
	// SeverityInfo describes a passed heuristic or a negligible cost source.
	SeverityInfo Severity = "info"
	// SeverityWarning describes a slow money leak or a degraded heuristic.
	SeverityWarning Severity = "warning"
	// SeverityCritical describes an instant money burner or a disabled guardrail.
	SeverityCritical Severity = "critical"
)

// Finding is a single result of one leak heuristic. Passed heuristics are
// reported as well (severity info) to make checks fully transparent.
type Finding struct {
	// Severity classifies the finding.
	Severity Severity
	// Region locates the finding (empty for account global findings).
	Region string
	// Resource identifies the affected resource or heuristic.
	Resource string
	// Message describes what was found (or verified).
	Message string
	// Burn is a human readable estimated burn rate (e.g. "~$400/month", empty if none).
	Burn string
}

// Report is the result of a full account check.
type Report struct {
	// Account is the checked account id.
	Account string
	// Healthy is true if no critical finding was raised.
	Healthy bool
	// Findings contains one entry per evaluated heuristic (passed and failed).
	Findings []Finding
	// Checked is the time the report was generated.
	Checked time.Time
}

// Cost is one aggregated cost data point.
type Cost struct {
	// Service is the provider service that generated the cost (empty for daily totals).
	Service string
	// Date is the day of the data point in ISO format (empty for service totals).
	Date string
	// Amount is the cost amount in the bill currency.
	Amount float64
}

// Bill aggregates the recent spend of one account. Providers usually deliver
// billing data with up to ~24h delay, so this is a trailing indicator; use
// Repository.Check for early leak detection.
type Bill struct {
	// Account is the billed account id.
	Account string
	// Currency of all amounts (e.g. "USD").
	Currency string
	// Window is the covered trailing window in days.
	Window int
	// Total is the spend over the whole window.
	Total float64
	// Daily contains one total per day (oldest first).
	Daily []Cost
	// Services contains the spend per service over the window (highest first).
	Services []Cost
	// Leaking is true if at least one day exceeded the configured daily budget.
	Leaking bool
	// Checked is the time the bill was generated.
	Checked time.Time
}

// Repository manages a pool of sandbox accounts on one provider.
//
// All operations are safe for concurrent use. Release runs the full cleanup
// and therefore blocks for a long time (potentially >30min); callers usually
// invoke it from a background worker and observe progress via Get/List.
type Repository interface {
	// Add registers an existing account into the pool and bootstraps it
	// (guardrails, access roles, billing wiring). The account must be
	// prepared manually beforehand (e.g. invited into the organization).
	Add(ctx context.Context, id string) (*Account, error)

	// Remove detaches the guardrails and drops the account from the pool.
	// The account itself is not deleted or cleaned. Used accounts cannot
	// be removed.
	Remove(ctx context.Context, id string) error

	// Get returns a single pooled account.
	Get(ctx context.Context, id string) (*Account, error)

	// List returns all pooled accounts.
	List(ctx context.Context) ([]*Account, error)

	// Acquire leases a ready account to the specified owner and marks it used.
	Acquire(ctx context.Context, owner string) (*Account, error)

	// Release takes an account back, wipes it completely and returns it to
	// the ready pool. If the cleanup fails or leftovers are detected the
	// account transitions to dirty; releasing a dirty account retries the
	// cleanup.
	Release(ctx context.Context, id string) error

	// Credentials returns short-lived administrator credentials for the
	// account, used to deploy challenges or to hand out to competitors.
	Credentials(ctx context.Context, id string) (*Credentials, error)

	// Bill returns the recent (delayed) spend of the account and flags
	// whether it leaks money above the configured daily budget.
	Bill(ctx context.Context, id string) (*Bill, error)

	// Check runs all leak heuristics against the account: it verifies the
	// guardrails are still enabled and intact and scans for known money
	// burners. Every heuristic is reported, passed or failed.
	Check(ctx context.Context, id string) (*Report, error)
}
