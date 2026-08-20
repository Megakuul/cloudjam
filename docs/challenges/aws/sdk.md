# The plugin SDK

**Read the source for the API; read this for what the source cannot tell you.** Signatures
and fields drift, so nothing here repeats them:

| What you need | Where it actually is |
| --- | --- |
| `Scenario`, `Check`, `Event` — every method and field | `pkg/challenge/challenge.go`, ~250 lines, read it whole |
| `Create` / `Read` / `List` / `Update` / `Delete` | `pkg/challenge/aws/aws.go`, ~140 lines |
| A resource type and its fields | `pkg/challenge/aws/<service>/<service>.go` — `grep -n "^type Bucket struct" -A 40 pkg/challenge/aws/s3/s3.go` |
| Which services exist at all | `ls pkg/challenge/aws/` |
| A worked challenge | `examples/challenges/s3-encryption` |
| jamctl's commands and flags | `go run ./cmd/jamctl --help` |

What follows is semantics, traps and intent — the things that are invisible in a signature
and will cost you a run if you guess.

Everything in the SDK is `//go:build wasip1`. It does not compile for your host and is not
meant to — `jamctl` sets `GOOS=wasip1 GOARCH=wasm` for you.

## Scenario

`challenge.New(title, interval)` returns the one scenario a plugin owns. `interval` is the
loop period and is clamped to a 10s minimum, so nothing you write can poll faster than that.

The builder methods (`AddDescription`, `AddClue`, `AddAsset`, `AddCheck`, `AddEvent`, and the
`Remove…` pair) are chainable and safe to call before or after `Start`. That split is the
part worth knowing: **anything added before `Start` is published as one initial metadata
call; anything added after is pushed incrementally.** That is what makes staged storytelling
work — an event can add a description, a clue and three checks mid-game and the player sees
them appear.

`Start()` publishes the metadata and enters the loop. It never returns, and it panics if
called twice.

## Permission and guardrail

Both are mandatory — `Start` panics if either is unset — and they are not the same thing:

| | What the player gets | What it becomes in AWS | Hard size limit |
| --- | --- | --- | --- |
| `SetPermission` | what they can do *now*; bootstrap and events widen it as the story moves | an **inline policy** on the sandbox role | **10 240 characters** |
| `SetGuardrail` | the ceiling they can never exceed, even by writing themselves a new policy | a **managed policy** attached as the role's **permissions boundary** | **6 144 characters** |

The size limits are IAM quotas, they are not negotiable, and going over is a hard failure:

```
create_guardrail: failed to create permission boundary: operation error IAM: CreatePolicy,
https response error StatusCode: 409, LimitExceeded: Cannot exceed quota for PolicySize: 6144
```

**A local `jamctl` run will not catch this.** fakecloud accepts an oversized policy happily
(verified: a 10 kB managed policy is created without complaint), so the challenge only
breaks once it is handed to a real account. And nothing stops when it does: host calls
cannot return errors to the plugin (see below), so the failure lands in the *host* log
prefixed with the host function name — `create_guardrail: …` — while the plugin sails on
and provisions a scenario the player has no boundary, and often no permissions, for.
Grep the run log for the host function names; do not assume silence means success.

### What blows the budget

Expanding a service's generated action groups. `policy.ActionsFrom` writes every action out
as a literal string, and the big services have hundreds:

| `ActionsFrom(…)` | Actions | Bytes of policy |
| --- | --- | --- |
| `ec2.ActionsRead, ec2.ActionsList, ec2.ActionsWrite` | 768 | **25 908** |
| `ssm.ActionsRead, ssm.ActionsList, ssm.ActionsWrite` | 157 | 4 632 |
| `s3.ActionsRead, s3.ActionsList, s3.ActionsWrite` | 141 | 4 206 |
| `logs.ActionsRead, logs.ActionsList, logs.ActionsWrite` | 126 | 3 485 |
| `dynamodb.ActionsRead, ActionsList, ActionsWrite` | 74 | 2 297 |

One service with `ActionsWrite` can already be four times the boundary quota. Two or three
of them together is the mistake that produced the error above.

### Write the guardrail with wildcards

The guardrail is a *ceiling*, not a grant — it decides which services the player can touch
at all, not which calls. It has no business being precise, and precision is what costs
bytes. Use service or prefix wildcards:

```go
SetGuardrail(policy.Document{
	Version: policy.Version20121017,
	Statement: []policy.Statement{
		// the player never needs iam; everything else the challenge lives in is open,
		// and SetPermission decides what they actually hold at any moment.
		{
			Effect:   policy.Allow,
			Action:   policy.Actions{"ec2:*", "logs:*", "kms:*", "ssm:*"},
			Resource: policy.ARNAll,
		},
	},
})
```

That is 116 bytes; those same four services expanded through `ActionsFrom` are 34 991. Keep
the fine-grained, per-ARN, generated-constant work in `SetPermission`, where the budget is
10 240 and the document is usually scoped to the handful of resources bootstrap just
created — `examples/challenges/audit-day` does both, and names its actions rather than
pulling in whole action groups.

If a challenge genuinely needs a narrow boundary, narrow it with `Deny` on the few dangerous
actions rather than enumerating everything you allow — a deny list of names is short, an
allow list of a whole service is not. `NotAction` is the other short form.

### Check the size before you ship

The `policy` package builds for the host, so the document is measurable without wasm:

```go
doc := policy.Document{ /* the same value the plugin passes to SetGuardrail */ }
fmt.Println(len(doc.String())) // must be < 6144 for a guardrail, < 10240 for a permission
```

Do that for every document the plugin can produce — including the widened ones bootstrap and
events install later, which fail exactly the same way and even more quietly.

Two more limits on the same call, for the same reason: a managed policy keeps only **5
versions**, and `SetGuardrail` after `Start` writes a new version each time without pruning
the old ones, so a plugin that rewrites its guardrail on every event dies with the same
`LimitExceeded` on the sixth. Set the guardrail once and let `SetPermission` do the moving.

## Check

A check is points, a throttle, a repeat flag and a `Trigger func() (bool, error)`.

`Trigger` returning an error reports it and scores nothing. Returning `false` is silent —
that is the normal state of an unsolved check, not a problem.

Use `Repeat` for a state worth *holding* (stayed under budget, kept the service healthy) and
give it a small value. Use a one-shot for a task *completed*.

> **Known bug, as of today:** `evaluateChecks` ranges over the check map and mutates a copy,
> so `last` and `retired` are never written back. In practice `Every` does not throttle and
> a non-`Repeat` check re-awards every cycle. Design as if it worked, but when you test, do
> not be surprised by runaway scores — and tell the user, since it inflates every scoreboard
> until it is fixed.

## Event

An event is a `Trigger func() (bool, error)` plus an `Event func(ctx, *Scenario) error`.

The body runs in its own goroutine and gets the scenario, so it can add checks, add
descriptions and mutate the account while it runs. When `Trigger` goes false again the
context is cancelled — respect `ctx.Done()` in anything long-running.

This is the chaos primitive: at 500 points, start terminating instances; after 20 minutes,
the "auditor" arrives and a new set of checks appears.

```go
s.AddEvent("outage", challenge.Event{
	Every:   30 * time.Second,
	Trigger: func() (bool, error) { return scoreAtLeast(500) },
	Event: func(ctx context.Context, s *challenge.Scenario) error {
		s.AddDescription("us-east-1a just went dark.")
		s.AddCheck("Survived the AZ loss", challenge.Check{Points: 120, Trigger: multiAZ})
		return terminateInstancesIn("us-east-1a")
	},
})
```

Do not call `AddEvent`/`RemoveEvent` from inside a `Trigger` — triggers run while the event
lock is held and it is not reentrant. From inside an `Event` body it is fine.

## Resources

`pkg/challenge/aws` is the whole cloud surface. `T` is always a pointer type.

```go
id, err := aws.Create(&s3.Bucket{BucketName: new("demo")})   // blocks until created
b, err  := aws.Read[*s3.Bucket](id)                          // live state
all, err := aws.List[*s3.Bucket]()                           // map[identifier]*Bucket
err = aws.Update(id, &s3.Bucket{...})                        // patch: only fields you set
err = aws.Delete[*s3.Bucket](id)                             // does not block
```

`Create` returns the **primary identifier** — for a bucket that is its name, for a VPC it is
`vpc-0abc…`. Keep it; it is how every later call reaches the resource. For resources AWS names
itself, `List` is the only way to find them again after a restart.

### Fields and constants

Every field is a pointer, so "unset" and "zero" are different things. Set them with `new`:

```go
&s3.Bucket{
	BucketName: new("cloudjam-demo"),
	VersioningConfiguration: &s3.VersioningConfiguration{
		Status: new(s3.VersioningConfigurationStatusEnabled),
	},
	Tags: []s3.BucketTag{{Key: new("cloudjam"), Value: new("demo")}},
}
```

Properties with a fixed value set are generated as typed constants
(`<Type><Property><Value>`). Use them; a typo in a raw string is a runtime failure, a typo in
a constant is a compile error.

Structs are generated per service from the AWS resource schemas and carry the exact Cloud
Control property names, read-only attributes included (`Arn`, `DomainName`, …). If you need
a resource type, look for it in `pkg/challenge/aws/<service>/`; if the service package is
missing, the type does not exist in the schema bundle.

### Update is a patch, and it is shallow

`Update` walks the fields you set and emits one RFC 6902 `add` per **top-level** property.
It deliberately does not descend, because `add` on `/Foo/Bar` is rejected outright when the
resource has no `Foo` yet — the normal case when a challenge adds missing configuration.

Consequence: setting one field of a nested object **replaces that whole object**. Write the
group whole, exactly as it should end up.

## Logging, reporting, score

`slog` is wired to the host, so `slog.Info("…")` lands in the run log — this is your debugging
channel. Errors inside triggers are reported automatically.

Score is written only through checks. There is no "add 10 points" call from arbitrary code;
if you want to award something from an event, add a `Repeat: false` check whose trigger
returns true once.

## The host ABI cannot return errors

The wasm ABI carries one pointer each way and the output structs have no error field. When a
host call fails, the guest gets a **zero value and a nil error** — the failure is logged host
side and is invisible to your plugin.

So: `aws.Create` returning `id == ""` means it failed. `aws.Read` returning a struct with
everything nil may mean the read failed, not that the player deleted the resource. Never
treat "empty" as "absent" in a check that awards points — use `List` and look for the
identifier, which distinguishes the two.

## A panic in a trigger ends the challenge

There is no recover around `Trigger`. One nil dereference takes down the whole plugin, the
loop stops and the player's game is over — this is not theoretical, it is the first thing
that happens when a check reads a resource that was never provisioned:

```
ERR read AWS::S3::Bucket "cloudjam-encrypt-me": decode host response: unexpected end of JSON input
panic: runtime error: invalid memory address or nil pointer dereference
```

So in every trigger:

- check the error **and** return early — `if err != nil { return false, err }` on its own line,
  not folded into a `||` with a dereference,
- nil-check every pointer before following it, including nested ones,
- never index a slice without checking its length.

A trigger that cannot read its resource should return `false, err`. The check reports and
retries next cycle; that is the correct behaviour while a resource is still being created.
