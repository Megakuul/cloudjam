// aws implements the sandbox repository on top of AWS Organizations member accounts.
//
// Pool state is stored as tags on the organization account objects themselves
// (cloudjam:state, cloudjam:owner, cloudjam:updated), so no external database
// is required and the pool is fully visible in the aws console at all times.
//
// Accounts must be invited into the organization manually before Add is called.
// Add then bootstraps the account: it creates an iam account alias, an
// administrator role for deployments and cleanup, and attaches two service
// control policies that block known instant money burners (see scp.go for the
// researched deny list). Release wipes the account with aws-nuke or cloud-nuke
// (fully non-interactive) and verifies afterwards that nothing expensive
// survived before the account is put back into the ready pool.
package aws

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"slices"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"codeberg.org/megakuul/cloudjam/internal/sandbox"
)

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

// Provider implements sandbox.Provider for aws organization accounts.
type Provider struct {
	logger slog.Logger

	regions       []string
	emailSuffix   string
	sandboxRole   string
	adminRole     string
	instanceTypes []string

	maxVolumeSize int
	maxDailyCost  float64
	// blockedAccounts contains additional account ids that must never be acquired
	// or cleaned (the management account is always blocked).
	blockedAccounts   []string
	managementAccount string
	createConfig      func(credentials.StaticCredentialsProvider) awssdk.Config
	rootOU            string
	cloudjamOU        string

	organizations *organizations.Client
	costexplorer  *costexplorer.Client
	sts           *sts.Client

	mutex sync.Mutex
}

var _ sandbox.Provider = (*Provider)(nil)

// New creates the repository and resolves the organization management account.
func New(ctx context.Context, config awssdk.Config) (*Provider, error) {
	repository := &Provider{
		adminRole:     "cloudjam-admin",
		sandboxRole:   "cloudjam-sandbox",
		regions:       []string{"us-east-1", "eu-central-1"},
		instanceTypes: []string{"t2.*", "t3.*", "t3a.*", "t4g.*", "*.micro", "*.small"},
		maxVolumeSize: 10,
		maxDailyCost:  10,

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

	organization, err := repository.organizations.DescribeOrganization(ctx, &organizations.DescribeOrganizationInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to describe organization: %w", err)
	}
	repository.managementAccount = *organization.Organization.MasterAccountId

	return repository, nil
}

func (r *Provider) bootstrap(ctx context.Context) error {
	roots, err := r.organizations.ListRoots(ctx, &organizations.ListRootsInput{
		MaxResults: new(int32(1)),
	})
	if err != nil {
		return err
	} else if len(roots.Roots) < 1 || roots.Roots[0].Id == nil {
		// never happens practically (there is always a root ou on organization management accounts).
		return fmt.Errorf("fuck the aws api design")
	}
	r.rootOU = *roots.Roots[0].Id

	_, err = r.organizations.CreateOrganizationalUnit(ctx, &organizations.CreateOrganizationalUnitInput{
		Name:     new("cloudjam"), // hardcoded unchangable API
		ParentId: &r.rootOU,
	})
	if err != nil {
		if _, ok := errors.AsType[*orgtypes.DuplicateOrganizationalUnitException](err); !ok {
			return err
		}
	}

	policiesResp, err := r.organizations.ListPoliciesForTarget(ctx, &organizations.ListPoliciesForTargetInput{
		TargetId: &r.cloudjamOU,
		Filter:   orgtypes.PolicyTypeServiceControlPolicy,
	})
	if err != nil {
		return err
	}

	guardPolicy := ""
	for _, policy := range policiesResp.Policies {
		if policy.Name != nil && *policy.Name == "cloudjam-guard" {
			guardPolicy = *policy.Id
		}
	}

	if guardPolicy == "" {
		_, err = r.organizations.CreatePolicy(ctx, &organizations.CreatePolicyInput{
			Name:    new("cloudjam-guard"), // hardcoded unchangable API
			Type:    orgtypes.PolicyTypeServiceControlPolicy,
			Content: new(""),
		})
		if err != nil {
			return fmt.Errorf("creating cloudjam guard policy: %w", err)
		}
	} else {
		_, err = r.organizations.UpdatePolicy(ctx, &organizations.UpdatePolicyInput{
			PolicyId: &guardPolicy,
			Content:  new(""),
		})
		if err != nil {
			return fmt.Errorf("updating cloudjam guard policy: %w", err)
		}
	}

	return nil
}

func (r *Provider) blocked(id string) bool {
	return id == r.managementAccount || slices.Contains(r.blockedAccounts, id)
}

// assume returns an sdk configuration with short-lived credentials for the
// specified role inside a member account, along with the session expiry.
func (r *Provider) assume(ctx context.Context, id string, role string, sessionDuration time.Duration) (awssdk.Config, error) {
	session, err := r.sts.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         new(fmt.Sprintf("arn:aws:iam::%s:role/%s", id, role)),
		RoleSessionName: new("cloudjam"),
		DurationSeconds: new(int32(sessionDuration.Seconds())),
	})
	if err != nil {
		return awssdk.Config{}, fmt.Errorf("failed to assume role %q in account %q: %w", role, id, err)
	}
	return r.createConfig(credentials.NewStaticCredentialsProvider(
		*session.Credentials.AccessKeyId,
		*session.Credentials.SecretAccessKey,
		*session.Credentials.SessionToken,
	)), nil
}

func (r *Provider) Get(ctx context.Context, id string) (*sandbox.Account, error) {
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

func (r *Provider) List(ctx context.Context) ([]*sandbox.Account, error) {
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
			accounts = append(accounts, &sandbox.Account{
				ID:   *entry.Id,
				Name: *entry.Name,
			})
		}
	}
	return accounts, nil
}
