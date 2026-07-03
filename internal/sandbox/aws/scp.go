package aws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"

	"github.com/aws/aws-sdk-go-v2/service/organizations"
	orgtypes "github.com/aws/aws-sdk-go-v2/service/organizations/types"

	"codeberg.org/megakuul/cloudjam/internal/sandbox"
)

// Two service control policies protect every pooled account:
//
// cloudjam-sandbox-guard denies api actions that burn money instantly or that
// create resources the cleanup tools cannot remove. The list is based on the
// scp published at hackingthe.cloud/aws/general-knowledge/block-expensive-actions-with-scps
// (reserved instance / capacity / savings plan purchases, private certificate
// authorities at ~$400/month, the ses deliverability dashboard at $1250/month,
// shield advanced at $3000/month, bedrock provisioned throughput, domain
// registrations, marketplace subscriptions, snowball orders, outposts) and is
// extended with the cloudwatch burner family (internet monitor, synthetics
// canaries, contributor insights, metric streams), exportable acm public
// certificates (billed per issued certificate via the acm:Export request
// condition), cloudhsm (~$1300/month per hsm), kendra indexes (~$800/month),
// opensearch serverless collections and medialive channels (several $/hour).
// It additionally denies retention locks (s3 object lock, glacier/backup
// vault locks) which would make the account impossible to nuke, denies
// leaving the organization or closing the account, and protects the
// bootstrap roles against tampering.
//
// cloudjam-sandbox-boundary restricts where and how big: all regions outside
// Config.Regions are denied (global services exempted), ec2 instance types
// are limited to Config.InstanceTypes, ebs volumes to Config.VolumeSize GiB
// and sagemaker workloads on accelerator instances (ml.p*/g*/trn*/inf*/dl*)
// are denied.
//
// Note that scps also apply to the account root user and cannot be modified
// from inside the member account, only from the management account.
const (
	policyGuard    = "cloudjam-sandbox-guard"
	policyBoundary = "cloudjam-sandbox-boundary"

	// maxPolicySize is the aws hard limit for scp documents in bytes.
	maxPolicySize = 5120
)

type policyDocument struct {
	Version   string            `json:"Version"`
	Statement []policyStatement `json:"Statement"`
}

type policyStatement struct {
	Sid       string                    `json:"Sid"`
	Effect    string                    `json:"Effect"`
	Action    []string                  `json:"Action,omitempty"`
	NotAction []string                  `json:"NotAction,omitempty"`
	Resource  []string                  `json:"Resource"`
	Condition map[string]map[string]any `json:"Condition,omitempty"`
}

func (r *Repository) guardPolicy() (string, error) {
	roles := []string{
		fmt.Sprintf("arn:aws:iam::*:role/%s", r.config.OrganizationRole),
		fmt.Sprintf("arn:aws:iam::*:role/%s", r.config.SandboxRole),
	}
	return marshalPolicy(policyDocument{
		Version: "2012-10-17",
		Statement: []policyStatement{
			{
				Sid:    "DenyCommitments",
				Effect: "Deny",
				Action: []string{
					"acm-pca:CreateCertificateAuthority",
					"aoss:CreateCollection",
					"aws-marketplace:AcceptAgreementApprovalRequest",
					"aws-marketplace:Subscribe",
					"bedrock:CreateModelCustomizationJob",
					"bedrock:CreateProvisionedModelThroughput",
					"bedrock:UpdateProvisionedModelThroughput",
					"cloudhsm:CreateCluster",
					"cloudhsm:CreateHsm",
					"devicefarm:PurchaseOffering",
					"dynamodb:PurchaseReservedCapacityOfferings",
					"ec2:AllocateHosts",
					"ec2:CreateCapacityReservation",
					"ec2:CreateCapacityReservationFleet",
					"ec2:ModifyReservedInstances",
					"ec2:PurchaseCapacityBlock",
					"ec2:PurchaseCapacityBlockExtension",
					"ec2:PurchaseHostReservation",
					"ec2:PurchaseReservedInstancesOffering",
					"ec2:PurchaseScheduledInstances",
					"elasticache:PurchaseReservedCacheNodesOffering",
					"es:PurchaseReservedElasticsearchInstanceOffering",
					"es:PurchaseReservedInstanceOffering",
					"fsx:CreateFileSystem",
					"kendra:CreateIndex",
					"lambda:PutProvisionedConcurrencyConfig",
					"mediaconnect:PurchaseOffering",
					"medialive:CreateChannel",
					"medialive:CreateInput",
					"medialive:PurchaseOffering",
					"memorydb:PurchaseReservedNodesOffering",
					"outposts:CreateOutpost",
					"rds:PurchaseReservedDBInstancesOffering",
					"redshift-serverless:CreateWorkgroup",
					"redshift:PurchaseReservedNodeOffering",
					"route53domains:RegisterDomain",
					"route53domains:RenewDomain",
					"route53domains:TransferDomain",
					"savingsplans:CreateSavingsPlan",
					"ses:PutDeliverabilityDashboardOption",
					"shield:CreateSubscription",
					"snowball:CreateCluster",
					"snowball:CreateJob",
				},
				Resource: []string{"*"},
			},
			{
				Sid:    "DenyMonitorTraps",
				Effect: "Deny",
				Action: []string{
					"cloudwatch:PutInsightRule",
					"cloudwatch:PutMetricStream",
					"internetmonitor:CreateMonitor",
					"synthetics:CreateCanary",
				},
				Resource: []string{"*"},
			},
			{
				Sid:      "DenyExportableCertificates",
				Effect:   "Deny",
				Action:   []string{"acm:RequestCertificate"},
				Resource: []string{"*"},
				Condition: map[string]map[string]any{
					"StringEquals": {"acm:Export": "ENABLED"},
				},
			},
			{
				Sid:      "DenyCertificateExport",
				Effect:   "Deny",
				Action:   []string{"acm:ExportCertificate"},
				Resource: []string{"*"},
			},
			{
				Sid:    "DenyRetentionLocks",
				Effect: "Deny",
				Action: []string{
					"backup:PutBackupVaultLockConfiguration",
					"glacier:CompleteVaultLock",
					"glacier:InitiateVaultLock",
					"s3-object-lambda:PutObjectLegalHold",
					"s3-object-lambda:PutObjectRetention",
					"s3:BypassGovernanceRetention",
					"s3:PutBucketObjectLockConfiguration",
					"s3:PutObjectLegalHold",
					"s3:PutObjectRetention",
				},
				Resource: []string{"*"},
			},
			{
				Sid:    "DenyPoolEscape",
				Effect: "Deny",
				Action: []string{
					"account:CloseAccount",
					"iam:CreateAccountAlias",
					"iam:DeleteAccountAlias",
					"organizations:LeaveOrganization",
				},
				Resource: []string{"*"},
				Condition: map[string]map[string]any{
					"ArnNotLike": {"aws:PrincipalARN": roles},
				},
			},
			{
				Sid:      "DenyGuardTamper",
				Effect:   "Deny",
				Action:   []string{"iam:*"},
				Resource: roles,
				Condition: map[string]map[string]any{
					"ArnNotLike": {"aws:PrincipalARN": roles},
				},
			},
		},
	})
}

func (r *Repository) boundaryPolicy() (string, error) {
	return marshalPolicy(policyDocument{
		Version: "2012-10-17",
		Statement: []policyStatement{
			{
				Sid:    "DenyOutsideRegions",
				Effect: "Deny",
				NotAction: []string{
					"budgets:*",
					"ce:*",
					"cloudfront:*",
					"cur:*",
					"globalaccelerator:*",
					"health:*",
					"iam:*",
					"organizations:*",
					"route53:*",
					"shield:*",
					"sts:*",
					"support:*",
					"tag:*",
					"waf:*",
					"wafv2:*",
				},
				Resource: []string{"*"},
				Condition: map[string]map[string]any{
					"StringNotEquals": {"aws:RequestedRegion": r.config.Regions},
				},
			},
			{
				Sid:      "DenyInstanceTypes",
				Effect:   "Deny",
				Action:   []string{"ec2:RunInstances"},
				Resource: []string{"arn:aws:ec2:*:*:instance/*"},
				Condition: map[string]map[string]any{
					"StringNotLike": {"ec2:InstanceType": r.config.InstanceTypes},
				},
			},
			{
				Sid:      "DenyLargeVolumes",
				Effect:   "Deny",
				Action:   []string{"ec2:CreateVolume", "ec2:RunInstances"},
				Resource: []string{"arn:aws:ec2:*:*:volume/*"},
				Condition: map[string]map[string]any{
					"NumericGreaterThan": {"ec2:VolumeSize": strconv.Itoa(r.config.VolumeSize)},
				},
			},
			{
				Sid:      "DenyAccelerators",
				Effect:   "Deny",
				Action:   []string{"sagemaker:*"},
				Resource: []string{"*"},
				Condition: map[string]map[string]any{
					"ForAnyValue:StringLike": {"sagemaker:InstanceTypes": []string{
						"ml.p*", "ml.g*", "ml.trn*", "ml.inf*", "ml.dl*",
					}},
				},
			},
		},
	})
}

func marshalPolicy(document policyDocument) (string, error) {
	content, err := json.Marshal(document)
	if err != nil {
		return "", fmt.Errorf("failed to marshal policy: %w", err)
	}
	if len(content) > maxPolicySize {
		return "", fmt.Errorf("policy exceeds the aws scp size limit (%d > %d bytes)", len(content), maxPolicySize)
	}
	return string(content), nil
}

// equalPolicy compares two policy documents structurally (aws normalizes whitespace).
func equalPolicy(expected, actual string) bool {
	var expectedTree, actualTree any
	if err := json.Unmarshal([]byte(expected), &expectedTree); err != nil {
		return false
	}
	if err := json.Unmarshal([]byte(actual), &actualTree); err != nil {
		return false
	}
	return reflect.DeepEqual(expectedTree, actualTree)
}

func (r *Repository) policies() (map[string]string, error) {
	guard, err := r.guardPolicy()
	if err != nil {
		return nil, err
	}
	boundary, err := r.boundaryPolicy()
	if err != nil {
		return nil, err
	}
	return map[string]string{policyGuard: guard, policyBoundary: boundary}, nil
}

func (r *Repository) findPolicy(ctx context.Context, name string) (*orgtypes.PolicySummary, error) {
	paginator := organizations.NewListPoliciesPaginator(r.organizations, &organizations.ListPoliciesInput{
		Filter: orgtypes.PolicyTypeServiceControlPolicy,
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list service control policies: %w", err)
		}
		for _, policy := range page.Policies {
			if *policy.Name == name {
				return &policy, nil
			}
		}
	}
	return nil, nil
}

// ensurePolicyType enables the scp policy type on the organization root
// (requires the organization to run with "all features" enabled).
func (r *Repository) ensurePolicyType(ctx context.Context) error {
	roots, err := r.organizations.ListRoots(ctx, &organizations.ListRootsInput{})
	if err != nil {
		return fmt.Errorf("failed to list organization roots: %w", err)
	}
	for _, root := range roots.Roots {
		enabled := false
		for _, policyType := range root.PolicyTypes {
			if policyType.Type == orgtypes.PolicyTypeServiceControlPolicy && policyType.Status == orgtypes.PolicyTypeStatusEnabled {
				enabled = true
			}
		}
		if !enabled {
			_, err := r.organizations.EnablePolicyType(ctx, &organizations.EnablePolicyTypeInput{
				RootId:     root.Id,
				PolicyType: orgtypes.PolicyTypeServiceControlPolicy,
			})
			var already *orgtypes.PolicyTypeAlreadyEnabledException
			if err != nil && !errors.As(err, &already) {
				return fmt.Errorf("failed to enable service control policies on root %q: %w", *root.Id, err)
			}
		}
	}
	return nil
}

// ensurePolicies creates or updates the sandbox scps and returns their ids by name.
func (r *Repository) ensurePolicies(ctx context.Context) (map[string]string, error) {
	if err := r.ensurePolicyType(ctx); err != nil {
		return nil, err
	}
	policies, err := r.policies()
	if err != nil {
		return nil, err
	}
	ids := map[string]string{}
	for name, content := range policies {
		summary, err := r.findPolicy(ctx, name)
		if err != nil {
			return nil, err
		}
		if summary == nil {
			created, err := r.organizations.CreatePolicy(ctx, &organizations.CreatePolicyInput{
				Name:        &name,
				Description: new("managed by cloudjam sandbox; blocks money burners in pooled ctf accounts"),
				Type:        orgtypes.PolicyTypeServiceControlPolicy,
				Content:     &content,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to create scp %q: %w", name, err)
			}
			ids[name] = *created.Policy.PolicySummary.Id
			r.config.Logger.Info("created sandbox scp", "policy", name)
			continue
		}
		ids[name] = *summary.Id
		current, err := r.organizations.DescribePolicy(ctx, &organizations.DescribePolicyInput{
			PolicyId: summary.Id,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to describe scp %q: %w", name, err)
		}
		if !equalPolicy(content, *current.Policy.Content) {
			_, err := r.organizations.UpdatePolicy(ctx, &organizations.UpdatePolicyInput{
				PolicyId: summary.Id,
				Content:  &content,
			})
			if err != nil {
				return nil, fmt.Errorf("failed to update scp %q: %w", name, err)
			}
			r.config.Logger.Info("updated drifted sandbox scp", "policy", name)
		}
	}
	return ids, nil
}

func (r *Repository) attachPolicies(ctx context.Context, id string) error {
	ids, err := r.ensurePolicies(ctx)
	if err != nil {
		return err
	}
	for name, policyID := range ids {
		_, err := r.organizations.AttachPolicy(ctx, &organizations.AttachPolicyInput{
			PolicyId: &policyID,
			TargetId: &id,
		})
		var duplicate *orgtypes.DuplicatePolicyAttachmentException
		if err != nil && !errors.As(err, &duplicate) {
			return fmt.Errorf("failed to attach scp %q to account %q: %w", name, id, err)
		}
	}
	return nil
}

func (r *Repository) detachPolicies(ctx context.Context, id string) error {
	attached, err := r.organizations.ListPoliciesForTarget(ctx, &organizations.ListPoliciesForTargetInput{
		TargetId: &id,
		Filter:   orgtypes.PolicyTypeServiceControlPolicy,
	})
	if err != nil {
		return fmt.Errorf("failed to list scps of account %q: %w", id, err)
	}
	for _, policy := range attached.Policies {
		if *policy.Name != policyGuard && *policy.Name != policyBoundary {
			continue
		}
		_, err := r.organizations.DetachPolicy(ctx, &organizations.DetachPolicyInput{
			PolicyId: policy.Id,
			TargetId: &id,
		})
		if err != nil {
			return fmt.Errorf("failed to detach scp %q from account %q: %w", *policy.Name, id, err)
		}
	}
	return nil
}

// verifyPolicies checks that both sandbox scps are attached to the account
// and that their deployed content still matches the expected documents.
func (r *Repository) verifyPolicies(ctx context.Context, id string) []sandbox.Finding {
	expected, err := r.policies()
	if err != nil {
		return []sandbox.Finding{{
			Severity: sandbox.SeverityWarning,
			Resource: "scp",
			Message:  fmt.Sprintf("failed to render expected policies: %v", err),
		}}
	}
	attached, err := r.organizations.ListPoliciesForTarget(ctx, &organizations.ListPoliciesForTargetInput{
		TargetId: &id,
		Filter:   orgtypes.PolicyTypeServiceControlPolicy,
	})
	if err != nil {
		return []sandbox.Finding{{
			Severity: sandbox.SeverityWarning,
			Resource: "scp",
			Message:  fmt.Sprintf("failed to list attached policies: %v", err),
		}}
	}

	var findings []sandbox.Finding
	for name, content := range expected {
		var summary *orgtypes.PolicySummary
		for _, policy := range attached.Policies {
			if *policy.Name == name {
				summary = &policy
				break
			}
		}
		if summary == nil {
			findings = append(findings, sandbox.Finding{
				Severity: sandbox.SeverityCritical,
				Resource: name,
				Message:  "guardrail scp is not attached to the account; money burners are unblocked",
			})
			continue
		}
		current, err := r.organizations.DescribePolicy(ctx, &organizations.DescribePolicyInput{
			PolicyId: summary.Id,
		})
		if err != nil {
			findings = append(findings, sandbox.Finding{
				Severity: sandbox.SeverityWarning,
				Resource: name,
				Message:  fmt.Sprintf("failed to describe attached scp: %v", err),
			})
			continue
		}
		if !equalPolicy(content, *current.Policy.Content) {
			findings = append(findings, sandbox.Finding{
				Severity: sandbox.SeverityCritical,
				Resource: name,
				Message:  "attached scp content drifted from the expected document; run Add again to restore it",
			})
			continue
		}
		var document policyDocument
		_ = json.Unmarshal([]byte(content), &document)
		findings = append(findings, sandbox.Finding{
			Severity: sandbox.SeverityInfo,
			Resource: name,
			Message:  fmt.Sprintf("guardrail scp attached and up to date (%d deny statements)", len(document.Statement)),
		})
	}
	return findings
}
