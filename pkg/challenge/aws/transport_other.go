//go:build !wasip1

package aws

import (
	"errors"

	"codeberg.org/megakuul/cloudjam/pkg/challenge"
)

// This file keeps the package compiling for non-wasm targets (so it is part of a
// normal `go build ./...` on the host), even though a challenge plugin is only
// ever built and run as a wasip1 reactor. The host functions do not exist off
// WASM, so every call is a no-op error. The exported entry points (init /
// evaluate) exist only in the wasip1 build.

var errNotWasm = errors.New("aws: host functions are only available inside a wasip1 plugin build")

type stubHost struct{}

func (stubHost) RegisterMeta(challenge.InitInput) error      { return errNotWasm }
func (stubHost) ReadScore() (float64, error)                 { return 0, errNotWasm }
func (stubHost) UpdateScore(string, float64) error           { return errNotWasm }
func (stubHost) CreateResource(string, string) error         { return errNotWasm }
func (stubHost) ReadResource(string, string) (string, error) { return "", errNotWasm }
func (stubHost) UpdateResource(string, string, string) error { return errNotWasm }
func (stubHost) DeleteResource(string, string) error         { return errNotWasm }
func (stubHost) Completed(string) (bool, error)              { return false, nil }
func (stubHost) MarkCompleted(string) error                  { return nil }
func (stubHost) Log(string)                                  {}
