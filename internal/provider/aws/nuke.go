package aws

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/ekristen/aws-nuke/v3/pkg/awsutil"
	awsnuke "github.com/ekristen/aws-nuke/v3/pkg/nuke"
	"github.com/ekristen/libnuke/pkg/filter"
	libnuke "github.com/ekristen/libnuke/pkg/nuke"
	"github.com/ekristen/libnuke/pkg/queue"
	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/scanner"
	"github.com/ekristen/libnuke/pkg/settings"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/sirupsen/logrus"

	awsresources "github.com/ekristen/aws-nuke/v3/resources"
)

// nukeSettings opts every deletion guard that aws-nuke knows how to lift.
//
// Each of these is off by default, because upstream is a tool a human points at
// an account they may not have meant to empty. Here the account is a sandbox
// that exists to be recycled, and a single protected table or load balancer
// left behind strands the whole eviction: the resource never becomes deletable,
// so libnuke retries it three times and gives up on the entire run.
//
// IncludeServiceLinkedRoles is deliberately left off. Service linked roles are
// deleted asynchronously by the owning service and frequently refuse outright,
// which would add permanently stuck items rather than remove them.
func nukeSettings() *settings.Settings {
	s := &settings.Settings{}
	for _, resource := range []string{
		awsresources.CloudFormationStackResource,
		awsresources.CloudWatchLogsLogGroupResource,
		awsresources.CognitoUserPoolResource,
		awsresources.DSQLClusterResource,
		awsresources.DocDBClusterResource,
		awsresources.DynamoDBTableResource,
		awsresources.EC2InstanceResource,
		awsresources.EKSClusterResource,
		awsresources.ELBv2Resource,
		awsresources.NeptuneClusterResource,
		awsresources.NeptuneGraphResource,
		awsresources.NeptuneInstanceResource,
		awsresources.PinpointPhoneNumberResource,
		awsresources.QLDBLedgerResource,
		awsresources.RDSInstanceResource,
	} {
		s.Set(resource, &settings.Setting{"DisableDeletionProtection": true})
	}
	// Set merges into an existing entry, so these extend the deletion protection
	// setting above rather than replacing it.
	s.Set(awsresources.EC2InstanceResource, &settings.Setting{"DisableStopProtection": true})
	s.Set(awsresources.NeptuneInstanceResource, &settings.Setting{"DisableClusterDeletionProtection": true})
	s.Set(awsresources.QuickSightSubscriptionResource, &settings.Setting{"DisableTerminationProtection": true})
	s.Set(awsresources.EC2ImageResource, &settings.Setting{
		awsresources.DisableDeregistrationProtectionSetting: true,
		awsresources.IncludeDeprecatedSetting:               true,
		awsresources.IncludeDisabledSetting:                 true,
	})
	// the guard scp denies object lock and legal holds, but an account that
	// predates it, or a gap in it, must not be able to pin a bucket forever.
	s.Set(awsresources.S3BucketResource, &settings.Setting{
		"BypassGovernanceRetention": true,
		"RemoveObjectLegalHold":     true,
	})
	return s
}

func (p *Provider) Nuke(ctx context.Context, id string) (err error) {
	// aws-nuke MutateOpts panics instead of returning when a region cannot
	// serve a resource type, and this runs inside a long lived server.
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("nuke account %q panicked: %v", id, recovered)
		}
	}()

	if p.managementAccount == id {
		return fmt.Errorf("account %q is the management account", id)
	}

	config, err := p.assume(ctx, id, p.adminRole, 10*time.Hour)
	if err != nil {
		return fmt.Errorf("assuming cleanup role: %w", err)
	}
	// aws-nuke builds its own sessions per region and service, so it needs the
	// assumed credentials as plain values. Handing it anything else means it
	// falls back to the ambient credential chain and nukes the wrong account.
	assumed, err := config.Credentials.Retrieve(ctx)
	if err != nil {
		return fmt.Errorf("retrieve assumed credentials: %w", err)
	}

	account, err := awsutil.NewAccount(&awsutil.Credentials{
		AccessKeyID:     assumed.AccessKeyID,
		SecretAccessKey: assumed.SecretAccessKey,
		SessionToken:    assumed.SessionToken,
	}, nil)
	if err != nil {
		return fmt.Errorf("resolve account from assumed credentials: %w", err)
	}
	if account.ID() != id {
		return fmt.Errorf("assumed credentials belong to account %q, not %q", account.ID(), id)
	}

	filters := filter.Filters{
		"IAMRole": {
			{Type: filter.Exact, Property: "Name", Value: p.adminRole},
		},
		"IAMRolePolicy": {
			{Type: filter.Exact, Property: "role:RoleName", Value: p.adminRole},
		},
		"IAMRolePolicyAttachment": {
			{Type: filter.Exact, Property: "RoleName", Value: p.adminRole},
		},
	}

	// disable the leaking logrus global logs
	logrus.SetOutput(io.Discard)

	logger := logrus.New()
	logger.SetLevel(logrus.WarnLevel)
	logger.SetOutput(logWriter{logger: p.logger})
	logger.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, DisableColors: true})

	runner := libnuke.New(&libnuke.Parameters{
		NoDryRun:   true,
		Force:      true,
		ForceSleep: 3,
		Quiet:      true,
	}, filters, nukeSettings())
	runner.SetLogger(logrus.NewEntry(logger))

	resourceTypes := types.ResolveResourceTypes(
		registry.GetNames(), nil, nil, nil,
		registry.GetAlternativeResourceTypeMapping(),
	)

	// "global" is where iam, route53 and cloudfront live; without it the account
	// keeps everything that is not regional.
	regions := append([]string{awsutil.GlobalRegionID}, p.regions...)
	for _, name := range regions {
		region := awsnuke.NewRegion(name, account.ResourceTypeToServiceType, account.NewSession, account.NewConfig)

		regionScanner, err := scanner.New(&scanner.Config{
			Owner:         name,
			ResourceTypes: resourceTypes,
			Opts: &awsnuke.ListerOpts{
				Region:    region,
				AccountID: &id,
				Logger:    logrus.NewEntry(logger).WithField("region", name),
			},
			Logger: logger,
		})
		if err != nil {
			return fmt.Errorf("create scanner for %s: %w", name, err)
		}
		if err := regionScanner.RegisterMutateOptsFunc(awsnuke.MutateOpts); err != nil {
			return fmt.Errorf("register mutate func for %s: %w", name, err)
		}
		if err := runner.RegisterScanner(awsnuke.Account, regionScanner); err != nil {
			return fmt.Errorf("register scanner for %s: %w", name, err)
		}
	}

	p.logger.Info(fmt.Sprintf("nuking account '%s' in regions %s...", id, strings.Join(regions, ", ")))
	if err := runner.Run(ctx); err != nil {
		// libnuke gives up with a bare "failed" once resources are stuck and nothing
		// is left in flight that could unblock them. The aws errors that caused it
		// only live on the queue items, because upstream aws-nuke is a cli that
		// prints them and exits. Pull them back out so the error is actionable.
		if stuck := stuckReasons(runner.Queue); len(stuck) > 0 {
			return fmt.Errorf("nuke account %q: %d resources stuck: %s",
				id, len(stuck), strings.Join(truncate(stuck, maxStuckReasons), "; "))
		}
		return fmt.Errorf("nuke account %q: %w", id, err)
	}
	p.logger.Info(fmt.Sprintf("nuked account '%s'", id))
	return nil
}

// maxStuckReasons caps how many resource reasons end up in the returned error,
// since it is persisted on the account record and a broad failure can strand
// hundreds of resources.
const maxStuckReasons = 10

// stuckReasons collects the resources libnuke left in the failed state together
// with the aws error that blocked their deletion.
func stuckReasons(q *queue.Queue) []string {
	reasons := []string{}
	for _, item := range q.GetItems() {
		if item.GetState() != queue.ItemStateFailed {
			continue
		}
		// resources that do not expose a legacy id are still worth reporting,
		// the type and region alone narrow it down enough to look it up.
		name, err := item.GetProperty("")
		if err != nil {
			name = "<unnamed>"
		}
		reasons = append(reasons, fmt.Sprintf("%s %s %q: %s", item.Owner, item.Type, name, item.GetReason()))
	}
	return reasons
}

func truncate(reasons []string, limit int) []string {
	if len(reasons) <= limit {
		return reasons
	}
	return append(reasons[:limit:limit], fmt.Sprintf("and %d more", len(reasons)-limit))
}

// pipeline to transfer logrus to slog.
type logWriter struct {
	logger *slog.Logger
}

func (w logWriter) Write(line []byte) (int, error) {
	w.logger.Warn(strings.TrimRight(string(line), "\n"))
	return len(line), nil
}
