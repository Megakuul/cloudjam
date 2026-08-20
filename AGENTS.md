# AGENTS.md

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
| `examples/challenges` | Worked examples. `s3-encryption` is the reference. |
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
working around it.
