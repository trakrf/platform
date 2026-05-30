// k6 load-test scenario for the trakrf API.
//
// Usage:
//   k6 run -e BASE_URL=https://gke.trakrf.app load-tests/k6/scenario.js
//
// Optional env vars:
//   BASE_URL   — defaults to http://localhost:8080
//   PEAK_VUS   — peak concurrent virtual users (default 20)
//   STAGE_S    — seconds per ramp stage (default 60)
//
// What it does:
//   setup()    creates a fresh org+user via /auth/signup, pre-seeds 3 locations
//              and 5 assets, and mints one INDEPENDENT refresh-token chain per
//              VU (shared-org scenario — no per-VU signup overhead).
//   default()  each iteration picks a weighted hot-path action (list/get/create/
//              delete) and records latency. New asset IDs are deleted on the next
//              iteration cycle so the org doesn't grow unbounded. On any 401 the
//              VU transparently refreshes its token and retries once.
//   teardown() deletes the org via DELETE /orgs/:id with confirm_name.
//
// Auth / soak note (TRA-843): the access token is a short-lived JWT; past its
// TTL every request 401s until refreshed. This scenario calls POST /auth/refresh
// on a 401 and retries, so it stays green across multi-hour soaks. Refresh
// tokens are single-use/rotated server-side, so each VU MUST hold its own chain
// — a shared chain would trip replay-detection at the synchronized TTL expiry
// and revoke everyone, cascading to permanent 401.

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { randomString, randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const BASE_URL = (__ENV.BASE_URL || 'http://localhost:8080').replace(/\/$/, '');
const API = `${BASE_URL}/api/v1`;
const PEAK_VUS = parseInt(__ENV.PEAK_VUS || '20', 10);
const STAGE_S = parseInt(__ENV.STAGE_S || '60', 10);
const PASSWORD = 'LoadTest123!';

const errors = new Counter('biz_errors');
const okRate = new Rate('biz_ok');
const refreshCount = new Counter('token_refreshes');
const refreshFail = new Counter('token_refresh_failures');
const listAssetsLat = new Trend('lat_list_assets', true);
const createAssetLat = new Trend('lat_create_asset', true);
const getAssetLat = new Trend('lat_get_asset', true);
const listLocationsLat = new Trend('lat_list_locations', true);
const reportsLat = new Trend('lat_reports', true);

export const options = {
  stages: [
    { duration: `${Math.round(STAGE_S / 2)}s`, target: Math.max(1, Math.floor(PEAK_VUS / 4)) },
    { duration: `${STAGE_S}s`, target: PEAK_VUS },
    { duration: `${STAGE_S}s`, target: PEAK_VUS },
    { duration: `${Math.round(STAGE_S / 2)}s`, target: 0 },
  ],
  thresholds: {
    // 401s that trigger a refresh-and-retry are expected on long soaks (~1 per
    // VU per access-token TTL), so allow a small raw-failure budget.
    http_req_failed: ['rate<0.02'],
    'http_req_duration{kind:read}': ['p(95)<500'],
    'http_req_duration{kind:write}': ['p(95)<1000'],
    biz_ok: ['rate>0.99'],
  },
  // Don't fail the run on threshold breach — record and report.
  noConnectionReuse: false,
};

function authHeaders(token) {
  return {
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
  };
}

function isoToday() {
  // RFC 3339 timestamp at start of UTC day.
  return new Date().toISOString().split('T')[0] + 'T00:00:00Z';
}

// POST /auth/signup → { data: { access_token, refresh_token, expires_in, user } }.
// Returns { access, refresh, orgId, orgName, email }.
function signup(suffix) {
  const orgName = `loadtest-${suffix}`;
  const email = `loadtest-${suffix}@example.com`;

  const res = http.post(
    `${API}/auth/signup`,
    JSON.stringify({ email, password: PASSWORD, org_name: orgName }),
    { headers: { 'Content-Type': 'application/json' }, tags: { kind: 'setup' } }
  );
  if (res.status !== 200 && res.status !== 201) {
    throw new Error(`signup failed: ${res.status} ${res.body}`);
  }
  const d = res.json().data;
  if (!d || !d.access_token) throw new Error(`signup ok but no access_token: ${res.body}`);

  // Resolve org id from /users/me.
  const me = http.get(`${API}/users/me`, authHeaders(d.access_token));
  if (me.status !== 200) throw new Error(`/users/me failed: ${me.status} ${me.body}`);
  const orgId = me.json().data.current_org.id;

  return { access: d.access_token, refresh: d.refresh_token, orgId, orgName, email };
}

// POST /auth/login → a fresh INDEPENDENT { access, refresh } chain for an
// existing user. Used to mint one chain per VU.
function login(email) {
  const res = http.post(
    `${API}/auth/login`,
    JSON.stringify({ email, password: PASSWORD }),
    { headers: { 'Content-Type': 'application/json' }, tags: { kind: 'setup' } }
  );
  if (res.status !== 200) throw new Error(`login failed: ${res.status} ${res.body}`);
  const d = res.json().data;
  if (!d || !d.access_token) throw new Error(`login ok but no access_token: ${res.body}`);
  return { access: d.access_token, refresh: d.refresh_token };
}

function createSeedLocation(token, n) {
  const res = http.post(
    `${API}/locations`,
    JSON.stringify({
      name: `seed-loc-${n}`,
      external_key: `SEED-LOC-${n}`,
      is_active: true,
      valid_from: isoToday(),
      tags: [],
    }),
    { ...authHeaders(token), tags: { kind: 'setup' } }
  );
  if (res.status !== 200 && res.status !== 201) {
    throw new Error(`seed location ${n} failed: ${res.status} ${res.body}`);
  }
  return res.json().data.id;
}

function createSeedAsset(token, n) {
  const res = http.post(
    `${API}/assets`,
    JSON.stringify({
      name: `seed-asset-${n}`,
      external_key: `SEED-ASSET-${n}`,
      is_active: true,
      valid_from: isoToday(),
      tags: [{ tag_type: 'rfid', value: `SEED-EPC-${n}-${randomString(8)}` }],
    }),
    { ...authHeaders(token), tags: { kind: 'setup' } }
  );
  if (res.status !== 200 && res.status !== 201) {
    throw new Error(`seed asset ${n} failed: ${res.status} ${res.body}`);
  }
  return res.json().data.id;
}

export function setup() {
  const suffix = `${Date.now()}-${randomString(6)}`;
  console.log(`[setup] signing up org ${suffix}`);
  const ctx = signup(suffix);
  console.log(`[setup] org id=${ctx.orgId} access len=${ctx.access.length} refresh len=${ctx.refresh && ctx.refresh.length}`);

  const locationIds = [];
  for (let i = 1; i <= 3; i++) locationIds.push(createSeedLocation(ctx.access, i));

  const assetIds = [];
  for (let i = 1; i <= 5; i++) assetIds.push(createSeedAsset(ctx.access, i));

  // One independent refresh chain per VU. The signup pair is chain 0; mint the
  // rest via login so no two VUs ever present the same single-use refresh token.
  const tokens = [{ access: ctx.access, refresh: ctx.refresh }];
  for (let i = 1; i < PEAK_VUS; i++) tokens.push(login(ctx.email));
  console.log(`[setup] minted ${tokens.length} independent token chains for ${PEAK_VUS} VUs`);
  console.log(`[setup] seeded ${locationIds.length} locations, ${assetIds.length} assets`);

  return { email: ctx.email, orgId: ctx.orgId, orgName: ctx.orgName, assetIds, locationIds, tokens };
}

// ---- per-VU mutable auth state (one JS realm per VU; persists across iters) ----
let vuAccess = null;
let vuRefresh = null;

function ensureAuth(data) {
  if (vuAccess === null) {
    const slot = data.tokens[(__VU - 1) % data.tokens.length];
    vuAccess = slot.access;
    vuRefresh = slot.refresh;
  }
}

// Exchange this VU's refresh token for a new access+refresh pair. Returns true
// on success and updates vuAccess/vuRefresh. k6 runs one iteration per VU at a
// time, so there is no intra-VU concurrent-refresh race.
function doRefresh() {
  const res = http.post(
    `${API}/auth/refresh`,
    JSON.stringify({ refresh_token: vuRefresh }),
    { headers: { 'Content-Type': 'application/json' }, tags: { kind: 'refresh' } }
  );
  if (res.status !== 200) {
    refreshFail.add(1);
    console.error(`[vu=${__VU}] refresh failed: ${res.status} ${res.body && res.body.slice(0, 200)}`);
    return false;
  }
  const b = res.json();
  if (!b || !b.access_token || !b.refresh_token) {
    refreshFail.add(1);
    console.error(`[vu=${__VU}] refresh ok but malformed body: ${res.body && res.body.slice(0, 200)}`);
    return false;
  }
  vuAccess = b.access_token;
  vuRefresh = b.refresh_token;
  refreshCount.add(1);
  return true;
}

// Run an HTTP thunk with the current access token; on 401, refresh once and
// retry. `fn(headers)` must perform and return the http response.
function withAuth(fn) {
  let res = fn(authHeaders(vuAccess));
  if (res.status === 401) {
    if (doRefresh()) {
      res = fn(authHeaders(vuAccess));
    }
  }
  return res;
}

// Weighted action picker — returns a key.
function pickAction() {
  const r = Math.random() * 100;
  if (r < 30) return 'list_assets';
  if (r < 50) return 'list_locations';
  if (r < 65) return 'get_asset';
  if (r < 80) return 'create_asset';
  if (r < 90) return 'delete_asset';
  return 'reports_current_locations';
}

// Track per-VU the asset IDs we created so we can delete one on the next cycle.
const vuCreated = [];

export default function (data) {
  ensureAuth(data);
  const action = pickAction();

  let res;
  let ok = false;

  if (action === 'list_assets') {
    res = withAuth((h) => http.get(`${API}/assets?limit=20`, { ...h, tags: { kind: 'read', op: 'list_assets' } }));
    listAssetsLat.add(res.timings.duration);
    ok = check(res, { 'list_assets 200': (r) => r.status === 200 });
  } else if (action === 'list_locations') {
    res = withAuth((h) => http.get(`${API}/locations?limit=20`, { ...h, tags: { kind: 'read', op: 'list_locations' } }));
    listLocationsLat.add(res.timings.duration);
    ok = check(res, { 'list_locations 200': (r) => r.status === 200 });
  } else if (action === 'get_asset') {
    const id = data.assetIds[randomIntBetween(0, data.assetIds.length - 1)];
    res = withAuth((h) => http.get(`${API}/assets/${id}`, { ...h, tags: { kind: 'read', op: 'get_asset' } }));
    getAssetLat.add(res.timings.duration);
    ok = check(res, { 'get_asset 200': (r) => r.status === 200 });
  } else if (action === 'create_asset') {
    const suffix = `${__VU}-${__ITER}-${randomString(4)}`;
    res = withAuth((h) => http.post(
      `${API}/assets`,
      JSON.stringify({
        name: `vu-asset-${suffix}`,
        external_key: `VU-ASSET-${suffix}`,
        is_active: true,
        valid_from: isoToday(),
        tags: [{ tag_type: 'rfid', value: `VU-EPC-${suffix}` }],
      }),
      { ...h, tags: { kind: 'write', op: 'create_asset' } }
    ));
    createAssetLat.add(res.timings.duration);
    ok = check(res, { 'create_asset 2xx': (r) => r.status === 200 || r.status === 201 });
    if (ok) {
      const newId = res.json().data && res.json().data.id;
      if (newId) vuCreated.push(newId);
    }
  } else if (action === 'delete_asset') {
    if (vuCreated.length === 0) {
      // Nothing to delete this cycle; do a light read instead so we still report.
      res = withAuth((h) => http.get(`${API}/assets?limit=5`, { ...h, tags: { kind: 'read', op: 'list_assets_fallback' } }));
      ok = check(res, { 'list_assets fallback 200': (r) => r.status === 200 });
    } else {
      const id = vuCreated.shift();
      res = withAuth((h) => http.del(`${API}/assets/${id}`, null, { ...h, tags: { kind: 'write', op: 'delete_asset' } }));
      ok = check(res, { 'delete_asset 2xx': (r) => r.status === 200 || r.status === 204 });
    }
  } else if (action === 'reports_current_locations') {
    res = withAuth((h) => http.get(`${API}/reports/asset-locations?limit=20`, { ...h, tags: { kind: 'read', op: 'reports_asset_locations' } }));
    reportsLat.add(res.timings.duration);
    ok = check(res, { 'reports_asset_locations 200': (r) => r.status === 200 });
  }

  okRate.add(ok);
  if (!ok) {
    errors.add(1, { op: action });
    console.warn(`[vu=${__VU} iter=${__ITER}] ${action} -> ${res && res.status}: ${res && res.body && res.body.slice(0, 200)}`);
  }

  // Small think-time so VUs aren't single-machine bound.
  sleep(Math.random() * 0.5);
}

export function teardown(data) {
  // The setup tokens are ~run-length old by now (and may be expired); mint a
  // fresh one to delete the org.
  console.log(`[teardown] deleting org ${data.orgId} (${data.orgName})`);
  let access;
  try {
    access = login(data.email).access;
  } catch (e) {
    console.error(`[teardown] re-login failed, org ${data.orgId} not deleted: ${e}`);
    return;
  }
  const res = http.del(
    `${API}/orgs/${data.orgId}`,
    JSON.stringify({ confirm_name: data.orgName }),
    authHeaders(access)
  );
  if (res.status !== 200 && res.status !== 204) {
    console.error(`[teardown] org delete failed: ${res.status} ${res.body}`);
  } else {
    console.log(`[teardown] org deleted ok`);
  }
}
