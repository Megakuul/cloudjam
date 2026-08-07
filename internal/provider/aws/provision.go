package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
)

func (p *Provider) Provision(ctx context.Context, name string) (string, error) {
	createResp, err := p.organizations.CreateAccount(ctx, &organizations.CreateAccountInput{
		AccountName:            &name,
		Email:                  new(name + p.emailSuffix),
		IamUserAccessToBilling: orgtypes.IAMUserAccessToBillingDeny,
		RoleName:               &p.adminRole,
	})
	if err != nil {
		return "", err
	}
	for {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(time.Minute * 5):
		}
		descResp, err := p.organizations.DescribeCreateAccountStatus(ctx, &organizations.DescribeCreateAccountStatusInput{
			CreateAccountRequestId: createResp.CreateAccountStatus.Id,
		})
		if err != nil {
			return "", err
		}
		switch descResp.CreateAccountStatus.State {
		case orgtypes.CreateAccountStateInProgress:
			break
		case orgtypes.CreateAccountStateFailed:
			return "", fmt.Errorf("account creation failed for '%s'; please inspect the account manually", *descResp.CreateAccountStatus.AccountId)
		}

		_, err = p.organizations.MoveAccount(ctx, &organizations.MoveAccountInput{
			AccountId:           descResp.CreateAccountStatus.AccountId,
			SourceParentId:      &p.rootOU,
			DestinationParentId: &p.cloudjamOU,
		})
		if err != nil {
			return "", fmt.Errorf("account ou assignment failed for '%s': %w", *descResp.CreateAccountStatus.AccountId, err)
		}
		return *descResp.CreateAccountStatus.AccountId, nil
	}
}
