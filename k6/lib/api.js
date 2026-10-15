// Expected statuses are declared per request via `responseCallback` so that
// legitimate business responses (409 insufficient stock, 409 already confirmed,
// 410 expired, 404 not found, 400 invalid input) do not inflate
// `http_req_failed`; only genuinely unexpected statuses fail the request.
import http from 'k6/http';
import { check } from 'k6';

import { BASE_URL, ITEM_ID, RESERVE_QUANTITY, USER_PREFIX } from '../config.js';

export const EXPECT_HEALTH = http.expectedStatuses(200);
export const EXPECT_STOCK = http.expectedStatuses(200);
export const EXPECT_RESERVE = http.expectedStatuses(201, 409);
export const EXPECT_CONFIRM = http.expectedStatuses(200, 409, 410);
export const EXPECT_NOT_FOUND = http.expectedStatuses(404);
export const EXPECT_BAD_REQUEST = http.expectedStatuses(400);

export const JSON_HEADERS = { 'Content-Type': 'application/json' };

export function url(path) {
  return `${BASE_URL}${path}`;
}

export function userId() {
  return `${USER_PREFIX}_${__VU}_${__ITER}`;
}

export function parseJson(res) {
  try {
    return res.json();
  } catch (_) {
    return null;
  }
}

export function getHealth() {
  return http.get(url('/health'), {
    responseCallback: EXPECT_HEALTH,
    tags: { endpoint: 'health' },
  });
}

export function getStock(itemId = ITEM_ID, callback = EXPECT_STOCK) {
  return http.get(url(`/api/v1/inventory/stock?item_id=${encodeURIComponent(itemId)}`), {
    responseCallback: callback,
    tags: { endpoint: 'stock' },
  });
}

export function reserve(itemId = ITEM_ID, quantity = RESERVE_QUANTITY, callback = EXPECT_RESERVE) {
  const body = JSON.stringify({ user_id: userId(), item_id: itemId, quantity });
  return http.post(url('/api/v1/inventory/reserve'), body, {
    headers: JSON_HEADERS,
    responseCallback: callback,
    tags: { endpoint: 'reserve' },
  });
}

export function confirm(reservationId, callback = EXPECT_CONFIRM) {
  const body = JSON.stringify({ reservation_id: reservationId });
  return http.post(url('/api/v1/inventory/confirm'), body, {
    headers: JSON_HEADERS,
    responseCallback: callback,
    tags: { endpoint: 'confirm' },
  });
}

// Encodes the service's core guarantee — stock is never negative and never
// over-sold — as: 0 <= reserved <= total, available >= 0, available == total - reserved.
export function checkStockInvariants(res, violationCounter) {
  const body = parseJson(res);

  const shapeOk = check(res, {
    'stock: 200 OK': (r) => r.status === 200,
    'stock: JSON body': () => body !== null,
  });
  if (!body) return shapeOk;

  const total = body.total_stock;
  const reserved = body.reserved_stock;
  const available = body.available_stock;
  const ints =
    Number.isInteger(total) && Number.isInteger(reserved) && Number.isInteger(available);

  const invariantsOk = check(res, {
    'stock: fields are integers': () => ints,
    'stock: total >= 0': () => ints && total >= 0,
    'stock: reserved >= 0': () => ints && reserved >= 0,
    'stock: available >= 0 (no negative stock)': () => ints && available >= 0,
    'stock: reserved <= total (never over-sold)': () => ints && reserved <= total,
    'stock: available == total - reserved': () => ints && available === total - reserved,
  });

  if (!invariantsOk && violationCounter) violationCounter.add(1);
  return shapeOk && invariantsOk;
}
