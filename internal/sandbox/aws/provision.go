package aws

import (
	"context"
	"fmt"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/sandbox"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
)

func (r *Provider) Provision(ctx context.Context, name string) (*sandbox.Account, error) {
	createResp, err := r.organizations.CreateAccount(ctx, &organizations.CreateAccountInput{
		AccountName:            &name,
		Email:                  new(name + r.emailSuffix),
		IamUserAccessToBilling: orgtypes.IAMUserAccessToBillingDeny,
		RoleName:               &r.adminRole,
	})
	if err != nil {
		return nil, err
	}
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(time.Minute * 5):
		}
		descResp, err := r.organizations.DescribeCreateAccountStatus(ctx, &organizations.DescribeCreateAccountStatusInput{
			CreateAccountRequestId: createResp.CreateAccountStatus.Id,
		})
		if err != nil {
			return nil, err
		}
		switch descResp.CreateAccountStatus.State {
		case orgtypes.CreateAccountStateInProgress:
			break
		case orgtypes.CreateAccountStateFailed:
			return nil, fmt.Errorf("account creation failed for '%s'; please inspect the account manually", *descResp.CreateAccountStatus.AccountId)
		}

		_, err = r.organizations.MoveAccount(ctx, &organizations.MoveAccountInput{
			AccountId:           descResp.CreateAccountStatus.AccountId,
			SourceParentId:      &r.rootOU,
			DestinationParentId: &r.cloudjamOU,
		})
		if err != nil {
			return nil, fmt.Errorf("account ou assignment failed for '%s': %w", *descResp.CreateAccountStatus.AccountId, err)
		}
		return &sandbox.Account{
			ID:   *descResp.CreateAccountStatus.AccountId,
			Name: *descResp.CreateAccountStatus.AccountName,
		}, nil
	}
}
