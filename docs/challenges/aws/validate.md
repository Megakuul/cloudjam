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
# compile, start a throwaway fakecloud container, run the plugin against it
go run ./cmd/jamctl run aws ./examples/challenges/s3-encryption

# the same plugin against real credentials instead
go run ./cmd/jamctl run aws ./examples/challenges/s3-encryption --fake=false
```

`--fake` defaults to **true** — that is the one flag whose default changes what you are
testing against; `--help` lists the rest. The container is removed when the run ends,
including on ctrl-c. Everything the plugin does shows up in the log: resources created,
points awarded, errors reported.

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

## Services that need a container, and the jamctl caveat

fakecloud runs Lambda, RDS, ElastiCache, MQ, MSK, ECS and EC2 userdata by starting a real
container per workload. Whether that works depends on **how fakecloud itself was started**:

- **Native binary on the host** — everything works. Verified: a Python Lambda created with
  inline `Code.ZipFile` invokes and returns its payload in about 40 ms.
- **In a container, the way `jamctl --fake` starts it** — it does not. fakecloud spawns the
  runtime as a sibling container and cannot reach it from inside its own network namespace,
  so every invoke fails with `container did not become ready within 10 seconds`. Mounting the
  docker socket gets it as far as starting the runtime — the container comes up and answers
  on its published port from the host — but fakecloud still cannot talk to it.

So if the challenge you are writing depends on a Lambda actually running (traffic generators,
anything that writes a tally), `jamctl run aws --fake` will provision the resource graph and
then silently generate no traffic. Check the fakecloud startup log: if it prints
`No container runtime (Docker/Podman) detected`, those services are metadata-only for that
run.

The fix is to run fakecloud natively (`curl -fsSL https://fakecloud.dev/install.sh | bash`,
or `brew install fakecloud`) on port 4566 and point the plugin at it, or to verify that part
of the challenge on a real account. Say which one you did.

Everything that is pure control plane — S3, IAM, SSM parameters, EC2 metadata, the Cloud
Control CRUDL the SDK is built on — works in the container and needs none of this.

Also: fakecloud prints its registered service list at startup. Read that line before assuming
a service exists.

## Cleaning up

fakecloud is thrown away with the container. On a real account, `jamctl` has a nuke path
(check `--help`); it erases everything the credentials can reach, which is why it only ever
points at a sandbox account.

## Reporting back

When you hand a challenge over, state:

- which checks you watched fire, and in which environment
- which you could not verify, and why
- the maximum score, and the split across acts
- anything you provisioned that costs money per hour
