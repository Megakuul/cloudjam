//go:build wasip1

package main

// The programs that run outside the plugin, and the packaging for the one
// the player deploys.
//
// generatorSource is the gate network: three independent producers (VIP,
// General, Press), each writing to its own Kinesis stream, plus the
// reconciliation that later scans the ledger to see what actually landed.
// It runs under a role the player cannot edit.
//
// admissionsSource is the consumer the outgoing vendor took with them. It
// ships to the player as a downloadable package, and wiring one consumer to
// three independent streams is the first task.

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
)

// generatorSource mints scans and settles the window that just closed.
//
// Kinesis is at-least-once the same way SQS is, so the same discipline
// applies: settle on the *previous* token, count each token once, credit
// nothing that arrives after its window closed.
//
// The VIP stream is where the story lives. It ships with one shard — enough
// for a quiet Tuesday, not for a sellout — and every PutRecords call that
// exceeds what one shard can carry comes back with per-record throttle
// errors, counted here as vip_throttled. Partition keys are unique per scan
// on every stream, including VIP: the fix is capacity, not routing, and
// adding shards actually helps.
const generatorSource = `import json, os, random, secrets, time
import boto3

REGION = os.environ.get("AWS_REGION") or os.environ.get("AWS_DEFAULT_REGION") or "us-east-1"
ENDPOINT = os.environ.get("AWS_ENDPOINT_URL") or None
CREDS = {"aws_access_key_id": "test", "aws_secret_access_key": "test"} if ENDPOINT else {}
kin = boto3.client("kinesis", region_name=REGION, endpoint_url=ENDPOINT, **CREDS)
ddb = boto3.client("dynamodb", region_name=REGION, endpoint_url=ENDPOINT, **CREDS)
ssm = boto3.client("ssm", region_name=REGION, endpoint_url=ENDPOINT, **CREDS)

LEDGER = os.environ["LEDGER_TABLE"]
TALLY = os.environ["TALLY_PARAM"]
VIP_STREAM = os.environ["VIP_STREAM"]
GENERAL_STREAM = os.environ["GENERAL_STREAM"]
PRESS_STREAM = os.environ["PRESS_STREAM"]

BASE = int(os.environ.get("BASE_RATE", "15"))
PEAK = int(os.environ.get("PEAK_RATE", "55"))
RAMP = int(os.environ.get("RAMP_WINDOWS", "16"))
# Act III turns this on: the VIP gate alone quadruples, the way a sold-out
# headliner's fans arrive all at once rather than trickling in.
VIP_SURGE = int(os.environ.get("VIP_SURGE_WINDOWS", "0"))


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


def scan_id(prefix, window, index):
    return "%s-%06d-%04d" % (prefix, window, index)


def make_record(gate, sid, token, window):
    body = json.dumps({
        "scan_id": sid, "gate": gate, "token": token, "window": window,
        "seat": "%s-%02d" % (random.choice("ABCDEFGH"), random.randint(1, 40)),
        "ts": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }).encode()
    return {"Data": body, "PartitionKey": sid}


def send(stream, entries):
    """Kinesis takes at most 500 records a call. Per-record ErrorCode in the
    response is how a throttled shard shows up - there is no exception for
    "some of the batch didn't make it", only a per-record verdict."""
    ok = throttled = 0
    for start in range(0, len(entries), 500):
        batch = entries[start:start + 500]
        if not batch:
            continue
        resp = kin.put_records(StreamName=stream, Records=batch)
        for r in resp.get("Records", []):
            if "ErrorCode" in r:
                throttled += 1
            else:
                ok += 1
    return ok, throttled


def settle(token):
    """A settle_count over one is a duplicate admission - the same
    at-least-once redelivery every stream in this codebase eventually
    produces, arrived at here through Kinesis instead of SQS."""
    clean = dup = 0
    if not token:
        return clean, dup
    start = None
    while True:
        kwargs = dict(
            TableName=LEDGER,
            FilterExpression="#t = :t",
            ExpressionAttributeNames={"#t": "token"},
            ExpressionAttributeValues={":t": {"S": token}},
            ProjectionExpression="settle_count",
        )
        if start:
            kwargs["ExclusiveStartKey"] = start
        page = ddb.scan(**kwargs)
        for item in page.get("Items", []):
            try:
                count = int(item.get("settle_count", {}).get("N", "1"))
            except (TypeError, ValueError):
                count = 1
            if count > 1:
                dup += 1
            else:
                clean += 1
        start = page.get("LastEvaluatedKey")
        if not start:
            return clean, dup


def handler(event, context):
    state = read_tally()
    window = int(state.get("window", "0")) + 1
    sent = int(state.get("sent", "0"))
    verified = int(state.get("verified", "0"))
    duplicate_landed = int(state.get("duplicate_landed", "0"))
    vip_throttled = int(state.get("vip_throttled", "0"))

    clean, dup = settle(state.get("token", ""))
    verified += clean
    duplicate_landed += dup

    rate = BASE + int((PEAK - BASE) * min(window, RAMP) / float(RAMP))
    vip_rate = rate * 4 if VIP_SURGE > 0 else rate
    token = secrets.token_hex(8)

    vip_entries = [make_record("vip", scan_id("V", window, i), token, window) for i in range(vip_rate)]
    general_entries = [make_record("general", scan_id("G", window, i), token, window) for i in range(rate)]
    press_entries = [make_record("press", scan_id("P", window, i), token, window) for i in range(max(rate // 4, 3))]

    vip_ok, vip_thr = send(VIP_STREAM, vip_entries)
    gen_ok, _ = send(GENERAL_STREAM, general_entries)
    press_ok, _ = send(PRESS_STREAM, press_entries)

    sent_now = vip_ok + vip_thr + gen_ok + press_ok
    vip_throttled += vip_thr

    write_tally({
        "window": window, "sent": sent + sent_now, "verified": verified,
        "duplicate_landed": duplicate_landed, "vip_throttled": vip_throttled,
        "rate": rate, "token": token,
        "updated": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    })
    return {"window": window, "sent": sent_now, "verified": verified}
`

// admissionsSource is the consumer the vendor took with them. Correct except
// for one thing: the write is unconditional. Kinesis redelivers the same
// record after a network blip or a Lambda retry the same way any
// at-least-once source does, and ADD on settle_count counts every delivery
// rather than every scan.
const admissionsSource = `import base64, json, os, time
import boto3

REGION = os.environ.get("AWS_REGION") or os.environ.get("AWS_DEFAULT_REGION") or "us-east-1"
ENDPOINT = os.environ.get("AWS_ENDPOINT_URL") or None
CREDS = {"aws_access_key_id": "test", "aws_secret_access_key": "test"} if ENDPOINT else {}
ddb = boto3.client("dynamodb", region_name=REGION, endpoint_url=ENDPOINT, **CREDS)
LEDGER = os.environ["LEDGER_TABLE"]


def handler(event, context):
    admitted = 0
    for record in event.get("Records", []):
        scan = json.loads(base64.b64decode(record["kinesis"]["data"]))
        ddb.update_item(
            TableName=LEDGER,
            Key={"scan_id": {"S": str(scan["scan_id"])}},
            UpdateExpression=(
                "ADD settle_count :one "
                "SET #tok = :tok, gate = :gate, seat = :seat, admitted_at = :at"
            ),
            ExpressionAttributeNames={"#tok": "token"},
            ExpressionAttributeValues={
                ":one": {"N": "1"},
                ":tok": {"S": str(scan["token"])},
                ":gate": {"S": str(scan.get("gate", ""))},
                ":seat": {"S": str(scan.get("seat", ""))},
                ":at": {"S": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())},
            },
        )
        admitted += 1
    return {"admitted": admitted}
`

// pipelineReadme travels in the zip. It states the contract, not the fix.
const pipelineReadme = `fenwick arena - admissions consumer
=====================================

Turnstile Systems Inc. ran gate admissions here for eight seasons and shut
down without notice when their parent company sold the division. Three gate
clusters - VIP, General, Press - each stream ticket scans onto their own
Kinesis stream. Nothing is consuming any of them.

WHAT A SCAN MEANS

scan_id identifies one physical ticket scan. Two records with the same
scan_id are the same scan, however far apart they arrive - Kinesis, like
every at-least-once source, will occasionally hand you the same record more
than once.

DEPLOYMENT

admissions.py, runtime python3.12, handler "admissions.handler". The
execution role is already in this account, scoped to all three streams and
the ledger; you cannot create your own and do not need to. Wire one event
source mapping per stream - three total, one consumer. Starting position
LATEST is fine; the gates do not stop running for you to catch up, and
neither does tonight's show.
`

func pipelinePackage() []byte {
	stamp := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	buffer := &bytes.Buffer{}
	archive := zip.NewWriter(buffer)
	for _, entry := range []struct{ name, content string }{
		{"admissions.py", admissionsSource},
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

// lambdaCodeZip and uploadLambdaCode: see meridian-farebox for the full
// rationale. Short version — inline Code.ZipFile is meant to carry plain
// source text that AWS Lambda wraps into a deployable zip server-side;
// fakecloud does not do that wrapping, so the plugin's own scenario-owned
// function builds a real zip and puts it through the asset channel (the only
// route a plugin has to S3 at all) instead.
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
