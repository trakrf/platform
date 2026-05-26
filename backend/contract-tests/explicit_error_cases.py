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


def call(
    method: str,
    path: str,
    body: Any | None = None,
    content_type: str = "application/json",
) -> tuple[int, dict | None]:
    url = f"{BASE_URL}{path}"
    data = None
    if body is not None:
        data = json.dumps(body).encode("utf-8")
    req = urllib.request.Request(url, data=data, method=method)
    req.add_header("Authorization", f"Bearer {API_KEY}")
    req.add_header("Content-Type", content_type)
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

    # --- invalid_value (variant): explicit null on non-nullable `name`.
    #     TRA-732 R2: null on a non-nullable field is `invalid_value`; the
    #     `required` code is reserved for the absent-key case (covered by
    #     the POST /assets {} probe above). ---
    status, body = call("POST", "/api/v1/assets", body={"name": None})
    record(observed, "POST /assets name=null → invalid_value on name", status, body)

    # --- too_short: empty-string on min_length 1 ---
    status, body = call("POST", "/api/v1/assets", body={"name": ""})
    record(observed, "POST /assets name='' → too_short on name", status, body)

    # --- too_long: over max_length 255 ---
    status, body = call("POST", "/api/v1/assets", body={"name": "x" * 256})
    record(observed, "POST /assets name=256x → too_long on name", status, body)

    # --- too_small: numeric below min=1. TRA-734 (BB40 F3) moved this off
    # POST /assets (location_id no longer settable); POST /locations carries
    # the same numeric min on parent_id. ---
    status, body = call(
        "POST",
        "/api/v1/locations",
        body={"name": "TRA-692 too_small probe", "parent_id": 0},
    )
    record(observed, "POST /locations parent_id=0 → too_small on parent_id", status, body)

    # --- too_large: list-param `limit` above its hard ceiling. TRA-720
    # removed the int32 cap on surrogate id columns, so parent_id no
    # longer carries an upper bound; the surviving numeric `max`
    # constraint that emits too_large is the maxListLimit on GET ?limit=.
    status, body = call("GET", "/api/v1/assets?limit=99999")
    record(observed, "GET /assets?limit=99999 → too_large on limit", status, body)

    # --- invalid_value: bad RFC 3339 valid_from ---
    status, body = call(
        "POST",
        "/api/v1/assets",
        body={"name": "TRA-692 invalid_value probe", "valid_from": "not-a-date"},
    )
    record(observed, "POST /assets valid_from=garbage → invalid_value", status, body)

    # --- fk_not_found: reference a non-existent parent location. TRA-734
    # (BB40 F3) moved this off POST /assets — asset location is no longer
    # settable on Create — but the location-hierarchy parent FK still uses
    # the same resolution path. ---
    status, body = call(
        "POST",
        "/api/v1/locations",
        body={
            "name": "TRA-692 fk_not_found probe",
            "parent_external_key": "does-not-exist-zzz-TRA-692",
        },
    )
    record(observed, "POST /locations parent_external_key=missing → fk_not_found", status, body)

    # --- ambiguous_fields: send both surrogate and natural-key parent refs.
    # TRA-734 (BB40 F3) moved this off POST /assets; POST /locations still
    # carries the mutually-exclusive parent_id / parent_external_key pair. ---
    status, body = call(
        "POST",
        "/api/v1/locations",
        body={
            "name": "TRA-692 ambiguous_fields probe",
            "parent_id": 1,
            "parent_external_key": SEED_LOCATION_EXTERNAL_KEY,
        },
    )
    record(observed, "POST /locations both parent fields → ambiguous_fields", status, body)

    # --- unknown_field: strict decoder rejects unrecognised body keys ---
    status, body = call(
        "POST",
        "/api/v1/assets",
        body={"name": "TRA-692 unknown_field probe", "totally_unknown_TRA_692": 1},
    )
    record(observed, "POST /assets unknown body field → unknown_field", status, body)

    # --- invalid_context: include_deleted on a detail endpoint (TRA-777 / BB62
    #     F3). The same parameter is accepted on the list sibling, so the
    #     rejection carries a specialized diagnostic with the
    #     `invalid_context` code instead of falling into the generic
    #     `unknown_field` bucket.
    status, body = call("GET", "/api/v1/assets/1?include_deleted=true")
    record(
        observed,
        "GET /assets/1?include_deleted=true → invalid_context",
        status,
        body,
    )

    # --- read_only: PATCH attempting to mutate a truly server-managed field ---
    # `id` is server-assigned and immutable with no public mutation path, so
    # a differing value returns `code: read_only` (the "server-managed"
    # branch of the TRA-780 F4 split — `external_key` and `tags` now emit
    # `invalid_context` instead, since they're mutable via a sub-resource
    # verb). PATCH requires application/merge-patch+json per the public
    # spec; sending plain application/json returns 415 with no fields[],
    # which fails coverage.
    aid = resolve_seed_asset_id()
    if aid is not None:
        status, body = call(
            "PATCH",
            f"/api/v1/assets/{aid}",
            body={"id": int(aid) + 99999},
            content_type="application/merge-patch+json",
        )
        record(
            observed,
            f"PATCH /assets/{aid} differing id → read_only",
            status,
            body,
        )

        # --- invalid_context: PATCH attempting to mutate external_key
        # directly — emits `code: invalid_context` post-TRA-780 F4 since the
        # field is mutable via POST /rename, not via PATCH. Complements the
        # query-param case above (include_deleted on a detail endpoint) so
        # both surfaces — body field and query param — are deterministically
        # covered.
        status, body = call(
            "PATCH",
            f"/api/v1/assets/{aid}",
            body={"external_key": "TRA-780-blocked"},
            content_type="application/merge-patch+json",
        )
        record(
            observed,
            f"PATCH /assets/{aid} external_key=… → invalid_context",
            status,
            body,
        )
    else:
        # Fallback: PATCH against external-key alias path if the API exposes
        # one, else log skip — coverage check will flag the gap.
        print(
            "[read_only/invalid_context PATCH] no seed asset resolvable; skipping cases (gate will flag)",
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
