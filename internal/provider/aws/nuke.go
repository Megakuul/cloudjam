package aws

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	nuke_aws "github.com/gruntwork-io/cloud-nuke/aws"
	nuke_config "github.com/gruntwork-io/cloud-nuke/config"
	"github.com/gruntwork-io/cloud-nuke/util"
)

func (r *Provider) Nuke(ctx context.Context, id string) error {
	// retarded ctx api implemented by cloudgrunts nuke tool (this API guys I'M LOSING MY FUCKING MIND)
	ctx = context.WithValue(ctx, util.ExcludeFirstSeenTagKey, true)
	ctx = context.WithValue(ctx, util.AccountIdKey, id)

	backoffTimeout := 10 * time.Second

	config, err := r.assume(ctx, id, r.adminRole, time.Hour*10)
	if err != nil {
		return err
	}
	r.logger.Info(fmt.Sprintf("starting nuke loop for account (account '%s')...", id))
	for {
		var retryErrs error
		var retryErrsLock sync.Mutex
		var regionWg sync.WaitGroup

		r.logger.Debug(fmt.Sprintf("nuking aws regional resources (account '%s')...", id))
		for _, region := range r.regions {
			regionWg.Add(1)
			go func() {
				defer regionWg.Done()
				regionConfig := config.Copy()
				regionConfig.Region = region
				if err := nukeRegion(ctx, regionConfig, region, nuke_config.Config{}); err != nil {
					retryErrsLock.Lock()
					defer retryErrsLock.Unlock()
					retryErrs = errors.Join(retryErrs, fmt.Errorf("%s: %w", region, err))
				}
			}()
		}
		regionWg.Wait()

		r.logger.Debug(fmt.Sprintf("nuking aws global resources (account '%s')...", id))
		retryErrs = errors.Join(retryErrs, nukeRegion(ctx, config, "global", nuke_config.Config{
			IAMRoles: nuke_config.ResourceType{ExcludeRule: nuke_config.FilterRule{
				NamesRegExp: []nuke_config.Expression{
					{RE: *regexp.MustCompile(fmt.Sprintf("^%s$", r.adminRole))},
					{RE: *regexp.MustCompile(fmt.Sprintf("^%s$", r.sandboxRole))},
				},
			}},
		}))

		if retryErrs == nil {
			return nil
		}
		r.logger.Debug(fmt.Sprintf("nuke will retry due to remaining resources (account '%s'):\n%v", id, retryErrs))
		select {
		case <-ctx.Done():
			return retryErrs
		case <-time.After(backoffTimeout):
			backoffTimeout *= 2
		}
	}
}

func nukeRegion(ctx context.Context, config aws.Config, region string, nukeConfig nuke_config.Config) error {
	registered := nuke_aws.GetAndInitRegisteredResources(config, region)

	var errs error
	resourceCount := 0
	for _, resource := range registered {
		r := *resource
		r.GetAndSetResourceConfig(nukeConfig)

		ids, err := r.GetAndSetIdentifiers(ctx, nukeConfig)
		if err != nil {
			errs = errors.Join(errs, err)
			continue
		}
		if len(ids) == 0 {
			continue
		}
		resourceCount += len(ids)

		for i := 0; i < len(ids); i += r.MaxBatchSize() {
			if _, err := r.Nuke(ctx, ids[i:min(i+r.MaxBatchSize(), len(ids))]); err != nil {
				errs = errors.Join(errs, fmt.Errorf("nuke %s: %w", r.ResourceName(), err))
			}
		}
	}
	// sounds paradox didn't we just nuke all resources.
	// Well rechecking the resource count now will fail almost always due to the async aws deletion process.
	// Therefore this function is only successfull if the account is already clean (otherwise it should retry).
	if resourceCount > 0 {
		errs = errors.Join(errs, fmt.Errorf("region not fully nuked (%d resources remaining)", resourceCount))
	}
	return errs
}
