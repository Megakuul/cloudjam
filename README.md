# CloudJam

> [!CAUTION]  
> Project is currently under construction 🏗️

Platform to design, host and play CTF like challenges for DevOps guys.

## Deployment 🪖
---

There are two ways to host CloudJam, the system is abstracting the underlying database and storage layer via `go-cloud` sdk.

### hornet 🐝 

Single binary launcher with documentdb used for development and cheap hosting.

**Deployment**:

```bash
TBD
```

### orca 🫍

Serverless pulumi deployment using Lambda and DynamoDB used for cost effective scaling.

**Deployment**:

```bash
TBD
```

## Development 🚀
---

```bash
# starts documentdb (data is in './.database/data' certs in './.database/cert')
docker-compose up -d

# start the local sveltekit vite server
cd web && pnpm i && pnpm run dev

# in another terminal start hornet in dev mode
export DATABASE_SOURCE="mongodb://username:password@127.0.0.1:10260/?tls=true&tlsCAFile=.database/cert/cert.crt"
go run ./cmd/hornet -D --token-secret abcd

# use the default addr for development; hornet proxies the ui to vite so don't worry about that. 
xdg-open http://127.0.0.1:9000
```

## RBAC Concept 🏷️
---

CloudJam uses a **one-role-per-user** RBAC concept (a user cannot be attached to multiple roles at the same time). 
This avoids ambiguity with multisource- or transitive permissions.


The permission model is based on a *zerotrust* security concept with two layers:

1. **Data permissions**: (defines which resources can be accessed) implemented by having a "scope" on every resource. Database requests MUST be checked against the granted "scopes".
2. **Action permissions**: (defines which rpc actions can be invoked) implemented by validating the rpc procedure name against a list of basic glob patterns defined per scope in the role.

**To visualize 🔮**: 

Every rpc call goes through `map[scope][]actionExpr` returning all scopes with at least 1 matching actionExpr.
If at least 1 scope matches the action is executed with data permissions being restricted to the matched scopes.


> [!IMPORTANT]  
> There are two special builtin scopes that may be attached to a role:
>
> **ScopeSelf** ("self"): Allows access to user owned data regardless of resource scope (-> self management). 
>
> **ScopeAdmin** ("admin"): Allows privilege escalation in the `rbac.ConfigureRole` call (-> full root access).


To keep critical code together and reducing dangerous misconfigurations the `rbac` API isolates all operations which can escalate privileges (e.g. `AttachRole`). Other APIs like `user` and `role` only manage metadata and should never even be capable of escalating privileges.

> [!NOTE]  
> Action permissions are ENFORCED via middleware, data permissions are be implemented manually per call.


## Things to SDK 🧩

List of things that need to be programmed manually and can be abstracted eventually:

1. Data permissions enforcement (avoid manual "scope in auth.Scopes(ctx)" query check)
2. Automatically create logger with proper request information in rpc methods (could be done by transporting labels via ctx)
3. Mapping query fields typesafe in Query() db calls (evtl. derive from model struct).
4. `AttachScope` avoid manual mapping of "Resource"->Models (technically all data just has a scope field so...).
5. Simplify the iter database interface (usually I don't need to iterate, so iter is unnecessary boilerplate).
6. Log errors automatically via rpc middleware instead of manual logging.

> [!IMPORTANT]  
> Note that implementing those automations opens a can of worms.
> If something was simple I would've already done it.
