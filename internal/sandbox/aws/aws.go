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
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/costexplorer"
	"github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"
	"github.com/aws/aws-sdk-go-v2/service/sts"

	"codeberg.org/megakuul/cloudjam/internal/sandbox"
)

// NukeTool selects the external cleanup tool.
type NukeTool string

const (
	// NukeToolAWSNuke uses github.com/ekristen/aws-nuke (v3).
	NukeToolAWSNuke NukeTool = "aws-nuke"
	// NukeToolCloudNuke uses github.com/gruntwork-io/cloud-nuke.
	NukeToolCloudNuke NukeTool = "cloud-nuke"
)

// Config configures the aws sandbox repository.
type Config struct {
	// Client is the sdk configuration of the organization management (payer) account.
	Client awssdk.Config
	// OrganizationRole is the pre-provisioned admin role assumable from the
	// management account (created automatically by aws for created/invited
	// accounts). Defaults to "OrganizationAccountAccessRole".
	OrganizationRole string
	// SandboxRole is the administrator role created during bootstrap; it is
	// used for deployments, competitor access and cleanup and is protected
	// against tampering by the guard scp. Defaults to "cloudjam-sandbox".
	SandboxRole string
	// SessionDuration is the lifetime of issued credentials. Defaults to 1h
	// (also the aws hard limit when the management client itself runs on an
	// assumed role, due to role chaining).
	SessionDuration time.Duration
	// Regions competitors may use; every other region is denied by the
	// boundary scp and only these regions are scanned by the leak checks.
	// Defaults to ["us-east-1", "eu-central-1"].
	Regions []string
	// InstanceTypes contains glob patterns of ec2 instance types competitors
	// may launch; everything else is denied by the boundary scp. Defaults to
	// ["t2.*", "t3.*", "t3a.*", "t4g.*", "*.micro", "*.small", "*.medium", "*.large"].
	InstanceTypes []string
	// VolumeSize is the maximum ebs volume size in GiB allowed by the
	// boundary scp. Defaults to 500.
	VolumeSize int
	// Nuke selects the cleanup tool. Defaults to NukeToolAWSNuke.
	Nuke NukeTool
	// NukeBinary overrides the cleanup tool binary path (defaults to the
	// tool name resolved via $PATH).
	NukeBinary string
	// WorkDir stores the generated nuke configurations and full nuke logs
	// per account for auditability. Defaults to "cloudjam-sandbox" inside
	// the os temp directory.
	WorkDir string
	// DailyBudget is the spend in USD per day above which an account is
	// flagged as leaking. Defaults to 10.
	DailyBudget float64
	// BillingWindow is the trailing window in days covered by Bill.
	// Defaults to 14.
	BillingWindow int
	// Blocklist contains additional account ids that must never be acquired
	// or cleaned (the management account is always blocked).
	Blocklist []string
	// Logger receives progress information. Defaults to slog.Default.
	Logger *slog.Logger
}

// Repository implements sandbox.Repository for aws organization accounts.
type Repository struct {
	config        Config
	management    string
	organizations *organizations.Client
	costexplorer  *costexplorer.Client
	sts           *sts.Client
	mutex         sync.Mutex
}

var _ sandbox.Repository = (*Repository)(nil)

// New creates the repository and resolves the organization management account.
func New(ctx context.Context, config Config) (*Repository, error) {
	if config.OrganizationRole == "" {
		config.OrganizationRole = "OrganizationAccountAccessRole"
	}
	if config.SandboxRole == "" {
		config.SandboxRole = "cloudjam-sandbox"
	}
	if config.SessionDuration == 0 {
		config.SessionDuration = time.Hour
	}
	if len(config.Regions) == 0 {
		config.Regions = []string{"us-east-1", "eu-central-1"}
	}
	if len(config.InstanceTypes) == 0 {
		config.InstanceTypes = []string{"t2.*", "t3.*", "t3a.*", "t4g.*", "*.micro", "*.small", "*.medium", "*.large"}
	}
	if config.VolumeSize == 0 {
		config.VolumeSize = 500
	}
	if config.Nuke == "" {
		config.Nuke = NukeToolAWSNuke
	}
	if config.NukeBinary == "" {
		config.NukeBinary = string(config.Nuke)
	}
	if config.WorkDir == "" {
		config.WorkDir = filepath.Join(os.TempDir(), "cloudjam-sandbox")
	}
	if config.DailyBudget == 0 {
		config.DailyBudget = 10
	}
	if config.BillingWindow == 0 {
		config.BillingWindow = 14
	}
	if config.Logger == nil {
		config.Logger = slog.Default()
	}
	if config.Client.Region == "" {
		config.Client.Region = "us-east-1"
	}

	repository := &Repository{
		config: config,
		// organizations and cost explorer are global services served from us-east-1.
		organizations: organizations.NewFromConfig(config.Client, func(o *organizations.Options) {
			o.Region = "us-east-1"
		}),
		costexplorer: costexplorer.NewFromConfig(config.Client, func(o *costexplorer.Options) {
			o.Region = "us-east-1"
		}),
		sts: sts.NewFromConfig(config.Client),
	}

	organization, err := repository.organizations.DescribeOrganization(ctx, &organizations.DescribeOrganizationInput{})
	if err != nil {
		return nil, fmt.Errorf("failed to describe organization: %w", err)
	}
	repository.management = *organization.Organization.MasterAccountId

	return repository, nil
}

func (r *Repository) blocked(id string) bool {
	return id == r.management || slices.Contains(r.config.Blocklist, id)
}

func (r *Repository) Add(ctx context.Context, id string) (*sandbox.Account, error) {
	if r.blocked(id) {
		return nil, fmt.Errorf("account %q is the management account or blocklisted", id)
	}
	account, err := r.organizations.DescribeAccount(ctx, &organizations.DescribeAccountInput{
		AccountId: &id,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to describe account (is it part of the organization?): %w", err)
	}
	if account.Account.Status != orgtypes.AccountStatusActive {
		return nil, fmt.Errorf("account %q is not active (status %q)", id, account.Account.Status)
	}

	if err := r.writeState(ctx, id, sandbox.StateBootstrapping, ""); err != nil {
		return nil, err
	}
	r.config.Logger.Info("bootstrapping sandbox account", "account", id)
	if err := r.bootstrap(ctx, id); err != nil {
		_ = r.writeState(ctx, id, sandbox.StateDirty, "")
		return nil, fmt.Errorf("failed to bootstrap account %q: %w", id, err)
	}
	if err := r.writeState(ctx, id, sandbox.StateReady, ""); err != nil {
		return nil, err
	}
	r.config.Logger.Info("sandbox account ready", "account", id)
	return r.Get(ctx, id)
}

func (r *Repository) Remove(ctx context.Context, id string) error {
	account, err := r.Get(ctx, id)
	if err != nil {
		return err
	}
	if account.State == sandbox.StateUsed {
		return fmt.Errorf("account %q is currently used by %q; release it first", id, account.Owner)
	}
	if err := r.detachPolicies(ctx, id); err != nil {
		return err
	}
	if err := r.clearState(ctx, id); err != nil {
		return err
	}
	r.config.Logger.Info("removed sandbox account from pool", "account", id)
	return nil
}

func (r *Repository) Get(ctx context.Context, id string) (*sandbox.Account, error) {
	return r.readAccount(ctx, id)
}

func (r *Repository) List(ctx context.Context) ([]*sandbox.Account, error) {
	return r.listAccounts(ctx)
}

func (r *Repository) Acquire(ctx context.Context, owner string) (*sandbox.Account, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()

	accounts, err := r.listAccounts(ctx)
	if err != nil {
		return nil, err
	}
	for _, account := range accounts {
		if account.State != sandbox.StateReady {
			continue
		}
		if err := r.writeState(ctx, account.ID, sandbox.StateUsed, owner); err != nil {
			return nil, err
		}
		account.State = sandbox.StateUsed
		account.Owner = owner
		r.config.Logger.Info("acquired sandbox account", "account", account.ID, "owner", owner)
		return account, nil
	}
	return nil, fmt.Errorf("no ready account in the pool")
}

func (r *Repository) Release(ctx context.Context, id string) error {
	r.mutex.Lock()
	account, err := r.readAccount(ctx, id)
	if err != nil {
		r.mutex.Unlock()
		return err
	}
	if account.State == sandbox.StateBootstrapping {
		r.mutex.Unlock()
		return fmt.Errorf("account %q is still bootstrapping", id)
	}
	if err := r.writeState(ctx, id, sandbox.StateCleaning, ""); err != nil {
		r.mutex.Unlock()
		return err
	}
	r.mutex.Unlock()

	r.config.Logger.Info("cleaning sandbox account", "account", id, "tool", r.config.Nuke)
	log, err := r.clean(ctx, id)
	if err != nil {
		_ = r.writeState(ctx, id, sandbox.StateDirty, "")
		return fmt.Errorf("failed to clean account %q (full log at %q): %w", id, log, err)
	}

	// verify with the leak heuristics that the account really is empty;
	// anything above info level keeps it out of the ready pool.
	findings, err := r.scan(ctx, id)
	if err != nil {
		_ = r.writeState(ctx, id, sandbox.StateDirty, "")
		return fmt.Errorf("failed to verify cleanup of account %q: %w", id, err)
	}
	var leftovers []string
	for _, finding := range findings {
		if finding.Severity != sandbox.SeverityInfo {
			leftovers = append(leftovers, fmt.Sprintf("%s/%s: %s", finding.Region, finding.Resource, finding.Message))
		}
	}
	if len(leftovers) > 0 {
		_ = r.writeState(ctx, id, sandbox.StateDirty, "")
		return fmt.Errorf("cleanup of account %q left resources behind (full log at %q): %v", id, log, leftovers)
	}

	if err := r.writeState(ctx, id, sandbox.StateReady, ""); err != nil {
		return err
	}
	r.config.Logger.Info("sandbox account cleaned and ready", "account", id, "log", log)
	return nil
}

func (r *Repository) Credentials(ctx context.Context, id string) (*sandbox.Credentials, error) {
	if _, err := r.readAccount(ctx, id); err != nil {
		return nil, err
	}
	config, expires, err := r.assume(ctx, id, r.config.SandboxRole)
	if err != nil {
		return nil, err
	}
	credentials, err := config.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve credentials: %w", err)
	}
	return &sandbox.Credentials{
		AccessKey: credentials.AccessKeyID,
		SecretKey: credentials.SecretAccessKey,
		Token:     credentials.SessionToken,
		Expires:   expires,
	}, nil
}

func (r *Repository) Check(ctx context.Context, id string) (*sandbox.Report, error) {
	if _, err := r.readAccount(ctx, id); err != nil {
		return nil, err
	}

	report := &sandbox.Report{
		Account: id,
		Checked: time.Now().UTC(),
	}

	// heuristic 1: are the guardrail policies still in place and unmodified?
	report.Findings = append(report.Findings, r.verifyPolicies(ctx, id)...)

	// heuristic 2: does the (delayed) billing data already show a leak?
	report.Findings = append(report.Findings, r.scanBilling(ctx, id)...)

	// heuristic 3: are known money burners running right now?
	findings, err := r.scan(ctx, id)
	if err != nil {
		return nil, err
	}
	report.Findings = append(report.Findings, findings...)

	report.Healthy = true
	for _, finding := range report.Findings {
		if finding.Severity == sandbox.SeverityCritical {
			report.Healthy = false
			break
		}
	}
	return report, nil
}
