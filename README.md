# CloudJam

> [!IMPORTANT]  
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

# launches hornet in dev mode
export DATABASE_SOURCE="mongodb://username:password@127.0.0.1:10260/?tls=true&tlsCAFile=.database/cert/cert.crt"
go run ./cmd/hornet -D 

# in another terminal start vite for ui Development
cd web && pnpm i && pnpm run dev

# use the default addr for development; hornet proxies the ui to vite so don't worry about that. 
xdg-open http://127.0.0.1:9000
```
