// Override any value with `k6 run -e KEY=value`. HTTP 409 (INSUFFICIENT_STOCK)
// is an expected business outcome once the 100-unit seed item drains.
const positive = (raw, fallback) => {
  const n = Number(raw);
  return Number.isFinite(n) && n > 0 ? n : fallback;
};

export const BASE_URL = (__ENV.BASE_URL || 'http://localhost:8080').replace(/\/+$/, '');
export const ITEM_ID = __ENV.ITEM_ID || 'item_4021';
export const RESERVE_QUANTITY = positive(__ENV.RESERVE_QUANTITY, 1);
export const UNKNOWN_ITEM_ID = __ENV.UNKNOWN_ITEM_ID || 'item_k6_missing';
export const USER_PREFIX = __ENV.USER_PREFIX || 'usr_k6';

export const STOCK_RATE = positive(__ENV.STOCK_RATE, 300);
export const RESERVE_RATE = positive(__ENV.RESERVE_RATE, 200);
export const LIFECYCLE_VUS = positive(__ENV.LIFECYCLE_VUS, 20);
export const DURATION = __ENV.DURATION || '30s';
export const RAMP = __ENV.RAMP || '10s';

export const P95_MS = positive(__ENV.P95_MS, 250);
export const P99_MS = positive(__ENV.P99_MS, 750);
