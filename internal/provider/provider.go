package provider

import (
	"context"
	"log/slog"
	"time"
)

type Leak struct {
	Severity slog.Level
	Region   string // empty for global services
	Resource string
	Message  string
	Burn     string // human readable estimated burn rate (e.g. "~$400/month")
}

type Cost struct {
	Service string
	Date    time.Time
	Amount  float64 // specified in good old fashioned us dollars
}

type Provider interface {
	// Provision rolls out a new account to the provider. Returns the account id.
	Provision(ctx context.Context, name string) (string, error)
	// Prepare prepares the account for a challenge with necessary guards and configurations.
	Prepare(ctx context.Context, id string) error
	// Nuke erases all contents of an account. This may also raze guardrails / configs, therefore Prepare() must be called before reusing.
	Nuke(ctx context.Context, id string) error
	// Resources creates a resource controller that can be used to modify resources on the account.
	Resources(ctx context.Context, id string, lifetime time.Duration) (ResourceController, error)
	// Credentials generates shortlived credentials that a end-user will use to connect to the specified account.
	// The format is a generic string that may be json formatted so that the frontend can interpret it (must be human readable as fallback).
	Credentials(ctx context.Context, id string, lifetime time.Duration) (string, error)

	// Check performs some heuristics on the account to check if resources are leaking.
	Check(ctx context.Context, id string) ([]Leak, error)
	// Cost returns aggregated cost data for the specified account in the specified time window (data might be delayed).
	Cost(ctx context.Context, id string, window time.Duration) ([]Cost, error)
}

// ResourceController provides a CRUDL abstraction over the providers IaC API (e.g. for AWS it is cloud control json).
// Input and output params are in provider specific format and are expected to be parsed / serialized
// in the plugin sdk.
type ResourceController interface {
	// Creates a resource and waits until successfully deployed. Returns the id of the resource.
	Create(ctx context.Context, resourceType, resourceData string) (string, error)
	// Reads a resource with the specified type and id.
	Read(ctx context.Context, resourceType, resourceID string) (string, error)
	// Updates a resource and waits until successfully updated.
	Update(ctx context.Context, resourceType, resourceID string) error
	// Deletes a resource and returns (does not wait for resource deletion).
	Delete(ctx context.Context, resourceType, resourceID string) error
	// Lists all resources from the specified type.
	List(ctx context.Context, resourceType string) (map[string]string, error)
}
