package aws

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/organizations"
)

func (p *Provider) Delete(ctx context.Context, id string) error {
	if err := p.Nuke(ctx, id); err != nil {
		return fmt.Errorf("failed to nuke account resources: %w", err)
	}
	_, err := p.organizations.CloseAccount(ctx, &organizations.CloseAccountInput{
		AccountId: &id,
	})
	return err
}
