//go:build wasip1

package main

// The programs outside the plugin, and the packaging for the one the player
// deploys.
//
// This pipeline has no downstream queue or table at all — the durable
// record is the Step Functions execution itself. generatorSource starts one
// execution per claim and later reads back what happened to it directly
// through the state machine's own execution history: SUCCEEDED with the
// adjudicator's own output, SUCCEEDED via the manual-review path, or FAILED
// outright because nothing caught the error. adjudicatorSource is the one
// function the player deploys, and it is correct — the thing that is broken
// is the state machine definition, not the code running inside it.

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
)

// generatorSource submits claims and settles the window before it.
//
// Kept "pending", a list of execution arns start_execution handed back, is
// the whole story: no token-and-rescan needed, because Step Functions already
// keeps the record this pipeline is scored against. A risky claim carries an
// amount outside what the adjudicator will accept — real, not forged, the
// same distinction meridian-farebox draws between a forged signature and a
// misread. Whether it becomes a manual review or an uncaught failure is
// entirely the state machine's problem now.
const generatorSource = `import json, os, random, secrets, time
import boto3

REGION = os.environ.get("AWS_REGION") or os.environ.get("AWS_DEFAULT_REGION") or "us-east-1"
ENDPOINT = os.environ.get("AWS_ENDPOINT_URL") or None
CREDS = {"aws_access_key_id": "test", "aws_secret_access_key": "test"} if ENDPOINT else {}
sfn = boto3.client("stepfunctions", region_name=REGION, endpoint_url=ENDPOINT, **CREDS)
ssm = boto3.client("ssm", region_name=REGION, endpoint_url=ENDPOINT, **CREDS)

STATE_MACHINE_ARN = os.environ["STATE_MACHINE_ARN"]
TALLY = os.environ["TALLY_PARAM"]

BASE = int(os.environ.get("BASE_RATE", "12"))
PEAK = int(os.environ.get("PEAK_RATE", "45"))
RAMP = int(os.environ.get("RAMP_WINDOWS", "16"))
RISKY_PERMILLE = int(os.environ.get("RISKY_PERMILLE", "120"))
# Act III turns this on: a filing deadline pushes a backlog through all at
# once rather than trickling in over the day.
FILING_SURGE = int(os.environ.get("FILING_SURGE_WINDOWS", "0"))


def read_tally():
    try:
        raw = ssm.get_parameter(Name=TALLY)["Parameter"]["Value"]
    except Exception:
        return {}
    state = {}
    for part in raw.split():
        key, _, value = part.partition("=")
        if key:
            state[key] = value
    return state


def write_tally(state):
    ssm.put_parameter(
        Name=TALLY, Type="String", Overwrite=True,
        Value=" ".join("%s=%s" % (k, v) for k, v in state.items()),
    )


def settle(pending):
    """Reads each execution this generator started back through Step
    Functions' own history, not through a rescan of some other resource -
    the state machine is the ledger here."""
    settled = reviewed = failed = 0
    still_pending = []
    for execution_arn in pending:
        try:
            resp = sfn.describe_execution(executionArn=execution_arn)
        except Exception:
            continue
        status = resp.get("status")
        if status == "RUNNING":
            still_pending.append(execution_arn)
            continue
        if status == "SUCCEEDED":
            output = resp.get("output") or ""
            if "manual_review" in output:
                reviewed += 1
            else:
                settled += 1
        elif status == "FAILED":
            failed += 1
        # ABORTED/TIMED_OUT: neither settled nor failed-uncaught: not this
        # scenario's failure mode, and not worth a category of its own.
    return settled, reviewed, failed, still_pending


def claim(prefix, window, index, token):
    amount = random.randint(-500, 0) if prefix == "R" else random.randint(200, 45000)
    return {
        "claim_id": "%s-%s-%06d-%04d" % (prefix, token, window, index),
        "claim_amount": amount,
        "window": window,
    }


def handler(event, context):
    state = read_tally()
    window = int(state.get("window", "0")) + 1
    sent = int(state.get("sent", "0"))
    settled = int(state.get("settled", "0"))
    reviewed = int(state.get("reviewed", "0"))
    failed_uncaught = int(state.get("failed_uncaught", "0"))

    pending = json.loads(state.get("pending", "[]"))
    s_now, r_now, f_now, still_pending = settle(pending)
    settled += s_now
    reviewed += r_now
    failed_uncaught += f_now

    rate = BASE + int((PEAK - BASE) * min(window, RAMP) / float(RAMP))
    if FILING_SURGE > 0:
        rate *= 3
    token = secrets.token_hex(6)

    started = []
    sent_now = 0
    for i in range(rate):
        risky = random.randrange(1000) < RISKY_PERMILLE
        c = claim("R" if risky else "T", window, i, token)
        try:
            resp = sfn.start_execution(
                stateMachineArn=STATE_MACHINE_ARN,
                name=c["claim_id"],
                input=json.dumps(c),
            )
            started.append(resp["executionArn"])
            sent_now += 1
        except Exception:
            pass

    write_tally({
        "window": window, "sent": sent + sent_now, "settled": settled,
        "reviewed": reviewed, "failed_uncaught": failed_uncaught,
        "pending": json.dumps(still_pending + started),
        "rate": rate, "updated": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    })
    return {"window": window, "sent": sent_now, "settled": settled}
`

// adjudicatorSource is the one function the player deploys, and it is
// correct: it validates the claim amount and rejects what a real claim
// cannot be. The bug in this pipeline is not here.
const adjudicatorSource = `import json

MIN_AMOUNT = 50
MAX_AMOUNT = 250000


def handler(event, context):
    amount = int(event.get("claim_amount", 0))
    if amount < MIN_AMOUNT or amount > MAX_AMOUNT:
        raise ValueError("implausible claim amount: %d" % amount)
    payout = round(amount * 0.8)
    return {"status": "settled", "claim_id": event.get("claim_id"), "payout": payout}
`

// pipelineReadme travels in the zip.
const pipelineReadme = `northbridge claims triage - adjudicator
==========================================

The adjudicator is the only Lambda in this pipeline, and it is not the
problem. It validates a claim's amount and rejects what a real claim cannot
be - too small to bother filing, too large for anything this desk handles
without a human. That is correct behavior, not a bug.

Runtime python3.12, handler "adjudicator.handler". The execution role is
already provisioned in this account; you cannot create your own.

WHAT IS ACTUALLY WRONG

The state machine that calls this function has one state and no error
handling. When the adjudicator rejects a claim - correctly - the whole
execution fails, and a rejection that should have gone to a human for review
instead simply never happened. Nothing downstream of this function will tell
you that. Only the state machine's own execution history will.

Fixing this is not a code change. It is a definition change: the Task state
needs a Catch, and the state it catches to needs to exist. Something that
returns a result containing manual_review is enough for this desk to count
it as reviewed rather than lost - the exact shape is yours to write.
`

func pipelinePackage() []byte {
	stamp := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	buffer := &bytes.Buffer{}
	archive := zip.NewWriter(buffer)
	for _, entry := range []struct{ name, content string }{
		{"adjudicator.py", adjudicatorSource},
		{"README", pipelineReadme},
	} {
		writer, err := archive.CreateHeader(&zip.FileHeader{
			Name:     entry.name,
			Method:   zip.Deflate,
			Modified: stamp,
		})
		if err != nil {
			return nil
		}
		if _, err := writer.Write([]byte(entry.content)); err != nil {
			return nil
		}
	}
	if err := archive.Close(); err != nil {
		return nil
	}
	return buffer.Bytes()
}

// brokenDefinition is the state machine bootstrap actually creates: one
// task, no error handling, pointed at a function that was never deployed.
// The plugin is never told the account id, so the placeholder is exactly
// that — a name nothing will ever resolve, on-story with "whoever set this
// up meant to come back and wire it properly, and never did."
const brokenDefinition = `{
  "Comment": "northbridge claims triage - do not disable",
  "StartAt": "Adjudicate",
  "States": {
    "Adjudicate": {
      "Type": "Task",
      "Resource": "arn:aws:lambda:REGION:ACCOUNT:function:REPLACE-ME",
      "End": true
    }
  }
}`

func lambdaCodeZip(source string) []byte {
	stamp := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	buffer := &bytes.Buffer{}
	archive := zip.NewWriter(buffer)
	writer, err := archive.CreateHeader(&zip.FileHeader{
		Name:     "index.py",
		Method:   zip.Deflate,
		Modified: stamp,
	})
	if err != nil {
		return nil
	}
	if _, err := writer.Write([]byte(source)); err != nil {
		return nil
	}
	if err := archive.Close(); err != nil {
		return nil
	}
	return buffer.Bytes()
}

func uploadLambdaCode(source string) (bucket, key string, err error) {
	zipped := lambdaCodeZip(source)
	if len(zipped) == 0 {
		return "", "", fmt.Errorf("build code archive: empty")
	}
	out, err := api.CreateAsset(zipped)
	if err != nil {
		return "", "", err
	}
	if out.URL == "" {
		return "", "", fmt.Errorf("create asset: empty url")
	}
	bucket, key, ok := strings.Cut(strings.TrimPrefix(out.URL, "s3://"), "/")
	if !ok {
		return "", "", fmt.Errorf("unexpected asset url %q", out.URL)
	}
	return bucket, key, nil
}
