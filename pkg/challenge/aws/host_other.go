//go:build !wasip1

package aws

import (
	"errors"

	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
)

// This file keeps the package building for the host toolchain (so it is part of
// a plain `go build ./...`) even though a plugin only ever runs as a wasip1
// module. The extism imports do not exist off WASM, so every call fails.

// ErrNoHost is returned by every host-backed call outside a WASM plugin.
var ErrNoHost = errors.New("aws: host functions are only available inside a wasip1 plugin build")

type noHost struct{}

func newHost() transport { return noHost{} }

func (noHost) Init(api.InitInput) (api.InitOutput, error) {
	return api.InitOutput{}, ErrNoHost
}

func (noHost) Report(api.ReportInput) (api.ReportOutput, error) {
	return api.ReportOutput{}, ErrNoHost
}

func (noHost) ReadScore(api.ReadScoreInput) (api.ReadScoreOutput, error) {
	return api.ReadScoreOutput{}, ErrNoHost
}

func (noHost) UpdateScore(api.UpdateScoreInput) (api.UpdateScoreOutput, error) {
	return api.UpdateScoreOutput{}, ErrNoHost
}

func (noHost) CreateResource(api.CreateResourceInput) (api.CreateResourceOutput, error) {
	return api.CreateResourceOutput{}, ErrNoHost
}

func (noHost) ReadResource(api.ReadResourceInput) (api.ReadResourceOutput, error) {
	return api.ReadResourceOutput{}, ErrNoHost
}

func (noHost) UpdateResource(api.UpdateResourceInput) (api.UpdateResourceOutput, error) {
	return api.UpdateResourceOutput{}, ErrNoHost
}

func (noHost) DeleteResource(api.DeleteResourceInput) (api.DeleteResourceOutput, error) {
	return api.DeleteResourceOutput{}, ErrNoHost
}

func (noHost) ListResource(api.ListResourceInput) (api.ListResourceOutput, error) {
	return api.ListResourceOutput{}, ErrNoHost
}

func (noHost) Log(string)     {}
func (noHost) SetError(error) {}
func (noHost) Output(string)  {}
