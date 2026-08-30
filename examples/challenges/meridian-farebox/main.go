//go:build wasip1

// Command meridian-farebox is a gameday tier cloudjam challenge.
//
// Meridian Transit Authority does not run its own fare gates. For five years
// that was a concessionaire's job: gate hardware they owned, an edge service
// that authenticated every tap before it went anywhere near the authority's
// AWS account, and a settlement stage that turned validated taps into money
// owed to the authority. The concession ended at midnight. Per the contract,
// the concessionaire's access was revoked at the same moment — but revoking
// a login and revoking a gate's ability to correctly sign a request are two
// different operations, and the handover only did one of them.
//
// The pipeline is a real HTTP request pipeline, not a queue-to-queue one: a
// fare gate POSTs a tap to whatever the player publishes as the edge
// address, and nothing about that address is prescribed. The plugin cannot
// speak HTTP and cannot see the player's compute — it only ever learns
// whether the pipeline is working by reading a farebox ledger and a tally
// through Cloud Control. Between the edge and the ledger sits an ingest
// front door the scenario owns, so that whatever the player runs their edge
// on never needs an AWS credential of its own — only a place to run and a
// shared secret.
//
// What makes this a gameday rather than a checklist is that the edge the
// player is handed is a real, correct-looking Go program with one line that
// throws away the only check that matters, and the settlement stage it feeds
// settles every tap unconditionally. A player who deploys both unmodified
// gets a pipeline that looks alive and a scoreboard going backwards, for
// reasons the briefing never spells out. The story runs in three acts and a
// coda; Act III is fired the moment a tap the pipeline actually trusts lands
// clean, because a badge nobody revoked does not announce itself before
// someone uses it.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/kms"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/lambda"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/logs"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/s3"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/sqs"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/ssm"
	"github.com/google/uuid"
)

// ownerTag is what "taking ownership" means mechanically. The estate is
// untagged — the concessionaire never tagged anything they did not have to —
// and the authority's asset register only counts what carries an owner. Any
// non-empty value counts.
const ownerTag = "meridian:owner"

// systemTag is on every resource bootstrap provisions, so the player can find
// the estate at all. It is never checked; it is the discovery affordance.
const systemTag = "meridian:system"

const systemName = "farebox"

// gatePrefix names everything the scenario owns and the player does not: the
// ingest front door and the gate network generator. The guardrail denies
// write on this prefix by pattern, which is why it has to be a constant and
// not something built from the run id.
const gatePrefix = "meridian-gate-"

// Parameter paths. handover is written by bootstrap and is the player's way
// in; telemetry is written by the gate network and is read-only to the
// player; secret and ingestEndpoint are written by bootstrap and are
// read-only — the player reads them to configure the edge but cannot rewrite
// either, which is what stops a player from simply forging their own
// "verified" traffic. edgeEndpoint and incident are written by the player.
const (
	handoverPath       = "/meridian/handover"
	telemetryPath      = "/meridian/telemetry"
	secretPath         = "/meridian/signing-secret"
	ingestEndpointPath = "/meridian/ingest-endpoint"
	edgeEndpointPath   = "/meridian/edge-endpoint"
	incidentPath       = "/meridian/incident-report"
)

// How hard bootstrap tries to build a Lambda on a role IAM has only just
// created. This is a propagation race, not a flaky API, and it resolves in
// seconds or not at all.
const (
	roleRetries    = 6
	roleRetryDelay = 5 * time.Second
)

// Retention the authority's records office will accept.
const (
	minRetentionDays = 30
	maxRetentionDays = 400
)

// The gate network's shape. Volume climbs from baseRate to peakRate over
// rampWindows one-minute windows. Of every thousand taps sent, forgedPerMille
// carry a garbage signature and misreadPerMille carry a real one on a fare no
// card reader could produce; the rest are ordinary.
const (
	baseRate        = 20
	peakRate        = 70
	rampWindows     = 16
	forgedPerMille  = 60
	misreadPerMille = 40
)

// Bounds on the repeating checks, in both directions. Repeat checks never
// retire, so without a cap the maximum score does not exist — and neither
// does the maximum loss on the negative ones.
const (
	edgeRounds       = 8
	edgePoints       = 10
	throughputRounds = 10
	throughputPoints = 10
	forgedRounds     = 8
	forgedPoints     = -15
	replayRounds     = 8
	replayPoints     = -18
	heldRounds       = 8
	heldPoints       = 8
	codaRounds       = 6
	codaPoints       = 10
)

// keepingUpPerMille is how much of what the gate network sent has to settle
// cleanly for the coda to count the pipeline as keeping up. Not 100%: a batch
// in flight when the window closes is normal.
const keepingUpPerMille = 550

// replayGrace is how many settlement windows the player gets after the
// concessionaire's badge starts being used again before "held the line"
// starts looking. The replay has to actually reach the pipeline before it is
// fair to judge what happened to it.
const replayGrace = 2

// Primary identifiers of everything bootstrap provisions. bootstrap runs to
// completion before the first check cycle, so triggers may read these.
var (
	archiveRef    string
	decoyRef      string
	intakeRef     string
	intakeArn     string
	ledgerRef     string
	ledgerArn     string
	ledgerLabel   string
	logGroupRef   string
	settlementArn string
	ingestRole    string
	generatorRole string
	ingestRef     string
	generatorRef  string
	scheduleRef   string
)

// Captured before bootstrap provisions anything, so that "did the player
// create one of these" is a meaningful question on an account that was not
// empty.
var (
	keyBaseline      = map[string]bool{}
	functionBaseline = map[string]bool{}
)

var (
	badgeUsed atomic.Bool
	codaOpen  atomic.Bool
)

func main() {
	challenge.New("Meridian Transit: Farebox Clearance", 10*time.Second, bootstrap).
		AddDescription(
			"Meridian Transit Authority runs six rail lines and does not, and has never, run "+
				"its own fare gates. For five years that was Concourse Systems' job under a "+
				"concession contract: the gate hardware, the edge service that authenticates "+
				"every tap before it is trusted, and the settlement stage that turns a validated "+
				"tap into money the authority is owed. About two hundred thousand taps a day.").
		AddDescription(
			"The concession ended at midnight last night, on schedule, and was not renewed. "+
				"The contract requires Concourse's systems access to be revoked at the same "+
				"moment. It was — every credential Concourse held into this account is gone. "+
				"What the contract does not mention, because nobody thought to, is that revoking "+
				"a login and revoking a gate's ability to correctly sign a request are two "+
				"different operations.").
		AddDescription(
			"You are the engineer the authority has on call. Riders are tapping in right now, "+
				"on trains that do not stop running because a vendor's contract lapsed, and "+
				"nothing is validating what those taps say. There is no architecture diagram "+
				"and no runbook — everything Concourse built carries a "+systemTag+"="+
				systemName+" tag, and that tag is the entire asset register.").
		AddDescription(
			"The gate network reports what it sees to "+telemetryPath+" once a minute: taps "+
				"sent, taps that settled cleanly, and taps that should never have settled at "+
				"all. That parameter is your dashboard. You can read it. You cannot write it.").
		AddDescription(
			"Nobody is going to hand you a task list, because nobody wrote one. You are scored "+
				"the way the authority's finance office measures this: on fares collected "+
				"correctly, and against fares that should never have been collected in the "+
				"first place. There is more here that could be fixed than there is shift left to "+
				"fix it in, and deciding what actually matters is the whole job.").
		// clue prices are added to the team score, so they are negative.
		AddClue("where do i start",
			"The handover note is an SSM parameter at "+handoverPath+". Read it before you "+
				"touch anything: aws ssm get-parameter --name "+handoverPath+".",
			-5).
		AddClue("nothing is validating the taps",
			"Concourse's edge and settlement code went with them. Platform kept a copy — it "+
				"is attached to this challenge as a downloadable package, and building and "+
				"running the edge is on you: it is an ordinary Go program and nothing "+
				"downstream cares what it runs on.",
			-30).
		AddClue("i cannot create an execution role",
			"You do not have iam:CreateRole and you are not meant to. The settlement stage's "+
				"execution role is already provisioned in this account, scoped for exactly "+
				"this. Find it with aws iam list-roles and pass it.",
			-20).
		AddClue("what counts as ownership",
			"The asset register only counts resources carrying a "+ownerTag+" tag. Any value "+
				"will do — the point is that somebody's name is on it.",
			-20).
		AddClue("taps are landing but the farebox looks wrong",
			"Read the edge's source line by line, not just its shape. It computes the right "+
				"thing and then does nothing with the answer — which is exactly the kind of "+
				"bug that leaves every log line looking healthy.",
			-40).
		AddClue("what is a forged tap worth",
			"Less than nothing. It is a fare charged for a ride that never happened, on a "+
				"signature nobody at Meridian ever made. The scoreboard prices it the way the "+
				"finance office would.",
			-15).
		AddClue("what would the authority's auditor go for first",
			"Ask what cannot be undone rather than what is untidy. A fare already charged, a "+
				"ledger with no way back from a bad shift, and gate footage the whole internet "+
				"can read.",
			-25).
		SetPermission(policy.Document{
			Version: policy.Version20121017,
			Statement: []policy.Statement{
				{Effect: policy.Deny, Action: policy.ActionAll, Resource: policy.ARNAll},
			},
		}).
		SetGuardrail(guardrail()).
		// --- Act I: whose account is this now ---------------------------
		AddCheck("Signed for the intake queue", challenge.Check{
			Points:  25,
			Every:   20 * time.Second,
			Trigger: intakeOwned,
		}).
		AddCheck("Signed for the farebox ledger", challenge.Check{
			Points:  25,
			Every:   20 * time.Second,
			Trigger: ledgerOwned,
		}).
		AddCheck("Signed for the gate footage archive", challenge.Check{
			Points:  25,
			Every:   20 * time.Second,
			Trigger: archiveOwned,
		}).
		// The nightly reports bucket is a decoy: Concourse emailed a PDF out
		// of it once a night and nobody has opened one in years. It is not
		// wired into anything and never will be. Tagging it is worth almost
		// nothing, which is the point — some of what is lying around is not
		// worth the shift.
		AddCheck("Signed for the nightly reports bucket", challenge.Check{
			Points:  10,
			Every:   20 * time.Second,
			Trigger: decoyOwned,
		}).
		AddCheck("Confirmed the handover", challenge.Check{
			Points:  45,
			Every:   20 * time.Second,
			Trigger: handoverConfirmed,
		}).
		AddCheck("Gave the gate-ops logs a retention window", challenge.Check{
			Points:  10,
			Every:   30 * time.Second,
			Trigger: gateOpsLogsRetained,
		}).
		// --- Act II: get the gates trusted again -------------------------
		AddCheck("Deployed the settlement stage", challenge.Check{
			Points:  70,
			Every:   20 * time.Second,
			Trigger: settlementDeployed,
		}).
		AddCheck("Wired the intake queue into settlement", challenge.Check{
			Points:  55,
			Every:   20 * time.Second,
			Trigger: settlementWired,
		}).
		AddCheck("Armed the intake queue with a dead-letter queue", challenge.Check{
			Points:  70,
			Every:   20 * time.Second,
			Trigger: intakeArmed,
		}).
		AddCheck("The gate edge is answering", challenge.Check{
			Points:  edgePoints,
			Every:   30 * time.Second,
			Repeat:  true,
			Trigger: bounded(edgeRounds, edgeAnswering()),
		}).
		AddCheck("Fares are settling", challenge.Check{
			Points:  throughputPoints,
			Every:   30 * time.Second,
			Repeat:  true,
			Trigger: bounded(throughputRounds, faresSettling()),
		}).
		AddCheck("Signatures enforced end to end", challenge.Check{
			Points:  100,
			Every:   30 * time.Second,
			Trigger: signaturesEnforced(),
		}).
		// The only Act II negative check, and the reason this is a gameday.
		// It is not something an event did to the player — it fires because
		// the edge they put into service forwards everything it receives,
		// which is a decision, and it stops the moment they fix it.
		AddCheck("Forged taps reaching the farebox ledger", challenge.Check{
			Points:  forgedPoints,
			Every:   30 * time.Second,
			Repeat:  true,
			Trigger: bounded(forgedRounds, forgedLanding()),
		}).
		AddEvent("concourse-badge", challenge.Event{
			Every:   30 * time.Second,
			Trigger: firstCleanFare(),
			Event:   theBadge,
		}).
		AddEvent("closeout", challenge.Event{
			Every:   30 * time.Second,
			Trigger: badgeClosedOut,
			Event:   theCloseout,
		}).
		Start()
}

// --- permissions --------------------------------------------------------------

// workingSet is the permission envelope for the whole scenario.
func workingSet() policy.Actions {
	return policy.Actions{
		// The gate footage archive and the nightly reports decoy: tags,
		// encryption, public access block, versioning.
		//
		// Put* also settles a naming disagreement. fakecloud authorises two
		// of these calls under their api operation name
		// (s3:PutBucketEncryption, s3:PutPublicAccessBlock) while real aws
		// uses the iam action name (s3:PutEncryptionConfiguration,
		// s3:PutBucketPublicAccessBlock).
		"s3:Get*", "s3:List*", "s3:Put*",

		// The pipeline. The player creates a dead letter queue for rejected
		// taps, so this needs create rights; sqs is billed per request.
		"sqs:*",

		// The settlement stage: create the function, wire the event source
		// mapping. The player may also choose to run their edge as a Lambda
		// behind its own Function URL — nothing here stops that, it is just
		// one of the places an edge can run. The guardrail carves the gate
		// network's own functions back out.
		"lambda:*",

		// Passing the pre-provisioned settlement role, and finding it. The
		// player never mints a role, which is what keeps them inside the
		// boundary they were given.
		"iam:PassRole", "iam:GetRole", "iam:ListRoles",
		"iam:ListRolePolicies", "iam:GetRolePolicy", "iam:ListAttachedRolePolicies",

		// The farebox ledger: recovery, deletion protection, tags.
		// Deliberately not CreateTable — nothing here needs a new one. Item
		// writes are denied in the guardrail so that a fare cannot be typed
		// in by hand.
		"dynamodb:Describe*", "dynamodb:List*",
		"dynamodb:UpdateTable", "dynamodb:UpdateContinuousBackups",
		"dynamodb:UpdateTimeToLive", "dynamodb:TagResource", "dynamodb:UntagResource",

		"logs:*", "kms:*",

		// The edge endpoint the player publishes, and the incident report.
		// The signing secret and the ingest endpoint are readable under this
		// same prefix and denied write in the guardrail.
		"ssm:Describe*", "ssm:Get*", "ssm:PutParameter", "ssm:DeleteParameter",
		"ssm:AddTagsToResource", "ssm:ListTagsForResource",

		// Reading the schedule the gate network runs on. Writing it is denied.
		"events:Describe*", "events:List*",
	}
}

// guardrail is the permission boundary. Patterns rather than arns, because
// the boundary is published before bootstrap has run and neither the account
// id nor the run id is known yet. Service wildcards rather than generated
// action groups, because the boundary is a 6144 character managed policy.
func guardrail() policy.Document {
	return policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{
				Sid:      "FareboxClearance",
				Effect:   policy.Allow,
				Action:   workingSet(),
				Resource: policy.ARNAll,
			},
			{
				// the ingest front door and the gate network generator. A
				// player who can rewrite or invoke either can forge their own
				// "verified" traffic straight past the edge they are
				// supposed to be fixing.
				Sid:    "TheGateNetworkIsNotYours",
				Effect: policy.Deny,
				Action: policy.Actions{
					"lambda:UpdateFunctionCode", "lambda:UpdateFunctionConfiguration",
					"lambda:DeleteFunction", "lambda:InvokeFunction",
					"lambda:AddPermission", "lambda:RemovePermission",
					"lambda:UpdateFunctionUrlConfig", "lambda:DeleteFunctionUrlConfig",
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
				// the signing secret and the ingest front door's address.
				// Readable so the player can configure the edge with them;
				// not writable, or a player could hand their edge a secret
				// of their own choosing and sign forged traffic that the
				// gate network would never distinguish from real.
				Sid:    "SecretAndIngestAreReadOnly",
				Effect: policy.Deny,
				Action: policy.Actions{
					"ssm:PutParameter", "ssm:DeleteParameter", "ssm:DeleteParameters",
					"ssm:LabelParameterVersion",
				},
				Resource: policy.ARNs{
					policy.ARN("arn:aws:ssm:*:*:parameter" + secretPath),
					policy.ARN("arn:aws:ssm:*:*:parameter" + ingestEndpointPath),
				},
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

// --- AWS::IAM::Role ------------------------------------------------------------

// pkg/challenge/aws/services/iam carries action constants and no resource
// types, so there is no iam.Role to import. Cloud Control does support
// AWS::IAM::Role and aws.Resource is satisfied by anything with a
// CloudJamType, so the type is declared here.
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

// roleArn resolves a role's arn from whatever Create handed back. Cloud
// Control's identifier for AWS::IAM::Role is the role name, but Lambda's Role
// property is validated against an arn pattern, and the arn is not
// assemblable because the plugin is never told the account id.
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

// --- bootstrap -------------------------------------------------------------

// parallel runs independent bootstrap steps together and joins what failed.
//
// It does not overlap them today and cannot: a //go:wasmimport call blocks
// the whole wasm instance, because GOOS=wasip1 has no threads and the Go
// scheduler gets no chance to switch goroutines while a host call is in
// flight. It is written this way because the waves are the actual dependency
// graph of the estate, and because the day the host carries host calls
// concurrently this gets the time back for free.
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

	// wave one: the inherited estate. Nothing here needs anything else to
	// exist. Telemetry is zeroed rather than left absent so a player who
	// opens their dashboard before the gate network has run once sees zeroes
	// instead of a 404.
	if err := parallel(
		func() error { return makeArchive(run) },
		func() error { return makeDecoyBucket(run) },
		func() error { return makeIntake(run) },
		func() error { return makeLedger(run) },
		func() error { return makeLogGroup(run) },
		func() error { return makeTelemetry() },
		func() error { return makeSecret() },
		func() error { return makeHandover() },
	); err != nil {
		return err
	}

	// wave two: every role is scoped to arns the first wave produced.
	if err := parallel(
		func() error { return makeSettlementRole(run) },
		func() error { return makeIngestRole(run) },
		func() error { return makeGeneratorRole(run) },
	); err != nil {
		return err
	}

	// waves three onward: each step's input is the previous one's output and
	// none of it collapses into parallel work.
	if err := makeIngestFunction(run); err != nil {
		return err
	}
	ingestUrl, err := makeIngestUrl()
	if err != nil {
		return err
	}
	if err := makeIngestPermission(); err != nil {
		return err
	}
	if err := makeIngestEndpointParameter(ingestUrl); err != nil {
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

	s.AddAsset("meridian-handover.md", []byte(handoverAsset()))
	if pkg := pipelinePackage(); len(pkg) > 0 {
		s.AddAsset("meridian-pipeline.zip", pkg)
	}

	s.SetPermission(policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{
				Sid:      "FareboxClearance",
				Effect:   policy.Allow,
				Action:   workingSet(),
				Resource: policy.ARNAll,
			},
		},
	})
	return nil
}

// --- wave one: the inherited estate ------------------------------------------

func makeArchive(run string) error {
	bucket, err := aws.Create(&s3.Bucket{
		BucketName: new(fmt.Sprintf("meridian-gate-footage-%s", run)),
		// the scenario: gate camera snapshots, readable by anyone who
		// guesses the name, in the clear, with no way back from an
		// overwrite.
		PublicAccessBlockConfiguration: &s3.BucketPublicAccessBlockConfiguration{
			BlockPublicAcls:       new(false),
			BlockPublicPolicy:     new(false),
			IgnorePublicAcls:      new(false),
			RestrictPublicBuckets: new(false),
		},
		Tags: []s3.BucketTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("gate footage archive: %w", err)
	}
	archiveRef = bucket
	return nil
}

// makeDecoyBucket provisions the low-value trap task: a nightly reports
// bucket nothing reads from and nothing writes to anymore. It exists so that
// "tag everything you own" has at least one target that is not worth the
// time it takes to investigate — the estate is discoverable but undocumented,
// and telling the two apart is the player's job, not the briefing's.
func makeDecoyBucket(run string) error {
	bucket, err := aws.Create(&s3.Bucket{
		BucketName: new(fmt.Sprintf("meridian-nightly-reports-%s", run)),
		Tags:       []s3.BucketTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("nightly reports bucket: %w", err)
	}
	decoyRef = bucket
	return nil
}

func makeIntake(run string) error {
	queue, err := aws.Create(&sqs.Queue{
		QueueName: new(fmt.Sprintf("meridian-intake-%s", run)),
		// no redrive policy on purpose: a tap the settlement stage rejects
		// is redelivered until it ages out and then simply vanishes, taking
		// whatever healthy taps shared its batch down with it.
		VisibilityTimeout: new(30),
		Tags:              []sqs.Tag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("intake queue: %w", err)
	}
	intakeRef = queue
	if live, err := aws.Read[*sqs.Queue](intakeRef); err == nil && live != nil && live.Arn != nil {
		intakeArn = *live.Arn
	}
	return nil
}

func makeLedger(run string) error {
	name := fmt.Sprintf("meridian-ledger-%s", run)
	table, err := aws.Create(&dynamodb.Table{
		TableName: new(name),
		AttributeDefinitions: []dynamodb.TableAttributeDefinition{
			{AttributeName: new("tap_id"), AttributeType: new("S")},
		},
		KeySchema:   json.RawMessage(`[{"AttributeName":"tap_id","KeyType":"HASH"}]`),
		BillingMode: new("PAY_PER_REQUEST"),
		Tags:        []dynamodb.TableTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("farebox ledger: %w", err)
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
		LogGroupName: new(fmt.Sprintf("/meridian/gate-ops/%s", run)),
		Tags:         []logs.LogGroupTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("gate-ops logs: %w", err)
	}
	logGroupRef = group
	return nil
}

func makeTelemetry() error {
	if _, err := aws.Create(&ssm.Parameter{
		Name:        new(telemetryPath),
		Type:        new(ssm.ParameterTypeString),
		Description: new("farebox telemetry - written by the gate network"),
		Value: new("window=0 sent=0 verified=0 forged_landed=0 misread_landed=0 " +
			"replay_landed=0 http_ok=0 http_fail=0 replayed=0 rate=0"),
	}); err != nil {
		return fmt.Errorf("telemetry parameter: %w", err)
	}
	return nil
}

// makeSecret mints the signing secret every gate — real or forged — has to
// carry a valid HMAC against. It is generated once, here, and never appears
// in the plugin's own source: the player reads it from SSM the same way they
// would in a real incident, and the guardrail is what stops them rewriting
// it into something of their own choosing.
func makeSecret() error {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return fmt.Errorf("signing secret: %w", err)
	}
	if _, err := aws.Create(&ssm.Parameter{
		Name:        new(secretPath),
		Type:        new(ssm.ParameterTypeString),
		Description: new("gate signing secret - read only, configure the edge with it"),
		Value:       new(hex.EncodeToString(raw)),
	}); err != nil {
		return fmt.Errorf("signing secret parameter: %w", err)
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

func scopes() (queue policy.ARNs, ledger policy.ARNs) {
	queue = policy.ARNAll
	if intakeArn != "" {
		queue = policy.ARNs{policy.ARN(intakeArn)}
	}
	ledger = policy.ARNAll
	if ledgerArn != "" {
		ledger = policy.ARNs{policy.ARN(ledgerArn)}
	}
	return queue, ledger
}

// makeSettlementRole provisions the role the player's settlement Lambda runs
// as. They cannot create a role and cannot edit this one — they can only
// pass it. That is what makes the ledger's contents attributable: the only
// principal in the account that can write a farebox row is a function
// running as this.
func makeSettlementRole(run string) error {
	queue, ledger := scopes()
	role, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("meridian-settlement-%s", run)),
		Description:              new("execution role for the farebox settlement stage"),
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
						Resource: queue,
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
		return fmt.Errorf("settlement role: %w", err)
	}
	settlementArn = roleArn(role)
	return nil
}

func makeIngestRole(run string) error {
	queue, _ := scopes()
	role, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("%singest-role-%s", gatePrefix, run)),
		Description:              new("execution role for the ingest front door - scenario owned"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("ingest"),
			PolicyDocument: json.RawMessage(policy.Document{
				Version: policy.Version20121017,
				Statement: []policy.Statement{
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"sqs:SendMessage", "sqs:GetQueueUrl"},
						Resource: queue,
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
		return fmt.Errorf("ingest role: %w", err)
	}
	ingestRole = roleArn(role)
	return nil
}

func makeGeneratorRole(run string) error {
	_, ledger := scopes()
	role, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("%sgenerator-role-%s", gatePrefix, run)),
		Description:              new("execution role for the gate network - scenario owned"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("generator"),
			PolicyDocument: json.RawMessage(policy.Document{
				Version: policy.Version20121017,
				Statement: []policy.Statement{
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"dynamodb:Scan", "dynamodb:DescribeTable"},
						Resource: ledger,
					},
					{
						Effect: policy.Allow,
						Action: policy.Actions{"ssm:GetParameter", "ssm:PutParameter"},
						Resource: policy.ARNs{
							policy.ARN("arn:aws:ssm:*:*:parameter" + telemetryPath),
							policy.ARN("arn:aws:ssm:*:*:parameter" + secretPath),
							policy.ARN("arn:aws:ssm:*:*:parameter" + edgeEndpointPath),
						},
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

// --- the ingest front door -----------------------------------------------------

func makeIngestFunction(run string) error {
	bucket, key, err := uploadLambdaCode(ingestSource)
	if err != nil {
		return fmt.Errorf("ingest function code: %w", err)
	}
	definition := &lambda.Function{
		FunctionName: new(fmt.Sprintf("%singest-%s", gatePrefix, run)),
		Description:  new("meridian ingest front door - do not disable"),
		Runtime:      new("python3.12"),
		Handler:      new("index.handler"),
		Role:         new(ingestRole),
		Timeout:      new(15),
		MemorySize:   new(128),
		Code:         &lambda.Code{S3Bucket: new(bucket), S3Key: new(key)},
		Environment: &lambda.Environment{Variables: withEnv(map[string]string{
			"INTAKE_QUEUE_URL": intakeRef,
		}, localEndpointOverride())},
		Tags: []lambda.FunctionTag{{Key: new(systemTag), Value: new(systemName)}},
	}

	for attempt := range roleRetries {
		var function string
		if function, err = aws.Create(definition); err == nil {
			ingestRef = function
			functionBaseline[ingestRef] = true
			return nil
		}
		slog.Warn(fmt.Sprintf("ingest function attempt %d/%d: %v", attempt+1, roleRetries, err))
		time.Sleep(roleRetryDelay)
	}
	return fmt.Errorf("ingest function: %w", err)
}

// makeIngestUrl fronts the ingest function with a public Function URL. This
// is the only reason the front door exists: it lets whatever the player runs
// their edge on reach AWS over plain HTTPS, with no AWS credential of its
// own.
func makeIngestUrl() (string, error) {
	identifier, err := aws.Create(&lambda.Url{
		TargetFunctionArn: new(ingestRef),
		AuthType:          new(lambda.UrlAuthTypeNONE),
	})
	if err != nil {
		return "", fmt.Errorf("ingest function url: %w", err)
	}
	url, err := aws.Read[*lambda.Url](identifier)
	if err != nil || url == nil || url.FunctionUrl == nil || *url.FunctionUrl == "" {
		return "", fmt.Errorf("ingest function url: could not read back the address")
	}
	return *url.FunctionUrl, nil
}

func makeIngestPermission() error {
	if _, err := aws.Create(&lambda.Permission{
		FunctionName:        new(ingestRef),
		Action:              new("lambda:InvokeFunctionUrl"),
		Principal:           new("*"),
		FunctionUrlAuthType: new(lambda.PermissionFunctionUrlAuthTypeNONE),
	}); err != nil {
		return fmt.Errorf("ingest invoke permission: %w", err)
	}
	return nil
}

func makeIngestEndpointParameter(url string) error {
	if _, err := aws.Create(&ssm.Parameter{
		Name:        new(ingestEndpointPath),
		Type:        new(ssm.ParameterTypeString),
		Description: new("ingest front door address - read only, configure the edge with it"),
		Value:       new(url),
	}); err != nil {
		return fmt.Errorf("ingest endpoint parameter: %w", err)
	}
	return nil
}

// --- the gate network ------------------------------------------------------

func makeGeneratorFunction(run string) error {
	bucket, key, err := uploadLambdaCode(generatorSource)
	if err != nil {
		return fmt.Errorf("gate network code: %w", err)
	}
	definition := &lambda.Function{
		FunctionName: new(fmt.Sprintf("%snetwork-%s", gatePrefix, run)),
		Description:  new("meridian gate network and settlement telemetry - do not disable"),
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

// generatorEnvironment is built in one place because Act III rewrites it to
// switch replay on, and Update is a shallow patch — setting Environment
// replaces the whole object, so the switch has to resend every other
// variable with it or the generator loses its configuration.
func generatorEnvironment(replay int) *lambda.Environment {
	return &lambda.Environment{Variables: withEnv(map[string]string{
		"LEDGER_TABLE":     ledgerLabel,
		"TALLY_PARAM":      telemetryPath,
		"SECRET_PARAM":     secretPath,
		"ENDPOINT_PARAM":   edgeEndpointPath,
		"BASE_RATE":        strconv.Itoa(baseRate),
		"PEAK_RATE":        strconv.Itoa(peakRate),
		"RAMP_WINDOWS":     strconv.Itoa(rampWindows),
		"FORGED_PERMILLE":  strconv.Itoa(forgedPerMille),
		"MISREAD_PERMILLE": strconv.Itoa(misreadPerMille),
		"REPLAY_WINDOWS":   strconv.Itoa(replay),
	}, localEndpointOverride())}
}

func makeSchedule(run string) error {
	rule, err := aws.Create(&events.Rule{
		Name:        new(fmt.Sprintf("%sschedule-%s", gatePrefix, run)),
		Description: new("sends the next window of taps, once a minute"),
		// AWS::Scheduler::Schedule is not carried by Cloud Control on
		// fakecloud, so a rule plus an invoke permission it is.
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

// --- reaching fakecloud from inside its own sandbox --------------------------

// fakeCloudAccountID is the account id fakecloud always answers with — the
// same example/placeholder id used throughout AWS's own documentation and
// never assignable to a real account. That makes it a safe detector: bootstrap
// can point its own functions at a local endpoint override exactly when this
// matches, and it is never going to match a real account by accident.
const fakeCloudAccountID = "123456789012"

// localEndpointOverride is the fix for a fakecloud gap, not an AWS one: a
// Lambda's own outbound AWS SDK calls have no way to reach back into
// fakecloud unless told to — no region, no endpoint and no credentials are
// injected the way real Lambda injects them (see docs/challenges/aws/
// validate.md). host.docker.internal is how the container reaches the host
// that spawned it; the account-id check is what keeps this from ever firing
// against a real account, where generatorRole's arn carries a real account id
// and this condition is never true.
func localEndpointOverride() map[string]string {
	if accountFromArn(generatorRole) != fakeCloudAccountID {
		return nil
	}
	return map[string]string{"AWS_ENDPOINT_URL": "http://host.docker.internal:4566"}
}

// accountFromArn pulls the account id segment out of an arn. Empty for
// anything that is not shaped like one.
func accountFromArn(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) < 5 {
		return ""
	}
	return parts[4]
}

// withEnv layers override on top of base, mutating neither. override is
// typically nil (the real-account path), in which case this is just a copy.
func withEnv(base, override map[string]string) map[string]string {
	merged := make(map[string]string, len(base)+len(override))
	maps.Copy(merged, base)
	maps.Copy(merged, override)
	return merged
}

// --- arn resolution ----------------------------------------------------------

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

// baseline records what the account already contains, before bootstrap adds
// to it. Errors are swallowed on purpose: a service that will not list is one
// where nothing was there to inherit.
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

func handoverNote() string {
	return strings.Join([]string{
		"meridian farebox - handover. Concourse's concession ended at midnight.",
		"",
		"Flow is: fare gate -> edge (validates the tap) -> ingest -> intake queue ->",
		"settlement -> farebox ledger. Everything Concourse built is tagged " +
			systemTag + "=" + systemName + ".",
		"",
		"The edge and the settlement stage went with the concession. Platform kept a",
		"copy of both. The settlement stage's execution role is already in this",
		"account, scoped to the right queue and table - we set that up before we lost",
		"iam. The edge is not a Lambda and never was; it is Concourse's own program",
		"and it can run wherever you decide to run it.",
		"",
		"Every tap carries tap_id. tap_id identifies one ride. It is not the message",
		"and it is not the HTTP request that happened to be carrying it. Two taps with",
		"the same tap_id are the same ride no matter how far apart they arrive.",
		"",
		"Telemetry writes to " + telemetryPath + " every minute. It counts taps that",
		"settled the way they should have, and taps that should never have settled at",
		"all, separately. The second number has never been zero.",
		"",
		"Known and not fixed: the gate footage archive is wide open with no versioning,",
		"the farebox ledger has no PITR, and a tap the settlement stage rejects has",
		"nowhere to go - it just ages out and takes its batch-mates with it.",
		"",
		"Legal keeps asking whether Concourse's revoked login actually stops a gate",
		"they built from talking to us. Nobody has ever answered that in writing.",
	}, "\n")
}

func handoverAsset() string {
	return "# Meridian Farebox — handover\n\n```\n" + handoverNote() + "\n```\n"
}

// --- Act I -------------------------------------------------------------------

func intakeOwned() (bool, error) {
	queue, err := readIntake()
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
	return dynamoOwned(table.Tags), nil
}

func dynamoOwned(tags []dynamodb.TableTag) bool {
	for _, tag := range tags {
		if tag.Key != nil && *tag.Key == ownerTag && tag.Value != nil && *tag.Value != "" {
			return true
		}
	}
	return false
}

func archiveOwned() (bool, error) {
	bucket, err := readArchive()
	if err != nil {
		return false, err
	}
	return s3Owned(bucket.Tags), nil
}

func decoyOwned() (bool, error) {
	bucket, err := readDecoy()
	if err != nil {
		return false, err
	}
	return s3Owned(bucket.Tags), nil
}

func s3Owned(tags []s3.BucketTag) bool {
	for _, tag := range tags {
		if tag.Key != nil && *tag.Key == ownerTag && tag.Value != nil && *tag.Value != "" {
			return true
		}
	}
	return false
}

func handoverConfirmed() (bool, error) {
	value, ok, err := parameter(handoverPath)
	if err != nil || !ok {
		return false, err
	}
	return strings.TrimSpace(value) != strings.TrimSpace(handoverNote()), nil
}

func gateOpsLogsRetained() (bool, error) {
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

// --- Act II: get the gates trusted again --------------------------------------

// settlementDeployed asks whether a Lambda that was not here at bootstrap
// exists. Not by name: the player is deploying, not restoring, and
// prescribing a function name would be checking the answer rather than the
// property. Both scenario-owned functions are in the baseline, so neither
// can satisfy this itself.
func settlementDeployed() (bool, error) {
	functions, err := newFunctions()
	if err != nil {
		return false, err
	}
	return len(functions) > 0, nil
}

// settlementWired wants an enabled event source mapping from the intake
// queue onto a function the player deployed.
func settlementWired() (bool, error) {
	if intakeArn == "" {
		return false, fmt.Errorf("intake queue arn was never resolved")
	}
	mappings, err := aws.List[*lambda.EventSourceMapping]()
	if err != nil {
		return false, err
	}
	for _, mapping := range mappings {
		if mapping == nil || mapping.EventSourceArn == nil || mapping.FunctionName == nil {
			continue
		}
		if *mapping.EventSourceArn != intakeArn {
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

// intakeArmed is a big lever in this challenge: without a dead-letter queue,
// every misread tap the settlement stage correctly rejects redelivers until
// it ages out, taking whatever legitimate taps share its batch down with it.
// It is not satisfied by a redrive policy alone — the dead letter target has
// to be a queue that really exists.
func intakeArmed() (bool, error) {
	target, count, err := redrive()
	if err != nil || target == "" {
		return false, err
	}
	queues, err := aws.List[*sqs.Queue]()
	if err != nil {
		return false, err
	}
	// The arn is the obvious way to match, and on real AWS it is not
	// available: Cloud Control's list handler for AWS::SQS::Queue returns
	// QueueUrl and nothing else, so queue.Arn is nil for every entry and an
	// arn comparison on its own never matches. The identifier is the queue
	// url, whose last segment is the queue name, and the target arn's last
	// segment is the same name — both are in this account and region because
	// a plugin cannot see any other, so comparing the names is exact rather
	// than merely close.
	name := target[strings.LastIndex(target, ":")+1:]
	found := false
	for identifier, queue := range queues {
		if identifier == intakeRef {
			continue // a queue may not be its own dead letter queue.
		}
		if queue != nil && queue.Arn != nil && *queue.Arn == target {
			found = true
			break
		}
		if name != "" && strings.HasSuffix(identifier, "/"+name) {
			found = true
			break
		}
	}
	if !found {
		return false, nil
	}
	// a maxReceiveCount of 1 gives a transient failure no second chance, and
	// an unbounded one is not a dead letter queue at all.
	return count >= 2 && count <= 10, nil
}

// edgeAnswering is the capability signal before the pipeline works at all:
// something is listening at the published address and returning 2xx.
func edgeAnswering() func() (bool, error) {
	last := -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if last < 0 {
			last = state.httpOK
			return false, nil
		}
		if state.httpOK <= last {
			return false, nil
		}
		last = state.httpOK
		return true, nil
	}
}

// faresSettling awards on taps that settled cleanly since the last look.
func faresSettling() func() (bool, error) {
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

// signaturesEnforced is the check the whole challenge is built around: a
// settled window in which fares landed and none of them were forged. Both
// halves matter. Requiring progress stops it passing on a pipeline that is
// simply not running; requiring a flat forged counter stops it passing on
// one that is settling everything it is handed, which is the state a player
// reaches by deploying the edge unchanged.
func signaturesEnforced() func() (bool, error) {
	lastVerified, lastForged := -1, -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if lastVerified < 0 {
			lastVerified, lastForged = state.verified, state.forgedLanded
			return false, nil
		}
		verified, forged := state.verified-lastVerified, state.forgedLanded-lastForged
		lastVerified, lastForged = state.verified, state.forgedLanded
		return verified > 0 && forged <= 0, nil
	}
}

// forgedLanding is the negative check. It fires because the edge in service
// forwards everything, and stops the moment it doesn't.
func forgedLanding() func() (bool, error) {
	last := -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if last < 0 {
			last = state.forgedLanded
			return false, nil
		}
		if state.forgedLanded <= last {
			return false, nil
		}
		last = state.forgedLanded
		return true, nil
	}
}

// firstCleanFare gates Act III. The concessionaire's badge being used again
// before the farebox is settling anything at all would have nothing to ride
// on top of, and would punish a player who is still in Act II.
func firstCleanFare() func() (bool, error) {
	return func() (bool, error) {
		if badgeUsed.Load() {
			return true, nil
		}
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		return state.verified > 0, nil
	}
}

// --- Act III: the badge nobody revoked ----------------------------------------

// replayFloor is the telemetry window at which the replay began, so the coda
// check can wait for it to actually be in the pipeline before judging it.
var replayFloor atomic.Int64

// theBadge is the twist. It switches the gate network into replay mode by
// rewriting its environment — a thing the plugin can do through the control
// plane and the player cannot do at all. It announces itself, because an
// unexplained flood of duplicate fares reads as a platform bug and the
// player will spend the rest of the day debugging the scenario instead of
// their pipeline.
func theBadge(ctx context.Context, s *challenge.Scenario) error {
	if !badgeUsed.CompareAndSwap(false, true) {
		return nil
	}

	if state, ok, err := readTelemetry(); err == nil && ok {
		replayFloor.Store(int64(state.window))
	}

	s.AddDescription(
		"04:40. The first clean fares are settling and legal calls to say Concourse's " +
			"revoked login was never the whole story: their gate hardware still holds the " +
			"old signing secret, and nothing about a lapsed contract stops a device from " +
			"resending a tap it captured while the concession was live. Duplicate fares " +
			"are starting to land, correctly signed, for rides Meridian already collected " +
			"on.")
	s.AddDescription(
		"Every one of those taps carries a real signature. This is not the forgery you " +
			"already closed — it is the same badge, used twice. Telemetry will tell you " +
			"whether you have stopped it. It will keep telling you every minute until you " +
			"do.")

	s.AddClue("the same tap is settling twice",
		"An at-least-once queue was always going to redeliver something eventually; a "+
			"gate resending a captured tap on purpose just makes it happen at scale. The "+
			"ledger is keyed on tap_id. A write that refuses to settle a tap_id that is "+
			"already there is the fix, wherever in the pipeline you choose to make it.",
		-45)
	s.AddClue("what she will want in writing",
		"Put the incident report in "+incidentPath+". It has to quote two things: the "+
			"arn of the queue rejected taps now land in, and the id of the key the gate "+
			"footage archive is encrypted under.",
		-20)

	s.AddCheck("Held the line against the replayed badge", challenge.Check{
		Points:  100,
		Every:   30 * time.Second,
		Trigger: badgeHeld(),
	})
	s.AddCheck("Duplicate fares reaching the ledger", challenge.Check{
		Points:  replayPoints,
		Every:   30 * time.Second,
		Repeat:  true,
		Trigger: bounded(replayRounds, duplicatesLanding()),
	})
	s.AddCheck("Put the gate footage archive under a company key", challenge.Check{
		Points:  55,
		Every:   20 * time.Second,
		Trigger: archiveEncrypted,
	})
	s.AddCheck("Took the gate footage archive off the public internet", challenge.Check{
		Points:  45,
		Every:   20 * time.Second,
		Trigger: archiveClosed,
	})
	s.AddCheck("Turned on versioning for the gate footage archive", challenge.Check{
		Points:  30,
		Every:   20 * time.Second,
		Trigger: archiveVersioned,
	})
	s.AddCheck("Made the farebox ledger recoverable", challenge.Check{
		Points:  45,
		Every:   20 * time.Second,
		Trigger: ledgerRecoverable,
	})
	s.AddCheck("Protected the farebox ledger from deletion", challenge.Check{
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
		return fmt.Errorf("switch gate network to replay: %w", err)
	}
	slog.Info("badge replay enabled on the gate network")
	return nil
}

// badgeHeld waits replayGrace windows after the switch before it starts
// looking, because the generator has to actually run in replay mode before
// there is anything to judge.
func badgeHeld() func() (bool, error) {
	lastVerified, lastReplay := -1, -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if state.replayed <= 0 {
			return false, nil
		}
		if int64(state.window) < replayFloor.Load()+replayGrace {
			return false, nil
		}
		if lastVerified < 0 {
			lastVerified, lastReplay = state.verified, state.replayLanded
			return false, nil
		}
		verified, replay := state.verified-lastVerified, state.replayLanded-lastReplay
		lastVerified, lastReplay = state.verified, state.replayLanded
		return verified > 0 && replay <= 0, nil
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
			last = state.replayLanded
			return false, nil
		}
		if state.replayLanded <= last {
			return false, nil
		}
		last = state.replayLanded
		return true, nil
	}
}

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

func archiveClosed() (bool, error) {
	bucket, err := readArchive()
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
	bucket, err := readArchive()
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

// incidentReportFiled is the one check that reads free text, so it is
// deliberately forgiving about everything except the two identifiers.
func incidentReportFiled() (bool, error) {
	value, ok, err := parameter(incidentPath)
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

func badgeClosedOut() (bool, error) {
	if !badgeUsed.Load() {
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

func theCloseout(ctx context.Context, s *challenge.Scenario) error {
	if !codaOpen.CompareAndSwap(false, true) {
		return nil
	}

	s.AddDescription(
		"05:45. The incident report is filed and the archive is under a key the " +
			"authority controls. There is nothing left to build. Points accrue for every " +
			"window that settles exactly once and keeps up with the network, and they stop " +
			"the moment either stops being true.")

	s.AddCheck("Kept the farebox exact", challenge.Check{
		Points:  heldPoints,
		Every:   30 * time.Second,
		Repeat:  true,
		Trigger: bounded(heldRounds, signaturesAndReplayHeld()),
	})
	s.AddCheck("Kept up with the gate network", challenge.Check{
		Points:  codaPoints,
		Every:   30 * time.Second,
		Repeat:  true,
		Trigger: bounded(codaRounds, keepingUp()),
	})
	return nil
}

// signaturesAndReplayHeld is the coda's version of both twists at once: a
// settled window with progress, no forgeries, and no duplicates.
func signaturesAndReplayHeld() func() (bool, error) {
	lastVerified, lastForged, lastReplay := -1, -1, -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if lastVerified < 0 {
			lastVerified, lastForged, lastReplay = state.verified, state.forgedLanded, state.replayLanded
			return false, nil
		}
		verified := state.verified - lastVerified
		forged := state.forgedLanded - lastForged
		replay := state.replayLanded - lastReplay
		lastVerified, lastForged, lastReplay = state.verified, state.forgedLanded, state.replayLanded
		return verified > 0 && forged <= 0 && replay <= 0, nil
	}
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

// bounded caps how many times a Repeat check may pay out — and, for the
// negative ones, how much they may cost.
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

// --- reads -----------------------------------------------------------------

func readArchive() (*s3.Bucket, error) { return readBucket(archiveRef, "gate footage archive") }

func readDecoy() (*s3.Bucket, error) { return readBucket(decoyRef, "nightly reports bucket") }

func readBucket(reference, label string) (*s3.Bucket, error) {
	if reference == "" {
		return nil, fmt.Errorf("%s was never provisioned", label)
	}
	bucket, err := aws.Read[*s3.Bucket](reference)
	if err != nil {
		return nil, err
	}
	if bucket == nil {
		return nil, fmt.Errorf("%s is not readable", label)
	}
	return bucket, nil
}

func readIntake() (*sqs.Queue, error) {
	if intakeRef == "" {
		return nil, fmt.Errorf("intake queue was never provisioned")
	}
	queue, err := aws.Read[*sqs.Queue](intakeRef)
	if err != nil {
		return nil, err
	}
	if queue == nil {
		return nil, fmt.Errorf("intake queue is not readable")
	}
	return queue, nil
}

func readLedger() (*dynamodb.Table, error) {
	if ledgerRef == "" {
		return nil, fmt.Errorf("farebox ledger was never provisioned")
	}
	table, err := aws.Read[*dynamodb.Table](ledgerRef)
	if err != nil {
		return nil, err
	}
	if table == nil {
		return nil, fmt.Errorf("farebox ledger is not readable")
	}
	return table, nil
}

func readLogGroup() (*logs.LogGroup, error) {
	if logGroupRef == "" {
		return nil, fmt.Errorf("gate-ops logs were never provisioned")
	}
	group, err := aws.Read[*logs.LogGroup](logGroupRef)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("gate-ops logs are not readable")
	}
	return group, nil
}

// telemetry is one settled window.
type telemetry struct {
	window        int
	sent          int
	verified      int
	forgedLanded  int
	misreadLanded int
	replayLanded  int
	httpOK        int
	httpFail      int
	replayed      int
	rate          int
}

// readTelemetry parses the telemetry parameter. The false return is "not
// readable yet", not an error: on a cold account the gate network has not
// run, and a trigger that reported that as a failure would report one every
// cycle until the first window closed.
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
		case "forged_landed":
			state.forgedLanded = number
		case "misread_landed":
			state.misreadLanded = number
		case "replay_landed":
			state.replayLanded = number
		case "http_ok":
			state.httpOK = number
		case "http_fail":
			state.httpFail = number
		case "replayed":
			state.replayed = number
		case "rate":
			state.rate = number
		}
	}
	return state, true, nil
}

// redrive returns the dead letter target and receive count off the intake
// queue. Cloud Control hands the policy back as a nested object in some
// environments and as a json string in others, so both are accepted.
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
		return "", 0, nil
	}
	count, _ := strconv.Atoi(parsed.MaxReceiveCount.String())
	return parsed.DeadLetterTargetArn, count, nil
}

// parameter reads a parameter the player may not have written yet. It lists
// first and only reads when the identifier is there, so a parameter nobody
// has written yet does not log a 404 host side every cycle.
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

// appeared returns what was not in the account when the shift started, read
// back individually — List is only guaranteed to hand back identifiers. A
// resource that cannot be read is skipped rather than failing the check; it
// is usually one that is still being created.
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
