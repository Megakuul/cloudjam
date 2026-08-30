# Validating a challenge with jamctl

A challenge that compiles is not a challenge that works. The failure you will actually ship
is a check that can never turn true, and the only way to catch it is to make it turn true.

## The tool

```bash
go run ./cmd/jamctl --help                 # confirm the current command surface
go run ./cmd/jamctl run aws --help         # confirm the current flags
```

Run these first. `jamctl` changes often: command names and flags move, and a command line
copied out of a document may be stale. If `jamctl` does not compile, stop and tell the user —
do not work around it by hand-rolling a runner.

What it does today:

```bash
# compile, download/run fakecloud natively, run the plugin against it
go run ./cmd/jamctl run aws ./examples/challenges/s3-encryption

# the same plugin against real credentials instead
go run ./cmd/jamctl run aws ./examples/challenges/s3-encryption --fake=false
```

`--fake` defaults to **true** — that is the one flag whose default changes what you are
testing against; `--help` lists the rest. The first run downloads fakecloud into your user
cache dir and reuses it after that; the process is stopped when the run ends, including on
ctrl-c. Everything the plugin does shows up in the log: resources created, points awarded,
errors reported.

There is no separate compile command — `run` compiles first. To produce a module without
running it, build it yourself:

```bash
GOOS=wasip1 GOARCH=wasm go build -o challenge.wasm ./examples/challenges/s3-encryption
```

## The loop

**1. Compile.**

```bash
GOOS=wasip1 GOARCH=wasm go build -o /dev/null ./path/to/plugin
```

**2. Run it and read the log.** You are looking for: metadata registered, every resource
created with an identifier, no reported errors. A resource that fails to create means every
check that depends on it is dead.

**3. Drive each check to true.** This is the step people skip. With the run going, use the
AWS CLI against the same fakecloud endpoint and make the change the player would make:

```bash
export AWS_ENDPOINT_URL=http://127.0.0.1:4566
export AWS_ACCESS_KEY_ID=test AWS_SECRET_ACCESS_KEY=test AWS_REGION=us-east-1

aws s3api put-bucket-encryption --bucket cloudjam-leaky \
  --server-side-encryption-configuration '{"Rules":[{"ApplyServerSideEncryptionByDefault":{"SSEAlgorithm":"AES256"}}]}'
```

Then watch for the award in the log. No award within a cycle or two means the check is broken
— almost always because the property does not come back on read.

**4. Check what the plugin actually sees.** When a check will not fire, look at the state
Cloud Control returns, not at what you set:

```bash
aws cloudcontrol get-resource --type-name AWS::S3::Bucket --identifier cloudjam-leaky \
  --query 'ResourceDescription.Properties' --output text | jq .
```

If the property you are checking is not in that JSON, your check cannot work. Pick another
signal.

## Cloud Control does not return everything

This is the single most important thing to internalise, and it is worse than "fakecloud is
incomplete": **fakecloud and real AWS disagree in both directions.**

- Real AWS resource handlers implement their own read logic, and several omit sub-resources
  that are technically separate API calls. Historically S3's handler has not returned things
  like `PublicAccessBlockConfiguration`; other services have their own gaps.
- fakecloud is a reimplementation. Measured today it *does* return
  `PublicAccessBlockConfiguration` for `AWS::S3::Bucket` — so a check that passes locally can
  be permanently dead on real AWS.
- It also has gaps of its own, and a smaller service list than AWS.

So a green run on fakecloud proves the plugin's **wiring** — it compiles, provisions, the loop
runs, the triggers execute. It does not prove the challenge is winnable on the real thing.

**What to do about it**

- Verify each check's property against the environment the challenge will ship to. For real
  AWS that means one run with `--fake=false` against a sandbox account.
- Prefer signals that are hard to omit: the existence of a resource (`List` and look for the
  identifier), tags, and top-level properties that are part of the resource's own create
  schema.
- Be suspicious of anything that is a separate AWS API call in real life —
  public access blocks, bucket policies, encryption settings, versioning, instance attributes.
  Those are exactly the ones handlers skip.
- When a property is unreliable, restructure: ask the player to *create* something that proves
  the fix (a specific config resource) rather than flipping a sub-property you cannot see.
- Say which checks you verified where. "Verified on fakecloud, unverified on AWS" is a useful
  handover; silence is not.

## Services that need a container, and why the Lambda you get still cannot do anything

fakecloud runs Lambda, RDS, ElastiCache, MQ, MSK, ECS and EC2 userdata by starting a real
container per workload through the Docker socket. `jamctl run aws --fake` downloads and runs
fakecloud as a **native process on the host** (cached under your user cache dir, no Docker
container for fakecloud itself) precisely so this works: a fakecloud that was itself inside a
container could not reliably reach the sibling containers it spawns to run those workloads.

That gets a Lambda to actually **invoke**. Getting it to do something *useful* once invoked —
call another AWS service and have that call land on fakecloud rather than vanish — needs a
little more, all confirmed end to end against a real native fakecloud (v0.44.10) with a
Lambda triggered automatically off an SQS event source mapping, writing a real row into a
real DynamoDB table:

1. **Inline `Code.ZipFile` source never becomes a real zip.** Real AWS Lambda (via
   CloudFormation/Cloud Control) accepts plain source text in `Code.ZipFile` for Python/Node
   functions and wraps it into a deployable zip server-side — that is the whole premise behind
   every `Code: &lambda.Code{ZipFile: new(pythonSource)}` call in this codebase. fakecloud does
   not do that wrapping: it stores whatever string arrives and tries to literally unzip it at
   invoke time, so every such function fails with `ZIP extraction failed: invalid Zip archive:
   Could not find EOCD`. Confirmed for both plain source and a base64-encoded real zip — neither
   survives. Not fixable from plugin code (no S3 data-plane access to fall back to, see
   `sdk.md`) — this only affects the plugin's own scenario-owned functions (generator, ingest).
   A player's own Lambda, deployed the normal way with `aws lambda create-function --zip-file
   fileb://...`, is a real zip and clears this step at the container level.
2. **No region.** `boto3.client(...)` with no explicit region dies with `NoRegionError`. Fix it
   in your challenge's Python source — don't rely on automatic detection, resolve it yourself
   and pass it explicitly:
   `REGION = os.environ.get("AWS_REGION") or os.environ.get("AWS_DEFAULT_REGION") or "us-east-1"`,
   then `boto3.client("dynamodb", region_name=REGION)`. Do **not** fix this by setting
   `AWS_REGION` as a function environment variable — it is a reserved Lambda key and real AWS's
   `CreateFunction`/`UpdateFunctionConfiguration` rejects the attempt outright. `meridian-farebox`
   does this in all three of its Python sources; copy the pattern — it costs nothing and is
   always correct, on fakecloud or real AWS.
3. **fakecloud has to be reachable from the container it spawned**, which needs two things
   outside this repo entirely and has nothing to do with the challenge:
   - **fakecloud must listen on all interfaces, not just loopback.** `jamctl` runs fakecloud
     with `--addr 0.0.0.0:<port>`, not `127.0.0.1`, for exactly this reason: the spawned
     container reaches the host over the Docker bridge gateway (`host.docker.internal`,
     resolvable inside the container — confirmed via `socket.gethostbyname`), and a fakecloud
     bound only to loopback refuses that connection outright even once it arrives.
   - **the host firewall has to let that connection through.** On a stock NixOS system this is
     the one worth knowing: `networking.firewall` (nftables-based) defaults to `policy drop` on
     the input chain for anything not in `networking.firewall.trustedInterfaces`, and `docker0`
     is not in that list by default. The symptom is a **silent timeout**, not a refusal — the
     packet is dropped, not rejected — which looks exactly like a routing problem and is not
     one. Fix: `networking.firewall.trustedInterfaces = [ "docker0" ];` (add to whatever else is
     already there) and `nixos-rebuild switch`. Confirmed against a symptom that had nothing to
     do with fakecloud at all: a bare `curlimages/curl` container timed out reaching a bare
     `python -m http.server` on the host the same way, until this was added.
4. **No credentials.** Past region, the same call dies with `NoCredentialsError: Unable to
   locate credentials`. fakecloud's own root bypass (any access key starting with `test`, see
   `/docs/reference/security`) clears it and is safe to use as a fallback the same way as
   region — real AWS always populates a real key, so the fallback is never reached there.
5. **Wrong endpoint.** With no `endpoint_url` set, `boto3` builds the real regional AWS
   endpoint and the call leaves the sandbox entirely — confirmed by checking fakecloud's own
   dispatch log, which never shows the request. `endpoint_url="http://host.docker.internal:4566"`
   is the fix, and — once 3 is sorted — it works: confirmed with a real settled row appearing in
   DynamoDB after an SQS-triggered invoke. **This is not safe to hardcode unconditionally in
   challenge code that also has to run on real AWS**, though: `AWS_ENDPOINT_URL` is absent from
   the Lambda environment on *both* fakecloud and real AWS, so there is no signal in the
   environment that tells code which one it is on. What would fix that properly is fakecloud
   injecting a hostname marker the way LocalStack injects `LOCALSTACK_HOSTNAME` — that is
   fakecloud's gap to close, not a challenge author's. Until then, the pattern above (region,
   credentials, endpoint, all three explicit) is the correct **manual verification recipe**:
   patch it into a copy of the function's source, deploy that copy, watch it actually settle
   something, then ship the unpatched version, which is what stays correct on real AWS.

Practically: fixing 1 (fakecloud's zip handling) is out of your reach and still blocks every
scenario-owned generator/stage in this codebase — `kestrel-chain-of-custody`,
`ridgeline-settlement`, `pueblo-night-shift` and `meridian-farebox` alike. Fixing 2 and 3 is a
one-time setup cost (already done for you in `jamctl`, plus whatever your OS's firewall needs).
Fixing 4 and 5 as a temporary patch is how you *prove* a player-deployed stage's logic is
correct before shipping the real version — do that for every stage in the challenge you are
writing, because a scenario that only "looks" wired (resources created, no errors reported) is
not the same thing as one that has been watched to actually move data. The one part you cannot
close this way is 1: any function using inline `Code.ZipFile` source (every scenario-owned
generator/stage) still cannot invoke at all, patched or not, so the traffic those specifically
produce still needs a real account to verify.

Everything that is pure control plane — S3, IAM, SSM parameters, EC2 metadata, the Cloud
Control CRUDL the SDK is built on — works fine and needs none of this.

Also: fakecloud prints its registered service list at startup. Read that line before assuming
a service exists.

## Cleaning up

fakecloud is stopped and its state discarded when the run ends, including on ctrl-c; nothing
persists between runs except the cached binary itself. On a real account, `jamctl` has a nuke path
(check `--help`); it erases everything the credentials can reach, which is why it only ever
points at a sandbox account.

## Reporting back

When you hand a challenge over, state:

- which checks you watched fire, and in which environment
- which you could not verify, and why
- the maximum score, and the split across acts
- anything you provisioned that costs money per hour
