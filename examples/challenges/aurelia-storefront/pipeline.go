//go:build wasip1

package main

// The programs outside the plugin.
//
// storefrontSource is the one Lambda behind the API, and it is correct — it
// answers every route it owns and has no notion that some of its traffic is
// hostile. Nothing here is deployed by the player; the entire pipeline is
// wired and reachable from bootstrap. What is unfinished is the layer in
// front of it: a WAFv2 WebACL that exists, is not attached to anything, has
// one rule left in count-only mode, and is missing the rule that should stop
// the busiest bot pattern outright.
//
// generatorSource is both halves of the traffic at once — real customers and
// the bots hammering the same front door — sent as genuine HTTP requests
// against the API's real endpoint, not simulated. It classifies what comes
// back by status code and keeps a running tally that the plugin's own checks
// read back through Cloud Control, the same way every gameday challenge in
// this codebase scores real traffic.

import (
	"archive/zip"
	"bytes"
	"fmt"
	"strings"
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge/api"
)

// storefrontSource never changes. It has four routes and none of them are
// the bug.
const storefrontSource = `import json


def response(code, body):
    return {"statusCode": code, "headers": {"content-type": "application/json"}, "body": json.dumps(body)}


def handler(event, context):
    ctx = event.get("requestContext", {}) or {}
    http = ctx.get("http", {}) or {}
    method = http.get("method", "GET")
    path = event.get("rawPath", "/")

    if path == "/catalog" and method == "GET":
        return response(200, {"items": ["lantern", "map-case", "field-kettle"]})
    if path == "/checkout" and method == "POST":
        return response(200, {"status": "ok", "order_id": context.aws_request_id})
    if path == "/export" and method == "GET":
        return response(200, {"export": "full-catalog-dump"})
    if path == "/login" and method == "POST":
        return response(200, {"status": "accepted"})
    return response(404, {"error": "not found"})
`

// generatorSource sends real HTTP requests at the real API endpoint - through
// the local override on fakecloud, directly on real AWS - and reads nothing
// back except the status code the WAF layer (or its absence) produced.
const generatorSource = `import json, os, random, secrets, time
from urllib.request import Request, urlopen
from urllib.error import HTTPError, URLError
import boto3

REGION = os.environ.get("AWS_REGION") or os.environ.get("AWS_DEFAULT_REGION") or "us-east-1"
ENDPOINT = os.environ.get("AWS_ENDPOINT_URL") or None
CREDS = {"aws_access_key_id": "test", "aws_secret_access_key": "test"} if ENDPOINT else {}
ssm = boto3.client("ssm", region_name=REGION, endpoint_url=ENDPOINT, **CREDS)

API_HOST = os.environ["API_HOST"]
API_ENDPOINT = os.environ["API_ENDPOINT"]
LOCAL_BASE = os.environ.get("LOCAL_HTTP_BASE") or None
TALLY = os.environ["TALLY_PARAM"]

BASE_RATE = int(os.environ.get("BASE_RATE", "20"))
BOT_RATE = int(os.environ.get("BOT_RATE", "6"))
STUFFING_SURGE = int(os.environ.get("STUFFING_SURGE_WINDOWS", "0"))


def base_url():
    return LOCAL_BASE if LOCAL_BASE else API_ENDPOINT


def call(method, path, body=None, headers=None):
    url = base_url() + path
    h = {"Host": API_HOST} if LOCAL_BASE else {}
    if headers:
        h.update(headers)
    data = body.encode() if body is not None else None
    req = Request(url, data=data, headers=h, method=method)
    try:
        with urlopen(req, timeout=5) as resp:
            return resp.status
    except HTTPError as e:
        return e.code
    except URLError:
        return 0
    except Exception:
        return 0


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


def handler(event, context):
    s = read_tally()
    window = int(s.get("window", "0")) + 1
    legit_served = int(s.get("legit_served", "0"))
    legit_blocked = int(s.get("legit_blocked", "0"))
    scrape_blocked = int(s.get("scrape_blocked", "0"))
    scrape_leaked = int(s.get("scrape_leaked", "0"))
    sqli_blocked = int(s.get("sqli_blocked", "0"))
    sqli_leaked = int(s.get("sqli_leaked", "0"))
    stuffing_blocked = int(s.get("stuffing_blocked", "0"))
    stuffing_leaked = int(s.get("stuffing_leaked", "0"))

    for _ in range(BASE_RATE):
        code = call("GET", "/catalog")
        if code == 200:
            legit_served += 1
        elif code == 403:
            legit_blocked += 1
        code = call("POST", "/checkout", body="item=%d&qty=1" % random.randint(1, 999))
        if code == 200:
            legit_served += 1
        elif code == 403:
            legit_blocked += 1

    for _ in range(max(3, BOT_RATE)):
        code = call("GET", "/export")
        if code == 403:
            scrape_blocked += 1
        elif code == 200:
            scrape_leaked += 1

    for _ in range(max(3, BOT_RATE)):
        code = call("POST", "/checkout", body="id=1' OR '1'='1 -- ")
        if code == 403:
            sqli_blocked += 1
        elif code == 200:
            sqli_leaked += 1

    if STUFFING_SURGE > 0:
        session = "stuffed-" + secrets.token_hex(4)
        for _ in range(60):
            code = call("POST", "/login", body="user=x&pass=y", headers={"X-Session-Id": session})
            if code == 403:
                stuffing_blocked += 1
            elif code == 200:
                stuffing_leaked += 1

    write_tally({
        "window": window,
        "legit_served": legit_served, "legit_blocked": legit_blocked,
        "scrape_blocked": scrape_blocked, "scrape_leaked": scrape_leaked,
        "sqli_blocked": sqli_blocked, "sqli_leaked": sqli_leaked,
        "stuffing_blocked": stuffing_blocked, "stuffing_leaked": stuffing_leaked,
        "updated": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    })
    return {"window": window}
`

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
