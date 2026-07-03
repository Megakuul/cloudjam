package aws

import (
	"context"
	"fmt"
	"path"
	"strings"

	awssdk "github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/acmpca"
	acmpcatypes "github.com/aws/aws-sdk-go-v2/service/acmpca/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudhsmv2"
	cloudhsmtypes "github.com/aws/aws-sdk-go-v2/service/cloudhsmv2/types"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/rds"
	"github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi"

	"codeberg.org/megakuul/cloudjam/internal/sandbox"
)

// probe is one leak heuristic executed against a single region.
type probe struct {
	// name identifies the heuristic in findings.
	name string
	// pass is reported (severity info) when the probe finds nothing anywhere,
	// so every heuristic is always visible in the report.
	pass string
	// run returns findings for everything suspicious in the region.
	run func(ctx context.Context, config awssdk.Config, region string) ([]sandbox.Finding, error)
}

// scan runs all resource heuristics against every allowed region (other
// regions are blocked by the boundary scp and cannot contain resources
// created by competitors). It is used both for live leak detection (Check)
// and for the post-cleanup verification (Release): everything above severity
// info keeps an account out of the ready pool.
func (r *Repository) scan(ctx context.Context, id string) ([]sandbox.Finding, error) {
	config, _, err := r.assume(ctx, id, r.config.SandboxRole)
	if err != nil {
		return nil, err
	}

	var findings []sandbox.Finding
	for _, probe := range r.probes() {
		found := []sandbox.Finding{}
		for _, region := range r.config.Regions {
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
				Message:  fmt.Sprintf("%s (checked regions: %s)", probe.pass, strings.Join(r.config.Regions, ", ")),
			})
		}
		findings = append(findings, found...)
	}
	return findings, nil
}

func (r *Repository) allowedInstanceType(instanceType string) bool {
	for _, pattern := range r.config.InstanceTypes {
		if match, _ := path.Match(pattern, instanceType); match {
			return true
		}
	}
	return false
}

func (r *Repository) probes() []probe {
	return []probe{
		{
			name: "ec2-instances",
			pass: "no running ec2 instances",
			run: func(ctx context.Context, config awssdk.Config, region string) ([]sandbox.Finding, error) {
				client := ec2.NewFromConfig(config, func(o *ec2.Options) { o.Region = region })
				var findings []sandbox.Finding
				paginator := ec2.NewDescribeInstancesPaginator(client, &ec2.DescribeInstancesInput{
					Filters: []ec2types.Filter{
						{Name: new("instance-state-name"), Values: []string{"pending", "running"}},
					},
				})
				for paginator.HasMorePages() {
					page, err := paginator.NextPage(ctx)
					if err != nil {
						return nil, err
					}
					for _, reservation := range page.Reservations {
						for _, instance := range reservation.Instances {
							finding := sandbox.Finding{
								Severity: sandbox.SeverityWarning,
								Region:   region,
								Resource: *instance.InstanceId,
								Message:  fmt.Sprintf("running instance of type %q", instance.InstanceType),
							}
							if !r.allowedInstanceType(string(instance.InstanceType)) {
								finding.Severity = sandbox.SeverityCritical
								finding.Message += " outside the allowed types (boundary scp breached?)"
							}
							findings = append(findings, finding)
						}
					}
				}
				return findings, nil
			},
		},
		{
			name: "ec2-nat-gateways",
			pass: "no nat gateways",
			run: func(ctx context.Context, config awssdk.Config, region string) ([]sandbox.Finding, error) {
				client := ec2.NewFromConfig(config, func(o *ec2.Options) { o.Region = region })
				var findings []sandbox.Finding
				paginator := ec2.NewDescribeNatGatewaysPaginator(client, &ec2.DescribeNatGatewaysInput{
					Filter: []ec2types.Filter{
						{Name: new("state"), Values: []string{"pending", "available"}},
					},
				})
				for paginator.HasMorePages() {
					page, err := paginator.NextPage(ctx)
					if err != nil {
						return nil, err
					}
					for _, gateway := range page.NatGateways {
						findings = append(findings, sandbox.Finding{
							Severity: sandbox.SeverityWarning,
							Region:   region,
							Resource: *gateway.NatGatewayId,
							Message:  "active nat gateway",
							Burn:     "~$35/month + traffic",
						})
					}
				}
				return findings, nil
			},
		},
		{
			name: "ec2-addresses",
			pass: "no unassociated elastic ips",
			run: func(ctx context.Context, config awssdk.Config, region string) ([]sandbox.Finding, error) {
				client := ec2.NewFromConfig(config, func(o *ec2.Options) { o.Region = region })
				addresses, err := client.DescribeAddresses(ctx, &ec2.DescribeAddressesInput{})
				if err != nil {
					return nil, err
				}
				var findings []sandbox.Finding
				for _, address := range addresses.Addresses {
					if address.AssociationId != nil {
						continue
					}
					findings = append(findings, sandbox.Finding{
						Severity: sandbox.SeverityWarning,
						Region:   region,
						Resource: awssdk.ToString(address.AllocationId),
						Message:  "unassociated elastic ip",
						Burn:     "~$4/month",
					})
				}
				return findings, nil
			},
		},
		{
			name: "ec2-dedicated-hosts",
			pass: "no dedicated hosts",
			run: func(ctx context.Context, config awssdk.Config, region string) ([]sandbox.Finding, error) {
				client := ec2.NewFromConfig(config, func(o *ec2.Options) { o.Region = region })
				hosts, err := client.DescribeHosts(ctx, &ec2.DescribeHostsInput{})
				if err != nil {
					return nil, err
				}
				var findings []sandbox.Finding
				for _, host := range hosts.Hosts {
					if host.State != ec2types.AllocationStateAvailable && host.State != ec2types.AllocationStatePending {
						continue
					}
					findings = append(findings, sandbox.Finding{
						Severity: sandbox.SeverityCritical,
						Region:   region,
						Resource: awssdk.ToString(host.HostId),
						Message:  "allocated dedicated host (mac hosts have a 24h minimum charge)",
						Burn:     "$500+/month",
					})
				}
				return findings, nil
			},
		},
		{
			name: "ec2-capacity-reservations",
			pass: "no active capacity reservations",
			run: func(ctx context.Context, config awssdk.Config, region string) ([]sandbox.Finding, error) {
				client := ec2.NewFromConfig(config, func(o *ec2.Options) { o.Region = region })
				reservations, err := client.DescribeCapacityReservations(ctx, &ec2.DescribeCapacityReservationsInput{})
				if err != nil {
					return nil, err
				}
				var findings []sandbox.Finding
				for _, reservation := range reservations.CapacityReservations {
					if reservation.State != ec2types.CapacityReservationStateActive {
						continue
					}
					findings = append(findings, sandbox.Finding{
						Severity: sandbox.SeverityCritical,
						Region:   region,
						Resource: awssdk.ToString(reservation.CapacityReservationId),
						Message:  "active capacity reservation bills like running instances",
					})
				}
				return findings, nil
			},
		},
		{
			name: "ec2-volumes",
			pass: "no unattached ebs volumes",
			run: func(ctx context.Context, config awssdk.Config, region string) ([]sandbox.Finding, error) {
				client := ec2.NewFromConfig(config, func(o *ec2.Options) { o.Region = region })
				var findings []sandbox.Finding
				paginator := ec2.NewDescribeVolumesPaginator(client, &ec2.DescribeVolumesInput{
					Filters: []ec2types.Filter{
						{Name: new("status"), Values: []string{"available"}},
					},
				})
				for paginator.HasMorePages() {
					page, err := paginator.NextPage(ctx)
					if err != nil {
						return nil, err
					}
					for _, volume := range page.Volumes {
						findings = append(findings, sandbox.Finding{
							Severity: sandbox.SeverityWarning,
							Region:   region,
							Resource: awssdk.ToString(volume.VolumeId),
							Message:  fmt.Sprintf("unattached ebs volume (%d GiB)", awssdk.ToInt32(volume.Size)),
							Burn:     "~$0.08/GiB/month",
						})
					}
				}
				return findings, nil
			},
		},
		{
			name: "acm-pca",
			pass: "no private certificate authorities",
			run: func(ctx context.Context, config awssdk.Config, region string) ([]sandbox.Finding, error) {
				client := acmpca.NewFromConfig(config, func(o *acmpca.Options) { o.Region = region })
				var findings []sandbox.Finding
				paginator := acmpca.NewListCertificateAuthoritiesPaginator(client, &acmpca.ListCertificateAuthoritiesInput{})
				for paginator.HasMorePages() {
					page, err := paginator.NextPage(ctx)
					if err != nil {
						return nil, err
					}
					for _, authority := range page.CertificateAuthorities {
						if authority.Status == acmpcatypes.CertificateAuthorityStatusDeleted {
							continue
						}
						findings = append(findings, sandbox.Finding{
							Severity: sandbox.SeverityCritical,
							Region:   region,
							Resource: awssdk.ToString(authority.Arn),
							Message:  fmt.Sprintf("private certificate authority in status %q", authority.Status),
							Burn:     "~$400/month",
						})
					}
				}
				return findings, nil
			},
		},
		{
			name: "cloudhsm",
			pass: "no cloudhsm clusters",
			run: func(ctx context.Context, config awssdk.Config, region string) ([]sandbox.Finding, error) {
				client := cloudhsmv2.NewFromConfig(config, func(o *cloudhsmv2.Options) { o.Region = region })
				clusters, err := client.DescribeClusters(ctx, &cloudhsmv2.DescribeClustersInput{})
				if err != nil {
					return nil, err
				}
				var findings []sandbox.Finding
				for _, cluster := range clusters.Clusters {
					if cluster.State == cloudhsmtypes.ClusterStateDeleted {
						continue
					}
					findings = append(findings, sandbox.Finding{
						Severity: sandbox.SeverityCritical,
						Region:   region,
						Resource: awssdk.ToString(cluster.ClusterId),
						Message:  fmt.Sprintf("cloudhsm cluster in state %q", cluster.State),
						Burn:     "~$1,300/month per hsm",
					})
				}
				return findings, nil
			},
		},
		{
			name: "rds",
			pass: "no rds instances",
			run: func(ctx context.Context, config awssdk.Config, region string) ([]sandbox.Finding, error) {
				client := rds.NewFromConfig(config, func(o *rds.Options) { o.Region = region })
				var findings []sandbox.Finding
				paginator := rds.NewDescribeDBInstancesPaginator(client, &rds.DescribeDBInstancesInput{})
				for paginator.HasMorePages() {
					page, err := paginator.NextPage(ctx)
					if err != nil {
						return nil, err
					}
					for _, instance := range page.DBInstances {
						class := awssdk.ToString(instance.DBInstanceClass)
						finding := sandbox.Finding{
							Severity: sandbox.SeverityWarning,
							Region:   region,
							Resource: awssdk.ToString(instance.DBInstanceIdentifier),
							Message:  fmt.Sprintf("rds instance of class %q in status %q", class, awssdk.ToString(instance.DBInstanceStatus)),
						}
						if !strings.HasPrefix(class, "db.t") {
							finding.Severity = sandbox.SeverityCritical
							finding.Message += "; class outside the burstable families"
							finding.Burn = "$100+/month"
						}
						findings = append(findings, finding)
					}
				}
				return findings, nil
			},
		},
		{
			name: "cloudwatch",
			pass: "no metric streams, contributor insights rules or heavy custom metrics",
			run: func(ctx context.Context, config awssdk.Config, region string) ([]sandbox.Finding, error) {
				client := cloudwatch.NewFromConfig(config, func(o *cloudwatch.Options) { o.Region = region })
				var findings []sandbox.Finding

				streams, err := client.ListMetricStreams(ctx, &cloudwatch.ListMetricStreamsInput{})
				if err != nil {
					return nil, err
				}
				for _, stream := range streams.Entries {
					findings = append(findings, sandbox.Finding{
						Severity: sandbox.SeverityWarning,
						Region:   region,
						Resource: awssdk.ToString(stream.Name),
						Message:  "cloudwatch metric stream bills per metric update",
					})
				}

				rules, err := client.DescribeInsightRules(ctx, &cloudwatch.DescribeInsightRulesInput{})
				if err != nil {
					return nil, err
				}
				for _, rule := range rules.InsightRules {
					findings = append(findings, sandbox.Finding{
						Severity: sandbox.SeverityWarning,
						Region:   region,
						Resource: awssdk.ToString(rule.Name),
						Message:  "contributor insights rule bills per matched log event",
					})
				}

				custom := 0
				paginator := cloudwatch.NewListMetricsPaginator(client, &cloudwatch.ListMetricsInput{})
				for paginator.HasMorePages() {
					page, err := paginator.NextPage(ctx)
					if err != nil {
						return nil, err
					}
					for _, metric := range page.Metrics {
						if !strings.HasPrefix(awssdk.ToString(metric.Namespace), "AWS/") {
							custom++
						}
					}
				}
				if custom > 500 {
					findings = append(findings, sandbox.Finding{
						Severity: sandbox.SeverityWarning,
						Region:   region,
						Resource: "custom-metrics",
						Message:  fmt.Sprintf("%d custom metrics; high cardinality dimensions burn fast", custom),
						Burn:     "~$0.30/metric/month",
					})
				}
				return findings, nil
			},
		},
		{
			name: "cloudwatch-logs",
			pass: "no large log groups without retention",
			run: func(ctx context.Context, config awssdk.Config, region string) ([]sandbox.Finding, error) {
				client := cloudwatchlogs.NewFromConfig(config, func(o *cloudwatchlogs.Options) { o.Region = region })
				var findings []sandbox.Finding
				paginator := cloudwatchlogs.NewDescribeLogGroupsPaginator(client, &cloudwatchlogs.DescribeLogGroupsInput{})
				for paginator.HasMorePages() {
					page, err := paginator.NextPage(ctx)
					if err != nil {
						return nil, err
					}
					for _, group := range page.LogGroups {
						stored := awssdk.ToInt64(group.StoredBytes)
						if group.RetentionInDays != nil || stored < 5<<30 {
							continue
						}
						findings = append(findings, sandbox.Finding{
							Severity: sandbox.SeverityWarning,
							Region:   region,
							Resource: awssdk.ToString(group.LogGroupName),
							Message:  fmt.Sprintf("log group stores %d GiB with retention disabled (ingestion bills $0.50/GB)", stored>>30),
							Burn:     "~$0.03/GiB/month + ingestion",
						})
					}
				}
				return findings, nil
			},
		},
		{
			name: "leftovers",
			pass: "no tagged resources present",
			run: func(ctx context.Context, config awssdk.Config, region string) ([]sandbox.Finding, error) {
				client := resourcegroupstaggingapi.NewFromConfig(config, func(o *resourcegroupstaggingapi.Options) { o.Region = region })
				var resources []string
				paginator := resourcegroupstaggingapi.NewGetResourcesPaginator(client, &resourcegroupstaggingapi.GetResourcesInput{})
				for paginator.HasMorePages() {
					page, err := paginator.NextPage(ctx)
					if err != nil {
						return nil, err
					}
					for _, resource := range page.ResourceTagMappingList {
						resources = append(resources, awssdk.ToString(resource.ResourceARN))
					}
				}
				if len(resources) == 0 {
					return nil, nil
				}
				samples := resources
				if len(samples) > 5 {
					samples = samples[:5]
				}
				return []sandbox.Finding{{
					Severity: sandbox.SeverityWarning,
					Region:   region,
					Resource: "tagging-api",
					Message:  fmt.Sprintf("%d tagged resources present (e.g. %s); untagged resources are not visible here", len(resources), strings.Join(samples, ", ")),
				}}, nil
			},
		},
	}
}
