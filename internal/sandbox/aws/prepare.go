package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
)

func (r *Provider) Prepare(ctx context.Context, id string) error {
	if r.blocked(id) {
		return fmt.Errorf("account %q is the management account or blocklisted", id)
	}
	r.organizations.UpdatePolicy(ctx, &organizations.UpdatePolicyInput{
		PolicyId: "",
	})
	r.organizations.CreatePolicy(ctx, &organizations.CreatePolicyInput{
		Content: new(""),
	})
	r.organizations.AttachPolicy(ctx, &organizations.AttachPolicyInput{
		TargetId: new(id),
	})

	// TODO also create roles

	config, err := r.assume(ctx, id, r.adminRole, time.Hour)
	if err != nil {
		return err
	}
	client := iam.NewFromConfig(config)

	aliases, err := client.ListAccountAliases(ctx, &iam.ListAccountAliasesInput{})
	if err != nil {
		return fmt.Errorf("failed to list account aliases: %w", err)
	}
	if len(aliases.AccountAliases) == 0 {
		_, err := client.CreateAccountAlias(ctx, &iam.CreateAccountAliasInput{
			AccountAlias: new(r.alias(id)),
		})
		if err != nil {
			return fmt.Errorf("failed to create account alias: %w", err)
		}
	}

	_, err = client.GetRole(ctx, &iam.GetRoleInput{RoleName: &r.config.SandboxRole})
	if _, missing := errors.AsType[*iamtypes.NoSuchEntityException](err); missing {
		trust := fmt.Sprintf(
			`{"Version":"2012-10-17","Statement":[{"Effect":"Allow","Principal":{"AWS":"arn:aws:iam::%s:root"},"Action":"sts:AssumeRole"}]}`,
			r.management,
		)
		_, err = client.CreateRole(ctx, &iam.CreateRoleInput{
			RoleName:                 &r.config.SandboxRole,
			Description:              new("managed by cloudjam sandbox; used for deployments and cleanup"),
			AssumeRolePolicyDocument: &trust,
		})
		if err != nil {
			return fmt.Errorf("failed to create sandbox role: %w", err)
		}
	} else if err != nil {
		return fmt.Errorf("failed to get sandbox role: %w", err)
	}
	_, err = client.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
		RoleName:  &r.config.SandboxRole,
		PolicyArn: new("arn:aws:iam::aws:policy/AdministratorAccess"),
	})
	if err != nil {
		return fmt.Errorf("failed to attach administrator policy to sandbox role: %w", err)
	}

	return r.attachPolicies(ctx, id)
}
