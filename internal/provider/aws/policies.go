package aws

import (
	"encoding/json"
	"fmt"
	"strconv"
)

type policyDocument struct {
	Version   string            `json:"Version"`
	Statement []policyStatement `json:"Statement"`
}

type policyStatement struct {
	Sid         string                    `json:"Sid"`
	Effect      string                    `json:"Effect"`
	Action      []string                  `json:"Action,omitempty"`
	NotAction   []string                  `json:"NotAction,omitempty"`
	Principal   map[string]string         `json:"Principal,omitempty"`
	Resource    []string                  `json:"Resource,omitempty"`
	NotResource []string                  `json:"NotResource,omitempty"`
	Condition   map[string]map[string]any `json:"Condition,omitempty"`
}

// guardPolicy constructs the primary security SCP for sandbox accounts.
// - protects the accounts root and sandbox role
// - blocks possible account escape routes
// - requires the boundaryPolicy to be attached to every new entity created.
//
// IMPORTANT: We give a lot of effort to keep this system clean and avoid privilege escalation inside the account,
// however, we are dealing with highly complex security by configuration here; keep in mind that it will 100% have at least one small
// bug or escape hatch. The retarded AWS system unfortunately does not allow a serious security concept here.
// It MUST be clearly mentioned in the ToS and other papers that escaping the account guards is NOT allowed
// and not part of the challenge (even if technically possible).
func guardControlPolicy(adminRole, sandboxRole, boundaryPolicy string) ([]byte, error) {
	adminRoleARN := fmt.Sprintf("arn:aws:iam::*:role/%s", adminRole)
	sandboxRoleARN := fmt.Sprintf("arn:aws:iam::*:role/%s", sandboxRole)
	boundaryPolicyARN := fmt.Sprintf("arn:aws:iam::*:policy/%s", boundaryPolicy)
	return json.Marshal(policyDocument{
		Version: "2012-10-17",
		Statement: []policyStatement{
			{
				Sid:    "FreezeUngovernedPrincipals",
				Effect: "Deny",
				Action: []string{
					"iam:UpdateAssumeRolePolicy",
					"iam:AttachRolePolicy",
					"iam:PutRolePolicy",
					"iam:DetachRolePolicy",
					"iam:DeleteRolePolicy",
					"iam:PutRolePermissionsBoundary",
					"iam:DeleteRolePermissionsBoundary",
					"iam:UpdateRole",
					"iam:TagRole",
					"iam:UntagRole",
					"iam:PassRole",
				},
				Resource: []string{
					"arn:aws:iam::*:role/aws-service-role/*",
					"arn:aws:iam::*:role/aws-reserved/*",
					"arn:aws:iam::*:role/service-role/*",
					"arn:aws:iam::*:role/OrganizationAccountAccessRole",
					"arn:aws:iam::*:role/AWSControlTowerExecution",
					"arn:aws:iam::*:role/aws-reserved/sso.amazonaws.com/*",
				},
				Condition: map[string]map[string]any{
					"ArnNotLike": {"aws:PrincipalARN": adminRoleARN},
				},
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
					"ArnNotLike": {"aws:PrincipalARN": adminRoleARN},
				},
			},
			{
				Sid:      "DenyGuardTamper",
				Effect:   "Deny",
				Action:   []string{"*"},
				Resource: []string{adminRoleARN, sandboxRoleARN},
				Condition: map[string]map[string]any{
					"ArnNotLike": {"aws:PrincipalARN": adminRoleARN},
				},
			},
			{
				Sid:    "DenyIdentityWithoutBoundary",
				Effect: "Deny",
				Action: []string{
					"iam:CreateUser",
					"iam:CreateRole",
					"iam:PutUserPolicy",
					"iam:PutRolePolicy",
					"iam:AttachUserPolicy",
					"iam:AttachRolePolicy",
				},
				Resource: []string{"*"},
				Condition: map[string]map[string]any{
					"StringNotEquals": {"iam:PermissionsBoundary": boundaryPolicyARN},
					"ArnNotLike":      {"aws:PrincipalARN": adminRoleARN},
				},
			},
			{
				Sid:    "DenyBoundaryDetach",
				Effect: "Deny",
				Action: []string{
					"iam:DeleteUserPermissionsBoundary",
					"iam:DeleteRolePermissionsBoundary",
				},
				Resource: []string{"*"},
				Condition: map[string]map[string]any{
					"ArnNotLike": {"aws:PrincipalARN": adminRoleARN},
				},
			},
			{
				Sid:    "DenyBoundarySwap",
				Effect: "Deny",
				Action: []string{
					"iam:PutUserPermissionsBoundary",
					"iam:PutRolePermissionsBoundary",
				},
				Resource: []string{"*"},
				Condition: map[string]map[string]any{
					"StringNotEquals": {"iam:PermissionsBoundary": boundaryPolicyARN},
					"ArnNotLike":      {"aws:PrincipalARN": adminRoleARN},
				},
			},
			{
				Sid:    "DenyBoundaryPolicyTamper",
				Effect: "Deny",
				Action: []string{
					"iam:CreatePolicyVersion",
					"iam:DeletePolicy",
					"iam:DeletePolicyVersion",
					"iam:SetDefaultPolicyVersion",
				},
				Resource: []string{boundaryPolicyARN},
				Condition: map[string]map[string]any{
					"ArnNotLike": {"aws:PrincipalARN": adminRoleARN},
				},
			},
		},
	})
}

// costControlPolicy generates an SCP that denies common cost / commitment traps.
// It also denies resources that should definitely never be used (like regions out of scope).
func costControlPolicy(regions []string) ([]byte, error) {
	return json.Marshal(policyDocument{
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
					"StringNotEquals": {"aws:RequestedRegion": regions},
				},
			},
			{
				Sid:      "DenyInstanceTypes",
				Effect:   "Deny",
				Action:   []string{"ec2:RunInstances"},
				Resource: []string{"arn:aws:ec2:*:*:instance/*"},
				Condition: map[string]map[string]any{
					"StringNotLike": {"ec2:InstanceType": allowedInstanceTypes},
				},
			},
			{
				Sid:      "DenyLargeVolumes",
				Effect:   "Deny",
				Action:   []string{"ec2:CreateVolume", "ec2:RunInstances"},
				Resource: []string{"arn:aws:ec2:*:*:volume/*"},
				Condition: map[string]map[string]any{
					"NumericGreaterThan": {"ec2:VolumeSize": strconv.Itoa(maxVolumeSize)},
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
		},
	})
}
