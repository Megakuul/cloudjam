//go:build wasip1

// Command ridgeline-settlement is a gameday tier cloudjam challenge.
//
// Ridgeline Power settles the regional grid: half-hourly meter readings from
// six substations become money owed between generators and suppliers, and the
// settled ledger is the instruction the clearing bank acts on. The team that
// ran the settlement platform was outsourced in January and the overnight run
// has not completed since Tuesday.
//
// The pipeline is four hops — meter intake, validation, settlement queue,
// settlement stage, settled ledger — and the player inherits the two ends and
// neither middle. Restarting it is Act II and it is not the hard part.
//
// The hard part is that the settlement stage they are handed settles
// unconditionally. SQS is at-least-once, so a redelivered message settles the
// same meter interval twice, and the ledger is not a report — it is money. At
// the rate a healthy queue redelivers, that is a handful of readings a night
// and nobody has ever noticed. Act III is a substation data logger coming back
// from a fault and flushing three days of backlog through a pipeline that has
// no idea it has seen any of it before.
//
// Nothing in the briefing says "idempotency". The README says a reading id
// identifies an interval and mentions, in a note nobody answered, that loggers
// flush their backlog. Working out what those two facts mean together, before
// the backlog arrives rather than after, is the challenge.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
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
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/kms"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/lambda"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/logs"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/s3"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/sqs"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/ssm"
	"github.com/google/uuid"
)

// ownerTag is what taking ownership means mechanically. The estate is untagged
// and the asset register counts only what carries an owner. Any non-empty value
// counts.
const ownerTag = "ridgeline:owner"

// systemTag is on everything bootstrap provisions. It is never checked; it is
// the discovery affordance, and it is the only inventory that exists.
const systemTag = "ridgeline:system"

const systemName = "settlement"

// meterPrefix names everything the scenario owns and the player does not.
const meterPrefix = "ridgeline-meters-"

const (
	handoverPath  = "/ridgeline/handover"
	reconPath     = "/ridgeline/reconciliation"
	exceptionPath = "/ridgeline/exception"
)

const (
	roleRetries    = 6
	roleRetryDelay = 5 * time.Second
)

const (
	minRetentionDays = 30
	maxRetentionDays = 400
)

// The meter feed's shape.
const (
	baseRate    = 12
	peakRate    = 40
	rampWindows = 16
)

// Bounds on the repeating checks, in both directions. Repeat checks never
// retire, so without a cap the maximum score does not exist — and neither does
// the maximum loss on the negative one.
const (
	throughputRounds = 10
	throughputPoints = 10
	doubleRounds     = 8
	doublePoints     = -18
	heldRounds       = 8
	heldPoints       = 8
	codaRounds       = 6
	codaPoints       = 6
)

// keepingUpPerMille is how much of what the meters sent has to settle exactly
// once for the run to count as keeping up. Not 100%: a batch in flight when the
// window closes is normal.
const keepingUpPerMille = 550

// replayGrace is how many settlement windows the player gets after the backlog
// arrives before the "absorbed the replay" check starts looking. The replay has
// to actually reach the pipeline before it is fair to ask what happened to it.
const replayGrace = 2

var (
	archiveRef    string
	intakeRef     string
	intakeArn     string
	settleRef     string
	settleArn     string
	ledgerRef     string
	ledgerArn     string
	ledgerLabel   string
	logGroupRef   string
	validationArn string
	settlementArn string
	meterRole     string
	meterRef      string
	scheduleRef   string
)

// Captured before bootstrap provisions anything, so that "did the player create
// one of these" is a meaningful question on an account that was not empty.
var (
	keyBaseline      = map[string]bool{}
	functionBaseline = map[string]bool{}
)

var (
	backlogArrived atomic.Bool
	codaOpen       atomic.Bool
)

func main() {
	challenge.New("Ridgeline Settlement: The Replay", 10*time.Second, bootstrap).
		AddDescription(
			"Ridgeline Power settles the regional grid. Six substations, half-hourly meter "+
				"readings, and a settlement run that turns each of those readings into money "+
				"owed between the generator that put the energy on the network and the "+
				"supplier that sold it. About four million pounds a night moves on the back "+
				"of it.").
		AddDescription(
			"The settled ledger is not a report and nobody reads it for interest. It is the "+
				"instruction the clearing bank acts on in the morning. What is in it at 06:00 "+
				"is what gets paid, and unwinding a payment that should not have happened "+
				"takes six weeks and a lawyer.").
		AddDescription(
			"The two people who ran this platform were outsourced in January. Their handover "+
				"was a parameter in this account and a shrug. The overnight run has not "+
				"completed since Tuesday, settlement is three days behind, and two suppliers "+
				"have now called the regulator rather than us.").
		AddDescription(
			"You are the engineer who inherited it. The meters do not stop — readings are "+
				"arriving now and piling up behind whatever broke. There is no diagram and no "+
				"runbook. Everything the old team built carries a "+systemTag+"="+systemName+
				" tag, and that tag is the whole asset register.").
		AddDescription(
			"The reconciliation writes what it finds to "+reconPath+" once a minute: how many "+
				"readings went out, how many settled cleanly, and how many settled more than "+
				"once. That parameter is your dashboard. You can read it. You cannot write "+
				"it.").
		AddDescription(
			"Nobody is going to hand you a task list, because nobody wrote one. You are "+
				"scored the way the business is measured: on money moved correctly, and "+
				"against money moved twice. There is far more here that could be fixed than "+
				"there is night to fix it in, and deciding what actually matters is the "+
				"whole job.").
		// Clue prices are added to the team score, so they are negative.
		AddClue("where do i start",
			"The old team's handover note is an SSM parameter at "+handoverPath+". Read it "+
				"before you touch anything: aws ssm get-parameter --name "+handoverPath+".",
			-5).
		AddClue("nothing is running the settlement",
			"Both stages came out with the outsourcing and neither went back. Platform kept "+
				"a copy — it is attached to this challenge as a downloadable package, and it "+
				"contains no deployment instructions of any kind.",
			-30).
		AddClue("i cannot create an execution role",
			"You do not have iam:CreateRole and you are not meant to. Both stages have roles "+
				"already provisioned in this account, scoped for exactly this. Find them with "+
				"aws iam list-roles and pass them.",
			-20).
		AddClue("what counts as ownership",
			"The asset register only counts resources carrying a "+ownerTag+" tag. Any value "+
				"will do — the point is that somebody's name is on it.",
			-20).
		AddClue("readings are settling but the double counter is not zero",
			"Read what the settlement stage does to the ledger, then read what the queue "+
				"guarantees about delivery. The stage never errors and the queue drains. "+
				"Those two facts are not as reassuring together as they look apart.",
			-40).
		AddClue("what is a double settlement worth",
			"Considerably less than nothing. It is a payment instruction the clearing bank "+
				"will act on twice against one meter interval, and getting it back takes six "+
				"weeks. The scoreboard prices it the way the finance director does.",
			-15).
		AddClue("what would hurt most at 06:00",
			"Ask what cannot be undone rather than what is untidy. A payment already made, a "+
				"ledger with no way back from a bad run, and readings that vanish instead of "+
				"landing somewhere you can look at them.",
			-25).
		SetPermission(policy.Document{
			Version: policy.Version20121017,
			Statement: []policy.Statement{
				{Effect: policy.Deny, Action: policy.ActionAll, Resource: policy.ARNAll},
			},
		}).
		SetGuardrail(guardrail()).
		// --- Act I: what did we just inherit ---------------------------------
		AddCheck("Signed for the meter intake", challenge.Check{
			Points:  25,
			Every:   20 * time.Second,
			Trigger: intakeOwned,
		}).
		AddCheck("Signed for the settlement queue", challenge.Check{
			Points:  25,
			Every:   20 * time.Second,
			Trigger: settlementQueueOwned,
		}).
		AddCheck("Signed for the settled ledger", challenge.Check{
			Points:  25,
			Every:   20 * time.Second,
			Trigger: ledgerOwned,
		}).
		AddCheck("Signed for the reading archive", challenge.Check{
			Points:  20,
			Every:   20 * time.Second,
			Trigger: archiveOwned,
		}).
		AddCheck("Confirmed the handover", challenge.Check{
			Points:  45,
			Every:   20 * time.Second,
			Trigger: handoverConfirmed,
		}).
		// Worth almost nothing on purpose. It looks like diligence and it is
		// the cheapest thing in the account; a player who starts here has
		// spent the first half hour of a settlement outage on log retention.
		AddCheck("Gave the settlement logs a retention window", challenge.Check{
			Points:  10,
			Every:   30 * time.Second,
			Trigger: settlementLogsRetained,
		}).
		// --- Act II: restart settlement ---------------------------------------
		AddCheck("Deployed the validation stage", challenge.Check{
			Points:  70,
			Every:   20 * time.Second,
			Trigger: validationDeployed,
		}).
		AddCheck("Deployed the settlement stage", challenge.Check{
			Points:  70,
			Every:   20 * time.Second,
			Trigger: settlementDeployed,
		}).
		AddCheck("Wired the meter intake into validation", challenge.Check{
			Points:  55,
			Every:   20 * time.Second,
			Trigger: intakeWired,
		}).
		AddCheck("Wired validation into settlement", challenge.Check{
			Points:  55,
			Every:   20 * time.Second,
			Trigger: settlementWired,
		}).
		AddCheck("Money is moving again", challenge.Check{
			Points:  throughputPoints,
			Every:   30 * time.Second,
			Repeat:  true,
			Trigger: bounded(throughputRounds, settlementLanding()),
		}).
		AddCheck("Every reading settled exactly once", challenge.Check{
			Points:  90,
			Every:   30 * time.Second,
			Trigger: settlingExactlyOnce(),
		}).
		// The only negative check, and the reason this is a gameday. It is not
		// something an event did to the player — it fires because they put a
		// stage into service that settles unconditionally, which is a decision,
		// and it stops the moment they fix it.
		AddCheck("Double settlements in the settled ledger", challenge.Check{
			Points:  doublePoints,
			Every:   30 * time.Second,
			Repeat:  true,
			Trigger: bounded(doubleRounds, doublesAppearing()),
		}).
		AddEvent("backlog", challenge.Event{
			Every:   30 * time.Second,
			Trigger: settlementRunning(),
			Event:   theBacklog,
		}).
		AddEvent("morning", challenge.Event{
			Every:   30 * time.Second,
			Trigger: backlogClosedOut,
			Event:   theMorningRun,
		}).
		Start()
}

// --- permissions --------------------------------------------------------------

func workingSet() policy.Actions {
	return policy.Actions{
		// The reading archive, including the object upload — the deployment
		// package has to go somewhere Lambda can read it from.
		//
		// Put* also settles a naming disagreement: fakecloud authorises two of
		// these under their api operation name (s3:PutBucketEncryption,
		// s3:PutPublicAccessBlock) while real aws uses the iam action name
		// (s3:PutEncryptionConfiguration, s3:PutBucketPublicAccessBlock).
		"s3:Get*", "s3:List*", "s3:Put*",

		// The pipeline. The player creates a dead letter queue for rejected
		// readings, so this needs create rights.
		"sqs:*",

		// The two stages. The guardrail carves the meter feed back out.
		"lambda:*",

		// Passing the pre-provisioned stage roles, and finding them. The player
		// never mints a role, which is what keeps them inside the boundary.
		"iam:PassRole", "iam:GetRole", "iam:ListRoles",
		"iam:ListRolePolicies", "iam:GetRolePolicy", "iam:ListAttachedRolePolicies",

		// The settled ledger: recovery, deletion protection, tags. Not
		// CreateTable — nothing here needs a new one. Item writes are denied in
		// the guardrail so that a settlement cannot be typed in by hand.
		"dynamodb:Describe*", "dynamodb:List*",
		"dynamodb:UpdateTable", "dynamodb:UpdateContinuousBackups",
		"dynamodb:UpdateTimeToLive", "dynamodb:TagResource", "dynamodb:UntagResource",

		"logs:*", "kms:*",

		// The handover note and the exception report.
		"ssm:Describe*", "ssm:Get*", "ssm:PutParameter", "ssm:DeleteParameter",
		"ssm:AddTagsToResource", "ssm:ListTagsForResource",

		"events:Describe*", "events:List*",
	}
}

// guardrail is the permission boundary. The deny statements are the
// load-bearing part: without them the player can rewrite the meter feed that
// generates the readings, forge the reconciliation that scores them, or type
// settlements straight into the ledger — and each of those makes the scoreboard
// a lie rather than a measurement.
//
// Patterns rather than arns, because the boundary is published before bootstrap
// has run. Service wildcards rather than generated action groups, because the
// boundary is a 6144 character managed policy and ActionsFrom on three of these
// services alone would be several times that.
func guardrail() policy.Document {
	return policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{
				Sid:      "SettlementRun",
				Effect:   policy.Allow,
				Action:   workingSet(),
				Resource: policy.ARNAll,
			},
			{
				Sid:    "TheMeterFeedIsNotYours",
				Effect: policy.Deny,
				Action: policy.Actions{
					"lambda:UpdateFunctionCode", "lambda:UpdateFunctionConfiguration",
					"lambda:DeleteFunction", "lambda:InvokeFunction",
					"lambda:AddPermission", "lambda:RemovePermission",
				},
				Resource: policy.ARNs{policy.ARN("arn:aws:lambda:*:*:function:" + meterPrefix + "*")},
			},
			{
				Sid:      "TheScheduleIsNotYours",
				Effect:   policy.Deny,
				Action:   policy.Actions{"events:*"},
				Resource: policy.ARNs{policy.ARN("arn:aws:events:*:*:rule/" + meterPrefix + "*")},
			},
			{
				Sid:    "ReconciliationIsReadOnly",
				Effect: policy.Deny,
				Action: policy.Actions{
					"ssm:PutParameter", "ssm:DeleteParameter", "ssm:DeleteParameters",
					"ssm:LabelParameterVersion",
				},
				Resource: policy.ARNs{policy.ARN("arn:aws:ssm:*:*:parameter" + reconPath)},
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

// --- AWS::IAM::Role -----------------------------------------------------------

// pkg/challenge/aws/services/iam carries action constants and no resource
// types, so there is no iam.Role to import. Cloud Control does support
// AWS::IAM::Role and aws.Resource is satisfied by anything with a CloudJamType.
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

// roleArn resolves a role's arn. Cloud Control's identifier for AWS::IAM::Role
// is the role name, but Lambda's Role property is validated against an arn
// pattern, and the arn is not assemblable because the plugin is never told the
// account id.
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

// --- bootstrap ----------------------------------------------------------------

// parallel runs independent bootstrap steps together and joins what failed.
//
// It does not overlap them today and cannot: a //go:wasmimport call blocks the
// whole wasm instance, because GOOS=wasip1 has no threads and the Go scheduler
// gets no chance to switch goroutines while a host call is in flight. It is
// written this way because the waves are the actual dependency graph of the
// estate, and because the day the host carries host calls concurrently this
// gets the time back for free.
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

	// wave one: the inherited estate. The reconciliation parameter is zeroed
	// rather than left absent, so a player who opens their dashboard before the
	// feed has run once sees zeroes instead of a 404.
	if err := parallel(
		func() error { return makeArchive(run) },
		func() error { return makeIntake(run) },
		func() error { return makeSettlementQueue(run) },
		func() error { return makeLedger(run) },
		func() error { return makeLogGroup(run) },
		func() error { return makeReconciliation() },
		func() error { return makeHandover() },
	); err != nil {
		return err
	}

	// wave two: every role is scoped to arns the first wave produced.
	if err := parallel(
		func() error { return makeValidationRole(run) },
		func() error { return makeSettlementRole(run) },
		func() error { return makeMeterRole(run) },
	); err != nil {
		return err
	}

	// waves three to five: each one's input is the previous one's output.
	if err := makeMeterFeed(run); err != nil {
		return err
	}
	if err := makeSchedule(run); err != nil {
		return err
	}
	if err := makeInvokePermission(); err != nil {
		return err
	}

	s.AddAsset("ridgeline-handover.md", []byte(handoverAsset()))
	if pkg := pipelinePackage(); len(pkg) > 0 {
		s.AddAsset("ridgeline-pipeline.zip", pkg)
	}

	s.SetPermission(policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{
				Sid:      "SettlementRun",
				Effect:   policy.Allow,
				Action:   workingSet(),
				Resource: policy.ARNAll,
			},
		},
	})
	return nil
}

func makeArchive(run string) error {
	bucket, err := aws.Create(&s3.Bucket{
		BucketName: new(fmt.Sprintf("ridgeline-readings-%s", run)),
		// the scenario: raw meter files, readable by anyone who guesses the
		// name, in the clear, with no way back from an overwrite.
		PublicAccessBlockConfiguration: &s3.BucketPublicAccessBlockConfiguration{
			BlockPublicAcls:       new(false),
			BlockPublicPolicy:     new(false),
			IgnorePublicAcls:      new(false),
			RestrictPublicBuckets: new(false),
		},
		Tags: []s3.BucketTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("reading archive: %w", err)
	}
	archiveRef = bucket
	return nil
}

func makeIntake(run string) error {
	queue, err := aws.Create(&sqs.Queue{
		QueueName: new(fmt.Sprintf("ridgeline-intake-%s", run)),
		// no redrive policy on purpose: a reading the validation stage rejects
		// is redelivered until it ages out and then simply vanishes, which is
		// one of the things the regulator will ask about.
		VisibilityTimeout: new(45),
		Tags:              []sqs.Tag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("meter intake: %w", err)
	}
	intakeRef = queue
	intakeArn = queueArn(intakeRef)
	return nil
}

func makeSettlementQueue(run string) error {
	queue, err := aws.Create(&sqs.Queue{
		QueueName:         new(fmt.Sprintf("ridgeline-settle-%s", run)),
		VisibilityTimeout: new(45),
		Tags:              []sqs.Tag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("settlement queue: %w", err)
	}
	settleRef = queue
	settleArn = queueArn(settleRef)
	return nil
}

// makeLedger builds the settled ledger keyed on reading_id.
//
// The key is the interval, not the delivery, and that is deliberate: it is the
// affordance that makes an idempotent settlement stage writable at all. A
// player who works out what is wrong can fix it with a condition expression on
// the write they already have, rather than needing a table they cannot create.
func makeLedger(run string) error {
	name := fmt.Sprintf("ridgeline-ledger-%s", run)
	table, err := aws.Create(&dynamodb.Table{
		TableName: new(name),
		AttributeDefinitions: []dynamodb.TableAttributeDefinition{
			{AttributeName: new("reading_id"), AttributeType: new("S")},
		},
		KeySchema:   json.RawMessage(`[{"AttributeName":"reading_id","KeyType":"HASH"}]`),
		BillingMode: new("PAY_PER_REQUEST"),
		Tags:        []dynamodb.TableTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("settled ledger: %w", err)
	}
	ledgerRef = table
	ledgerLabel = name
	// the identifier is the name in some environments and the arn in others,
	// and the settlement stage is configured with LEDGER_TABLE — a wrong value
	// there is a pipeline that cannot work and a debugging session nobody
	// enjoys.
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
		LogGroupName: new(fmt.Sprintf("/ridgeline/settlement/%s", run)),
		Tags:         []logs.LogGroupTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("settlement logs: %w", err)
	}
	logGroupRef = group
	return nil
}

func makeReconciliation() error {
	if _, err := aws.Create(&ssm.Parameter{
		Name:        new(reconPath),
		Type:        new(ssm.ParameterTypeString),
		Description: new("settlement reconciliation - written by the meter feed"),
		Value:       new("window=0 sent=0 exact=0 double=0 replayed=0 rate=0"),
	}); err != nil {
		return fmt.Errorf("reconciliation parameter: %w", err)
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

// --- wave two: the roles the player cannot mint -------------------------------

func scopes() (queues policy.ARNs, ledger policy.ARNs) {
	queues = policy.ARNs{}
	if intakeArn != "" {
		queues = append(queues, policy.ARN(intakeArn))
	}
	if settleArn != "" {
		queues = append(queues, policy.ARN(settleArn))
	}
	if len(queues) == 0 {
		queues = policy.ARNAll
	}
	ledger = policy.ARNAll
	if ledgerArn != "" {
		ledger = policy.ARNs{policy.ARN(ledgerArn)}
	}
	return queues, ledger
}

func makeValidationRole(run string) error {
	queues, _ := scopes()
	role, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("ridgeline-validation-%s", run)),
		Description:              new("execution role for the validation stage"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("validation"),
			PolicyDocument: json.RawMessage(policy.Document{
				Version: policy.Version20121017,
				Statement: []policy.Statement{
					{
						Effect: policy.Allow,
						Action: policy.Actions{
							"sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes",
							"sqs:SendMessage", "sqs:SendMessageBatch", "sqs:GetQueueUrl",
						},
						Resource: queues,
					},
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents"},
						Resource: policy.ARNAll,
					},
				},
			}.String()),
		}},
		Tags: []iamResourceTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("validation role: %w", err)
	}
	validationArn = roleArn(role)
	return nil
}

// makeSettlementRole grants ConditionCheck alongside the write, because the fix
// the challenge is asking for is a conditional write and a role that cannot
// perform one would make the correct answer fail for the wrong reason.
func makeSettlementRole(run string) error {
	queues, ledger := scopes()
	role, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("ridgeline-settlement-%s", run)),
		Description:              new("execution role for the settlement stage"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("settlement"),
			PolicyDocument: json.RawMessage(policy.Document{
				Version: policy.Version20121017,
				Statement: []policy.Statement{
					{
						Effect: policy.Allow,
						Action: policy.Actions{
							"sqs:ReceiveMessage", "sqs:DeleteMessage", "sqs:GetQueueAttributes",
							"sqs:GetQueueUrl",
						},
						Resource: queues,
					},
					{
						Effect: policy.Allow,
						Action: policy.Actions{
							"dynamodb:PutItem", "dynamodb:UpdateItem", "dynamodb:GetItem",
							"dynamodb:ConditionCheckItem", "dynamodb:DescribeTable",
						},
						Resource: ledger,
					},
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents"},
						Resource: policy.ARNAll,
					},
				},
			}.String()),
		}},
		Tags: []iamResourceTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("settlement role: %w", err)
	}
	settlementArn = roleArn(role)
	return nil
}

func makeMeterRole(run string) error {
	queues, ledger := scopes()
	role, err := aws.Create(&iamRole{
		RoleName:                 new(meterPrefix + "role-" + run),
		Description:              new("execution role for the meter feed - scenario owned"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("meters"),
			PolicyDocument: json.RawMessage(policy.Document{
				Version: policy.Version20121017,
				Statement: []policy.Statement{
					{
						Effect: policy.Allow,
						Action: policy.Actions{
							"sqs:SendMessage", "sqs:SendMessageBatch", "sqs:GetQueueAttributes",
							"sqs:GetQueueUrl",
						},
						Resource: queues,
					},
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"dynamodb:Scan", "dynamodb:DescribeTable"},
						Resource: ledger,
					},
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"ssm:GetParameter", "ssm:PutParameter"},
						Resource: policy.ARNAll,
					},
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"logs:CreateLogGroup", "logs:CreateLogStream", "logs:PutLogEvents"},
						Resource: policy.ARNAll,
					},
				},
			}.String()),
		}},
		Tags: []iamResourceTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("meter feed role: %w", err)
	}
	meterRole = roleArn(role)
	return nil
}

// --- waves three to five: the meter feed --------------------------------------

// meterEnvironment is the feed's configuration. It is built in one place
// because Act III rewrites it to switch the replay on, and Update is a shallow
// patch — setting Environment replaces the whole object, so the replay switch
// has to resend every other variable with it or the feed loses its wiring.
func meterEnvironment(replay int) *lambda.Environment {
	return &lambda.Environment{Variables: map[string]string{
		"INTAKE_QUEUE_URL": intakeRef,
		"LEDGER_TABLE":     ledgerLabel,
		"TALLY_PARAM":      reconPath,
		"BASE_RATE":        strconv.Itoa(baseRate),
		"PEAK_RATE":        strconv.Itoa(peakRate),
		"RAMP_WINDOWS":     strconv.Itoa(rampWindows),
		"REPLAY_WINDOWS":   strconv.Itoa(replay),
	}}
}

func makeMeterFeed(run string) error {
	name := fmt.Sprintf("%sfeed-%s", meterPrefix, run)

	definition := &lambda.Function{
		FunctionName: new(name),
		Description:  new("ridgeline substation feed and settlement reconciliation - do not disable"),
		Runtime:      new("python3.12"),
		Handler:      new("index.handler"),
		Role:         new(meterRole),
		Timeout:      new(120),
		MemorySize:   new(256),
		// inline source, because the plugin has no data plane: it cannot put an
		// object in s3, so a function whose code lives in s3 is not something
		// bootstrap can build.
		Code:        &lambda.Code{ZipFile: new(meterSource)},
		Environment: meterEnvironment(0),
		Tags:        []lambda.FunctionTag{{Key: new(systemTag), Value: new(systemName)}},
	}

	var err error
	for attempt := range roleRetries {
		var function string
		if function, err = aws.Create(definition); err == nil {
			meterRef = function
			functionBaseline[meterRef] = true
			return nil
		}
		slog.Warn(fmt.Sprintf("meter feed attempt %d/%d: %v", attempt+1, roleRetries, err))
		time.Sleep(roleRetryDelay)
	}
	return fmt.Errorf("meter feed: %w", err)
}

func makeSchedule(run string) error {
	rule, err := aws.Create(&events.Rule{
		Name:        new(fmt.Sprintf("%sschedule-%s", meterPrefix, run)),
		Description: new("collects the next settlement window, once a minute"),
		// AWS::Scheduler::Schedule is not carried by Cloud Control on
		// fakecloud, so a rule plus an invoke permission it is.
		ScheduleExpression: new("rate(1 minute)"),
		State:              new(events.RuleStateENABLED),
		Targets: []events.Target{{
			Id:  new("meter-feed"),
			Arn: new(functionArn(meterRef)),
		}},
	})
	if err != nil {
		return fmt.Errorf("meter schedule: %w", err)
	}
	scheduleRef = rule
	return nil
}

func makeInvokePermission() error {
	if _, err := aws.Create(&lambda.Permission{
		FunctionName: new(meterRef),
		Action:       new("lambda:InvokeFunction"),
		Principal:    new("events.amazonaws.com"),
		SourceArn:    new(ruleArn(scheduleRef)),
	}); err != nil {
		return fmt.Errorf("meter invoke permission: %w", err)
	}
	return nil
}

// --- arn resolution -----------------------------------------------------------

func queueArn(identifier string) string {
	if identifier == "" || strings.HasPrefix(identifier, "arn:") {
		return identifier
	}
	queue, err := aws.Read[*sqs.Queue](identifier)
	if err != nil || queue == nil || queue.Arn == nil || *queue.Arn == "" {
		return ""
	}
	return *queue.Arn
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

// baseline records what the account already contains. Errors are swallowed: a
// service that will not list is one where nothing was there to inherit, and
// treating that as a bootstrap failure would take the challenge down over an
// empty account.
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

// --- the handover note ---------------------------------------------------------

// handoverNote is what the outsourced team left behind. It names the pieces,
// admits what they knew was wrong, and contains the two facts that together
// give the whole game away — without ever putting them next to each other.
func handoverNote() string {
	return strings.Join([]string{
		"ridgeline settlement - handover. Last day, so this is what there is.",
		"",
		"Flow is meters -> intake -> validation -> settlement queue -> settlement -> ledger.",
		"Everything we built is tagged " + systemTag + "=" + systemName + ".",
		"",
		"Both stages came out during the transition and were never redeployed. Platform",
		"has the package. Execution roles for them are already in the account, scoped to",
		"the right queues - we set those up before we lost iam.",
		"",
		"The ledger is keyed on reading_id. reading_id is the meter interval. It is not",
		"the message and it is not the run. We were quite careful about that and it is",
		"the only thing in here we were careful about.",
		"",
		"Reconciliation writes to " + reconPath + " every minute. It counts readings that",
		"settled once and readings that settled more than once, separately. The second",
		"number should be zero. It has never been exactly zero.",
		"",
		"Known and not fixed: the reading archive is wide open, the ledger has no PITR,",
		"and a reading validation rejects has nowhere to go - it just ages out.",
		"",
		"Ops keep asking whether it is safe for a substation logger to flush its backlog",
		"after an outage. We keep not answering. - J & K",
	}, "\n")
}

func handoverAsset() string {
	return "# Ridgeline Settlement — handover\n\n```\n" + handoverNote() + "\n```\n"
}

// --- Act I --------------------------------------------------------------------

func intakeOwned() (bool, error) {
	queue, err := readIntake()
	if err != nil {
		return false, err
	}
	return sqsOwned(queue.Tags), nil
}

func settlementQueueOwned() (bool, error) {
	queue, err := readSettlementQueue()
	if err != nil {
		return false, err
	}
	return sqsOwned(queue.Tags), nil
}

func sqsOwned(tags []sqs.Tag) bool {
	for _, tag := range tags {
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

func archiveOwned() (bool, error) {
	bucket, err := readArchive()
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

func settlementLogsRetained() (bool, error) {
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

// --- Act II -------------------------------------------------------------------

func validationDeployed() (bool, error) { return stageDeployed(intakeArn) }

func settlementDeployed() (bool, error) { return stageDeployed(settleArn) }

// stageDeployed is "a function the player built is reading this queue". Not by
// name: the player is deploying, not restoring, and prescribing a function name
// would be checking the answer rather than the property. The meter feed is in
// the baseline, so it cannot satisfy this itself.
func stageDeployed(source string) (bool, error) {
	if source == "" {
		return false, fmt.Errorf("queue arn was never resolved")
	}
	functions, err := newFunctions()
	if err != nil {
		return false, err
	}
	if len(functions) == 0 {
		return false, nil
	}
	wired, err := mappedFunctions(source)
	if err != nil {
		return false, err
	}
	for identifier := range functions {
		for _, target := range wired {
			if matchesFunction(identifier, target) {
				return true, nil
			}
		}
	}
	return false, nil
}

func intakeWired() (bool, error) { return hopWired(intakeArn) }

func settlementWired() (bool, error) { return hopWired(settleArn) }

func hopWired(source string) (bool, error) {
	if source == "" {
		return false, fmt.Errorf("queue arn was never resolved")
	}
	targets, err := mappedFunctions(source)
	if err != nil {
		return false, err
	}
	return len(targets) > 0, nil
}

// mappedFunctions returns the functions an enabled event source mapping points
// at for this queue, excluding the scenario's own feed.
//
// Enabled is checked defensively: Cloud Control does not always return it, and
// a nil Enabled is read as enabled rather than as a reason to fail a check the
// player has legitimately satisfied.
func mappedFunctions(source string) ([]string, error) {
	mappings, err := aws.List[*lambda.EventSourceMapping]()
	if err != nil {
		return nil, err
	}
	targets := []string{}
	for _, mapping := range mappings {
		if mapping == nil || mapping.EventSourceArn == nil || mapping.FunctionName == nil {
			continue
		}
		if *mapping.EventSourceArn != source {
			continue
		}
		if mapping.Enabled != nil && !*mapping.Enabled {
			continue
		}
		if isMeterFeed(*mapping.FunctionName) {
			continue
		}
		targets = append(targets, *mapping.FunctionName)
	}
	return targets, nil
}

func isMeterFeed(reference string) bool {
	return strings.Contains(reference, meterPrefix)
}

func matchesFunction(identifier, target string) bool {
	if identifier == "" || target == "" {
		return false
	}
	return identifier == target ||
		strings.HasSuffix(target, ":"+identifier) ||
		strings.HasSuffix(identifier, ":"+target)
}

// settlementLanding awards on readings that settled cleanly since the last look.
// The first observation only establishes the baseline: there is no way to tell
// how much of an already-nonzero counter this player earned.
func settlementLanding() func() (bool, error) {
	last := -1
	return func() (bool, error) {
		state, ok, err := readReconciliation()
		if err != nil || !ok {
			return false, err
		}
		if last < 0 {
			last = state.exact
			return false, nil
		}
		if state.exact <= last {
			return false, nil
		}
		last = state.exact
		return true, nil
	}
}

// doublesAppearing is the negative check: money moved twice since the last look.
// It runs on the same cadence as settlementLanding so that a pipeline producing
// both gets paid and charged in the same cycle and the player can see the net.
func doublesAppearing() func() (bool, error) {
	last := -1
	return func() (bool, error) {
		state, ok, err := readReconciliation()
		if err != nil || !ok {
			return false, err
		}
		if last < 0 {
			last = state.double
			return false, nil
		}
		if state.double <= last {
			return false, nil
		}
		last = state.double
		return true, nil
	}
}

// settlingExactlyOnce is a settled window in which readings settled and none of
// them settled twice. Both halves matter: requiring progress stops it passing on
// a pipeline that is not running, and requiring a flat double counter stops it
// passing on one that is settling a mixture — which is the state a player
// reaches by deploying the package unchanged.
func settlingExactlyOnce() func() (bool, error) {
	lastExact, lastDouble := -1, -1
	return func() (bool, error) {
		state, ok, err := readReconciliation()
		if err != nil || !ok {
			return false, err
		}
		if lastExact < 0 {
			lastExact, lastDouble = state.exact, state.double
			return false, nil
		}
		exact, double := state.exact-lastExact, state.double-lastDouble
		lastExact, lastDouble = state.exact, state.double
		return exact > 0 && double <= 0, nil
	}
}

// settlementRunning gates Act III: the backlog cannot arrive before there is a
// pipeline for it to arrive at, or the player is punished for still being in
// Act II.
func settlementRunning() func() (bool, error) {
	return func() (bool, error) {
		if backlogArrived.Load() {
			return true, nil
		}
		state, ok, err := readReconciliation()
		if err != nil || !ok {
			return false, err
		}
		return state.exact > 0, nil
	}
}

// --- Act III: the backlog ------------------------------------------------------

// replayFloor is the reconciliation window at which the backlog started, so the
// coda check can wait for it to actually be in the pipeline before judging it.
var replayFloor atomic.Int64

// theBacklog is the twist, and it is the "break something" kind of event —
// carefully, and only something the player can recover from. It switches the
// meter feed into replay mode by rewriting its environment, which is a thing the
// plugin can do through the control plane and the player cannot do at all.
//
// It announces itself, because an unexplained flood of duplicate settlements
// reads as a platform bug and the player will spend the rest of the gameday
// debugging the scenario instead of their pipeline.
func theBacklog(ctx context.Context, s *challenge.Scenario) error {
	if !backlogArrived.CompareAndSwap(false, true) {
		return nil
	}

	if state, ok, err := readReconciliation(); err == nil && ok {
		replayFloor.Store(int64(state.window))
	}

	s.AddDescription(
		"04:05. Elkhorn's data logger has been offline since Sunday with a backhaul fault. " +
			"The line was repaired twenty minutes ago and the logger is doing exactly what " +
			"it is built to do: flushing everything it buffered while it was dark, as fast " +
			"as the link will carry it.")
	s.AddDescription(
		"Those readings are not new. Every one of them is an interval this platform has " +
			"already seen, and depending on what your settlement stage does when it meets a " +
			"reading it has settled before, this is either a quiet hour or four million " +
			"pounds of duplicate payment instructions going to the clearing bank at 06:00. " +
			"The reconciliation will tell you which. It is going to keep telling you every " +
			"minute until you make it stop.")

	s.AddClue("the backlog is settling twice",
		"An at-least-once queue was always going to deliver something twice; the backlog "+
			"just made it happen thousands of times at once. The ledger is keyed on the "+
			"reading. A write that refuses to overwrite a reading that is already there is "+
			"the fix — and deciding what the stage does when that write is refused is the "+
			"half that decides whether the queue drains or stalls.",
		-45)
	s.AddClue("what she will want in writing",
		"Put the settlement exception report in "+exceptionPath+". It has to quote two "+
			"things: the arn of the queue rejected readings now land in, and the id of the "+
			"key the reading archive is encrypted under.",
		-20)

	s.AddCheck("Absorbed the backlog without paying twice", challenge.Check{
		Points:  80,
		Every:   30 * time.Second,
		Trigger: absorbedTheBacklog(),
	})
	s.AddCheck("Gave rejected readings somewhere to land", challenge.Check{
		Points:  50,
		Every:   20 * time.Second,
		Trigger: intakeArmed,
	})
	s.AddCheck("Made the settled ledger recoverable", challenge.Check{
		Points:  45,
		Every:   20 * time.Second,
		Trigger: ledgerRecoverable,
	})
	s.AddCheck("Protected the settled ledger from deletion", challenge.Check{
		Points:  25,
		Every:   20 * time.Second,
		Trigger: ledgerProtected,
	})
	s.AddCheck("Put the reading archive under a key Ridgeline controls", challenge.Check{
		Points:  55,
		Every:   20 * time.Second,
		Trigger: archiveEncrypted,
	})
	s.AddCheck("Filed the settlement exception report", challenge.Check{
		Points:  55,
		Every:   20 * time.Second,
		Trigger: exceptionReportFiled,
	})

	// switch the feed into replay. Environment is rewritten whole because
	// Update is a shallow patch: setting one variable would drop the rest and
	// leave the feed pointing at nothing.
	if meterRef == "" {
		return fmt.Errorf("meter feed was never provisioned")
	}
	if err := aws.Update(meterRef, &lambda.Function{
		Environment: meterEnvironment(1),
	}); err != nil {
		return fmt.Errorf("switch meter feed to replay: %w", err)
	}
	slog.Info("backlog replay enabled on the meter feed")
	return nil
}

// absorbedTheBacklog is the check the twist exists for: the replay is in the
// pipeline and readings are still settling exactly once.
//
// It waits replayGrace windows after the switch before it starts looking,
// because the feed has to actually run before there is anything to judge, and a
// check that reads "did you survive it" before it has happened is a check that
// awards for nothing.
func absorbedTheBacklog() func() (bool, error) {
	lastExact, lastDouble := -1, -1
	return func() (bool, error) {
		state, ok, err := readReconciliation()
		if err != nil || !ok {
			return false, err
		}
		if state.replayed <= 0 {
			return false, nil
		}
		if int64(state.window) < replayFloor.Load()+replayGrace {
			return false, nil
		}
		if lastExact < 0 {
			lastExact, lastDouble = state.exact, state.double
			return false, nil
		}
		exact, double := state.exact-lastExact, state.double-lastDouble
		lastExact, lastDouble = state.exact, state.double
		return exact > 0 && double <= 0, nil
	}
}

func intakeArmed() (bool, error) {
	target, count, err := redrive()
	if err != nil {
		return false, err
	}
	if target == "" {
		return false, nil
	}
	// a maxReceiveCount of 1 gives a transient failure no second chance, and an
	// unbounded one is not a dead letter queue at all.
	return count >= 2 && count <= 10, nil
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

// archiveEncrypted wants a key Ridgeline controls, specifically not the one AWS
// hands out. "Is there any default encryption" is worth nothing: since January
// 2023 S3 applies SSE-S3 to every new bucket, so that check is true the instant
// bootstrap creates it. aws:kms is the state that has to be reached
// deliberately.
func archiveEncrypted() (bool, error) {
	bucket, err := readArchive()
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

// exceptionReportFiled is the one check that reads free text, so it is
// deliberately forgiving about everything except the two identifiers.
func exceptionReportFiled() (bool, error) {
	value, ok, err := parameter(exceptionPath)
	if err != nil || !ok {
		return false, err
	}
	target, _, err := redrive()
	if err != nil {
		return false, err
	}
	if target == "" || !strings.Contains(value, target) {
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

func backlogClosedOut() (bool, error) {
	if !backlogArrived.Load() {
		return false, nil
	}
	if codaOpen.Load() {
		return true, nil
	}
	filed, err := exceptionReportFiled()
	if err != nil || !filed {
		return false, err
	}
	return archiveEncrypted()
}

func theMorningRun(ctx context.Context, s *challenge.Scenario) error {
	if !codaOpen.CompareAndSwap(false, true) {
		return nil
	}

	s.AddDescription(
		"05:30. The backlog is through, the exception report is filed, and the clearing " +
			"bank takes the ledger as it stands at 06:00. There is nothing left to build. " +
			"Points accrue for every window that settles exactly once and keeps up with the " +
			"meters, and they stop the moment either stops being true.")

	s.AddCheck("Kept settlement exact", challenge.Check{
		Points:  heldPoints,
		Every:   30 * time.Second,
		Repeat:  true,
		Trigger: bounded(heldRounds, settlingExactlyOnce()),
	})
	s.AddCheck("Kept up with the meters", challenge.Check{
		Points:  codaPoints,
		Every:   30 * time.Second,
		Repeat:  true,
		Trigger: bounded(codaRounds, keepingUp()),
	})
	return nil
}

func keepingUp() func() (bool, error) {
	lastSent, lastExact := -1, -1
	return func() (bool, error) {
		state, ok, err := readReconciliation()
		if err != nil || !ok {
			return false, err
		}
		if lastSent < 0 {
			lastSent, lastExact = state.sent, state.exact
			return false, nil
		}
		sent, exact := state.sent-lastSent, state.exact-lastExact
		lastSent, lastExact = state.sent, state.exact
		if sent <= 0 {
			return false, nil
		}
		return exact*1000 >= sent*keepingUpPerMille, nil
	}
}

// bounded caps how many times a Repeat check may pay out — and, for the negative
// one, how much it may cost.
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

// --- reads ----------------------------------------------------------------------

// Every read returns an error rather than a nil struct when the resource is not
// there. There is no recover around a trigger: one nil dereference takes the
// plugin down and the player's game is over.

func readArchive() (*s3.Bucket, error) {
	if archiveRef == "" {
		return nil, fmt.Errorf("reading archive was never provisioned")
	}
	bucket, err := aws.Read[*s3.Bucket](archiveRef)
	if err != nil {
		return nil, err
	}
	if bucket == nil {
		return nil, fmt.Errorf("reading archive is not readable")
	}
	return bucket, nil
}

func readIntake() (*sqs.Queue, error) { return readQueue(intakeRef, "meter intake") }

func readSettlementQueue() (*sqs.Queue, error) { return readQueue(settleRef, "settlement queue") }

func readQueue(reference, label string) (*sqs.Queue, error) {
	if reference == "" {
		return nil, fmt.Errorf("%s was never provisioned", label)
	}
	queue, err := aws.Read[*sqs.Queue](reference)
	if err != nil {
		return nil, err
	}
	if queue == nil {
		return nil, fmt.Errorf("%s is not readable", label)
	}
	return queue, nil
}

func readLedger() (*dynamodb.Table, error) {
	if ledgerRef == "" {
		return nil, fmt.Errorf("settled ledger was never provisioned")
	}
	table, err := aws.Read[*dynamodb.Table](ledgerRef)
	if err != nil {
		return nil, err
	}
	if table == nil {
		return nil, fmt.Errorf("settled ledger is not readable")
	}
	return table, nil
}

func readLogGroup() (*logs.LogGroup, error) {
	if logGroupRef == "" {
		return nil, fmt.Errorf("settlement logs were never provisioned")
	}
	group, err := aws.Read[*logs.LogGroup](logGroupRef)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("settlement logs are not readable")
	}
	return group, nil
}

// reconciliation is one settled window.
type reconciliation struct {
	window   int
	sent     int
	exact    int
	double   int
	replayed int
	rate     int
}

// readReconciliation parses the reconciliation parameter. The false return is
// "not readable yet", not an error: on a cold account the feed has not run, and
// a trigger that reported that as a failure would report one every cycle until
// the first window closed.
func readReconciliation() (reconciliation, bool, error) {
	value, ok, err := parameter(reconPath)
	if err != nil || !ok {
		return reconciliation{}, false, err
	}
	state := reconciliation{}
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
		case "exact":
			state.exact = number
		case "double":
			state.double = number
		case "replayed":
			state.replayed = number
		case "rate":
			state.rate = number
		}
	}
	return state, true, nil
}

// redrive returns the dead letter target and receive count off the meter intake.
// Cloud Control hands the policy back as a nested object in some environments
// and as a json string in others, so both are accepted.
func redrive() (string, int, error) {
	queue, err := readIntake()
	if err != nil {
		return "", 0, err
	}
	raw := []byte(queue.RedrivePolicy)
	if len(raw) == 0 {
		return "", 0, nil
	}
	var nested string
	if err := json.Unmarshal(raw, &nested); err == nil {
		raw = []byte(nested)
	}
	var parsed struct {
		DeadLetterTargetArn string      `json:"deadLetterTargetArn"`
		MaxReceiveCount     json.Number `json:"maxReceiveCount"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		// a policy the player wrote by hand and got wrong is their problem to
		// see, not an error worth reporting every cycle.
		return "", 0, nil
	}
	count, _ := strconv.Atoi(parsed.MaxReceiveCount.String())
	return parsed.DeadLetterTargetArn, count, nil
}

// parameter reads a parameter the player may not have written yet.
//
// It lists first and only reads when the identifier is there. "Not there yet"
// is the normal state of a parameter the player has not written, and going
// straight to Read would log a 404 host side every cycle until they do. The
// value cannot be taken from the list itself: a Cloud Control list handler is
// only obliged to return primary identifiers, and whether the properties come
// with them is environment-dependent.
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

// appeared returns what was not in the account when the night started, read
// back individually — List is only guaranteed to hand back identifiers. A
// resource that cannot be read is skipped rather than failing the check; it is
// usually one that is still being created.
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
