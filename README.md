# Inventory Reservation System

A high-throughput inventory reservation service for flash-sale traffic, plus a React + Vite
mini-dashboard.

The Go + Gin backend reserves limited stock safely under concurrency — a single atomic conditional
`UPDATE` guarantees **no over-selling and no negative stock** — and unconfirmed reservations
**auto-expire after 5 minutes**, returning their stock to the pool. The dashboard shows live stock,
lets a user place a reservation, renders a live countdown, and confirms the purchase.

- Design rationale: [`RFC.md`](./RFC.md)
- Implementation blueprint: [`.agent/ARCHITECTURE.md`](./.agent/ARCHITECTURE.md)
- API contract: [`openapi.yaml`](./openapi.yaml)

## Stack

| Layer      | Technology                                   |
| ---------- | -------------------------------------------- |
| Backend    | Go 1.25 · Gin · pgx v5                        |
| Database   | PostgreSQL 16                                 |
| Frontend   | React 19 · Vite 8 · TypeScript                |
| Orchestration | Docker Compose                             |
| Tests      | `go test -race` (backend) · Vitest (frontend) |

## Prerequisites

- **Docker** and **Docker Compose** — to run the whole stack.
- **Go 1.25+** — only to run the backend outside Docker.
- **Node 22+** — only to run the frontend outside Docker.

## Quick start (whole stack)

One command brings up PostgreSQL, the backend, and the frontend:

```bash
docker compose up --build
```

| Service   | URL / Port                    | Notes                                  |
| --------- | ----------------------------- | -------------------------------------- |
| Frontend  | http://localhost:3000         | nginx-served SPA, proxies `/api`       |
| Backend   | http://localhost:8080         | Gin API (`/api/v1/inventory/*`)        |
| PostgreSQL| `localhost:5432`              | user/password/db: `indico`             |

The database is seeded on first boot with a single demo item, `item_4021` (_Flash Sale Widget_,
100 units), so you can use the dashboard immediately.

Stop the stack with `Ctrl+C`, or in the background:

```bash
docker compose up --build -d   # start detached
docker compose down            # stop and remove containers
docker compose down -v         # ...and delete the database volume (fresh reset)
```

> The migration in `backend/migrations/` is applied by PostgreSQL **only on first boot** of an empty
> data volume. If you change the schema, run `docker compose down -v` to re-initialise.

## Running the pieces individually

Useful for development with hot reload. Only PostgreSQL runs in Docker; the backend and frontend run
locally.

### 1. PostgreSQL (via Docker Compose)

```bash
docker compose up -d postgres
```

### 2. Backend (Gin)

```bash
cd backend
go run ./cmd/server
```

The defaults already point at the Compose PostgreSQL (`localhost:5432`), so no configuration is
needed. To override, export environment variables (the backend reads them from the process
environment):

```bash
export DATABASE_URL="postgres://indico:indico@localhost:5432/indico?sslmode=disable"
export PORT="8080"
go run ./cmd/server
```

`backend/.env.example` lists every variable. Note that Compose loads a `.env` file from the **repo
root** (copy `backend/.env.example` there) to override defaults; the Go binary itself does not read
`.env` files.

Health check: `curl http://localhost:8080/health` → `{"status":"ok"}`.

### 3. Frontend (React + Vite)

```bash
cd frontend
npm install
npm run dev
```

Open http://localhost:5173. The Vite dev server proxies `/api` and `/health` to the backend at
`http://localhost:8080` (override with `BACKEND_URL`). Proxying is required because the backend does
not send CORS headers — in production the nginx container proxies `/api` the same way.

## Configuration

| Variable                              | Default                                                              | Where                |
| ------------------------------------- | -------------------------------------------------------------------- | -------------------- |
| `PORT`                                | `8080`                                                               | backend              |
| `DATABASE_URL`                        | `postgres://indico:indico@localhost:5432/indico?sslmode=disable`     | backend              |
| `RESERVATION_TTL`                     | `5m`                                                                 | backend              |
| `SWEEP_INTERVAL`                      | `30s`                                                                | backend              |
| `DB_MAX_CONNS`                        | `10`                                                                 | backend              |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB` | `indico` / `indico` / `indico`                 | compose (postgres)   |
| `BACKEND_URL`                         | `http://backend:8080`                                                | compose (frontend)   |
| `VITE_API_BASE_URL`                   | _(empty → same-origin)_                                              | frontend build       |
| `FRONTEND_PORT`                       | `3000`                                                               | compose (host port)  |

## API

Base path `/api/v1/inventory`. Success responses carry `status: "success"`; every non-2xx returns a
single error envelope:

```json
{ "error": { "code": "INSUFFICIENT_STOCK", "message": "Only 3 units available, requested 5.",
             "details": { "item_id": "item_4021", "available": 3, "requested": 5 } } }
```

| Method | Path                             | Success | Error codes                                           |
| ------ | -------------------------------- | ------- | ----------------------------------------------------- |
| POST   | `/api/v1/inventory/reserve`      | `201`   | `INVALID_INPUT` (400), `ITEM_NOT_FOUND` (404), `INSUFFICIENT_STOCK` (409) |
| POST   | `/api/v1/inventory/confirm`      | `200`   | `RESERVATION_NOT_FOUND` (404), `RESERVATION_EXPIRED` (410), `ALREADY_CONFIRMED` (409) |
| GET    | `/api/v1/inventory/stock`        | `200`   | `INVALID_INPUT` (400), `ITEM_NOT_FOUND` (404)         |

```bash
# Check stock
curl "http://localhost:8080/api/v1/inventory/stock?item_id=item_4021"

# Reserve 2 units (5-minute TTL)
curl -X POST http://localhost:8080/api/v1/inventory/reserve \
  -H 'Content-Type: application/json' \
  -d '{"user_id":"usr_9981","item_id":"item_4021","quantity":2}'

# Confirm the reservation (use the reservation_id returned above)
curl -X POST http://localhost:8080/api/v1/inventory/confirm \
  -H 'Content-Type: application/json' \
  -d '{"reservation_id":"res_883291"}'
```

## Tests

Backend unit + race tests (no database required — the end-to-end suite is excluded by a build tag):

```bash
cd backend && go test -race -v ./...
```

Backend end-to-end API tests (opt-in; require the running stack):

```bash
docker compose up -d
cd backend && go test -tags=e2e -v ./e2e/...
```

The e2e suite targets `http://localhost:8080` and the Compose database by default; override with
`E2E_BASE_URL` and `E2E_DATABASE_URL`.

Frontend unit + integration tests (Vitest):

```bash
cd frontend && npm test
```

Frontend type-check/build and lint:

```bash
cd frontend && npm run build
cd frontend && npm run lint
```

## Project layout

```
.
├── RFC.md                  # design rationale (RFC-0001)
├── openapi.yaml            # API contract
├── docker-compose.yaml     # postgres + backend + frontend
├── backend/                # Go + Gin service (cmd, internal, migrations, Dockerfile)
└── frontend/               # React + Vite SPA (src, Dockerfile, nginx.conf.template)
```

## How it stays correct under load

- **Reserve** is one atomic `UPDATE ... WHERE (total_stock - reserved_stock) >= $qty` inside a
  transaction; an over-selling request matches **zero rows**, so stock can never go negative.
- **Confirm** flips `ACTIVE → CONFIRMED` guarded by `status = 'ACTIVE'`, making retries idempotent.
- **Expiry** is handled by a background sweeper (`FOR UPDATE SKIP LOCKED`) plus a lazy on-read check,
  so released stock is always consistent even between sweeps.
- The backend shuts down gracefully on `SIGINT`/`SIGTERM`, draining in-flight requests before
  stopping the sweeper and closing the pool.
