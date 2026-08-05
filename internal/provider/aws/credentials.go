package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func (p *Provider) Credentials(ctx context.Context, id string, lifetime time.Duration) (string, error) {
	config, err := p.assume(ctx, id, p.adminRole, lifetime)
	if err != nil {
		return "", err
	}
	credentials, err := config.Credentials.Retrieve(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve credentials: %w", err)
	}
	rawCredentials, _ := json.Marshal(map[string]string{
		"AWS_ACCESS_KEY_ID":     credentials.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": credentials.SecretAccessKey,
		"AWS_SESSION_TOKEN":     credentials.SessionToken,
	})
	return string(rawCredentials), nil
}
