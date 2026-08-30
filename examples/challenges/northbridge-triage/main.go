//go:build wasip1

// Command northbridge-triage is a gameday tier cloudjam challenge.
//
// Northbridge Mutual adjudicates property claims through a Step Functions
// state machine: one Task state calling one Lambda. Whoever built it left
// mid-project — the state machine points at a function that was never
// deployed, and it has no error handling at all. The player's job is not to
// fix broken code; the one Lambda in this pipeline is already correct. The
// job is to finish an unfinished state machine: wire the Task at a real
// function, and give it somewhere to go when that function legitimately
// rejects a claim, because right now "legitimately rejects" and "vanishes
// without a trace" are the same outcome.
//
// There is no queue and no ledger table. The durable record this pipeline
// is scored against is Step Functions' own execution history — a different
// sink and a different read path than every other challenge in this
// codebase, on purpose.
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
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/events"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/kms"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/lambda"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/logs"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/s3"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/ssm"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/stepfunctions"
	"github.com/google/uuid"
)

const (
	ownerTag   = "northbridge:owner"
	systemTag  = "northbridge:system"
	systemName = "triage"
)

const gatePrefix = "northbridge-desk-"

const (
	handoverPath  = "/northbridge/handover"
	telemetryPath = "/northbridge/telemetry"
	incidentPath  = "/northbridge/incident-report"
)

const (
	roleRetries    = 6
	roleRetryDelay = 5 * time.Second
)

const (
	minRetentionDays = 30
	maxRetentionDays = 400
)

const (
	baseRate      = 12
	peakRate      = 45
	rampWindows   = 16
	riskyPerMille = 120
)

const (
	throughputPoints = 10
	throughputRounds = 10
	failedRounds     = 8
	failedPoints     = -18
	reviewedRounds   = 6
	reviewedPoints   = 10
	heldRounds       = 8
	heldPoints       = 8
	codaRounds       = 6
	codaPoints       = 10
)

const (
	keepingUpPerMille = 500
	filingGrace       = 2
)

var (
	archiveRef      string
	decoyRef        string
	stateMachineRef string
	stateMachineArn string
	logGroupRef     string
	adjudicatorArn  string
	generatorRole   string
	generatorRef    string
	scheduleRef     string
)

var (
	keyBaseline      = map[string]bool{}
	functionBaseline = map[string]bool{}
)

var (
	filingStarted atomic.Bool
	codaOpen      atomic.Bool
)

func main() {
	challenge.New("Northbridge Mutual: Claims Triage", 10*time.Second, bootstrap).
		AddDescription(
			"Northbridge Mutual adjudicates property claims through a single Step Functions "+
				"state machine: one task, one Lambda, one decision — pay it, or don't. Whoever "+
				"built it left partway through. The state machine points at a function that "+
				"was never deployed, and there is no error handling anywhere in it.").
		AddDescription(
			"Claims have not stopped arriving because the pipeline is unfinished. Every claim "+
				"that would have been correctly rejected — an amount too small to bother "+
				"filing, or too large for this desk to clear without a human looking at it — "+
				"is not going to a reviewer. It is going nowhere. Nobody downstream has "+
				"noticed yet, because nothing downstream gets told when an execution just "+
				"fails.").
		AddDescription(
			"You are the engineer who inherited this. There is no architecture diagram and "+
				"no runbook — everything here carries a "+systemTag+"="+systemName+" tag, and "+
				"that tag is the entire asset register.").
		AddDescription(
			"The claims desk reports what it sees to "+telemetryPath+" once a minute: claims "+
				"filed, claims settled, claims sent to manual review, and claims that simply "+
				"failed with nowhere to go. That parameter is your dashboard. You can read it. "+
				"You cannot write it.").
		AddDescription(
			"Nobody is handing you a task list. You are scored the way the claims desk is "+
				"measured: on claims correctly settled, against claims that failed and were "+
				"never seen by anyone. There is more here than there is time to finish before "+
				"the next filing deadline.").
		AddClue("where do i start",
			"The handover note is an SSM parameter at "+handoverPath+". Read it first: "+
				"aws ssm get-parameter --name "+handoverPath+".",
			-5).
		AddClue("the state machine points at nothing",
			"The one Lambda this pipeline needs is not a bug fix — platform's copy of it is "+
				"attached to this challenge as a downloadable package, and it is already "+
				"correct. Deploying it and pointing the state machine's Task at it is the "+
				"first job.",
			-30).
		AddClue("i cannot create an execution role",
			"You do not have iam:CreateRole and are not meant to. The adjudicator's execution "+
				"role is already provisioned. Find it with aws iam list-roles.",
			-20).
		AddClue("what counts as ownership",
			"The asset register only counts resources carrying a "+ownerTag+" tag. Any value "+
				"works.",
			-20).
		AddClue("claims are settling but the failure count is climbing",
			"Read what the adjudicator does when it rejects a claim, then read what the "+
				"state machine does when a task raises. It does not retry and it does not "+
				"catch. Both of those are choices you can make in the definition, not the "+
				"code.",
			-40).
		AddClue("what is an uncaught failure worth",
			"Considerably less than nothing — a claim a human should have reviewed, gone "+
				"with no record at all. The scoreboard prices it the way an auditor would.",
			-15).
		AddClue("what would the auditor ask for first",
			"Ask what leaves no trace rather than what is untidy. A claim that failed and "+
				"vanished, a state machine with no execution history retained, and photos "+
				"the whole internet can read.",
			-25).
		SetPermission(policy.Document{
			Version: policy.Version20121017,
			Statement: []policy.Statement{
				{Effect: policy.Deny, Action: policy.ActionAll, Resource: policy.ARNAll},
			},
		}).
		SetGuardrail(guardrail()).
		// --- Act I: whose account is this now ------------------------------
		AddCheck("Signed for the claims workflow", challenge.Check{
			Points:  30,
			Every:   20 * time.Second,
			Trigger: stateMachineOwned,
		}).
		AddCheck("Signed for the claims document archive", challenge.Check{
			Points:  25,
			Every:   20 * time.Second,
			Trigger: archiveOwned,
		}).
		AddCheck("Signed for the marketing assets bucket", challenge.Check{
			Points:  10,
			Every:   20 * time.Second,
			Trigger: decoyOwned,
		}).
		AddCheck("Confirmed the handover", challenge.Check{
			Points:  45,
			Every:   20 * time.Second,
			Trigger: handoverConfirmed,
		}).
		AddCheck("Gave the adjudicator logs a retention window", challenge.Check{
			Points:  10,
			Every:   30 * time.Second,
			Trigger: logsRetained,
		}).
		// --- Act II: finish the workflow ------------------------------------
		AddCheck("Deployed the adjudicator", challenge.Check{
			Points:  70,
			Every:   20 * time.Second,
			Trigger: adjudicatorDeployed,
		}).
		AddCheck("Wired the workflow to the adjudicator", challenge.Check{
			Points:  60,
			Every:   20 * time.Second,
			Trigger: resourceWired,
		}).
		AddCheck("Gave rejected claims somewhere to go", challenge.Check{
			Points:  70,
			Every:   20 * time.Second,
			Trigger: catchAdded,
		}).
		AddCheck("Claims are settling", challenge.Check{
			Points:  throughputPoints,
			Every:   30 * time.Second,
			Repeat:  true,
			Trigger: bounded(throughputRounds, claimsSettling()),
		}).
		AddCheck("Every legitimate claim resolved, one way or another", challenge.Check{
			Points:  90,
			Every:   30 * time.Second,
			Trigger: resolvedCleanly(),
		}).
		AddCheck("Claims failing with no review", challenge.Check{
			Points:  failedPoints,
			Every:   30 * time.Second,
			Repeat:  true,
			Trigger: bounded(failedRounds, failuresLanding()),
		}).
		AddEvent("filing-deadline", challenge.Event{
			Every:   30 * time.Second,
			Trigger: firstCleanSettlement(),
			Event:   theFilingDeadline,
		}).
		AddEvent("audit-closeout", challenge.Event{
			Every:   30 * time.Second,
			Trigger: filingClosedOut,
			Event:   theAuditCloseout,
		}).
		Start()
}

// --- permissions -----------------------------------------------------------

func workingSet() policy.Actions {
	return policy.Actions{
		"s3:Get*", "s3:List*", "s3:Put*",
		"states:DescribeStateMachine", "states:UpdateStateMachine", "states:ListStateMachines",
		"states:TagResource", "states:UntagResource",
		"lambda:*",
		"iam:PassRole", "iam:GetRole", "iam:ListRoles",
		"iam:ListRolePolicies", "iam:GetRolePolicy", "iam:ListAttachedRolePolicies",
		"logs:*", "kms:*",
		"ssm:Describe*", "ssm:Get*", "ssm:PutParameter", "ssm:DeleteParameter",
		"ssm:AddTagsToResource", "ssm:ListTagsForResource",
		"events:Describe*", "events:List*",
	}
}

// guardrail. The player never needs to start or read executions at all —
// the workflow is theirs to finish, not theirs to run by hand — so states
// beyond definition management is denied outright rather than merely
// carved around, and the telemetry parameter and the desk's own function
// are protected the same way every gameday challenge in this codebase
// protects its scoring channel.
func guardrail() policy.Document {
	return policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{Sid: "ClaimsTriage", Effect: policy.Allow, Action: workingSet(), Resource: policy.ARNAll},
			{
				Sid:    "TheDeskIsNotYours",
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
				Sid:    "NoRunningItYourself",
				Effect: policy.Deny,
				Action: policy.Actions{
					"states:StartExecution", "states:StartSyncExecution", "states:StopExecution",
					"states:DeleteStateMachine",
				},
				Resource: policy.ARNAll,
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
		func() error { return makeLogGroup(run) },
		func() error { return makeTelemetry() },
		func() error { return makeHandover() },
	); err != nil {
		return err
	}

	if err := parallel(
		func() error { return makeAdjudicatorRole(run) },
		func() error { return makeGeneratorRole(run) },
		func() error { return makeStateMachineRole(run) },
	); err != nil {
		return err
	}

	if err := makeStateMachine(run); err != nil {
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

	s.AddAsset("northbridge-handover.md", []byte(handoverAsset()))
	if pkg := pipelinePackage(); len(pkg) > 0 {
		s.AddAsset("northbridge-adjudicator.zip", pkg)
	}

	s.SetPermission(policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{Sid: "ClaimsTriage", Effect: policy.Allow, Action: workingSet(), Resource: policy.ARNAll},
		},
	})
	return nil
}

func makeArchive(run string) error {
	bucket, err := aws.Create(&s3.Bucket{
		BucketName: new(fmt.Sprintf("claim-docs-%s", run)),
		PublicAccessBlockConfiguration: &s3.BucketPublicAccessBlockConfiguration{
			BlockPublicAcls:       new(false),
			BlockPublicPolicy:     new(false),
			IgnorePublicAcls:      new(false),
			RestrictPublicBuckets: new(false),
		},
		Tags: []s3.BucketTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("claim document archive: %w", err)
	}
	archiveRef = bucket
	return nil
}

func makeDecoyBucket(run string) error {
	bucket, err := aws.Create(&s3.Bucket{
		BucketName: new(fmt.Sprintf("marketing-assets-%s", run)),
		Tags:       []s3.BucketTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("marketing assets bucket: %w", err)
	}
	decoyRef = bucket
	return nil
}

func makeLogGroup(run string) error {
	group, err := aws.Create(&logs.LogGroup{
		LogGroupName: new(fmt.Sprintf("/northbridge/adjudicator/%s", run)),
		Tags:         []logs.LogGroupTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("adjudicator logs: %w", err)
	}
	logGroupRef = group
	return nil
}

func makeTelemetry() error {
	if _, err := aws.Create(&ssm.Parameter{
		Name:        new(telemetryPath),
		Type:        new(ssm.ParameterTypeString),
		Description: new("claims telemetry - written by the claims desk"),
		Value:       new("window=0 sent=0 settled=0 reviewed=0 failed_uncaught=0 rate=0"),
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

func makeAdjudicatorRole(run string) error {
	role, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("northbridge-adjudicator-%s", run)),
		Description:              new("execution role for the claims adjudicator"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("adjudicator"),
			PolicyDocument: json.RawMessage(policy.Document{
				Version: policy.Version20121017,
				Statement: []policy.Statement{
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
		return fmt.Errorf("adjudicator role: %w", err)
	}
	adjudicatorArn = roleArn(role)
	return nil
}

func makeGeneratorRole(run string) error {
	role, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("%srole-%s", gatePrefix, run)),
		Description:              new("execution role for the claims desk - scenario owned"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("desk"),
			PolicyDocument: json.RawMessage(policy.Document{
				Version: policy.Version20121017,
				Statement: []policy.Statement{
					{
						Effect: policy.Allow,
						Action: policy.Actions{
							"states:StartExecution", "states:DescribeExecution", "states:ListExecutions",
						},
						Resource: policy.ARNAll,
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

// makeStateMachineRole is the state machine's own execution role — what it
// runs the Task and (once the player writes one) the manual-review Pass
// state as. It needs lambda:InvokeFunction on any function because the
// plugin cannot know in advance which one the player will point the Task
// state at.
func makeStateMachineRole(run string) error {
	role, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("northbridge-workflow-%s", run)),
		Description:              new("execution role for the claims workflow itself"),
		AssumeRolePolicyDocument: assumedBy("states.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("workflow"),
			PolicyDocument: json.RawMessage(policy.Document{
				Version: policy.Version20121017,
				Statement: []policy.Statement{
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"lambda:InvokeFunction"},
						Resource: policy.ARNAll,
					},
					{
						Effect: policy.Allow,
						Action: policy.Actions{
							"logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents",
							"logs:DescribeLogGroups", "logs:DescribeResourcePolicies",
							"logs:PutResourcePolicy",
						},
						Resource: policy.ARNAll,
					},
				},
			}.String()),
		}},
		Tags: []iamResourceTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("workflow role: %w", err)
	}
	stateMachineWorkflowRole = roleArn(role)
	return nil
}

var stateMachineWorkflowRole string

func makeStateMachine(run string) error {
	name := fmt.Sprintf("northbridge-claims-%s", run)
	machine, err := aws.Create(&stepfunctions.StateMachine{
		StateMachineName: new(name),
		RoleArn:          new(stateMachineWorkflowRole),
		DefinitionString: new(brokenDefinition),
		Tags:             []stepfunctions.StateMachineTagsEntry{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("claims workflow: %w", err)
	}
	stateMachineRef = machine
	if live, err := aws.Read[*stepfunctions.StateMachine](stateMachineRef); err == nil && live != nil && live.Arn != nil {
		stateMachineArn = *live.Arn
	}
	return nil
}

func generatorEnvironment(surge int) *lambda.Environment {
	return &lambda.Environment{Variables: withEnv(map[string]string{
		"STATE_MACHINE_ARN":    stateMachineArn,
		"TALLY_PARAM":          telemetryPath,
		"BASE_RATE":            strconv.Itoa(baseRate),
		"PEAK_RATE":            strconv.Itoa(peakRate),
		"RAMP_WINDOWS":         strconv.Itoa(rampWindows),
		"RISKY_PERMILLE":       strconv.Itoa(riskyPerMille),
		"FILING_SURGE_WINDOWS": strconv.Itoa(surge),
	}, localEndpointOverride())}
}

func makeGeneratorFunction(run string) error {
	bucket, key, err := uploadLambdaCode(generatorSource)
	if err != nil {
		return fmt.Errorf("claims desk code: %w", err)
	}
	definition := &lambda.Function{
		FunctionName: new(fmt.Sprintf("%sscheduler-%s", gatePrefix, run)),
		Description:  new("northbridge claims desk and telemetry - do not disable"),
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
		slog.Warn(fmt.Sprintf("claims desk attempt %d/%d: %v", attempt+1, roleRetries, err))
		time.Sleep(roleRetryDelay)
	}
	return fmt.Errorf("claims desk: %w", err)
}

func makeSchedule(run string) error {
	rule, err := aws.Create(&events.Rule{
		Name:               new(fmt.Sprintf("%sschedule-%s", gatePrefix, run)),
		Description:        new("files the next window of claims, once a minute"),
		ScheduleExpression: new("rate(1 minute)"),
		State:              new(events.RuleStateENABLED),
		Targets: []events.Target{{
			Id:  new("claims-desk"),
			Arn: new(functionArn(generatorRef)),
		}},
	})
	if err != nil {
		return fmt.Errorf("claims desk schedule: %w", err)
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
		return fmt.Errorf("claims desk invoke permission: %w", err)
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
		"northbridge claims triage - handover. Whoever set this up left partway through.",
		"",
		"Flow is: a claim comes in -> Step Functions state machine -> one Lambda task,",
		"the adjudicator -> paid, or not. Everything here is tagged " +
			systemTag + "=" + systemName + ".",
		"",
		"The adjudicator was never deployed. Platform kept a copy - it is correct, and the",
		"execution role for it is already in this account, scoped to nothing more than",
		"CloudWatch Logs, because it does not need anything else.",
		"",
		"The state machine has one state and no Catch. A claim the adjudicator correctly",
		"rejects does not become a review. It becomes a failed execution and nothing else.",
		"",
		"Telemetry writes to " + telemetryPath + " every minute: claims sent, settled,",
		"sent to review, and failed with nowhere to go. The last number has never been zero.",
		"",
		"Known and not fixed: the claim document archive is wide open with no versioning,",
		"and the workflow keeps no execution logs at all - if an auditor asks what happened",
		"to a claim six weeks ago, the honest answer today is that nobody kept it.",
	}, "\n")
}

func handoverAsset() string {
	return "# Northbridge Claims — handover\n\n```\n" + handoverNote() + "\n```\n"
}

// --- Act I -------------------------------------------------------------------

func stateMachineOwned() (bool, error) {
	machine, err := readStateMachine()
	if err != nil {
		return false, err
	}
	for _, tag := range machine.Tags {
		if tag.Key != nil && *tag.Key == ownerTag && tag.Value != nil && *tag.Value != "" {
			return true, nil
		}
	}
	return false, nil
}

func archiveOwned() (bool, error) { return bucketOwned(archiveRef, "claim document archive") }
func decoyOwned() (bool, error)   { return bucketOwned(decoyRef, "marketing assets bucket") }

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

// --- Act II: finish the workflow ----------------------------------------------

func adjudicatorDeployed() (bool, error) {
	functions, err := newFunctions()
	if err != nil {
		return false, err
	}
	return len(functions) > 0, nil
}

// resourceWired reads the state machine's DefinitionString back and looks
// for the Task state's Resource pointing at a function the player deployed
// — not by name, by identity: the arn has to resolve to something that was
// not here at bootstrap.
func resourceWired() (bool, error) {
	definition, err := readDefinition()
	if err != nil {
		return false, err
	}
	taskResource, _ := taskState(definition)
	if taskResource == "" {
		return false, nil
	}
	functions, err := newFunctions()
	if err != nil {
		return false, err
	}
	for identifier, live := range functions {
		if live == nil || live.Arn == nil {
			continue
		}
		if *live.Arn == taskResource || strings.Contains(taskResource, identifier) {
			return true, nil
		}
	}
	return false, nil
}

func catchAdded() (bool, error) {
	definition, err := readDefinition()
	if err != nil {
		return false, err
	}
	_, hasCatch := taskState(definition)
	return hasCatch, nil
}

// taskState parses just enough of the definition to answer two questions:
// what does the Adjudicate task point at, and does it have a non-empty
// Catch. It tolerates a state named differently, since the player is
// rewriting this file freely and "Adjudicate" was never a contract, only a
// starting point.
func taskState(definition map[string]any) (resource string, hasCatch bool) {
	states, _ := definition["States"].(map[string]any)
	for _, raw := range states {
		state, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if kind, _ := state["Type"].(string); kind != "Task" {
			continue
		}
		if r, ok := state["Resource"].(string); ok && r != "" {
			resource = r
		}
		if catches, ok := state["Catch"].([]any); ok && len(catches) > 0 {
			hasCatch = true
		}
		if resource != "" {
			return resource, hasCatch
		}
	}
	return resource, hasCatch
}

func readDefinition() (map[string]any, error) {
	machine, err := readStateMachine()
	if err != nil {
		return nil, err
	}
	if machine.DefinitionString == nil || *machine.DefinitionString == "" {
		return nil, fmt.Errorf("claims workflow definition is not readable")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(*machine.DefinitionString), &parsed); err != nil {
		// a definition the player is mid-edit on may not parse for a cycle
		// or two - that is not a reason to fail the check, just to wait.
		return nil, nil
	}
	return parsed, nil
}

func claimsSettling() func() (bool, error) {
	last := -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if last < 0 {
			last = state.settled
			return false, nil
		}
		if state.settled <= last {
			return false, nil
		}
		last = state.settled
		return true, nil
	}
}

func resolvedCleanly() func() (bool, error) {
	lastSettled, lastReviewed, lastFailed := -1, -1, -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if lastSettled < 0 {
			lastSettled, lastReviewed, lastFailed = state.settled, state.reviewed, state.failedUncaught
			return false, nil
		}
		resolved := (state.settled - lastSettled) + (state.reviewed - lastReviewed)
		failed := state.failedUncaught - lastFailed
		lastSettled, lastReviewed, lastFailed = state.settled, state.reviewed, state.failedUncaught
		return resolved > 0 && failed <= 0, nil
	}
}

func failuresLanding() func() (bool, error) {
	last := -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if last < 0 {
			last = state.failedUncaught
			return false, nil
		}
		if state.failedUncaught <= last {
			return false, nil
		}
		last = state.failedUncaught
		return true, nil
	}
}

func firstCleanSettlement() func() (bool, error) {
	return func() (bool, error) {
		if filingStarted.Load() {
			return true, nil
		}
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		return state.settled > 0, nil
	}
}

// --- Act III: the filing deadline --------------------------------------------

var filingFloor atomic.Int64

func theFilingDeadline(ctx context.Context, s *challenge.Scenario) error {
	if !filingStarted.CompareAndSwap(false, true) {
		return nil
	}

	if state, ok, err := readTelemetry(); err == nil && ok {
		filingFloor.Store(int64(state.window))
	}

	s.AddDescription(
		"A state filing deadline lands at the end of the week, and every claim in the " +
			"backlog that has been sitting behind this unfinished pipeline is about to arrive " +
			"at once rather than trickle in over the month like it normally would. Whatever " +
			"is still uncaught when that hits will fail at three times today's volume, all " +
			"in the same afternoon.")
	s.AddDescription(
		"An external auditor is also coming, on the strength of a complaint from a claimant " +
			"whose rejection nobody could explain. She will want to see execution history, " +
			"not just a scoreboard that says things are fine now.")

	s.AddClue("she means execution logs, not the tally",
		"The workflow's own LoggingConfiguration, sending everything at level ALL to a log "+
			"group you control, is what makes a specific claim's history producible instead "+
			"of gone the moment its execution ages out.",
		-25)
	s.AddClue("what she will want in writing",
		"Put the incident report in "+incidentPath+". It has to quote two things: the id "+
			"of the key the claim document archive is encrypted under, and the arn of the "+
			"log group the workflow's execution history now writes to.",
		-20)

	s.AddCheck("Held the line for the filing deadline", challenge.Check{
		Points:  90,
		Every:   30 * time.Second,
		Trigger: heldTheDeadline(),
	})
	s.AddCheck("Claims routed to manual review", challenge.Check{
		Points:  reviewedPoints,
		Every:   30 * time.Second,
		Repeat:  true,
		Trigger: bounded(reviewedRounds, reviewedLanding()),
	})
	s.AddCheck("Gave the workflow an execution audit trail", challenge.Check{
		Points:  60,
		Every:   20 * time.Second,
		Trigger: loggingEnabled,
	})
	s.AddCheck("Put the claim document archive under a company key", challenge.Check{
		Points:  55,
		Every:   20 * time.Second,
		Trigger: archiveEncrypted,
	})
	s.AddCheck("Took the claim document archive off the public internet", challenge.Check{
		Points:  45,
		Every:   20 * time.Second,
		Trigger: archiveClosed,
	})
	s.AddCheck("Turned on versioning for the claim document archive", challenge.Check{
		Points:  30,
		Every:   20 * time.Second,
		Trigger: archiveVersioned,
	})
	s.AddCheck("Filed the incident report", challenge.Check{
		Points:  55,
		Every:   20 * time.Second,
		Trigger: incidentReportFiled,
	})

	if generatorRef == "" {
		return fmt.Errorf("claims desk was never provisioned")
	}
	if err := aws.Update(generatorRef, &lambda.Function{
		Environment: generatorEnvironment(1),
	}); err != nil {
		return fmt.Errorf("switch claims desk to filing surge: %w", err)
	}
	slog.Info("filing surge enabled on the claims desk")
	return nil
}

func heldTheDeadline() func() (bool, error) {
	lastResolved, lastFailed := -1, -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if int64(state.window) < filingFloor.Load()+filingGrace {
			return false, nil
		}
		resolvedNow := state.settled + state.reviewed
		if lastResolved < 0 {
			lastResolved, lastFailed = resolvedNow, state.failedUncaught
			return false, nil
		}
		resolved := resolvedNow - lastResolved
		failed := state.failedUncaught - lastFailed
		lastResolved, lastFailed = resolvedNow, state.failedUncaught
		return resolved > 0 && failed <= 0, nil
	}
}

func reviewedLanding() func() (bool, error) {
	last := -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if last < 0 {
			last = state.reviewed
			return false, nil
		}
		if state.reviewed <= last {
			return false, nil
		}
		last = state.reviewed
		return true, nil
	}
}

func loggingEnabled() (bool, error) {
	machine, err := readStateMachine()
	if err != nil {
		return false, err
	}
	cfg := machine.LoggingConfiguration
	if cfg == nil || cfg.Level == nil {
		return false, nil
	}
	if *cfg.Level != stepfunctions.LoggingConfigurationLevelALL {
		return false, nil
	}
	return len(cfg.Destinations) > 0, nil
}

func archiveEncrypted() (bool, error) {
	bucket, err := readBucket(archiveRef, "claim document archive")
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
	bucket, err := readBucket(archiveRef, "claim document archive")
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
	bucket, err := readBucket(archiveRef, "claim document archive")
	if err != nil {
		return false, err
	}
	if bucket.VersioningConfiguration == nil || bucket.VersioningConfiguration.Status == nil {
		return false, nil
	}
	return *bucket.VersioningConfiguration.Status == s3.VersioningConfigurationStatusEnabled, nil
}

func incidentReportFiled() (bool, error) {
	value, ok, err := parameter(incidentPath)
	if err != nil || !ok {
		return false, err
	}
	machine, err := readStateMachine()
	if err != nil {
		return false, err
	}
	logGroupArn := ""
	if machine.LoggingConfiguration != nil {
		for _, destination := range machine.LoggingConfiguration.Destinations {
			if destination.CloudWatchLogsLogGroup != nil && destination.CloudWatchLogsLogGroup.LogGroupArn != nil {
				logGroupArn = *destination.CloudWatchLogsLogGroup.LogGroupArn
				break
			}
		}
	}
	if logGroupArn == "" || !strings.Contains(value, logGroupArn) {
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

// --- the coda ------------------------------------------------------------------

func filingClosedOut() (bool, error) {
	if !filingStarted.Load() {
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

func theAuditCloseout(ctx context.Context, s *challenge.Scenario) error {
	if !codaOpen.CompareAndSwap(false, true) {
		return nil
	}

	s.AddDescription(
		"The audit closes clean and the incident report is filed. There is nothing left to " +
			"build. Points accrue for every window that resolves every claim it touches and " +
			"keeps up with the desk, and they stop the moment either stops being true.")

	s.AddCheck("Kept every claim resolved", challenge.Check{
		Points:  heldPoints,
		Every:   30 * time.Second,
		Repeat:  true,
		Trigger: bounded(heldRounds, resolvedCleanly()),
	})
	s.AddCheck("Kept up with the desk", challenge.Check{
		Points:  codaPoints,
		Every:   30 * time.Second,
		Repeat:  true,
		Trigger: bounded(codaRounds, keepingUp()),
	})
	return nil
}

func keepingUp() func() (bool, error) {
	lastSent, lastResolved := -1, -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		resolvedNow := state.settled + state.reviewed
		if lastSent < 0 {
			lastSent, lastResolved = state.sent, resolvedNow
			return false, nil
		}
		sent, resolved := state.sent-lastSent, resolvedNow-lastResolved
		lastSent, lastResolved = state.sent, resolvedNow
		if sent <= 0 {
			return false, nil
		}
		return resolved*1000 >= sent*keepingUpPerMille, nil
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

func readStateMachine() (*stepfunctions.StateMachine, error) {
	if stateMachineRef == "" {
		return nil, fmt.Errorf("claims workflow was never provisioned")
	}
	machine, err := aws.Read[*stepfunctions.StateMachine](stateMachineRef)
	if err != nil {
		return nil, err
	}
	if machine == nil {
		return nil, fmt.Errorf("claims workflow is not readable")
	}
	return machine, nil
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
		return nil, fmt.Errorf("adjudicator logs were never provisioned")
	}
	group, err := aws.Read[*logs.LogGroup](logGroupRef)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("adjudicator logs are not readable")
	}
	return group, nil
}

type telemetry struct {
	window         int
	sent           int
	settled        int
	reviewed       int
	failedUncaught int
	rate           int
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
		case "settled":
			state.settled = number
		case "reviewed":
			state.reviewed = number
		case "failed_uncaught":
			state.failedUncaught = number
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
