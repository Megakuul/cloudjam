// Command s3-encryption is an example cloudjam challenge plugin.
//
// The player is dropped into an account containing an unencrypted, publicly
// exposed S3 bucket, and scores points as they lock it down.
//
// Build it as a wasip1 command module — the runtime calls main at startup, so
// there is nothing to export:
//
//	GOOS=wasip1 GOARCH=wasm go build -o s3-encryption.wasm ./examples/challenges/s3-encryption
package main

import (
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws"
	"github.com/awslabs/goformation/v7/cloudformation/s3"
	"github.com/awslabs/goformation/v7/cloudformation/tags"
)

const bucket = "cloudjam-encrypt-me"

func main() {
	c := aws.New("Lock Down the Bucket",
		"A teammate spun up an S3 bucket with no encryption and no public-access "+
			"guardrails. Secure it.")

	c.Clue("encryption", "Default encryption lives under BucketEncryption.")
	c.Clue("public", "Block public access with a PublicAccessBlockConfiguration.")

	// The deliberately insecure starting point. No error to handle: it is created
	// when Run starts and retried until it succeeds, with failures reported.
	c.Add(&s3.Bucket{
		BucketName: aws.String(bucket),
		Tags:       []tags.Tag{{Key: "cloudjam", Value: "s3-encryption"}},
	})

	c.Check("default-encryption").
		Reason("Enabled default encryption on the bucket").
		Points(50).
		Every(15 * time.Second).
		Done(encrypted)

	// Partial credit: 15 points per public-access guardrail, awarded once all
	// four are on.
	c.Check("block-public-access").
		Reason("Blocked public access to the bucket").
		PointsFrom(func() (float64, error) {
			b, err := aws.Read[*s3.Bucket](bucket)
			if err != nil {
				return 0, err
			}
			return float64(15 * guardrails(b)), nil
		}).
		Every(15 * time.Second).
		Done(func() (bool, error) {
			b, err := aws.Read[*s3.Bucket](bucket)
			if err != nil {
				return false, err
			}
			return guardrails(b) == 4, nil
		})

	// Repeat: 5 points every round the bucket stays tagged, so keeping the
	// account tidy pays for as long as the challenge runs.
	c.Check("stays-tagged").
		Reason("Kept the bucket tagged").
		Points(5).
		Repeat().
		Every(time.Minute).
		Done(tagged)

	c.Run()
}

func encrypted() (bool, error) {
	b, err := aws.Read[*s3.Bucket](bucket)
	if err != nil {
		return false, err
	}
	if b.BucketEncryption == nil {
		return false, nil
	}
	for _, rule := range b.BucketEncryption.ServerSideEncryptionConfiguration {
		if rule.ServerSideEncryptionByDefault != nil {
			return true, nil
		}
	}
	return false, nil
}

// guardrails counts how many of the four public-access blocks are switched on.
func guardrails(b *s3.Bucket) int {
	cfg := b.PublicAccessBlockConfiguration
	if cfg == nil {
		return 0
	}
	on := 0
	for _, flag := range []*bool{
		cfg.BlockPublicAcls,
		cfg.BlockPublicPolicy,
		cfg.IgnorePublicAcls,
		cfg.RestrictPublicBuckets,
	} {
		if aws.True(flag) {
			on++
		}
	}
	return on
}

func tagged() (bool, error) {
	b, err := aws.Read[*s3.Bucket](bucket)
	if err != nil {
		return false, err
	}
	for _, tag := range b.Tags {
		if tag.Key == "cloudjam" {
			return true, nil
		}
	}
	return false, nil
}
