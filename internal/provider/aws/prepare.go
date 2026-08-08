package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/google/uuid"
)

func (p *Provider) Prepare(ctx context.Context, id string) error {
	if p.managementAccount == id {
		return fmt.Errorf("account %q is the management account", id)
	}
	config, err := p.assume(ctx, id, p.adminRole, time.Hour)
	if err != nil {
		return err
	}
	iamClient := iam.NewFromConfig(config)
	s3Client := s3.NewFromConfig(config)

	sandboxTrust, err := json.Marshal(policyDocument{
		Version: "2012-10-17",
		Statement: []policyStatement{
			{
				Effect: "Allow",
				Principal: map[string]string{
					"AWS": fmt.Sprintf("arn:aws:iam::%s:root", id),
				},
				Action: []string{"sts:AssumeRole"},
			},
		},
	})

	_, err = iamClient.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 &p.sandboxRole,
		AssumeRolePolicyDocument: new(string(sandboxTrust)),
	})
	if err != nil {
		if _, ok := errors.AsType[*iamtypes.EntityAlreadyExistsException](err); !ok {
			return fmt.Errorf("creating sandbox role: %v", err)
		}
	}

	_, err = iamClient.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{
		RoleName:   &p.sandboxRole,
		PolicyName: new(sandboxInlinePolicy),
	})
	if err != nil {
		if _, ok := errors.AsType[*iamtypes.NoSuchEntityException](err); !ok {
			return fmt.Errorf("failed to remove old role policy from sandbox role: %w", err)
		}
	}

	p.assetBucket = fmt.Sprintf("asset-bucket-%s", uuid.NewString())
	_, err = s3Client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: new(p.assetBucket),
	})
	if err != nil {
		return fmt.Errorf("failed to create asset bucket: %w", err)
	}
	return nil
}
