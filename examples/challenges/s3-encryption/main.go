//go:build wasip1

// Command s3-encryption is an example cloudjam challenge plugin.
//
// The player is dropped into an account containing an unencrypted, publicly
// exposed S3 bucket, and scores points as they lock it down.
package main

import (
	"fmt"
	"log/slog"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/s3"
)

const bucket = "cloudjam-encrypt-me"

func main() {
	challenge.New("Lock Down the Bucket", 10*time.Second).
		AddDescription("A teammate spun up an S3 bucket with no encryption and no public-access guardrails. Secure it.").
		AddClue("encryption", "Default encryption lives under BucketEncryption.").
		AddClue("public", "Block public access with a PublicAccessBlockConfiguration.").
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
	slog.Debug("testing the locked")
	b, err := aws.Read[*s3.Bucket](bucket)
	if err != nil || b.PublicAccessBlockConfiguration == nil {
		if err != nil {
			slog.Error(err.Error())
		}
		slog.Debug(*b.Arn)
		slog.Debug(fmt.Sprint(b.PublicAccessBlockConfiguration))
		return false, err
	}
	slog.Debug("now performing the check")
	block := b.PublicAccessBlockConfiguration
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
	slog.Debug("the final countdown")
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
