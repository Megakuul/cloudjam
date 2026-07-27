package aws

import (
	"context"
	"fmt"
	"time"

	"codeberg.org/megakuul/cloudjam/internal/sandbox"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
)

func (r *Provider) Provision(ctx context.Context, id string) (*sandbox.Account, error) {
	if r.blocked(id) {
		return nil, fmt.Errorf("account %q is the management account or blocklisted", id)
	}
	createResp, err := r.organizations.CreateAccount(ctx, &organizations.CreateAccountInput{
		AccountName:            new(id),
		Email:                  new(id + r.emailSuffix),
		IamUserAccessToBilling: orgtypes.IAMUserAccessToBillingDeny,
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
		return &sandbox.Account{
			ID:   *descResp.CreateAccountStatus.AccountId,
			Name: *descResp.CreateAccountStatus.AccountName,
		}, nil
	}

	// TODO also create roles
}
