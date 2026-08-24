//go:build wasip1

package main

// This file holds the two pieces of software that run *inside the player's
// account*, and the packaging for the one the player deploys themselves.
//
// The split matters. The plugin cannot move data: `pkg/challenge/aws` is
// Cloud Control only, so there is no SendMessage, no PutItem, no PutObject. A
// challenge that wants live traffic has to put a program in the account and
// read its tally back through the control plane. That program is
// generatorSource — the "load box" the platform team runs, owned by the
// scenario, running under an execution role the player cannot edit.
//
// workerSource is the other half: the consumer the contractor took with them.
// It ships to the player as a downloadable zip and it is their job to deploy
// it. It is deliberately naive — one poison message fails its whole batch —
// because that is the flaw the dead-letter queue exists to fix.

import (
	"archive/zip"
	"bytes"
	"time"
)

// generatorSource is the dispatch load box.
//
// Each run settles the previous window and opens a new one:
//
//  1. read the tally parameter (the only state it has — Lambda is stateless
//     and a scheduled function cannot be trusted to run on the same sandbox)
//  2. count the ledger rows carrying the token issued last window, and credit
//     exactly those to the cumulative delivered counter
//  3. mint a fresh token, send this window's jobs carrying it
//  4. write the tally back
//
// Settling on the *previous* token is what keeps the count honest. Each token
// is counted once and then never looked at again, so nothing is credited
// twice, and freight delivered after its window has closed does not count —
// which is the correct incentive for a pipeline that is supposed to keep up.
const generatorSource = `import json, os, random, secrets, time
import boto3

sqs = boto3.client("sqs")
ddb = boto3.client("dynamodb")
ssm = boto3.client("ssm")

QUEUE = os.environ["DISPATCH_QUEUE_URL"]
TABLE = os.environ["LEDGER_TABLE"]
TALLY = os.environ["TALLY_PARAM"]

BASE = int(os.environ.get("BASE_RATE", "12"))
PEAK = int(os.environ.get("PEAK_RATE", "45"))
RAMP = int(os.environ.get("RAMP_WINDOWS", "20"))
POISON_PERMILLE = int(os.environ.get("POISON_PERMILLE", "70"))

CITIES = ["Pueblo", "Trinidad", "Alamosa", "La Junta", "Walsenburg",
          "Salida", "Canon City", "Lamar", "Raton", "Durango"]
NAMES = ["R. Alvarez", "M. Whitcomb", "D. Okafor", "S. Nakamura", "T. Brandt",
         "J. Delgado", "K. Ferreira", "L. Mbeki", "P. Ostrowski", "H. Quinones"]


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


def count_token(token):
    """How many ledger rows carry this token. Paginated: Scan returns a page
    count, not a table count, and a short read here silently understates the
    player's throughput."""
    if not token:
        return 0
    total, start = 0, None
    while True:
        kwargs = dict(
            TableName=TABLE,
            Select="COUNT",
            FilterExpression="#t = :t",
            ExpressionAttributeNames={"#t": "token"},
            ExpressionAttributeValues={":t": {"S": token}},
        )
        if start:
            kwargs["ExclusiveStartKey"] = start
        page = ddb.scan(**kwargs)
        total += page.get("Count", 0)
        start = page.get("LastEvaluatedKey")
        if not start:
            return total


def job(token, window):
    return json.dumps({
        "shipment_id": "PF-%s" % secrets.token_hex(6),
        "token": token,
        "window": window,
        "consignee": random.choice(NAMES),
        "destination": random.choice(CITIES) + ", CO",
        "pallets": random.randint(1, 24),
    })


def legacy_job():
    """The jobs the old dispatcher still emits. Not JSON, and carrying the
    whole consignee record in the clear - which is the thing nobody has looked
    at because these have never landed anywhere readable."""
    return "LEGACY|%s|%d %s St|+1-719-555-%04d|signed:%s" % (
        random.choice(NAMES), random.randint(100, 9999),
        random.choice(CITIES), random.randint(0, 9999),
        secrets.token_hex(4),
    )


def handler(event, context):
    state = read_tally()

    window = int(state.get("window", "0")) + 1
    sent = int(state.get("sent", "0"))
    delivered = int(state.get("delivered", "0"))
    poison = int(state.get("poison", "0"))

    # settle the window that just closed, exactly once.
    delivered += count_token(state.get("token", ""))

    # peak season: volume climbs for the first RAMP windows and stays up.
    rate = BASE + int((PEAK - BASE) * min(window, RAMP) / float(RAMP))

    token = secrets.token_hex(8)
    batch, sent_now, poison_now = [], 0, 0
    for _ in range(rate):
        if random.randrange(1000) < POISON_PERMILLE:
            body, poison_now = legacy_job(), poison_now + 1
        else:
            body = job(token, window)
        batch.append({"Id": str(len(batch)), "MessageBody": body})
        sent_now += 1
        if len(batch) == 10:
            sqs.send_message_batch(QueueUrl=QUEUE, Entries=batch)
            batch = []
    if batch:
        sqs.send_message_batch(QueueUrl=QUEUE, Entries=batch)

    write_tally({
        "window": window,
        "sent": sent + sent_now,
        "delivered": delivered,
        "poison": poison + poison_now,
        "rate": rate,
        "token": token,
        "updated": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    })
    return {"window": window, "sent": sent_now, "delivered": delivered}
`

// workerSource is the consumer the contractor never handed over.
//
// It is correct except for one thing: json.loads on a legacy job raises, and
// the raise fails the whole batch. Without a dead-letter queue that batch comes
// back forever and takes the healthy jobs in it down every time. With one, the
// poison ages out after maxReceiveCount and throughput recovers. That is the
// entire argument for a dlq, made in the tally instead of in a lecture.
const workerSource = `import json, os, time
import boto3

ddb = boto3.client("dynamodb")
TABLE = os.environ["LEDGER_TABLE"]


def handler(event, context):
    records = event.get("Records", [])
    for record in records:
        job = json.loads(record["body"])
        ddb.put_item(TableName=TABLE, Item={
            "shipment_id": {"S": str(job["shipment_id"])},
            "token": {"S": str(job["token"])},
            "destination": {"S": str(job.get("destination", ""))},
            "pallets": {"N": str(int(job.get("pallets", 1)))},
            "delivered_at": {"S": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())},
        })
    return {"processed": len(records)}
`

// workerReadme travels in the zip. The player gets the deployment shape from
// the artifact itself rather than from a clue they have to buy.
const workerReadme = `roadrunner dispatch worker
==========================

This is the consumer half of the dispatch pipeline. Jobs arrive on the
dispatch queue; each one becomes a row in the shipment ledger. Nothing is
processing the queue right now.

To deploy it:

  1. upload this zip somewhere the Lambda service can read it - the manifest
     archive in this account will do.
  2. create a python3.12 function from it. Handler is "worker.handler".
     There is an execution role waiting in this account for it; you do not
     have permission to create your own, and you do not need to.
  3. set LEDGER_TABLE in the function environment to the shipment ledger's
     table name.
  4. point an event source mapping at the dispatch queue.

The worker writes the "token" field from each job into the ledger row. The
platform team's inventory counts freight by that token. If you drop it, the
work does not count as done.
`

// workerPackage builds the deployment zip in memory.
//
// A fixed modification time keeps the artifact byte-identical between runs, so
// a player who downloads it twice does not get two different files, and a
// checksum in a writeup stays true.
func workerPackage() []byte {
	stamp := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	buffer := &bytes.Buffer{}
	archive := zip.NewWriter(buffer)
	// a slice, not a map: map iteration order is randomised, and two runs that
	// emit the entries in a different order do not produce the same bytes.
	for _, entry := range []struct{ name, content string }{
		{"worker.py", workerSource},
		{"README", workerReadme},
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
