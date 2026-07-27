package aws

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"

	"codeberg.org/megakuul/cloudjam/internal/sandbox"
)

// Pool state lives directly on the organization account objects as tags.
// This keeps the pool observable in the aws console / cli without any
// additional storage. Transitions are serialized with the repository mutex;
// tags provide no compare-and-swap, so concurrent repository instances on the
// same organization must be avoided.
const (
	tagState   = "cloudjam:state"
	tagOwner   = "cloudjam:owner"
	tagUpdated = "cloudjam:updated"
)

func (r *Repository) readTags(ctx context.Context, id string) (map[string]string, error) {
	tags := map[string]string{}
	input := &organizations.ListTagsForResourceInput{ResourceId: &id}
	for {
		page, err := r.organizations.ListTagsForResource(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("failed to list tags of account %q: %w", id, err)
		}
		for _, tag := range page.Tags {
			tags[*tag.Key] = *tag.Value
		}
		if page.NextToken == nil {
			return tags, nil
		}
		input.NextToken = page.NextToken
	}
}

func (r *Repository) writeState(ctx context.Context, id string, state sandbox.State, owner string) error {
	updated := time.Now().UTC().Format(time.RFC3339)
	_, err := r.organizations.TagResource(ctx, &organizations.TagResourceInput{
		ResourceId: &id,
		Tags: []orgtypes.Tag{
			{Key: new(tagState), Value: new(string(state))},
			{Key: new(tagOwner), Value: &owner},
			{Key: new(tagUpdated), Value: &updated},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to tag account %q with state %q: %w", id, state, err)
	}
	return nil
}

func (r *Repository) clearState(ctx context.Context, id string) error {
	_, err := r.organizations.UntagResource(ctx, &organizations.UntagResourceInput{
		ResourceId: &id,
		TagKeys:    []string{tagState, tagOwner, tagUpdated},
	})
	if err != nil {
		return fmt.Errorf("failed to untag account %q: %w", id, err)
	}
	return nil
}

func (r *Repository) readAccount(ctx context.Context, id string) (*sandbox.Account, error) {
	descResp, err := r.organizations.DescribeAccount(ctx, &organizations.DescribeAccountInput{
		AccountId: &id,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe account %q: %w", id, err)
	}
	tags, err := r.readTags(ctx, id)
	if err != nil {
		return nil, err
	}
	if tags[tagState] == "" {
		return nil, fmt.Errorf("account %q is not part of the sandbox pool", id)
	}
	updated, _ := time.Parse(time.RFC3339, tags[tagUpdated])
	return &sandbox.Account{
		ID:    *descResp.Account.Id,
		Name:  *descResp.Account.Name,
		State: sandbox.State(tags[tagState]),
	}, nil
}

func (r *Repository) listAccounts(ctx context.Context) ([]*sandbox.Account, error) {
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
	return accounts, nil
}
