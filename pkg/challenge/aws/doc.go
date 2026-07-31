// Package aws is a plugin-friendly SDK for writing AWS "gameday"-style challenge
// plugins that compile to a WASM guest for the cloudjam host.
//
// # Model
//
// The host (see internal/challenge, which drives the Cloud Control API behind
// pkg/challenge) loads a challenge plugin and calls its exported "init"
// function; from then on the guest drives the scenario, issuing resource and
// score operations back to the host. This package hides all of that: an author
// declares a Config and the SDK owns the extism import/export plumbing, the
// lifecycle, JSON marshalling, and idempotent scoring.
//
// A plugin is one Go file. It declares a challenge with typed CloudFormation
// resources (via goformation) and scores the player when the account reaches a
// desired state:
//
//	package main
//
//	import (
//		"codeberg.org/megakuul/cloudjam/pkg/challenge/aws"
//		"github.com/awslabs/goformation/v7/cloudformation/s3"
//	)
//
//	func init() {
//		aws.Serve(aws.Config{
//			Title:       "Encrypt the Bucket",
//			Description: "An S3 bucket was created without encryption. Turn it on.",
//			Clues: []aws.Clue{
//				{Text: "Cloud Control patches use JSON Pointer paths.", Cost: 5},
//			},
//			// Setup provisions the initial (broken) scenario.
//			Setup: func(c *aws.Context) error {
//				return c.Create(&s3.Bucket{
//					BucketName: aws.String("cloudjam-encrypt-me"),
//				})
//			},
//			Objectives: []aws.Objective{
//				{
//					ID:          "bucket-encrypted",
//					Description: "Enabled default encryption on the bucket",
//					Points:      50,
//					Check: func(c *aws.Context) (bool, error) {
//						var b s3.Bucket
//						if err := c.Read("cloudjam-encrypt-me", &b); err != nil {
//							return false, err
//						}
//						return b.BucketEncryption != nil, nil
//					},
//				},
//			},
//		})
//	}
//
//	func main() {}
//
// # Resources
//
// Any goformation resource (github.com/awslabs/goformation/v7/cloudformation/...)
// is an aws.Resource. The SDK translates it to the exact type name and property
// shape the Cloud Control API expects, so Context.Create / Read / Update /
// Delete are fully type-safe. Optional scalar properties are pointers; use the
// aws.String / aws.Bool / aws.Int helpers to set them inline. Mutations are
// expressed as JSON Patch operations via aws.Replace / aws.Add / aws.Remove.
//
// # Scoring
//
// Each Objective's Check is evaluated on every host invocation. The first time
// it returns true the Points are awarded exactly once; completion is persisted
// so re-evaluation never double-pays. Objective IDs must be unique and stable.
//
// # Building
//
// A plugin MUST be built as a wasip1 reactor so the Go runtime initialises (and
// runs the package init that calls Serve) while leaving the exported functions
// callable:
//
//	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared -o challenge.wasm ./path/to/plugin
//
// A plain (non c-shared) build produces a command whose exports trap with
// "runtime notInitialized" — always pass -buildmode=c-shared.
package aws
