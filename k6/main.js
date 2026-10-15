import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Trend } from 'k6/metrics';

import {
  ITEM_ID,
  UNKNOWN_ITEM_ID,
  STOCK_RATE,
  RESERVE_RATE,
  LIFECYCLE_VUS,
  DURATION,
  RAMP,
  P95_MS,
  P99_MS,
} from './config.js';
import * as api from './lib/api.js';

const reserveCreated = new Counter('reserve_created');
const reserveNoStock = new Counter('reserve_insufficient_stock');
const reserveUnexpected = new Counter('reserve_unexpected');
const confirmConfirmed = new Counter('confirm_confirmed');
const confirmAlready = new Counter('confirm_already');
const confirmExpired = new Counter('confirm_expired');
const confirmUnexpected = new Counter('confirm_unexpected');
const stockViolations = new Counter('stock_invariant_violations');
const reserveLatency = new Trend('reserve_latency_ms', true);

export const options = {
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  scenarios: {
    smoke: {
      executor: 'per-vu-iterations',
      exec: 'smoke',
      vus: 1,
      iterations: 1,
      tags: { scenario: 'smoke' },
    },
    stock_read: {
      executor: 'ramping-arrival-rate',
      exec: 'stockRead',
      startRate: 0,
      timeUnit: '1s',
      preAllocatedVUs: 25,
      maxVUs: 400,
      startTime: '2s',
      stages: [
        { target: Math.ceil(STOCK_RATE / 4), duration: RAMP },
        { target: STOCK_RATE, duration: DURATION },
        { target: 0, duration: RAMP },
      ],
      tags: { scenario: 'stock_read' },
    },
    reserve_burst: {
      executor: 'ramping-arrival-rate',
      exec: 'reserveBurst',
      startRate: 0,
      timeUnit: '1s',
      preAllocatedVUs: 25,
      maxVUs: 500,
      startTime: '2s',
      stages: [
        { target: Math.ceil(RESERVE_RATE / 4), duration: RAMP },
        { target: RESERVE_RATE, duration: DURATION },
        { target: 0, duration: RAMP },
      ],
      tags: { scenario: 'reserve_burst' },
    },
    lifecycle: {
      executor: 'ramping-vus',
      exec: 'lifecycle',
      startVUs: 1,
      startTime: '2s',
      stages: [
        { target: LIFECYCLE_VUS, duration: RAMP },
        { target: LIFECYCLE_VUS, duration: DURATION },
        { target: 0, duration: RAMP },
      ],
      tags: { scenario: 'lifecycle' },
    },
    error_paths: {
      executor: 'ramping-arrival-rate',
      exec: 'errorPaths',
      startRate: 0,
      timeUnit: '1s',
      preAllocatedVUs: 10,
      maxVUs: 60,
      startTime: '2s',
      stages: [
        { target: 20, duration: RAMP },
        { target: 20, duration: DURATION },
        { target: 0, duration: RAMP },
      ],
      tags: { scenario: 'error_paths' },
    },
  },
  thresholds: {
    checks: ['rate>0.99'],
    http_req_failed: ['rate<0.01'],
    'http_req_duration{scenario:stock_read}': [`p(95)<${P95_MS}`, `p(99)<${P99_MS}`],
    'http_req_duration{scenario:reserve_burst}': [`p(95)<${P95_MS}`, `p(99)<${P99_MS}`],
    'http_req_duration{scenario:lifecycle}': [`p(95)<${P95_MS}`, `p(99)<${P99_MS}`],
    'http_req_duration{scenario:error_paths}': [`p(95)<${P95_MS}`, `p(99)<${P99_MS}`],
    reserve_unexpected: ['count==0'],
    confirm_unexpected: ['count==0'],
    stock_invariant_violations: ['count==0'],
  },
};

// k6 requires a default export; every scenario routes to a named exec instead.
export default function () {}

export function smoke() {
  const healthRes = api.getHealth();
  const healthBody = api.parseJson(healthRes);
  check(healthRes, {
    'health: 200 OK': (r) => r.status === 200,
    'health: status is ok': () => !!healthBody && healthBody.status === 'ok',
  });

  const created = api.reserve();
  check(created, { 'smoke reserve: 201 or 409': (r) => r.status === 201 || r.status === 409 });
  if (created.status !== 201) return;

  reserveCreated.add(1);
  const body = api.parseJson(created);
  if (!body || !body.reservation_id) return;

  const confirmed = api.confirm(body.reservation_id);
  check(confirmed, { 'smoke confirm: 200': (r) => r.status === 200 });
}

export function stockRead() {
  const res = api.getStock();
  api.checkStockInvariants(res, stockViolations);
}

export function reserveBurst() {
  const res = api.reserve();
  reserveLatency.add(res.timings.duration);

  check(res, { 'reserve: 201 or 409': (r) => r.status === 201 || r.status === 409 });

  if (res.status === 201) {
    reserveCreated.add(1);
    const body = api.parseJson(res);
    check(res, {
      'reserve 201: reservation_id present': () =>
        !!body && typeof body.reservation_id === 'string' && body.reservation_id.length > 0,
      'reserve 201: echoes requested item_id': () => !!body && body.item_id === ITEM_ID,
      'reserve 201: quantity >= 1': () => !!body && Number.isInteger(body.quantity) && body.quantity >= 1,
    });
  } else if (res.status === 409) {
    reserveNoStock.add(1);
  } else {
    reserveUnexpected.add(1);
  }
}

export function lifecycle() {
  const created = api.reserve();
  check(created, { 'lifecycle reserve: 201 or 409': (r) => r.status === 201 || r.status === 409 });

  if (created.status === 409) {
    reserveNoStock.add(1);
    sleep(0.05);
    return;
  }
  if (created.status !== 201) {
    reserveUnexpected.add(1);
    return;
  }

  reserveCreated.add(1);
  const body = api.parseJson(created);
  if (!body || !body.reservation_id) {
    reserveUnexpected.add(1);
    return;
  }

  const confirmed = api.confirm(body.reservation_id);
  check(confirmed, {
    'lifecycle confirm: 200/409/410': (r) => r.status === 200 || r.status === 409 || r.status === 410,
  });

  if (confirmed.status === 200) confirmConfirmed.add(1);
  else if (confirmed.status === 409) confirmAlready.add(1);
  else if (confirmed.status === 410) confirmExpired.add(1);
  else confirmUnexpected.add(1);

  sleep(Math.random() * 0.2);
}

export function errorPaths() {
  const stockRes = api.getStock(UNKNOWN_ITEM_ID, api.EXPECT_NOT_FOUND);
  const stockBody = api.parseJson(stockRes);
  check(stockRes, {
    'unknown stock: 404': (r) => r.status === 404,
    'unknown stock: ITEM_NOT_FOUND': () =>
      !!stockBody && !!stockBody.error && stockBody.error.code === 'ITEM_NOT_FOUND',
  });

  const reserveRes = api.reserve(UNKNOWN_ITEM_ID, 1, api.EXPECT_NOT_FOUND);
  const reserveBody = api.parseJson(reserveRes);
  check(reserveRes, {
    'unknown reserve: 404': (r) => r.status === 404,
    'unknown reserve: ITEM_NOT_FOUND': () =>
      !!reserveBody && !!reserveBody.error && reserveBody.error.code === 'ITEM_NOT_FOUND',
  });

  const confirmRes = api.confirm(`res_k6_missing_${__VU}_${__ITER}`, api.EXPECT_NOT_FOUND);
  const confirmBody = api.parseJson(confirmRes);
  check(confirmRes, {
    'unknown confirm: 404': (r) => r.status === 404,
    'unknown confirm: RESERVATION_NOT_FOUND': () =>
      !!confirmBody && !!confirmBody.error && confirmBody.error.code === 'RESERVATION_NOT_FOUND',
  });

  // Missing user_id and quantity proves the request validator, not the DB, rejects it.
  const badRes = http.post(
    api.url('/api/v1/inventory/reserve'),
    JSON.stringify({ item_id: ITEM_ID }),
    { headers: api.JSON_HEADERS, responseCallback: api.EXPECT_BAD_REQUEST, tags: { endpoint: 'reserve' } },
  );
  const badBody = api.parseJson(badRes);
  check(badRes, {
    'malformed reserve: 400': (r) => r.status === 400,
    'malformed reserve: INVALID_INPUT': () =>
      !!badBody && !!badBody.error && badBody.error.code === 'INVALID_INPUT',
  });
}
