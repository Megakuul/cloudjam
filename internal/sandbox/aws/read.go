package aws

import (
	"context"
	"fmt"

	"codeberg.org/megakuul/cloudjam/internal/sandbox"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
)

func (r *Repository) Get(ctx context.Context, id string) (*sandbox.Account, error) {
	descResp, err := r.organizations.DescribeAccount(ctx, &organizations.DescribeAccountInput{
		AccountId: &id,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe account %q: %w", id, err)
	}
	return &sandbox.Account{
		ID:   *descResp.Account.Id,
		Name: *descResp.Account.Name,
	}, nil
}

func (r *Repository) List(ctx context.Context) ([]*sandbox.Account, error) {
	var accounts []*sandbox.Account
	paginator := organizations.NewListAccountsPaginator(r.organizations, &organizations.ListAccountsInput{})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list organization accounts: %w", err)
		}
		for _, entry := range page.Accounts {
			if r.blocked(*entry.Id) {
				continue
			}
			account, err := r.readAccount(ctx, *entry.Id)
			if err != nil {
				continue // accounts without pool tags are not managed by the sandbox.
			}
			accounts = append(accounts, account)
		}
	}
	return accounts
}
