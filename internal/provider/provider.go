package provider

import (
	"context"
	"io"
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
	// Access creates a access controller that can be used to modify challenge account access.
	Access(ctx context.Context, id string, lifetime time.Duration) (AccessController, error)
	// Assets creates a asset controller that can be used to manage challenge assets.
	Assets(ctx context.Context, id string, lifetime time.Duration) (AssetController, error)
	// Resources creates a resource controller that can be used to modify resources on the account.
	Resources(ctx context.Context, id string, lifetime time.Duration) (ResourceController, error)
	// Credentials generates shortlived credentials that a end-user will use to connect to the specified account.
	// The format is a generic string that may be json formatted so that the frontend can interpret it (must be human readable as fallback).
	Credentials(ctx context.Context, id string, lifetime time.Duration) (string, error)

	// Check performs some heuristics on the account to check if resources are leaking.
	Check(ctx context.Context, id string) ([]Leak, error)
	// Cost returns aggregated cost data for the specified account in the specified time window (data might be delayed).
	Cost(ctx context.Context, id string, window time.Duration) ([]Cost, error)

	// Delete removes the account from the provider and cleans up resources in the process.
	Delete(ctx context.Context, id string) error
}

// AccessController provides an API to modify an accounts challenge credential permissions / guardrails.
// All operations work with provider specific policy documents that limit the users access to the account resources.
type AccessController interface {
	// CreatePermission creates the initial access rights for the challenge user credential access.
	CreatePermission(ctx context.Context, policy string) error
	// UpdatePermission replaces the permission policy for the user credential access.
	UpdatePermission(ctx context.Context, policy string) error
	// CreateGuardrail creates a boundary for the user credential access.
	// Unlike the permission the guardrail is designed to avoid ANY kind of privilege escalation
	// (on the permission it may be part of the challenge to escalate from it).
	CreateGuardrail(ctx context.Context, policy string) error
	// UpdateGuardrail replaces the boundary for the user credential access.
	UpdateGuardrail(ctx context.Context, policy string) error
}

// AssetController provides a CRUD abstraction over the providers asset storage (e.g. for AWS it is s3).
// The idea of assets is to provide them locally in the provider account for the challenge (e.g. a go binary that does something on lambda).
type AssetController interface {
	// Creates an asset on the path (provider specific) and returns a human readable accessible url (provider specific).
	Create(ctx context.Context, path string, asset io.Reader) (string, error)
	// Read reads the asset from the path and returns the human readable accessible url and the asset.
	Read(ctx context.Context, path string) (string, io.ReadCloser, error)
	// Update changes the assets path and returns teh new human readable accessible url.
	Update(ctx context.Context, oldPath, newPath string) (string, error)
	// Delete removes the asset from teh path.
	Delete(ctx context.Context, path string) error
}

// ResourceController provides a CRUDL abstraction over the providers IaC API (e.g. for AWS it is cloud control json).
// Input and output params are in provider specific format and are expected to be parsed / serialized
// in the plugin sdk.
type ResourceController interface {
	// Creates a resource and waits until successfully deployed. Returns the id of the resource.
	Create(ctx context.Context, resourceType, resourceState string) (string, error)
	// Reads a resource with the specified type and id.
	Read(ctx context.Context, resourceType, resourceID string) (string, error)
	// Updates a resource and waits until successfully updated.
	Update(ctx context.Context, resourceType, resourceID, resourceState string) error
	// Deletes a resource and returns (does not wait for resource deletion).
	Delete(ctx context.Context, resourceType, resourceID string) error
	// Lists all resources from the specified type.
	List(ctx context.Context, resourceType string) (map[string]string, error)
}
