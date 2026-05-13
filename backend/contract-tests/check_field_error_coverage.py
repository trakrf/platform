#!/usr/bin/env python3
# TRA-692: Asserts every value in the OpenAPI FieldErrorCode enum was
# observed at least once in a real validation_error response during the
# contract-test run. Exits non-zero with the missing list when coverage
# is incomplete.
#
# Reads the source-of-truth FieldErrorCode enum from the OpenAPI spec and
# the observed codes from a JSONL file written by explicit_error_cases.py
# (one row per observed FieldError, with a string "code" key). Schemathesis
# fuzz responses are NOT consumed — the deterministic explicit-cases file
# is the contract, by design.

from __future__ import annotations
import argparse
import json
import sys
from pathlib import Path

import yaml


def declared_codes(spec_path: Path) -> set[str]:
    with spec_path.open() as f:
        spec = yaml.safe_load(f)
    schemas = (spec.get("components") or {}).get("schemas") or {}
    fec = schemas.get("FieldErrorCode") or {}
    enum = fec.get("enum") or []
    if not enum:
        raise SystemExit(
            f"FieldErrorCode.enum not found in {spec_path}; expected at "
            "components.schemas.FieldErrorCode.enum"
        )
    return set(enum)


def observed_codes(observed_path: Path) -> set[str]:
    codes: set[str] = set()
    with observed_path.open() as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                row = json.loads(line)
            except json.JSONDecodeError:
                continue
            code = row.get("code")
            if isinstance(code, str):
                codes.add(code)
    return codes


def main() -> int:
    ap = argparse.ArgumentParser(
        description="Assert every FieldErrorCode enum value was observed at least once."
    )
    ap.add_argument("--spec", required=True, type=Path)
    ap.add_argument("--observed", required=True, type=Path)
    ap.add_argument(
        "--allow-missing",
        default="",
        help="Comma-separated enum values exempt from coverage. Document each "
        "entry inline at the call site (justfile) — exemptions are a smell.",
    )
    args = ap.parse_args()

    declared = declared_codes(args.spec)
    observed = observed_codes(args.observed)
    allowed_missing = {c.strip() for c in args.allow_missing.split(",") if c.strip()}

    unexpected_allowed = allowed_missing - declared
    if unexpected_allowed:
        print(
            f"❌ --allow-missing contains codes not in FieldErrorCode.enum: "
            f"{sorted(unexpected_allowed)}",
            file=sys.stderr,
        )
        return 1

    missing = (declared - observed) - allowed_missing
    if missing:
        print(
            f"❌ FieldErrorCode enum coverage gap — declared but never "
            f"observed: {sorted(missing)}",
            file=sys.stderr,
        )
        print(f"   spec:     {args.spec}", file=sys.stderr)
        print(
            f"   observed: {args.observed} ({len(observed)} distinct codes)",
            file=sys.stderr,
        )
        return 1

    covered = declared & observed
    print(
        f"✅ FieldErrorCode coverage: {len(covered)}/{len(declared)} enum values observed"
    )
    if allowed_missing:
        print(f"   allow-list: {sorted(allowed_missing)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
