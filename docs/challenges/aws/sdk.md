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
