package challenge

import (
	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
	"fmt"
	"time"
)

// Host is everything a Challenge needs from the plugin host. A provider package
// (pkg/challenge/aws) implements it over the WASM imports; tests use a fake.
type Host interface {
	Register(api.InitInput) error
	Report(message string) error
	Award(reason string, points float64) error
	Log(message string)
}

// Challenge is a scenario: some resources to provision and some checks that
// score the player's progress against them.
//
// Build one with New, declare what it needs, then Run it:
//
//	c := challenge.New(host, "Lock Down the Bucket", "Secure it.")
//	c.Provision("bucket", createBucket)
//	c.Check("encrypted").Points(50).Done(encrypted)
//	c.Run()
type Challenge struct {
	host  Host
	meta  api.InitInput
	steps []*step
	specs []*Spec

	interval time.Duration
	timeout  time.Duration

	// now and sleep are swappable so tests can drive the loop without waiting.
	now   func() time.Time
	sleep func(time.Duration)
}

// step is provisioning work, retried every round until it succeeds once.
type step struct {
	name string
	run  func() error
	done bool
}

// New builds a challenge that reports to host and shows title and description to
// the player.
func New(host Host, title, description string) *Challenge {
	return &Challenge{
		host:     host,
		meta:     api.InitInput{Title: title, Description: description},
		interval: 10 * time.Second,
		now:      time.Now,
		sleep:    time.Sleep,
	}
}

// Clue adds a hint the player can reveal.
func (c *Challenge) Clue(id, text string) {
	if c.meta.Clues == nil {
		c.meta.Clues = map[string]string{}
	}
	c.meta.Clues[id] = text
}

// Interval sets how long Run waits between rounds. Defaults to 10s.
func (c *Challenge) Interval(d time.Duration) { c.interval = d }

// Timeout stops Run after d, even with checks still outstanding. Zero (the
// default) means run until there is nothing left to do — which for a challenge
// with a repeating check is forever, until the host tears the plugin down.
func (c *Challenge) Timeout(d time.Duration) { c.timeout = d }

// Provision registers setup work. Run retries it every round until it succeeds,
// reporting each failure, so callers never handle the error themselves. Provider
// packages build their typed resource declarations on top of this.
func (c *Challenge) Provision(name string, run func() error) {
	c.steps = append(c.steps, &step{name: name, run: run})
}

// Check starts declaring a scored objective and returns its builder:
//
//	c.Check("bucket-encrypted").
//		Reason("Enabled default encryption").
//		Points(50).
//		Every(30 * time.Second).
//		Done(encrypted)
//
// The check is registered immediately, so the builder calls may come in any
// order and any of them may be left out.
func (c *Challenge) Check(id string) *Spec {
	spec := &Spec{id: id}
	c.specs = append(c.specs, spec)
	return spec
}

// Run executes the challenge: register the metadata, provision the scenario,
// then evaluate the checks on a timer until nothing is left to do.
//
// It is the plugin's whole body — a WASM command module runs main at startup, so
// there is no lifecycle to export and nothing else to call.
func (c *Challenge) Run() {
	if err := c.host.Register(c.meta); err != nil {
		c.report(fmt.Errorf("register challenge: %w", err))
	}

	var deadline time.Time
	if c.timeout > 0 {
		deadline = c.now().Add(c.timeout)
	}

	for {
		c.provision()
		c.evaluate()

		if !c.pending() {
			c.host.Log("challenge: nothing left to do")
			return
		}
		if !deadline.IsZero() && !c.now().Before(deadline) {
			c.host.Log("challenge: timeout reached")
			return
		}
		c.sleep(c.interval)
	}
}

// provision retries every step that has not yet succeeded.
func (c *Challenge) provision() {
	for _, s := range c.steps {
		if s.done {
			continue
		}
		if err := s.run(); err != nil {
			c.report(fmt.Errorf("provision %s: %w", s.name, err))
			continue
		}
		s.done = true
		c.host.Log(fmt.Sprintf("provisioned %s", s.name))
	}
}

// evaluate runs every check that is due and awards the ones that pass.
func (c *Challenge) evaluate() {
	now := c.now()
	for _, s := range c.specs {
		if s.retired || !s.due(now) {
			continue
		}
		s.last = now

		if s.done == nil {
			c.report(fmt.Errorf("check %q has no condition", s.id))
			s.retired = true
			continue
		}
		passed, err := s.done()
		if err != nil {
			c.report(fmt.Errorf("check %q: %w", s.id, err))
			continue
		}
		if !passed {
			continue
		}

		points, err := s.award()
		if err != nil {
			c.report(fmt.Errorf("check %q: points: %w", s.id, err))
			continue
		}
		if err := c.host.Award(s.why(), points); err != nil {
			// Leave the check live so the award is attempted again next round.
			c.report(fmt.Errorf("check %q: award: %w", s.id, err))
			continue
		}
		c.host.Log(fmt.Sprintf("check %q passed (+%g points)", s.id, points))

		if !s.repeat {
			s.retired = true
		}
	}
}

// pending reports whether any work is left: a step still failing, or a check
// that has not been retired.
func (c *Challenge) pending() bool {
	for _, s := range c.steps {
		if !s.done {
			return true
		}
	}
	for _, s := range c.specs {
		if !s.retired {
			return true
		}
	}
	return false
}

// report sends a failure to the host's report endpoint, falling back to the log
// if even that fails — at which point there is nowhere else to put it.
func (c *Challenge) report(err error) {
	if reportErr := c.host.Report(err.Error()); reportErr != nil {
		c.host.Log(fmt.Sprintf("report failed: %v (original: %v)", reportErr, err))
	}
}
