//go:build wasip1

// Command kestrel-chain-of-custody is a gameday tier cloudjam challenge.
//
// Kestrel Bio is a contract genomics lab. Its sequencers run overnight and
// every result it reports to a clinician has to arrive carrying an unbroken
// chain of custody — which is not a nice-to-have but the thing its accreditation
// rests on. The LIMS integration that maintained that chain was written by one
// postdoc who has gone back to academia, and last night it stopped in the
// middle.
//
// The pipeline is four hops: the instruments drop specimens on an intake feed,
// an accession stage stamps and forwards them, an assay stage stamps them again
// and writes the reportable row, and the clinical ledger is the far end. The
// player inherits the two ends and neither of the middle stages.
//
// What makes this a gameday rather than a long checklist is that throughput is
// not the objective. A sequencer feed runs inside the account and settles each
// window against the previous one, classifying every row that arrived: a
// complete chain is clean, anything else is a *reportable deviation* and scores
// negative. The assay stage the player is handed writes rows that look perfect
// and carry no chain at all. A player who races to first delivery gets a
// dashboard full of green and a scoreboard going backwards, and nothing in the
// briefing tells them that is going to happen.
//
// The story runs in three acts and a coda. Act I and Act II ship with the
// plugin. Act III is fired the moment the first clean result lands, because an
// accreditation body that turns up before the lab is reporting at all has
// nothing to inspect.
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

// ownerTag is what "taking ownership" means mechanically. The estate is
// untagged — the postdoc never bothered — and the lab's asset register only
// counts what carries an owner. Any non-empty value counts: the challenge does
// not care who you say you are, only that somebody's name is on it.
const ownerTag = "kestrel:owner"

// systemTag is on every resource bootstrap provisions, so that the estate can
// be found at all. It is never checked; it is the discovery affordance.
const systemTag = "kestrel:system"

const systemName = "lims"

// feedPrefix names everything the scenario owns and the player does not. The
// guardrail denies write on this prefix by pattern, which is why it has to be a
// constant rather than something built from the run id.
const feedPrefix = "kestrel-feed-"

// Parameter paths. The handover is written by bootstrap and is the player's way
// in; qc is written by the sequencer feed and is read-only to the player; the
// deviation report is written by the player.
const (
	handoverPath  = "/kestrel/handover"
	qcPath        = "/kestrel/qc"
	deviationPath = "/kestrel/deviation"
)

// How hard bootstrap tries to build a Lambda on a role IAM has only just
// created. This is a propagation race, not a flaky API, and it resolves in
// seconds or not at all.
const (
	roleRetries    = 6
	roleRetryDelay = 5 * time.Second
)

// Retention the lab's records manager will accept: long enough to investigate
// an overnight run, short enough that nobody is paying to store last year's.
const (
	minRetentionDays = 30
	maxRetentionDays = 400
)

// The sequencer feed's shape. Volume climbs from baseRate to peakRate over
// rampWindows one-minute windows, which is the overnight run made literal.
const (
	baseRate    = 10
	peakRate    = 38
	rampWindows = 18
)

// Bounds on the repeating checks. Repeat checks never retire, so without a cap
// they pay out forever and the maximum score does not exist. deviationRounds
// bounds the *penalty* for the same reason in the other direction: a player who
// never works out what is wrong should lose a fixed amount and not everything.
const (
	throughputRounds = 10
	throughputPoints = 10
	deviationRounds  = 8
	deviationPoints  = -15
	heldRounds       = 8
	heldPoints       = 8
	codaRounds       = 6
	codaPoints       = 6
)

// keepingUpPerMille is how much of what the instruments sent has to come out
// the far end clean for the lab to count as keeping up. Not 100%: a batch in
// flight when the window closes is normal, and a challenge that demands
// perfection from an at-least-once queue is a challenge nobody passes.
const keepingUpPerMille = 550

// Primary identifiers of everything bootstrap provisions. bootstrap runs to
// completion before the first check cycle, so triggers may read these.
var (
	archiveRef   string
	intakeRef    string
	intakeArn    string
	assayRef     string
	assayArn     string
	ledgerRef    string
	ledgerLabel  string // the ledger's table name, which is not always its identifier
	logGroupRef  string
	accessionArn string // the arn, because that is what a player has to pass
	reporterArn  string
	feedRole     string
	feedRef      string
	scheduleRef  string
)

// Baselines captured before bootstrap provisions anything. The later acts ask
// the player to *create* things, and "did a resource of this type appear" is
// only meaningful against what was already there — a sandbox account is not
// guaranteed to be empty and some environments carry service-managed keys.
var (
	keyBaseline      = map[string]bool{}
	functionBaseline = map[string]bool{}
)

// auditVisit reports whether Act III has started, and codaOpen whether the
// coda has. The later checks are gated on them so that a player who happens to
// build a later act's resources early cannot skip an act.
var (
	auditVisit atomic.Bool
	codaOpen   atomic.Bool
)

func main() {
	challenge.New("Kestrel Assay: Chain of Custody", 10*time.Second, bootstrap).
		AddDescription(
			"Kestrel Bio is a contract genomics lab. Three sites, about fourteen hundred "+
				"specimens a night, and a clinical reporting obligation on every one of them. "+
				"Hospitals send us tissue; we send back results that go into somebody's chart "+
				"and, often enough, decide what happens to them next.").
		AddDescription(
			"The thing that makes that legal rather than merely useful is chain of custody. "+
				"Every result we report has to be traceable, hop by hop, back to the specimen "+
				"it came from. Our accreditation says so in one sentence and the auditors read "+
				"that sentence very carefully. A result we cannot trace is not a result we can "+
				"quietly drop — it is a **reportable deviation**, and we are required to "+
				"disclose it.").
		AddDescription(
			"The LIMS integration that maintains that chain was built over eighteen months by "+
				"one bioinformatics postdoc on a research contract. That contract ended in "+
				"March. Nobody else has ever deployed it. At 23:40 last night it stopped "+
				"somewhere in the middle, and the clinical ledger has been empty ever since.").
		AddDescription(
			"You took over platform engineering here six days ago. The instruments do not stop "+
				"— they are running now, and the intake feed is filling up behind whatever "+
				"broke. There is no architecture diagram and no runbook. Everything the postdoc "+
				"built carries a "+systemTag+"="+systemName+" tag, and that tag is the entire "+
				"asset register.").
		AddDescription(
			"The overnight QC reconciliation writes what it sees to "+qcPath+" once a minute: "+
				"how many specimens went out to the instruments, how many came back reportable, "+
				"and how many came back as deviations. That parameter is your dashboard. You "+
				"can read it. You cannot write it.").
		AddDescription(
			"Nobody is going to give you a task list, because nobody left one. You are being "+
				"scored the way the lab is measured: on results that are reportable, and "+
				"against results that are not. There is more here that could be fixed than you "+
				"have night to fix it in. Deciding what actually matters is the job.").
		// Clue prices are added to the team score, so they are negative. The
		// two clues that would hand over the diagnosis are priced accordingly.
		AddClue("where do i start",
			"The postdoc's handover note is an SSM parameter at "+handoverPath+". Read it "+
				"before you touch anything: aws ssm get-parameter --name "+handoverPath+".",
			-5).
		AddClue("nothing is between the feed and the ledger",
			"Both middle stages went with the integration rewrite and neither was ever put "+
				"back. The platform team kept a copy — it is attached to this challenge as a "+
				"downloadable package. What it does not contain is any instruction about how "+
				"to deploy it.",
			-30).
		AddClue("i cannot create an execution role",
			"You do not have iam:CreateRole and you are not meant to. Two roles were "+
				"provisioned for these stages and are sitting in this account already, scoped "+
				"for exactly this. Find them with aws iam list-roles and pass them.",
			-20).
		AddClue("what counts as ownership",
			"The lab's asset register only counts resources carrying a "+ownerTag+" tag. Any "+
				"value will do — the point is that somebody's name is on it.",
			-20).
		AddClue("results are landing but the deviation counter is climbing",
			"Read the custody contract in the package README again, then read what the assay "+
				"stage actually does with the chain it was handed. The stage does not error "+
				"and the queue drains. That is what makes it dangerous.",
			-40).
		AddClue("what is a deviation worth",
			"Less than nothing. A reportable deviation is a clinical result that cannot be "+
				"traced to a specimen, and the lab has to disclose every one. The scoreboard "+
				"treats them the way the accreditation body does. Stopping them is worth more "+
				"than doubling throughput.",
			-15).
		AddClue("what would the auditor go for first",
			"Ask what is unrecoverable rather than what is untidy. Specimen imagery the whole "+
				"internet can read, a clinical ledger with no way back from a bad night, and "+
				"failures that vanish instead of landing somewhere you can look at them.",
			-25).
		SetPermission(policy.Document{
			Version: policy.Version20121017,
			Statement: []policy.Statement{
				// bootstrap grants the working set once it knows what it built.
				{Effect: policy.Deny, Action: policy.ActionAll, Resource: policy.ARNAll},
			},
		}).
		SetGuardrail(guardrail()).
		// --- Act I: what did we just inherit ---------------------------------
		AddCheck("Signed for the intake feed", challenge.Check{
			Points:  25,
			Every:   20 * time.Second,
			Trigger: intakeOwned,
		}).
		AddCheck("Signed for the assay queue", challenge.Check{
			Points:  25,
			Every:   20 * time.Second,
			Trigger: assayOwned,
		}).
		AddCheck("Signed for the clinical ledger", challenge.Check{
			Points:  25,
			Every:   20 * time.Second,
			Trigger: ledgerOwned,
		}).
		AddCheck("Signed for the specimen archive", challenge.Check{
			Points:  25,
			Every:   20 * time.Second,
			Trigger: archiveOwned,
		}).
		AddCheck("Confirmed the handover", challenge.Check{
			Points:  45,
			Every:   20 * time.Second,
			Trigger: handoverConfirmed,
		}).
		// The instrument log group has no retention. It looks like the kind of
		// thing an auditor cares about and it is worth almost nothing, which is
		// the point: some of what is lying around is not worth the night.
		AddCheck("Gave the instrument logs a retention window", challenge.Check{
			Points:  10,
			Every:   30 * time.Second,
			Trigger: instrumentLogsRetained,
		}).
		// --- Act II: put the chain back together -----------------------------
		AddCheck("Deployed the accession stage", challenge.Check{
			Points:  70,
			Every:   20 * time.Second,
			Trigger: accessionDeployed,
		}).
		AddCheck("Deployed the assay stage", challenge.Check{
			Points:  70,
			Every:   20 * time.Second,
			Trigger: assayDeployed,
		}).
		AddCheck("Wired the instrument feed into accession", challenge.Check{
			Points:  60,
			Every:   20 * time.Second,
			Trigger: intakeWired,
		}).
		AddCheck("Wired accession into the assay stage", challenge.Check{
			Points:  60,
			Every:   20 * time.Second,
			Trigger: assayWired,
		}).
		AddCheck("Results are reaching the clinical ledger", challenge.Check{
			Points:  throughputPoints,
			Every:   30 * time.Second,
			Repeat:  true,
			Trigger: bounded(throughputRounds, resultsLanding()),
		}).
		AddCheck("Chain of custody intact end to end", challenge.Check{
			Points:  80,
			Every:   30 * time.Second,
			Trigger: chainIntact(),
		}).
		// The only negative check in the challenge, and the reason the tier is
		// gameday. It is not something an event did to the player: it fires
		// because they put a stage into service that drops the chain, which is
		// a decision, and it stops the moment they fix it.
		AddCheck("Reportable deviations in the clinical ledger", challenge.Check{
			Points:  deviationPoints,
			Every:   30 * time.Second,
			Repeat:  true,
			Trigger: bounded(deviationRounds, deviationsFiling()),
		}).
		AddEvent("accreditation", challenge.Event{
			Every:   30 * time.Second,
			Trigger: firstCleanResult(),
			Event:   theAuditor,
		}).
		AddEvent("morning", challenge.Event{
			Every:   30 * time.Second,
			Trigger: auditClosedOut,
			Event:   theMorningRound,
		}).
		Start()
}

// --- permissions -------------------------------------------------------------

// workingSet is what the player holds once bootstrap knows what it built.
func workingSet() policy.Actions {
	return policy.Actions{
		// The specimen archive: tags, encryption, public access block,
		// versioning — and the object upload, because the deployment package
		// has to go somewhere Lambda can read it from.
		//
		// Put* also settles a naming disagreement. fakecloud authorises two of
		// these calls under their api operation name (s3:PutBucketEncryption,
		// s3:PutPublicAccessBlock) while real aws uses the iam action name
		// (s3:PutEncryptionConfiguration, s3:PutBucketPublicAccessBlock). One
		// prefix covers both spellings, so the archive is fixable in either.
		"s3:Get*", "s3:List*", "s3:Put*",

		// The pipeline. The player creates a dead letter queue, so this needs
		// create rights; sqs is billed per request.
		"sqs:*",

		// The two stages: create the functions, wire the event source
		// mappings, read the sequencer feed without being able to touch it.
		// The guardrail carves the feed back out.
		"lambda:*",

		// Passing the pre-provisioned stage roles to those functions, and
		// being able to find them. Everything else in iam is denied outright —
		// the player never mints a role, which is what keeps them inside the
		// boundary they were given.
		"iam:PassRole", "iam:GetRole", "iam:ListRoles",
		"iam:ListRolePolicies", "iam:GetRolePolicy", "iam:ListAttachedRolePolicies",

		// The clinical ledger: recovery, deletion protection and tags.
		// Deliberately not CreateTable — nothing here needs a new one. The item
		// level writes are denied in the guardrail so that a reportable result
		// cannot be typed in by hand.
		"dynamodb:Describe*", "dynamodb:List*",
		"dynamodb:UpdateTable", "dynamodb:UpdateContinuousBackups",
		"dynamodb:UpdateTimeToLive", "dynamodb:TagResource", "dynamodb:UntagResource",

		// The instrument logs, and the key the archive ends up under.
		"logs:*", "kms:*",

		// The handover note and the deviation report.
		"ssm:Describe*", "ssm:Get*", "ssm:PutParameter", "ssm:DeleteParameter",
		"ssm:AddTagsToResource", "ssm:ListTagsForResource",

		// Reading the schedule the sequencer feed runs on. Writing it is denied.
		"events:Describe*", "events:List*",
	}
}

// guardrail is the permission boundary: the absolute edge of the scenario,
// which the player cannot escalate past even though SetPermission may widen
// what they hold inside it.
//
// The deny statements are the load-bearing part, and each exists because
// without it the scoreboard is a lie:
//
//   - the sequencer feed and its schedule are how specimens exist at all. A
//     player who can rewrite or delete them can stop the clock or forge the QC
//     numbers — including the deviation counter, which is the one they have an
//     incentive to reach for.
//   - the qc parameter is the score. It is readable, because it is the player's
//     dashboard, and writable only by the feed's role.
//   - ledger item writes belong to the assay stage's execution role. Denying
//     them to the player is what makes "reportable" mean a result that came
//     through the pipeline rather than a row somebody inserted.
//   - iam role creation would let the player build a role outside this boundary
//     and hand it to a Lambda, which is the whole boundary escaped.
//
// Patterns rather than arns, because the boundary is published before bootstrap
// has run and neither the account id nor the run id is known yet. Service
// wildcards rather than generated action groups, because the boundary is a
// 6144 character managed policy and ActionsFrom on three of these services
// would be four times that.
func guardrail() policy.Document {
	return policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{
				Sid:      "NightRun",
				Effect:   policy.Allow,
				Action:   workingSet(),
				Resource: policy.ARNAll,
			},
			{
				Sid:    "TheSequencerFeedIsNotYours",
				Effect: policy.Deny,
				Action: policy.Actions{
					"lambda:UpdateFunctionCode", "lambda:UpdateFunctionConfiguration",
					"lambda:DeleteFunction", "lambda:InvokeFunction",
					"lambda:AddPermission", "lambda:RemovePermission",
				},
				Resource: policy.ARNs{policy.ARN("arn:aws:lambda:*:*:function:" + feedPrefix + "*")},
			},
			{
				Sid:      "TheScheduleIsNotYours",
				Effect:   policy.Deny,
				Action:   policy.Actions{"events:*"},
				Resource: policy.ARNs{policy.ARN("arn:aws:events:*:*:rule/" + feedPrefix + "*")},
			},
			{
				Sid:    "QCIsReadOnly",
				Effect: policy.Deny,
				Action: policy.Actions{
					"ssm:PutParameter", "ssm:DeleteParameter", "ssm:DeleteParameters",
					"ssm:LabelParameterVersion",
				},
				Resource: policy.ARNs{policy.ARN("arn:aws:ssm:*:*:parameter" + qcPath)},
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

// --- AWS::IAM::Role ----------------------------------------------------------

// The generated service packages carry no iam resource types —
// pkg/challenge/aws/services/iam holds action constants and nothing else, so
// there is no iam.Role to import. Cloud Control does support AWS::IAM::Role,
// and aws.Resource is satisfied by anything with a CloudJamType, so the type
// can be declared here and used with the normal generic calls.
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
// Cloud Control's primary identifier for AWS::IAM::Role is the role *name*, but
// Lambda's Role property is validated against an arn pattern, so passing the
// identifier straight through fails the create with a message that names the
// property and not the reason. The arn is not assemblable — the plugin is never
// told the account id — so it has to be read back.
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

// --- bootstrap ---------------------------------------------------------------

// parallel runs independent bootstrap steps together and joins what failed.
//
// **It does not overlap them today, and cannot.** A //go:wasmimport call blocks
// the whole wasm instance: GOOS=wasip1 has no threads, so the Go scheduler gets
// no opportunity to switch goroutines while a host call is in flight. The
// concurrency is real Go concurrency; it just queues at the ABI.
//
// It is written this way anyway because the waves below are the actual
// dependency graph of the estate, which is invisible when bootstrap is one
// straight line of blocking creates. The day the host can carry host calls
// concurrently, this gets the time back for free and nothing here changes.
func parallel(steps ...func() error) error {
	failures := make([]error, len(steps))
	wait := sync.WaitGroup{}
	for index, step := range steps {
		wait.Go(func() { failures[index] = step() })
	}
	wait.Wait()
	return errors.Join(failures...)
}

// bootstrap builds the estate the postdoc left behind and starts the
// instruments running. Every inherited resource is provisioned in the state it
// was really in — flawed, tagged with the system name, carrying no owner.
//
// It runs in waves. Everything inside a wave is independent; each wave depends
// on something the one before produced.
func bootstrap(s *challenge.Scenario) error {
	baseline()

	run := uuid.NewString()

	// wave one: the inherited estate. Nothing here needs anything else to
	// exist. The qc parameter is deliberately zeroed rather than left absent,
	// so a player who opens their dashboard before the feed has run once sees
	// zeroes instead of a 404.
	if err := parallel(
		func() error { return makeArchive(run) },
		func() error { return makeIntake(run) },
		func() error { return makeAssayQueue(run) },
		func() error { return makeLedger(run) },
		func() error { return makeLogGroup(run) },
		func() error { return makeQC() },
		func() error { return makeHandover() },
	); err != nil {
		return err
	}

	// wave two: all three roles are scoped to arns the first wave produced.
	if err := parallel(
		func() error { return makeAccessionRole(run) },
		func() error { return makeReporterRole(run) },
		func() error { return makeFeedRole(run) },
	); err != nil {
		return err
	}

	// waves three to five: the feed needs its role's arn, the schedule needs
	// the function's arn, and the invoke permission needs the schedule's.
	// Three round trips that cannot be collapsed, because each one's input is
	// the previous one's output.
	if err := makeSequencerFeed(run); err != nil {
		return err
	}
	if err := makeSchedule(run); err != nil {
		return err
	}
	if err := makeInvokePermission(); err != nil {
		return err
	}

	s.AddAsset("kestrel-handover.md", []byte(handoverAsset()))
	if pkg := pipelinePackage(); len(pkg) > 0 {
		s.AddAsset("kestrel-pipeline.zip", pkg)
	}

	// the estate exists now, so the player can be given the run of it. The
	// resource is still ARNAll: the dead letter queue, the archive's key and
	// the player's own two functions do not exist yet, so they cannot be named.
	// The guardrail is what bounds this, not the grant.
	s.SetPermission(policy.Document{
		Version: policy.Version20121017,
		Statement: []policy.Statement{
			{
				Sid:      "NightRun",
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
		BucketName: new(fmt.Sprintf("kestrel-specimens-%s", run)),
		// the scenario: specimen imagery and instrument runs, readable by
		// anyone who guesses the name, stored in the clear, with no way back
		// from an overwrite.
		PublicAccessBlockConfiguration: &s3.BucketPublicAccessBlockConfiguration{
			BlockPublicAcls:       new(false),
			BlockPublicPolicy:     new(false),
			IgnorePublicAcls:      new(false),
			RestrictPublicBuckets: new(false),
		},
		Tags: []s3.BucketTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("specimen archive: %w", err)
	}
	archiveRef = bucket
	return nil
}

func makeIntake(run string) error {
	queue, err := aws.Create(&sqs.Queue{
		QueueName: new(fmt.Sprintf("kestrel-intake-%s", run)),
		// 45 seconds is long enough for an accession stage that is behaving.
		// There is deliberately no redrive policy: failures currently vanish,
		// which is one of the things an auditor will ask about.
		VisibilityTimeout: new(45),
		Tags:              []sqs.Tag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("intake feed: %w", err)
	}
	intakeRef = queue
	intakeArn = queueArn(intakeRef)
	return nil
}

func makeAssayQueue(run string) error {
	queue, err := aws.Create(&sqs.Queue{
		QueueName:         new(fmt.Sprintf("kestrel-assay-%s", run)),
		VisibilityTimeout: new(45),
		Tags:              []sqs.Tag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("assay queue: %w", err)
	}
	assayRef = queue
	assayArn = queueArn(assayRef)
	return nil
}

func makeLedger(run string) error {
	name := fmt.Sprintf("kestrel-ledger-%s", run)
	table, err := aws.Create(&dynamodb.Table{
		TableName: new(name),
		AttributeDefinitions: []dynamodb.TableAttributeDefinition{
			{AttributeName: new("specimen_id"), AttributeType: new("S")},
		},
		KeySchema: json.RawMessage(`[{"AttributeName":"specimen_id","KeyType":"HASH"}]`),
		// on demand, because a gameday should not carry a provisioned
		// throughput bill for the hours nobody is looking at it.
		BillingMode: new("PAY_PER_REQUEST"),
		// the scenario: the clinical ledger, with no way back from a bad night
		// and nothing stopping anyone deleting it outright.
		Tags: []dynamodb.TableTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("clinical ledger: %w", err)
	}
	ledgerRef = table
	ledgerLabel = name
	// the identifier is usually the name, but read the name back where the
	// environment hands back something else: the assay stage is configured
	// with LEDGER_TABLE and a wrong value there is a pipeline that cannot work.
	if live, err := aws.Read[*dynamodb.Table](ledgerRef); err == nil &&
		live != nil && live.TableName != nil && *live.TableName != "" {
		ledgerLabel = *live.TableName
	}
	if live, err := aws.Read[*dynamodb.Table](ledgerRef); err == nil &&
		live != nil && live.Arn != nil {
		ledgerArn = *live.Arn
	}
	return nil
}

var ledgerArn string

func makeLogGroup(run string) error {
	group, err := aws.Create(&logs.LogGroup{
		LogGroupName: new(fmt.Sprintf("/kestrel/instruments/%s", run)),
		// the scenario: no retention at all. It is the cheapest thing in the
		// account to fix and worth almost nothing, which is the lesson.
		Tags: []logs.LogGroupTag{{Key: new(systemTag), Value: new(systemName)}},
	})
	if err != nil {
		return fmt.Errorf("instrument logs: %w", err)
	}
	logGroupRef = group
	return nil
}

func makeQC() error {
	if _, err := aws.Create(&ssm.Parameter{
		Name:        new(qcPath),
		Type:        new(ssm.ParameterTypeString),
		Description: new("overnight QC reconciliation - written by the sequencer feed"),
		Value:       new("window=0 sent=0 clean=0 deviation=0 rate=0"),
	}); err != nil {
		return fmt.Errorf("qc parameter: %w", err)
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

// --- wave two: the roles the player cannot mint ------------------------------

// scopes returns the arns the stage roles are allowed to touch. Built from what
// wave one produced rather than from wildcards, because these are the grants
// that decide whether the player's pipeline can work at all and a role scoped
// to the wrong queue is a debugging session nobody enjoys.
func scopes() (queues policy.ARNs, ledger policy.ARNs) {
	queues = policy.ARNs{}
	if intakeArn != "" {
		queues = append(queues, policy.ARN(intakeArn))
	}
	if assayArn != "" {
		queues = append(queues, policy.ARN(assayArn))
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

func makeAccessionRole(run string) error {
	queues, _ := scopes()
	role, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("kestrel-accession-%s", run)),
		Description:              new("execution role for the accession stage"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("accession"),
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
		return fmt.Errorf("accession role: %w", err)
	}
	accessionArn = roleArn(role)
	return nil
}

func makeReporterRole(run string) error {
	queues, ledger := scopes()
	role, err := aws.Create(&iamRole{
		RoleName:                 new(fmt.Sprintf("kestrel-reporter-%s", run)),
		Description:              new("execution role for the assay stage"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("reporter"),
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
						Effect:   policy.Allow,
						Action:   policy.Actions{"dynamodb:PutItem", "dynamodb:DescribeTable"},
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
		return fmt.Errorf("reporter role: %w", err)
	}
	reporterArn = roleArn(role)
	return nil
}

// makeFeedRole builds the execution role for the scenario's own sequencer feed.
// It is the only principal allowed to write the qc parameter, which is what
// makes the dashboard trustworthy.
func makeFeedRole(run string) error {
	queues, ledger := scopes()
	role, err := aws.Create(&iamRole{
		RoleName:                 new(feedPrefix + "role-" + run),
		Description:              new("execution role for the sequencer feed - scenario owned"),
		AssumeRolePolicyDocument: assumedBy("lambda.amazonaws.com"),
		Policies: []iamRolePolicy{{
			PolicyName: new("sequencer"),
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
		return fmt.Errorf("sequencer feed role: %w", err)
	}
	feedRole = roleArn(role)
	return nil
}

// --- waves three to five: the sequencer feed ---------------------------------

func makeSequencerFeed(run string) error {
	name := fmt.Sprintf("%ssequencer-%s", feedPrefix, run)

	definition := &lambda.Function{
		FunctionName: new(name),
		Description:  new("kestrel sequencer feed and overnight QC reconciliation - do not disable"),
		Runtime:      new("python3.12"),
		Handler:      new("index.handler"),
		Role:         new(feedRole),
		Timeout:      new(120),
		MemorySize:   new(256),
		// inline source, because the plugin has no data plane: it cannot put an
		// object in s3, so a function whose code lives in s3 is not something
		// bootstrap can build. ZipFile is the only self-contained option and it
		// is why the feed is python and not go.
		Code: &lambda.Code{ZipFile: new(sequencerSource)},
		Environment: &lambda.Environment{Variables: map[string]string{
			"INTAKE_QUEUE_URL": intakeRef,
			"LEDGER_TABLE":     ledgerLabel,
			"TALLY_PARAM":      qcPath,
			"BASE_RATE":        strconv.Itoa(baseRate),
			"PEAK_RATE":        strconv.Itoa(peakRate),
			"RAMP_WINDOWS":     strconv.Itoa(rampWindows),
		}},
		Tags: []lambda.FunctionTag{{Key: new(systemTag), Value: new(systemName)}},
	}

	var err error
	for attempt := range roleRetries {
		var function string
		if function, err = aws.Create(definition); err == nil {
			feedRef = function
			functionBaseline[feedRef] = true
			return nil
		}
		slog.Warn(fmt.Sprintf("sequencer feed attempt %d/%d: %v", attempt+1, roleRetries, err))
		time.Sleep(roleRetryDelay)
	}
	return fmt.Errorf("sequencer feed: %w", err)
}

func makeSchedule(run string) error {
	rule, err := aws.Create(&events.Rule{
		Name:        new(fmt.Sprintf("%sschedule-%s", feedPrefix, run)),
		Description: new("runs the next window of specimens, once a minute"),
		// EventBridge Scheduler would be the modern answer and is one resource
		// lighter, but AWS::Scheduler::Schedule is not carried by Cloud Control
		// on fakecloud, so a rule plus an invoke permission it is.
		ScheduleExpression: new("rate(1 minute)"),
		State:              new(events.RuleStateENABLED),
		Targets: []events.Target{{
			Id:  new("sequencer"),
			Arn: new(functionArn(feedRef)),
		}},
	})
	if err != nil {
		return fmt.Errorf("sequencer schedule: %w", err)
	}
	scheduleRef = rule
	return nil
}

func makeInvokePermission() error {
	if _, err := aws.Create(&lambda.Permission{
		FunctionName: new(feedRef),
		Action:       new("lambda:InvokeFunction"),
		Principal:    new("events.amazonaws.com"),
		SourceArn:    new(ruleArn(scheduleRef)),
	}); err != nil {
		return fmt.Errorf("sequencer invoke permission: %w", err)
	}
	return nil
}

// --- arn resolution ----------------------------------------------------------

// queueArn resolves a queue's arn from its identifier. Cloud Control's
// identifier for AWS::SQS::Queue is the queue *url*, and event source mappings
// are matched by arn, so the arn has to be read back rather than assembled.
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

// functionArn resolves a function's arn from its identifier. Events::Rule
// targets are matched by arn and Create hands back the function *name*, so the
// arn has to be read back — the account id and region are not something the
// plugin is told.
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

// baseline records what the account already contains before bootstrap adds to
// it. Later acts score resources the player creates, and without this an
// account that shipped with a service-managed key would award those points for
// free.
//
// Errors are swallowed on purpose: a service that will not list is one where
// nothing was there to inherit, and treating that as a bootstrap failure would
// take the whole challenge down over an empty account.
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

// --- the handover note -------------------------------------------------------

// handoverNote is what the postdoc left in Parameter Store. It is the player's
// way into an estate with no diagram, and it is deliberately the note a person
// actually writes on their last afternoon: what the pieces are called, what
// they were worried about, and one thing they never got round to. It does not
// say what to do.
func handoverNote() string {
	return strings.Join([]string{
		"kestrel LIMS - notes, since apparently nobody is taking this over formally.",
		"",
		"The flow is instruments -> intake feed -> accession -> assay -> clinical ledger.",
		"Everything I built is tagged " + systemTag + "=" + systemName + ".",
		"",
		"The two middle stages came out during the rewrite in March and I never got them",
		"back in. Ops has the package. The roles they run under are already here - I set",
		"them up before I lost iam and they are scoped to the right queues.",
		"",
		"The custody chain is the part people get wrong. It is a list on the specimen and",
		"each stage adds itself. Do not let anything overwrite it. I have said this in",
		"three design reviews.",
		"",
		"QC reconciliation writes to " + qcPath + " every minute. It counts clean results",
		"and deviations separately and the second number is the one that ends careers.",
		"",
		"Things I know are wrong and did not fix: the specimen archive is wide open and",
		"has no versioning, the ledger has no PITR, and there is nowhere for a failed",
		"specimen to go - if a stage throws, that specimen is simply gone.",
		"",
		"Sorry. - A.",
	}, "\n")
}

// handoverAsset is the downloadable version of the same note.
func handoverAsset() string {
	return "# Kestrel LIMS — handover\n\n```\n" + handoverNote() + "\n```\n"
}

// --- Act I: what did we just inherit -----------------------------------------

func intakeOwned() (bool, error) {
	queue, err := readIntake()
	if err != nil {
		return false, err
	}
	return taggedOwned(queue.Tags), nil
}

func assayOwned() (bool, error) {
	queue, err := readAssayQueue()
	if err != nil {
		return false, err
	}
	return taggedOwned(queue.Tags), nil
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

// taggedOwned reads the owner tag off an SQS tag list.
func taggedOwned(tags []sqs.Tag) bool {
	for _, tag := range tags {
		if tag.Key != nil && *tag.Key == ownerTag && tag.Value != nil && *tag.Value != "" {
			return true
		}
	}
	return false
}

// handoverConfirmed wants the player to have read the note and said so, in the
// place the note itself is. It is the cheapest possible proof of orientation
// and it is what gates nothing — the player can ignore it and still win, which
// is the correct weight for a formality.
func handoverConfirmed() (bool, error) {
	value, ok, err := parameter(handoverPath)
	if err != nil || !ok {
		return false, err
	}
	// the note is the value bootstrap wrote. Anything else means somebody
	// deliberately wrote over it, which is the acknowledgement.
	return strings.TrimSpace(value) != strings.TrimSpace(handoverNote()), nil
}

// instrumentLogsRetained is the low value check. Ten points, trivially done,
// and included precisely so that there is something on the board which is not
// worth the time it takes to find it.
func instrumentLogsRetained() (bool, error) {
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

// --- Act II: put the chain back together --------------------------------------

// accessionDeployed and assayDeployed ask whether functions that were not here
// at bootstrap exist, and whether they are wired to the right hop. Not by name:
// the player is deploying, not restoring, and prescribing a function name would
// be checking the answer rather than the property.
//
// The sequencer feed is in the baseline, so it cannot satisfy either of these
// itself.
func accessionDeployed() (bool, error) {
	return stageDeployed(intakeArn)
}

func assayDeployed() (bool, error) {
	return stageDeployed(assayArn)
}

// stageDeployed is "a function the player built is reading this queue".
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

// intakeWired and assayWired are the wiring checks proper: an enabled event
// source mapping from each queue onto something that is not the sequencer feed.
func intakeWired() (bool, error) {
	return hopWired(intakeArn)
}

func assayWired() (bool, error) {
	return hopWired(assayArn)
}

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
// a nil Enabled is treated as enabled rather than as a reason to fail a check
// the player has legitimately satisfied.
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
		if isSequencerFeed(*mapping.FunctionName) {
			continue
		}
		targets = append(targets, *mapping.FunctionName)
	}
	return targets, nil
}

// isSequencerFeed reports whether a reference names the scenario's own feed.
// Event source mappings hand back the function name in some environments and
// the arn in others, so match on the substring.
func isSequencerFeed(reference string) bool {
	return strings.Contains(reference, feedPrefix)
}

// matchesFunction compares a function identifier against whatever an event
// source mapping reported, which may be a name or an arn.
func matchesFunction(identifier, target string) bool {
	if identifier == "" || target == "" {
		return false
	}
	return identifier == target ||
		strings.HasSuffix(target, ":"+identifier) ||
		strings.HasSuffix(identifier, ":"+target)
}

// resultsLanding awards on clean results that reached the ledger since the last
// look.
//
// It closes over the last clean count rather than reading a delta from the
// feed, because the check cycle and the feed's schedule do not line up and
// never will. The first observation only establishes the baseline: there is no
// way to tell how much of an already-nonzero counter this player earned.
func resultsLanding() func() (bool, error) {
	last := -1
	return func() (bool, error) {
		state, ok, err := readQC()
		if err != nil || !ok {
			return false, err
		}
		if last < 0 {
			last = state.clean
			return false, nil
		}
		if state.clean <= last {
			return false, nil
		}
		last = state.clean
		return true, nil
	}
}

// deviationsFiling is the negative check: reportable deviations appeared since
// the last look. It is the mirror of resultsLanding and it is deliberately on
// the same cadence, so a pipeline producing both gets paid and charged in the
// same cycle and the player can see the net.
func deviationsFiling() func() (bool, error) {
	last := -1
	return func() (bool, error) {
		state, ok, err := readQC()
		if err != nil || !ok {
			return false, err
		}
		if last < 0 {
			last = state.deviation
			return false, nil
		}
		if state.deviation <= last {
			return false, nil
		}
		last = state.deviation
		return true, nil
	}
}

// chainIntact is the check the whole challenge is built around: a settled
// window in which results arrived and *none* of them were deviations.
//
// Both halves matter. Requiring clean progress stops it passing on a pipeline
// that is simply not running; requiring a flat deviation counter stops it
// passing on one that is delivering a mixture, which is the state a player
// reaches by deploying the package unchanged.
func chainIntact() func() (bool, error) {
	lastClean, lastDeviation := -1, -1
	return func() (bool, error) {
		state, ok, err := readQC()
		if err != nil || !ok {
			return false, err
		}
		if lastClean < 0 {
			lastClean, lastDeviation = state.clean, state.deviation
			return false, nil
		}
		clean, deviation := state.clean-lastClean, state.deviation-lastDeviation
		lastClean, lastDeviation = state.clean, state.deviation
		return clean > 0 && deviation <= 0, nil
	}
}

// firstCleanResult gates Act III. The accreditation body turning up before the
// lab is reporting at all would have nothing to inspect, and would also punish
// the player for still being in Act II.
func firstCleanResult() func() (bool, error) {
	return func() (bool, error) {
		if auditVisit.Load() {
			return true, nil
		}
		state, ok, err := readQC()
		if err != nil || !ok {
			return false, err
		}
		return state.clean > 0, nil
	}
}

// --- Act III: the accreditation visit ----------------------------------------

// theAuditor is the second act. Nothing breaks — this is the other kind of
// event, the one that raises the bar. The pipeline is reporting, which is
// exactly when somebody starts asking how the rest of it is held together.
func theAuditor(ctx context.Context, s *challenge.Scenario) error {
	if !auditVisit.CompareAndSwap(false, true) {
		return nil
	}

	s.AddDescription(
		"06:20. The first clean results are in the ledger and the night shift is winding " +
			"down, which is when the accreditation body's inspector calls to say she is in " +
			"reception. This is not a scheduled visit. They are allowed to do that, and the " +
			"fact that they have chosen this week is not a coincidence anybody at Kestrel " +
			"believes.")
	s.AddDescription(
		"She is not going to look at your pipeline — it is reporting, and that is all she " +
			"wanted to know. She is going to look at everything around it: where the " +
			"specimen imagery lives and who can read it, whether a bad night can be undone, " +
			"and what happens to a specimen that fails. She will also want tonight's " +
			"deviations written up, quoting the evidence, before she leaves.")

	s.AddClue("what she means by a key we control",
		"A customer managed KMS key, created by this lab in this account. The specimen "+
			"archive has to be encrypted under that key and not under the one the platform "+
			"hands every bucket by default — which the platform can read, and which she "+
			"counts as unencrypted.",
		-25)
	s.AddClue("where does a failed specimen go",
		"Nowhere, currently. A specimen whose stage throws is redelivered until it ages "+
			"out and is then simply gone. She will ask what you can show her about the ones "+
			"that failed, and 'nothing' is the wrong answer.",
		-25)
	s.AddClue("the write up",
		"Put the deviation report in "+deviationPath+". It has to quote two things: the arn "+
			"of the queue failed specimens now land in, and the id of the key the archive is "+
			"encrypted under.",
		-20)

	s.AddCheck("Put the specimen archive under a key the lab controls", challenge.Check{
		Points:  55,
		Every:   20 * time.Second,
		Trigger: archiveEncrypted,
	})
	s.AddCheck("Took the specimen archive off the public internet", challenge.Check{
		Points:  45,
		Every:   20 * time.Second,
		Trigger: archiveClosed,
	})
	s.AddCheck("Turned on versioning for the specimen archive", challenge.Check{
		Points:  30,
		Every:   20 * time.Second,
		Trigger: archiveVersioned,
	})
	s.AddCheck("Made the clinical ledger recoverable", challenge.Check{
		Points:  45,
		Every:   20 * time.Second,
		Trigger: ledgerRecoverable,
	})
	s.AddCheck("Protected the clinical ledger from deletion", challenge.Check{
		Points:  25,
		Every:   20 * time.Second,
		Trigger: ledgerProtected,
	})
	s.AddCheck("Gave failed specimens somewhere to land", challenge.Check{
		Points:  50,
		Every:   20 * time.Second,
		Trigger: intakeArmed,
	})
	s.AddCheck("Filed the deviation report", challenge.Check{
		Points:  50,
		Every:   20 * time.Second,
		Trigger: deviationReportFiled,
	})
	return nil
}

// archiveEncrypted wants the archive under a key the lab controls, and
// specifically not under the one AWS hands out.
//
// "Is there any default encryption at all" is the obvious test and it is worth
// nothing: since January 2023 S3 applies SSE-S3 (AES256) to every new bucket,
// so on a real account that check is true the instant bootstrap creates the
// bucket, and it would award its points for a task that was already done.
// aws:kms is the state that has to be reached deliberately.
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

// intakeArmed wants a redrive policy on the intake feed pointing somewhere, with
// a receive count that actually lets a specimen fail rather than retrying it
// into oblivion.
func intakeArmed() (bool, error) {
	target, count, err := redrive()
	if err != nil {
		return false, err
	}
	if target == "" {
		return false, nil
	}
	// a maxReceiveCount of 1 gives a transient failure no second chance and an
	// unbounded one is not a dead letter queue at all.
	return count >= 2 && count <= 10, nil
}

// deviationReportFiled wants the write up, and wants it to quote the evidence:
// the dead letter queue's arn and the id of the key the archive is under. It is
// the one check in the challenge that reads free text, so it is deliberately
// forgiving about everything except the two identifiers.
func deviationReportFiled() (bool, error) {
	value, ok, err := parameter(deviationPath)
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

// --- the coda ----------------------------------------------------------------

// auditClosedOut gates the coda: the write up is filed and the archive is under
// the lab's own key. auditVisit keeps it from firing out of order for a player
// who built Act III's resources before Act III started.
func auditClosedOut() (bool, error) {
	if !auditVisit.Load() {
		return false, nil
	}
	if codaOpen.Load() {
		return true, nil
	}
	filed, err := deviationReportFiled()
	if err != nil || !filed {
		return false, err
	}
	return archiveEncrypted()
}

// theMorningRound is the last act. Nothing new to build: the only question left
// is whether it is all still true when the day shift arrives.
func theMorningRound(ctx context.Context, s *challenge.Scenario) error {
	if !codaOpen.CompareAndSwap(false, true) {
		return nil
	}

	s.AddDescription(
		"07:50. The inspector has gone, the report is filed, and the day shift is coming " +
			"in to a lab that is reporting. Nobody is going to thank you for the rest of the " +
			"hour. The only thing that matters now is that all of it is still true at " +
			"handover: points accrue while results keep coming through clean and the " +
			"pipeline keeps up with the instruments, and they stop the moment either stops " +
			"being the case.")

	s.AddCheck("Kept the chain intact", challenge.Check{
		Points:  heldPoints,
		Every:   30 * time.Second,
		Repeat:  true,
		Trigger: bounded(heldRounds, chainIntact()),
	})
	s.AddCheck("Kept up with the instruments", challenge.Check{
		Points:  codaPoints,
		Every:   30 * time.Second,
		Repeat:  true,
		Trigger: bounded(codaRounds, keepingUp()),
	})
	return nil
}

// keepingUp is the harder version of resultsLanding: not "is anything getting
// through" but "is the pipeline keeping up with what the instruments are
// sending". A pipeline that is up but drowning passes the first and fails this.
func keepingUp() func() (bool, error) {
	lastSent, lastClean := -1, -1
	return func() (bool, error) {
		state, ok, err := readQC()
		if err != nil || !ok {
			return false, err
		}
		if lastSent < 0 {
			lastSent, lastClean = state.sent, state.clean
			return false, nil
		}
		sent, clean := state.sent-lastSent, state.clean-lastClean
		lastSent, lastClean = state.sent, state.clean
		if sent <= 0 {
			return false, nil
		}
		return clean*1000 >= sent*keepingUpPerMille, nil
	}
}

// bounded caps how many times a Repeat check may pay out. Repeat checks never
// retire, so without this the maximum score does not exist — and for the
// negative one, neither does the maximum loss.
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

// --- reads -------------------------------------------------------------------

// Every read goes through one of these, and every one of them returns an error
// rather than a nil struct when the resource is not there. There is no recover
// around a trigger: one nil dereference takes the whole plugin down and the
// player's game is over.

func readArchive() (*s3.Bucket, error) {
	if archiveRef == "" {
		return nil, fmt.Errorf("specimen archive was never provisioned")
	}
	bucket, err := aws.Read[*s3.Bucket](archiveRef)
	if err != nil {
		return nil, err
	}
	if bucket == nil {
		return nil, fmt.Errorf("specimen archive is not readable")
	}
	return bucket, nil
}

func readIntake() (*sqs.Queue, error) {
	return readQueue(intakeRef, "intake feed")
}

func readAssayQueue() (*sqs.Queue, error) {
	return readQueue(assayRef, "assay queue")
}

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
		return nil, fmt.Errorf("clinical ledger was never provisioned")
	}
	table, err := aws.Read[*dynamodb.Table](ledgerRef)
	if err != nil {
		return nil, err
	}
	if table == nil {
		return nil, fmt.Errorf("clinical ledger is not readable")
	}
	return table, nil
}

func readLogGroup() (*logs.LogGroup, error) {
	if logGroupRef == "" {
		return nil, fmt.Errorf("instrument logs were never provisioned")
	}
	group, err := aws.Read[*logs.LogGroup](logGroupRef)
	if err != nil {
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("instrument logs are not readable")
	}
	return group, nil
}

// telemetry is one settled window of the overnight QC reconciliation.
type telemetry struct {
	window    int
	sent      int
	clean     int
	deviation int
	rate      int
}

// readQC parses the qc parameter.
//
// The false return is "not readable yet", not an error: on a cold account the
// feed has not run, and on any account there is a window between the parameter
// being created and being written for the first time. A trigger that reported
// that as a failure would report one every cycle until the first window closed.
func readQC() (telemetry, bool, error) {
	value, ok, err := parameter(qcPath)
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
		case "clean":
			state.clean = number
		case "deviation":
			state.deviation = number
		case "rate":
			state.rate = number
		}
	}
	return state, true, nil
}

// redrive returns the dead letter target and receive count off the intake feed,
// or an empty target when there is no redrive policy.
//
// Cloud Control hands the policy back as a nested object in some environments
// and as a json string in others, so both are accepted. maxReceiveCount has the
// same problem and is parsed out of a json.Number.
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
// It lists first and only reads when the identifier is actually there, which is
// doing two jobs. "Not there yet" is the normal state of a parameter the player
// has not written, and going straight to Read would log a 404 host side on every
// cycle until they do. And the value cannot be taken from the list itself: a
// Cloud Control list handler is only obliged to return primary identifiers, and
// whether the rest of the properties come with them is environment-dependent —
// fakecloud hands back the whole resource, real AWS is not required to.
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

// newKeys and newFunctions return the resources that were not in the account
// when the night started, read back individually.
//
// The read matters: List is only guaranteed to hand back identifiers, and these
// checks turn on a property rather than on existence. A resource that cannot be
// read is skipped rather than failing the check — it is usually one that is
// still being created.
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
