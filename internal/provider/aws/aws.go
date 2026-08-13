package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"codeberg.org/megakuul/cloudjam/internal/provider"
)

// Credentials provides the serializable format in which this provider accepts credentials.
type Credentials struct {
	Endpoint  *string `json:"endpoint"`
	Region    string  `json:"region"`
	AccessKey string  `json:"access_key"`
	SecretKey string  `json:"secret_key"`
}

const sandboxInlinePolicy = "cloudjam"

// maxVolumeSize defines the hardcoded maximum storage size (ebs).
const maxVolumeSize = 50 // 50 gb

// allowedInstanceTypes defines a hardcoded list of allowed ec2 instance types.
var allowedInstanceTypes = []string{
	"t2.*", "t3.*", "t3a.*", "t4g.*", "*.micro", "*.small", "*.medium",
}

// isAllowedInstance tells you if the specified instanceType is allowed according to allowedInstanceTypes.
func isAllowedInstance(instanceType string) bool {
	for _, pattern := range allowedInstanceTypes {
		if ok, _ := path.Match(pattern, instanceType); ok {
			return true
		}
	}
	return false
}

var _ provider.Provider = &Provider{}

// Provider implements sandbox.Provider for aws organization accounts.
type Provider struct {
	logger *slog.Logger

	regions        []string
	emailSuffix    string
	sandboxRole    string
	adminRole      string
	boundaryPolicy string
	createConfig   func(credentials.StaticCredentialsProvider) awssdk.Config

	// data is loaded on bootstrap from AWS
	managementAccount string
	rootOU            string
	cloudjamOU        string
	// data is loaded on preparation
	assetBucket string

	organizations *organizations.Client
	costexplorer  *costexplorer.Client
	sts           *sts.Client
}

var _ provider.Provider = (*Provider)(nil)

type ProviderOption func(*Provider)

// New creates the repository and resolves the organization management account.
func New(ctx context.Context, rawCreds string, opts ...ProviderOption) (*Provider, error) {
	creds := &Credentials{}
	if err := json.Unmarshal([]byte(rawCreds), creds); err != nil {
		return nil, fmt.Errorf("invalid provider credentials: %w", err)
	}
	config := awssdk.Config{
		Region:       creds.Region,
		Credentials:  credentials.NewStaticCredentialsProvider(creds.AccessKey, creds.SecretKey, ""),
		BaseEndpoint: creds.Endpoint,
	}
	provider := &Provider{
		logger:         slog.Default(),
		regions:        []string{"us-east-1", "eu-central-1"},
		emailSuffix:    "+cloudjam@example.com",
		sandboxRole:    "cloudjam-sandbox",
		adminRole:      "cloudjam-admin",
		boundaryPolicy: "cloudjam-boundary",
		createConfig: func(provider credentials.StaticCredentialsProvider) awssdk.Config {
			newConfig := config.Copy()
			newConfig.Credentials = provider
			return newConfig
		},
		organizations: organizations.NewFromConfig(config, func(o *organizations.Options) {
			o.Region = "us-east-1"
		}),
		costexplorer: costexplorer.NewFromConfig(config, func(o *costexplorer.Options) {
			o.Region = "us-east-1"
		}),
		sts: sts.NewFromConfig(config),
	}
	for _, opt := range opts {
		opt(provider)
	}

	organization, err := provider.organizations.DescribeOrganization(ctx, &organizations.DescribeOrganizationInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to describe organization: %w", err)
	}
	provider.managementAccount = *organization.Organization.MasterAccountId

	return provider, provider.bootstrap(ctx)
}

func WithLogger(logger *slog.Logger) ProviderOption {
	return func(p *Provider) { p.logger = logger }
}

func WithRegions(regions ...string) ProviderOption {
	return func(p *Provider) { p.regions = regions }
}

// WithEmailSuffix defines the emailsuffix used to create accounts
// (the provider will generate dynamic uuids as prefix for each account).
func WithEmailSuffix(suffix string) ProviderOption {
	return func(p *Provider) { p.emailSuffix = suffix }
}

// WithAdminRole defines the name of the IAM admin role created in accounts.
func WithAdminRole(adminRole string) ProviderOption {
	return func(p *Provider) { p.adminRole = adminRole }
}

// WithSandboxRole defines the name of the IAM sandbox role created in accounts.
func WithSandboxRole(sandboxRole string) ProviderOption {
	return func(p *Provider) { p.sandboxRole = sandboxRole }
}

// WithBoundaryPolicy defines the name of the IAM boundary policy that is required on all created IAM resources.
func WithBoundaryPolicy(boundaryPolicy string) ProviderOption {
	return func(p *Provider) { p.boundaryPolicy = boundaryPolicy }
}

func (p *Provider) bootstrap(ctx context.Context) error {
	roots, err := p.organizations.ListRoots(ctx, &organizations.ListRootsInput{
		MaxResults: new(int32(1)),
	})
	if err != nil {
		return err
	} else if len(roots.Roots) < 1 || roots.Roots[0].Id == nil {
		// never happens practically (there is always a root ou on organization management accounts).
		return fmt.Errorf("fuck the aws api design")
	}
	p.rootOU = *roots.Roots[0].Id

	ouListResp, err := p.organizations.ListOrganizationalUnitsForParent(ctx, &organizations.ListOrganizationalUnitsForParentInput{
		ParentId: &p.rootOU,
	})
	if err != nil {
		return fmt.Errorf("list account ous: %w", err)
	}

	for _, ou := range ouListResp.OrganizationalUnits {
		if ou.Name != nil && *ou.Name == "cloudjam" {
			p.cloudjamOU = *ou.Id
		}
	}
	if p.cloudjamOU == "" {
		ouResp, err := p.organizations.CreateOrganizationalUnit(ctx, &organizations.CreateOrganizationalUnitInput{
			Name:     new("cloudjam"), // hardcoded unchangable API
			ParentId: &p.rootOU,
		})
		if err != nil {
			return fmt.Errorf("creating cloudjam ou: %w", err)
		}
		p.cloudjamOU = *ouResp.OrganizationalUnit.Id
	}

	policiesResp, err := p.organizations.ListPoliciesForTarget(ctx, &organizations.ListPoliciesForTargetInput{
		TargetId: &p.cloudjamOU,
		Filter:   orgtypes.PolicyTypeServiceControlPolicy,
	})
	if err != nil {
		return fmt.Errorf("list account policies: %w", err)
	}

	guardPolicyID := ""
	for _, policy := range policiesResp.Policies {
		if policy.Name != nil && *policy.Name == "cloudjam-guard" {
			guardPolicyID = *policy.Id
		}
	}

	guardPolicyContent, err := guardControlPolicy(p.adminRole, p.sandboxRole, p.boundaryPolicy)
	if err != nil {
		return err
	}
	if guardPolicyID == "" {
		_, err = p.organizations.CreatePolicy(ctx, &organizations.CreatePolicyInput{
			Name:        new("cloudjam-guard"), // hardcoded unchangable API
			Description: new("read the fucking cloudjam manual"),
			Type:        orgtypes.PolicyTypeServiceControlPolicy,
			Content:     new(string(guardPolicyContent)),
		})
		if err != nil {
			return fmt.Errorf("creating cloudjam guard policy: %w", err)
		}
	} else {
		_, err = p.organizations.UpdatePolicy(ctx, &organizations.UpdatePolicyInput{
			PolicyId: &guardPolicyID,
			Content:  new(string(guardPolicyContent)),
		})
		if err != nil {
			return fmt.Errorf("updating cloudjam guard policy: %w", err)
		}
	}

	_, err = p.organizations.AttachPolicy(ctx, &organizations.AttachPolicyInput{
		PolicyId: &guardPolicyID,
		TargetId: &p.cloudjamOU,
	})
	if err != nil {
		if _, ok := errors.AsType[*orgtypes.DuplicatePolicyAttachmentException](err); !ok {
			return fmt.Errorf("attaching guard policy (%q) to cloudjam ou (%q): %w", guardPolicyID, p.cloudjamOU, err)
		}
	}

	return nil
}

// assume returns an sdk configuration with short-lived credentials for the
// specified role inside a member account, along with the session expiry.
func (p *Provider) assume(ctx context.Context, id string, role string, sessionDuration time.Duration) (awssdk.Config, error) {
	session, err := p.sts.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         new(fmt.Sprintf("arn:aws:iam::%s:role/%s", id, role)),
		RoleSessionName: new("cloudjam"),
		DurationSeconds: new(int32(sessionDuration.Seconds())),
	})
	if err != nil {
		return awssdk.Config{}, fmt.Errorf("failed to assume role %q in account %q: %w", role, id, err)
	}
	return p.createConfig(credentials.NewStaticCredentialsProvider(
		*session.Credentials.AccessKeyId,
		*session.Credentials.SecretAccessKey,
		*session.Credentials.SessionToken,
	)), nil
}
