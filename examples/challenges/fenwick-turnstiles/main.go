//go:build wasip1

// Command fenwick-turnstiles is a gameday tier cloudjam challenge.
//
// Fenwick Arena does not scan its own tickets. For eight seasons that was
// Turnstile Systems Inc.'s job: three independent gate clusters — VIP,
// General, Press — each streaming scans onto its own Kinesis stream, and a
// single admissions consumer turning every scan into a seat filled in the
// arena's ledger. Turnstile's parent company sold the division last week,
// and the new owner shut down the contract effective immediately. Nothing
// has been consuming any of the three streams since.
//
// The pipeline fans in rather than chains: three independent Kinesis
// streams, one consumer the player deploys and wires three times. That
// consumer is correct except that its write is unconditional, so Kinesis'
// at-least-once redelivery — the same guarantee every queue in this
// ecosystem makes — turns into duplicate admissions the moment real load
// hits it. The twist is a capacity problem rather than a data problem: the
// VIP stream ships with one shard, plenty for a quiet night, and a sellout
// show pushes VIP volume past what one shard can carry the moment the
// headliner's fans all arrive in the same twenty minutes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/policy"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/dynamodb"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/events"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/kinesis"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/kms"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/lambda"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/logs"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/s3"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/ssm"
	"github.com/google/uuid"
)

const ownerTag = "fenwick:owner"
const systemTag = "fenwick:system"
const systemName = "admissions"

// gatePrefix names the scenario's own generator function.
const gatePrefix = "fenwick-gate-"

const (
	handoverPath  = "/fenwick/handover"
	telemetryPath = "/fenwick/telemetry"
	incidentPath  = "/fenwick/incident-report"
)

const (
	roleRetries    = 6
	roleRetryDelay = 5 * time.Second
)

const (
	minRetentionDays = 30
	maxRetentionDays = 400
)

// The gate network's shape. VIP ships with one shard — see makeVipStream —
// and vipShardTarget is how many the player has to reach to survive the
// sellout at peak surge volume.
const (
	baseRate       = 15
	peakRate       = 55
	rampWindows    = 16
	vipShardTarget = 3
)

const (
	throughputPoints = 10
	throughputRounds = 10
	duplicateRounds  = 8
	duplicatePoints  = -18
	throttleRounds   = 8
	throttlePoints   = -18
	heldRounds       = 8
	heldPoints       = 8
	codaRounds       = 6
	codaPoints       = 10
)

const keepingUpPerMille = 550
const surgeGrace = 2

var (
	archiveRef       string
	decoyRef         string
	vipStreamRef     string
	vipStreamArn     string
	generalStreamArn string
	pressStreamArn   string
	ledgerRef        string
	ledgerArn        string
	ledgerLabel      string
	logGroupRef      string
	admissionsArn    string
	generatorRole    string
	generatorRef     string
	scheduleRef      string
)

var (
	keyBaseline      = map[string]bool{}
	functionBaseline = map[string]bool{}
)

var (
	selloutStarted atomic.Bool
	codaOpen       atomic.Bool
)

func main() {
	challenge.New("Fenwick Arena: Gate Fan-In", 10*time.Second, bootstrap).
		AddDescription(
			"Fenwick Arena runs about ninety shows a year across three gate clusters: VIP, "+
				"General and Press. For eight seasons every one of them was Turnstile Systems "+
				"Inc.'s problem — their scanners, their stream, their consumer turning a badge "+
				"scan into a seat filled. Their parent company sold the division nine days ago "+
				"and the new owner ended the support contract on the spot.").
		AddDescription(
			"Tonight is not a quiet Tuesday. It is the second of three sold-out nights for an "+
				"act big enough that the VIP list alone runs past four hundred names, and the "+
				"doors open in a few hours whether anything here works or not. Every gate is "+
				"still streaming scans onto its own Kinesis stream. Nothing has consumed any of "+
				"them since the contract ended.").
		AddDescription(
			"You are the engineer the arena's ops director just called. There is no "+
				"architecture diagram and no runbook — everything Turnstile built carries a "+
				systemTag+"="+systemName+" tag, and that tag is the entire asset register.").
		AddDescription(
			"The gate network reports what it sees to "+telemetryPath+" once a minute: scans "+
				"sent, scans that admitted cleanly, and scans that admitted more than once. "+
				"That parameter is your dashboard. You can read it. You cannot write it.").
		AddDescription(
			"Nobody is handing you a task list. You are scored the way the arena's box office "+
				"is measured: on seats filled correctly, against seats charged twice, and "+
				"against a VIP line that stops moving because a stream provisioned for a quiet "+
				"night meets a sold-out one. There is more here than there is time before doors.").
		AddClue("where do i start",
			"The handover note is an SSM parameter at "+handoverPath+". Read it first: "+
				"aws ssm get-parameter --name "+handoverPath+".",
			-5).
		AddClue("nothing is consuming the streams",
			"Turnstile's consumer went with them, but platform kept a copy — it is attached "+
				"to this challenge as a downloadable package. Building and deploying it is on "+
				"you.",
			-30).
		AddClue("i cannot create an execution role",
			"You do not have iam:CreateRole and are not meant to. The consumer's execution "+
				"role is already provisioned, scoped to all three streams and the ledger. Find "+
				"it with aws iam list-roles.",
			-20).
		AddClue("what counts as ownership",
			"The asset register only counts resources carrying a "+ownerTag+" tag. Any value "+
				"works — the point is a name is on it.",
			-20).
		AddClue("admissions are moving but the duplicate count is climbing",
			"Read what the consumer does to the ledger, then read what Kinesis guarantees "+
				"about delivery. It never errors and the streams drain. That combination is "+
				"the problem, not the absence of one.",
			-40).
		AddClue("what is a duplicate admission worth",
			"Considerably less than nothing — a seat charged twice with only one body in it. "+
				"The scoreboard prices it the way the box office would.",
			-15).
		AddClue("what would the ops director worry about first",
			"Ask what cannot be undone rather than what is untidy. A ledger with no way back "+
				"from tonight, photos the whole internet can read, and a gate that has stopped "+
				"moving people at all.",
			-25).
		SetPermission(policy.Document{
			Version: policy.Version20121017,
			Statement: []policy.Statement{
				{Effect: policy.Deny, Action: policy.ActionAll, Resource: policy.ARNAll},
			},
		}).
		SetGuardrail(guardrail()).
		// --- Act I: whose account is this now ------------------------------
		AddCheck("Signed for the gate streams", challenge.Check{
			Points:  30,
			Every:   20 * time.Second,
			Trigger: streamsOwned,
		}).
		AddCheck("Signed for the admissions ledger", challenge.Check{
			Points:  25,
			Every:   20 * time.Second,
			Trigger: ledgerOwned,
		}).
		AddCheck("Signed for the gate photo archive", challenge.Check{
			Points:  25,
			Every:   20 * time.Second,
			Trigger: archiveOwned,
		}).
		AddCheck("Signed for the merch inventory bucket", challenge.Check{
			Points:  10,
			Every:   20 * time.Second,
			Trigger: decoyOwned,
		}).
		AddCheck("Confirmed the handover", challenge.Check{
			Points:  45,
			Every:   20 * time.Second,
			Trigger: handoverConfirmed,
		}).
		AddCheck("Gave the admissions logs a retention window", challenge.Check{
			Points:  10,
			Every:   30 * time.Second,
			Trigger: logsRetained,
		}).
		// --- Act II: get admissions moving ---------------------------------
		AddCheck("Deployed the admissions consumer", challenge.Check{
			Points:  70,
			Every:   20 * time.Second,
			Trigger: admissionsDeployed,
		}).
		AddCheck("Wired the VIP stream into admissions", challenge.Check{
			Points:  45,
			Every:   20 * time.Second,
			Trigger: vipWired,
		}).
		AddCheck("Wired the General stream into admissions", challenge.Check{
			Points:  45,
			Every:   20 * time.Second,
			Trigger: generalWired,
		}).
		AddCheck("Wired the Press stream into admissions", challenge.Check{
			Points:  45,
			Every:   20 * time.Second,
			Trigger: pressWired,
		}).
		AddCheck("Armed admissions with a failure destination", challenge.Check{
			Points:  60,
			Every:   20 * time.Second,
			Trigger: failureDestinationArmed,
		}).
		AddCheck("Admissions are clearing", challenge.Check{
			Points:  throughputPoints,
			Every:   30 * time.Second,
			Repeat:  true,
			Trigger: bounded(throughputRounds, admissionsClearing()),
		}).
		AddCheck("Every scan admitted exactly once", challenge.Check{
			Points:  90,
			Every:   30 * time.Second,
			Trigger: admittedExactlyOnce(),
		}).
		AddCheck("Duplicate admissions reaching the ledger", challenge.Check{
			Points:  duplicatePoints,
			Every:   30 * time.Second,
			Repeat:  true,
			Trigger: bounded(duplicateRounds, duplicatesLanding()),
		}).
		AddEvent("sellout", challenge.Event{
			Every:   30 * time.Second,
			Trigger: firstCleanAdmission(),
			Event:   theSellout,
		}).
		AddEvent("last-call", challenge.Event{
			Every:   30 * time.Second,
			Trigger: selloutClosedOut,
			Event:   theLastCall,
		}).
		Start()
}

// --- permissions -----------------------------------------------------------

func workingSet() policy.Actions {
	return policy.Actions{
		"s3:Get*", "s3:List*", "s3:Put*",
		"sqs:*",
		"kinesis:Describe*", "kinesis:List*", "kinesis:TagResource", "kinesis:UntagResource",
		"kinesis:UpdateShardCount", "kinesis:IncreaseStreamRetentionPeriod",
		"kinesis:StartStreamEncryption",
		"lambda:*",
		"iam:PassRole", "iam:GetRole", "iam:ListRoles",
		"iam:ListRolePolicies", "iam:GetRolePolicy", "iam:ListAttachedRolePolicies",
		"dynamodb:Describe*", "dynamodb:List*",
		"dynamodb:UpdateTable", "dynamodb:UpdateContinuousBackups",
		"dynamodb:UpdateTimeToLive", "dynamodb:TagResource", "dynamodb:UntagResource",
		"logs:*", "kms:*",
		"ssm:Describe*", "ssm:Get*", "ssm:PutParameter", "ssm:DeleteParameter",
		"ssm:AddTagsToResource", "ssm:ListTagsForResource",
		"events:Describe*", "events:List*",
	}
}

// guardrail bounds what the working set above cannot: the gate network's own
// function and schedule, the telemetry parameter, and the ledger's item
// writes, which belong to the admissions consumer's own execution role and
// nobody else — the same shape every gameday challenge in this codebase
// uses, and for the same reasons. See meridian-farebox for the long version.
func guardrail() policy.Document {
	return policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{
				Sid:      "GateFanIn",
				Effect:   policy.Allow,
				Action:   workingSet(),
				Resource: policy.ARNAll,
			},
			{
				Sid:    "TheGateNetworkIsNotYours",
				Effect: policy.Deny,
				Action: policy.Actions{
					"lambda:UpdateFunctionCode", "lambda:UpdateFunctionConfiguration",
					"lambda:DeleteFunction", "lambda:InvokeFunction",
					"lambda:AddPermission", "lambda:RemovePermission",
				},
				Resource: policy.ARNs{policy.ARN("arn:aws:lambda:*:*:function:" + gatePrefix + "*")},
			},
			{
				Sid:      "TheScheduleIsNotYours",
				Effect:   policy.Deny,
				Action:   policy.Actions{"events:*"},
				Resource: policy.ARNs{policy.ARN("arn:aws:events:*:*:rule/" + gatePrefix + "*")},
			},
			{
				Sid:    "TelemetryIsReadOnly",
				Effect: policy.Deny,
				Action: policy.Actions{
					"ssm:PutParameter", "ssm:DeleteParameter", "ssm:DeleteParameters",
					"ssm:LabelParameterVersion",
				},
				Resource: policy.ARNs{policy.ARN("arn:aws:ssm:*:*:parameter" + telemetryPath)},
			},
			{
				Sid:    "TheLedgerIsWrittenByThePipeline",
				Effect: policy.Deny,
				Action: policy.Actions{
					"dynamodb:PutItem", "dynamodb:UpdateItem", "dynamodb:DeleteItem",
					"dynamodb:BatchWriteItem",
				},
				Resource: policy.ARNAll,
			},
			{
				Sid:    "NoRoleMinting",
				Effect: policy.Deny,
				Action: policy.Actions{
					"iam:CreateRole", "iam:PutRolePolicy", "iam:AttachRolePolicy",
					"iam:UpdateAssumeRolePolicy", "iam:CreatePolicy", "iam:CreatePolicyVersion",
					"iam:PutRolePermissionsBoundary", "iam:DeleteRolePermissionsBoundary",
				},
				Resource: policy.ARNAll,
			},
			{
				Sid:      "NoStandingCharges",
				Effect:   policy.Deny,
				Action:   policy.Actions{"lambda:PutProvisionedConcurrencyConfig"},
				Resource: policy.ARNAll,
			},
		},
	}
}

// --- AWS::IAM::Role ----------------------------------------------------------

type iamRole struct {
	Arn                      *string          `json:"Arn,omitempty"`
	RoleName                 *string          `json:"RoleName,omitempty"`
	Description              *string          `json:"Description,omitempty"`
	AssumeRolePolicyDocument json.RawMessage  `json:"AssumeRolePolicyDocument,omitempty"`
	Policies                 []iamRolePolicy  `json:"Policies,omitempty"`
	Tags                     []iamResourceTag `json:"Tags,omitempty"`
}

func (iamRole) CloudJamType() string { return "AWS::IAM::Role" }

type iamRolePolicy struct {
	PolicyName     *string         `json:"PolicyName,omitempty"`
	PolicyDocument json.RawMessage `json:"PolicyDocument,omitempty"`
}

type iamResourceTag struct {
	Key   *string `json:"Key,omitempty"`
	Value *string `json:"Value,omitempty"`
}

func roleArn(identifier string) string {
	if identifier == "" || strings.HasPrefix(identifier, "arn:") {
		return identifier
	}
	role, err := aws.Read[*iamRole](identifier)
	if err != nil || role == nil || role.Arn == nil || *role.Arn == "" {
		return identifier
	}
	return *role.Arn
}

func assumedBy(service string) json.RawMessage {
	return json.RawMessage(policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{
				Effect:    policy.Allow,
				Principal: policy.PrincipalService(service),
				Action:    policy.Actions{"sts:AssumeRole"},
			},
		},
	}.String())
}

// --- reaching fakecloud from inside its own sandbox -------------------------

const fakeCloudAccountID = "123456789012"

// localEndpointOverride: see meridian-farebox for the full rationale. The
// account id check is what keeps this from ever firing against a real
// account — generatorRole's arn carries a real account id there, and this
// condition is never true.
func localEndpointOverride() map[string]string {
	if accountFromArn(generatorRole) != fakeCloudAccountID {
		return nil
	}
	return map[string]string{"AWS_ENDPOINT_URL": "http://host.docker.internal:4566"}
}

func accountFromArn(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) < 5 {
		return ""
	}
	return parts[4]
}

func withEnv(base, override map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(override))
	maps.Copy(merged, base)
	maps.Copy(merged, override)
	return merged
}

// --- bootstrap ---------------------------------------------------------------

func parallel(steps ...func() error) error {
	failures := make([]error, len(steps))
	wait := sync.WaitGroup{}
	for index, step := range steps {
		wait.Go(func() { failures[index] = step() })
	}
	wait.Wait()
	return errors.Join(failures...)
}

func bootstrap(s *challenge.Scenario) error {
	baseline()

	run := uuid.NewString()

	if err := parallel(
		func() error { return makeArchive(run) },
		func() error { return makeDecoyBucket(run) },
		func() error { return makeVipStream(run) },
		func() error { return makeGeneralStream(run) },
		func() error { return makePressStream(run) },
		func() error { return makeLedger(run) },
		func() error { return makeLogGroup(run) },
		func() error { return makeTelemetry() },
		func() error { return makeHandover() },
	); err != nil {
		return err
	}

	if err := parallel(
		func() error { return makeAdmissionsRole(run) },
		func() error { return makeGeneratorRole(run) },
	); err != nil {
		return err
	}

	if err := makeGeneratorFunction(run); err != nil {
		return err
	}
	if err := makeSchedule(run); err != nil {
		return err
	}
	if err := makeInvokePermission(); err != nil {
		return err
	}

	s.AddAsset("fenwick-handover.md", []byte(handoverAsset()))
	if pkg := pipelinePackage(); len(pkg) > 0 {
		s.AddAsset("fenwick-admissions.zip", pkg)
	}

	s.SetPermission(policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{Sid: "GateFanIn", Effect: policy.Allow, Action: workingSet(), Resource: policy.ARNAll},
		},
	})
	return nil
}

func makeArchive(run string) error {
	bucket, err := aws.Create(&s3.Bucket{
		BucketName: new(fmt.Sprintf("fenwick-gate-photos-%s", run)),
		PublicAccessBlockConfiguration: &s3.BucketPublicAccessBlockConfiguration{
			BlockPublicAcls:       new(false),
			BlockPublicPolicy:     new(false),
			IgnorePublicAcls:      new(false),
			RestrictPublicBuckets: new(false),
		},
		Tags: []s3.BucketTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("gate photo archive: %w", err)
	}
	archiveRef = bucket
	return nil
}

func makeDecoyBucket(run string) error {
	bucket, err := aws.Create(&s3.Bucket{
		BucketName: new(fmt.Sprintf("fenwick-merch-inventory-%s", run)),
		Tags:       []s3.BucketTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("merch inventory bucket: %w", err)
	}
	decoyRef = bucket
	return nil
}

// makeVipStream is the scenario: one shard, scoped to what a quiet night
// needs and nothing like what a sold-out one does.
func makeVipStream(run string) error {
	name := fmt.Sprintf("fenwick-vip-%s", run)
	stream, err := aws.Create(&kinesis.Stream{
		Name:       new(name),
		ShardCount: new(1),
		Tags:       []kinesis.Tag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("vip stream: %w", err)
	}
	vipStreamRef = stream
	if live, err := aws.Read[*kinesis.Stream](vipStreamRef); err == nil && live != nil && live.Arn != nil {
		vipStreamArn = *live.Arn
	}
	return nil
}

func makeGeneralStream(run string) error {
	name := fmt.Sprintf("fenwick-general-%s", run)
	stream, err := aws.Create(&kinesis.Stream{
		Name:       new(name),
		ShardCount: new(2),
		Tags:       []kinesis.Tag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("general stream: %w", err)
	}
	if live, err := aws.Read[*kinesis.Stream](stream); err == nil && live != nil && live.Arn != nil {
		generalStreamArn = *live.Arn
	}
	generalStreamRefGlobal = stream
	return nil
}

func makePressStream(run string) error {
	name := fmt.Sprintf("fenwick-press-%s", run)
	stream, err := aws.Create(&kinesis.Stream{
		Name:       new(name),
		ShardCount: new(1),
		Tags:       []kinesis.Tag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("press stream: %w", err)
	}
	if live, err := aws.Read[*kinesis.Stream](stream); err == nil && live != nil && live.Arn != nil {
		pressStreamArn = *live.Arn
	}
	pressStreamRefGlobal = stream
	return nil
}

// generalStreamRefGlobal / pressStreamRefGlobal hold the primary identifiers
// for ownership checks; vipStreamRef already serves that role for VIP.
var (
	generalStreamRefGlobal string
	pressStreamRefGlobal   string
)

func makeLedger(run string) error {
	name := fmt.Sprintf("fenwick-ledger-%s", run)
	table, err := aws.Create(&dynamodb.Table{
		TableName: new(name),
		AttributeDefinitions: []dynamodb.TableAttributeDefinition{
			{AttributeName: new("scan_id"), AttributeType: new("S")},
		},
		KeySchema:   json.RawMessage(`[{"AttributeName":"scan_id","KeyType":"HASH"}]`),
		BillingMode: new("PAY_PER_REQUEST"),
		Tags:        []dynamodb.TableTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("admissions ledger: %w", err)
	}
	ledgerRef = table
	ledgerLabel = name
	if live, err := aws.Read[*dynamodb.Table](ledgerRef); err == nil && live != nil {
		if live.TableName != nil && *live.TableName != "" {
			ledgerLabel = *live.TableName
		}
		if live.Arn != nil {
			ledgerArn = *live.Arn
		}
	}
	return nil
}

func makeLogGroup(run string) error {
	group, err := aws.Create(&logs.LogGroup{
		LogGroupName: new(fmt.Sprintf("/fenwick/admissions/%s", run)),
		Tags:         []logs.LogGroupTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("admissions logs: %w", err)
	}
	logGroupRef = group
	return nil
}

func makeTelemetry() error {
	if _, err := aws.Create(&ssm.Parameter{
		Name:        new(telemetryPath),
		Type:        new(ssm.ParameterTypeString),
		Description: new("gate telemetry - written by the gate network"),
		Value:       new("window=0 sent=0 verified=0 duplicate_landed=0 vip_throttled=0 rate=0"),
	}); err != nil {
		return fmt.Errorf("telemetry parameter: %w", err)
	}
	return nil
}

func makeHandover() error {
	if _, err := aws.Create(&ssm.Parameter{
		Name:        new(handoverPath),
		Type:        new(ssm.ParameterTypeString),
		Description: new("handover note"),
		Value:       new(handoverNote()),
	}); err != nil {
		return fmt.Errorf("handover note: %w", err)
	}
	return nil
}

func scopes() (streams policy.ARNs, ledger policy.ARNs) {
	streams = policy.ARNs{}
	for _, arn := range []string{vipStreamArn, generalStreamArn, pressStreamArn} {
		if arn != "" {
			streams = append(streams, policy.ARN(arn))
		}
	}
	if len(streams) == 0 {
		streams = policy.ARNAll
	}
	ledger = policy.ARNAll
	if ledgerArn != "" {
		ledger = policy.ARNs{policy.ARN(ledgerArn)}
	}
	return streams, ledger
}

func makeAdmissionsRole(run string) error {
	streams, ledger := scopes()
	role, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("fenwick-admissions-%s", run)),
		Description:              new("execution role for the admissions consumer"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("admissions"),
			PolicyDocument: json.RawMessage(policy.Document{
				Version: policy.Version20121017,
				Statement: []policy.Statement{
					{
						Effect: policy.Allow,
						Action: policy.Actions{
							"kinesis:GetRecords", "kinesis:GetShardIterator", "kinesis:DescribeStream",
							"kinesis:DescribeStreamSummary", "kinesis:ListShards", "kinesis:SubscribeToShard",
						},
						Resource: streams,
					},
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"dynamodb:UpdateItem", "dynamodb:DescribeTable"},
						Resource: ledger,
					},
					{
						Effect: policy.Allow,
						Action: policy.Actions{
							"logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents",
						},
						Resource: policy.ARNAll,
					},
				},
			}.String()),
		}},
		Tags: []iamResourceTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("admissions role: %w", err)
	}
	admissionsArn = roleArn(role)
	return nil
}

func makeGeneratorRole(run string) error {
	streams, ledger := scopes()
	role, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("%srole-%s", gatePrefix, run)),
		Description:              new("execution role for the gate network - scenario owned"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("gatenetwork"),
			PolicyDocument: json.RawMessage(policy.Document{
				Version: policy.Version20121017,
				Statement: []policy.Statement{
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"kinesis:PutRecord", "kinesis:PutRecords", "kinesis:DescribeStreamSummary"},
						Resource: streams,
					},
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"dynamodb:Scan", "dynamodb:DescribeTable"},
						Resource: ledger,
					},
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"ssm:GetParameter", "ssm:PutParameter"},
						Resource: policy.ARNs{policy.ARN("arn:aws:ssm:*:*:parameter" + telemetryPath)},
					},
					{
						Effect: policy.Allow,
						Action: policy.Actions{
							"logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents",
						},
						Resource: policy.ARNAll,
					},
				},
			}.String()),
		}},
		Tags: []iamResourceTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("generator role: %w", err)
	}
	generatorRole = roleArn(role)
	return nil
}

func generatorEnvironment(surge int) *lambda.Environment {
	return &lambda.Environment{Variables: withEnv(map[string]string{
		"LEDGER_TABLE":      ledgerLabel,
		"TALLY_PARAM":       telemetryPath,
		"VIP_STREAM":        vipStreamRef,
		"GENERAL_STREAM":    generalStreamRefGlobal,
		"PRESS_STREAM":      pressStreamRefGlobal,
		"BASE_RATE":         strconv.Itoa(baseRate),
		"PEAK_RATE":         strconv.Itoa(peakRate),
		"RAMP_WINDOWS":      strconv.Itoa(rampWindows),
		"VIP_SURGE_WINDOWS": strconv.Itoa(surge),
	}, localEndpointOverride())}
}

func makeGeneratorFunction(run string) error {
	bucket, key, err := uploadLambdaCode(generatorSource)
	if err != nil {
		return fmt.Errorf("gate network code: %w", err)
	}
	definition := &lambda.Function{
		FunctionName: new(fmt.Sprintf("%snetwork-%s", gatePrefix, run)),
		Description:  new("fenwick gate network and admissions telemetry - do not disable"),
		Runtime:      new("python3.12"),
		Handler:      new("index.handler"),
		Role:         new(generatorRole),
		Timeout:      new(120),
		MemorySize:   new(256),
		Code:         &lambda.Code{S3Bucket: new(bucket), S3Key: new(key)},
		Environment:  generatorEnvironment(0),
		Tags:         []lambda.FunctionTag{{Key: new(systemTag), Value: new(systemName)}},
	}

	for attempt := range roleRetries {
		var function string
		if function, err = aws.Create(definition); err == nil {
			generatorRef = function
			functionBaseline[generatorRef] = true
			return nil
		}
		slog.Warn(fmt.Sprintf("gate network attempt %d/%d: %v", attempt+1, roleRetries, err))
		time.Sleep(roleRetryDelay)
	}
	return fmt.Errorf("gate network: %w", err)
}

func makeSchedule(run string) error {
	rule, err := aws.Create(&events.Rule{
		Name:               new(fmt.Sprintf("%sschedule-%s", gatePrefix, run)),
		Description:        new("sends the next window of scans, once a minute"),
		ScheduleExpression: new("rate(1 minute)"),
		State:              new(events.RuleStateENABLED),
		Targets: []events.Target{{
			Id:  new("gate-network"),
			Arn: new(functionArn(generatorRef)),
		}},
	})
	if err != nil {
		return fmt.Errorf("gate network schedule: %w", err)
	}
	scheduleRef = rule
	return nil
}

func makeInvokePermission() error {
	if _, err := aws.Create(&lambda.Permission{
		FunctionName: new(generatorRef),
		Action:       new("lambda:InvokeFunction"),
		Principal:    new("events.amazonaws.com"),
		SourceArn:    new(ruleArn(scheduleRef)),
	}); err != nil {
		return fmt.Errorf("gate network invoke permission: %w", err)
	}
	return nil
}

func functionArn(identifier string) string {
	if identifier == "" || strings.HasPrefix(identifier, "arn:") {
		return identifier
	}
	function, err := aws.Read[*lambda.Function](identifier)
	if err != nil || function == nil || function.Arn == nil || *function.Arn == "" {
		return identifier
	}
	return *function.Arn
}

func ruleArn(identifier string) string {
	if identifier == "" || strings.HasPrefix(identifier, "arn:") {
		return identifier
	}
	rule, err := aws.Read[*events.Rule](identifier)
	if err != nil || rule == nil || rule.Arn == nil || *rule.Arn == "" {
		return identifier
	}
	return *rule.Arn
}

func baseline() {
	_ = parallel(
		func() error { return record[*kms.Key](keyBaseline) },
		func() error { return record[*lambda.Function](functionBaseline) },
	)
}

func record[T aws.Resource](into map[string]bool) error {
	found, err := aws.List[T]()
	if err != nil {
		return err
	}
	for identifier := range found {
		into[identifier] = true
	}
	return nil
}

func handoverNote() string {
	return strings.Join([]string{
		"fenwick admissions - handover. Turnstile's contract ended nine days ago.",
		"",
		"Flow is: three gate clusters -> three Kinesis streams (VIP, General, Press) -> one",
		"admissions consumer -> the ledger. Everything Turnstile built is tagged " +
			systemTag + "=" + systemName + ".",
		"",
		"The consumer went with them. Platform kept a copy - it is not a Lambda that",
		"exists yet, and the execution role for it is already in this account, scoped to",
		"all three streams and the ledger.",
		"",
		"scan_id identifies one physical scan. It is not the Kinesis record and not the",
		"delivery - two records with the same scan_id are the same scan no matter how far",
		"apart they land.",
		"",
		"Telemetry writes to " + telemetryPath + " every minute: scans sent, admitted",
		"cleanly, and admitted more than once. The third number has never been zero.",
		"",
		"Known and not fixed: the gate photo archive is wide open with no versioning, the",
		"ledger has no PITR, and the VIP stream has one shard. It has always had one shard.",
		"Nobody has ever booked an act this big before.",
	}, "\n")
}

func handoverAsset() string {
	return "# Fenwick Admissions — handover\n\n```\n" + handoverNote() + "\n```\n"
}

// --- Act I -------------------------------------------------------------------

func streamsOwned() (bool, error) {
	for _, ref := range []string{vipStreamRef, generalStreamRefGlobal, pressStreamRefGlobal} {
		stream, err := readStream(ref)
		if err != nil {
			return false, err
		}
		if !streamOwned(stream) {
			return false, nil
		}
	}
	return true, nil
}

func streamOwned(stream *kinesis.Stream) bool {
	for _, tag := range stream.Tags {
		if tag.Key != nil && *tag.Key == ownerTag && tag.Value != nil && *tag.Value != "" {
			return true
		}
	}
	return false
}

func ledgerOwned() (bool, error) {
	table, err := readLedger()
	if err != nil {
		return false, err
	}
	for _, tag := range table.Tags {
		if tag.Key != nil && *tag.Key == ownerTag && tag.Value != nil && *tag.Value != "" {
			return true, nil
		}
	}
	return false, nil
}

func archiveOwned() (bool, error) { return bucketOwned(archiveRef, "gate photo archive") }
func decoyOwned() (bool, error)   { return bucketOwned(decoyRef, "merch inventory bucket") }

func bucketOwned(ref, label string) (bool, error) {
	bucket, err := readBucket(ref, label)
	if err != nil {
		return false, err
	}
	for _, tag := range bucket.Tags {
		if tag.Key != nil && *tag.Key == ownerTag && tag.Value != nil && *tag.Value != "" {
			return true, nil
		}
	}
	return false, nil
}

func handoverConfirmed() (bool, error) {
	value, ok, err := parameter(handoverPath)
	if err != nil || !ok {
		return false, err
	}
	return strings.TrimSpace(value) != strings.TrimSpace(handoverNote()), nil
}

func logsRetained() (bool, error) {
	group, err := readLogGroup()
	if err != nil {
		return false, err
	}
	if group.RetentionInDays == nil {
		return false, nil
	}
	days := *group.RetentionInDays
	return days >= minRetentionDays && days <= maxRetentionDays, nil
}

// --- Act II: get admissions moving -------------------------------------------

func admissionsDeployed() (bool, error) {
	functions, err := newFunctions()
	if err != nil {
		return false, err
	}
	return len(functions) > 0, nil
}

func vipWired() (bool, error)     { return streamWired(vipStreamArn) }
func generalWired() (bool, error) { return streamWired(generalStreamArn) }
func pressWired() (bool, error)   { return streamWired(pressStreamArn) }

func streamWired(streamArn string) (bool, error) {
	if streamArn == "" {
		return false, fmt.Errorf("stream arn was never resolved")
	}
	mappings, err := aws.List[*lambda.EventSourceMapping]()
	if err != nil {
		return false, err
	}
	for _, mapping := range mappings {
		if mapping == nil || mapping.EventSourceArn == nil || mapping.FunctionName == nil {
			continue
		}
		if *mapping.EventSourceArn != streamArn {
			continue
		}
		if mapping.Enabled != nil && !*mapping.Enabled {
			continue
		}
		if !isGateNetwork(*mapping.FunctionName) {
			return true, nil
		}
	}
	return false, nil
}

func isGateNetwork(reference string) bool {
	return strings.Contains(reference, gatePrefix)
}

// failureDestinationArmed wants an event source mapping onto a player
// function to carry a DestinationConfig.OnFailure — Kinesis' equivalent of a
// dead letter queue, set on the mapping rather than the stream because a
// stream has no redrive policy of its own.
func failureDestinationArmed() (bool, error) {
	mappings, err := aws.List[*lambda.EventSourceMapping]()
	if err != nil {
		return false, err
	}
	for identifier := range mappings {
		live, err := aws.Read[*lambda.EventSourceMapping](identifier)
		if err != nil || live == nil || live.FunctionName == nil {
			continue
		}
		if isGateNetwork(*live.FunctionName) {
			continue
		}
		if live.DestinationConfig != nil && live.DestinationConfig.OnFailure != nil &&
			live.DestinationConfig.OnFailure.Destination != nil &&
			*live.DestinationConfig.OnFailure.Destination != "" {
			return true, nil
		}
	}
	return false, nil
}

func admissionsClearing() func() (bool, error) {
	last := -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if last < 0 {
			last = state.verified
			return false, nil
		}
		if state.verified <= last {
			return false, nil
		}
		last = state.verified
		return true, nil
	}
}

func admittedExactlyOnce() func() (bool, error) {
	lastVerified, lastDup := -1, -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if lastVerified < 0 {
			lastVerified, lastDup = state.verified, state.duplicateLanded
			return false, nil
		}
		verified := state.verified - lastVerified
		dup := state.duplicateLanded - lastDup
		lastVerified, lastDup = state.verified, state.duplicateLanded
		return verified > 0 && dup <= 0, nil
	}
}

func duplicatesLanding() func() (bool, error) {
	last := -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if last < 0 {
			last = state.duplicateLanded
			return false, nil
		}
		if state.duplicateLanded <= last {
			return false, nil
		}
		last = state.duplicateLanded
		return true, nil
	}
}

func firstCleanAdmission() func() (bool, error) {
	return func() (bool, error) {
		if selloutStarted.Load() {
			return true, nil
		}
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		return state.verified > 0, nil
	}
}

// --- Act III: the sellout -----------------------------------------------------

var surgeFloor atomic.Int64

func theSellout(ctx context.Context, s *challenge.Scenario) error {
	if !selloutStarted.CompareAndSwap(false, true) {
		return nil
	}

	if state, ok, err := readTelemetry(); err == nil && ok {
		surgeFloor.Store(int64(state.window))
	}

	s.AddDescription(
		"An hour to doors. The VIP list for tonight is the biggest Fenwick has ever run, and " +
			"the ops director just found out the stream it runs on has one shard — the same " +
			"one shard it has had since Turnstile built this, because nobody has ever booked " +
			"an act this size before. If it cannot keep up, the VIP line stops moving and four " +
			"hundred people who paid the most stand outside the longest.")
	s.AddDescription(
		"Telemetry will start showing vip_throttled climbing the moment the surge outruns " +
			"what one shard can carry. A stream's shard count is yours to change; it is not " +
			"something Turnstile locked behind anything you do not already have.")

	s.AddClue("what actually fixes a hot shard",
		"More shards, and only more shards — Kinesis routes by partition key hash to a "+
			"fixed shard mapping, so nothing about your consumer's code changes what one "+
			"shard can carry. UpdateShardCount on the VIP stream is the whole fix.",
		-30)
	s.AddClue("what she will want in writing",
		"Put the incident report in "+incidentPath+". It has to quote two things: the arn "+
			"of the failure destination your admissions consumer redirects rejected scans "+
			"to, and the id of the key the gate photo archive is encrypted under.",
		-20)

	s.AddCheck("Scaled the VIP stream for the sellout", challenge.Check{
		Points:  90,
		Every:   20 * time.Second,
		Trigger: vipScaled,
	})
	s.AddCheck("VIP gate throttling scans", challenge.Check{
		Points:  throttlePoints,
		Every:   30 * time.Second,
		Repeat:  true,
		Trigger: bounded(throttleRounds, vipThrottling()),
	})
	s.AddCheck("Put the gate photo archive under a company key", challenge.Check{
		Points:  55,
		Every:   20 * time.Second,
		Trigger: archiveEncrypted,
	})
	s.AddCheck("Took the gate photo archive off the public internet", challenge.Check{
		Points:  45,
		Every:   20 * time.Second,
		Trigger: archiveClosed,
	})
	s.AddCheck("Turned on versioning for the gate photo archive", challenge.Check{
		Points:  30,
		Every:   20 * time.Second,
		Trigger: archiveVersioned,
	})
	s.AddCheck("Made the admissions ledger recoverable", challenge.Check{
		Points:  45,
		Every:   20 * time.Second,
		Trigger: ledgerRecoverable,
	})
	s.AddCheck("Protected the admissions ledger from deletion", challenge.Check{
		Points:  25,
		Every:   20 * time.Second,
		Trigger: ledgerProtected,
	})
	s.AddCheck("Filed the incident report", challenge.Check{
		Points:  55,
		Every:   20 * time.Second,
		Trigger: incidentReportFiled,
	})

	if generatorRef == "" {
		return fmt.Errorf("gate network was never provisioned")
	}
	if err := aws.Update(generatorRef, &lambda.Function{
		Environment: generatorEnvironment(1),
	}); err != nil {
		return fmt.Errorf("switch gate network to surge: %w", err)
	}
	slog.Info("vip surge enabled on the gate network")
	return nil
}

func vipScaled() (bool, error) {
	stream, err := readStream(vipStreamRef)
	if err != nil {
		return false, err
	}
	if stream.ShardCount == nil {
		return false, nil
	}
	return *stream.ShardCount >= vipShardTarget, nil
}

func vipThrottling() func() (bool, error) {
	lastVerified, lastThrottled := -1, -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if int64(state.window) < surgeFloor.Load()+surgeGrace {
			return false, nil
		}
		if lastVerified < 0 {
			lastVerified, lastThrottled = state.verified, state.vipThrottled
			return false, nil
		}
		throttled := state.vipThrottled - lastThrottled
		lastVerified, lastThrottled = state.verified, state.vipThrottled
		return throttled > 0, nil
	}
}

func archiveEncrypted() (bool, error) {
	bucket, err := readBucket(archiveRef, "gate photo archive")
	if err != nil {
		return false, err
	}
	if bucket.BucketEncryption == nil {
		return false, nil
	}
	for _, rule := range bucket.BucketEncryption.ServerSideEncryptionConfiguration {
		if rule.ServerSideEncryptionByDefault == nil {
			continue
		}
		algorithm := rule.ServerSideEncryptionByDefault.SSEAlgorithm
		if algorithm != nil && *algorithm == s3.ServerSideEncryptionByDefaultSSEAlgorithmAwsKms {
			return true, nil
		}
	}
	return false, nil
}

func archiveClosed() (bool, error) {
	bucket, err := readBucket(archiveRef, "gate photo archive")
	if err != nil {
		return false, err
	}
	block := bucket.PublicAccessBlockConfiguration
	if block == nil {
		return false, nil
	}
	for _, flag := range []*bool{
		block.BlockPublicAcls, block.BlockPublicPolicy,
		block.IgnorePublicAcls, block.RestrictPublicBuckets,
	} {
		if flag == nil || !*flag {
			return false, nil
		}
	}
	return true, nil
}

func archiveVersioned() (bool, error) {
	bucket, err := readBucket(archiveRef, "gate photo archive")
	if err != nil {
		return false, err
	}
	if bucket.VersioningConfiguration == nil || bucket.VersioningConfiguration.Status == nil {
		return false, nil
	}
	return *bucket.VersioningConfiguration.Status == s3.VersioningConfigurationStatusEnabled, nil
}

func ledgerRecoverable() (bool, error) {
	table, err := readLedger()
	if err != nil {
		return false, err
	}
	spec := table.PointInTimeRecoverySpecification
	if spec == nil || spec.PointInTimeRecoveryEnabled == nil {
		return false, nil
	}
	return *spec.PointInTimeRecoveryEnabled, nil
}

func ledgerProtected() (bool, error) {
	table, err := readLedger()
	if err != nil {
		return false, err
	}
	if table.DeletionProtectionEnabled == nil {
		return false, nil
	}
	return *table.DeletionProtectionEnabled, nil
}

func incidentReportFiled() (bool, error) {
	value, ok, err := parameter(incidentPath)
	if err != nil || !ok {
		return false, err
	}
	destination, err := failureDestinationArn()
	if err != nil {
		return false, err
	}
	if destination == "" || !strings.Contains(value, destination) {
		return false, nil
	}
	keys, err := newKeys()
	if err != nil {
		return false, err
	}
	for identifier, key := range keys {
		id := identifier
		if key != nil && key.KeyId != nil && *key.KeyId != "" {
			id = *key.KeyId
		}
		if id != "" && strings.Contains(value, id) {
			return true, nil
		}
	}
	return false, nil
}

func failureDestinationArn() (string, error) {
	mappings, err := aws.List[*lambda.EventSourceMapping]()
	if err != nil {
		return "", err
	}
	for identifier := range mappings {
		live, err := aws.Read[*lambda.EventSourceMapping](identifier)
		if err != nil || live == nil || live.FunctionName == nil || isGateNetwork(*live.FunctionName) {
			continue
		}
		if live.DestinationConfig != nil && live.DestinationConfig.OnFailure != nil &&
			live.DestinationConfig.OnFailure.Destination != nil {
			return *live.DestinationConfig.OnFailure.Destination, nil
		}
	}
	return "", nil
}

// --- the coda ------------------------------------------------------------------

func selloutClosedOut() (bool, error) {
	if !selloutStarted.Load() {
		return false, nil
	}
	if codaOpen.Load() {
		return true, nil
	}
	filed, err := incidentReportFiled()
	if err != nil || !filed {
		return false, err
	}
	return archiveEncrypted()
}

func theLastCall(ctx context.Context, s *challenge.Scenario) error {
	if !codaOpen.CompareAndSwap(false, true) {
		return nil
	}

	s.AddDescription(
		"Doors close soon and the incident report is filed. There is nothing left to build. " +
			"Points accrue for every window that admits cleanly and keeps up with the gates, " +
			"and they stop the moment either stops being true.")

	s.AddCheck("Kept admissions exact", challenge.Check{
		Points:  heldPoints,
		Every:   30 * time.Second,
		Repeat:  true,
		Trigger: bounded(heldRounds, admittedExactlyOnce()),
	})
	s.AddCheck("Kept up with the gates", challenge.Check{
		Points:  codaPoints,
		Every:   30 * time.Second,
		Repeat:  true,
		Trigger: bounded(codaRounds, keepingUp()),
	})
	return nil
}

func keepingUp() func() (bool, error) {
	lastSent, lastVerified := -1, -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if lastSent < 0 {
			lastSent, lastVerified = state.sent, state.verified
			return false, nil
		}
		sent, verified := state.sent-lastSent, state.verified-lastVerified
		lastSent, lastVerified = state.sent, state.verified
		if sent <= 0 {
			return false, nil
		}
		return verified*1000 >= sent*keepingUpPerMille, nil
	}
}

func bounded(rounds int, trigger func() (bool, error)) func() (bool, error) {
	fired := 0
	return func() (bool, error) {
		if fired >= rounds {
			return false, nil
		}
		passed, err := trigger()
		if err != nil || !passed {
			return false, err
		}
		fired++
		return true, nil
	}
}

// --- reads ---------------------------------------------------------------------

func readStream(ref string) (*kinesis.Stream, error) {
	if ref == "" {
		return nil, fmt.Errorf("stream was never provisioned")
	}
	stream, err := aws.Read[*kinesis.Stream](ref)
	if err != nil {
		return nil, err
	}
	if stream == nil {
		return nil, fmt.Errorf("stream is not readable")
	}
	return stream, nil
}

func readLedger() (*dynamodb.Table, error) {
	if ledgerRef == "" {
		return nil, fmt.Errorf("admissions ledger was never provisioned")
	}
	table, err := aws.Read[*dynamodb.Table](ledgerRef)
	if err != nil {
		return nil, err
	}
	if table == nil {
		return nil, fmt.Errorf("admissions ledger is not readable")
	}
	return table, nil
}

func readBucket(ref, label string) (*s3.Bucket, error) {
	if ref == "" {
		return nil, fmt.Errorf("%s was never provisioned", label)
	}
	bucket, err := aws.Read[*s3.Bucket](ref)
	if err != nil {
		return nil, err
	}
	if bucket == nil {
		return nil, fmt.Errorf("%s is not readable", label)
	}
	return bucket, nil
}

func readLogGroup() (*logs.LogGroup, error) {
	if logGroupRef == "" {
		return nil, fmt.Errorf("admissions logs were never provisioned")
	}
	group, err := aws.Read[*logs.LogGroup](logGroupRef)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("admissions logs are not readable")
	}
	return group, nil
}

type telemetry struct {
	window          int
	sent            int
	verified        int
	duplicateLanded int
	vipThrottled    int
	rate            int
}

func readTelemetry() (telemetry, bool, error) {
	value, ok, err := parameter(telemetryPath)
	if err != nil || !ok {
		return telemetry{}, false, err
	}
	state := telemetry{}
	for field := range strings.FieldsSeq(value) {
		key, raw, found := strings.Cut(field, "=")
		if !found {
			continue
		}
		number, err := strconv.Atoi(raw)
		if err != nil {
			continue
		}
		switch key {
		case "window":
			state.window = number
		case "sent":
			state.sent = number
		case "verified":
			state.verified = number
		case "duplicate_landed":
			state.duplicateLanded = number
		case "vip_throttled":
			state.vipThrottled = number
		case "rate":
			state.rate = number
		}
	}
	return state, true, nil
}

func parameter(path string) (string, bool, error) {
	parameters, err := aws.List[*ssm.Parameter]()
	if err != nil {
		return "", false, err
	}
	found, ok := parameters[path]
	if !ok {
		return "", false, nil
	}
	if found != nil && found.Value != nil && *found.Value != "" {
		return *found.Value, true, nil
	}
	live, err := aws.Read[*ssm.Parameter](path)
	if err != nil {
		return "", false, err
	}
	if live == nil || live.Value == nil {
		return "", false, nil
	}
	return *live.Value, true, nil
}

func newKeys() (map[string]*kms.Key, error) { return appeared[*kms.Key](keyBaseline) }

func newFunctions() (map[string]*lambda.Function, error) {
	return appeared[*lambda.Function](functionBaseline)
}

func appeared[T aws.Resource](baseline map[string]bool) (map[string]T, error) {
	found, err := aws.List[T]()
	if err != nil {
		return nil, err
	}
	fresh := map[string]T{}
	for identifier := range found {
		if baseline[identifier] {
			continue
		}
		live, err := aws.Read[T](identifier)
		if err != nil {
			continue
		}
		fresh[identifier] = live
	}
	return fresh, nil
}
