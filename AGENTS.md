# AGENTS.md — LLMPool Bootstrap Playbook (Phase 1)

This file defines how agents and developers initialize this repository from scratch.

## 1) Scope (Phase 1)

Build a **backend-first** Go service with:
- Gin HTTP API
- `GET /health` endpoint
- Zap logger
- Viper config (YAML defaults + env overrides)
- Clean Architecture folder boundaries

Frontend is minimal and out of scope for initialization.

---

## 2) Required Stack

- Go `1.22+`
- Gin
- Zap
- Viper
- Docker + Docker Compose
- PostgreSQL + Redis (as dependencies)

---

## 3) Clean Architecture Rules (Non-negotiable)

1. **Dependency direction**: `delivery -> usecase -> domain` only.
2. Domain layer must not import framework packages (Gin, DB drivers, Redis clients).
3. Infrastructure implements interfaces declared by usecase/domain.
4. Handlers contain no business logic.
5. Config and logger are initialized before router setup.

---

## 4) Target Repository Structure

Create this structure first:

```text
.
├── cmd/
│   └── api/
│       └── main.go
├── configs/
│   └── default.yml
├── internal/
│   ├── domain/
│   │   └── health/
│   │       └── entity.go
│   ├── usecase/
│   │   └── health/
│   │       ├── interface.go
│   │       └── service.go
│   ├── delivery/
│   │   └── http/
│   │       ├── handler/
│   │       │   └── health_handler.go
│   │       └── router.go
│   ├── infra/
│   │   ├── config/
│   │   │   └── config.go
│   │   ├── logger/
│   │   │   └── zap.go
│   │   ├── postgres/
│   │   │   └── client.go
│   │   └── redis/
│   │       └── client.go
│   └── platform/
│       └── server/
│           └── http_server.go
├── pkg/
│   └── response/
│       └── response.go
├── docker-compose.yml
├── Dockerfile
├── Makefile
└── go.mod
```

---

## 5) Initialization Commands (Run in order)

> Replace module path with your actual repository path.

```bash
go mod init github.com/your-org/llmpool
go get github.com/gin-gonic/gin
go get github.com/spf13/viper
go get go.uber.org/zap
go get github.com/redis/go-redis/v9
go get github.com/jackc/pgx/v5
go mod tidy
```

---

## 6) Configuration Contract (Viper)

## 6.1 Defaults file
Path: `configs/default.yml`

```yaml
app:
  name: llmpool
  env: dev

server:
  host: 0.0.0.0
  port: 8080
  read_timeout: 10s
  write_timeout: 0s
  idle_timeout: 30s

log:
  level: info
  development: true

postgres:
  dsn: postgres://postgres:postgres@postgres:5432/llmpool?sslmode=disable

redis:
  addr: redis:6379
  password: ""
  db: 0

orchestrator:
  lb_strategy: round-robin
```

## 6.2 Precedence (must follow)
1. Defaults (`viper.SetDefault`)
2. YAML file (`configs/default.yml`)
3. Environment overrides (`LLMPOOL_*`)

Use env replacer:
- `.` -> `_`
- Example: `LLMPOOL_SERVER_PORT=9090`

---

## 7) Logger Contract (Zap)

- Initialize logger **after** config load.
- `development=true` -> development logger; otherwise production logger.
- Parse level from config (`debug|info|warn|error`).
- Always `defer logger.Sync()` in `main`.

---

## 8) HTTP Contract (Minimum)

### `GET /health`
- Status: `200`
- Response JSON:

```json
{"status":"ok"}
```

No dependency checks in this endpoint (liveness only).

---

## 9) Docker Contracts

## 9.1 docker-compose.yml
Must include services:
- `app`
- `postgres`
- `redis`

## 9.2 Required app env
- `LLMPOOL_SERVER_PORT`
- `LLMPOOL_POSTGRES_DSN`
- `LLMPOOL_REDIS_ADDR`
- `LLMPOOL_ORCHESTRATOR_LB_STRATEGY` (`round-robin` or `fill-first`)

---

## 10) Makefile Targets (minimum)

- `make run` — run API locally
- `make build` — build binary
- `make test` — run tests
- `make lint` — run linter
- `make up` — docker compose up
- `make down` — docker compose down

---

## 11) Done Criteria for Initialization

Initialization is complete only if all are true:
1. `go run ./cmd/api` starts successfully.
2. `GET /health` returns `200 {"status":"ok"}`.
3. Config values can be overridden by env vars.
4. Zap logs are visible on startup.
5. `docker compose up` starts app + postgres + redis.

---

## 12) Mandatory Verification Workflow

After any code change, always run:

```bash
make test
make lint
```

Linting uses `golangci-lint`. Install locally if missing:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
```

If test or lint fails, fix the issue before marking work complete.

---

## 13) Anti-patterns to Avoid

- Do not put business logic inside Gin handlers.
- Do not read env variables directly from handlers/usecases.
- Do not import infrastructure packages in domain layer.
- Do not hardcode secrets in YAML committed to git.
- Do not skip graceful shutdown for HTTP server.
