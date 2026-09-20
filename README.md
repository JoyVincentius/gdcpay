# gdcpay — Task Management API

Multi-user Task Management REST API built with **Go** and **Gin**, backed by **PostgreSQL** running in **Docker**. Each user manages their own tasks and can assign tasks to teammates within the same team.

Built for the Back End Developer technical test.

## Tech Stack

| Concern            | Choice                                    |
|---------------------|--------------------------------------------|
| Language             | Go 1.22+                                   |
| HTTP framework       | [Gin](https://github.com/gin-gonic/gin)    |
| Database             | PostgreSQL 16 (Docker)                     |
| Migrations           | [golang-migrate](https://github.com/golang-migrate/migrate) |
| Auth                 | JWT (access token)                         |
| Logging              | structured JSON (e.g. `zerolog`/`zap`)     |
| Containerization     | Docker + Docker Compose                    |

## Project Status

All items in [Requirements Coverage](#requirements-coverage) are implemented and verified against a live Postgres instance (see each section below for how).

## Architecture Overview

The service follows a simplified **clean / layered architecture** to keep HTTP concerns, business logic, and persistence decoupled and independently testable:

```
Handler (Gin)  →  Service (business logic)  →  Repository (Postgres)
     ↑                     ↑
 Middleware          Domain contracts
 (auth, logging,      (interfaces)
  error handler,
  idempotency)
```

- **`handler`** — parses HTTP requests, validates input, calls services, shapes responses. No business logic.
- **`service`** — business rules: ownership checks, idempotency handling, transactional assign flow.
- **`repository`** — SQL queries via `database/sql`/`pgx`, one implementation per domain entity, behind interfaces defined in `domain` so services can be unit-tested with mocks/stubs (no live DB needed).
- **`middleware`** — JWT auth, structured request logging, global panic/error recovery, idempotency key handling.
- **`domain`** — entities and repository/service interfaces, framework-agnostic.

Layout:

```
gdcpay/
├── cmd/
│   └── api/                    # main.go — wiring, DI, server bootstrap
├── internal/
│   ├── domain/                 # entities + interfaces (User, Team, Task, TaskLog,
│   │                           #   TxManager, Notifier, IdempotencyStore...)
│   ├── handler/                 # Gin HTTP handlers
│   ├── service/                  # business logic (auth, task, idempotency, assign)
│   ├── repository/
│   │   ├── postgres/               # database/sql + pgx implementations
│   │   └── memory/                  # in-memory IdempotencyStore used by unit tests
│   ├── notification/                # mock/log Notifier
│   ├── middleware/                   # auth, logging, recovery
│   ├── pkg/
│   │   ├── jwtutil/                    # JWT sign/verify
│   │   └── response/                    # structured success/error JSON helpers
│   ├── dto/                              # request/response payloads
│   └── config/                            # env config loading
├── migrations/                              # SQL migration files (golang-migrate)
├── docker-compose.yml                         # Postgres + migrate + api
├── Dockerfile
├── .env.example
├── go.mod
└── README.md
```

## Database Design

- `teams` — `id, name, created_at`
- `users` — `id, team_id, name, email (unique), password_hash, created_at`
- `tasks` — `id, owner_id, assignee_id, title, description, status, created_at, updated_at`
- `task_logs` — `id, task_id, changed_by, old_assignee_id, new_assignee_id, action, created_at` (audit trail written inside the assign transaction)
- `idempotency_keys` — `key (UUID, PK), user_id, request_hash, response_status, response_body, created_at, expires_at` (24h TTL, unique constraint on `key` enforces at-most-once creation under concurrent requests)

## Getting Started

### Prerequisites
- Go 1.22+
- Docker & Docker Compose

### 1. Configure environment

```bash
cp .env.example .env
```

| Variable        | Description                          |
|------------------|----------------------------------------|
| `APP_PORT`         | HTTP port the API listens on (default `8080`) |
| `DATABASE_URL`      | Postgres DSN, e.g. `postgres://gdcpay:gdcpay@localhost:5401/gdcpay?sslmode=disable` (host port `5401` maps to Postgres in Docker) |
| `JWT_SECRET`         | Secret used to sign JWT access tokens |
| `JWT_EXPIRES_IN`      | Access token TTL, e.g. `24h`          |
| `IDEMPOTENCY_TTL`      | Idempotency key window, default `24h` |

### 2. Start the database (Docker)

```bash
docker compose up -d db
```

### 3. Run migrations

```bash
docker compose run --rm migrate    # or: make migrate-up
```

### 4. Run the API

```bash
go run ./cmd/api
```

The API will be available at `http://localhost:8080`.

### Run everything with Docker Compose

```bash
docker compose up --build
```

This brings up Postgres, runs migrations, and starts the API container.

### Run tests

```bash
go test ./... -race -v
```

Unit tests run without a live database or external services (in-memory/mock repositories, see [internal/service](internal/service)). They cover the mandatory idempotency race conditions (sequential + concurrent duplicate keys) plus, as a bonus, the assign flow's authorization rules and its transactional rollback-on-failure guarantee. The `-race` flag is required to validate the concurrency tests.

## API Endpoints

### Auth

| Method | Path             | Description         |
|--------|-------------------|-----------------------|
| POST    | `/auth/register`    | Register a new user     |
| POST    | `/auth/login`        | Login, returns JWT access token |

### Tasks
_All routes below require `Authorization: Bearer <token>`._

| Method | Path                  | Description                          |
|--------|------------------------|-----------------------------------------|
| POST    | `/tasks`                  | Create a task. Requires `Idempotency-Key` header (UUID) |
| GET     | `/tasks`                    | List tasks. Supports `?status=`, `?search=`, `?page=`, `?limit=` |
| GET     | `/tasks/:id`                  | Get task detail |
| PUT     | `/tasks/:id`                    | Update a task |
| DELETE  | `/tasks/:id`                      | Delete a task |
| POST    | `/tasks/:id/assign`                 | Assign task to another user in the same team (transactional, owner-only) |

### Error Response Format

All errors return a consistent JSON shape:

```json
{
  "status": 400,
  "code": "VALIDATION_ERROR",
  "message": "title is required",
  "timestamp": "2026-09-18T10:00:00Z"
}
```

4xx errors are treated as client errors, 5xx as server errors; internal details/stack traces are never exposed in responses. A global recovery middleware converts panics into a structured 500 response.

### Task Visibility & Ownership

- `GET /tasks` and `GET /tasks/:id` are scoped to tasks the caller **owns or is assigned to**.
- `PUT /tasks/:id` and `DELETE /tasks/:id` are **owner-only**; an assignee can view a task but not modify or delete it.
- A task that exists but belongs to someone unrelated returns the same `404 TASK_NOT_FOUND` as a missing task, so existence is never leaked to other users.

### Idempotency

`POST /tasks` **requires** an `Idempotency-Key: <uuid>` header (missing or malformed → `400 VALIDATION_ERROR`):
- First request with a new key creates the task (`201`) and stores the response byte-for-byte.
- A repeated request with the same key within the 24h window (`IDEMPOTENCY_TTL`) replays the exact original response — same status, same body — without creating a duplicate task.
- Reusing a key with a **different** request body returns `409 IDEMPOTENCY_KEY_CONFLICT`.
- After the 24h window expires, the key is recycled and a new request with it creates a new task.
- **Concurrency-safe by construction, not by locking in application code**: the key is the primary key of an `idempotency_keys` table. The "winning" request's `INSERT ... ON CONFLICT (key) DO NOTHING` runs inside a transaction; any concurrent request for the same key blocks on Postgres's own row-level lock until the winner's transaction commits or rolls back, then reads the committed result via `SELECT ... FOR UPDATE` instead of racing ahead. If the winner's work fails, its transaction rolls back and the key is freed for a genuine retry. See [internal/repository/postgres/idempotency_store.go](internal/repository/postgres/idempotency_store.go).
- An in-memory implementation of the same `domain.IdempotencyStore` interface ([internal/repository/memory/idempotency_store.go](internal/repository/memory/idempotency_store.go)) backs the unit tests below, so the concurrency guarantee is verified without a database.

### Task Assignment (Transactional)

`POST /tasks/:id/assign` (owner-only, body: `{"assignee_id": "<uuid>"}`) does three things as one atomic unit of work:
1. Updates `tasks.assignee_id` (and `updated_at`).
2. Writes an audit row to `task_logs` (`changed_by`, `old_assignee_id`, `new_assignee_id`, `action`).
3. Sends a (mocked/logged) notification via `domain.Notifier`.

The assignee must belong to the caller's team (`422 ASSIGNEE_NOT_IN_TEAM` otherwise). All three steps run inside one `domain.TxManager.WithinTransaction` call ([internal/repository/postgres/tx.go](internal/repository/postgres/tx.go)); repositories pick up the ambient `*sql.Tx` from the request context instead of the caller threading it through. If **any** step fails — including the notification — the whole transaction rolls back, so the assignee change and the audit log never persist partially. This was verified against the real database by temporarily forcing the notifier to fail: the task's `assignee_id` and `task_logs` were confirmed unchanged afterward.

The task row is read with `SELECT ... FOR UPDATE` inside the transaction, so concurrent assign requests on the same task serialize instead of racing.

### Logging

Every request produces a structured JSON log line with `request_id` (UUID), `method`, `path`, `status_code`, and `latency`. Log level is `INFO` for 2xx/3xx, `WARN` for 4xx, `ERROR` for 5xx.

## Requirements Coverage

- [x] Register / Login / JWT auth
- [x] Task CRUD endpoints
- [x] Filter by status, search by title, pagination
- [x] Idempotency key on `POST /tasks` (header, 24h window, concurrency-safe)
- [x] Structured error handling + global panic recovery
- [x] Transactional `POST /tasks/:id/assign` (update assignee + task_logs + mock notification, with rollback on failure)
- [x] Structured JSON request logging
- [x] Unit tests for idempotency race conditions (sequential + concurrent), run with mocked dependencies

## License

Private technical test submission.
