package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// assume returns an sdk configuration with short-lived credentials for the
// specified role inside a member account, along with the session expiry.
func (r *Repository) assume(ctx context.Context, id string, role string) (awssdk.Config, time.Time, error) {
	session, err := r.sts.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         new(fmt.Sprintf("arn:aws:iam::%s:role/%s", id, role)),
		RoleSessionName: new("cloudjam-sandbox"),
		DurationSeconds: new(int32(r.config.SessionDuration.Seconds())),
	})
	if err != nil {
		return awssdk.Config{}, time.Time{}, fmt.Errorf("failed to assume role %q in account %q: %w", role, id, err)
	}
	config := r.config.Client.Copy()
	config.Credentials = credentials.NewStaticCredentialsProvider(
		*session.Credentials.AccessKeyId,
		*session.Credentials.SecretAccessKey,
		*session.Credentials.SessionToken,
	)
	return config, *session.Credentials.Expiration, nil
}

// alias returns the iam account alias enforced during bootstrap. aws-nuke
// refuses to touch accounts without an alias, which doubles as a safety net.
func (r *Repository) alias(id string) string {
	return fmt.Sprintf("cloudjam-sandbox-%s", id)
}

// bootstrap prepares a member account: iam account alias, sandbox admin role
// and the two guardrail scps. All steps are idempotent so Add can be re-run
// on dirty accounts. The alias and role are created before the scps attach,
// because the guard scp denies alias changes afterwards.
func (r *Repository) bootstrap(ctx context.Context, id string) error {
	config, _, err := r.assume(ctx, id, r.config.OrganizationRole)
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
