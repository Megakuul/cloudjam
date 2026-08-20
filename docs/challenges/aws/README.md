# Building a CloudJam AWS challenge

A challenge is a Go `main` package compiled to wasm. It provisions a scenario into a real
AWS account, then loops: evaluate checks, award points, fire events. The player works in
the account with their own credentials; the plugin only ever sees what it can read back
through the Cloud Control API.

Read [`sdk.md`](sdk.md) before writing code — the API is small but has sharp edges.

## The loop you must follow

Do not hand over a challenge you have not run.

1. **Agree the shape** — tier, story, what the player actually does. See [`design.md`](design.md).
2. **Write it** — one package under `examples/challenges/<name>/` (or where the user says).
3. **Compile** — `GOOS=wasip1 GOARCH=wasm go build ./path`. Fix until clean.
4. **Run it** — `go run ./cmd/jamctl run aws ./path`. Watch it provision.
5. **Prove every check fires** — a check that can never go true is a broken challenge, and
   this is the failure mode you will actually hit. [`validate.md`](validate.md) has the method.
6. **Report honestly** — which checks you saw fire, which you could not verify and why.

## Non-negotiables

- **Every check must be provably reachable.** Before shipping, drive the account into the
  solved state yourself and watch the points land. If you cannot observe a property through
  Cloud Control, the check is worthless — pick a different signal.
- **Cloud Control reads are incomplete and environment-dependent.** fakecloud and real AWS
  omit different things. Never assume; verify against the environment the challenge ships to.
- **Points must be bounded.** Know the maximum score. State it.
- **No unwinnable states.** If an event deletes something the player needs, they must be able
  to rebuild it — or the check must retire, not keep punishing.
- **The account is a real account.** Every resource costs money and must be nukeable. No
  NAT gateways, no RDS clusters, no `p4d` instances unless the user asked for the bill.
- **Guardrail and permission documents must fit IAM's quotas** — 6144 characters for the
  guardrail, 10240 for the permission. A local run does not enforce them; the real account
  does, and the challenge then runs with no boundary at all. Write guardrails with service
  wildcards and see [`sdk.md`](sdk.md#permission-and-guardrail).

## Anatomy

```go
//go:build wasip1

package main

import (
	"time"

	"codeberg.org/megakuul/cloudjam/pkg/challenge"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws"
	"codeberg.org/megakuul/cloudjam/pkg/challenge/aws/s3"
)

func main() {
	s := challenge.New("Lock Down the Bucket", 10*time.Second)

	s.AddDescription("Marketing left a bucket wide open. Close it before the audit.")
	s.AddClue("Where does encryption live?", "BucketEncryption on the bucket resource.")

	id, err := aws.Create(&s3.Bucket{BucketName: new("cloudjam-leaky")})
	if err != nil {
		// the scenario could not be built; say so and keep going
	}

	s.AddCheck("Enabled default encryption", challenge.Check{
		Points: 50,
		Every:  15 * time.Second,
		Trigger: func() (bool, error) {
			b, err := aws.Read[*s3.Bucket](id)
			if err != nil || b.BucketEncryption == nil {
				return false, err
			}
			return len(b.BucketEncryption.ServerSideEncryptionConfiguration) > 0, nil
		},
	})

	s.Start() // never returns
}
```

[`sdk.md`](sdk.md) covers events, assets, incremental clues, scoring and the known bugs.

## Difficulty, in one line each

| Tier | Player time | Shape |
| --- | --- | --- |
| Warmup | 15–45 min | One resource, 2–4 checks, no events. |
| Standard | 1–3 h | A small system, 5–10 checks, one or two events. |
| Gameday | Half a day+ | A story in acts, 15+ checks, chaos events, traffic under load. |

[`design.md`](design.md) is the real guide: story structure, act pacing, chaos events,
traffic generation and how to score requests that were served versus dropped.

## Before you say it is done

- [ ] Compiles for `wasip1`.
- [ ] Runs under `jamctl` and provisions without errors in the log.
- [ ] Every check observed going false → true, or explicitly flagged as unverified.
- [ ] Every guardrail document under 6144 characters and every permission document under
      10240, measured with `len(doc.String())` — including the ones bootstrap and events set.
- [ ] Maximum score stated, and it adds up.
- [ ] Every resource is nukeable and cheap.
- [ ] Clues are useful without giving the answer away.
