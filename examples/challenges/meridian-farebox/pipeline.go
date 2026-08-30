//go:build wasip1

package main

// This file holds every program that runs outside the plugin itself, and the
// packaging for the ones the player deploys.
//
// The plugin cannot move data or speak HTTP — pkg/challenge/aws is Cloud
// Control only, so there is no SendMessage, no PutItem, and no way to send a
// request anywhere. A challenge scored on a real request pipeline has to put
// programs in the account (and in front of it) and read their tally back
// through the control plane.
//
// ingestSource and generatorSource are scenario-owned: the generator is the
// gate network, minting taps and grading what came back; ingest is the front
// door those taps enter AWS through once the player's edge has cleared them.
// Neither is the player's to touch.
//
// edgeSource and settlementSource are what the concessionaire took with them.
// They ship as a downloadable package. edgeSource is Go — a real program the
// player builds into a real binary and runs *somewhere of their own choosing*,
// which is the whole point: the plugin never names a compute service for it,
// because nothing downstream cares where it runs, only that it answers.

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
)

// ingestSource is the front door: a Lambda behind a public Function URL, with
// no authentication of its own. It exists so that whatever the player deploys
// their edge on never needs an AWS credential — it hands taps to AWS over
// plain HTTPS, the same way it received them.
//
// It performs no validation beyond "is this JSON with the two fields the
// pipeline is keyed on." Everything else — whether the tap should have been
// trusted at all — was the edge's decision, made before this was ever called.
const ingestSource = `import base64, json, os
import boto3

# REGION/ENDPOINT resolve the way every AWS SDK client should when it might be
# running against a local emulator instead of AWS: nothing here does anything
# on real Lambda, where AWS_REGION always exists and AWS_ENDPOINT_URL never
# does. ENDPOINT is only ever set by this scenario's own bootstrap, and only
# when it has already confirmed it is talking to fakecloud - see
# localEndpointOverride in main.go.
REGION = os.environ.get("AWS_REGION") or os.environ.get("AWS_DEFAULT_REGION") or "us-east-1"
ENDPOINT = os.environ.get("AWS_ENDPOINT_URL") or None
CREDS = {"aws_access_key_id": "test", "aws_secret_access_key": "test"} if ENDPOINT else {}
sqs = boto3.client("sqs", region_name=REGION, endpoint_url=ENDPOINT, **CREDS)
INTAKE = os.environ["INTAKE_QUEUE_URL"]


def handler(event, context):
    body = event.get("body", "")
    if event.get("isBase64Encoded"):
        body = base64.b64decode(body).decode("utf-8", "replace")
    try:
        tap = json.loads(body)
    except Exception:
        return {"statusCode": 400, "body": "malformed"}
    if not tap.get("tap_id") or not tap.get("token"):
        return {"statusCode": 400, "body": "missing fields"}
    sqs.send_message(QueueUrl=INTAKE, MessageBody=json.dumps(tap))
    return {"statusCode": 200, "body": "ok"}
`

// generatorSource is the gate network and the settlement auditor in one
// function, running under a role the player cannot edit. Each run settles the
// window that just closed and opens a new one:
//
//  1. read the tally (its only state — Lambda is stateless)
//  2. scan the farebox ledger for rows carrying the *previous* window's
//     token and classify each by what its tap_id and settle count say
//  3. mint a fresh token, sign a new batch of taps, and POST every one of
//     them at whatever endpoint the player has published
//  4. write the tally back
//
// Three tap shapes go out every window, distinguishable only by prefix:
//
//	T-  a legitimate tap, correctly signed. Should settle exactly once.
//	X-  a forged tap — a garbage signature standing in for a device that was
//	    never a real Meridian gate. A correct edge rejects it before it ever
//	    reaches AWS.
//	M-  a legitimate gate having a bad day: correctly signed, and carrying a
//	    fare no card reader could actually produce. Authentication is not the
//	    question here — this one is real, it is just wrong, and the answer
//	    lives downstream of the edge.
//
// Settling on the *previous* token, and counting each token exactly once, is
// what keeps the tally honest — the same discipline every generator in this
// codebase uses and for the same reason: nothing is credited twice, and
// nothing that arrives after its window closed counts at all.
const generatorSource = `import hashlib, hmac, json, os, random, secrets, time
import urllib.request, urllib.error
import boto3

# See ingest.py's comment on REGION/ENDPOINT - same reasoning, same guarantee
# that ENDPOINT is never set outside a confirmed fakecloud run.
REGION = os.environ.get("AWS_REGION") or os.environ.get("AWS_DEFAULT_REGION") or "us-east-1"
ENDPOINT = os.environ.get("AWS_ENDPOINT_URL") or None
CREDS = {"aws_access_key_id": "test", "aws_secret_access_key": "test"} if ENDPOINT else {}
ssm = boto3.client("ssm", region_name=REGION, endpoint_url=ENDPOINT, **CREDS)
ddb = boto3.client("dynamodb", region_name=REGION, endpoint_url=ENDPOINT, **CREDS)

LEDGER = os.environ["LEDGER_TABLE"]
TALLY = os.environ["TALLY_PARAM"]
SECRET_PARAM = os.environ["SECRET_PARAM"]
ENDPOINT_PARAM = os.environ["ENDPOINT_PARAM"]

BASE = int(os.environ.get("BASE_RATE", "20"))
PEAK = int(os.environ.get("PEAK_RATE", "70"))
RAMP = int(os.environ.get("RAMP_WINDOWS", "16"))
FORGED_PERMILLE = int(os.environ.get("FORGED_PERMILLE", "60"))
MISREAD_PERMILLE = int(os.environ.get("MISREAD_PERMILLE", "40"))

# Act III turns this on by rewriting the function's environment. When it is
# set, every run also re-sends last window's legitimate taps under a fresh
# token - the concessionaire's badge, which nobody revoked, still opening
# gates.
REPLAY = int(os.environ.get("REPLAY_WINDOWS", "0"))

STATIONS = ["MERIDIAN-CENTRAL", "HARBOR-JCT", "OLD-MILL", "UNIVERSITY",
            "FOUNDRY-SQ", "NORTHGATE", "RIVERBEND"]


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


def read_param(name):
    try:
        return ssm.get_parameter(Name=name)["Parameter"]["Value"]
    except Exception:
        return ""


def canonical(tap):
    return "|".join([
        tap["tap_id"], tap["token"], str(tap["window"]), tap["station"],
        tap["rider_hash"], str(tap["fare_cents"]), tap["ts"],
    ])


def sign(secret, message):
    return hmac.new(secret.encode(), message.encode(), hashlib.sha256).hexdigest()


def make_tap(prefix, index, window, token, secret, forge=False):
    tap = {
        "tap_id": "%s-%06d-%04d" % (prefix, window, index),
        "token": token,
        "window": window,
        "station": random.choice(STATIONS),
        "rider_hash": secrets.token_hex(8),
        "fare_cents": random.choice([-50, 0, 9900]) if prefix == "M" else random.randint(200, 450),
        "ts": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    tap["sig"] = secrets.token_hex(32) if forge else sign(secret, canonical(tap))
    return tap


def post_tap(endpoint, tap):
    data = json.dumps(tap).encode()
    req = urllib.request.Request(
        endpoint, data=data, headers={"Content-Type": "application/json"}, method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=5) as resp:
            return resp.status
    except urllib.error.HTTPError as e:
        return e.code
    except Exception:
        return 0


def settle(token):
    """Classify every ledger row carrying this token. Paginated: Scan returns
    a page count, not a table count, and a short read here silently
    understates the player's throughput."""
    verified = forged = misread = replay = 0
    if not token:
        return verified, forged, misread, replay
    start = None
    while True:
        kwargs = dict(
            TableName=LEDGER,
            FilterExpression="#t = :t",
            ExpressionAttributeNames={"#t": "token"},
            ExpressionAttributeValues={":t": {"S": token}},
            ProjectionExpression="tap_id, settle_count",
        )
        if start:
            kwargs["ExclusiveStartKey"] = start
        page = ddb.scan(**kwargs)
        for item in page.get("Items", []):
            tap_id = item.get("tap_id", {}).get("S", "")
            try:
                count = int(item.get("settle_count", {}).get("N", "1"))
            except (TypeError, ValueError):
                count = 1
            if tap_id.startswith("X-"):
                forged += 1
            elif tap_id.startswith("M-"):
                misread += 1
            elif count > 1:
                replay += 1
            else:
                verified += 1
        start = page.get("LastEvaluatedKey")
        if not start:
            return verified, forged, misread, replay


def handler(event, context):
    state = read_tally()

    window = int(state.get("window", "0")) + 1
    sent = int(state.get("sent", "0"))
    verified = int(state.get("verified", "0"))
    forged_landed = int(state.get("forged_landed", "0"))
    misread_landed = int(state.get("misread_landed", "0"))
    replay_landed = int(state.get("replay_landed", "0"))
    http_ok = int(state.get("http_ok", "0"))
    http_fail = int(state.get("http_fail", "0"))
    replayed = int(state.get("replayed", "0"))

    v, f, m, r = settle(state.get("token", ""))
    verified += v
    forged_landed += f
    misread_landed += m
    replay_landed += r

    endpoint = read_param(ENDPOINT_PARAM)
    secret = read_param(SECRET_PARAM)

    rate = BASE + int((PEAK - BASE) * min(window, RAMP) / float(RAMP))
    token = secrets.token_hex(8)

    taps = []
    if endpoint and secret:
        for index in range(rate):
            roll = random.randrange(1000)
            if roll < FORGED_PERMILLE:
                taps.append(make_tap("X", index, window, token, secret, forge=True))
            elif roll < FORGED_PERMILLE + MISREAD_PERMILLE:
                taps.append(make_tap("M", index, window, token, secret))
            else:
                taps.append(make_tap("T", index, window, token, secret))

        replay_now = 0
        if REPLAY > 0 and window > 1:
            previous_rate = int(state.get("rate", str(BASE)))
            for index in range(previous_rate):
                taps.append(make_tap("T", index, window - 1, token, secret))
                replay_now += 1
        replayed += replay_now

    sent_now, ok_now, fail_now = 0, 0, 0
    for tap in taps:
        status = post_tap(endpoint, tap)
        sent_now += 1
        if 200 <= status < 300:
            ok_now += 1
        else:
            fail_now += 1

    write_tally({
        "window": window,
        "sent": sent + sent_now,
        "verified": verified,
        "forged_landed": forged_landed,
        "misread_landed": misread_landed,
        "replay_landed": replay_landed,
        "http_ok": http_ok + ok_now,
        "http_fail": http_fail + fail_now,
        "replayed": replayed,
        "rate": rate,
        "token": token,
        "updated": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    })
    return {"window": window, "sent": sent_now, "verified": verified}
`

// edgeSource is the gate edge — the program that used to sit in front of
// every tap before it was allowed anywhere near AWS. It is Go, it is meant to
// be built into a real binary, and it is deliberately never told where to
// run: EC2, a container, whatever the player already has lying around. Every
// downstream check reads the ledger and the tally, neither of which knows or
// cares what process answered the HTTP request.
//
// It is correct except for one line. The signature is computed and compared
// - hmac.Equal is even the right, constant-time way to compare it - and then
// the result is thrown away. Every tap is forwarded regardless of whether it
// was ever signed by anyone Meridian trusts. Nothing about the request or
// response says so; the pipeline downstream looks exactly as healthy as a
// working one, right up until someone reads what landed in the ledger.
const edgeSource = `package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type tap struct {
	TapID     string ` + "`json:\"tap_id\"`" + `
	Token     string ` + "`json:\"token\"`" + `
	Window    int    ` + "`json:\"window\"`" + `
	Station   string ` + "`json:\"station\"`" + `
	RiderHash string ` + "`json:\"rider_hash\"`" + `
	FareCents int    ` + "`json:\"fare_cents\"`" + `
	Ts        string ` + "`json:\"ts\"`" + `
	Sig       string ` + "`json:\"sig\"`" + `
}

func canonical(t tap) string {
	return strings.Join([]string{
		t.TapID, t.Token, fmt.Sprintf("%d", t.Window), t.Station,
		t.RiderHash, fmt.Sprintf("%d", t.FareCents), t.Ts,
	}, "|")
}

func sign(secret, message string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	return hex.EncodeToString(mac.Sum(nil))
}

func main() {
	secret := os.Getenv("EDGE_SECRET")
	ingest := os.Getenv("EDGE_INGEST_URL")
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	if secret == "" || ingest == "" {
		log.Fatal("EDGE_SECRET and EDGE_INGEST_URL are required")
	}

	client := &http.Client{Timeout: 5 * time.Second}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	http.HandleFunc("/taps", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		var t tap
		if err := json.Unmarshal(body, &t); err != nil || t.TapID == "" || t.Token == "" {
			http.Error(w, "malformed tap", http.StatusBadRequest)
			return
		}

		expected := sign(secret, canonical(t))
		// TODO(vendor): this is the whole check. hmac.Equal is correct and
		// constant-time. Comparing its result to something is the part
		// nobody ever got around to.
		_ = hmac.Equal([]byte(expected), []byte(t.Sig))

		resp, err := client.Post(ingest, "application/json", strings.NewReader(string(body)))
		if err != nil {
			http.Error(w, "upstream unreachable", http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
	})

	log.Printf("meridian gate edge listening on :%s", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}
`

// settlementSource is the second stage: it takes whatever the edge forwarded
// and turns it into a farebox row. It is wrong in two ways, and they are
// independent of each other and of the edge's bug.
//
// The bounds check is real and correct - a fare outside what a card reader
// can physically produce is rejected rather than trusted - but rejecting it
// means raising, and an SQS batch with no dead-letter queue behind it does
// not have anywhere for that rejection to go. It redelivers until it ages
// out, and every healthy tap sharing its batch redelivers with it.
//
// The ledger write is unconditional. ADD on settle_count is the right
// instinct for "count how many times this happened" and the wrong one for "a
// tap should only ever settle once" - which is the same lesson every
// at-least-once queue in this codebase eventually teaches, arrived at here by
// a different road: not a redelivery, but a device outside AWS entirely
// resending something it captured once and correctly signed.
const settlementSource = `import json, os, time
import boto3

# REGION always resolves correctly on real Lambda; ENDPOINT is never set
# there. Setting AWS_ENDPOINT_URL yourself when deploying this against
# fakecloud (a real credential's worth of extra config, nothing baked in
# here) is what lets you verify this stage actually settles a fare before
# you ship it - see docs/challenges/aws/validate.md.
REGION = os.environ.get("AWS_REGION") or os.environ.get("AWS_DEFAULT_REGION") or "us-east-1"
ENDPOINT = os.environ.get("AWS_ENDPOINT_URL") or None
CREDS = {"aws_access_key_id": "test", "aws_secret_access_key": "test"} if ENDPOINT else {}
ddb = boto3.client("dynamodb", region_name=REGION, endpoint_url=ENDPOINT, **CREDS)
LEDGER = os.environ["LEDGER_TABLE"]

MIN_FARE = 25
MAX_FARE = 2500


def handler(event, context):
    settled = 0
    for record in event.get("Records", []):
        tap = json.loads(record["body"])
        fare = int(tap.get("fare_cents", 0))
        if fare < MIN_FARE or fare > MAX_FARE:
            # a misread, not a forgery - the signature on this was real. Not
            # ours to fix and not ours to swallow.
            raise ValueError("implausible fare on tap %s: %d cents" % (tap.get("tap_id"), fare))
        ddb.update_item(
            TableName=LEDGER,
            Key={"tap_id": {"S": str(tap["tap_id"])}},
            UpdateExpression=(
                "ADD settle_count :one "
                "SET #tok = :tok, station = :stn, fare_cents = :fare, settled_at = :at"
            ),
            ExpressionAttributeNames={"#tok": "token"},
            ExpressionAttributeValues={
                ":one": {"N": "1"},
                ":tok": {"S": str(tap["token"])},
                ":stn": {"S": str(tap.get("station", ""))},
                ":fare": {"N": str(fare)},
                ":at": {"S": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime())},
            },
        )
        settled += 1
    return {"settled": settled}
`

// pipelineReadme travels in the zip. It states the contract each half owes
// the tap it handles and says nothing about idempotency, replay, or where to
// run anything - those are the inferences the challenge is testing, and
// writing them down would just be checking the answer.
const pipelineReadme = `meridian farebox - gate edge and settlement
=============================================

Two pieces came out of the account when the concessionaire's contract ended
at midnight. Nobody at the authority has ever run either of them.

THE EDGE (edge.go)

Every fare gate in the network POSTs a tap to whatever answers at the address
you publish. This is that address's software. It has always been a plain Go
program - build it, run it, wherever you like. Nothing downstream of it knows
or cares what it is running on; it only cares that something is answering.

  build:   GOOS=linux GOARCH=amd64 go build -o edge .
  run:     EDGE_SECRET=<the signing secret> EDGE_INGEST_URL=<given to you
           separately> ./edge
  listens: POST /taps on :8080 (override with PORT)

Once it is reachable, tell the rest of the pipeline where: put the URL in
the SSM parameter named in your briefing. The gate network probes it once a
minute and nothing moves until it does.

THE CONTRACT

Every tap carries a signature over its own fields. A tap Meridian's gates
never produced does not carry a signature Meridian's gates would have made.
The edge is the only thing standing between "whoever POSTs to this address"
and the settlement pipeline, and it is the only place in the whole system
that is in a position to tell the difference.

THE SETTLEMENT STAGE (settlement.py)

Reads a validated tap off the intake queue and writes the farebox row. Fare
bounds are enforced already - a reading outside what a card reader can
produce is rejected, not trusted. Handler is "settlement.handler", runtime
python3.12. An execution role is already sitting in this account for it; you
cannot create your own and you do not need to.

A tap identifies one ride. Two taps carrying the same tap_id are the same
ride, no matter how far apart they arrive, no matter what device sent them,
and no matter whether the first delivery is the one you remember settling.

OPERATIONAL NOTE

The concessionaire's contract required their systems access to be revoked at
midnight. Revoking a login and revoking a gate's ability to correctly sign a
request are two different operations, and only one of them happened.
`

// pipelinePackage builds the deployment zip in memory. A fixed modification
// time keeps the artifact byte-identical between runs, so a player who
// downloads it twice does not get two different files.
func pipelinePackage() []byte {
	stamp := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

	buffer := &bytes.Buffer{}
	archive := zip.NewWriter(buffer)
	for _, entry := range []struct{ name, content string }{
		{"edge.go", edgeSource},
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

// lambdaCodeZip wraps source as the single-file zip every scenario-owned
// function expects: entry name index.py, matching the "index.handler"
// every scenario-owned function is declared with. A fixed modification time
// keeps repeated builds byte-identical, same reasoning as pipelinePackage.
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

// uploadLambdaCode is how the plugin's own scenario-owned functions get a
// real zip instead of the inline Code.ZipFile convenience path: ZipFile
// there is meant to carry plain source text that AWS Lambda wraps into a
// deployable zip server-side, and nothing in Cloud Control's own resource
// model can do that on the plugin's behalf.
//
// The asset channel (AddAsset's underlying host call) is the only way a
// plugin can put bytes in S3 at all — Cloud Control has no AWS::S3::Object
// resource, so there is no other route. Calling the host call directly
// rather than going through Scenario.AddAsset is deliberate: AddAsset also
// registers the object as a player-visible download, and Lambda code that
// exists purely so bootstrap can point Code.S3Bucket/S3Key at it is not a
// handout.
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
