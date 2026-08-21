package aws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"
)

func (p *Provider) Credentials(ctx context.Context, id string, lifetime time.Duration) (string, error) {
	config, err := p.assume(ctx, id, p.sandboxRole, lifetime)
	if err != nil {
		return "", err
	}
	credentials, err := config.Credentials.Retrieve(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve credentials: %w", err)
	}
	session, _ := json.Marshal(map[string]string{
		"sessionId":    credentials.AccessKeyID,
		"sessionKey":   credentials.SecretAccessKey,
		"sessionToken": credentials.SessionToken,
	})

	// this weird endpoint generates an AWS console sign in token:
	// https://docs.aws.amazon.com/IAM/latest/UserGuide/id_roles_providers_enable-console-custom-url.html
	client := http.Client{Timeout: time.Second * 10}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://signin.aws.amazon.com/federation?Action=getSigninToken&Session="+url.QueryEscape(string(session)), nil,
	)
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var result struct{ SigninToken string }
	json.NewDecoder(resp.Body).Decode(&result)

	rawCredentials, _ := json.Marshal(map[string]string{
		"AWS_ACCESS_KEY_ID":     credentials.AccessKeyID,
		"AWS_SECRET_ACCESS_KEY": credentials.SecretAccessKey,
		"AWS_SESSION_TOKEN":     credentials.SessionToken,
		"URL": fmt.Sprintf(
			"https://signin.aws.amazon.com/federation?Action=login&Destination=%s&SigninToken=%s",
			url.QueryEscape("https://console.aws.amazon.com/"), result.SigninToken,
		),
	})
	return string(rawCredentials), nil
}
