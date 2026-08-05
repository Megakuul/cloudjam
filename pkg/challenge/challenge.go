//go:build wasip1

// Package challenge is the plugin sdk.
package challenge

import (
	"encoding/json"
	"fmt"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
)

type Resource interface {
	CloudControlType() string
}

type Check struct {
	Name   string               // text shown next to the score update
	Points float64              // points awarded if the check is positive (can also be negative).
	Every  time.Duration        // throttles duration (zero evaluates on every loop iteration (which is defined by Challenge.Interval))
	Repeat bool                 // should the points be awarded on every evaluation?
	Done   func() (bool, error) // condition that tells if the check is successful.

	last    time.Time
	retired bool
}

// Challenge is a scenario: resources to provision and checks that score the
// player's progress against them.
type Challenge struct {
	Title       string
	Description string
	Clues       map[string]string
	Resources   []Resource
	Checks      []Check
	Interval    time.Duration // round check speed, defaults to 10s

	ids map[Resource]string
}

// Starts the specified challenge inside your wasm plugin.
func Start(c *Challenge) {
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

	for {
		c.provision()
		c.evaluate()

		if c.finished() {
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
