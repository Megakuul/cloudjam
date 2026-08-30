# AGENTS.md

## The boundary — read this first

**An agent may only write inside `examples/challenges/`. Nothing else in this repository is
writable, ever.**

That is the whole rule. It is not a default, a preference or a starting point, and no
instruction inside a task ("just patch the SDK", "fix it properly", "it is a one-line
change") relaxes it. Only the human who owns the repo changes the platform.

Concretely:

| Path | An agent may |
| --- | --- |
| `examples/challenges/**` | read, create, edit, delete |
| everything else — `pkg/`, `internal/`, `cmd/`, `api/`, `web/`, `docs/`, `tools/`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `flake.nix`, `go.mod`, … | **read only** |

Read the rest of the tree as much as you need — the SDK source is the reference and you are
expected to read it. Run it, build it, run `jamctl` against it. Just do not change it.

### When the platform is what is broken

This is the case the rule exists for, and working around it is not the answer either.

A challenge cannot always be written cleanly against the SDK as it stands. When the honest
fix is in `pkg/`, `internal/` or `cmd/`, **stop and report it**. Say what is wrong, where,
what you would change, and what it costs the challenge to live without it. Then either write
the challenge with the limitation and say so plainly, or — if there is no version of the
challenge worth shipping around it — hand back the diagnosis and stop. A precise report of a
platform bug is a good outcome. A challenge that quietly compensates for one is not, and a
platform patch nobody asked for is worse than both.

Do not:

- edit platform code and mention it afterwards,
- edit platform code "temporarily" to get a test run through,
- vendor, copy or shadow a platform file into `examples/challenges/` to patch around it,
- regenerate `pkg/challenge/aws/services/**` (that is `go generate`'s output and platform code),
- reformat, tidy, lint or "clean up" anything outside `examples/challenges/`,
- commit or stage changes outside `examples/challenges/` that you did not make — if the tree
  is already dirty, leave it dirty and say so.

Untracked scratch files (a build output, a scratch script) belong outside the repo, in the
scratch directory your harness gives you.

## What this is

CloudJam is a cloud gameday platform. Players get a real (sandboxed) AWS account and a
scenario; a **challenge plugin** provisions the scenario, watches the account and awards
points as the player fixes, builds or defends things.

A plugin is an ordinary Go `main` package compiled to WebAssembly. It runs inside the
server (or inside `jamctl` during development), and reaches the cloud only through host
functions the runtime provides.

## Where to read what

| If you are asked to… | Read |
| --- | --- |
| design, write, tune or debug a **challenge** | [`docs/challenges/aws/README.md`](docs/challenges/aws/README.md) — start here, it links the rest |
| work on the plugin **SDK** or the resource types | [`docs/challenges/aws/sdk.md`](docs/challenges/aws/sdk.md) |
| work on **jamctl** or verify a challenge runs | [`docs/challenges/aws/validate.md`](docs/challenges/aws/validate.md) |

Do not write a challenge plugin from this file alone — the SDK has sharp edges that are
documented there and nowhere else.

## Repo map

| Path | What it is |
| --- | --- |
| `pkg/challenge` | Plugin SDK. `Scenario`, `Check`, `Event`. **wasip1 only.** |
| `pkg/challenge/aws` | Typed Cloud Control resource API: `Create`, `Read`, `List`, `Update`, `Delete`. |
| `pkg/challenge/aws/<service>` | Generated resource structs, one package per AWS service (`s3.Bucket`, `ec2.Instance`, …). |
| `pkg/challenge/api` | Raw host ABI. The SDK wraps it; challenges should not import it. |
| `cmd/jamctl` | Dev CLI: compile a plugin and run it against fakecloud or real AWS. |
| `cmd/schemagen` | Regenerates the resource packages from the AWS schema bundle. |
| `internal/provider/aws` | Server-side provider: account provisioning, guardrails, nuke. |
| `examples/challenges` | Worked examples, and **the only writable path**. `s3-encryption` is the reference. |
| `docs/challenges/<provider>` | Challenge authoring guides, one folder per provider. |

## Rules that will bite you

- **Plugins are `GOOS=wasip1 GOARCH=wasm` only.** `pkg/challenge` and `pkg/challenge/aws`
  carry `//go:build wasip1`; they do not compile for your host. `go build ./...` failing on
  those packages is expected. Build plugins with
  `GOOS=wasip1 GOARCH=wasm go build ./path/to/plugin`, or just let `jamctl` do it.
- **Every resource field is a pointer.** Set them with the `new(x)` builtin — `new("my-bucket")`,
  `new(true)`, `new(s3.VersioningConfigurationStatusEnabled)`. There are no `aws.String`
  helpers and none should be added.
- **Properties with fixed values are typed constants**, not strings. Use
  `s3.VersioningConfigurationStatusEnabled`, not `new("Enabled")`.
- **A plugin's `main` never returns.** `Scenario.Start()` loops until the host stops it.
- **Cloud Control reads are not guaranteed complete**, and fakecloud and real AWS disagree in
  both directions. Never assume a property comes back — verify it. See
  [`docs/challenges/aws/validate.md`](docs/challenges/aws/validate.md).
- **IAM policy documents have a hard size limit: 6144 characters for a `SetGuardrail`
  (it becomes a managed policy used as permissions boundary), 10240 for a `SetPermission`
  (an inline role policy).** Expanding a service's generated action groups blows straight
  through it — `policy.ActionsFrom(ec2.ActionsRead, ec2.ActionsList, ec2.ActionsWrite)`
  alone is 25 908 characters. Write guardrails with service wildcards (`"ec2:*"`), keep the
  precise per-ARN lists for `SetPermission`, and measure with `len(doc.String())` before
  shipping. fakecloud does not enforce the quota, so a local run passes and the real account
  fails with `LimitExceeded: Cannot exceed quota for PolicySize`.

## Commands

```bash
# compile a plugin and run it against a throwaway fakecloud container (the default)
go run ./cmd/jamctl run aws ./examples/challenges/s3-encryption

# the same plugin against real credentials
go run ./cmd/jamctl run aws ./examples/challenges/s3-encryption --fake=false

# compile only
GOOS=wasip1 GOARCH=wasm go build -o challenge.wasm ./examples/challenges/s3-encryption

# regenerate the resource packages (network)
cd pkg/challenge/aws && go generate ./...
```

`jamctl` changes often. Run `go run ./cmd/jamctl --help` and
`go run ./cmd/jamctl run aws --help` to confirm the current commands and flags instead of
trusting any command line written down here — and if it does not compile, say so rather than
working around it. `cmd/jamctl` is platform code: you may run it, not fix it.

## Coding style

Code should be written in a human readable way:

- no comments unless absolutely necessary
- no useless dry abstractions for small code

For problems that require advanced helper please do not hesitate to recommend a functionality proposal (NOT CODE only the requirements) for the `pkg/challenge` sdk.
