package aws

import (
	"context"
	"fmt"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"

	"codeberg.org/megakuul/cloudjam/internal/sandbox"
)

// probe is one leak heuristic executed against a single region.
type probe struct {
	name string
	// pass is reported (severity info) when the probe finds nothing anywhere,
	// so every heuristic is always visible in the report.
	pass string
	// run returns findings for everything suspicious in the region.
	run func(ctx context.Context, config awssdk.Config, region string) ([]sandbox.Leak, error)
}

func (r *Provider) Check(ctx context.Context, id string) ([]sandbox.Leak, error) {
	leaks := []sandbox.Leak{}

	// heuristic 1: are the guardrail policies still in place and unmodified?
	leaks = append(leaks, r.verifyPolicies(ctx, id)...)

	// heuristic 2: does the (delayed) billing data already show a leak?
	leaks = append(leaks, r.scanBilling(ctx, id)...)

	// heuristic 3: are known money burners running right now?
	findings, err := r.scan(ctx, id)
	if err != nil {
		return nil, err
	}
	leaks = append(leaks, findings...)

	return leaks, nil
}

// scan runs all resource heuristics against every allowed region (other
// regions are blocked by the boundary scp and cannot contain resources
// created by competitors). It is used both for live leak detection (Check)
// and for the post-cleanup verification (Release): everything above severity
// info keeps an account out of the ready pool.
func (r *Provider) scan(ctx context.Context, id string) ([]sandbox.Finding, error) {
	config, _, err := r.assume(ctx, id, r.sandboxRole)
	if err != nil {
		return nil, err
	}

	var findings []sandbox.Finding
	for _, probe := range probes {
		found := []sandbox.Finding{}
		for _, region := range r.regions {
			results, err := probe.run(ctx, config, region)
			if err != nil {
				found = append(found, sandbox.Finding{
					Severity: sandbox.SeverityWarning,
					Region:   region,
					Resource: probe.name,
					Message:  fmt.Sprintf("heuristic failed, leak state unknown: %v", err),
				})
				continue
			}
			found = append(found, results...)
		}
		if len(found) == 0 {
			found = append(found, sandbox.Finding{
				Severity: sandbox.SeverityInfo,
				Resource: probe.name,
				Message:  fmt.Sprintf("%s (checked regions: %s)", probe.pass, strings.Join(r.regions, ", ")),
			})
		}
		findings = append(findings, found...)
	}
	return findings, nil
}
