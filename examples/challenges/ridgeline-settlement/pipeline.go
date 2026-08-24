//go:build wasip1

package main

// The three programs that run *inside the player's account*.
//
// meterSource is the scenario's own: the substation feed and the settlement
// reconciliation in one function, running under a role the player cannot edit.
// It mints half-hourly meter readings, and one window later it goes and looks
// at what the player's pipeline did with them.
//
// validationSource and settlementSource are the two stages the outsourced team
// left behind. They ship to the player as a downloadable package.

import (
	"archive/zip"
	"bytes"
	"time"
)

// meterSource is the substation feed and the settlement reconciliation.
//
// Reading ids are deterministic — RL-<window>-<index> — which is what makes the
// replay in Act III expressible at all. The feed does not have to remember what
// it sent last night; it can regenerate the ids of any window it likes. Real
// meter data works the same way for the same reason: a reading is identified by
// meter and interval, not by when the message happened to be sent.
//
// Each run settles the window that just closed:
//
//  1. read the tally parameter (its only state)
//  2. scan the settled ledger for rows carrying the previous window's token and
//     count how many times each reading was settled
//  3. mint a fresh token, send this window's readings — plus, in replay mode,
//     the previous window's readings all over again
//  4. write the tally back
//
// settled_count is the whole measurement. A reading settled once is money moved
// correctly. A reading settled twice is money moved twice, which in this
// business is not a bug report, it is a phone call from a supplier's finance
// director.
const meterSource = `import json, os, random, secrets, time
import boto3

sqs = boto3.client("sqs")
ddb = boto3.client("dynamodb")
ssm = boto3.client("ssm")

INTAKE = os.environ["INTAKE_QUEUE_URL"]
LEDGER = os.environ["LEDGER_TABLE"]
TALLY = os.environ["TALLY_PARAM"]

BASE = int(os.environ.get("BASE_RATE", "12"))
PEAK = int(os.environ.get("PEAK_RATE", "40"))
RAMP = int(os.environ.get("RAMP_WINDOWS", "16"))

# Act III turns this on by rewriting the function's environment. When it is
# set, every run also re-sends the previous window's readings - the data logger
# at a substation coming back after a fault and flushing its backlog, which is
# an ordinary operational event and not an attack.
REPLAY = int(os.environ.get("REPLAY_WINDOWS", "0"))

SUBSTATIONS = ["RIDGELINE-N", "RIDGELINE-S", "CARBON-VALLEY", "ELKHORN",
               "TWIN-FORKS", "GRANITE-PASS"]
SUPPLIERS = ["Northwind Energy", "Cascade Retail", "Fourpeaks Power",
             "Meridian Supply", "Alpenglow Utilities"]


def read_tally():
    """Missing or unreadable is a cold start, which is not an error worth
    failing the run over."""
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


def settle(token):
    """Classify the rows carrying this token.

    Returns (exact, double). A row settled once is exact; a row whose
    settled_count went past one was settled more than once and is money paid
    twice. Paginated, because Scan returns a page count and not a table count.

    A row with no settled_count at all is counted as exact rather than as a
    problem: a player who rewrote the stage to do a plain conditional put and
    dropped the counter has built something correct, and the reconciliation
    should not punish them for not keeping our bookkeeping field.
    """
    if not token:
        return 0, 0
    exact, double, start = 0, 0, None
    while True:
        kwargs = dict(
            TableName=LEDGER,
            FilterExpression="#t = :t",
            ExpressionAttributeNames={"#t": "token"},
            ExpressionAttributeValues={":t": {"S": token}},
            ProjectionExpression="settled_count",
        )
        if start:
            kwargs["ExclusiveStartKey"] = start
        page = ddb.scan(**kwargs)
        for item in page.get("Items", []):
            raw = item.get("settled_count", {}).get("N")
            try:
                count = int(raw) if raw is not None else 1
            except (TypeError, ValueError):
                count = 1
            if count > 1:
                double += 1
            else:
                exact += 1
        start = page.get("LastEvaluatedKey")
        if not start:
            return exact, double


def reading(window, index, token):
    kwh = round(random.uniform(0.4, 780.0), 3)
    return json.dumps({
        # deterministic: this is the identity of the reading, not of the
        # message that happens to be carrying it.
        "reading_id": "RL-%06d-%04d" % (window, index),
        "token": token,
        "window": window,
        "substation": random.choice(SUBSTATIONS),
        "supplier": random.choice(SUPPLIERS),
        "kwh": kwh,
        "unit_price": 0.1412,
        "interval_end": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    })


def send(entries):
    for start in range(0, len(entries), 10):
        batch = entries[start:start + 10]
        for position, entry in enumerate(batch):
            entry["Id"] = str(position)
        sqs.send_message_batch(QueueUrl=INTAKE, Entries=batch)


def handler(event, context):
    state = read_tally()

    window = int(state.get("window", "0")) + 1
    sent = int(state.get("sent", "0"))
    exact = int(state.get("exact", "0"))
    double = int(state.get("double", "0"))
    replayed = int(state.get("replayed", "0"))

    exact_now, double_now = settle(state.get("token", ""))
    exact += exact_now
    double += double_now

    rate = BASE + int((PEAK - BASE) * min(window, RAMP) / float(RAMP))
    token = secrets.token_hex(8)

    entries = [{"MessageBody": reading(window, index, token)} for index in range(rate)]
    sent_now = len(entries)

    # the backlog flush. The same readings, by id, that were already settled
    # last window - carrying this window's token so that the reconciliation
    # sees what the pipeline did with them this time.
    replay_now = 0
    if REPLAY > 0 and window > 1:
        previous_rate = int(state.get("rate", str(BASE)))
        for index in range(previous_rate):
            entries.append({"MessageBody": reading(window - 1, index, token)})
            replay_now += 1

    send(entries)

    write_tally({
        "window": window,
        "sent": sent + sent_now,
        "exact": exact,
        "double": double,
        "replayed": replayed + replay_now,
        "rate": rate,
        "token": token,
        "updated": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    })
    return {"window": window, "sent": sent_now, "replayed": replay_now}
`

// validationSource is the first stage. It is correct.
//
// It drops readings that fail a sanity check rather than settling them, which
// is the right behaviour and also the reason the intake queue needs somewhere
// for a rejected reading to go — currently there is nowhere and they evaporate.
const validationSource = `import json, os
import boto3

sqs = boto3.client("sqs")
SETTLEMENT_QUEUE = os.environ["SETTLEMENT_QUEUE_URL"]

MAX_KWH = 5000.0


def handler(event, context):
    entries = []
    for record in event.get("Records", []):
        r = json.loads(record["body"])
        kwh = float(r.get("kwh", 0))
        if kwh < 0 or kwh > MAX_KWH:
            # implausible interval. Not settleable, and not ours to fix.
            raise ValueError("implausible reading %s: %s kWh" % (r.get("reading_id"), kwh))
        r["amount"] = round(kwh * float(r.get("unit_price", 0)), 4)
        entries.append({"MessageBody": json.dumps(r)})

    for start in range(0, len(entries), 10):
        batch = entries[start:start + 10]
        for position, entry in enumerate(batch):
            entry["Id"] = str(position)
        sqs.send_message_batch(QueueUrl=SETTLEMENT_QUEUE, Entries=batch)
    return {"validated": len(entries)}
`

// settlementSource is the second stage, and it is the one that is wrong.
//
// The bug is not that it crashes — it never crashes. It is that ADD is
// unconditional. SQS is at-least-once, so a redelivered message settles the
// same reading a second time and settled_count goes to two: money moved twice
// against one meter interval. At the trickle rate a healthy queue redelivers
// at, that is a handful of readings a night and nobody has ever noticed. When a
// substation flushes three days of backlog it is thousands.
//
// The fix is a judgement call rather than a line: making the write conditional
// is the easy half, and deciding what the stage should then do with a reading
// it has already settled is the half that decides whether the pipeline stalls.
const settlementSource = `import json, os, time
import boto3

ddb = boto3.client("dynamodb")
LEDGER = os.environ["LEDGER_TABLE"]


def handler(event, context):
    for record in event.get("Records", []):
        r = json.loads(record["body"])
        ddb.update_item(
            TableName=LEDGER,
            Key={"reading_id": {"S": str(r["reading_id"])}},
            UpdateExpression=(
                "ADD settled_count :one, settled_amount :amt "
                "SET #tok = :tok, substation = :sub, supplier = :sup, settled_at = :at"
            ),
            ExpressionAttributeNames={"#tok": "token"},
            ExpressionAttributeValues={
                ":one": {"N": "1"},
                ":amt": {"N": str(r.get("amount", 0))},
                ":tok": {"S": str(r["token"])},
                ":sub": {"S": str(r.get("substation", ""))},
                ":sup": {"S": str(r.get("supplier", ""))},
                ":at": {"S": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())},
            },
        )
    return {"settled": len(event.get("Records", []))}
`

// pipelineReadme travels in the zip. It describes what settlement means and
// what the operator is on the hook for. It does not describe how to deploy
// anything, and it does not mention idempotency — that is the inference the
// challenge is actually testing.
const pipelineReadme = `ridgeline settlement - pipeline stages
======================================

These are the two stages of the overnight settlement run. They came out of
the platform when the settlement team was outsourced and were never put back.

WHAT SETTLEMENT IS

A meter reading is one substation, one half-hour interval, one number. The
settlement run turns each reading into money owed between the generator that
put the energy on the network and the supplier that sold it. The settled
ledger is not a report. It is the instruction the clearing bank acts on.

The ledger is keyed by reading_id, and reading_id identifies the interval -
not the message, not the delivery, not the run. Two messages carrying the
same reading_id are the same reading. They are always the same reading, no
matter how far apart they arrive or what happened in between.

WHAT EACH STAGE DOES

  validation.py    reads from the meter intake, rejects readings that could
                   not physically have happened, prices the rest and passes
                   them to the settlement stage.

  settlement.py    reads a priced reading and settles it into the ledger.

Both are python3.12. Handlers are "validation.handler" and
"settlement.handler". Execution roles for both already exist in this account;
you will not be able to create your own.

OPERATIONAL NOTE, JAN

Substation data loggers buffer locally when they lose their backhaul and
flush the backlog when it comes back. Ops have asked us twice to confirm this
is safe. We have never got round to answering them.
`

// pipelinePackage builds the deployment zip in memory. A fixed modification
// time keeps the artifact byte-identical between runs.
func pipelinePackage() []byte {
	stamp := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	buffer := &bytes.Buffer{}
	archive := zip.NewWriter(buffer)
	for _, entry := range []struct{ name, content string }{
		{"validation.py", validationSource},
		{"settlement.py", settlementSource},
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
