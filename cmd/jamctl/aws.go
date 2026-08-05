package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/cloudcontrol"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	nukeaws "github.com/gruntwork-io/cloud-nuke/aws"
	nukeconfig "github.com/gruntwork-io/cloud-nuke/config"
	nukeutil "github.com/gruntwork-io/cloud-nuke/util"
)

// awsConfig loads credentials. An endpoint points everything at localstack,
// which ignores the credentials but still wants them present to sign with.
func awsConfig(ctx context.Context, profile, region, endpoint string) (awssdk.Config, error) {
	options := []func(*config.LoadOptions) error{}
	if profile != "" {
		options = append(options, config.WithSharedConfigProfile(profile))
	}
	if region != "" {
		options = append(options, config.WithRegion(region))
	}
	if endpoint != "" {
		options = append(options,
			config.WithBaseEndpoint(endpoint),
			config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
		)
	}
	cfg, err := config.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return cfg, fmt.Errorf("load aws configuration: %w", err)
	}
	if cfg.Region == "" {
		return cfg, errors.New("no aws region configured; pass --region or set AWS_REGION")
	}
	return cfg, nil
}

// callerAccount resolves the account the credentials belong to, so a command
// that deploys into it or erases it can say which one it means.
func callerAccount(ctx context.Context, cfg awssdk.Config) (string, error) {
	identity, err := sts.NewFromConfig(cfg).GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", fmt.Errorf("resolve the calling account: %w", err)
	}
	return *identity.Account, nil
}

// resources is the cloud control CRUDL the plugin api runs on, for accounts
// reached without the cloudjam provider (your own, or localstack).
type resources struct {
	client *cloudcontrol.Client
}

func (r *resources) Create(ctx context.Context, resourceType, state string) (string, error) {
	resp, err := r.client.CreateResource(ctx, &cloudcontrol.CreateResourceInput{
		TypeName:     &resourceType,
		DesiredState: &state,
	})
	if err != nil {
		return "", err
	}
	status, err := cloudcontrol.NewResourceRequestSuccessWaiter(r.client).WaitForOutput(ctx,
		&cloudcontrol.GetResourceRequestStatusInput{RequestToken: resp.ProgressEvent.RequestToken},
		20*time.Minute)
	if err != nil {
		return "", err
	}
	return *status.ProgressEvent.Identifier, nil
}

func (r *resources) Read(ctx context.Context, resourceType, id string) (string, error) {
	resp, err := r.client.GetResource(ctx, &cloudcontrol.GetResourceInput{
		TypeName:   &resourceType,
		Identifier: &id,
	})
	if err != nil {
		return "", err
	}
	return *resp.ResourceDescription.Properties, nil
}

func (r *resources) Update(ctx context.Context, resourceType, id, patch string) error {
	resp, err := r.client.UpdateResource(ctx, &cloudcontrol.UpdateResourceInput{
		TypeName:      &resourceType,
		Identifier:    &id,
		PatchDocument: &patch,
	})
	if err != nil {
		return err
	}
	_, err = cloudcontrol.NewResourceRequestSuccessWaiter(r.client).WaitForOutput(ctx,
		&cloudcontrol.GetResourceRequestStatusInput{RequestToken: resp.ProgressEvent.RequestToken},
		20*time.Minute)
	return err
}

func (r *resources) Delete(ctx context.Context, resourceType, id string) error {
	_, err := r.client.DeleteResource(ctx, &cloudcontrol.DeleteResourceInput{
		TypeName:   &resourceType,
		Identifier: &id,
	})
	return err
}

func (r *resources) List(ctx context.Context, resourceType string) (map[string]string, error) {
	resp, err := r.client.ListResources(ctx, &cloudcontrol.ListResourcesInput{TypeName: &resourceType})
	if err != nil {
		return nil, err
	}
	states := map[string]string{}
	for _, resource := range resp.ResourceDescriptions {
		states[*resource.Identifier] = *resource.Properties
	}
	return states, nil
}

// nukeAccount erases everything cloud-nuke can reach with these credentials.
// It is the same sweep internal/provider/aws runs, for accounts that are not
// organization members.
func nukeAccount(ctx context.Context, cfg awssdk.Config, account string, regions []string) error {
	// cloud-nuke reads both of these off the context.
	ctx = context.WithValue(ctx, nukeutil.ExcludeFirstSeenTagKey, true)
	ctx = context.WithValue(ctx, nukeutil.AccountIdKey, account)

	backoff := 10 * time.Second
	var remaining error

	// Deletion is asynchronous nearly everywhere in aws, so a region that had
	// anything in it reports as unclean even when every call succeeded. Retry
	// until a pass finds nothing left.
	for attempt := 1; attempt <= 3; attempt++ {
		remaining = nil
		var lock sync.Mutex
		var group sync.WaitGroup

		slog.Info("nuking regions", "attempt", attempt, "regions", regions)
		for _, region := range regions {
			group.Go(func() {
				regional := cfg.Copy()
				regional.Region = region
				if err := nukeRegion(ctx, regional, region, nukeconfig.Config{}); err != nil {
					lock.Lock()
					defer lock.Unlock()
					remaining = errors.Join(remaining, err)
				}
			})
		}
		group.Wait()

		remaining = errors.Join(remaining, nukeRegion(ctx, cfg, "global", nukeconfig.Config{
			// Nuking the role you are authenticated as ends the sweep early and
			// leaves the rest of the account standing.
			IAMRoles: nukeconfig.ResourceType{ExcludeRule: nukeconfig.FilterRule{
				NamesRegExp: []nukeconfig.Expression{
					{RE: *regexp.MustCompile(`^cloudjam-admin$`)},
					{RE: *regexp.MustCompile(`^cloudjam-sandbox$`)},
				},
			}},
		}))
		if remaining == nil {
			return nil
		}

		// A sweep touches every resource type cloud-nuke knows, so a bad pass
		// joins over a hundred errors. Count them here, print them on --verbose.
		failures := strings.Split(remaining.Error(), "\n")
		slog.Debug("nuke failures", "error", remaining)
		if attempt == 3 || ctx.Err() != nil {
			return fmt.Errorf("account not fully nuked, %d failures (rerun with --verbose for all of them):\n%s",
				len(failures), strings.Join(failures[:min(5, len(failures))], "\n"))
		}
		slog.Warn("resources remain, retrying", "failures", len(failures), "backoff", backoff)

		select {
		case <-ctx.Done():
		case <-time.After(backoff):
			backoff *= 2
		}
	}
	return remaining
}

func nukeRegion(ctx context.Context, cfg awssdk.Config, region string, nukeConfig nukeconfig.Config) error {
	var errs error
	count := 0

	for _, resource := range nukeaws.GetAndInitRegisteredResources(cfg, region) {
		r := *resource
		r.GetAndSetResourceConfig(nukeConfig)

		ids, err := r.GetAndSetIdentifiers(ctx, nukeConfig)
		if err != nil {
			errs = errors.Join(errs, fmt.Errorf("%s: list %s: %w", region, r.ResourceName(), err))
			continue
		}
		count += len(ids)

		for i := 0; i < len(ids); i += r.MaxBatchSize() {
			batch := ids[i:min(i+r.MaxBatchSize(), len(ids))]
			if _, err := r.Nuke(ctx, batch); err != nil {
				errs = errors.Join(errs, fmt.Errorf("%s: nuke %s: %w", region, r.ResourceName(), err))
			}
		}
	}
	if count > 0 {
		errs = errors.Join(errs, fmt.Errorf("%s: %d resources remaining", region, count))
	}
	return errs
}
