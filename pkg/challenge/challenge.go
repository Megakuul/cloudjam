// Package challenge is the plugin sdk: fill in a Challenge, call Run.
//
//	func main() {
//		bucket := &s3.Bucket{BucketName: aws.String("cloudjam-demo")}
//
//		c := &challenge.Challenge{
//			Title:       "Lock Down the Bucket",
//			Description: "A bucket has no encryption. Fix it.",
//			Resources:   []challenge.Resource{bucket},
//		}
//		c.Checks = []challenge.Check{{
//			Name:   "Enabled default encryption",
//			Points: 50,
//			Every:  15 * time.Second,
//			Done:   encrypted,
//		}}
//		c.Run()
//	}
package challenge

import (
	"encoding/json"
	"fmt"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
)

// Resource is anything the challenge provisions. The generated aws resource
// types implement it.
type Resource interface {
	CloudControlType() string
}

// Check is one scored objective.
type Check struct {
	// Name is the text shown next to the score update.
	Name string
	// Points is awarded when Done first returns true.
	Points float64
	// Every throttles evaluation. Zero evaluates every round.
	Every time.Duration
	// Repeat awards the check every round it holds instead of once.
	Repeat bool
	// Done is the condition being scored.
	Done func() (bool, error)

	last    time.Time
	retired bool
}

// Challenge is a scenario: resources to provision and checks that score the
// player's progress against them.
type Challenge struct {
	Title       string
	Description string
	// Clues are hints the player can reveal, by id.
	Clues map[string]string
	// Resources are provisioned when Run starts and retried until they exist.
	Resources []Resource
	Checks    []Check
	// Interval is the wait between rounds. Defaults to 10s.
	Interval time.Duration
	// Timeout stops Run after this long. Zero runs until there is nothing left
	// to do, which for a repeating check is until the host tears the plugin down.
	Timeout time.Duration

	ids map[Resource]string
}

// ID is the identifier a provisioned resource got, which is how a check reaches
// a resource aws named itself. It is empty until the resource exists.
func (c *Challenge) ID(resource Resource) string { return c.ids[resource] }

// Run registers the challenge, provisions it and then scores the checks on a
// timer. It is the plugin's whole body: a wasm command module runs main at
// startup, so there is nothing to export.
func (c *Challenge) Run() {
	if c.Interval <= 0 {
		c.Interval = 10 * time.Second
	}
	c.ids = map[Resource]string{}

	if _, err := api.Init(api.InitInput{
		Title:       c.Title,
		Description: c.Description,
		Clues:       c.Clues,
	}); err != nil {
		c.report(err)
	}

	deadline := time.Time{}
	if c.Timeout > 0 {
		deadline = time.Now().Add(c.Timeout)
	}

	for {
		c.provision()
		c.evaluate()

		if c.finished() {
			return
		}
		if !deadline.IsZero() && time.Now().After(deadline) {
			return
		}
		time.Sleep(c.Interval)
	}
}

// provision creates the resources that are not there yet, retrying every round.
// It deliberately only ensures they exist: the player is supposed to change
// them, and enforcing their properties would undo that.
func (c *Challenge) provision() {
	for _, resource := range c.Resources {
		if c.ids[resource] != "" {
			continue
		}
		desired, err := json.Marshal(resource)
		if err != nil {
			c.report(err)
			continue
		}
		out, err := api.CreateResource(api.CreateResourceInput{
			Type:    resource.CloudControlType(),
			Desired: string(desired),
		})
		if err != nil {
			c.report(fmt.Errorf("create %s: %w", resource.CloudControlType(), err))
			continue
		}
		if out.Identifier == "" {
			c.report(fmt.Errorf("create %s: failed", resource.CloudControlType()))
			continue
		}
		c.ids[resource] = out.Identifier
	}
}

// evaluate runs the checks that are due and awards the ones that pass.
func (c *Challenge) evaluate() {
	now := time.Now()
	for i := range c.Checks {
		check := &c.Checks[i]
		if check.retired || check.Done == nil {
			continue
		}
		if !check.last.IsZero() && now.Sub(check.last) < check.Every {
			continue
		}
		check.last = now

		passed, err := check.Done()
		if err != nil {
			c.report(fmt.Errorf("check %q: %w", check.Name, err))
			continue
		}
		if !passed {
			continue
		}
		if _, err := api.UpdateScore(api.UpdateScoreInput{
			Reason:    check.Name,
			Increment: check.Points,
		}); err != nil {
			// Leave it live so the award is attempted again next round.
			c.report(fmt.Errorf("check %q: award: %w", check.Name, err))
			continue
		}
		if !check.Repeat {
			check.retired = true
		}
	}
}

// finished reports whether anything is left: a resource still missing, or a
// check that has not been retired.
func (c *Challenge) finished() bool {
	if len(c.ids) < len(c.Resources) {
		return false
	}
	for _, check := range c.Checks {
		if !check.retired && check.Done != nil {
			return false
		}
	}
	return true
}

func (c *Challenge) report(err error) {
	api.Report(api.ReportInput{Error: err.Error()})
}
