# ARCHITECTURE.md — Implementation Blueprint

> **Audience:** the AI coding agent implementing this project.
> **Status:** authoritative. When this document and any other file disagree, follow this document and `openapi.yaml`.
> **Source of truth for design rationale:** `RFC.md` (RFC-0001). This file is the actionable, build-ready distillation plus the assignment's required written deliverable.

---

## 0. How To Use This Document

1. Read **§2 Non-Negotiable Constraints** first — violating any item means submission rejection.
2. Build to the **§4 repository layout**, **§5 schema**, and **§6 API contract** exactly.
3. The concurrency rules in **§7** are the single most important correctness requirement. Do not "simplify" them into read-then-write logic.
4. Track completion against **§15 Definition of Done** and follow **§16 Implementation Order**.
5. Stop and flag if a requirement is ambiguous; do not invent scope beyond §2–§12.

---

## 1. Project Summary

A high-throughput **inventory reservation** service for flash-sale traffic. A Go + Gin backend on a
**real PostgreSQL** database atomically reserves limited stock, confirms reservations, and reports
live stock. Unconfirmed reservations **auto-expire after 5 minutes** and their stock is released. A
React + Vite mini-dashboard drives the workflows and renders negative states.

- Backend weighting: **80%** — Go + Gin + PostgreSQL.
- Frontend weighting: **20%** — React + Vite only.
- Hard time budget: **4–8 hours**.

---

## 2. Non-Negotiable Constraints

| # | Constraint | Enforcement |
| - | ---------- | ----------- |
| C1 | Backend must be **Go + Gin** (`github.com/gin-gonic/gin`). | No other Go web framework/router. |
| C2 | Database must be a **real PostgreSQL** instance via **Docker Compose**. | **No** SQLite, in-memory maps, or mock stores. |
| C3 | Frontend must be **React + Vite** (JS or TS). | **No** Next.js, Nuxt, Vue, Svelte, Angular, plain HTML/JS. |
| C4 | **Single-command startup** via `docker compose up`. | Compose brings up PostgreSQL + backend (+ optional frontend). |
| C5 | **SQL migration / init scripts** committed. | Schema applied on first boot. |
| C6 | **Strict transaction handling**; no over-sell, no negative stock under concurrency. | See §7. |
| C7 | **Automatic expiry cleanup** returns expired holds to available stock. | See §8. |
| C8 | **Graceful shutdown** on SIGINT/SIGTERM; no dropped DB connections. | See §9. |
| C9 | **Unit + stress tests**, passing under `go test -race -v ./...`. | See §12. |
| C10 | **Incremental git history** across backend and frontend phases. | No single mega-commit. |
| C11 | **README.md** with run instructions. | postgres, backend, frontend. |

---

## 3. System Architecture

```
React + Vite SPA ──HTTP/JSON──▶ Gin API (Go) ──SQL──▶ PostgreSQL 16
   • InventoryTracker             • Router/middleware            • items
   • ReservationForm              • Handlers (HTTP)              • reservations
   • ActiveReservationCard        • Service (txn orchestration)
   • 5-min countdown              • Repository (pgx)
                                  • Expiry Sweeper (goroutine)
```

Layering (backend): **router → middleware → handlers → service → repository → PostgreSQL**.
Responsibilities are strict: handlers parse/validate and shape HTTP responses; the service owns
transactions and business rules; the repository owns SQL. No SQL in handlers; no HTTP types in the
repository.

---

## 4. Target Repository Layout

```
.
├── .agent/
│   └── ARCHITECTURE.md            # this file
├── RFC.md
├── openapi.yaml                   # API contract (source of truth for endpoints)
├── docker-compose.yaml
├── .env.example
├── README.md
├── backend/
│   ├── cmd/server/main.go         # wiring, graceful shutdown
│   ├── internal/
│   │   ├── config/config.go       # env loading (DSN, port, TTL, sweep interval)
│   │   ├── httpapi/
│   │   │   ├── router.go          # route registration
│   │   │   ├── middleware.go      # logging, recovery, error mapping
│   │   │   └── handlers.go        # reserve/confirm/stock handlers
│   │   ├── service/
│   │   │   └── inventory.go       # Reserve, Confirm, GetStock (transactions)
│   │   ├── repository/
│   │   │   └── postgres.go        # SQL via pgx; no business logic
│   │   ├── domain/
│   │   │   ├── models.go          # Reservation, Item, errors
│   │   │   └── errors.go          # typed sentinel errors (see §6.4)
│   │   └── sweeper/sweeper.go     # time.Ticker expiry loop
│   ├── migrations/0001_init.sql
│   ├── Dockerfile                 # multi-stage build
│   └── go.mod
├── frontend/
│   ├── src/
│   │   ├── api/client.ts          # fetch wrapper, envelope unwrapping
│   │   ├── components/InventoryTracker.tsx
│   │   ├── components/ReservationForm.tsx
│   │   ├── components/ActiveReservationCard.tsx
│   │   ├── hooks/useCountdown.ts
│   │   ├── hooks/usePolling.ts
│   │   ├── types.ts
│   │   ├── App.tsx
│   │   └── main.tsx
│   ├── Dockerfile                 # build + serve static
│   ├── package.json
│   └── vite.config.ts
└── ARCHITECTURE.md                # assignment submission copy (1–2 pages) — see §17
```

---

## 5. Data Model (PostgreSQL)

```sql
-- backend/migrations/0001_init.sql
CREATE TABLE items (
    item_id        TEXT PRIMARY KEY,
    name           TEXT        NOT NULL,
    total_stock    INTEGER     NOT NULL CHECK (total_stock >= 0),
    reserved_stock INTEGER     NOT NULL DEFAULT 0 CHECK (reserved_stock >= 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT reserved_not_exceed_total CHECK (reserved_stock <= total_stock)
);

CREATE TYPE reservation_status AS ENUM ('ACTIVE', 'CONFIRMED', 'EXPIRED');

CREATE TABLE reservations (
    reservation_id TEXT PRIMARY KEY,
    user_id        TEXT        NOT NULL,
    item_id        TEXT        NOT NULL REFERENCES items(item_id),
    quantity       INTEGER     NOT NULL CHECK (quantity > 0),
    status         reservation_status NOT NULL DEFAULT 'ACTIVE',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ NOT NULL,
    confirmed_at   TIMESTAMPTZ
);

CREATE INDEX idx_reservations_due
    ON reservations (expires_at) WHERE status = 'ACTIVE';
CREATE INDEX idx_reservations_item_status
    ON reservations (item_id, status);

-- Seed for demos/tests
INSERT INTO items (item_id, name, total_stock) VALUES ('item_4021', 'Flash Sale Widget', 100);
```

Rules the agent must preserve:

- `available_stock` is **derived** (`total_stock - reserved_stock`); never store it.
- Confirming decrements **both** `total_stock` and `reserved_stock` (committed units leave the pool).
- The `CHECK (reserved_stock <= total_stock)` is a required backstop — keep it.

---

## 6. API Contract

The machine-readable contract is `openapi.yaml` (OpenAPI **3.0.3**). Implement it exactly. Base path
`/api/v1/inventory`.

### 6.1 Endpoints

| Method | Path | Success | Purpose |
| ------ | ---- | ------- | ------- |
| POST | `/api/v1/inventory/reserve` | `201` | Atomically reserve stock (5-min TTL). |
| POST | `/api/v1/inventory/confirm` | `200` | Commit an active reservation. |
| GET  | `/api/v1/inventory/stock` | `200` | Real-time stock breakdown (`?item_id=`). |

### 6.2 Payloads

```jsonc
// POST /reserve  request
{ "user_id": "usr_9981", "item_id": "item_4021", "quantity": 2 }
// POST /reserve  201
{ "status": "success", "reservation_id": "res_883291", "item_id": "item_4021",
  "quantity": 2, "expires_at": "2026-07-20T16:35:00Z" }

// POST /confirm  request
{ "reservation_id": "res_883291" }
// POST /confirm  200
{ "status": "success", "reservation_id": "res_883291", "confirmed_at": "2026-07-20T16:32:00Z" }

// GET /stock?item_id=item_4021  200
{ "item_id": "item_4021", "total_stock": 100, "reserved_stock": 15, "available_stock": 85 }
```

### 6.3 Error Envelope (mandatory, identical on every non-2xx)

```json
{ "error": { "code": "INSUFFICIENT_STOCK",
             "message": "Only 3 units available, requested 5.",
             "details": { "item_id": "item_4021", "available": 3, "requested": 5 } } }
```

`code` ∈ `INVALID_INPUT | ITEM_NOT_FOUND | INSUFFICIENT_STOCK | RESERVATION_NOT_FOUND |
RESERVATION_EXPIRED | ALREADY_CONFIRMED | INTERNAL_ERROR`. The frontend switches on `code`.

### 6.4 Status Code Mapping

| Scenario | Endpoint | HTTP | `error.code` |
| -------- | -------- | ---- | ------------ |
| Created | reserve | 201 | — |
| Invalid/missing fields, `quantity <= 0` | reserve | 400 | `INVALID_INPUT` |
| Unknown `item_id` | reserve | 404 | `ITEM_NOT_FOUND` |
| Insufficient available stock | reserve | 409 | `INSUFFICIENT_STOCK` |
| Confirmed | confirm | 200 | — |
| Unknown reservation | confirm | 404 | `RESERVATION_NOT_FOUND` |
| Expired reservation | confirm | 410 | `RESERVATION_EXPIRED` |
| Already confirmed | confirm | 409 | `ALREADY_CONFIRMED` |
| Stock returned | stock | 200 | — |
| Unknown item | stock | 404 | `ITEM_NOT_FOUND` |
| Missing/invalid `item_id` | stock | 400 | `INVALID_INPUT` |
| Unhandled fault | any | 500 | `INTERNAL_ERROR` |

Implement typed sentinel errors in `domain/errors.go` and map them to `(status, code)` in one place
(middleware/error mapper) so every handler returns the envelope consistently.

---

## 7. Concurrency & Locking Strategy  *(assignment prompt 1)*

**Rule: reserve is a single atomic conditional `UPDATE`; never read-then-write.**

```sql
BEGIN;

UPDATE items
   SET reserved_stock = reserved_stock + $1, updated_at = now()
 WHERE item_id = $2
   AND (total_stock - reserved_stock) >= $1;
-- rows affected: 0 => not enough stock (or missing item); 1 => ok

INSERT INTO reservations
  (reservation_id, user_id, item_id, quantity, status, expires_at)
VALUES ($3, $4, $5, $6, 'ACTIVE', now() + interval '5 minutes');

COMMIT;
```

- **Why safe:** PostgreSQL row-level locking serializes writers on the same `items` row and
  re-evaluates the `WHERE` predicate against the latest committed version, so an over-selling request
  matches **0 rows**. The increment and the reservation insert commit atomically or not at all.
- **Isolation:** default `READ COMMITTED` is sufficient; **do not** use `SERIALIZABLE`.
- **Disambiguate 0 rows:** run an existence check to return `ITEM_NOT_FOUND` (404) vs
  `INSUFFICIENT_STOCK` (409).
- **Deadlocks:** each reservation touches one item row — keep it that way. If multi-item support is
  ever added, lock rows in deterministic `item_id` order.

**Confirm (idempotent):**

```sql
BEGIN;
UPDATE reservations
   SET status = 'CONFIRMED', confirmed_at = now()
 WHERE reservation_id = $1 AND status = 'ACTIVE'
RETURNING item_id, quantity;   -- 0 rows => inspect status: EXPIRED(410)/ALREADY_CONFIRMED(409)/404

UPDATE items
   SET total_stock = total_stock - $2, reserved_stock = reserved_stock - $2, updated_at = now()
 WHERE item_id = $3;
COMMIT;
```

The `status = 'ACTIVE'` guard makes concurrent/repeat confirms safe — only the first wins.

---

## 8. Reservation Lifecycle & Expiry Cleanup

States: `ACTIVE → CONFIRMED | EXPIRED` (strictly linear).

- **Reserve:** available → reserved.
- **Confirm:** reserved → committed (`total_stock` and `reserved_stock` both decrement).
- **Expire:** reserved → available (`reserved_stock` decrements).

Expiry uses **two** mechanisms:

1. **Sweeper (primary):** a `time.Ticker` goroutine (e.g., every 30s) runs, in one transaction:
   ```sql
   WITH due AS (
       SELECT reservation_id, item_id, quantity FROM reservations
        WHERE status = 'ACTIVE' AND expires_at <= now()
        FOR UPDATE SKIP LOCKED
   )
   UPDATE reservations r SET status = 'EXPIRED'
     FROM due WHERE r.reservation_id = due.reservation_id;
   -- then UPDATE items ... reserved_stock = reserved_stock - due.quantity FROM due ...
   ```
   `SKIP LOCKED` keeps it safe under overlapping/multi-replica sweeps.
2. **Lazy check (safety net):** reserve/confirm treat `expires_at <= now()` as expired even before a
   sweep, so there is no visible gap between ticks. Confirm on an expired row → `410`.

---

## 9. Graceful Shutdown

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()
srv := &http.Server{Addr: ":" + cfg.Port, Handler: router}
go func() { _ = srv.ListenAndServe() }()
<-ctx.Done()
stop()
shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
_ = srv.Shutdown(shutdownCtx)   // drain in-flight requests
sweeperCancel()                 // stop ticker
db.Close()                      // close pgx pool
```

Order matters: drain HTTP **before** stopping the sweeper and closing the pool, so no write is cut
mid-transaction and no connection leaks.

---

## 10. Frontend Architecture (React + Vite)

- **`api/client.ts`** — one `fetch` wrapper: sets JSON headers, unwraps the success/error envelope,
  throws a typed `ApiError` carrying `code`/`details`.
- **`InventoryTracker`** — total/reserved/available via `GET /stock`; polls every ~3s via
  `usePolling` and offers a manual **Refresh**; discards stale responses (generation guard).
- **`ReservationForm`** — controlled `user_id` + `quantity`; `POST /reserve`; disables while in
  flight; renders the error envelope inline on non-2xx.
- **`ActiveReservationCard`** — shows the active reservation and a **live 5-minute countdown** from
  `expires_at` (`useCountdown`, tick 1s, `mm:ss`); at zero → Expired state, disable **Confirm
  Purchase**.
- **Confirm flow** — `POST /confirm`; on success re-poll stock; on `410`/`409`/`404` show the mapped
  message from `code`.

Local component state only (no state library). Scaffold with `npm create vite@latest` (React + TS).

---

## 11. Docker & Configuration

`docker-compose.yaml`:

- **postgres** — `postgres:16`, named volume, healthcheck `pg_isready`; mount `./backend/migrations`
  to `/docker-entrypoint-initdb.d`.
- **backend** — built from `backend/Dockerfile` (multi-stage); `depends_on: postgres: {condition:
  service_healthy}`.
- **frontend** — optional; built from `frontend/Dockerfile`; API base URL via build arg / env.

`.env.example`: `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `DATABASE_URL`, `PORT`,
`RESERVATION_TTL=5m`, `SWEEP_INTERVAL=30s`. Defaults must let `docker compose up` work with no edits.

---

## 12. Testing Strategy

- **Unit (table-driven):** reserve success, insufficient stock, `quantity <= 0`, unknown item, confirm
  success, confirm expired, confirm already-confirmed, stock derivation.
- **Stress/race:** ~200 goroutines each reserve 1 unit of an item seeded with ~50; assert
  `successes == 50`, `reserved_stock <= total_stock`, never negative. Add a reserve+expiry scenario to
  verify release accounting.
- **Gate:** `go test -race -v ./...` must pass cleanly. Repository tests run against the Compose
  PostgreSQL (or testcontainers).

---

## 13. Distributed Scaling & Failure Modes  *(assignment prompt 2)*

Scaling to 10 stateless Gin instances against one PostgreSQL:

1. **Duplicate sweepers** → double-release risk. Mitigate with `FOR UPDATE SKIP LOCKED` (already in
   §8) and/or elect a **single leader** via `pg_advisory_lock` or a lease row.
2. **Connection exhaustion** → bound each pool (`SetMaxOpenConns`) and front PostgreSQL with
   **PgBouncer** (transaction pooling).
3. **Idempotency** → the `status = 'ACTIVE'` confirm guard already handles LB/client retries.
4. **Read pressure** → route `GET /stock` to **read replicas** (accept slight lag); writes to primary.
5. **Single-primary ceiling** → shard by `item_id` (preserves the single-row atomic update) or move
   the hot counter to Redis `DECRBY` with durable reconciliation back to PostgreSQL.

The service is already **stateless** (all shared state lives in PostgreSQL), so instances can be
scaled/replaced freely behind a load balancer once the sweeper is single-writer and pools are bounded.

---

## 14. Engineering Trade-offs & AI Transparency  *(assignment prompt 3)*

- **Go:** prioritized correctness (atomic UPDATE, sweeper, graceful shutdown, race tests) over extra
  endpoints. Denormalized counters trade storage redundancy for a one-statement atomic reservation.
- **React:** local state + polling over a data library — simpler, fewer deps; cost is slightly stale
  data between ticks.
- **Expiry:** app-level ticker over `pg_cron` — testable in Go, portable; cost is single-writer
  bookkeeping when scaled out.

**AI transparency (illustrative, must be replaced with a real experience):** a plausible AI-generated
reserve handler reads the row, checks `available >= qty` in Go, then issues a separate `UPDATE` — a
read-modify-write race that over-sells under concurrency. Fixed by moving the guard into the `UPDATE`
`WHERE` clause (`AND (total_stock - reserved_stock) >= $1`) and branching on rows-affected. The
`reserved_stock <= total_stock` CHECK and the concurrency test were added as guards.

---

## 15. Definition of Done

- [ ] `docker compose up` starts PostgreSQL + backend (and frontend) with no manual steps.
- [ ] All three endpoints match `openapi.yaml`; success + every negative case returns the envelope.
- [ ] Concurrency stress test proves **no over-sell** and **no negative stock**.
- [ ] Expired reservations release stock automatically (sweeper + lazy check).
- [ ] Server shuts down gracefully on SIGINT/SIGTERM without dropping connections.
- [ ] `go test -race -v ./...` passes.
- [ ] React dashboard: live tracker, reservation form, 5-min countdown, confirm button, error states.
- [ ] `README.md` documents setup; `ARCHITECTURE.md` addresses the three prompts.
- [ ] Git history is incremental across backend and frontend phases.

---

## 16. Implementation Order (for the agent)

1. Scaffold repo, `docker-compose.yaml`, `.env.example`, `backend/go.mod`, migration SQL.
2. Domain models + sentinel errors; repository (SQL) with generated IDs.
3. Service: reserve/confirm/getStock transactions (§7) with unit tests.
4. Gin router + handlers + error-mapping middleware (§6).
5. Expiry sweeper (§8) + graceful shutdown wiring in `main.go` (§9).
6. Stress/race test (§12); run `go test -race -v ./...`.
7. Scaffold Vite React app; `api/client.ts`, components, hooks (§10).
8. Wire frontend to backend; verify all three workflows end-to-end.
9. Write `README.md`; finalize `ARCHITECTURE.md`.
10. Commit incrementally at each phase boundary.

---

## 17. Note on the Submission Copy

The assignment requires a 1–2 page `ARCHITECTURE.md` covering the three prompts. **§7, §13, and §14
above are written to satisfy those prompts directly**; when preparing the submission copy at the repo
root, condense this document to those three sections plus a short system overview and the schema.
This `.agent/ARCHITECTURE.md` remains the full implementation blueprint for the agent.
