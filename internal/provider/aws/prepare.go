package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

func (p *Provider) Prepare(ctx context.Context, id string) error {
	if p.blocked(id) {
		return fmt.Errorf("account %q is the management account or blocklisted", id)
	}
	config, err := p.assume(ctx, id, p.adminRole, time.Hour)
	if err != nil {
		return err
	}
	iamClient := iam.NewFromConfig(config)

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
		PermissionsBoundary:      &p.boundaryARN,
	})
	if err != nil {
		if _, ok := errors.AsType[*iamtypes.EntityAlreadyExistsException](err); !ok {
			return fmt.Errorf("creating sandbox role: %v", err)
		}
	}

	sandboxPolicy, err := json.Marshal(policyDocument{
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

	_, err = iamClient.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       &p.sandboxRole,
		PolicyName:     new("cloudjam"),
		PolicyDocument: new(string(sandboxPolicy)),
	})
	if err != nil {
		return fmt.Errorf("failed to attach role policy to sandbox role: %w", err)
	}
	return nil
}
