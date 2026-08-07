//go:build wasip1

// Package challenge is the plugin sdk.
package challenge

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
)

// DEVELOPER NOTICE: this code is specifically designed for the cloudjam plugin lifetime.
// It contains goroutine leaks, functions that disregard context and other usually bad practices.
// Please do not program normal go code like this. This is an exception that works due to the reduced scope and the short lifetime of plugins.

// Event defines a custom update in the scenario.
// Example: If the user reaches 500 points, start shutting down EC2 instances and add a check that awards points if enough are available.
type Event struct {
	Every   time.Duration // throttles duration (zero evaluates on every loop iteration (which is defined by Challenge.Interval))
	Event   func(ctx context.Context, s *Scenario) error
	Trigger func() (bool, error)

	last time.Time
}

// Check defines an entity that rewards points to the user if he solves part of the challenge.
// Example: If the user enabled
type Check struct {
	Points  float64              // points awarded if the check is positive (can also be negative).
	Every   time.Duration        // throttles duration (zero evaluates on every loop iteration (which is defined by Challenge.Interval))
	Repeat  bool                 // should the points be awarded on every evaluation?
	Trigger func() (bool, error) // condition that tells if the check is successful.

	last    time.Time
	retired bool
}

// Scenario is the main challenge subject, per challenge there should only be one scenario.
type Scenario struct {
	interval time.Duration // round check speed, defaults to 10s

	bootstrap func() error

	// init params exist to collect all metadata in the initial builder
	// and then perform one single CreateMeta() call, after init is true, updates are incrementally.
	initTitle        string
	initDescriptions []string
	initAssets       map[string]string
	initClues        map[string]string
	init             atomic.Bool

	checksLock sync.RWMutex
	checks     map[string]Check

	eventsLock   sync.RWMutex // also used for activeEvents
	events       map[string]Event
	activeEvents map[string]context.CancelFunc
}

// New creates the scenario for your plugin.
func New(title string, interval time.Duration, boostrap func() error) *Scenario {
	return &Scenario{
		bootstrap:    boostrap,
		initTitle:    title,
		interval:     max(10*time.Second, interval),
		init:         atomic.Bool{},
		initAssets:   map[string]string{},
		initClues:    map[string]string{},
		checks:       map[string]Check{},
		events:       map[string]Event{},
		activeEvents: map[string]context.CancelFunc{},
	}
}

// AddAsset uploads the provided asset with a specified name to the providers asset storage.
// The user will be able to retrieve this asset.
func (c *Scenario) AddAsset(name string, asset []byte) *Scenario {
	assetMeta, err := api.CreateAsset(asset)
	if err != nil {
		c.report(fmt.Errorf("failed to create asset '%s': %w", name, err))
		return c
	}
	newAssetMeta, err := api.UpdateAsset(api.UpdateAssetInput{
		OldName: assetMeta.Name,
		NewName: name,
	})
	if err != nil {
		c.report(fmt.Errorf("failed to create asset '%s': %w", name, err))
		return c
	}
	if !c.init.Load() {
		c.initAssets[newAssetMeta.NewURL] = name
	} else {
		if _, err := api.UpdateMeta(api.UpdateMetaInput{
			AdditionalAssets: map[string]string{
				newAssetMeta.NewURL: name,
			},
		}); err != nil {
			c.report(err)
		}
	}
	return c
}

// AddDescription adds a description text to the scenario.
func (c *Scenario) AddDescription(description string) *Scenario {
	if !c.init.Load() {
		c.initDescriptions = append(c.initDescriptions, description)
	} else {
		if _, err := api.UpdateMeta(api.UpdateMetaInput{
			AdditionalDescriptions: []string{description},
		}); err != nil {
			c.report(err)
		}
	}
	return c
}

// AddClue adds a clue to the scenario (hint is what the user always sees, secret is what he gets when uncovering the hint).
func (c *Scenario) AddClue(hint, secret string) *Scenario {
	if !c.init.Load() {
		c.initClues[hint] = secret
	} else {
		if _, err := api.UpdateMeta(api.UpdateMetaInput{
			AdditionalClues: map[string]string{
				hint: secret,
			},
		}); err != nil {
			c.report(err)
		}
	}
	return c
}

// AddCheck adds a check to the next cycle.
func (c *Scenario) AddCheck(name string, check Check) *Scenario {
	c.checksLock.Lock()
	defer c.checksLock.Unlock()
	c.checks[name] = check
	return c
}

// RemoveCheck removes the check in the next cycle.
func (c *Scenario) RemoveCheck(name string) *Scenario {
	c.checksLock.Lock()
	defer c.checksLock.Unlock()
	delete(c.checks, name)
	return c
}

// AddEvent adds a event to the next cycle.
func (c *Scenario) AddEvent(name string, event Event) *Scenario {
	c.eventsLock.Lock()
	defer c.eventsLock.Unlock()
	c.events[name] = event
	return c
}

// RemoveEvent removes the event in the next cycle.
func (c *Scenario) RemoveEvent(name string, event Event) *Scenario {
	c.eventsLock.Lock()
	defer c.eventsLock.Unlock()
	delete(c.events, name)
	if cancel, ok := c.activeEvents[name]; ok {
		cancel()
	}
	delete(c.activeEvents, name)
	return c
}

// Starts the specified scenario inside your wasm plugin.
// Start may only be called once (otherwise it will panic).
func (c *Scenario) Start() {
	if !c.init.CompareAndSwap(false, true) {
		panic("scenario cannot be started twice!")
	}
	if _, err := api.CreateMeta(api.CreateMetaInput{
		Title:        c.initTitle,
		Descriptions: c.initDescriptions,
		Clues:        c.initClues,
		Assets:       c.initAssets,
	}); err != nil {
		c.report(err)
	}

	if err := c.bootstrap(); err != nil {
		c.report(err)
	}

	for {
		c.evaluateChecks()
		c.orchestrateEvents()

		time.Sleep(c.interval)
	}
}

// orchestrateEvents checks all event triggers and starts / stops them accordingly.
func (c *Scenario) orchestrateEvents() {
	c.eventsLock.Lock()
	defer c.eventsLock.Unlock()
	for name, event := range c.events {
		active, err := event.Trigger()
		if err != nil {
			c.report(fmt.Errorf("event start trigger %q: %w", name, err))
			continue
		}
		if cancel, ok := c.activeEvents[name]; ok {
			if !active {
				cancel()
			}
		} else {
			if active {
				ctx, cancel := context.WithCancel(context.Background())
				c.activeEvents[name] = cancel
				go func() {
					if err := event.Event(ctx, c); err != nil {
						c.report(err)
					}
				}()
			}
		}
	}
}

// evaluateChecks evaluates all checks and awards points accordingly.
func (c *Scenario) evaluateChecks() {
	c.checksLock.Lock()
	defer c.checksLock.Unlock()
	now := time.Now()
	for name, check := range c.checks {
		if check.retired || check.Trigger == nil {
			continue
		}
		if !check.last.IsZero() && now.Sub(check.last) < check.Every {
			continue
		}
		check.last = now

		passed, err := check.Trigger()
		if err != nil {
			c.report(fmt.Errorf("check %q: %w", name, err))
			continue
		}
		if !passed {
			continue
		}
		if _, err := api.UpdateScore(api.UpdateScoreInput{
			Reason:    name,
			Increment: check.Points,
		}); err != nil {
			// Leave it live so the award is attempted again next round.
			c.report(fmt.Errorf("check %q: award: %w", name, err))
			continue
		}
		if !check.Repeat {
			check.retired = true
		}
	}
}

func (c *Scenario) report(err error) {
	_, rErr := api.Report(api.ReportInput{Error: err.Error()})
	if rErr != nil {
		slog.Error(fmt.Sprintf("failed to report error (%v): %v", rErr, err))
	}
}
