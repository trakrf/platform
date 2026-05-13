#!/usr/bin/env python3
# TRA-692: Deterministic supplementary cases that provoke each FieldErrorCode
# enum value at least once. Run before Schemathesis against the same server.
# Output: $OUT_DIR/observed_codes.jsonl — one JSON line per observed
# FieldError, structured as {"code": str, "field": str, "case": str}.
#
# These cases exist because Schemathesis fuzz coverage of every enum value
# is non-deterministic across runs/versions; the §1.2 ticket requires a
# stable assertion that each declared code can actually be emitted. The
# coverage gate (check_field_error_coverage.py) reads ONLY this file —
# Schemathesis stays the broader generator-driven layer.

from __future__ import annotations
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path
from typing import Any

BASE_URL = os.environ["BASE_URL"].rstrip("/")
API_KEY = os.environ["API_KEY"]
OUT_DIR = Path(os.environ["OUT_DIR"])
OUT_DIR.mkdir(parents=True, exist_ok=True)
OBSERVED_PATH = OUT_DIR / "observed_codes.jsonl"

# Seed fixtures from backend/database/seeds/contract_test_seed.sql:
#   locations: LOC-0001, WHS-01, wh1
#   asset:     ASSET-0001 (in LOC-0001)
SEED_LOCATION_EXTERNAL_KEY = "WHS-01"
SEED_ASSET_EXTERNAL_KEY = "ASSET-0001"


def call(method: str, path: str, body: Any | None = None) -> tuple[int, dict | None]:
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


def record(observed: list[dict], case_name: str, status: int, body: dict | None) -> None:
    if not body:
        print(f"[{case_name}] status={status} (no JSON body)", file=sys.stderr)
        return
    err = body.get("error") or {}
    fields = err.get("fields") or []
    if not fields:
        print(
            f"[{case_name}] status={status} type={err.get('type')} title={err.get('title')} (no fields[])",
            file=sys.stderr,
        )
        return
    for f in fields:
        code = f.get("code")
        if code:
            observed.append({"code": code, "field": f.get("field"), "case": case_name})


def resolve_seed_asset_id() -> str | None:
    qs = urllib.parse.urlencode({"external_key": SEED_ASSET_EXTERNAL_KEY})
    status, body = call("GET", f"/api/v1/assets?{qs}")
    if status != 200 or not body:
        print(
            f"could not resolve seed asset id via GET (status={status}); "
            "read_only case will use the external_key directly and may fall back to 404",
            file=sys.stderr,
        )
        return None
    data = body.get("data") or []
    if not data:
        return None
    aid = data[0].get("id")
    return str(aid) if aid is not None else None


def main() -> int:
    observed: list[dict] = []

    # --- required: omitted required `name` field on POST /assets ---
    status, body = call("POST", "/api/v1/assets", body={})
    record(observed, "POST /assets {} → required on name", status, body)

    # --- required (variant): explicit null on non-nullable `name` ---
    status, body = call("POST", "/api/v1/assets", body={"name": None})
    record(observed, "POST /assets name=null → required on name", status, body)

    # --- too_short: empty-string on min_length 1 ---
    status, body = call("POST", "/api/v1/assets", body={"name": ""})
    record(observed, "POST /assets name='' → too_short on name", status, body)

    # --- too_long: over max_length 255 ---
    status, body = call("POST", "/api/v1/assets", body={"name": "x" * 256})
    record(observed, "POST /assets name=256x → too_long on name", status, body)

    # --- too_small: numeric below min=1 ---
    status, body = call(
        "POST",
        "/api/v1/assets",
        body={"name": "TRA-692 too_small probe", "location_id": 0},
    )
    record(observed, "POST /assets location_id=0 → too_small on location_id", status, body)

    # --- too_large: numeric above max=2147483647 ---
    status, body = call(
        "POST",
        "/api/v1/assets",
        body={"name": "TRA-692 too_large probe", "location_id": 9999999999},
    )
    record(observed, "POST /assets location_id=9999999999 → too_large on location_id", status, body)

    # --- invalid_value: bad RFC 3339 valid_from ---
    status, body = call(
        "POST",
        "/api/v1/assets",
        body={"name": "TRA-692 invalid_value probe", "valid_from": "not-a-date"},
    )
    record(observed, "POST /assets valid_from=garbage → invalid_value", status, body)

    # --- fk_not_found: reference a non-existent location ---
    status, body = call(
        "POST",
        "/api/v1/assets",
        body={
            "name": "TRA-692 fk_not_found probe",
            "location_external_key": "does-not-exist-zzz-TRA-692",
        },
    )
    record(observed, "POST /assets location_external_key=missing → fk_not_found", status, body)

    # --- ambiguous_fields: send both surrogate and natural-key location refs ---
    status, body = call(
        "POST",
        "/api/v1/assets",
        body={
            "name": "TRA-692 ambiguous_fields probe",
            "location_id": 1,
            "location_external_key": SEED_LOCATION_EXTERNAL_KEY,
        },
    )
    record(observed, "POST /assets both location fields → ambiguous_fields", status, body)

    # --- unknown_field: strict decoder rejects unrecognised body keys ---
    status, body = call(
        "POST",
        "/api/v1/assets",
        body={"name": "TRA-692 unknown_field probe", "totally_unknown_TRA_692": 1},
    )
    record(observed, "POST /assets unknown body field → unknown_field", status, body)

    # --- read_only: PATCH attempting to mutate external_key directly ---
    aid = resolve_seed_asset_id()
    if aid is not None:
        status, body = call(
            "PATCH",
            f"/api/v1/assets/{aid}",
            body={"external_key": "TRA-692-blocked"},
        )
        record(
            observed,
            f"PATCH /assets/{aid} external_key=… → read_only",
            status,
            body,
        )
    else:
        # Fallback: PATCH against external-key alias path if the API exposes
        # one, else log skip — coverage check will flag the gap.
        print(
            "[read_only] no seed asset resolvable; skipping case (gate will flag)",
            file=sys.stderr,
        )

    with OBSERVED_PATH.open("w", encoding="utf-8") as f:
        for r in observed:
            f.write(json.dumps(r) + "\n")

    distinct = sorted({r["code"] for r in observed})
    print(f"wrote {len(observed)} observations ({len(distinct)} distinct codes) → {OBSERVED_PATH}")
    print(f"distinct codes: {distinct}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
