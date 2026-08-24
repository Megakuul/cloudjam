# Designing the challenge

The plugin is the easy part. The design is what makes it a gameday instead of a checklist.

## Start with the incident, not the resource

Bad: "the player enables encryption on a bucket." Good: "a customer emailed to say they found
your invoices in a Google search — find what is exposed and close it before the auditor's
report lands in 40 minutes."

The difference is that the second one has a **motive, a clock and a discovery phase**. The
player has to look before they can act. Encryption is still the check; the story is what makes
them earn it.

Every challenge should be able to answer:

- Who is the player supposed to be? (on-call engineer, new hire, incident commander)
- What went wrong, and who is upset about it?
- What does "done" look like, and how does the player know?
- What happens if they do nothing? (something should get worse)

## Tiers

### Warmup — 15 to 45 minutes

One resource, one idea. Three or four checks in the 10–50 point range. No events. The player
should never need the AWS docs.

Provision the flawed thing, check that the flaw is gone. `examples/challenges/s3-encryption`
is exactly this and is the template.

### Standard — 1 to 3 hours

A small system: a VPC with something in it, a queue with a consumer, a bucket behind a
distribution. Five to ten checks, partial credit, one or two events. The player needs to
understand how the pieces connect, not just flip a setting.

Structure it as **discover → fix → prove**. The last third should be a check that only passes
if the fix actually works end to end, not just if the configuration looks right.

### Gameday — half a day or more

This is the format the platform exists for, and it is not simply a longer Standard. Two
things make a challenge gameday tier, and a challenge that has the act structure without
them is a long checklist:

**The brief states obligations, not steps.** Warmup tells the player what to do. Gameday
tells them what the business is on the hook for and lets them work out what that means.
"Enable point-in-time recovery on the ledger" is a Warmup instruction; "the regulator can
ask us to reproduce any result from the last 30 days, and right now we cannot" is a gameday
one. Never enumerate the fix. Never number the tasks. If the briefing can be followed
top-to-bottom without the player forming their own model of the estate, it is not gameday.

**The player supplies the judgment a good technical manager would.** They should have to
decide what matters first, what is safe to defer, and what is not worth doing at all — with
more surface in front of them than they have time for, and no list telling them which parts
are scored. Make some of the available work genuinely low value, and make at least one
plausible shortcut cost points rather than earn them. The scoreboard is the only feedback
they get on whether their priorities were right, and it should be able to tell them they
were wrong.

Concretely, that means the estate must be **discoverable but undocumented** (tags, a
handover note, an asset — see below), the total scored work must exceed what fits in the
window, and no check may be satisfiable by pattern-matching a sentence in the briefing.

#### Pipelines: the shape that carries a gameday best

The strongest gameday scenario is a **multi-stage pipeline the player has to complete or
repair, scored on the data that comes out the far end**. It gives you everything the tier
needs in one structure: the stages are the discovery, wiring them is the core work, and the
output is a scoreboard that cannot be faked by configuration that merely looks right.

Build it as three or more hops the data must survive — an intake queue, one or more
transform stages, a durable sink — and have each stage stamp the record it passes on. Then
score the **far end**, not the wiring:

- **Throughput** — records that arrived. The baseline "is it working at all" signal.
- **Integrity** — records that arrived carrying every stage's stamp, in order. This is the
  check that separates a pipeline that works from one that merely moves bytes, and it is
  the one worth the most points.
- **Deviation** — records that arrived with a broken or partial chain. Score these
  **negative**. A half-wired pipeline that delivers untraceable records is worse than one
  that delivers nothing, and making the scoreboard say so is what forces the player to
  think like a manager instead of racing to first delivery.

The tally comes back through an SSM parameter written from inside the account — see
[traffic generation](#traffic-generation-and-scoring-requests) below, which is the same
machinery. Have the generator settle each window against the *previous* window's token so
nothing is counted twice, and classify as it settles: the plugin should read counters, not
re-derive them.

Two worked examples, and they fail in different directions — read whichever matches the
scenario you are writing:

| Example | The pipeline | What the far end measures |
| --- | --- | --- |
| `kestrel-chain-of-custody` | intake → accession → assay → clinical ledger | **provenance.** Each stage stamps the record; the stage the player is handed *replaces* the chain instead of appending, so results arrive looking perfect and carrying no history. |
| `ridgeline-settlement` | meter intake → validation → settlement → settled ledger | **idempotency.** The stage settles unconditionally, so an at-least-once queue pays twice. Act III is a substation flushing three days of backlog through it. |

`examples/challenges/pueblo-night-shift` is the simpler two-hop version of the same shape.

Structure it in acts, each act gated on progress or elapsed time, each act adding
descriptions, clues and checks:

1. **Act I — orientation (0–45 min).** The player inherits an undocumented system. Cheap
   checks for finding and describing things: tag the resources you own, get the app answering
   at all. 10–20% of total points.
2. **Act II — the incident (45 min–2 h).** The failure arrives. Traffic starts, or a
   dependency breaks. Real work: scale it, secure it, make it redundant. 40–50% of points.
3. **Act III — the twist (2 h+).** A chaos event invalidates a shortcut. The AZ they put
   everything in goes away; the credentials they hardcoded get rotated; the bill spikes and
   there is now a cost check. 30–40% of points.
4. **Coda.** A held state: keep it up and under budget for the last 30 minutes. `Repeat`
   checks, small values, awarded per cycle.

Pace the acts with events, not with a wall clock alone — a player who is still stuck in Act I
should not get hit by Act III.

## Chaos events

An event is a `Trigger` plus a body. Good triggers:

- **Progress** — `score >= 500`, or a specific check passed. Paces the story to the player.
- **Elapsed time** — capture `start := time.Now()` before `Start()` and compare. Use for
  pressure, sparingly.
- **State** — the player created the thing that the event is about to break.

Good events do one of three things: **break something** (terminate, detach, revoke),
**raise the bar** (new checks with tighter requirements), or **reveal** (a new description,
a new asset, a clue that reframes the problem).

Rules:

- Never break something the player cannot rebuild.
- Announce it. `AddDescription` in the event body — an unexplained failure reads as a platform
  bug, and the player will spend their half day debugging you.
- Retire the checks the event invalidated with `RemoveCheck`, or they will keep paying out.
- Do not stack two events in the same cycle.

## Assets

`AddAsset(name, bytes)` puts a downloadable file in front of the player. Use it for the
things a real incident comes with: an architecture diagram that is subtly wrong, a
`terraform plan` output, a log excerpt with the smoking gun, a CSV of expected request
volume, a runbook from the person who left.

Assets are generated in the plugin, so they can be built from the account's live state —
an inventory of what you provisioned, with one row deliberately missing.

## Traffic generation and scoring requests

The pattern the platform wants: something sends requests at the player's system, and points
follow the requests that were **served** — not a configuration check. It is the difference
between "you configured an ASG" and "you stayed up".

The plugin cannot make HTTP requests. It only sees the account through Cloud Control. So the
generator must run **inside the account**, and it must write its tally somewhere the plugin
can read.

### The tally channel

Cloud Control only reaches the control plane, so most data stores are unreadable — there is
no resource type for an S3 object, a DynamoDB item or an SQS message. Two channels work:

**SSM parameter (recommended).** `AWS::SSM::Parameter` is a Cloud Control resource and its
`Value` comes back on read. The generator does `ssm:PutParameter` with a running tally; the
plugin reads it every cycle.

```go
p, err := aws.Read[*ssm.Parameter]("/cloudjam/tally")
// p.Value is "served=1841 failed=17"  — parse it, award on the delta
```

**Marker resources.** The generator creates one small resource per event (a parameter per
probe window, named with a timestamp); the plugin `List`s them and counts. Better when you
care about *when* things happened, worse for high rates.

### Building the generator

A Lambda on a schedule, all via Cloud Control: `AWS::IAM::Role` for the execution role,
`AWS::Lambda::Function` with the code inline in `Code.ZipFile` (works for the Python and
Node runtimes — anything else needs the code in S3, which you do not have a good way to
place), `AWS::Scheduler::Schedule` or `AWS::Events::Rule` plus `AWS::Lambda::Permission` to
fire it every minute.

Keep the function tiny: probe the player's endpoint, add to the tally, `PutParameter`. Have
it write **cumulative** counters and score the delta, so a missed cycle does not lose data.

### Scoring it

Read the tally each cycle and award on the change since last time. A `Repeat: true` check
whose trigger closes over the last-seen counters is the natural shape. Award for served
requests, subtract for failures if you want teeth — `Points` may be negative.

Do not award per request at face value; pick a rate that makes the traffic worth roughly as
much as one good configuration check per act, or the scoreboard becomes a clock.

### What this costs you

- **The player can see and edit the tally parameter.** Treat scores from it as advisory
  unless the sandbox role is denied write on that path.
- **Lambda only runs on fakecloud if fakecloud runs natively.** Started in a container the
  way `jamctl --fake` does it, invokes fail and your generator silently produces no traffic —
  the resource graph provisions and nothing happens. See `validate.md`.
- **Polling granularity is the scenario interval**, 10s at best.

If that is too much machinery for the challenge you are writing, the honest fallback is to
score the *capability* rather than the traffic: multi-AZ, a health check that passes, an ASG
with a minimum size. Weaker, but it works today with no moving parts.

## Scoring budget

Pick a total (1000 is a good gameday number) and divide it before you write checks:

- Discovery and orientation: 10–20%
- The core work: 40–50%
- The twist: 30–40%
- Held state (`Repeat`): 10%, accumulated slowly

Partial credit beats all-or-nothing — three checks at 20 for three guardrails beats one at 60.
Negative points only for things the player chose to do, never for something an event did to
them.

## Anti-patterns

- **Checks on properties Cloud Control will not return.** The most common way a challenge is
  quietly broken. Verify first — see [`validate.md`](validate.md).
- **Guessing the answer.** A check on an exact bucket name or a specific instance type when
  any equivalent solution should count. Check the *property that matters*.
- **Silent scenarios.** If provisioning fails, the player sees an empty account and no
  explanation. Report it and put it in the briefing.
- **Unbounded cost.** Anything with an hourly rate needs a reason. No NAT gateways.
- **A story with no clock.** If nothing gets worse, nothing is urgent, and it is a checklist.
