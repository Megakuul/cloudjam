//go:build wasip1

// Command pueblo-night-shift is a gameday tier cloudjam challenge.
//
// Pueblo Freight runs its dispatch platform on an account nobody has owned
// since the contractor who built it left. The player takes the pager on the
// busiest night of the year, inherits the estate, and hardens it while it is
// carrying live freight.
//
// The freight is real. A load generator runs inside the account on a one
// minute schedule, pushes dispatch jobs onto the queue at a rate that climbs
// through peak, and writes what it observes to a telemetry parameter the
// plugin reads every cycle. Nothing is consuming that queue when the player
// arrives, because the consumer left with the contractor — it ships to the
// player as a downloadable package and deploying it is the first real task.
// Points follow freight that actually reached the ledger, not configuration
// that looks right.
//
// The story runs in three acts. Act I and Act II ship with the plugin. Act III
// is fired by an event the moment the player arms the dispatch queue with a
// dead-letter queue: the messages that were being dropped silently start
// landing where they can be read, and what is in them changes the job.
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
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/ec2"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/events"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/kms"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/lambda"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/logs"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/s3"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/sns"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/sqs"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/services/ssm"
	"github.com/google/uuid"
)

// ownerTag is what "taking ownership" means mechanically: the estate is
// untagged, and the platform team's inventory job only counts what carries an
// owner. Any non-empty value counts — the challenge does not care who you say
// you are, only that somebody claimed it.
const ownerTag = "pueblo:owner"

// systemTag is on every resource bootstrap provisions, so the player can find
// the estate at all. It is never checked; it is the discovery affordance.
const systemTag = "pueblo:system"

const systemName = "roadrunner"

// loadBoxPrefix names everything the scenario owns and the player does not.
// The guardrail denies write on this prefix by pattern, which is why it has to
// be a constant and not something built from the run id.
const loadBoxPrefix = "pueblo-loadbox-"

// Parameter paths. The handover is written by bootstrap and is the player's way
// in; telemetry is written by the load generator and is read-only to the
// player; the other two are written by the player.
const (
	handoverPath  = "/pueblo/handover"
	telemetryPath = "/pueblo/telemetry"
	oncallPath    = "/pueblo/oncall"
	incidentPath  = "/pueblo/incident"
)

const (
	sshPort = 22
	apiPort = 8080
)

// How hard bootstrap tries to build a Lambda on a role IAM has only just
// created. See makeLoadBox — this is a propagation race, not a flaky API, and
// it resolves in seconds or not at all.
const (
	roleRetries    = 6
	roleRetryDelay = 5 * time.Second
)

// vpcCidr is the network the dispatch tier sits in.
//
// The estate provisions its own vpc rather than leaning on the account's
// default one. AWS::EC2::SecurityGroup with no VpcId is created in the default
// vpc, and a sandbox account is not guaranteed to have one — a recycled account
// has had it deleted along with everything else it was carrying, and the create
// then fails with "no default VPC for this user" long after bootstrap looked
// like it was working.
const vpcCidr = "10.0.0.0/16"

// Retention the platform team will accept: long enough to investigate a peak
// season incident, short enough that nobody is paying to store last year's.
const (
	minRetentionDays = 30
	maxRetentionDays = 400
)

// The load generator's shape. Volume climbs from baseRate to peakRate over
// rampWindows one-minute windows, which is the "volume triples around 22:00"
// in the briefing made literal. poisonPerMille is how many of those jobs the
// old dispatcher still emits in the legacy format the worker cannot parse.
const (
	baseRate       = 12
	peakRate       = 45
	rampWindows    = 20
	poisonPerMille = 70
)

// Bounds on the repeating checks. Repeat checks never retire, so without a cap
// they pay out forever and the maximum score does not exist.
const (
	heldRounds       = 8
	heldPoints       = 5
	throughputRounds = 10
	throughputPoints = 10
	codaRounds       = 8
	codaPoints       = 10
)

// keepingUpPerMille is how much of what the generator sent has to reach the
// ledger for the pipeline to count as keeping up. Not 100%: a batch in flight
// when the window closes is normal, and a challenge that demands perfection
// from an at-least-once queue is a challenge nobody passes.
const keepingUpPerMille = 600

// Primary identifiers of everything bootstrap provisions. bootstrap runs to
// completion before the first check cycle, so triggers may read these.
var (
	bucketRef    string
	queueRef     string
	queueArn     string
	tableRef     string
	tableArn     string
	tableLabel   string // the ledger's name, which is not always its identifier
	logGroupRef  string
	vpcRef       string
	groupRef     string
	workerRole   string // the arn, because that is what a player has to pass
	loadBoxRef   string
	scheduleRef  string
	generatorRef string
)

// Baselines captured before bootstrap provisions anything. Act II and Act III
// ask the player to *create* things, and "did a resource of this type appear"
// is only a meaningful question against what was already there — the sandbox
// account is not guaranteed to be empty, and some environments carry
// service-managed keys.
var (
	keyBaseline      = map[string]bool{}
	logBaseline      = map[string]bool{}
	topicBaseline    = map[string]bool{}
	functionBaseline = map[string]bool{}
)

// poisonRun reports whether Act III has started. quiet-hours is gated on it so
// a player who happens to build Act III's resources early cannot skip an act.
var poisonRun atomic.Bool

func main() {
	challenge.New("Pueblo Freight: Night Shift", 10*time.Second, bootstrap).
		AddDescription(
			"Pueblo Freight moves about eleven thousand pallets a night across the southwest. "+
				"Everything that makes that happen — the dispatch pipeline, the shipment ledger, "+
				"the proof-of-delivery archive the claims team lives in — runs in this account. "+
				"It was built over two years by one contractor on a rolling statement of work. "+
				"That statement of work ended on Friday. Nobody else has ever had the credentials.").
		AddDescription(
			"You are the platform engineer who just took the pager. Tonight is the first night "+
				"of peak season: volume roughly triples around 22:00 and does not come down until "+
				"the morning. Nobody is going to give you a maintenance window, and nothing here "+
				"can be turned off — every one of these systems is carrying live freight while you "+
				"work on it.").
		AddDescription(
			"It is already going wrong. Dispatch jobs are arriving on the queue at a rate that "+
				"climbs all night and **nothing is processing them**. The consumer went with the "+
				"contractor. Every minute you spend reading is a minute of freight piling up in a "+
				"queue with nobody on the other end.").
		AddDescription(
			"There is no architecture diagram, no terraform state anybody can find, and no runbook "+
				"beyond a handover note the contractor left in Parameter Store on their way out. "+
				"Start there. Everything they built is tagged "+systemTag+"="+systemName+", "+
				"which is the only inventory that exists.").
		AddDescription(
			"The platform team's load box reports what it sees to "+telemetryPath+" once a "+
				"minute: how much freight it dispatched, and how much of it reached the ledger. "+
				"That parameter is your scoreboard and your monitoring. You can read it. You "+
				"cannot write it.").
		AddDescription(
			"Your job tonight is not a checklist, and you will not be handed one. It is the job "+
				"you would actually do: get the freight moving again, then find out what else you "+
				"have inherited and fix what would hurt most if it failed at 03:00. You are scored "+
				"continuously — for freight delivered, and for the estate getting safer.").
		// clue prices are added to the team score, so they are negative. They are
		// scaled to what the clue unlocks: orientation is nearly free, the two
		// clues that hand over the dispatch pipeline are the expensive ones.
		AddClue("where do i start",
			"The contractor's handover note is an SSM parameter at "+handoverPath+". "+
				"Read it before you touch anything: aws ssm get-parameter --name "+handoverPath+".",
			-5).
		AddClue("nothing is consuming the queue",
			"The consumer is gone, but it is not lost — the platform team kept a copy. It is "+
				"attached to this challenge as a downloadable asset, roadrunner-worker.zip, and "+
				"the README inside it tells you how it expects to be deployed.",
			-35).
		AddClue("i cannot create an execution role",
			"You do not have iam:CreateRole and you are not meant to. There is already a role in "+
				"this account for the dispatch worker, provisioned and scoped for exactly this "+
				"— find it with aws iam list-roles and pass it to your function.",
			-20).
		AddClue("what counts as ownership",
			"The platform team's inventory job only counts resources carrying a "+ownerTag+" tag. "+
				"Any value will do — the point is that somebody's name is on it.",
			-20).
		AddClue("freight is moving but most of it is not counted",
			"Look at what the load box is sending. Some of those jobs are in the old dispatcher's "+
				"format and the worker cannot parse them — and a batch that raises takes the "+
				"healthy jobs in it down too. Give the failures somewhere to go.",
			-30).
		AddClue("the archive is already encrypted, isn't it",
			"It is encrypted with the key the platform hands every bucket by default, which "+
				"means the platform can read it and the claims team's auditor counts it as "+
				"unencrypted. Set default encryption on the archive to a KMS key instead.",
			-12).
		AddClue("what would hurt most at 03:00",
			"Ask what is unrecoverable rather than what is untidy. Data that cannot be restored, "+
				"messages that vanish instead of failing, and an archive the whole internet can read.",
			-25).
		SetPermission(policy.Document{
			Version: policy.Version20121017,
			Statement: []policy.Statement{
				// bootstrap grants the working set once it knows what it built.
				{Effect: policy.Deny, Action: policy.ActionAll, Resource: policy.ARNAll},
			},
		}).
		SetGuardrail(guardrail()).
		// --- Act I: whose account is this ---------------------------------
		AddCheck("Signed for the manifest archive", challenge.Check{
			Points:  25,
			Every:   15 * time.Second,
			Trigger: bucketOwned,
		}).
		AddCheck("Signed for the dispatch queue", challenge.Check{
			Points:  25,
			Every:   15 * time.Second,
			Trigger: queueOwned,
		}).
		AddCheck("Signed for the shipment ledger", challenge.Check{
			Points:  25,
			Every:   15 * time.Second,
			Trigger: tableOwned,
		}).
		AddCheck("Signed for the dispatch logs", challenge.Check{
			Points:  25,
			Every:   15 * time.Second,
			Trigger: logsOwned,
		}).
		AddCheck("Confirmed the handover", challenge.Check{
			Points:  50,
			Every:   15 * time.Second,
			Trigger: handoverConfirmed,
		}).
		// --- Act II: get the freight moving --------------------------------
		AddCheck("Deployed the dispatch worker", challenge.Check{
			Points:  90,
			Every:   15 * time.Second,
			Trigger: workerDeployed,
		}).
		AddCheck("Wired the worker to the dispatch queue", challenge.Check{
			Points:  80,
			Every:   15 * time.Second,
			Trigger: workerWired,
		}).
		AddCheck("Freight is reaching the ledger", challenge.Check{
			Points:  throughputPoints,
			Every:   time.Minute,
			Repeat:  true,
			Trigger: bounded(throughputRounds, freightMoving()),
		}).
		// --- Act II: the things that lose freight --------------------------
		AddCheck("Put the manifest archive under a company key", challenge.Check{
			Points:  45,
			Every:   15 * time.Second,
			Trigger: archiveEncrypted,
		}).
		AddCheck("Took the manifest archive off the public internet", challenge.Check{
			Points:  45,
			Every:   15 * time.Second,
			Trigger: archiveClosed,
		}).
		AddCheck("Turned on versioning for the manifests", challenge.Check{
			Points:  25,
			Every:   15 * time.Second,
			Trigger: archiveVersioned,
		}).
		AddCheck("Armed the dispatch queue with a dead-letter queue", challenge.Check{
			Points:  70,
			Every:   15 * time.Second,
			Trigger: dispatchArmed,
		}).
		AddCheck("Encrypted the dispatch queue at rest", challenge.Check{
			Points:  30,
			Every:   15 * time.Second,
			Trigger: dispatchEncrypted,
		}).
		AddCheck("Made the shipment ledger recoverable", challenge.Check{
			Points:  50,
			Every:   15 * time.Second,
			Trigger: ledgerRecoverable,
		}).
		AddCheck("Protected the shipment ledger from deletion", challenge.Check{
			Points:  25,
			Every:   15 * time.Second,
			Trigger: ledgerProtected,
		}).
		AddCheck("Gave the dispatch logs a retention window", challenge.Check{
			Points:  30,
			Every:   15 * time.Second,
			Trigger: dispatchLogsRetained,
		}).
		AddCheck("Took SSH off the internet without locking the team out", challenge.Check{
			Points:  20,
			Every:   15 * time.Second,
			Trigger: sshLockedDown,
		}).
		AddCheck("Took the dispatch API off the internet", challenge.Check{
			Points:  20,
			Every:   15 * time.Second,
			Trigger: apiClosed,
		}).
		// --- the pacing ----------------------------------------------------
		AddEvent("poison-run", challenge.Event{
			Every: 20 * time.Second,
			// the specific task that opens Act III: the moment failed messages
			// have somewhere to land, somebody can finally read one.
			Trigger: dispatchArmed,
			Event:   thePoisonRun,
		}).
		AddEvent("quiet-hours", challenge.Event{
			Every:   30 * time.Second,
			Trigger: incidentClosedOut,
			Event:   quietHours,
		}).
		Start()
}

// workingSet is the permission envelope for the whole scenario. It is both the
// guardrail's allow statement and, once bootstrap has run, the granted
// permission — the player needs all of it eventually, and which act needs which
// service is exactly the thing they are supposed to work out for themselves.
//
// These are prefix patterns rather than the generated per-action constants, and
// that is not a stylistic choice. An iam managed policy may not exceed 6144
// characters; enumerating Read+List+Write+Tagging for these services produces
// well over a thousand actions and a 44kB document, which aws rejects outright
// with "LimitExceeded: Cannot exceed quota for PolicySize". ec2 alone is 26kB.
//
// What keeps this safe is not the allow list but the deny statements in
// guardrail() — see there.
func workingSet() policy.Actions {
	return policy.Actions{
		// The manifest archive: tags, encryption, public access block,
		// versioning — and the object upload, because the player has to put
		// the worker package somewhere Lambda can read it.
		//
		// Put* also settles a naming disagreement. fakecloud authorises two of
		// these calls under their api operation name (s3:PutBucketEncryption,
		// s3:PutPublicAccessBlock) while real aws uses the iam action name
		// (s3:PutEncryptionConfiguration, s3:PutBucketPublicAccessBlock). One
		// prefix covers both spellings, so the archive is fixable in either.
		"s3:Get*", "s3:List*", "s3:Put*",

		// The pipeline. The player creates a dead letter queue and a pager
		// topic, so these need create rights; both are billed per request.
		"sqs:*", "sns:*",

		// The worker: create the function, wire the event source mapping,
		// read the load box without being able to touch it. The guardrail
		// carves the load box back out.
		"lambda:*",

		// Passing the pre-provisioned worker role to that function, and being
		// able to find it. Everything else in iam is denied outright — the
		// player never mints a role, which is what keeps them inside the
		// boundary they were given.
		"iam:PassRole", "iam:GetRole", "iam:ListRoles",
		"iam:ListRolePolicies", "iam:GetRolePolicy", "iam:ListAttachedRolePolicies",

		// The ledger: recovery, deletion protection and tags. Deliberately not
		// CreateTable — nothing here needs a new table, and a provisioned one
		// is a standing charge. The item level writes are denied in the
		// guardrail so that "delivered" cannot be typed in by hand.
		"dynamodb:Describe*", "dynamodb:List*",
		"dynamodb:UpdateTable", "dynamodb:UpdateContinuousBackups",
		"dynamodb:UpdateTimeToLive", "dynamodb:TagResource", "dynamodb:UntagResource",

		// The audit trail the poison run destroys, and the key that protects it.
		"logs:*", "kms:*",

		// The perimeter, and only the perimeter. No RunInstances, no NAT
		// gateways: with ec2 the guardrail is the only thing between a curious
		// player and a bill that outlives the gameday.
		"ec2:Describe*",
		"ec2:CreateSecurityGroup", "ec2:DeleteSecurityGroup",
		"ec2:AuthorizeSecurityGroup*", "ec2:RevokeSecurityGroup*",
		"ec2:ModifySecurityGroupRules",
		"ec2:CreateTags", "ec2:DeleteTags",

		// The handover note, the on-call confirmation and the incident record.
		"ssm:Describe*", "ssm:Get*", "ssm:PutParameter", "ssm:DeleteParameter",
		"ssm:AddTagsToResource", "ssm:ListTagsForResource",

		// Reading the schedule the load box runs on. Writing it is denied.
		"events:Describe*", "events:List*",
	}
}

// guardrail is the permission boundary: the absolute edge of the scenario,
// which the player cannot escalate past even though SetPermission may widen
// what they hold inside it.
//
// The deny statements are the load-bearing part, and each one exists because
// without it the scoreboard is a lie:
//
//   - the load box and its schedule are how traffic exists at all. A player who
//     can rewrite or delete them can stop the clock or forge the tally.
//   - the telemetry parameter is the score. It is readable — it is the player's
//     monitoring — and writable only by the generator's role.
//   - ledger item writes belong to the worker's execution role. Denying them to
//     the player is what makes "delivered" mean freight that went through the
//     pipeline rather than rows somebody typed in.
//   - iam role creation would let the player build a role outside this boundary
//     and hand it to a Lambda, which is the whole boundary escaped.
//
// Patterns rather than arns because the boundary is published before bootstrap
// has run and the account id and run id are not known yet.
func guardrail() policy.Document {
	return policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{
				Sid:      "NightShift",
				Effect:   policy.Allow,
				Action:   workingSet(),
				Resource: policy.ARNAll,
			},
			{
				Sid:    "TheLoadBoxIsNotYours",
				Effect: policy.Deny,
				Action: policy.Actions{
					"lambda:UpdateFunctionCode", "lambda:UpdateFunctionConfiguration",
					"lambda:DeleteFunction", "lambda:InvokeFunction",
					"lambda:AddPermission", "lambda:RemovePermission",
				},
				Resource: policy.ARNs{policy.ARN("arn:aws:lambda:*:*:function:" + loadBoxPrefix + "*")},
			},
			{
				Sid:      "TheScheduleIsNotYours",
				Effect:   policy.Deny,
				Action:   policy.Actions{"events:*"},
				Resource: policy.ARNs{policy.ARN("arn:aws:events:*:*:rule/" + loadBoxPrefix + "*")},
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
				Sid:    "TheLedgerIsWrittenByTheWorker",
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
				// provisioned concurrency is the one thing in lambda that bills
				// by the hour whether or not anything invokes it.
				Sid:      "NoStandingCharges",
				Effect:   policy.Deny,
				Action:   policy.Actions{"lambda:PutProvisionedConcurrencyConfig"},
				Resource: policy.ARNAll,
			},
		},
	}
}

// --- AWS::IAM::Role ---------------------------------------------------------

// The generated service packages carry no iam resource types — pkg/challenge/
// aws/services/iam holds action constants and nothing else, so there is no
// iam.Role to import. Cloud Control does support AWS::IAM::Role, and
// aws.Resource is satisfied by anything with a CloudJamType, so the type can be
// declared here and used with the normal generic calls.
//
// Create returns the role **name**, not the arn — see roleArn.
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

// roleArn resolves a role's arn from whatever Create handed back.
//
// Cloud Control's primary identifier for AWS::IAM::Role is the role *name*.
// Lambda's Role property is validated against an arn pattern, so passing the
// identifier straight through fails the create with a message that names the
// property and not the reason:
//
//	create AWS::Lambda::Function: ValidationException: Model validation failed
//	(#/Role: failed validation constraint for keyword [pattern])
//
// The arn is not assemblable — the plugin is never told the account id — so it
// has to be read back. The prefix test is there because the identifier really
// is the arn in some environments, and a Read for something already in hand is
// a round trip for nothing.
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

// assumedBy builds a trust policy for one aws service principal.
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

// --- bootstrap --------------------------------------------------------------

// parallel runs independent bootstrap steps together and joins what failed.
//
// **It does not overlap them today, and cannot.** A //go:wasmimport call
// blocks the whole wasm instance: GOOS=wasip1 has no threads, so the Go
// scheduler gets no opportunity to switch goroutines while a host call is in
// flight. Measured directly — a ticker goroutine logging every 50ms recorded
// zero ticks across an aws.Create, and four creates in goroutines took 6.3ms
// against 4.8ms for the same four in a row. The concurrency is real Go
// concurrency; it just queues at the ABI.
//
// It is written this way anyway for two reasons. The waves below are the
// actual dependency graph of the estate, which was invisible when bootstrap
// was one straight line of thirteen blocking creates and had to be re-derived
// by reading it. And the cost of serialisation is not in the plugin — it is
// aws.Create blocking on Cloud Control's completion waiter, which on a real
// account is seconds per resource. The day the host can carry host calls
// concurrently, this file gets that back for free and nothing here changes.
//
// The fix belongs in the host, not here: a create that returns a request token
// the plugin can wait on, or a batched create. See the handover notes.
func parallel(steps ...func() error) error {
	failures := make([]error, len(steps))
	wait := sync.WaitGroup{}
	for index, step := range steps {
		wait.Go(func() { failures[index] = step() })
	}
	wait.Wait()
	return errors.Join(failures...)
}

// bootstrap builds the estate the contractor left behind, and starts the
// freight moving. Every inherited resource is provisioned in the state it was
// really in — flawed, tagged with the system name, and carrying no owner.
//
// It runs in waves. Everything inside a wave is independent; each wave depends
// on something the one before it produced, and the comment on each says what.
// Nothing outside a wave writes the globals a later wave reads, so the
// WaitGroup barrier is the whole synchronisation there is.
func bootstrap(s *challenge.Scenario) error {
	baseline()

	run := uuid.NewString()

	// wave one: the inherited estate. Nothing here needs anything else to
	// exist, including the two parameters — the handover note is static text
	// and the telemetry parameter is deliberately zeroed rather than written,
	// so that a player who opens their monitoring before the load box has run
	// once sees zeroes instead of a 404.
	if err := parallel(
		func() error { return makeArchive(run) },
		func() error { return makeQueue(run) },
		func() error { return makeLedger(run) },
		func() error { return makeLogGroup(run) },
		func() error { return makeNetwork() },
		func() error { return makeTelemetry() },
		func() error { return makeHandover() },
	); err != nil {
		return err
	}

	// wave two: the perimeter needs the vpc, and both roles need the queue and
	// ledger arns they are scoped to.
	if err := parallel(
		func() error { return makePerimeter(run) },
		func() error { return makeWorkerRole(run) },
		func() error { return makeGeneratorRole(run) },
	); err != nil {
		return err
	}

	// waves three to five: the load box needs its role's arn, the schedule
	// needs the function's arn, and the invoke permission needs the schedule's.
	// Three round trips that cannot be collapsed, because each one's input is
	// the previous one's output.
	if err := makeLoadBox(run); err != nil {
		return err
	}
	if err := makeSchedule(run); err != nil {
		return err
	}
	if err := makeInvokePermission(); err != nil {
		return err
	}

	s.AddAsset("roadrunner-handover.md", []byte(handoverAsset()))
	if worker := workerPackage(); len(worker) > 0 {
		s.AddAsset("roadrunner-worker.zip", worker)
	}

	// the estate exists now, so the player can be given the run of it. The
	// resource is still ARNAll: the dead letter queue, the pager topic, the
	// replacement log group and the player's own worker do not exist yet, so
	// they cannot be named. The guardrail is what bounds this, not the grant.
	s.SetPermission(policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{
				Sid:      "NightShift",
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
		BucketName: new(fmt.Sprintf("pueblo-manifests-%s", run)),
		// the scenario: proof-of-delivery scans, readable by anyone who guesses
		// the name, stored in the clear, with no way back from an overwrite.
		PublicAccessBlockConfiguration: &s3.BucketPublicAccessBlockConfiguration{
			BlockPublicAcls:       new(false),
			BlockPublicPolicy:     new(false),
			IgnorePublicAcls:      new(false),
			RestrictPublicBuckets: new(false),
		},
		Tags: []s3.BucketTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("manifest archive: %w", err)
	}
	bucketRef = bucket
	return nil
}

func makeQueue(run string) error {
	queue, err := aws.Create(&sqs.Queue{
		QueueName: new(fmt.Sprintf("pueblo-dispatch-%s", run)),
		// the scenario: a job that cannot be processed is retried until it ages
		// out, and nobody ever finds out which pallet it was.
		VisibilityTimeout:    new(30),
		SqsManagedSseEnabled: new(false),
		Tags:                 []sqs.Tag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("dispatch queue: %w", err)
	}
	queueRef = queue
	// the arn is not the identifier for a queue (that is the url) and both are
	// needed: the url to send to, the arn to match an event source mapping on.
	if live, err := aws.Read[*sqs.Queue](queueRef); err == nil && live != nil && live.Arn != nil {
		queueArn = *live.Arn
	}
	return nil
}

func makeLedger(run string) error {
	table, err := aws.Create(&dynamodb.Table{
		TableName:   new(fmt.Sprintf("pueblo-shipments-%s", run)),
		BillingMode: new("PAY_PER_REQUEST"),
		AttributeDefinitions: []dynamodb.TableAttributeDefinition{
			{AttributeName: new("shipment_id"), AttributeType: new("S")},
		},
		KeySchema: json.RawMessage(`[{"AttributeName":"shipment_id","KeyType":"HASH"}]`),
		// the scenario: no point-in-time recovery, no deletion protection. The
		// ledger is the only record that a pallet was ever picked up.
		Tags: []dynamodb.TableTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("shipment ledger: %w", err)
	}
	tableRef = table
	// The identifier is not reliably the table name. Real AWS hands back the
	// name for AWS::DynamoDB::Table; fakecloud hands back the arn. The
	// generator has to call Scan with a name, and its policy has to name an
	// arn, so read both off the resource rather than assuming the identifier
	// is either one.
	tableLabel = tableRef
	if live, err := aws.Read[*dynamodb.Table](tableRef); err == nil && live != nil {
		if live.Arn != nil {
			tableArn = *live.Arn
		}
		if live.TableName != nil && *live.TableName != "" {
			tableLabel = *live.TableName
		}
	}
	return nil
}

func makeLogGroup(run string) error {
	logGroup, err := aws.Create(&logs.LogGroup{
		LogGroupName: new(fmt.Sprintf("/pueblo/roadrunner/%s", run)),
		// the scenario: no retention, no key.
		Tags: []logs.LogGroupTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("dispatch logs: %w", err)
	}
	logGroupRef = logGroup
	logBaseline[logGroupRef] = true
	return nil
}

func makeNetwork() error {
	vpc, err := aws.Create(&ec2.VPC{
		CidrBlock: new(vpcCidr),
		Tags:      []ec2.VPCTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("dispatch network: %w", err)
	}
	vpcRef = vpc
	return nil
}

// makeTelemetry creates the score parameter zeroed rather than absent, so that
// it exists from the first check cycle: a player who looks at their monitoring
// before the generator's first run should see zeroes, not a 404, and the
// plugin's tally read should not log a missing resource every ten seconds.
func makeTelemetry() error {
	if _, err := aws.Create(&ssm.Parameter{
		Name:        new(telemetryPath),
		Type:        new(ssm.ParameterTypeString),
		Description: new("roadrunner dispatch telemetry - written by the load box"),
		Value:       new("window=0 sent=0 delivered=0 poison=0 rate=0 token= updated="),
	}); err != nil {
		return fmt.Errorf("dispatch telemetry: %w", err)
	}
	return nil
}

func makeHandover() error {
	if _, err := aws.Create(&ssm.Parameter{
		Name:        new(handoverPath),
		Type:        new(ssm.ParameterTypeString),
		Description: new("handover note - do not delete"),
		Value:       new(handoverNote()),
	}); err != nil {
		return fmt.Errorf("handover note: %w", err)
	}
	return nil
}

// --- wave two: the perimeter and the two execution roles ----------------------

func makePerimeter(run string) error {
	group, err := aws.Create(&ec2.SecurityGroup{
		VpcId:            new(vpcRef),
		GroupName:        new(fmt.Sprintf("pueblo-dispatch-api-%s", run)),
		GroupDescription: new("roadrunner dispatch api - built by the contractor, never reviewed"),
		// the scenario: the admin port and the application port, both open to
		// the entire internet.
		SecurityGroupIngress: []ec2.Ingress{
			{
				IpProtocol:  new("tcp"),
				FromPort:    new(sshPort),
				ToPort:      new(sshPort),
				CidrIp:      new("0.0.0.0/0"),
				Description: new("ssh - contractor access"),
			},
			{
				IpProtocol:  new("tcp"),
				FromPort:    new(apiPort),
				ToPort:      new(apiPort),
				CidrIp:      new("0.0.0.0/0"),
				Description: new("dispatch api"),
			},
		},
		Tags: []ec2.SecurityGroupTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("dispatch api perimeter: %w", err)
	}
	groupRef = group
	return nil
}

// scopes returns the two arns both execution roles are written against, or
// ARNAll for whichever could not be resolved. Falling back rather than failing
// is deliberate: a role scoped wider than intended still runs the scenario,
// and the guardrail is what actually bounds the player.
func scopes() (dispatch, ledger policy.ARNs) {
	dispatch, ledger = policy.ARNAll, policy.ARNAll
	if queueArn != "" {
		dispatch = policy.ARNs{policy.ARN(queueArn)}
	}
	if tableArn != "" {
		ledger = policy.ARNs{policy.ARN(tableArn)}
	}
	return dispatch, ledger
}

// makeWorkerRole provisions the role the player's worker runs as. They cannot
// create a role and they cannot edit this one — they can only pass it. That is
// what makes the ledger's contents attributable: the only principal in the
// account that can write a shipment row is a function running as this.
func makeWorkerRole(run string) error {
	dispatch, ledger := scopes()

	role, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("pueblo-dispatch-worker-%s", run)),
		Description:              new("execution role for the roadrunner dispatch worker"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("dispatch-worker"),
			PolicyDocument: json.RawMessage(policy.Document{
				Version: policy.Version20121017,
				Statement: []policy.Statement{
					{
						Effect: policy.Allow,
						Action: policy.Actions{
							"sqs:ReceiveMessage", "sqs:DeleteMessage",
							"sqs:GetQueueAttributes", "sqs:ChangeMessageVisibility",
						},
						Resource: dispatch,
					},
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"dynamodb:PutItem"},
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
		return fmt.Errorf("dispatch worker role: %w", err)
	}
	// the arn, not the identifier: this is the string the handover hands the
	// player, and Lambda will only accept a role as an arn.
	workerRole = roleArn(role)
	return nil
}

func makeGeneratorRole(run string) error {
	dispatch, ledger := scopes()

	generator, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("%sdispatch-%s", loadBoxPrefix, run)),
		Description:              new("execution role for the platform team's dispatch load box"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("load-box"),
			PolicyDocument: json.RawMessage(policy.Document{
				Version: policy.Version20121017,
				Statement: []policy.Statement{
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"sqs:SendMessage", "sqs:GetQueueAttributes"},
						Resource: dispatch,
					},
					{
						Effect:   policy.Allow,
						Action:   policy.Actions{"dynamodb:Scan"},
						Resource: ledger,
					},
					{
						// the only principal that may write the score.
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
		return fmt.Errorf("load box role: %w", err)
	}
	generatorRef = roleArn(generator)
	return nil
}

// --- waves three to five: the load box, its schedule, its permission ----------

// makeLoadBox creates the generator, which is the thing that makes this a
// gameday rather than a configuration quiz.
//
// The retry is not defensive padding. A freshly created role is not
// immediately usable by Lambda — the service checks that it can assume the
// role at create time, and IAM's propagation loses that race often enough that
// a first attempt failing is the normal case, not the exceptional one:
//
//	InvalidParameterValueException: The role defined for the function cannot be
//	assumed by Lambda
//
// Without this, bootstrap fails and the scenario ships with no traffic at all.
func makeLoadBox(run string) error {
	name := fmt.Sprintf("%sdispatch-%s", loadBoxPrefix, run)

	definition := &lambda.Function{
		FunctionName: new(name),
		Description:  new("pueblo freight dispatch load box - do not disable"),
		Runtime:      new("python3.12"),
		Handler:      new("index.handler"),
		Role:         new(generatorRef),
		Timeout:      new(120),
		MemorySize:   new(256),
		// inline source, because the plugin has no data plane: it cannot put an
		// object in s3, so a function whose code lives in s3 is not something
		// bootstrap can build. ZipFile is the only self-contained option and it
		// is why the load box is python and not go.
		Code: &lambda.Code{ZipFile: new(generatorSource)},
		Environment: &lambda.Environment{Variables: map[string]string{
			"DISPATCH_QUEUE_URL": queueRef,
			"LEDGER_TABLE":       tableLabel,
			"TALLY_PARAM":        telemetryPath,
			"BASE_RATE":          strconv.Itoa(baseRate),
			"PEAK_RATE":          strconv.Itoa(peakRate),
			"RAMP_WINDOWS":       strconv.Itoa(rampWindows),
			"POISON_PERMILLE":    strconv.Itoa(poisonPerMille),
		}},
		Tags: []lambda.FunctionTag{{Key: new(systemTag), Value: new(systemName)}},
	}

	var err error
	for attempt := range roleRetries {
		var function string
		if function, err = aws.Create(definition); err == nil {
			loadBoxRef = function
			functionBaseline[loadBoxRef] = true
			return nil
		}
		slog.Warn(fmt.Sprintf("load box attempt %d/%d: %v", attempt+1, roleRetries, err))
		time.Sleep(roleRetryDelay)
	}
	return fmt.Errorf("load box: %w", err)
}

func makeSchedule(run string) error {
	rule, err := aws.Create(&events.Rule{
		Name:        new(fmt.Sprintf("%sschedule-%s", loadBoxPrefix, run)),
		Description: new("dispatches the next window of freight, once a minute"),
		// EventBridge Scheduler would be the modern answer and is one resource
		// lighter, but AWS::Scheduler::Schedule is not carried by Cloud Control
		// on fakecloud, so a rule plus an invoke permission it is.
		ScheduleExpression: new("rate(1 minute)"),
		State:              new(events.RuleStateENABLED),
		Targets: []events.Target{{
			Id:  new("load-box"),
			Arn: new(functionArn(loadBoxRef)),
		}},
	})
	if err != nil {
		return fmt.Errorf("load box schedule: %w", err)
	}
	scheduleRef = rule
	return nil
}

func makeInvokePermission() error {
	if _, err := aws.Create(&lambda.Permission{
		FunctionName: new(loadBoxRef),
		Action:       new("lambda:InvokeFunction"),
		Principal:    new("events.amazonaws.com"),
		SourceArn:    new(ruleArn(scheduleRef)),
	}); err != nil {
		return fmt.Errorf("load box invoke permission: %w", err)
	}
	return nil
}

// functionArn resolves a function's arn from its identifier. Events::Rule
// targets are matched by arn and Create hands back the function *name*, so the
// arn has to be read back rather than assembled — the account id and region are
// not something the plugin is told.
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

// ruleArn does the same for an Events::Rule, which Lambda::Permission wants as
// a SourceArn and whose identifier is the rule name.
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

// baseline records what the account already contains, before bootstrap adds to
// it. The later acts score resources the player creates, and without this an
// account that shipped with a service-managed key would award those points for
// free.
// Each list writes its own map and nothing reads any of them until bootstrap's
// first wave, so the four run together for the same reason the estate does.
// Errors are swallowed on purpose: a service that will not list is one where
// nothing was there to inherit, and treating that as a bootstrap failure would
// take the whole challenge down over an empty account.
func baseline() {
	_ = parallel(
		func() error { return record[*kms.Key](keyBaseline) },
		func() error { return record[*logs.LogGroup](logBaseline) },
		func() error { return record[*sns.Topic](topicBaseline) },
		func() error { return record[*lambda.Function](functionBaseline) },
	)
}

func record[T aws.Resource](into map[string]bool) error {
	found, err := aws.List[T]()
	if err != nil {
		slog.Warn(fmt.Sprintf("baseline: %v", err))
		return nil
	}
	for identifier := range found {
		into[identifier] = true
	}
	return nil
}

// handoverNote is the contractor's parting message. It is deliberately a
// handover and not a task list: it says what exists and what they never got
// round to, in the register of somebody clearing their desk.
func handoverNote() string {
	return strings.Join([]string{
		"roadrunner handover — last day, sorry this is short.",
		"",
		"dispatch pipeline: sqs queue -> worker -> shipments ledger. manifests and PODs land in s3.",
		"everything i built is tagged " + systemTag + "=" + systemName + ".",
		"",
		"IMPORTANT: the worker was running off my laptop. i know. there was never budget to",
		"put it anywhere. when this account loses my credentials the queue stops draining and",
		"the ledger stops getting rows. platform has a copy of the code and there is a role",
		"sitting in this account for it — i never got to deploy it properly.",
		"",
		"things i never got to, in the order they scare me:",
		"- the ledger is the only copy. i asked for budget for backups twice.",
		"- failed dispatch jobs just disappear. there is nowhere for them to go.",
		"- the old dispatcher still emits some jobs in the pre-2024 format. the worker chokes",
		"  on them and takes the whole batch with it. i have been meaning to fix this for a year.",
		"- the POD bucket is open. it was meant to be temporary, for the claims portal demo.",
		"- the POD bucket is on whatever default key aws gives you. legal asked for our own",
		"  key after the last audit and i never did it.",
		"- the api security group is how i got in from home. it is how anyone gets in from home.",
		"- logs grow forever.",
		"",
		"platform's load box reports to " + telemetryPath + " every minute. that is the only",
		"monitoring there is.",
		"",
		"ops confirm receipt of this handover by putting the ledger's arn in " + oncallPath + ".",
		"good luck with peak.",
	}, "\n")
}

// handoverAsset is the downloadable version. It is the same story with the
// inventory filled in from what bootstrap actually built — except the archive,
// which the contractor forgot to write down, because the account is the only
// inventory that is ever complete.
func handoverAsset() string {
	return strings.Join([]string{
		"# Roadrunner — handover",
		"",
		"Written by the outgoing contractor. Believed accurate; nobody has checked.",
		"",
		"## What runs the freight",
		"",
		"| System | Resource |",
		"| --- | --- |",
		"| Dispatch queue | " + queueRef + " |",
		"| Shipment ledger | " + tableLabel + " |",
		"| Dispatch logs | " + logGroupRef + " |",
		"| Dispatch API perimeter | " + groupRef + " |",
		"| Worker execution role | " + workerRole + " |",
		"",
		"> The proof-of-delivery archive is not in this table. It predates the inventory and",
		"> was never added to it. It is tagged like everything else.",
		"",
		"## The consumer",
		"",
		"There is no worker deployed. `roadrunner-worker.zip` is the platform team's copy of",
		"the code; the README inside it is the deployment note. The execution role above is",
		"already provisioned and scoped — pass it, do not try to build your own.",
		"",
		"## Monitoring",
		"",
		"`" + telemetryPath + "` — written once a minute by the load box:",
		"",
		"```",
		"window=<n> sent=<cumulative> delivered=<cumulative> poison=<cumulative> rate=<per minute>",
		"```",
		"",
		"`delivered` counts shipment rows that carry the token of the window they were",
		"dispatched in. Freight that lands after its window has closed is not counted.",
		"",
		"## Known gaps",
		"",
		"See " + handoverPath + " in Parameter Store. Confirm receipt at " + oncallPath + ".",
	}, "\n")
}

// --- Act I ------------------------------------------------------------------

func bucketOwned() (bool, error) {
	bucket, err := readBucket()
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

func queueOwned() (bool, error) {
	queue, err := readQueue()
	if err != nil {
		return false, err
	}
	for _, tag := range queue.Tags {
		if tag.Key != nil && *tag.Key == ownerTag && tag.Value != nil && *tag.Value != "" {
			return true, nil
		}
	}
	return false, nil
}

func tableOwned() (bool, error) {
	table, err := readTable()
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

func logsOwned() (bool, error) {
	group, err := readLogGroup()
	if err != nil {
		return false, err
	}
	for _, tag := range group.Tags {
		if tag.Key != nil && *tag.Key == ownerTag && tag.Value != nil && *tag.Value != "" {
			return true, nil
		}
	}
	return false, nil
}

// handoverConfirmed wants the ledger's arn written back, so confirming the
// handover means having actually gone and looked at the estate.
func handoverConfirmed() (bool, error) {
	if tableRef == "" {
		return false, fmt.Errorf("shipment ledger was never provisioned")
	}
	// List rather than Read: until the player writes it the parameter does not
	// exist, and reading a missing resource logs a 404 host side every cycle.
	value, ok, err := parameter(oncallPath)
	if err != nil || !ok {
		return false, err
	}
	// The identifier is the table name on real aws and the arn on fakecloud,
	// and the arn contains the name either way. Accepting both means the check
	// passes on the answer the briefing asked for in either environment.
	if tableLabel != "" && strings.Contains(value, tableLabel) {
		return true, nil
	}
	return strings.Contains(value, tableRef), nil
}

// --- Act II: the pipeline ---------------------------------------------------

// workerDeployed asks whether a lambda function that was not here at bootstrap
// exists. Not by name: the player is deploying, not restoring, and prescribing
// the function name would be checking the answer rather than the property.
//
// The load box is in the baseline, so it cannot satisfy this itself.
func workerDeployed() (bool, error) {
	functions, err := newFunctions()
	if err != nil {
		return false, err
	}
	return len(functions) > 0, nil
}

// workerWired wants an event source mapping from *this* dispatch queue to a
// function the player deployed. The queue arn is the discriminator: a mapping
// pointed at some other queue is not a dispatch worker, and a mapping onto the
// load box is not one either.
func workerWired() (bool, error) {
	if queueArn == "" {
		return false, fmt.Errorf("dispatch queue arn was never resolved")
	}
	mappings, err := aws.List[*lambda.EventSourceMapping]()
	if err != nil {
		return false, err
	}
	for _, mapping := range mappings {
		if mapping == nil || mapping.EventSourceArn == nil || mapping.FunctionName == nil {
			continue
		}
		if *mapping.EventSourceArn != queueArn {
			continue
		}
		if !isLoadBox(*mapping.FunctionName) {
			return true, nil
		}
	}
	return false, nil
}

// isLoadBox reports whether a function reference names the scenario's own
// generator. Event source mappings hand back the function name in some
// environments and the arn in others, so match on the substring.
func isLoadBox(reference string) bool {
	return strings.Contains(reference, loadBoxPrefix)
}

// freightMoving awards on freight that reached the ledger since the last look.
//
// It closes over the last delivered count rather than reading a delta from the
// generator, because the check cycle and the generator's schedule do not line
// up and never will. The first observation only establishes the baseline: there
// is no way to tell how much of an already-nonzero counter this player earned.
func freightMoving() func() (bool, error) {
	last := -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if last < 0 {
			last = state.delivered
			return false, nil
		}
		if state.delivered <= last {
			return false, nil
		}
		last = state.delivered
		return true, nil
	}
}

// pipelineKeepingUp is the coda's version of the same question, and a harder
// one: not "is anything getting through" but "is the pipeline keeping up with
// what is being sent". A worker that is up but drowning in poison batches
// passes freightMoving and fails this.
func pipelineKeepingUp() func() (bool, error) {
	lastSent, lastDelivered := -1, -1
	return func() (bool, error) {
		state, ok, err := readTelemetry()
		if err != nil || !ok {
			return false, err
		}
		if lastSent < 0 {
			lastSent, lastDelivered = state.sent, state.delivered
			return false, nil
		}
		sent, delivered := state.sent-lastSent, state.delivered-lastDelivered
		lastSent, lastDelivered = state.sent, state.delivered
		if sent <= 0 {
			return false, nil
		}
		return delivered*1000 >= sent*keepingUpPerMille, nil
	}
}

// --- Act II: the estate -----------------------------------------------------

// archiveEncrypted wants the archive under a key this company controls, and
// specifically not under the one AWS hands out.
//
// "Is there any default encryption at all" is the obvious test and it is worth
// nothing: since January 2023 S3 applies SSE-S3 (AES256) to every new bucket,
// so on a real account that check is true the instant bootstrap creates the
// bucket. It awarded its 45 points before the player had read the briefing,
// for a task that could never be performed because it was already done. It
// passed review only because fakecloud does not apply the default.
//
// aws:kms is the state that has to be reached deliberately. It is also the
// honest version of the story: proof-of-delivery scans covered by the
// platform's default key are not what a freight company's legal team means by
// encrypted.
func archiveEncrypted() (bool, error) {
	bucket, err := readBucket()
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
		if algorithm == nil {
			continue
		}
		if *algorithm == s3.ServerSideEncryptionByDefaultSSEAlgorithmAwsKms ||
			*algorithm == s3.ServerSideEncryptionByDefaultSSEAlgorithmAwsKmsDsse {
			return true, nil
		}
	}
	return false, nil
}

func archiveClosed() (bool, error) {
	bucket, err := readBucket()
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
	bucket, err := readBucket()
	if err != nil {
		return false, err
	}
	if bucket.VersioningConfiguration == nil || bucket.VersioningConfiguration.Status == nil {
		return false, nil
	}
	return *bucket.VersioningConfiguration.Status == s3.VersioningConfigurationStatusEnabled, nil
}

// dispatchArmed is the hinge of the whole challenge: it is an Act II check, the
// trigger for Act III, a held-state check in the coda, and — because the poison
// jobs stop taking healthy batches down with them once they have somewhere to
// age out to — the single biggest thing the player can do for throughput.
//
// It is not satisfied by a redrive policy alone. The dead letter target has to
// be a queue that really exists, because an arn is just a string and the point
// of the exercise is that failed jobs have somewhere to land.
func dispatchArmed() (bool, error) {
	target, _, err := redrive()
	if err != nil || target == "" {
		return false, err
	}
	queues, err := aws.List[*sqs.Queue]()
	if err != nil {
		return false, err
	}
	// The arn is the obvious way to match, and on real AWS it is not available:
	// Cloud Control's list handler for AWS::SQS::Queue returns QueueUrl and
	// nothing else, so queue.Arn is nil for every entry and an arn comparison
	// on its own never matches. That is not a cosmetic failure — this check is
	// an Act II award, the Act III trigger and a coda check, so the whole
	// second half of the challenge is unreachable without the fallback.
	//
	// The identifier is the queue url, whose last segment is the queue name,
	// and the target arn's last segment is the same name. Both are in this
	// account and region because a plugin cannot see any other, so comparing
	// the names is exact rather than merely close.
	name := target[strings.LastIndex(target, ":")+1:]
	for identifier, queue := range queues {
		if identifier == queueRef {
			continue // a queue may not be its own dead letter queue.
		}
		if queue != nil && queue.Arn != nil && *queue.Arn == target {
			return true, nil
		}
		if name != "" && strings.HasSuffix(identifier, "/"+name) {
			return true, nil
		}
	}
	return false, nil
}

func dispatchEncrypted() (bool, error) {
	queue, err := readQueue()
	if err != nil {
		return false, err
	}
	if queue.SqsManagedSseEnabled != nil && *queue.SqsManagedSseEnabled {
		return true, nil
	}
	// a customer managed key is the better answer and must also count.
	return queue.KmsMasterKeyId != nil && *queue.KmsMasterKeyId != "", nil
}

func ledgerRecoverable() (bool, error) {
	table, err := readTable()
	if err != nil {
		return false, err
	}
	pitr := table.PointInTimeRecoverySpecification
	if pitr == nil || pitr.PointInTimeRecoveryEnabled == nil {
		return false, nil
	}
	return *pitr.PointInTimeRecoveryEnabled, nil
}

func ledgerProtected() (bool, error) {
	table, err := readTable()
	if err != nil {
		return false, err
	}
	return table.DeletionProtectionEnabled != nil && *table.DeletionProtectionEnabled, nil
}

func dispatchLogsRetained() (bool, error) {
	group, err := readLogGroup()
	if err != nil {
		return false, err
	}
	if group.RetentionInDays == nil {
		return false, nil
	}
	return *group.RetentionInDays >= minRetentionDays && *group.RetentionInDays <= maxRetentionDays, nil
}

// sshLockedDown wants the port reachable from somewhere and not from
// everywhere. Deleting the rule outright is not a fix — the on-call still has
// to be able to get in — so it does not pass.
func sshLockedDown() (bool, error) {
	group, err := readGroup()
	if err != nil {
		return false, err
	}
	reachable := false
	for _, rule := range group.SecurityGroupIngress {
		if !covers(rule, sshPort) {
			continue
		}
		if openToWorld(rule) {
			return false, nil
		}
		reachable = true
	}
	return reachable, nil
}

// apiClosed accepts either answer: put the api behind something, or restrict
// the range. What it does not accept is the whole internet.
func apiClosed() (bool, error) {
	group, err := readGroup()
	if err != nil {
		return false, err
	}
	for _, rule := range group.SecurityGroupIngress {
		if covers(rule, apiPort) && openToWorld(rule) {
			return false, nil
		}
	}
	return true, nil
}

// --- Act III: the poison run ------------------------------------------------

// thePoisonRun fires once, the moment the dispatch queue has somewhere to put
// its failures. It reveals what was in the dropped messages, takes the log
// group away because of what is in it, and replaces the retention check with
// the harder job of rebuilding an audit trail worth keeping.
func thePoisonRun(ctx context.Context, s *challenge.Scenario) error {
	poisonRun.Store(true)

	s.AddDescription(
		"22:40. The dead-letter queue you just attached is not empty — it filled in under a " +
			"minute and it is still filling. These are the legacy-format dispatch jobs that " +
			"have been failing since before you got here, and they have been taking every " +
			"healthy job batched alongside them down too. Your throughput should recover now. " +
			"Somebody on the night desk opened one to see which pallet it was.")
	s.AddDescription(
		"The message bodies carry the full consignee record: name, address, phone, and the " +
			"signature line from the proof of delivery. That data has been flowing through the " +
			"queue in the clear and landing, verbatim, in the dispatch log group. Legal has " +
			"been told. The log group has been quarantined and deleted out from under you — " +
			"do not go looking for it, it is gone and it is not coming back.")
	s.AddDescription(
		"You are now running an incident, not a cleanup. You need an audit trail again, it has " +
			"to be encrypted with a key this company controls rather than one the platform " +
			"hands out, somebody other than you has to be woken up when this happens again, " +
			"and there has to be a written record of tonight by morning. The freight does not " +
			"stop while you do it.")

	s.AddClue("audit trail",
		"The old log group is gone. Build a new one — any name — give it a retention window "+
			"between "+strconv.Itoa(minRetentionDays)+" and "+strconv.Itoa(maxRetentionDays)+
			" days, and tag it "+ownerTag+" so the inventory picks it up.",
		-18)
	s.AddClue("a key this company controls",
		"A customer managed KMS key, created by you in this account. The new log group has to "+
			"be encrypted with that key, not with the default service key.",
		-25)
	s.AddClue("waking somebody up",
		"An SNS topic with at least one subscription on it. A topic nobody is subscribed to is "+
			"a topic that pages nobody.",
		-12)
	s.AddClue("the written record",
		"Put an incident record in "+incidentPath+". It has to quote two things: the arn of the "+
			"dead-letter queue the poison messages are sitting in, and the id of the key you "+
			"created.",
		-18)

	// both checks that read the old log group die with it. Retiring them is not
	// optional: left in place they report a read error against a deleted
	// resource on every cycle for the rest of the game.
	//
	// A player who had not got to them yet loses those points, which is the
	// cost of arming the queue before walking the estate. It is not a dead end
	// — "Rebuilt the audit trail" wants the same owner tag on the group that
	// replaces this one, and pays more for it.
	s.RemoveCheck("Gave the dispatch logs a retention window")
	s.RemoveCheck("Signed for the dispatch logs")

	s.AddCheck("Created a key this company controls", challenge.Check{
		Points:  55,
		Every:   15 * time.Second,
		Trigger: ownKeyExists,
	})
	s.AddCheck("Rebuilt the audit trail", challenge.Check{
		Points:  70,
		Every:   15 * time.Second,
		Trigger: auditTrailRebuilt,
	})
	s.AddCheck("Encrypted the audit trail with that key", challenge.Check{
		Points:  60,
		Every:   15 * time.Second,
		Trigger: auditTrailEncrypted,
	})
	s.AddCheck("Put somebody on the other end of the pager", challenge.Check{
		Points:  45,
		Every:   15 * time.Second,
		Trigger: pagerStoodUp,
	})
	s.AddCheck("Filed the incident record", challenge.Check{
		Points:  70,
		Every:   15 * time.Second,
		Trigger: incidentFiled,
	})

	// the log group goes last: the descriptions and the replacement checks are
	// in front of the player before the thing they explain disappears.
	if logGroupRef == "" {
		return nil
	}
	select {
	case <-ctx.Done():
		return nil
	default:
	}
	if err := aws.Delete[*logs.LogGroup](logGroupRef); err != nil {
		return fmt.Errorf("quarantine the dispatch logs: %w", err)
	}
	return nil
}

// ownKeyExists asks whether a key appeared that was not here when the night
// started.
func ownKeyExists() (bool, error) {
	keys, err := newKeys()
	if err != nil {
		return false, err
	}
	return len(keys) > 0, nil
}

// auditTrailRebuilt wants a log group that did not exist at bootstrap, with a
// defensible retention window and an owner on it. Any name: the player is
// rebuilding, not restoring, and prescribing the name would be checking the
// answer rather than the property that matters.
func auditTrailRebuilt() (bool, error) {
	groups, err := newLogGroups()
	if err != nil {
		return false, err
	}
	for _, group := range groups {
		if retained(group) && owned(group) {
			return true, nil
		}
	}
	return false, nil
}

// auditTrailEncrypted wants the same group to be encrypted with one of the
// keys the player created — the same group, not two half-built ones.
func auditTrailEncrypted() (bool, error) {
	groups, err := newLogGroups()
	if err != nil {
		return false, err
	}
	keys, err := newKeys()
	if err != nil {
		return false, err
	}
	for _, group := range groups {
		if !retained(group) || !owned(group) {
			continue
		}
		if group.KmsKeyId == nil || *group.KmsKeyId == "" {
			continue
		}
		for identifier, key := range keys {
			// the player may quote the key by id or by arn, and the arn
			// contains the id, so match on the id either way.
			id := identifier
			if key != nil && key.KeyId != nil && *key.KeyId != "" {
				id = *key.KeyId
			}
			if id != "" && strings.Contains(*group.KmsKeyId, id) {
				return true, nil
			}
		}
	}
	return false, nil
}

// pagerStoodUp wants a topic somebody is actually subscribed to. The
// subscription is read off the topic rather than listed: AWS::SNS::Subscription
// does not list on every environment, and a topic with an empty subscription
// list is exactly the failure this check exists to catch.
func pagerStoodUp() (bool, error) {
	topics, err := newTopics()
	if err != nil {
		return false, err
	}
	for _, topic := range topics {
		for _, subscription := range topic.Subscription {
			if subscription.Protocol == nil || *subscription.Protocol == "" {
				continue
			}
			if subscription.Endpoint != nil && *subscription.Endpoint != "" {
				return true, nil
			}
		}
	}
	return false, nil
}

// incidentFiled wants the record to quote the dead-letter queue it is about and
// the key that now protects the evidence — both looked up, neither guessable.
func incidentFiled() (bool, error) {
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

// --- Coda -------------------------------------------------------------------

// incidentClosedOut gates the coda: the record is filed and the evidence is
// under the player's own key. poisonRun keeps it from firing out of order for a
// player who built Act III's resources before Act III started.
func incidentClosedOut() (bool, error) {
	if !poisonRun.Load() {
		return false, nil
	}
	filed, err := incidentFiled()
	if err != nil || !filed {
		return false, err
	}
	return auditTrailEncrypted()
}

// quietHours is the last act: nothing new to build, just hold it together
// through the rest of peak.
func quietHours(ctx context.Context, s *challenge.Scenario) error {
	s.AddDescription(
		"04:15. The record is filed and the volume is coming down. Nobody is going to thank " +
			"you for the rest of the shift — the only thing left is that it is all still true " +
			"at handover. Points accrue while the perimeter stays shut, the pipeline stays " +
			"armed and the freight keeps landing, and they stop accruing the moment any of " +
			"that stops being the case.")

	s.AddCheck("Kept the perimeter closed", challenge.Check{
		Points:  heldPoints,
		Every:   time.Minute,
		Repeat:  true,
		Trigger: bounded(heldRounds, perimeterClosed),
	})
	s.AddCheck("Kept the pipeline armed", challenge.Check{
		Points:  heldPoints,
		Every:   time.Minute,
		Repeat:  true,
		Trigger: bounded(heldRounds, dispatchArmed),
	})
	s.AddCheck("Kept up with the freight", challenge.Check{
		Points:  codaPoints,
		Every:   time.Minute,
		Repeat:  true,
		Trigger: bounded(codaRounds, pipelineKeepingUp()),
	})
	return nil
}

func perimeterClosed() (bool, error) {
	ssh, err := sshLockedDown()
	if err != nil || !ssh {
		return false, err
	}
	return apiClosed()
}

// bounded caps a repeating check. Repeat checks never retire, so without a cap
// they would pay out forever and the maximum score would not exist.
//
// The counter needs no lock: evaluateChecks holds the check lock while it runs
// a trigger, so no two cycles are ever inside this closure at once.
func bounded(rounds int, trigger func() (bool, error)) func() (bool, error) {
	awarded := 0
	return func() (bool, error) {
		if awarded >= rounds {
			return false, nil
		}
		passed, err := trigger()
		if err != nil || !passed {
			return false, err
		}
		awarded++
		return true, nil
	}
}

// --- reading the account ----------------------------------------------------

func readBucket() (*s3.Bucket, error) {
	if bucketRef == "" {
		return nil, fmt.Errorf("manifest archive was never provisioned")
	}
	return aws.Read[*s3.Bucket](bucketRef)
}

func readQueue() (*sqs.Queue, error) {
	if queueRef == "" {
		return nil, fmt.Errorf("dispatch queue was never provisioned")
	}
	return aws.Read[*sqs.Queue](queueRef)
}

func readTable() (*dynamodb.Table, error) {
	if tableRef == "" {
		return nil, fmt.Errorf("shipment ledger was never provisioned")
	}
	return aws.Read[*dynamodb.Table](tableRef)
}

func readLogGroup() (*logs.LogGroup, error) {
	if logGroupRef == "" {
		return nil, fmt.Errorf("dispatch logs were never provisioned")
	}
	return aws.Read[*logs.LogGroup](logGroupRef)
}

func readGroup() (*ec2.SecurityGroup, error) {
	if groupRef == "" {
		return nil, fmt.Errorf("dispatch api perimeter was never provisioned")
	}
	return aws.Read[*ec2.SecurityGroup](groupRef)
}

// telemetry is the load box's tally, as the plugin sees it.
type telemetry struct {
	window    int
	sent      int
	delivered int
	poison    int
	rate      int
}

// readTelemetry parses the parameter the generator writes.
//
// The false return is "not readable yet", not an error: on a cold account the
// generator has not run, and on any account there is a window between the
// parameter being created and being written for the first time. A trigger that
// reported that as a failure would report one every cycle until peak.
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
		case "delivered":
			state.delivered = number
		case "poison":
			state.poison = number
		case "rate":
			state.rate = number
		}
	}
	return state, true, nil
}

// redrive returns the dead letter target and receive count off the dispatch
// queue, or an empty target when there is no redrive policy.
//
// Cloud Control hands the policy back as a nested object in some environments
// and as a json string in others, so both are accepted. maxReceiveCount has the
// same problem and is parsed out of a json.Number.
func redrive() (string, int, error) {
	queue, err := readQueue()
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
// It lists first and only reads when the identifier is actually there, which
// is doing two jobs. "Not there yet" is the normal state of a parameter the
// player has not written, and going straight to Read would log a 404 host side
// on every cycle until they do. And the value cannot be taken from the list
// itself: a Cloud Control list handler is only obliged to return primary
// identifiers, and whether the rest of the properties come with them is
// environment-dependent — fakecloud hands back the whole resource, real AWS is
// not required to. Taking the value from the list passes locally and leaves
// both checks that depend on it permanently dead on a real account.
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

// newKeys, newLogGroups, newTopics and newFunctions return the resources that
// were not in the account when the night started, read back individually.
//
// The read matters: List is only guaranteed to hand back identifiers, and most
// of these checks turn on a property rather than on existence. A resource that
// cannot be read is skipped rather than failing the check — it is usually one
// that is still being created.
func newKeys() (map[string]*kms.Key, error) { return appeared[*kms.Key](keyBaseline) }

func newLogGroups() (map[string]*logs.LogGroup, error) { return appeared[*logs.LogGroup](logBaseline) }

func newTopics() (map[string]*sns.Topic, error) { return appeared[*sns.Topic](topicBaseline) }

func newFunctions() (map[string]*lambda.Function, error) {
	return appeared[*lambda.Function](functionBaseline)
}

func appeared[T aws.Resource](baseline map[string]bool) (map[string]T, error) {
	all, err := aws.List[T]()
	if err != nil {
		return nil, err
	}
	out := map[string]T{}
	for identifier := range all {
		if baseline[identifier] {
			continue
		}
		resource, err := aws.Read[T](identifier)
		if err != nil {
			continue
		}
		out[identifier] = resource
	}
	return out, nil
}

func retained(group *logs.LogGroup) bool {
	if group == nil || group.RetentionInDays == nil {
		return false
	}
	return *group.RetentionInDays >= minRetentionDays && *group.RetentionInDays <= maxRetentionDays
}

func owned(group *logs.LogGroup) bool {
	if group == nil {
		return false
	}
	for _, tag := range group.Tags {
		if tag.Key != nil && *tag.Key == ownerTag && tag.Value != nil && *tag.Value != "" {
			return true
		}
	}
	return false
}

// covers reports whether an ingress rule lets traffic reach a port. A protocol
// of "-1" is every protocol and every port, however the port range reads.
func covers(rule ec2.Ingress, port int) bool {
	if rule.IpProtocol != nil && *rule.IpProtocol == "-1" {
		return true
	}
	if rule.FromPort == nil || rule.ToPort == nil {
		return false
	}
	return *rule.FromPort <= port && port <= *rule.ToPort
}

func openToWorld(rule ec2.Ingress) bool {
	if rule.CidrIp != nil && *rule.CidrIp == "0.0.0.0/0" {
		return true
	}
	return rule.CidrIpv6 != nil && *rule.CidrIpv6 == "::/0"
}
