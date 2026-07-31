// Command s3-encryption is an example cloudjam challenge plugin.
//
// The player is dropped into an account containing an unencrypted, publicly
// blockable S3 bucket. Points are awarded as they lock it down: enabling default
// encryption, then blocking all public access.
//
// Build it as a wasip1 reactor:
//
//	GOOS=wasip1 GOARCH=wasm go build -buildmode=c-shared \
//		-o s3-encryption.wasm ./examples/challenges/s3-encryption
package main

import (
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws"
	"github.com/awslabs/goformation/v7/cloudformation/s3"
)

const bucketName = "cloudjam-s3-encryption"

func init() {
	aws.Serve(aws.Config{
		Title: "Lock Down the Bucket",
		Description: "A teammate spun up an S3 bucket with no encryption and no " +
			"public-access guardrails. Secure it using the Cloud Control API.",
		Clues: []aws.Clue{
			{Text: "Default encryption lives under BucketEncryption.", Cost: 5},
			{Text: "Block public access with a PublicAccessBlockConfiguration.", Cost: 5},
		},

		// Provision the deliberately-insecure starting point.
		Setup: func(c *aws.Context) error {
			return c.Create(&s3.Bucket{
				BucketName: aws.String(bucketName),
			})
		},

		Objectives: []aws.Objective{
			{
				ID:          "default-encryption",
				Description: "Enabled default encryption on the bucket",
				Points:      50,
				Check: func(c *aws.Context) (bool, error) {
					var b s3.Bucket
					if err := c.Read(bucketName, &b); err != nil {
						return false, err
					}
					return hasDefaultEncryption(&b), nil
				},
			},
			{
				ID:          "block-public-access",
				Description: "Blocked all public access to the bucket",
				Points:      50,
				Check: func(c *aws.Context) (bool, error) {
					var b s3.Bucket
					if err := c.Read(bucketName, &b); err != nil {
						return false, err
					}
					return blocksAllPublicAccess(b.PublicAccessBlockConfiguration), nil
				},
			},
		},
	})
}

func hasDefaultEncryption(b *s3.Bucket) bool {
	if b.BucketEncryption == nil {
		return false
	}
	for _, rule := range b.BucketEncryption.ServerSideEncryptionConfiguration {
		if rule.ServerSideEncryptionByDefault != nil {
			return true
		}
	}
	return false
}

func blocksAllPublicAccess(cfg *s3.Bucket_PublicAccessBlockConfiguration) bool {
	if cfg == nil {
		return false
	}
	on := func(p *bool) bool { return p != nil && *p }
	return on(cfg.BlockPublicAcls) &&
		on(cfg.BlockPublicPolicy) &&
		on(cfg.IgnorePublicAcls) &&
		on(cfg.RestrictPublicBuckets)
}

func main() {}
