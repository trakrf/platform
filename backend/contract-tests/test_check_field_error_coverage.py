"""Unit tests for the FieldErrorCode coverage gate."""

from __future__ import annotations
import json
import subprocess
import sys
from pathlib import Path

import yaml

THIS = Path(__file__).parent
SCRIPT = THIS / "check_field_error_coverage.py"


def write_spec(tmp_path: Path, codes: list[str]) -> Path:
    spec = {
        "openapi": "3.1.0",
        "components": {
            "schemas": {"FieldErrorCode": {"type": "string", "enum": list(codes)}},
        },
    }
    p = tmp_path / "spec.yaml"
    p.write_text(yaml.safe_dump(spec))
    return p


def write_observed(tmp_path: Path, observed_codes: list[str]) -> Path:
    p = tmp_path / "observed.jsonl"
    p.write_text(
        "\n".join(
            json.dumps({"code": c, "field": "f", "case": "t"}) for c in observed_codes
        )
    )
    return p


def run(spec: Path, observed: Path, allowlist: list[str] | None = None) -> subprocess.CompletedProcess[str]:
    cmd = [sys.executable, str(SCRIPT), "--spec", str(spec), "--observed", str(observed)]
    if allowlist:
        cmd += ["--allow-missing", ",".join(allowlist)]
    return subprocess.run(cmd, capture_output=True, text=True)


def test_full_coverage_passes(tmp_path: Path) -> None:
    spec = write_spec(tmp_path, ["required", "invalid_value"])
    observed = write_observed(tmp_path, ["required", "invalid_value"])
    r = run(spec, observed)
    assert r.returncode == 0, r.stderr + r.stdout
    assert "2/2" in r.stdout


def test_missing_codes_fail_and_list_in_stderr(tmp_path: Path) -> None:
    spec = write_spec(tmp_path, ["required", "invalid_value", "fk_not_found"])
    observed = write_observed(tmp_path, ["required"])
    r = run(spec, observed)
    assert r.returncode != 0
    assert "invalid_value" in r.stderr
    assert "fk_not_found" in r.stderr


def test_allowlist_skips_a_code(tmp_path: Path) -> None:
    spec = write_spec(tmp_path, ["required", "too_small"])
    observed = write_observed(tmp_path, ["required"])
    r = run(spec, observed, allowlist=["too_small"])
    assert r.returncode == 0, r.stderr + r.stdout


def test_allowlist_with_unknown_code_fails(tmp_path: Path) -> None:
    spec = write_spec(tmp_path, ["required"])
    observed = write_observed(tmp_path, ["required"])
    r = run(spec, observed, allowlist=["nonexistent_code"])
    assert r.returncode != 0
    assert "nonexistent_code" in r.stderr


def test_empty_observed_file_fails(tmp_path: Path) -> None:
    spec = write_spec(tmp_path, ["required"])
    observed = tmp_path / "empty.jsonl"
    observed.write_text("")
    r = run(spec, observed)
    assert r.returncode != 0
    assert "required" in r.stderr


def test_missing_enum_in_spec_errors(tmp_path: Path) -> None:
    spec = tmp_path / "spec.yaml"
    spec.write_text("openapi: 3.1.0\n")  # no FieldErrorCode
    observed = write_observed(tmp_path, ["required"])
    r = run(spec, observed)
    assert r.returncode != 0
    assert "FieldErrorCode" in r.stderr
