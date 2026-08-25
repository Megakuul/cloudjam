package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/provider"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

type AccessController struct {
	accountID      string
	permissionRole string
	boundaryPolicy string
	client         *iam.Client
}

// NewAccessController creates a aws iam access controller.
// Usually you would create the asset controller from provider.Access(), this is just for external tooling.
func NewAccessController(client *iam.Client, accountID, permissionRole, boundaryPolicy string) *AccessController {
	return &AccessController{accountID: accountID, permissionRole: permissionRole, boundaryPolicy: boundaryPolicy, client: client}
}

func (p *Provider) Access(ctx context.Context, id string, lifetime time.Duration) (provider.AccessController, error) {
	cfg, err := p.assume(ctx, id, p.adminRole, lifetime)
	if err != nil {
		return nil, err
	}
	return &AccessController{
		client:         iam.NewFromConfig(cfg),
		accountID:      id,
		permissionRole: p.sandboxRole,
		boundaryPolicy: p.boundaryPolicy,
	}, nil
}

func (a *AccessController) CreatePermission(ctx context.Context, policy string) error {
	_, err := a.client.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       &a.permissionRole,
		PolicyName:     new(sandboxInlinePolicy),
		PolicyDocument: &policy,
	})
	if err != nil {
		return fmt.Errorf("failed to create permission: %w", err)
	}
	return nil
}

func (a *AccessController) UpdatePermission(ctx context.Context, policy string) error {
	_, err := a.client.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       &a.permissionRole,
		PolicyName:     new(sandboxInlinePolicy),
		PolicyDocument: &policy,
	})
	if err != nil {
		return fmt.Errorf("failed to update permission: %w", err)
	}
	return nil
}

func (a *AccessController) CreateGuardrail(ctx context.Context, policy string) error {
	_, err := a.client.CreatePolicy(ctx, &iam.CreatePolicyInput{
		PolicyName:     new(a.boundaryPolicy),
		PolicyDocument: &policy,
	})
	if err != nil {
		return fmt.Errorf("failed to create permission boundary: %w", err)
	}
	_, err = a.client.PutRolePermissionsBoundary(ctx, &iam.PutRolePermissionsBoundaryInput{
		RoleName:            &a.permissionRole,
		PermissionsBoundary: new(fmt.Sprintf("arn:aws:iam::%s:policy/%s", a.accountID, a.boundaryPolicy)),
	})
	if err != nil {
		return fmt.Errorf("failed to update permission boundary: %w", err)
	}
	return nil
}

func (a *AccessController) UpdateGuardrail(ctx context.Context, policy string) error {
	_, err := a.client.CreatePolicyVersion(ctx, &iam.CreatePolicyVersionInput{
		PolicyArn:      new(fmt.Sprintf("arn:aws:iam::%s:policy/%s", a.accountID, a.boundaryPolicy)),
		PolicyDocument: &policy,
		SetAsDefault:   true,
	})
	if err != nil {
		return fmt.Errorf("failed to update permission boundary: %w", err)
	}
	return nil
}

func (a *AccessController) Lock(ctx context.Context) error {
	lockPolicy, err := json.Marshal(policyDocument{
		Version: "2012-10-17",
		Statement: []policyStatement{
			{
				Effect:   "Deny",
				Action:   []string{"*"},
				Resource: []string{"*"},
			},
		},
	})
	if err != nil {
		return err
	}
	_, err = a.client.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       &a.permissionRole,
		PolicyName:     new(sandboxInlinePolicy),
		PolicyDocument: new(string(lockPolicy)),
	})
	if err != nil {
		return fmt.Errorf("failed to update permission: %w", err)
	}
	return nil
}
