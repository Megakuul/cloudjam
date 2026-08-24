//go:build wasip1

// Command s3-encryption is an example cloudjam challenge plugin.
//
// The player is dropped into an account containing an unencrypted, publicly
// exposed S3 bucket, and scores points as they lock it down.
package main

import (
	"fmt"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/policy"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/s3"
	"github.com/google/uuid"
)

// bucketPrefix is only a prefix: bucket names are globally unique on real aws,
// so bootstrap appends a uuid and the checks work off the identifier it got
// back, never off this string.
const bucketPrefix = "cloudjam-encrypt-me"

// bucketTag is the tag the player has to keep on the bucket.
const bucketTag = "cloudjam"

// bucketRef is the primary identifier of the provisioned bucket. bootstrap runs
// to completion before the check loop starts, so the triggers can read it.
var bucketRef string

func main() {
	challenge.New("Lock Down the Bucket", 10*time.Second, bootstrap).
		AddDescription("A teammate spun up an S3 bucket with no encryption and no public-access guardrails. Secure it.").
		// clue prices are added to the team score, so they are negative: a clue
		// costs roughly a fifth of the check it unlocks.
		AddClue("encryption", "Default encryption lives under BucketEncryption.", -10).
		AddClue("public", "Block public access with a PublicAccessBlockConfiguration.", -12).
		AddClue("inventory", "The bucket carries a tag the audit relies on. Leave it alone.", -3).
		SetPermission(policy.Document{
			Version: policy.Version20121017,
			Statement: []policy.Statement{
				{Effect: policy.Deny, Action: policy.ActionAll, Resource: policy.ARNAll}, // default lock down and configure on bootstrap.
			},
		}).
		SetGuardrail(policy.Document{
			Version: policy.Version20121017,
			Statement: []policy.Statement{
				{Effect: policy.Allow, Action: policy.ActionAll, Resource: policy.ARNAll}, // no iam related permission so guardrail is not needed.
			},
		}).
		AddCheck("Enabled default encryption on the bucket", challenge.Check{
			Points:  50,
			Every:   15 * time.Second,
			Trigger: encrypted,
		}).
		AddCheck("Blocked public access to the bucket", challenge.Check{
			Points:  60,
			Every:   15 * time.Second,
			Trigger: locked,
		}).
		AddCheck("Kept the bucket tagged", challenge.Check{
			Points:  5,
			Every:   time.Minute,
			Repeat:  true,
			Trigger: tagged,
		}).
		Start()
}

func bootstrap(s *challenge.Scenario) error {
	name := fmt.Sprintf("%s-%s", bucketPrefix, uuid.NewString())
	ref, err := aws.Create(&s3.Bucket{
		BucketName: new(name),
		// the scenario: wide open on purpose, this is what the player fixes.
		PublicAccessBlockConfiguration: &s3.BucketPublicAccessBlockConfiguration{
			BlockPublicAcls:       new(false),
			BlockPublicPolicy:     new(false),
			IgnorePublicAcls:      new(false),
			RestrictPublicBuckets: new(false),
		},
		Tags: []s3.BucketTag{{Key: new(bucketTag), Value: new("s3-encryption")}},
	})
	if err != nil {
		return err
	}
	bucketRef = ref

	// prefer the arn cloud control reports, but do not depend on it: it is a
	// read-only attribute and not every environment fills it in.
	arn := fmt.Sprintf("arn:aws:s3:::%s", name)
	if created, err := aws.Read[*s3.Bucket](bucketRef); err == nil && created.Arn != nil {
		arn = *created.Arn
	}

	s.SetPermission(policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{
				Sid:    "S3Access",
				Effect: policy.Allow,
				Action: policy.ActionsFrom(
					s3.ActionsRead,
					s3.ActionsList,
					[]string{
						s3.ActionPutBucketPublicAccessBlock,
						s3.ActionPutEncryptionConfiguration,
						// so a player who drops the tag can put it back.
						s3.ActionPutBucketTagging,
					},
				),
				Resource: policy.ARNsFrom(arn, fmt.Sprintf("%s/*", arn)),
			},
		},
	})
	return nil
}

func encrypted() (bool, error) {
	b, err := readBucket()
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

func locked() (bool, error) {
	b, err := readBucket()
	if err != nil {
		return false, err
	}
	block := b.PublicAccessBlockConfiguration
	if block == nil {
		return false, nil
	}
	if block.BlockPublicAcls == nil || !*block.BlockPublicAcls {
		return false, nil
	}
	if block.BlockPublicPolicy == nil || !*block.BlockPublicPolicy {
		return false, nil
	}
	if block.IgnorePublicAcls == nil || !*block.IgnorePublicAcls {
		return false, nil
	}
	if block.RestrictPublicBuckets == nil || !*block.RestrictPublicBuckets {
		return false, nil
	}
	return true, nil
}

func tagged() (bool, error) {
	b, err := readBucket()
	if err != nil {
		return false, err
	}
	for _, tag := range b.Tags {
		if tag.Key != nil && *tag.Key == bucketTag {
			return true, nil
		}
	}
	return false, nil
}

// readBucket reads the provisioned bucket. It reports rather than awards when
// the bucket is missing, so a failed bootstrap cannot hand out points.
func readBucket() (*s3.Bucket, error) {
	if bucketRef == "" {
		return nil, fmt.Errorf("bucket was never provisioned")
	}
	return aws.Read[*s3.Bucket](bucketRef)
}
