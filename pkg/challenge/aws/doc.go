// Package aws is the SDK for writing cloudjam AWS challenge plugins.
//
// A plugin is one main function: describe the challenge, declare the resources
// it needs and the checks that score it, then Run. There is nothing to export
// and no lifecycle to implement — the WASM runtime calls main at startup and Run
// drives the rest.
//
//	package main
//
//	import (
//		"time"
//
//		"codeberg.org/megakuul/cloudjam/pkg/challenge/aws"
//		"github.com/awslabs/goformation/v7/cloudformation/s3"
//	)
//
//	const bucket = "cloudjam-encrypt-me"
//
//	func main() {
//		c := aws.New("Lock Down the Bucket",
//			"A bucket was created without encryption. Turn it on.")
//		c.Clue("where", "Default encryption lives under BucketEncryption.")
//
//		c.Add(&s3.Bucket{BucketName: aws.String(bucket)})
//
//		c.Check("bucket-encrypted").
//			Reason("Enabled default encryption").
//			Points(50).
//			Every(30 * time.Second).
//			Done(func() (bool, error) {
//				b, err := aws.Read[*s3.Bucket](bucket)
//				if err != nil {
//					return false, err
//				}
//				return b.BucketEncryption != nil, nil
//			})
//
//		c.Run()
//	}
//
// # Resources
//
// The host backs the resource API with the AWS Cloud Control API, which speaks
// the same resource type names ("AWS::S3::Bucket") and property shapes as
// CloudFormation. Any goformation resource — everything under
// github.com/awslabs/goformation/v7/cloudformation/... — can therefore be handed
// straight to Add, Create, Read, Update, Delete and List, and the Cloud
// Control type name is derived from the Go type itself. Nothing to register,
// nothing to declare.
//
// Optional properties are pointers: aws.String / aws.Bool / aws.Int set them
// inline, aws.Value / aws.True read them back safely. Mutations are RFC 6902
// patches built with aws.Replace / aws.Add / aws.Remove.
//
// goformation models only writable properties, so read-only attributes AWS
// assigns (Arn, VpcId, instance state) have no struct field. ReadState returns
// the raw Cloud Control document alongside the typed value for those, and List
// keys its result by primary identifier, which is how you find resources AWS
// names itself.
//
// Add is the declarative path: it creates the resource when Run starts and keeps
// retrying until that succeeds, reporting failures to the host, so a plugin
// never handles a provisioning error. Create and friends are the imperative
// path, for setup a declaration cannot express.
//
// # Checks
//
// Every check is a condition plus a payout. By default it is awarded once, the
// first time the condition holds, and then retired. Repeat awards it every round
// the condition holds. Points is a fixed reward; PointsFrom computes one at
// award time. Every throttles how often an expensive condition is tested.
//
// The bookkeeping lives one layer up, in pkg/challenge, and is provider
// agnostic; this package supplies the AWS resource layer and the host transport.
// The raw host contract is pkg/challenge/api, which a plugin never touches.
//
// # Waiting
//
// Cloud Control mutations are asynchronous. Create, Update and Delete return as
// soon as the operation is accepted unless you pass aws.Wait, which blocks until
// it reaches a terminal state. Add always waits, so a check in the same round
// sees a resource that is really there.
//
// # Building
//
// A plugin is an ordinary wasip1 command module:
//
//	GOOS=wasip1 GOARCH=wasm go build \
//		-o challenge.wasm ./examples/challenges/s3-encryption
//
// Off wasip1 the package still compiles — that is what keeps it in `go build
// ./...` and testable — but every host-backed call returns ErrNoHost.
package aws
