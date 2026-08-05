//go:build wasip1

// Command s3-encryption is an example cloudjam challenge plugin.
//
// The player is dropped into an account containing an unencrypted, publicly
// exposed S3 bucket, and scores points as they lock it down.
package main

import (
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/s3"
)

const bucket = "cloudjam-encrypt-me"

func main() {
	c := &challenge.Challenge{
		Title: "Lock Down the Bucket",
		Description: "A teammate spun up an S3 bucket with no encryption and no " +
			"public-access guardrails. Secure it.",
		Clues: map[string]string{
			"encryption": "Default encryption lives under BucketEncryption.",
			"public":     "Block public access with a PublicAccessBlockConfiguration.",
		},
		Resources: []challenge.Resource{
			&s3.Bucket{
				BucketName: new(bucket),
				Tags:       []s3.BucketTag{{Key: new("cloudjam"), Value: new("s3-encryption")}},
			},
		},
		Checks: []challenge.Check{
			{
				Name:   "Enabled default encryption on the bucket",
				Points: 50,
				Every:  15 * time.Second,
				Done:   encrypted,
			},
			{
				Name:   "Blocked public access to the bucket",
				Points: 60,
				Every:  15 * time.Second,
				Done:   locked,
			},
			{
				// Repeats: keeping the account tidy pays every round.
				Name:   "Kept the bucket tagged",
				Points: 5,
				Every:  time.Minute,
				Repeat: true,
				Done:   tagged,
			},
		},
	}
	challenge.Start(c)
}

func encrypted() (bool, error) {
	b, err := aws.Read[*s3.Bucket](bucket)
	if err != nil || b.BucketEncryption == nil {
		return false, err
	}
	for _, rule := range b.BucketEncryption.ServerSideEncryptionConfiguration {
		if rule.ServerSideEncryptionByDefault != nil {
			return true, nil
		}
	}
	return false, nil
}

func locked() (bool, error) {
	b, err := aws.Read[*s3.Bucket](bucket)
	if err != nil || b.PublicAccessBlockConfiguration == nil {
		return false, err
	}
	block := b.PublicAccessBlockConfiguration
	for _, flag := range []*bool{
		block.BlockPublicAcls,
		block.BlockPublicPolicy,
		block.IgnorePublicAcls,
		block.RestrictPublicBuckets,
	} {
		if flag == nil || !*flag {
			return false, nil
		}
	}
	return true, nil
}

func tagged() (bool, error) {
	b, err := aws.Read[*s3.Bucket](bucket)
	if err != nil {
		return false, err
	}
	for _, tag := range b.Tags {
		if tag.Key != nil && *tag.Key == "cloudjam" {
			return true, nil
		}
	}
	return false, nil
}
