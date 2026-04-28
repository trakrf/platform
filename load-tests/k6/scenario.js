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
//              and 5 assets, and hands every VU the same auth token (shared-org
//              scenario — no per-VU signup overhead).
//   default()  each iteration picks a weighted hot-path action (list/get/create/
//              delete) and records latency. New asset IDs are deleted on the next
//              iteration cycle so the org doesn't grow unbounded.
//   teardown() deletes the org via DELETE /orgs/:id with confirm_name.

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';
import { randomString, randomIntBetween } from 'https://jslib.k6.io/k6-utils/1.4.0/index.js';

const BASE_URL = (__ENV.BASE_URL || 'http://localhost:8080').replace(/\/$/, '');
const API = `${BASE_URL}/api/v1`;
const PEAK_VUS = parseInt(__ENV.PEAK_VUS || '20', 10);
const STAGE_S = parseInt(__ENV.STAGE_S || '60', 10);

const errors = new Counter('biz_errors');
const okRate = new Rate('biz_ok');
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
    http_req_failed: ['rate<0.01'],
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
  return new Date().toISOString().split('T')[0];
}

// Signup a fresh org+user and return { token, orgId, orgName }.
function signup(suffix) {
  const orgName = `loadtest-${suffix}`;
  const email = `loadtest-${suffix}@example.com`;
  const password = 'LoadTest123!';

  const res = http.post(
    `${API}/auth/signup`,
    JSON.stringify({ email, password, org_name: orgName }),
    { headers: { 'Content-Type': 'application/json' }, tags: { kind: 'setup' } }
  );

  if (res.status !== 200 && res.status !== 201) {
    throw new Error(`signup failed: ${res.status} ${res.body}`);
  }
  const body = res.json();
  const token = body && body.data && body.data.token;
  if (!token) throw new Error(`signup ok but no token: ${res.body}`);

  // Resolve org id from /users/me.
  const me = http.get(`${API}/users/me`, authHeaders(token));
  if (me.status !== 200) throw new Error(`/users/me failed: ${me.status} ${me.body}`);
  const orgId = me.json().data.current_org.id;

  return { token, orgId, orgName, email };
}

function createSeedLocation(token, n) {
  const res = http.post(
    `${API}/locations`,
    JSON.stringify({
      name: `seed-loc-${n}`,
      identifier: `SEED-LOC-${n}`,
      is_active: true,
      valid_from: isoToday(),
      tags: [],
    }),
    { ...authHeaders(token), tags: { kind: 'setup' } }
  );
  if (res.status !== 200 && res.status !== 201) {
    throw new Error(`seed location ${n} failed: ${res.status} ${res.body}`);
  }
  return res.json().data.surrogate_id;
}

function createSeedAsset(token, n) {
  const res = http.post(
    `${API}/assets`,
    JSON.stringify({
      name: `seed-asset-${n}`,
      identifier: `SEED-ASSET-${n}`,
      type: 'asset',
      is_active: true,
      valid_from: isoToday(),
      tags: [{ type: 'rfid', value: `SEED-EPC-${n}-${randomString(8)}` }],
    }),
    { ...authHeaders(token), tags: { kind: 'setup' } }
  );
  if (res.status !== 200 && res.status !== 201) {
    throw new Error(`seed asset ${n} failed: ${res.status} ${res.body}`);
  }
  return res.json().data.surrogate_id;
}

export function setup() {
  const suffix = `${Date.now()}-${randomString(6)}`;
  console.log(`[setup] signing up org ${suffix}`);
  const ctx = signup(suffix);
  console.log(`[setup] org id=${ctx.orgId} token len=${ctx.token.length}`);

  const locationIds = [];
  for (let i = 1; i <= 3; i++) locationIds.push(createSeedLocation(ctx.token, i));

  const assetIds = [];
  for (let i = 1; i <= 5; i++) assetIds.push(createSeedAsset(ctx.token, i));

  console.log(`[setup] seeded ${locationIds.length} locations, ${assetIds.length} assets`);
  return { ...ctx, assetIds, locationIds };
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
  const auth = authHeaders(data.token);
  const action = pickAction();

  let res;
  let ok = false;

  if (action === 'list_assets') {
    res = http.get(`${API}/assets?limit=20`, { ...auth, tags: { kind: 'read', op: 'list_assets' } });
    listAssetsLat.add(res.timings.duration);
    ok = check(res, { 'list_assets 200': (r) => r.status === 200 });
  } else if (action === 'list_locations') {
    res = http.get(`${API}/locations?limit=20`, { ...auth, tags: { kind: 'read', op: 'list_locations' } });
    listLocationsLat.add(res.timings.duration);
    ok = check(res, { 'list_locations 200': (r) => r.status === 200 });
  } else if (action === 'get_asset') {
    const id = data.assetIds[randomIntBetween(0, data.assetIds.length - 1)];
    res = http.get(`${API}/assets/by-id/${id}`, { ...auth, tags: { kind: 'read', op: 'get_asset' } });
    getAssetLat.add(res.timings.duration);
    ok = check(res, { 'get_asset 200': (r) => r.status === 200 });
  } else if (action === 'create_asset') {
    const suffix = `${__VU}-${__ITER}-${randomString(4)}`;
    res = http.post(
      `${API}/assets`,
      JSON.stringify({
        name: `vu-asset-${suffix}`,
        identifier: `VU-ASSET-${suffix}`,
        type: 'asset',
        is_active: true,
        valid_from: isoToday(),
        tags: [{ type: 'rfid', value: `VU-EPC-${suffix}` }],
      }),
      { ...auth, tags: { kind: 'write', op: 'create_asset' } }
    );
    createAssetLat.add(res.timings.duration);
    ok = check(res, { 'create_asset 2xx': (r) => r.status === 200 || r.status === 201 });
    if (ok) {
      const newId = res.json().data && res.json().data.surrogate_id;
      if (newId) vuCreated.push(newId);
    }
  } else if (action === 'delete_asset') {
    if (vuCreated.length === 0) {
      // Nothing to delete this cycle; do a light read instead so we still report.
      res = http.get(`${API}/assets?limit=5`, { ...auth, tags: { kind: 'read', op: 'list_assets_fallback' } });
      ok = check(res, { 'list_assets fallback 200': (r) => r.status === 200 });
    } else {
      const id = vuCreated.shift();
      res = http.del(`${API}/assets/by-id/${id}`, null, { ...auth, tags: { kind: 'write', op: 'delete_asset' } });
      ok = check(res, { 'delete_asset 2xx': (r) => r.status === 200 || r.status === 204 });
    }
  } else if (action === 'reports_current_locations') {
    res = http.get(`${API}/locations/current?limit=20`, {
      ...auth,
      tags: { kind: 'read', op: 'locations_current' },
    });
    reportsLat.add(res.timings.duration);
    ok = check(res, { 'locations_current 200': (r) => r.status === 200 });
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
  // Best-effort: delete assets we created during the run, then the org.
  console.log(`[teardown] deleting org ${data.orgId} (${data.orgName})`);
  const res = http.del(
    `${API}/orgs/${data.orgId}`,
    JSON.stringify({ confirm_name: data.orgName }),
    authHeaders(data.token)
  );
  if (res.status !== 200 && res.status !== 204) {
    console.error(`[teardown] org delete failed: ${res.status} ${res.body}`);
  } else {
    console.log(`[teardown] org deleted ok`);
  }
}
