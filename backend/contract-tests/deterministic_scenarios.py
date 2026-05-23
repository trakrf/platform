#!/usr/bin/env python3
# TRA-800: Deterministic multi-step scenarios that Schemathesis can't compose.
#
# Schemathesis is generator-driven; it can't reliably build the
# "asset-scanned-into-location, then DELETE that location" flow that crosses
# two endpoints. The location-delete guard (CountActiveAssetsAtLocation,
# TRA-644 / BB22 F2) was silently disabled for all modern assets the moment
# TRA-734 nulled current_location_id on new assets, and ~30 BB cycles missed
# it because nothing in the recurring suite exercised DELETE against a
# location with a populated scan. This script closes that gap.
#
# The placement (ASSET-0001 scanned at LOC-0001) is established by the
# contract-test seed at backend/database/seeds/contract_test_seed.sql; this
# script asserts the guard fires on DELETE. Keeping placement in the seed
# rather than calling /api/v1/inventory/save here avoids widening the
# minted-key scope set (scans:write is internal-only by design).
#
# Exits 0 on success, 1 on any assertion miss. Runs before Schemathesis so a
# guard regression fails this gate before the broader fuzz run.

from __future__ import annotations

import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from typing import Any

BASE_URL = os.environ["BASE_URL"].rstrip("/")
API_KEY = os.environ["API_KEY"]

# Seed fixture from backend/database/seeds/contract_test_seed.sql. LOC-0001 is
# intentionally NOT referenced as a spec example (WHS-01 and wh1 are), so the
# pre-seeded scan on it does not change Schemathesis's behavior against the
# example surface.
SCENARIO_LOCATION_EXTERNAL_KEY = "LOC-0001"


def call(
    method: str,
    path: str,
    body: Any | None = None,
) -> tuple[int, dict | None]:
    url = f"{BASE_URL}{path}"
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", f"Bearer {API_KEY}")
    req.add_header("Content-Type", "application/json")
    try:
        with urllib.request.urlopen(req, timeout=10) as resp:
            return resp.status, _safe_json(resp.read())
    except urllib.error.HTTPError as exc:
        return exc.code, _safe_json(exc.read())


def _safe_json(b: bytes) -> dict | None:
    if not b:
        return None
    try:
        return json.loads(b)
    except Exception:
        return None


def fail(case: str, msg: str, status: int | None = None, body: dict | None = None) -> None:
    bits = [f"[{case}] {msg}"]
    if status is not None:
        bits.append(f"status={status}")
    if body is not None:
        bits.append(f"body={json.dumps(body)}")
    print(" ".join(bits), file=sys.stderr)


def resolve_location_id(external_key: str) -> int | None:
    qs = urllib.parse.urlencode({"external_key": external_key})
    status, body = call("GET", f"/api/v1/locations?{qs}")
    if status != 200 or not body:
        return None
    data = body.get("data") or []
    if not data:
        return None
    lid = data[0].get("id")
    return int(lid) if lid is not None else None


# TRA-800 / BB22 F2 regression scenario: a location with an asset scanned into
# it (seeded) must refuse DELETE with 409 conflict. A 204 here means the
# guard regressed to vacuous-pass — the failure mode after TRA-734 nulled
# current_location_id.
def scenario_delete_blocked_by_active_scan() -> bool:
    case = "asset-scanned-at-location → DELETE → 409"

    location_id = resolve_location_id(SCENARIO_LOCATION_EXTERNAL_KEY)
    if location_id is None:
        fail(case, f"could not resolve seed location {SCENARIO_LOCATION_EXTERNAL_KEY}")
        return False

    status, body = call("DELETE", f"/api/v1/locations/{location_id}")
    if status != 409:
        fail(
            case,
            "DELETE on location with active scanned asset must be 409 — guard "
            "regressed to vacuous-pass (see TRA-734 incident)",
            status,
            body,
        )
        return False

    err = (body or {}).get("error") or {}
    if err.get("type") != "conflict":
        fail(case, "expected error.type=conflict", status, body)
        return False
    detail = err.get("detail") or ""
    if "assets" not in detail.lower():
        fail(case, "expected detail to mention 'assets'", status, body)
        return False

    return True


def main() -> int:
    scenarios = [
        scenario_delete_blocked_by_active_scan,
    ]
    failed = 0
    for scenario in scenarios:
        ok = scenario()
        status_str = "PASS" if ok else "FAIL"
        print(f"[{status_str}] {scenario.__name__}")
        if not ok:
            failed += 1
    if failed:
        print(f"❌ deterministic scenarios: {failed} failed", file=sys.stderr)
        return 1
    print("✅ deterministic scenarios: all passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
