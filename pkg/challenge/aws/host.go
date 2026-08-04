package aws

import "codeberg.org/megakuul/cloudjam/pkg/challenge/api"

// transport is the guest's view of the host functions declared in
// pkg/challenge/api. Every method maps to exactly one host call and speaks the raw
// wire structs; all typing happens in the layers above.
//
// The wasip1 build binds it to the extism imports, the host build to an inert
// stub, and tests swap it for a fake.
type transport interface {
	Init(api.InitInput) (api.InitOutput, error)
	Report(api.ReportInput) (api.ReportOutput, error)
	ReadScore(api.ReadScoreInput) (api.ReadScoreOutput, error)
	UpdateScore(api.UpdateScoreInput) (api.UpdateScoreOutput, error)

	CreateResource(api.CreateResourceInput) (api.CreateResourceOutput, error)
	ReadResource(api.ReadResourceInput) (api.ReadResourceOutput, error)
	UpdateResource(api.UpdateResourceInput) (api.UpdateResourceOutput, error)
	DeleteResource(api.DeleteResourceInput) (api.DeleteResourceOutput, error)
	ListResource(api.ListResourceInput) (api.ListResourceOutput, error)

	Log(msg string)
	SetError(err error)
	Output(msg string)
}

// host is the transport every exported function in this package goes through.
// newHost is supplied per build target.
var host transport = newHost()
