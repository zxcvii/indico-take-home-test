# k6 stress suite

Load and stress tests for the Inventory Reservation backend, written with
[k6](https://k6.io). They exercise every public endpoint, drive the flash-sale
reservation path to its concurrency limit, and assert the service's core
guarantees (no over-sell, no negative stock, stable error codes) while loaded.

## Layout

| File            | Purpose                                                        |
| --------------- | -------------------------------------------------------------- |
| `config.js`     | Environment-driven configuration (base URL, rates, budgets)    |
| `lib/api.js`    | Request wrappers + expected-status callbacks + invariant checks |
| `main.js`       | Scenarios, custom metrics and thresholds                       |

## Prerequisites

- **k6** (`brew install k6`) — v0.42+ for per-request `responseCallback`.
- A **running backend** on `http://localhost:8080`
  (`docker compose up --build`).

## Run

```bash
# Against the default local backend.
k6 run k6/main.js

# Higher write pressure, shorter run, tighter budget.
k6 run -e RESERVE_RATE=500 -e DURATION=20s -e P95_MS=150 k6/main.js

# Emit a machine-readable summary for CI.
k6 run --summary-export=k6/summary.json k6/main.js
```

## Scenarios

All scenarios run concurrently under one load profile.

| Scenario        | Endpoint(s)                     | Load shape                         | Validates |
| --------------- | ------------------------------- | ---------------------------------- | --------- |
| `smoke`         | `GET /health`, reserve, confirm | 1 VU × 1 iteration                 | Service is reachable and a full reserve→confirm cycle works |
| `stock_read`    | `GET /stock`                    | ramp to `STOCK_RATE` req/s         | Read latency + stock-consistency invariants on every read |
| `reserve_burst` | `POST /reserve`                 | ramp to `RESERVE_RATE` req/s       | Atomic no-over-sell under heavy contention; the core test |
| `lifecycle`     | `POST /reserve` → `POST /confirm` | ramp to `LIFECYCLE_VUS` VUs      | End-to-end order flow, confirm idempotency |
| `error_paths`   | all                             | ramp to 20 req/s                   | Stable `404`/`400` error codes under load |

Legitimate business outcomes are **not** treated as failures: `reserve` may
return `409 INSUFFICIENT_STOCK`, and `confirm` may return `409 ALREADY_CONFIRMED`
or `410 RESERVATION_EXPIRED`. Each request declares its expected statuses via
`responseCallback`, so `http_req_failed` reflects only genuine problems
(unexpected 4xx/5xx and transport errors).

> **Finite stock:** the demo seed (`item_4021`) has only **100 units**, so once
> stock is drained almost every `reserve` returns `409`. That is the realistic
> flash-sale outcome, not a bug. Reset the database for a pristine run:
> ```bash
> docker compose down -v && docker compose up --build
> ```

## Configuration

Every value is overridable with `-e KEY=value`.

| Variable           | Default                  | Meaning |
| ------------------ | ------------------------ | ------- |
| `BASE_URL`         | `http://localhost:8080`  | Backend base URL |
| `ITEM_ID`          | `item_4021`              | Item under test |
| `RESERVE_QUANTITY` | `1`                      | Units per reserve attempt |
| `UNKNOWN_ITEM_ID`  | `item_k6_missing`        | Id used for 404 assertions |
| `USER_PREFIX`      | `usr_k6`                 | Synthetic user-id prefix |
| `STOCK_RATE`       | `300`                    | Target stock reads per second |
| `RESERVE_RATE`     | `200`                    | Target reserve attempts per second |
| `LIFECYCLE_VUS`    | `20`                     | VUs on the reserve→confirm flow |
| `DURATION`         | `30s`                    | Sustained phase per scenario |
| `RAMP`             | `10s`                    | Ramp-up/down per scenario |
| `P95_MS`           | `250`                    | 95th-percentile latency budget (ms) |
| `P99_MS`           | `750`                    | 99th-percentile latency budget (ms) |

## Thresholds (pass/fail gates)

- `checks` rate `> 99%`
- `http_req_failed` rate `< 1%` (unexpected statuses only)
- `http_req_duration` p95/p99 under budget, per scenario
- `reserve_unexpected`, `confirm_unexpected`, `stock_invariant_violations` `== 0`

A non-zero exit code from `k6 run` means at least one threshold failed.

## Reading the results

- **`reserve_created` vs `reserve_insufficient_stock`** — how much real demand
  the finite stock absorbed. A high 409 share is expected on `item_4021`.
- **`reserve_latency_ms`** — reserve-endpoint latency across all outcomes.
- **`stock_invariant_violations`** — must stay `0`; any value means a read
  reported inconsistent stock (`available != total - reserved`, negative
  available, or `reserved > total`), i.e. a correctness regression.
- **`http_req_failed`** — should stay near zero; spikes indicate 5xx or
  transport failures under load.
