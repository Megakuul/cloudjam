package aws

import (
	"encoding/json"
	"fmt"

	"codeberg.org/megakuul/cloudjam/pkg/challenge"
)

// host is the guest's view of the capabilities the WASM host exposes behind
// pkg/challenge. Every method maps to exactly one host function; the wasip1
// build wires these to the extism imports, while non-wasm builds get an inert
// stub so the package still compiles as part of `go build ./...`.
type host interface {
	// RegisterMeta reports the challenge metadata (title, description, clues)
	// to the host. Sent once, during the init phase.
	RegisterMeta(challenge.InitInput) error

	ReadScore() (float64, error)
	UpdateScore(reason string, increment float64) error

	CreateResource(typeName, desired string) error
	// ReadResource returns the resource's current state as the raw Cloud
	// Control properties JSON.
	ReadResource(typeName, identifier string) (string, error)
	UpdateResource(typeName, identifier, patch string) error
	DeleteResource(typeName, identifier string) error

	// Completed / MarkCompleted persist per-objective award state across
	// invocations so points are never granted twice.
	Completed(id string) (bool, error)
	MarkCompleted(id string) error

	Log(msg string)
}

// Clue is an optional hint the player may reveal, at the cost of Cost points.
type Clue struct {
	// Text is the hint shown to the player. It doubles as the clue's key in the
	// host-side clue map, so keep it unique within a challenge.
	Text string
	// Cost is the score penalty for revealing the clue.
	Cost float64
}

// Check reports whether an objective's success condition currently holds. It is
// handed a Context so it can inspect live resource state, the running score, and
// anything else the host exposes. Returning an error aborts the whole
// evaluation (the host call fails); returning (false, nil) simply means "not yet".
type Check func(*Context) (bool, error)

// Objective is a scored goal. Its Check is evaluated on every host invocation;
// the first time it reports true, Points are awarded exactly once and the
// objective is marked complete so it never pays out again.
type Objective struct {
	// ID uniquely identifies the objective. It is the persistence key for the
	// "already awarded" bookkeeping, so it must be stable and unique.
	ID string
	// Description is a human-readable reason attached to the score update.
	Description string
	// Points is the reward granted when Check first succeeds.
	Points float64
	// Check is the success condition.
	Check Check
}

// Config is the declarative definition of a challenge: static metadata plus the
// logic that provisions resources and scores progress. It reads like a config
// with a few function fields for the behaviour that cannot be expressed as data.
type Config struct {
	// Title and Description are shown to the player.
	Title       string
	Description string

	// Clues are optional, cost-bearing hints.
	Clues []Clue

	// Setup runs once, during the init phase, before any objective is checked.
	// This is where the initial scenario is provisioned (Context.Create) — the
	// broken bucket, the over-permissive role, the misconfigured instance the
	// player must fix. It is optional.
	Setup func(*Context) error

	// Objectives are the scored goals, evaluated on every invocation.
	Objectives []Objective
}

// Challenge is a validated, ready-to-run challenge built from a Config. Create
// one with New, or register it directly with Serve.
type Challenge struct {
	cfg Config
}

// New builds a Challenge from a Config without registering it. Most plugins want
// Serve instead; New is handy for tests.
func New(cfg Config) *Challenge {
	return &Challenge{cfg: cfg}
}

// validate rejects configurations that would misbehave at runtime.
func (c *Challenge) validate() error {
	if c.cfg.Title == "" {
		return fmt.Errorf("challenge: Title must not be empty")
	}
	seen := make(map[string]struct{}, len(c.cfg.Objectives))
	for i, o := range c.cfg.Objectives {
		if o.ID == "" {
			return fmt.Errorf("challenge: objective %d has an empty ID", i)
		}
		if o.Check == nil {
			return fmt.Errorf("challenge: objective %q has no Check", o.ID)
		}
		if _, dup := seen[o.ID]; dup {
			return fmt.Errorf("challenge: duplicate objective ID %q", o.ID)
		}
		seen[o.ID] = struct{}{}
	}
	return nil
}

// initInput projects the static config onto the pkg/challenge wire struct.
func (c *Challenge) initInput() challenge.InitInput {
	clues := make(map[string]float64, len(c.cfg.Clues))
	for _, cl := range c.cfg.Clues {
		clues[cl.Text] = cl.Cost
	}
	return challenge.InitInput{
		Title:       c.cfg.Title,
		Description: c.cfg.Description,
		Clues:       clues,
	}
}

// phase distinguishes the lifecycle entry points the host may call.
type phase int

const (
	// phaseInit runs metadata registration + Setup, then a first evaluation.
	phaseInit phase = iota
	// phaseEvaluate re-checks objectives to award newly reached goals.
	phaseEvaluate
)

// run executes one lifecycle invocation against a host. It is pure — all
// platform specifics live behind the host interface — so it is exercised
// directly in tests with a mock host.
func (c *Challenge) run(h host, p phase) error {
	ctx := &Context{host: h}

	if p == phaseInit {
		if err := h.RegisterMeta(c.initInput()); err != nil {
			return fmt.Errorf("register metadata: %w", err)
		}
		if c.cfg.Setup != nil {
			if err := c.cfg.Setup(ctx); err != nil {
				return fmt.Errorf("setup: %w", err)
			}
		}
	}

	for _, o := range c.cfg.Objectives {
		done, err := h.Completed(o.ID)
		if err != nil {
			return fmt.Errorf("objective %q: read completion: %w", o.ID, err)
		}
		if done {
			continue
		}
		ok, err := o.Check(ctx)
		if err != nil {
			return fmt.Errorf("objective %q: %w", o.ID, err)
		}
		if !ok {
			continue
		}
		reason := o.Description
		if reason == "" {
			reason = o.ID
		}
		if err := h.UpdateScore(reason, o.Points); err != nil {
			return fmt.Errorf("objective %q: award: %w", o.ID, err)
		}
		if err := h.MarkCompleted(o.ID); err != nil {
			return fmt.Errorf("objective %q: mark completed: %w", o.ID, err)
		}
		h.Log(fmt.Sprintf("objective %q completed (+%g points)", o.ID, o.Points))
	}
	return nil
}

// registered is the single challenge this plugin serves. It is set from the
// plugin's package init (via Serve) and consumed by the exported entry points.
var registered *Challenge

// Serve registers cfg as the challenge this plugin exposes and returns the built
// Challenge. Call it from the plugin's package init so the challenge is in place
// before the host invokes any exported function:
//
//	func init() { aws.Serve(aws.Config{ ... }) }
//
// A plugin serves exactly one challenge; the last Serve wins.
func Serve(cfg Config) *Challenge {
	registered = New(cfg)
	return registered
}

// Register makes an already-built Challenge the one this plugin serves.
func Register(c *Challenge) { registered = c }

// Context is the handle passed to Setup and to every objective Check. It offers
// strongly typed resource operations and score access, all backed by the host.
type Context struct {
	host host
}

// Create provisions a resource from its typed goformation definition.
//
//	err := c.Create(&s3.Bucket{BucketName: aws.String("my-bucket")})
func (c *Context) Create(res Resource) error {
	typeName, desired, err := desiredState(res)
	if err != nil {
		return err
	}
	return c.host.CreateResource(typeName, desired)
}

// Read loads a resource's live state into a typed goformation value. The type
// name is taken from into, so pass a pointer to the matching struct and the
// primary identifier (usually the name property you set at Create time):
//
//	var b s3.Bucket
//	if err := c.Read("my-bucket", &b); err != nil { ... }
//	encrypted := b.BucketEncryption != nil
//
// Read-only attributes returned by the API that the struct does not model are
// ignored rather than causing an error.
func (c *Context) Read(identifier string, into Resource) error {
	state, err := c.host.ReadResource(into.AWSCloudFormationType(), identifier)
	if err != nil {
		return err
	}
	if state == "" {
		return fmt.Errorf("read %s %q: empty state", into.AWSCloudFormationType(), identifier)
	}
	return decodeProperties(into, []byte(state))
}

// ReadRaw returns a resource's live state as the raw Cloud Control properties
// JSON, for cases the typed Read cannot express. typeName is a Cloud Control
// type name such as "AWS::S3::Bucket".
func (c *Context) ReadRaw(typeName, identifier string) (json.RawMessage, error) {
	state, err := c.host.ReadResource(typeName, identifier)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(state), nil
}

// Update applies one or more JSON Patch operations to a provisioned resource.
// res only supplies the type name (an empty literal such as &s3.Bucket{} is
// fine); identifier selects the instance:
//
//	c.Update(&s3.Bucket{}, "my-bucket", aws.Replace("/BucketEncryption", enc))
func (c *Context) Update(res Resource, identifier string, ops ...PatchOp) error {
	if len(ops) == 0 {
		return fmt.Errorf("update %s %q: no patch operations", res.AWSCloudFormationType(), identifier)
	}
	patch, err := json.Marshal(ops)
	if err != nil {
		return fmt.Errorf("marshal patch: %w", err)
	}
	return c.host.UpdateResource(res.AWSCloudFormationType(), identifier, string(patch))
}

// Delete removes a provisioned resource. res supplies only the type name.
func (c *Context) Delete(res Resource, identifier string) error {
	return c.host.DeleteResource(res.AWSCloudFormationType(), identifier)
}

// Score returns the challenge's current score as tracked by the host.
func (c *Context) Score() (float64, error) {
	return c.host.ReadScore()
}

// Award grants points outside of the objective machinery. Prefer Objectives for
// normal scoring; Award is an escape hatch for bespoke logic in Setup or a
// custom Check. Note it is not deduplicated — call it once per event.
func (c *Context) Award(reason string, points float64) error {
	return c.host.UpdateScore(reason, points)
}

// Log writes a diagnostic message through the host's logging channel.
func (c *Context) Log(msg string) { c.host.Log(msg) }
