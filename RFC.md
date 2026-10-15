# RFC-0001: High-Throughput Inventory Reservation System & Mini Dashboard

| Field            | Value                                                                 |
| ---------------- | --------------------------------------------------------------------- |
| **RFC Number**   | RFC-0001                                                              |
| **Title**        | High-Throughput Inventory Reservation System & Mini Dashboard         |
| **Status**       | Draft                                                                 |
| **Author**       | Full Stack Engineer (Take-Home Submission)                            |
| **Date**         | 2026-10-15                                                            |
| **Target Level** | Mid–Senior Full Stack                                                 |
| **Stack**        | Go + Gin + PostgreSQL (backend, 80%) · React + Vite (frontend, 20%)    |
| **Time Budget**  | 4–8 hours                                                             |

---

## 1. Summary

This RFC proposes the design of a reservation service that safely reserves limited product
inventory for concurrent users during flash-sale traffic. The backend is a Go service built on the
Gin HTTP framework and backed by a **real PostgreSQL** instance running under Docker Compose. It
exposes three REST endpoints — reserve, confirm, and stock lookup — and guarantees that, under
heavy concurrency, **stock is never over-sold and never goes negative**. Reservations held for more
than **five minutes** without confirmation are automatically released back to available stock. The
system shuts down gracefully, ships with unit and race-tested stress tests, and is paired with a
React + Vite mini-dashboard that surfaces live stock, lets a user place a reservation, shows a
live countdown, and renders negative backend states clearly.

---

## 2. Motivation & Context

Flash sales concentrate enormous demand into a very short window. Thousands of users issue
"reserve" requests for the same high-demand item within milliseconds of each other. A naive
implementation — read the current stock, check it in application code, then write the new value —
is a classic **read-modify-write race**: two requests both read `available = 1`, both believe a
reservation is possible, and both write, leaving the item over-sold and the stock counter
inconsistent.

The core engineering problem is therefore not CRUD; it is **concurrency control over a shared,
contended resource**. The system must be:

- **Correct under concurrency** — no over-selling, no negative allocation, no lost updates.
- **Resilient in time** — unconfirmed holds must not permanently lock inventory; they expire.
- **Operationally clean** — graceful shutdown, no dropped database connections, runnable with a
  single `docker compose up`.

This RFC defines the schema, the transaction and locking strategy, the API contract, the expiry
mechanism, and the frontend behavior that together satisfy these constraints.

---

## 3. Goals

- Atomically reserve inventory only when sufficient **available** stock exists.
- Confirm a reservation, permanently committing it and decrementing physical inventory exactly once.
- Automatically release expired (5-minute TTL) holds back to available stock.
- Provide a live, consistent stock breakdown (total / reserved / available).
- Guarantee no over-sell and no negative stock under concurrent load.
- Run the full stack (PostgreSQL + backend [+ frontend]) with one command.
- Pass `go test -race -v ./...` with unit and concurrency/stress tests.
- Shut down gracefully on SIGINT/SIGTERM without corrupting state.

## 4. Non-Goals

- Authentication/authorization of end users (out of scope for the time budget; `user_id` is a
  trusted client-supplied identifier for this exercise).
- Payment processing or order fulfilment beyond inventory commitment.
- Multi-region or multi-primary database topologies.
- A production-grade distributed scheduler for expiry (a single-writer app-level sweeper is
  sufficient here; see §9).
- Rich UI design system; the dashboard is functional, not a polished design deliverable.

---

## 5. Proposed Design

### 5.1 System Architecture

```
                         ┌──────────────────────────────┐
                         │      React + Vite SPA        │
                         │  ┌────────────────────────┐  │
                         │  │ InventoryTracker       │  │  poll GET /stock
                         │  │ ReservationForm        │  │  POST /reserve
                         │  │ ActiveReservationCard  │  │  POST /confirm
                         │  │  + 5-min countdown     │  │
                         │  └────────────────────────┘  │
                         └───────────────┬──────────────┘
                                         │ HTTP / JSON
                                         ▼
                    ┌────────────────────────────────────────┐
                    │            Gin API (Go)                │
                    │  Router ─ Middleware ─ Handlers        │
                    │  Service layer (txn orchestration)     │
                    │  Repository (pgx / database/sql)       │
                    │                                        │
                    │  Background: Expiry Sweeper (ticker)   │
                    └───────────────┬────────────────────────┘
                                    │ SQL (parameterized)
                                    ▼
                    ┌────────────────────────────────────────┐
                    │           PostgreSQL 16                │
                    │   items          reservations          │
                    │   (row-level locking + atomic UPDATE)  │
                    └────────────────────────────────────────┘
```

The Gin service is split into **routing**, **handlers** (HTTP concerns), a **service layer**
(transaction orchestration and business rules), and a **repository** (SQL). A background
**expiry sweeper** goroutine runs alongside the HTTP server and is stopped on shutdown.

### 5.2 Data Model & PostgreSQL Schema

Two tables. `items` holds the stock counters; `reservations` holds the lifecycle of each hold.

```sql
-- migrations/0001_init.sql
CREATE TABLE items (
    item_id        TEXT PRIMARY KEY,
    name           TEXT        NOT NULL,
    total_stock    INTEGER     NOT NULL CHECK (total_stock >= 0),
    reserved_stock INTEGER     NOT NULL DEFAULT 0 CHECK (reserved_stock >= 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- invariant: reserved can never exceed total
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

-- Fast lookup of due reservations for the sweeper.
CREATE INDEX idx_reservations_due
    ON reservations (expires_at)
    WHERE status = 'ACTIVE';

-- Fast lookup of a user's active holds for an item.
CREATE INDEX idx_reservations_item_status
    ON reservations (item_id, status);
```

**Design rationale:**

- Stock is stored **denormalized** on `items` as `total_stock` and `reserved_stock`. This makes the
  availability check a single-row, gravity-free comparison and lets the reservation be expressed as
  one atomic `UPDATE`, which is the key to concurrency safety (§5.4).
- `available_stock` is **derived** (`total_stock - reserved_stock`) and never stored, so there is no
  third counter to drift out of sync.
- The `CHECK (reserved_stock <= total_stock)` constraint is a **database-level backstop**: even if a
  future code path had a logic bug, PostgreSQL would reject a state where reserved exceeds total.
- `total_stock` doubles as reserved-and-committed inventory: on confirm we decrement **both**
  `total_stock` and `reserved_stock` (see §5.6), keeping the committed quantity out of the pool.
- `reservations.status` is a real enum, giving the DB authority over valid states.

### 5.3 API Contract

All responses are JSON. Success responses return a `status: "success"` discriminator. Errors use a
**single consistent envelope** so the client has one parsing path:

```json
{
  "error": {
    "code": "INSUFFICIENT_STOCK",
    "message": "Only 3 units available, requested 5.",
    "details": { "item_id": "item_4021", "available": 3, "requested": 5 }
  }
}
```

`code` is a stable, machine-readable string the frontend switches on; `message` is human-readable;
`details` carries structured context for the UI.

#### 5.3.1 Reserve Stock

`POST /api/v1/inventory/reserve`

Request:

```json
{ "user_id": "usr_9981", "item_id": "item_4021", "quantity": 2 }
```

Success `201 Created`:

```json
{
  "status": "success",
  "reservation_id": "res_883291",
  "item_id": "item_4021",
  "quantity": 2,
  "expires_at": "2026-07-20T16:35:00Z"
}
```

#### 5.3.2 Confirm Reservation

`POST /api/v1/inventory/confirm`

Request:

```json
{ "reservation_id": "res_883291" }
```

Success `200 OK`:

```json
{
  "status": "success",
  "reservation_id": "res_883291",
  "confirmed_at": "2026-07-20T16:32:00Z"
}
```

#### 5.3.3 Check Inventory Status

`GET /api/v1/inventory/stock?item_id=item_4021`

Success `200 OK`:

```json
{
  "item_id": "item_4021",
  "total_stock": 100,
  "reserved_stock": 15,
  "available_stock": 85
}
```

#### 5.3.4 Status Code Mapping

| Scenario                              | Endpoint  | HTTP | `error.code`         |
| ------------------------------------- | --------- | ---- | -------------------- |
| Reservation created                   | reserve   | 201  | —                    |
| Insufficient available stock          | reserve   | 409  | `INSUFFICIENT_STOCK` |
| Invalid/missing fields, qty ≤ 0       | reserve   | 400  | `INVALID_INPUT`      |
| Unknown `item_id`                     | reserve   | 404  | `ITEM_NOT_FOUND`     |
| Reservation confirmed                 | confirm   | 200  | —                    |
| Reservation not found                 | confirm   | 404  | `RESERVATION_NOT_FOUND` |
| Reservation expired                   | confirm   | 410  | `RESERVATION_EXPIRED` |
| Reservation already confirmed         | confirm   | 409  | `ALREADY_CONFIRMED`  |
| Stock returned                        | stock     | 200  | —                    |
| Unknown item                          | stock     | 404  | `ITEM_NOT_FOUND`     |
| Missing/invalid `item_id`             | stock     | 400  | `INVALID_INPUT`      |
| Unhandled server fault                | any       | 500  | `INTERNAL_ERROR`     |

`409 Conflict` is used for contention against current state (insufficient stock, already
confirmed); `410 Gone` is semantically correct for an expired reservation — the resource existed but
is no longer available in its original form.

### 5.4 Concurrency & Locking Strategy

**Decision: a single atomic conditional `UPDATE` inside a transaction, checking rows-affected.**

Instead of reading stock into the application and deciding there (which invites the read-modify-write
race), we push the entire check-and-decrement into one SQL statement whose `WHERE` clause encodes the
availability invariant:

```sql
UPDATE items
   SET reserved_stock = reserved_stock + $1,
       updated_at     = now()
 WHERE item_id = $2
   AND (total_stock - reserved_stock) >= $1;
```

Then, within the same transaction:

```sql
INSERT INTO reservations
  (reservation_id, user_id, item_id, quantity, status, expires_at)
VALUES ($1, $2, $3, $4, 'ACTIVE', now() + interval '5 minutes');
```

If the `UPDATE` affects **0 rows**, either the item does not exist or there was insufficient stock
and the transaction rolls back with `INSUFFICIENT_STOCK` (or `ITEM_NOT_FOUND`, distinguished by a
prior existence probe or a second diagnostic query). If it affects **1 row**, the reservation insert
proceeds and the transaction commits. Both statements run in one `BEGIN … COMMIT` so the counter
increment and the reservation row are always created together or not at all.

**Why this is safe.** PostgreSQL evaluates the `UPDATE` under row-level locking: concurrent updates
to the same `items` row serialize, and the `WHERE` predicate is re-evaluated against the
latest committed version for each contender. A request that would drive `reserved_stock` past
`total_stock` simply matches zero rows — no over-sell is possible. This is preferable to
`SELECT … FOR UPDATE` followed by a separate `UPDATE` because it removes an entire round-trip and
avoids an explicit lock-unlock window, though both are correct (see §6).

**Isolation level.** The default **`READ COMMITTED`** is sufficient. Under `READ COMMITTED`,
`UPDATE` re-checks the predicate against the newest row version after acquiring the row lock, which
is exactly the semantics we rely on. We deliberately avoid `SERIALIZABLE`, which would add
serialization-failure retries without buying correctness here.

**Deadlocks.** Each reservation touches exactly one `items` row and appends to `reservations`, so
there is no multi-row lock ordering to get wrong. If future work reserves multiple items in one
transaction, rows must be locked in a deterministic order (e.g., sorted by `item_id`) to prevent
deadlocks.

**Idempotency of confirm.** Confirmation is guarded by the reservation's status: the update is
scoped to `WHERE reservation_id = $1 AND status = 'ACTIVE'`. A repeat confirm affects zero rows and
returns `ALREADY_CONFIRMED` rather than double-decrementing inventory.

### 5.5 Reservation Lifecycle State Machine

```
            reserve (available stock)
                   │
                   ▼
              ┌─────────┐
              │ ACTIVE  │◄──── created, counts against reserved_stock
              └────┬────┘
        confirm    │    expiry (5 min)
       (commit)    │    (sweeper / lazy check)
                   ▼            ▼
            ┌───────────┐  ┌──────────┐
            │ CONFIRMED │  │ EXPIRED  │
            └───────────┘  └──────────┘
         total_stock -= q   reserved_stock -= q
         reserved_stock -=q  (returned to available)
```

Physical inventory (`total_stock`) is decremented **only on confirm**, never on reserve. A reserve
only moves quantity from available into reserved; an expiry moves it back. The state machine is
strictly linear — no transition out of `CONFIRMED` or `EXPIRED`.

### 5.6 Confirm — Transaction Detail

```sql
BEGIN;

-- 1. Flip the reservation, only if still ACTIVE.
UPDATE reservations
   SET status = 'CONFIRMED', confirmed_at = now()
 WHERE reservation_id = $1
   AND status = 'ACTIVE'
RETURNING item_id, quantity;

-- If 0 rows: inspect status → EXPIRED (410) or ALREADY_CONFIRMED (409) or not found (404).

-- 2. Commit physically: remove the held units from the pool entirely.
UPDATE items
   SET total_stock    = total_stock    - $2,
       reserved_stock = reserved_stock - $2,
       updated_at     = now()
 WHERE item_id = $3;

COMMIT;
```

Step 1 is the concurrency-critical guard: only one concurrent confirm can flip `ACTIVE →
CONFIRMED`, so the second gets zero rows. If the reservation is `EXPIRED`, no physical stock is
touched — the sweeper already returned it.

### 5.7 Automatic Expiry Cleanup

Two complementary mechanisms, because either alone has a gap:

**Background sweeper (primary).** A Go goroutine driven by a `time.Ticker` (e.g., every 30s) runs:

```sql
WITH due AS (
    SELECT reservation_id, item_id, quantity
      FROM reservations
     WHERE status = 'ACTIVE' AND expires_at <= now()
     FOR UPDATE SKIP LOCKED
)
UPDATE reservations r
   SET status = 'EXPIRED'
  FROM due
 WHERE r.reservation_id = due.reservation_id;

-- then, per affected item, return quantities to available:
UPDATE items
   SET reserved_stock = reserved_stock - due.quantity
  FROM due
 WHERE items.item_id = due.item_id;
```

The status flip and the stock return run in one transaction. `FOR UPDATE SKIP LOCKED` makes the
sweeper safe to run concurrently (and, later, on multiple replicas) — each row is processed exactly
once. The partial index on `(expires_at) WHERE status='ACTIVE'` keeps this scan small.

**Lazy / on-read reconciliation (safety net).** The reserve and stock paths treat a reservation as
expired if `expires_at <= now()` even before the sweeper has run. A confirm on an expired row is
rejected with `410`. This guarantees correctness between sweeper ticks, so a 30s ticker never causes
a visible inconsistency.

### 5.8 Graceful Shutdown

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

srv := &http.Server{Addr: ":8080", Handler: router}

go func() { _ = srv.ListenAndServe() }()

<-ctx.Done()                       // signal received
stop()                             // restore default behavior

shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
_ = srv.Shutdown(shutdownCtx)      // drain in-flight requests

sweeperCancel()                    // stop the ticker goroutine
db.Close()                         // close the pgx pool / sql.DB
```

`signal.NotifyContext` couples OS signals to a context; `http.Server.Shutdown` stops accepting new
connections and waits for in-flight requests to finish (bounded by a 10s timeout), so no request is
abruptly severed. The sweeper goroutine and the database pool are torn down after the HTTP drain,
ensuring no write is cut mid-transaction and no connection is leaked.

### 5.9 Frontend Design (React + Vite)

The SPA is a small set of components with a thin data layer:

- `InventoryTracker` — renders total / reserved / available for the selected item. Fetches
  `GET /stock` on mount and on a `setInterval` poll (e.g., every 3s), with a manual **Refresh**
  button. Polling via `useEffect` with a cleanup that clears the interval; a generation guard
  discards stale responses.
- `ReservationForm` — controlled inputs for **User ID** and **Quantity**; submits
  `POST /reserve`; disables the submit while in-flight; renders the error envelope inline on
  non-2xx.
- `ActiveReservationCard` — shown after a successful reserve; displays `reservation_id`, `item_id`,
  `quantity`, and a **live 5-minute countdown** derived from `expires_at`.
- **Countdown** — a `useEffect`/`setInterval` ticking every second computing
  `remaining = max(0, expiresAt - Date.now())`, formatted `mm:ss`. At zero the card switches to an
  "Expired" state and disables **Confirm Purchase**; the tracker refreshes so the released stock is
  visible promptly.
- **Confirm** — `POST /confirm` with the active `reservation_id`; on success shows confirmation and
  re-polls stock; on `410 RESERVATION_EXPIRED` / `409 ALREADY_CONFIRMED` / `404` renders the
  corresponding message from the error envelope.

All state is local React state for this scope; a single small `api.js` module centralizes
`fetch` calls and unwraps the consistent success/error envelope, keeping components free of HTTP
plumbing. The app is scaffolded with `npm create vite@latest` using the React + TypeScript template —
**React + Vite strictly**, with no Next.js/Nuxt/other frameworks.

### 5.10 Docker & Local Orchestration

`docker-compose.yaml` defines three services so the system runs with a single `docker compose up`:

- `postgres` — official `postgres:16` image, a named volume for persistence, and a healthcheck
  (`pg_isready`). Migration SQL in `./migrations` is mounted to `/docker-entrypoint-initdb.d` so the
  schema and seed data apply on first boot.
- `backend` — built from the Go `Dockerfile` (multi-stage: build then slim runtime). Declares
  `depends_on: postgres: condition: service_healthy` so it never races the database.
- `frontend` — optional; built from the Vite `Dockerfile` (build then serve static assets), with the
  API base URL injected via an env var / Vite build arg.

Environment variables (DB DSN, server port, TTL) come from a `.env` file, with sane defaults.

### 5.11 Testing Strategy

- **Unit tests** — table-driven tests for the service layer: successful reserve, insufficient stock,
  zero/negative quantity, unknown item, successful confirm, expired confirm, already-confirmed
  confirm, and the stock derivation. Repository tests run against a disposable PostgreSQL (the
  Compose database, or testcontainers).
- **Stress / race test** — spin up N goroutines (e.g., 200) that each attempt to reserve 1 unit of an
  item seeded with M (e.g., 50) units, then assert `successes == M` and `reserved_stock <= total_stock`
  and no negative availability. Run a second scenario mixing reserve + expiry to confirm release
  accounting.
- **Gate** — `go test -race -v ./...`. The race detector is meaningful here because the sweeper and
  request handlers both touch shared state through the DB and the service structs; the suite must
  pass cleanly under `-race`.

---

## 6. Alternatives Considered

| Option                                   | Pros                                              | Cons                                                       | Verdict |
| ---------------------------------------- | ------------------------------------------------- | ---------------------------------------------------------- | ------- |
| **Atomic conditional `UPDATE`**          | One round-trip; no explicit lock window; race-free | Requires careful rows-affected handling                    | **Chosen** |
| `SELECT … FOR UPDATE` then `UPDATE`      | Explicit, familiar, easy to reason about          | Extra round-trip; holds lock across app logic              | Viable fallback |
| Optimistic locking (version column)      | No DB locks; good for low contention              | Retry storms under flash-sale contention                   | Rejected |
| In-memory / SQLite / mocks               | Trivial local dev                                 | **Strictly disallowed**; no real concurrency semantics     | Rejected |
| Redis-based distributed reservation      | Very high throughput; cross-service               | Adds infra; durability/atomicity of the final commit still lands in PG | Overkill for scope |
| DB-level scheduler (`pg_cron`)           | Keeps expiry off the app tier                     | Extra extension/setup; less testable in Go suite           | Rejected for time budget |
| Advisory locks (`pg_advisory_lock`)      | Simple mutual exclusion                           | Session-scoped; easy to leak; less granular than row locks | Rejected |

The decisive trade-off is **atomic conditional `UPDATE`** for reserve/confirm (simplest correct
primitive) combined with **`FOR UPDATE SKIP LOCKED`** in the sweeper (correct under concurrent and
future multi-replica execution).

---

## 7. Trade-offs & AI Transparency

Within the 4–8 hour budget we deliberately optimized for **demonstrable correctness over scope**:

- **Go backend** — we invested in the transaction/locking design, the expiry sweeper, graceful
  shutdown, and a genuine concurrency test rather than adding endpoints (carts, search, admin). The
  denormalized `items` counters trade a bit of storage redundancy for a single-statement atomic
  reservation, which is the single most important simplification.
- **React frontend** — we chose local component state and `setInterval` polling instead of adding a
  data-fetching/state library. Polling is adequate for a dashboard of this size and avoids
  dependency overhead; the cost is slightly stale data between ticks, which is acceptable here.
- **Expiry** — an app-level ticker was chosen over `pg_cron` to keep the behaviour testable inside
  the Go suite and portable across environments, accepting that it must be made single-writer when
  the service scales out (§9).

**AI transparency (illustrative).** A common AI-suggested implementation for reserve is to read the
row, check availability in Go (`if available >= qty`), then `UPDATE` in a separate statement, within
`READ COMMITTED`. That suggestion is **flawed**: it reintroduces the read-modify-write race because
the check happens in application memory, not atomically with the write — under concurrent load it
over-sells. We fixed it by moving the guard into the `WHERE` clause of the `UPDATE`
(`AND (total_stock - reserved_stock) >= $1`) and branching on rows-affected, so PostgreSQL performs
the check-and-decrement atomically. We also added the `reserved_stock <= total_stock` CHECK as a
second line of defence, and a concurrency test that would have failed against the naive version.

---

## 8. Distributed Scaling & Failure Modes

**Scaling horizontally to 10 stateless Gin instances against one PostgreSQL** exposes several
failure modes:

1. **Multiple sweepers.** Every instance would run its own ticker and race to expire the same rows,
   risking double-returns to `reserved_stock`. Mitigation already designed in: the sweeper uses
   `FOR UPDATE SKIP LOCKED`, so overlapping sweeps partition work instead of duplicating it. For a
   cleaner model, elect a **single leader** via `pg_advisory_lock` (session lock) or a
   `sweeper_leases` row, and have only the leader sweep; the rest stay idle.
2. **Connection-pool exhaustion.** 10 instances × a large per-instance pool can exceed
   `max_connections`. Mitigation: bound each instance's pool (`SetMaxOpenConns`), and place
   **PgBouncer** in transaction-pooling mode in front of PostgreSQL.
3. **Idempotency.** Retries from load balancers or clients must not double-confirm; the
   `status = 'ACTIVE'` guard in §5.6 makes confirm naturally idempotent.
4. **Read scaling.** Stock reads dominate during a sale; route `GET /stock` to **read replicas**,
   accepting slight replication lag, while all writes go to the primary.
5. **Single primary is the ceiling.** One PostgreSQL primary serializes writes on the hot `items`
   rows. Beyond a point, redesign options include **sharding by `item_id`** (each item's counters
   live in one shard, preserving the single-row atomic update) or moving the hot counter to an
   atomic store (Redis `DECRBY` with a durable reconciliation back to PG), keeping PostgreSQL as the
   durable system of record.

For a **truly stateless** deployment, the design already pushes all shared state into PostgreSQL —
the Gin instances hold no reservation state in memory — so they can be scaled, killed, and replaced
freely behind a load balancer, provided the sweeper is single-writer and pools are bounded.

---

## 9. Security Considerations

- **Input validation** — every handler validates presence, type, and range (`quantity > 0`, non-empty
  `user_id`/`item_id`) before touching the database, returning `400 INVALID_INPUT`.
- **SQL injection** — all queries use parameterized statements (`$1`, `$2`) via `pgx`/`database/sql`;
  no string-concatenated SQL.
- **Trust boundary** — `user_id` is client-supplied and unauthenticated in this exercise; a
  production system would derive it from an authenticated session/JWT and reject mismatches.
- **Resource abuse** — a reservation endpoint is a natural DoS target during sales; a production
  deployment adds per-user/IP **rate limiting** (middleware) and caps the number of concurrent
  active reservations per user.
- **Response hygiene** — error envelopes expose stable codes and safe messages, never raw SQL errors
  or stack traces.
- **Secrets** — the DB DSN and credentials come from environment variables / `.env`, never committed.

---

## 10. Open Questions

1. **Reservation-per-user limits** — should a user be allowed many concurrent holds on one item, or
   should there be a cap to reduce hoarding during sales?
2. **Sweeper cadence vs. precision** — is a 30s sweep tick acceptable, or should the TTL release be
   event-driven (e.g., `LISTEN/NOTIFY` or a scheduled job) for tighter expiry accuracy?
3. **Partial confirmation** — is a reservation all-or-nothing, or should buyers confirm a subset of
   the held quantity? The current schema assumes all-or-nothing.
4. **Retention** — how long should `EXPIRED`/`CONFIRMED` reservation rows be retained for audit
   before archival/purge?
5. **Multi-item reservations** — does the roadmap require reserving several items atomically, which
   would introduce multi-row lock ordering and deadlock-avoidance concerns (§5.4)?

---

## 11. References & Appendix

- Assignment brief: *Fullstack Take Home Assignment — High-Throughput Inventory Reservation System &
  Mini Dashboard* (Go + Gin + PostgreSQL | React + Vite; 4–8h).
- Required written deliverable: `ARCHITECTURE.md` (1–2 pages) covering architectural design & DB
  locking strategy, distributed scaling & failure modes, and engineering trade-offs & AI
  transparency.
- Setup deliverable: `README.md` with instructions to run PostgreSQL via `docker compose up`, start
  the Gin backend, and run the React + Vite frontend.
- Quality gates: `go test -race -v ./...`; single-command `docker compose up`; incremental Git
  history across frontend and backend phases.
