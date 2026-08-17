package aws

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/ekristen/aws-nuke/v3/pkg/awsutil"
	awsnuke "github.com/ekristen/aws-nuke/v3/pkg/nuke"
	"github.com/ekristen/libnuke/pkg/filter"
	libnuke "github.com/ekristen/libnuke/pkg/nuke"
	"github.com/ekristen/libnuke/pkg/registry"
	"github.com/ekristen/libnuke/pkg/scanner"
	"github.com/ekristen/libnuke/pkg/settings"
	"github.com/ekristen/libnuke/pkg/types"
	"github.com/sirupsen/logrus"

	_ "github.com/ekristen/aws-nuke/v3/resources"
)

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
		return err
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

	logger := logrus.New()
	logger.SetOutput(logWriter{logger: p.logger})
	logger.SetFormatter(&logrus.TextFormatter{DisableTimestamp: true, DisableColors: true})

	runner := libnuke.New(&libnuke.Parameters{
		NoDryRun:   true,
		Force:      true,
		ForceSleep: 3,
		Quiet:      true,
	}, filters, &settings.Settings{})
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
		return fmt.Errorf("nuke account %q: %w", id, err)
	}
	p.logger.Info(fmt.Sprintf("nuked account '%s'", id))
	return nil
}

// pipeline to transfer logrus to slog.
type logWriter struct {
	logger *slog.Logger
}

func (w logWriter) Write(line []byte) (int, error) {
	w.logger.Info(strings.TrimRight(string(line), "\n"))
	return len(line), nil
}
