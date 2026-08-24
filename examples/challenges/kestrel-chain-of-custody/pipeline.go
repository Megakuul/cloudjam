//go:build wasip1

package main

// This file holds the three programs that run *inside the player's account*.
//
// The plugin cannot move data — pkg/challenge/aws is Cloud Control only, so
// there is no SendMessage, no PutItem, no PutObject. A challenge scored on data
// that actually flowed has to put a program in the account and read its tally
// back through the control plane.
//
// sequencerSource is that program: the instrument feed, owned by the scenario,
// running under an execution role the player cannot edit. It is both the
// producer and the auditor — it mints the specimens and it is the thing that
// decides, one window later, which of them came out the far end intact.
//
// accessionSource and assaySource are the two halves of the chain the postdoc
// took with them. They ship to the player as a downloadable package. The player
// deploys them; nothing here does it for them.

import (
	"archive/zip"
	"bytes"
	"time"
)

// sequencerSource is the instrument feed and the QC auditor in one function.
//
// Each run settles the window that just closed and opens a new one:
//
//  1. read the tally parameter (its only state — Lambda is stateless and a
//     scheduled function cannot be trusted to land on the same sandbox twice)
//  2. scan the ledger for rows carrying the *previous* window's token and
//     classify each one: a complete custody chain is clean, anything else is a
//     deviation
//  3. mint a fresh token and drop this window's specimens on the intake queue
//  4. write the tally back
//
// Settling on the previous token is what keeps the count honest. Each token is
// classified once and then never looked at again, so nothing is credited twice,
// and a result that arrives after its window closed does not count — which is
// the correct incentive for a pipeline that is supposed to keep up with the
// instruments.
//
// The classification is the whole point of the challenge and it deliberately
// lives here rather than in the plugin. Cloud Control cannot read a DynamoDB
// item, so "did this row carry a complete chain of custody" is a question only
// something inside the account can answer.
const sequencerSource = `import json, os, random, secrets, time
import boto3

sqs = boto3.client("sqs")
ddb = boto3.client("dynamodb")
ssm = boto3.client("ssm")

INTAKE = os.environ["INTAKE_QUEUE_URL"]
LEDGER = os.environ["LEDGER_TABLE"]
TALLY = os.environ["TALLY_PARAM"]

BASE = int(os.environ.get("BASE_RATE", "10"))
PEAK = int(os.environ.get("PEAK_RATE", "38"))
RAMP = int(os.environ.get("RAMP_WINDOWS", "18"))

SITES = ["KB-DENVER", "KB-BOULDER", "KB-FTCOLLINS"]
ASSAYS = ["WGS-30X", "WES-100X", "PANEL-TSO500", "RNA-SEQ", "METHYL-EPIC"]

# The order the chain has to be in. A record that carries both stamps but
# picked them up in the wrong order did not go through the pipeline as
# designed, and under the lab's accreditation that is the same deviation as
# a missing stamp.
REQUIRED = ["accession", "assay"]


def read_tally():
    """Parse the space separated key=value tally. Missing or unreadable is a
    cold start, which is not an error worth failing the run over."""
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
        Name=TALLY,
        Type="String",
        Overwrite=True,
        Value=" ".join("%s=%s" % (k, v) for k, v in state.items()),
    )


def chain_of(item):
    """Pull the custody chain off a ledger row.

    The row is written by the player's own code, so nothing about its shape is
    guaranteed. Every branch that could raise on a malformed row returns an
    empty chain instead: a row we cannot parse is a row with no chain, which is
    a deviation, and that is the honest reading rather than a crash that would
    stop the instruments.
    """
    raw = item.get("custody", {})
    if "S" in raw:
        try:
            parsed = json.loads(raw["S"])
        except Exception:
            return []
    elif "L" in raw:
        parsed = []
        for element in raw["L"]:
            if "S" in element:
                parsed.append(element["S"])
            elif "M" in element and "stage" in element["M"]:
                parsed.append(element["M"]["stage"].get("S", ""))
    else:
        return []

    chain = []
    for step in parsed if isinstance(parsed, list) else []:
        if isinstance(step, str):
            chain.append(step)
        elif isinstance(step, dict):
            chain.append(str(step.get("stage", "")))
    return chain


def settle(token):
    """Classify every ledger row carrying this token.

    Returns (clean, deviation). Paginated, because Scan returns a page count
    and not a table count — a short read here silently understates the
    player's throughput and would make a working pipeline look broken.
    """
    if not token:
        return 0, 0
    clean, deviation, start = 0, 0, None
    while True:
        kwargs = dict(
            TableName=LEDGER,
            FilterExpression="#t = :t",
            ExpressionAttributeNames={"#t": "token"},
            ExpressionAttributeValues={":t": {"S": token}},
            ProjectionExpression="custody",
        )
        if start:
            kwargs["ExclusiveStartKey"] = start
        page = ddb.scan(**kwargs)
        for item in page.get("Items", []):
            if chain_of(item) == REQUIRED:
                clean += 1
            else:
                deviation += 1
        start = page.get("LastEvaluatedKey")
        if not start:
            return clean, deviation


def specimen(token, window):
    return json.dumps({
        "specimen_id": "KB-%s" % secrets.token_hex(6),
        "token": token,
        "window": window,
        "site": random.choice(SITES),
        "assay": random.choice(ASSAYS),
        "reads": random.randint(2_000_000, 90_000_000),
        "collected_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        # the chain starts empty. Every stage that handles this specimen is
        # supposed to append itself before passing it on.
        "custody": [],
    })


def handler(event, context):
    state = read_tally()

    window = int(state.get("window", "0")) + 1
    sent = int(state.get("sent", "0"))
    clean = int(state.get("clean", "0"))
    deviation = int(state.get("deviation", "0"))

    # settle the window that just closed, exactly once.
    clean_now, deviation_now = settle(state.get("token", ""))
    clean += clean_now
    deviation += deviation_now

    # the overnight run: volume climbs for the first RAMP windows and stays up.
    rate = BASE + int((PEAK - BASE) * min(window, RAMP) / float(RAMP))

    token = secrets.token_hex(8)
    batch, sent_now = [], 0
    for _ in range(rate):
        batch.append({"Id": str(len(batch)), "MessageBody": specimen(token, window)})
        sent_now += 1
        if len(batch) == 10:
            sqs.send_message_batch(QueueUrl=INTAKE, Entries=batch)
            batch = []
    if batch:
        sqs.send_message_batch(QueueUrl=INTAKE, Entries=batch)

    write_tally({
        "window": window,
        "sent": sent + sent_now,
        "clean": clean,
        "deviation": deviation,
        "rate": rate,
        "token": token,
        "updated": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    })
    return {"window": window, "sent": sent_now, "clean": clean, "deviation": deviation}
`

// accessionSource is the first stage: specimens arrive from the instruments,
// get an accession number, and move on to the assay queue.
//
// It is correct, and it is the easy half. The interesting thing about it is
// what it does *not* do — it does not write to the ledger, so a player who
// deploys only this one has a pipeline that drops everything on the floor and
// a tally that stays at zero.
const accessionSource = `import json, os, secrets, time
import boto3

sqs = boto3.client("sqs")
ASSAY_QUEUE = os.environ["ASSAY_QUEUE_URL"]


def handler(event, context):
    entries = []
    for record in event.get("Records", []):
        specimen = json.loads(record["body"])
        chain = specimen.get("custody", [])
        chain.append({
            "stage": "accession",
            "at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
            "accession_no": "A%s" % secrets.token_hex(4),
        })
        specimen["custody"] = chain
        entries.append({"Id": str(len(entries)), "MessageBody": json.dumps(specimen)})
        if len(entries) == 10:
            sqs.send_message_batch(QueueUrl=ASSAY_QUEUE, Entries=entries)
            entries = []
    if entries:
        sqs.send_message_batch(QueueUrl=ASSAY_QUEUE, Entries=entries)
    return {"accessioned": len(event.get("Records", []))}
`

// assaySource is the second stage, and it is the one that is subtly wrong.
//
// It writes the ledger row correctly in every respect except that it replaces
// the custody chain with its own stamp instead of appending to it. Every row it
// writes therefore carries "assay" and nothing before it — a result in the
// clinical ledger that cannot be traced back to the specimen it came from.
//
// That is a deviation, not a failure. Nothing errors, the queue drains, the
// ledger fills up, and the QC dashboard shows a lab that looks like it is
// working. Only the deviation counter says otherwise, which is exactly the
// thing a good technical manager is supposed to notice before the accreditation
// body does.
const assaySource = `import json, os, time
import boto3

ddb = boto3.client("dynamodb")
LEDGER = os.environ["LEDGER_TABLE"]


def handler(event, context):
    written = 0
    for record in event.get("Records", []):
        specimen = json.loads(record["body"])
        chain = [{
            "stage": "assay",
            "at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
        }]
        ddb.put_item(TableName=LEDGER, Item={
            "specimen_id": {"S": str(specimen["specimen_id"])},
            "token": {"S": str(specimen["token"])},
            "site": {"S": str(specimen.get("site", ""))},
            "assay": {"S": str(specimen.get("assay", ""))},
            "custody": {"S": json.dumps(chain)},
            "reported_at": {"S": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())},
        })
        written += 1
    return {"reported": written}
`

// pipelineReadme travels in the zip.
//
// It states the *contract* — what a stage owes the record it passes on — and
// says nothing about how to deploy anything. That is deliberate and it is the
// difference between this and a warmup: the player is given the obligations the
// lab is under and has to work out the shape of the fix themselves. There are
// two functions, two event source mappings, two execution roles already sitting
// in the account and a set of environment variables to work out, and none of
// that is written down anywhere.
const pipelineReadme = `kestrel LIMS integration - pipeline stages
=========================================

These are the two stages that sit between the instrument feed and the
clinical ledger. They were pulled out of the deployment when the integration
was rewritten and never went back.

THE CUSTODY CONTRACT

Every specimen carries a "custody" field: an ordered list of the stages that
have handled it. The contract is one line long and the whole accreditation
rests on it:

    a stage APPENDS itself to the chain. It never replaces it.

A result that reaches the ledger having lost part of its chain is not a
missing result. It is a clinical result that cannot be traced to a specimen,
which is a reportable deviation, and the lab is required to disclose it. The
QC dashboard counts those separately from the ones that never arrived, and it
is not the counter you want moving.

WHAT EACH STAGE OWES

  accession.py  reads a specimen from the intake feed, appends its stamp,
                and passes it to the assay stage.

  assay.py      reads an accessioned specimen, appends its stamp, and writes
                the reportable row to the clinical ledger. The ledger row has
                to carry the instrument token it arrived with - the overnight
                QC reconciliation counts by that token, and a row without one
                did not happen as far as the lab is concerned.

Both stages are python3.12. Handlers are "accession.handler" and
"assay.handler". Neither of them creates its own credentials; the roles they
need are already in this account and you will not be able to make new ones.

These files came off the postdoc's laptop. They ran in production in this
state. That is not the same as saying they are correct.
`

// pipelinePackage builds the deployment zip in memory.
//
// A fixed modification time keeps the artifact byte-identical between runs, so
// a player who downloads it twice does not get two different files.
func pipelinePackage() []byte {
	stamp := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	buffer := &bytes.Buffer{}
	archive := zip.NewWriter(buffer)
	// a slice, not a map: map iteration order is randomised, and two runs that
	// emit the entries in a different order do not produce the same bytes.
	for _, entry := range []struct{ name, content string }{
		{"accession.py", accessionSource},
		{"assay.py", assaySource},
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
