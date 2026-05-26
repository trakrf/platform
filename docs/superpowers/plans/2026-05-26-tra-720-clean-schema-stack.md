# TRA-720 Clean Schema Stack Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current 44-file migration stack with a clean 10-file stack that defines the canonical end-state schema with bigint surrogate PKs/FKs, a single keyed-Feistel ID generator, and a surrogate key on `tag_scans` (folding TRA-836). Also remove all Go-side `int32`-ceiling plumbing.

**Architecture:** Up-only migration files numbered `000001`–`000010`, organized by concern not chronology. A new `trakrf.generate_obfuscated_id()` PL/pgSQL function implements a keyed Feistel (50-bit block, 6 rounds, HMAC-SHA256 round function), output OR'd with `(1::bigint << 50)` so new IDs land disjoint from migrated `[1, 2^31)` IDs. Go reference at `backend/internal/obfuscatedid/` exists for test-vector parity. Old migrations deleted from the directory; git tag `pre-tra-720` preserves history.

**Tech Stack:** PostgreSQL 16, TimescaleDB extension, pgcrypto extension, golang-migrate, Go 1.22+, pgx/v5, just task runner, docker-compose for local DB.

**Reference:** See [TRA-720 design doc](../specs/2026-05-26-tra-720-clean-schema-stack-design.md) for full design rationale.

---

## Pre-flight check

Before starting Task 1, verify environment:

```bash
docker compose version          # docker compose v2+
go version                       # 1.22+
just --version
which migrate                    # golang-migrate CLI; install: brew install golang-migrate
```

Confirm the local DB is reachable:

```bash
just database up
psql "$PG_URL_LOCAL" -c "SELECT version();"
```

Confirm Linear ticket access (for posting handoff comment to TRA-810 later):

```bash
echo "TRA-720 implementation starting" # No actual posting; just verify mental model
```

---

## Task 1: Bootstrap `obfuscatedid` Go package with first test vector

**Files:**
- Create: `backend/internal/obfuscatedid/obfuscatedid.go`
- Create: `backend/internal/obfuscatedid/obfuscatedid_test.go`
- Create: `backend/internal/obfuscatedid/testdata/.gitkeep`

- [ ] **Step 1: Create the package skeleton**

`backend/internal/obfuscatedid/obfuscatedid.go`:
```go
// Package obfuscatedid implements the keyed Feistel ID generator used by
// trakrf.generate_obfuscated_id() in the database. This Go implementation is
// the reference oracle for test vectors and the PL/pgSQL parity check.
//
// Construction: 50-bit Feistel (2 x 25-bit halves), 6 rounds, HMAC-SHA256 round
// function truncated to 25 bits, output OR'd with (1 << 50) so the value lands
// in [2^50, 2^51) — disjoint from the migrated 31-bit ID range.
package obfuscatedid

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

const (
	BlockBits = 50
	HalfBits  = 25
	Rounds    = 6
	Mask25    = (uint64(1) << HalfBits) - 1
	HighBit   = uint64(1) << BlockBits
)

// Encrypt maps a sequence value into a 51-bit obfuscated ID. seqValue must be
// less than 2^50 (the Feistel block size).
func Encrypt(masterKey []byte, seqValue uint64) (uint64, error) {
	if seqValue >= HighBit {
		return 0, fmt.Errorf("sequence overflow: %d >= 2^%d", seqValue, BlockBits)
	}
	L := (seqValue >> HalfBits) & Mask25
	R := seqValue & Mask25
	for i := 1; i <= Rounds; i++ {
		rk := roundKey(masterKey, i)
		L, R = R, L^f(rk, R)
	}
	return ((L << HalfBits) | R) | HighBit, nil
}

func roundKey(masterKey []byte, round int) []byte {
	h := hmac.New(sha256.New, masterKey)
	fmt.Fprintf(h, "round-%d", round)
	return h.Sum(nil)
}

func f(roundKey []byte, x uint64) uint64 {
	h := hmac.New(sha256.New, roundKey)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], x)
	h.Write(buf[:])
	sum := h.Sum(nil)
	// Take first 4 bytes as big-endian uint32, then mask to 25 bits.
	return uint64(binary.BigEndian.Uint32(sum[0:4])) & Mask25
}
```

- [ ] **Step 2: Write the first failing test**

`backend/internal/obfuscatedid/obfuscatedid_test.go`:
```go
package obfuscatedid

import (
	"encoding/hex"
	"testing"
)

const testMasterKeyHex = "6f626675736361746f72746573746b657920303132333435363738396162636465"

func mustDecodeHex(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("decode hex: %v", err)
	}
	return b
}

func TestEncrypt_HighBitAlwaysSet(t *testing.T) {
	key := mustDecodeHex(t, testMasterKeyHex)
	for _, seq := range []uint64{1, 2, 100, 1 << 24, (1 << 50) - 1} {
		id, err := Encrypt(key, seq)
		if err != nil {
			t.Fatalf("Encrypt(%d): unexpected error %v", seq, err)
		}
		if id < (1 << 50) {
			t.Errorf("Encrypt(%d) = %d, expected >= 2^50", seq, id)
		}
		if id >= (1 << 51) {
			t.Errorf("Encrypt(%d) = %d, expected < 2^51", seq, id)
		}
	}
}

func TestEncrypt_OverflowError(t *testing.T) {
	key := mustDecodeHex(t, testMasterKeyHex)
	_, err := Encrypt(key, 1<<50)
	if err == nil {
		t.Error("Encrypt(2^50) should return overflow error")
	}
}
```

`backend/internal/obfuscatedid/testdata/.gitkeep`: empty file to keep the dir in git.

- [ ] **Step 3: Run tests to verify they pass**

```bash
cd backend && go test ./internal/obfuscatedid/...
```

Expected: PASS for both `TestEncrypt_HighBitAlwaysSet` and `TestEncrypt_OverflowError`.

- [ ] **Step 4: Add bijection test**

Append to `obfuscatedid_test.go`:
```go
func TestEncrypt_Bijection(t *testing.T) {
	key := mustDecodeHex(t, testMasterKeyHex)
	const N = 10_000
	seen := make(map[uint64]uint64, N)
	for seq := uint64(1); seq <= N; seq++ {
		id, err := Encrypt(key, seq)
		if err != nil {
			t.Fatalf("Encrypt(%d): %v", seq, err)
		}
		if prev, ok := seen[id]; ok {
			t.Fatalf("collision: Encrypt(%d) == Encrypt(%d) == %d", seq, prev, id)
		}
		seen[id] = seq
	}
}
```

Run: `go test ./internal/obfuscatedid/...` — expect PASS.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/obfuscatedid/
git commit -m "$(cat <<'EOF'
feat(obfuscatedid): Go reference implementation of keyed Feistel

50-bit block (2 x 25-bit halves), 6 rounds, HMAC-SHA256 round function
truncated to 25 bits, output OR'd with (1 << 50) for disjoint domain
from migrated 31-bit IDs. Includes high-bit, overflow, and bijection
tests over [1, 10000].

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 2: Generate blessed test vectors

**Files:**
- Create: `backend/internal/obfuscatedid/cmd/genvectors/main.go`
- Create: `backend/internal/obfuscatedid/testdata/vectors.json`

- [ ] **Step 1: Write the vector generator CLI**

`backend/internal/obfuscatedid/cmd/genvectors/main.go`:
```go
// genvectors writes blessed Feistel test vectors to testdata/vectors.json.
// Run via: go run ./internal/obfuscatedid/cmd/genvectors > internal/obfuscatedid/testdata/vectors.json
package main

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"os"

	"github.com/trakrf/platform/backend/internal/obfuscatedid"
)

type Vector struct {
	Seq      uint64 `json:"seq"`
	Expected uint64 `json:"expected"`
}

type Bundle struct {
	MasterKeyHex string   `json:"master_key_hex"`
	Vectors      []Vector `json:"vectors"`
}

func main() {
	const keyHex = "6f626675736361746f72746573746b657920303132333435363738396162636465"
	key, err := hex.DecodeString(keyHex)
	if err != nil {
		log.Fatalf("decode hex: %v", err)
	}
	seqs := []uint64{1, 2, 100, 12345, 1 << 24, 1 << 25, (1 << 49)}
	vectors := make([]Vector, 0, len(seqs))
	for _, s := range seqs {
		id, err := obfuscatedid.Encrypt(key, s)
		if err != nil {
			log.Fatalf("Encrypt(%d): %v", s, err)
		}
		vectors = append(vectors, Vector{Seq: s, Expected: id})
	}
	bundle := Bundle{MasterKeyHex: keyHex, Vectors: vectors}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(bundle); err != nil {
		log.Fatalf("encode: %v", err)
	}
}
```

- [ ] **Step 2: Generate the vectors**

```bash
cd backend && go run ./internal/obfuscatedid/cmd/genvectors > internal/obfuscatedid/testdata/vectors.json
cat backend/internal/obfuscatedid/testdata/vectors.json
```

Expected output: a JSON file with `master_key_hex` and a `vectors` array of 7 entries. Each `expected` will be a value in `[2^50, 2^51)`.

- [ ] **Step 3: Add a vectors-based regression test**

Append to `backend/internal/obfuscatedid/obfuscatedid_test.go`:
```go
import (
	"encoding/json"
	"os"
	"path/filepath"
)

type testVector struct {
	Seq      uint64 `json:"seq"`
	Expected uint64 `json:"expected"`
}

type testBundle struct {
	MasterKeyHex string       `json:"master_key_hex"`
	Vectors      []testVector `json:"vectors"`
}

func loadTestBundle(t *testing.T) testBundle {
	t.Helper()
	path := filepath.Join("testdata", "vectors.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read vectors: %v", err)
	}
	var b testBundle
	if err := json.Unmarshal(data, &b); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return b
}

func TestEncrypt_AgainstBlessedVectors(t *testing.T) {
	b := loadTestBundle(t)
	key := mustDecodeHex(t, b.MasterKeyHex)
	for _, v := range b.Vectors {
		got, err := Encrypt(key, v.Seq)
		if err != nil {
			t.Fatalf("Encrypt(%d): %v", v.Seq, err)
		}
		if got != v.Expected {
			t.Errorf("Encrypt(%d) = %d, want %d", v.Seq, got, v.Expected)
		}
	}
}
```

Consolidate imports at top of file (single `import` block with all stdlib paths).

- [ ] **Step 4: Run the full Go test suite**

```bash
cd backend && go test ./internal/obfuscatedid/...
```

Expected: all four tests pass, including `TestEncrypt_AgainstBlessedVectors`.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/obfuscatedid/cmd/ backend/internal/obfuscatedid/testdata/vectors.json backend/internal/obfuscatedid/obfuscatedid_test.go
git commit -m "$(cat <<'EOF'
test(obfuscatedid): blessed test vectors + regression test

Generated by go run ./internal/obfuscatedid/cmd/genvectors. Vectors
serve as the parity oracle for the PL/pgSQL implementation in 000002.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 3: Write migration `000001_extensions_and_schema.up.sql`

**Files:**
- Create: `backend/migrations/000001_extensions_and_schema.up.sql`

- [ ] **Step 1: Write the migration**

`backend/migrations/000001_extensions_and_schema.up.sql`:
```sql
-- TRA-720 — extensions + schema foundation for the clean migration stack.
--
-- pgcrypto is explicit (Cloud had it implicit via TimescaleDB Cloud defaults;
-- CNPG does not). Required by trakrf.generate_obfuscated_id() in 000002 for
-- pgcrypto.hmac().
--
-- ltree is intentionally NOT installed. It was used by the dropped
-- locations.path column (000018, dropped in 000042). Nothing else depends on
-- it; reinstall when a future feature requires it.

CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS trakrf;
```

- [ ] **Step 2: Apply to a fresh local DB and verify**

```bash
just database reset       # answer 'yes' when prompted
just database up
psql "$PG_URL_LOCAL" -f backend/migrations/000001_extensions_and_schema.up.sql
psql "$PG_URL_LOCAL" -c "SELECT extname FROM pg_extension WHERE extname IN ('timescaledb','pgcrypto','ltree') ORDER BY extname;"
psql "$PG_URL_LOCAL" -c "SELECT schema_name FROM information_schema.schemata WHERE schema_name = 'trakrf';"
```

Expected: timescaledb + pgcrypto present, ltree absent, trakrf schema exists.

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/000001_extensions_and_schema.up.sql
git commit -m "$(cat <<'EOF'
feat(migrations): 000001 extensions and schema foundation (TRA-720)

Explicit pgcrypto for the Feistel hmac() round function. ltree omitted
(dropped with locations.path in 000042 of the legacy stack; nothing else
depends on it).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 4: Write migration `000002_id_generator.up.sql` and validate against blessed vectors

**Files:**
- Create: `backend/migrations/000002_id_generator.up.sql`
- Create: `backend/database/test/feistel_parity_test.sql`

- [ ] **Step 1: Write the migration**

`backend/migrations/000002_id_generator.up.sql`:
```sql
-- TRA-720 — keyed Feistel ID generator + updated_at trigger function.
-- Defined before any table file so subsequent CREATE TRIGGER statements can
-- reference these functions.
--
-- Architecture:
--   * trakrf._feistel_encrypt(seq_value BIGINT) — internal pure function,
--     exposed for test parity against the Go reference at
--     backend/internal/obfuscatedid.
--   * trakrf.generate_obfuscated_id() — the TRIGGER function that wraps
--     _feistel_encrypt and pulls seq_value from nextval(TG_ARGV[0]).
--
-- Construction: 50-bit block (2 x 25-bit halves), 6 rounds, HMAC-SHA256
-- round function truncated to 25 bits, output OR'd with (1::bigint << 50)
-- so values land in [2^50, 2^51) — disjoint from migrated 31-bit IDs from
-- the legacy generate_hashed_id / generate_permuted_id stack.
--
-- Master key is set per database via:
--   ALTER DATABASE <db> SET app.obfuscation_key = '<64-hex-char-secret>';
--
-- Sequence overflow guard: a sequence reaching 2^50 means 1.1 quadrillion
-- inserts on a single table — unreachable in practice. The exception is
-- defensive.

SET search_path = trakrf, public;

CREATE OR REPLACE FUNCTION trakrf._feistel_encrypt(seq_value BIGINT) RETURNS BIGINT
LANGUAGE plpgsql STABLE AS $$
DECLARE
    master_key BYTEA;
    L BIGINT;
    R BIGINT;
    L_new BIGINT;
    round_idx INT;
    round_key BYTEA;
    f_out BIGINT;
    MASK25 CONSTANT BIGINT := (1::bigint << 25) - 1;
BEGIN
    IF seq_value >= (1::bigint << 50) THEN
        RAISE EXCEPTION 'Feistel input overflow: % >= 2^50', seq_value;
    END IF;

    master_key := decode(current_setting('app.obfuscation_key'), 'hex');

    L := (seq_value >> 25) & MASK25;
    R := seq_value & MASK25;

    FOR round_idx IN 1..6 LOOP
        round_key := hmac(('round-' || round_idx)::bytea, master_key, 'sha256');
        -- Take first 4 bytes of HMAC(int8send(R), round_key), interpret as
        -- big-endian uint32, mask to 25 bits.
        f_out := ('x' || encode(substring(
                    hmac(int8send(R), round_key, 'sha256')
                    FROM 1 FOR 4), 'hex'))::bit(32)::bigint & MASK25;
        L_new := R;
        R := L # f_out;
        L := L_new;
    END LOOP;

    RETURN ((L << 25) | R) | (1::bigint << 50);
END;
$$;

CREATE OR REPLACE FUNCTION trakrf.generate_obfuscated_id() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    seq_name TEXT := TG_ARGV[0];
    seq_value BIGINT;
BEGIN
    seq_value := nextval(seq_name);
    NEW.id := trakrf._feistel_encrypt(seq_value);
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION trakrf.update_updated_at_column() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;
```

- [ ] **Step 2: Write the SQL parity test**

`backend/database/test/feistel_parity_test.sql`:
```sql
-- TRA-720 — Validate trakrf._feistel_encrypt against the blessed Go test
-- vectors at backend/internal/obfuscatedid/testdata/vectors.json.
--
-- Usage (from repo root):
--   export PG_URL_LOCAL=...
--   ./backend/database/test/run_feistel_parity.sh
-- The script sets app.obfuscation_key from vectors.json, then loads this
-- file. This file does NOT set the GUC itself — it expects the runner to.

\set ON_ERROR_STOP on
SET search_path = trakrf, public;

DO $$
DECLARE
    expected BIGINT;
    got BIGINT;
    seq BIGINT;
    fail_count INT := 0;
BEGIN
    FOR seq, expected IN
        SELECT s::BIGINT, e::BIGINT FROM (VALUES
            -- These five rows are populated by the runner script from
            -- vectors.json. Placeholders here are overwritten before this
            -- block runs. (Runner uses sed to substitute the real values.)
            (1::BIGINT, 0::BIGINT),
            (2::BIGINT, 0::BIGINT),
            (100::BIGINT, 0::BIGINT),
            (12345::BIGINT, 0::BIGINT),
            (16777216::BIGINT, 0::BIGINT),
            (33554432::BIGINT, 0::BIGINT),
            (562949953421312::BIGINT, 0::BIGINT)
        ) AS v(s, e)
    LOOP
        got := trakrf._feistel_encrypt(seq);
        IF got <> expected THEN
            RAISE WARNING 'Mismatch at seq=%: got=%, expected=%', seq, got, expected;
            fail_count := fail_count + 1;
        END IF;
    END LOOP;
    IF fail_count > 0 THEN
        RAISE EXCEPTION 'Feistel parity test FAILED: % mismatch(es)', fail_count;
    END IF;
    RAISE NOTICE 'Feistel parity test PASSED for all vectors';
END $$;
```

Then create the runner: `backend/database/test/run_feistel_parity.sh`:
```bash
#!/usr/bin/env bash
# TRA-720 — Run feistel_parity_test.sql with values from vectors.json substituted in.
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
VECTORS="$REPO_ROOT/backend/internal/obfuscatedid/testdata/vectors.json"
TPL="$REPO_ROOT/backend/database/test/feistel_parity_test.sql"
TMP=$(mktemp --suffix=.sql)
trap "rm -f $TMP" EXIT

# Extract master_key_hex
KEY_HEX=$(jq -r '.master_key_hex' "$VECTORS")
# Extract VALUES rows
ROWS=$(jq -r '.vectors | map("    (\(.seq)::BIGINT, \(.expected)::BIGINT)") | join(",\n")' "$VECTORS")

# Build the SQL file: prepend SET app.obfuscation_key, substitute the VALUES block.
{
    echo "SET app.obfuscation_key = '$KEY_HEX';"
    # Replace the placeholder VALUES block with the real rows
    awk -v rows="$ROWS" '
        /-- These five rows/ { in_block = 1; print; next }
        in_block && /AS v\(s, e\)/ {
            print rows
            print "        ) AS v(s, e)"
            in_block = 0
            next
        }
        in_block { next }
        { print }
    ' "$TPL"
} > "$TMP"

psql "$PG_URL_LOCAL" -f "$TMP"
```

Make it executable: `chmod +x backend/database/test/run_feistel_parity.sh`

- [ ] **Step 3: Apply 000002 and run the parity test**

```bash
psql "$PG_URL_LOCAL" -f backend/migrations/000002_id_generator.up.sql
# Set the obfuscation key on the local DB:
psql "$PG_URL_LOCAL" -c "ALTER DATABASE $(psql $PG_URL_LOCAL -At -c 'SELECT current_database()') SET app.obfuscation_key = '6f626675736361746f72746573746b657920303132333435363738396162636465';"
# Reconnect so the DB-level GUC propagates:
./backend/database/test/run_feistel_parity.sh
```

Expected output: `NOTICE: Feistel parity test PASSED for all vectors`.

If parity fails: investigate the PL/pgSQL byte-order or truncation. The likely culprit is the `f_out := ('x' || encode(substring(...) FROM 1 FOR 4), 'hex'))::bit(32)::bigint & MASK25` expression — confirm it interprets HMAC output as big-endian.

- [ ] **Step 4: Commit**

```bash
git add backend/migrations/000002_id_generator.up.sql backend/database/test/
git commit -m "$(cat <<'EOF'
feat(migrations): 000002 keyed Feistel id generator (TRA-720)

trakrf._feistel_encrypt() implements a 50-bit, 6-round keyed Feistel
network with HMAC-SHA256 round function. trakrf.generate_obfuscated_id()
is the TRIGGER wrapper. trakrf.update_updated_at_column() is unchanged
from legacy 000001. SQL parity test validates against Go test vectors
at backend/internal/obfuscatedid/testdata/vectors.json.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 5: Write migration `000003_organizations_and_users.up.sql`

**Files:**
- Create: `backend/migrations/000003_organizations_and_users.up.sql`

- [ ] **Step 1: Write the migration**

`backend/migrations/000003_organizations_and_users.up.sql`:
```sql
-- TRA-720 — organizations, users, org_users, org_invitations, password_reset_tokens.
-- All surrogate PK/FK columns are BIGINT. RLS disabled on users + org_users
-- (auth needs unrestricted access before session GUCs are set; mirrors legacy 000020).
-- BIGSERIAL for org_invitations.id and password_reset_tokens.id (Tier 2 widening).

SET search_path = trakrf, public;

-- ============================================================================
-- org_role enum
-- ============================================================================
CREATE TYPE org_role AS ENUM ('viewer', 'operator', 'manager', 'admin');

-- ============================================================================
-- organizations
-- ============================================================================
CREATE SEQUENCE organization_seq AS BIGINT;

CREATE TABLE organizations (
    id          BIGINT PRIMARY KEY,
    name        VARCHAR(255) NOT NULL,
    identifier  VARCHAR(255) UNIQUE,
    metadata    JSONB DEFAULT '{}',
    valid_from  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to    TIMESTAMPTZ DEFAULT NULL,
    is_active   BOOLEAN NOT NULL DEFAULT true,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at  TIMESTAMPTZ
);

CREATE INDEX idx_organizations_identifier ON organizations(identifier);

CREATE TRIGGER generate_id_trigger
    BEFORE INSERT ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('organization_seq');

CREATE TRIGGER update_organizations_updated_at
    BEFORE UPDATE ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

COMMENT ON TABLE organizations IS 'Application customer identity and tenant root for multi-tenancy';
COMMENT ON COLUMN organizations.id IS 'Primary key - keyed Feistel ID';
COMMENT ON COLUMN organizations.identifier IS 'URL-safe identifier for MQTT topics and routing';

-- ============================================================================
-- users
-- ============================================================================
CREATE SEQUENCE user_seq AS BIGINT;

CREATE TABLE users (
    id              BIGINT PRIMARY KEY,
    email           VARCHAR(255) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    last_login_at   TIMESTAMPTZ,
    password_hash   VARCHAR(255),
    settings        JSONB DEFAULT '{}',
    metadata        JSONB DEFAULT '{}',
    is_superadmin   BOOLEAN NOT NULL DEFAULT FALSE,
    last_org_id     BIGINT REFERENCES organizations(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX idx_users_email ON users(email);

CREATE TRIGGER generate_user_id_trigger
    BEFORE INSERT ON users
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('user_seq');

CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

COMMENT ON TABLE users IS 'Stores users associated with orgs in the SaaS application';
COMMENT ON COLUMN users.id IS 'Primary key - keyed Feistel ID';
COMMENT ON COLUMN users.is_superadmin IS 'Cross-org superadmin flag (legacy 000022)';
COMMENT ON COLUMN users.last_org_id IS 'Last org context, for org-switch routing (legacy 000022)';

-- ============================================================================
-- org_users (composite PK)
-- ============================================================================
CREATE TABLE org_users (
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    user_id         BIGINT NOT NULL REFERENCES users(id),
    role            org_role NOT NULL DEFAULT 'viewer',
    status          VARCHAR(20) NOT NULL DEFAULT 'active',
    last_login_at   TIMESTAMPTZ,
    settings        JSONB DEFAULT '{}',
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMPTZ,

    CONSTRAINT valid_status CHECK (status IN ('active', 'inactive', 'suspended', 'invited')),
    PRIMARY KEY (org_id, user_id)
);

CREATE INDEX idx_org_users_org ON org_users(org_id);
CREATE INDEX idx_org_users_user ON org_users(user_id);
CREATE INDEX idx_org_users_role ON org_users(role);
CREATE INDEX idx_org_users_status ON org_users(status);

CREATE TRIGGER update_org_users_updated_at
    BEFORE UPDATE ON org_users
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

COMMENT ON TABLE org_users IS 'Junction table managing user membership and roles within organizations';

-- ============================================================================
-- org_invitations
-- ============================================================================
CREATE TABLE org_invitations (
    id          BIGSERIAL PRIMARY KEY,
    org_id      BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email       VARCHAR(255) NOT NULL,
    role        org_role NOT NULL DEFAULT 'viewer',
    token       VARCHAR(64) NOT NULL,
    invited_by  BIGINT REFERENCES users(id) ON DELETE SET NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    accepted_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT unique_org_email UNIQUE(org_id, email)
);

CREATE INDEX idx_org_invitations_token ON org_invitations(token);
CREATE INDEX idx_org_invitations_org_id ON org_invitations(org_id);
CREATE INDEX idx_org_invitations_email ON org_invitations(email);

-- ============================================================================
-- password_reset_tokens
-- ============================================================================
CREATE TABLE password_reset_tokens (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token       VARCHAR(64) NOT NULL,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_password_reset_tokens_token ON password_reset_tokens(token);
CREATE INDEX idx_password_reset_tokens_expires ON password_reset_tokens(expires_at);
```

- [ ] **Step 2: Apply and verify**

```bash
psql "$PG_URL_LOCAL" -f backend/migrations/000003_organizations_and_users.up.sql
psql "$PG_URL_LOCAL" -c "\d trakrf.organizations"
psql "$PG_URL_LOCAL" -c "\d trakrf.users"
psql "$PG_URL_LOCAL" -c "\d trakrf.org_users"
# Confirm BIGINT types on PKs and FKs:
psql "$PG_URL_LOCAL" -c "SELECT table_name, column_name, data_type FROM information_schema.columns WHERE table_schema='trakrf' AND data_type LIKE '%int%' ORDER BY table_name, ordinal_position;"
```

Expected: every `id`, `org_id`, `user_id`, `last_org_id`, `invited_by` shows as `bigint`. No `integer` rows.

Test an insert to verify the trigger fires correctly:
```bash
psql "$PG_URL_LOCAL" -c "INSERT INTO trakrf.organizations (name, identifier) VALUES ('Test Org', 'test-org') RETURNING id;"
```
Expected: a returned `id` value in the range `[1125899906842624, 2251799813685248)` (i.e., `[2^50, 2^51)`).

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/000003_organizations_and_users.up.sql
git commit -m "$(cat <<'EOF'
feat(migrations): 000003 organizations, users, org_users, invitations, password reset (TRA-720)

All surrogate PK/FK columns BIGINT. BIGSERIAL on org_invitations.id and
password_reset_tokens.id (Tier 2 widening for schema uniformity). org_role
enum from legacy 000022. RLS disabled on users + org_users to preserve
auth pre-session-GUC access pattern from legacy 000020.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 6: Write migration `000004_locations.up.sql`

**Files:**
- Create: `backend/migrations/000004_locations.up.sql`

- [ ] **Step 1: Write the migration**

`backend/migrations/000004_locations.up.sql`:
```sql
-- TRA-720 — locations table. Post-rename (external_key, not identifier).
-- No ltree path/depth (dropped in legacy 000042). BIGINT throughout.

SET search_path = trakrf, public;

CREATE SEQUENCE location_seq AS BIGINT;

CREATE TABLE locations (
    id                  BIGINT PRIMARY KEY,
    org_id              BIGINT NOT NULL REFERENCES organizations(id),
    external_key        VARCHAR(255) NOT NULL,
    name                VARCHAR(255) NOT NULL,
    description         TEXT,
    parent_location_id  BIGINT REFERENCES locations(id),
    valid_from          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to            TIMESTAMPTZ DEFAULT NULL,
    is_active           BOOLEAN NOT NULL DEFAULT true,
    metadata            JSONB DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          TIMESTAMPTZ,
    CONSTRAINT no_self_reference CHECK (id != parent_location_id)
);

CREATE INDEX idx_locations_org ON locations(org_id);
CREATE INDEX idx_locations_external_key ON locations(external_key);
CREATE INDEX idx_locations_parent ON locations(parent_location_id);
CREATE INDEX idx_locations_valid ON locations(valid_from, valid_to);
CREATE INDEX idx_locations_active ON locations(is_active) WHERE is_active = true;
CREATE UNIQUE INDEX locations_org_id_external_key_unique
    ON locations(org_id, external_key) WHERE deleted_at IS NULL;

CREATE TRIGGER generate_location_id_trigger
    BEFORE INSERT ON locations
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('location_seq');

CREATE TRIGGER update_locations_updated_at
    BEFORE UPDATE ON locations
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE locations ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_locations ON locations
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE locations IS 'Stores location information with temporal validity';
COMMENT ON COLUMN locations.id IS 'Primary key - keyed Feistel ID';
COMMENT ON COLUMN locations.external_key IS 'External natural key for the location (unique per org for live rows)';
```

- [ ] **Step 2: Apply and verify**

```bash
psql "$PG_URL_LOCAL" -f backend/migrations/000004_locations.up.sql
psql "$PG_URL_LOCAL" -c "\d trakrf.locations"
psql "$PG_URL_LOCAL" -c "SELECT policyname, qual FROM pg_policies WHERE tablename = 'locations';"
```
Expected: policy `org_isolation_locations` exists with `::bigint` cast on `current_setting`.

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/000004_locations.up.sql
git commit -m "$(cat <<'EOF'
feat(migrations): 000004 locations table (TRA-720)

BIGINT id/org_id/parent_location_id. external_key column (post-rename
000036). No ltree path/depth (dropped 000042). RLS with ::BIGINT cast.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 7: Write migration `000005_scan_devices_and_points.up.sql`

**Files:**
- Create: `backend/migrations/000005_scan_devices_and_points.up.sql`

- [ ] **Step 1: Write the migration**

`backend/migrations/000005_scan_devices_and_points.up.sql`:
```sql
-- TRA-720 — scan_devices + scan_points. Column name 'identifier' (never
-- renamed; only locations and assets got the external_key rename).

SET search_path = trakrf, public;

-- ============================================================================
-- scan_devices
-- ============================================================================
CREATE SEQUENCE scan_device_seq AS BIGINT;

CREATE TABLE scan_devices (
    id              BIGINT PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    identifier      VARCHAR(255) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    type            VARCHAR(50) NOT NULL,
    serial_number   VARCHAR(255),
    model           VARCHAR(100),
    description     TEXT,
    valid_from      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to        TIMESTAMPTZ DEFAULT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMPTZ,
    UNIQUE(org_id, identifier, valid_from)
);

CREATE INDEX idx_scan_devices_org ON scan_devices(org_id);
CREATE INDEX idx_scan_devices_identifier ON scan_devices(identifier);
CREATE INDEX idx_scan_devices_valid ON scan_devices(valid_from, valid_to);
CREATE INDEX idx_scan_devices_type ON scan_devices(type);
CREATE INDEX idx_scan_devices_active ON scan_devices(is_active) WHERE is_active = true;

CREATE TRIGGER generate_scan_device_id_trigger
    BEFORE INSERT ON scan_devices
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('scan_device_seq');

CREATE TRIGGER update_scan_devices_updated_at
    BEFORE UPDATE ON scan_devices
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE scan_devices ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_scan_devices ON scan_devices
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE scan_devices IS 'Stores scan device information with temporal validity';
COMMENT ON COLUMN scan_devices.id IS 'Primary key - keyed Feistel ID';
COMMENT ON COLUMN scan_devices.identifier IS 'Natural key/business identifier (e.g., cs463-214). Used in MQTT topics';

-- ============================================================================
-- scan_points
-- ============================================================================
CREATE SEQUENCE scan_point_seq AS BIGINT;

CREATE TABLE scan_points (
    id              BIGINT PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    scan_device_id  BIGINT NOT NULL REFERENCES scan_devices(id),
    location_id     BIGINT REFERENCES locations(id),
    identifier      VARCHAR(255) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    antenna_port    INT,
    description     TEXT,
    valid_from      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to        TIMESTAMPTZ DEFAULT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMPTZ,
    UNIQUE(org_id, identifier, valid_from)
);

CREATE INDEX idx_scan_points_org ON scan_points(org_id);
CREATE INDEX idx_scan_points_device ON scan_points(scan_device_id);
CREATE INDEX idx_scan_points_location ON scan_points(location_id);
CREATE INDEX idx_scan_points_identifier ON scan_points(identifier);
CREATE INDEX idx_scan_points_valid ON scan_points(valid_from, valid_to);
CREATE INDEX idx_scan_points_active ON scan_points(is_active) WHERE is_active = true;

CREATE TRIGGER generate_scan_point_id_trigger
    BEFORE INSERT ON scan_points
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('scan_point_seq');

CREATE TRIGGER update_scan_points_updated_at
    BEFORE UPDATE ON scan_points
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE scan_points ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_scan_points ON scan_points
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE scan_points IS 'Stores scan point (sensor/antenna) information with temporal validity';
COMMENT ON COLUMN scan_points.id IS 'Primary key - keyed Feistel ID';
COMMENT ON COLUMN scan_points.antenna_port IS 'Antenna port number for RFID readers with multiple antennas (NOT widened to bigint; small range)';
```

- [ ] **Step 2: Apply and verify**

```bash
psql "$PG_URL_LOCAL" -f backend/migrations/000005_scan_devices_and_points.up.sql
psql "$PG_URL_LOCAL" -c "\d trakrf.scan_devices"
psql "$PG_URL_LOCAL" -c "\d trakrf.scan_points"
```

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/000005_scan_devices_and_points.up.sql
git commit -m "$(cat <<'EOF'
feat(migrations): 000005 scan_devices and scan_points (TRA-720)

All surrogate PK/FK BIGINT. antenna_port stays INT (port number 1-4,
not an ID). RLS on both with ::BIGINT cast.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 8: Write migration `000006_assets.up.sql`

**Files:**
- Create: `backend/migrations/000006_assets.up.sql`

- [ ] **Step 1: Write the migration**

`backend/migrations/000006_assets.up.sql`:
```sql
-- TRA-720 — assets table. Post-rename (external_key, not identifier).
-- No current_location_id (dropped 000043). No type column (dropped 000035).

SET search_path = trakrf, public;

CREATE SEQUENCE asset_seq AS BIGINT;

CREATE TABLE assets (
    id              BIGINT PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    external_key    VARCHAR(255) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    valid_from      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to        TIMESTAMPTZ DEFAULT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMPTZ,
    UNIQUE(org_id, external_key, valid_from)
);

CREATE INDEX idx_assets_org ON assets(org_id);
CREATE INDEX idx_assets_external_key ON assets(external_key);
CREATE INDEX idx_assets_valid ON assets(valid_from, valid_to);
CREATE INDEX idx_assets_active ON assets(is_active) WHERE is_active = true;
CREATE UNIQUE INDEX assets_org_id_external_key_unique
    ON assets(org_id, external_key) WHERE deleted_at IS NULL;

CREATE TRIGGER generate_asset_id_trigger
    BEFORE INSERT ON assets
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('asset_seq');

CREATE TRIGGER update_assets_updated_at
    BEFORE UPDATE ON assets
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE assets ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_assets ON assets
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE assets IS 'Stores tracked assets with temporal validity';
COMMENT ON COLUMN assets.id IS 'Primary key - keyed Feistel ID';
COMMENT ON COLUMN assets.external_key IS 'External natural key for the asset (caller-supplied or auto-generated ASSET-NNNN, unique per org for live rows)';
```

- [ ] **Step 2: Apply and verify**

```bash
psql "$PG_URL_LOCAL" -f backend/migrations/000006_assets.up.sql
psql "$PG_URL_LOCAL" -c "\d trakrf.assets"
```

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/000006_assets.up.sql
git commit -m "$(cat <<'EOF'
feat(migrations): 000006 assets table (TRA-720)

external_key column (post-000037 rename). No current_location_id
(dropped 000043). No type column (dropped 000035). BIGINT throughout.
Partial unique index (org_id, external_key) WHERE deleted_at IS NULL.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 9: Write migration `000007_tags.up.sql`

**Files:**
- Create: `backend/migrations/000007_tags.up.sql`

- [ ] **Step 1: Write the migration**

`backend/migrations/000007_tags.up.sql`:
```sql
-- TRA-720 — tags table (was 'identifiers' in legacy 000009, renamed 000033).
-- Mutually exclusive asset_id or location_id (tag_target check constraint).
-- Partial unique on (org_id, type, value) WHERE deleted_at IS NULL.

SET search_path = trakrf, public;

CREATE SEQUENCE tag_seq AS BIGINT;

CREATE TABLE tags (
    id              BIGINT PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    type            VARCHAR(50) NOT NULL,
    value           VARCHAR(255) NOT NULL,
    asset_id        BIGINT REFERENCES assets(id),
    location_id     BIGINT REFERENCES locations(id),
    valid_from      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to        TIMESTAMPTZ DEFAULT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMPTZ,

    CONSTRAINT tag_target CHECK (
        (asset_id IS NOT NULL AND location_id IS NULL) OR
        (asset_id IS NULL AND location_id IS NOT NULL)
    )
);

CREATE INDEX idx_tags_org ON tags(org_id);
CREATE INDEX idx_tags_asset ON tags(asset_id);
CREATE INDEX idx_tags_location ON tags(location_id);
CREATE INDEX idx_tags_value ON tags(value);
CREATE INDEX idx_tags_valid ON tags(valid_from, valid_to);
CREATE INDEX idx_tags_type ON tags(type);
CREATE INDEX idx_tags_active ON tags(is_active) WHERE is_active = true;
CREATE UNIQUE INDEX tags_org_id_type_value_unique
    ON tags(org_id, type, value) WHERE deleted_at IS NULL;

CREATE TRIGGER generate_tag_id_trigger
    BEFORE INSERT ON tags
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('tag_seq');

CREATE TRIGGER update_tags_updated_at
    BEFORE UPDATE ON tags
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE tags ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_tags ON tags
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE tags IS 'Stores physical/logical tags (RFID, BLE, NFC, barcode, serial, etc.) with temporal validity';
COMMENT ON COLUMN tags.id IS 'Primary key - keyed Feistel ID';
COMMENT ON COLUMN tags.type IS 'Tag type: rfid, ble, nfc, barcode, serial, mac, qr, etc.';
COMMENT ON COLUMN tags.value IS 'The actual tag value (EPC, MAC address, NFC UID, barcode digits, etc.)';
```

- [ ] **Step 2: Apply and verify**

```bash
psql "$PG_URL_LOCAL" -f backend/migrations/000007_tags.up.sql
psql "$PG_URL_LOCAL" -c "\d trakrf.tags"
```

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/000007_tags.up.sql
git commit -m "$(cat <<'EOF'
feat(migrations): 000007 tags table (TRA-720)

Renamed from identifiers (legacy 000033). BIGINT throughout. Mutually
exclusive asset_id/location_id via tag_target CHECK constraint. Partial
unique (org_id, type, value) WHERE deleted_at IS NULL.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 10: Write migration `000008_scan_hypertables.up.sql` (folds TRA-836)

**Files:**
- Create: `backend/migrations/000008_scan_hypertables.up.sql`

- [ ] **Step 1: Write the migration**

`backend/migrations/000008_scan_hypertables.up.sql`:
```sql
-- TRA-720 — TimescaleDB hypertables for raw scan ingestion and derived events.
-- tag_scans: surrogate id BIGINT IDENTITY (TRA-836 fold-in) eliminates burst-rate
-- PK collisions. asset_scans: composite content PK (timestamp, org_id, asset_id)
-- preserved — dedup-by-content is intentional.

SET search_path = trakrf, public;

-- ============================================================================
-- tag_scans (was identifier_scans in legacy 000010, renamed 000033)
-- TRA-836: new surrogate id BIGINT IDENTITY, PK is (created_at, id) so multiple
-- same-topic messages in the same microsecond no longer collide on PK.
-- ============================================================================
CREATE TABLE tag_scans (
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    id              BIGINT GENERATED ALWAYS AS IDENTITY,
    message_topic   TEXT NOT NULL,
    message_data    JSONB NOT NULL,
    PRIMARY KEY (created_at, id)
);

SELECT create_hypertable('tag_scans', 'created_at');
SELECT set_chunk_time_interval('tag_scans', INTERVAL '1 day');
SELECT add_retention_policy('tag_scans', INTERVAL '30 days');

CREATE INDEX idx_tag_scans_topic ON tag_scans(message_topic, created_at DESC);

COMMENT ON TABLE tag_scans IS 'Raw MQTT message capture from RFID readers - pure data lake for tag scans';
COMMENT ON COLUMN tag_scans.id IS 'Internal monotonic surrogate (TRA-836). Not Feistel-obfuscated: never wire-exposed, high insert rate.';
COMMENT ON COLUMN tag_scans.created_at IS 'Timestamp when message was received';
COMMENT ON COLUMN tag_scans.message_topic IS 'MQTT topic (e.g., trakrf.id/cs463-214/scan)';
COMMENT ON COLUMN tag_scans.message_data IS 'Raw MQTT message payload as JSON';

-- ============================================================================
-- asset_scans (derived hypertable, composite content PK)
-- ============================================================================
CREATE TABLE asset_scans (
    timestamp       TIMESTAMPTZ NOT NULL,
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    asset_id        BIGINT NOT NULL REFERENCES assets(id),
    location_id     BIGINT REFERENCES locations(id),
    scan_point_id   BIGINT REFERENCES scan_points(id),
    tag_scan_id     BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (timestamp, org_id, asset_id)
);

CREATE INDEX idx_asset_scans_org_time ON asset_scans(org_id, timestamp DESC);
CREATE INDEX idx_asset_scans_asset_time ON asset_scans(asset_id, timestamp DESC);
CREATE INDEX idx_asset_scans_location_time ON asset_scans(location_id, timestamp DESC);
CREATE INDEX idx_asset_scans_scan_point_time ON asset_scans(scan_point_id, timestamp DESC);

SELECT create_hypertable('asset_scans', 'timestamp');
SELECT set_chunk_time_interval('asset_scans', INTERVAL '1 day');
SELECT add_retention_policy('asset_scans', INTERVAL '365 days');

COMMENT ON TABLE asset_scans IS 'TimescaleDB hypertable for derived asset scan events (business-level data)';
COMMENT ON COLUMN asset_scans.tag_scan_id IS 'Link to source raw tag_scans.id (no FK; cannot reference hypertable)';
```

- [ ] **Step 2: Apply and verify**

```bash
psql "$PG_URL_LOCAL" -f backend/migrations/000008_scan_hypertables.up.sql
psql "$PG_URL_LOCAL" -c "\d trakrf.tag_scans"
psql "$PG_URL_LOCAL" -c "\d trakrf.asset_scans"
psql "$PG_URL_LOCAL" -c "SELECT hypertable_name FROM timescaledb_information.hypertables WHERE hypertable_schema = 'trakrf';"
```
Expected: tag_scans + asset_scans are listed as hypertables, tag_scans PK is `(created_at, id)`.

Test burst-insert (verify TRA-836 fix):
```bash
psql "$PG_URL_LOCAL" <<'EOF'
INSERT INTO trakrf.tag_scans (message_topic, message_data)
SELECT 'test-topic', '{"x":1}'::jsonb FROM generate_series(1, 100);
SELECT COUNT(*) FROM trakrf.tag_scans WHERE message_topic = 'test-topic';
EOF
```
Expected: 100 rows. (Under old PK shape, many would collide on `(now(), 'test-topic')` and rollback.)

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/000008_scan_hypertables.up.sql
git commit -m "$(cat <<'EOF'
feat(migrations): 000008 scan hypertables, folds TRA-836 (TRA-720)

tag_scans: new surrogate id BIGINT IDENTITY, PK is (created_at, id).
Closes TRA-836 (burst-insert PK collision on same-microsecond messages).
asset_scans: composite (timestamp, org_id, asset_id) PK preserved —
content-based dedup is intentional. All FKs BIGINT.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 11: Write migration `000009_bulk_import_and_api_keys.up.sql`

**Files:**
- Create: `backend/migrations/000009_bulk_import_and_api_keys.up.sql`

- [ ] **Step 1: Write the migration**

`backend/migrations/000009_bulk_import_and_api_keys.up.sql`:
```sql
-- TRA-720 — bulk_import_jobs + api_keys.
-- bulk_import_jobs: tags_created counter (post-000025), corrected valid_row_counts
-- (post-000026). api_keys: self-FK created_by_key_id (post-000029). RLS on
-- bulk_import_jobs only — api_keys is app-layer enforced (auth reads before
-- session GUC is set; per legacy 000027 comment).

SET search_path = trakrf, public;

-- ============================================================================
-- bulk_import_jobs
-- ============================================================================
CREATE SEQUENCE bulk_import_job_seq AS BIGINT;

CREATE TABLE bulk_import_jobs (
    id              BIGINT PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    status          TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    total_rows      INT NOT NULL DEFAULT 0,
    processed_rows  INT NOT NULL DEFAULT 0,
    failed_rows     INT NOT NULL DEFAULT 0,
    tags_created    INT NOT NULL DEFAULT 0,
    errors          JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at    TIMESTAMPTZ,
    CONSTRAINT valid_row_counts CHECK (
        processed_rows >= 0 AND failed_rows >= 0
        AND processed_rows + failed_rows <= total_rows
    )
);

CREATE INDEX idx_bulk_import_jobs_org_id ON bulk_import_jobs(org_id);
CREATE INDEX idx_bulk_import_jobs_status ON bulk_import_jobs(status);
CREATE INDEX idx_bulk_import_jobs_created_at ON bulk_import_jobs(created_at DESC);

CREATE TRIGGER generate_bulk_import_job_id_trigger
    BEFORE INSERT ON bulk_import_jobs
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('bulk_import_job_seq');

ALTER TABLE bulk_import_jobs ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_bulk_import_jobs ON bulk_import_jobs
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE bulk_import_jobs IS 'Tracks async bulk import operations for assets';
COMMENT ON COLUMN bulk_import_jobs.tags_created IS 'Number of tag rows created by this job (post-000025)';

-- ============================================================================
-- api_keys
-- ============================================================================
CREATE SEQUENCE api_key_seq AS BIGINT;

CREATE TABLE api_keys (
    id                  BIGINT PRIMARY KEY,
    jti                 UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    org_id              BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                VARCHAR(255) NOT NULL,
    scopes              TEXT[] NOT NULL,
    created_by          BIGINT REFERENCES users(id),
    created_by_key_id   BIGINT REFERENCES api_keys(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at          TIMESTAMPTZ,
    last_used_at        TIMESTAMPTZ,
    revoked_at          TIMESTAMPTZ,
    CONSTRAINT api_keys_creator_exactly_one
        CHECK ((created_by IS NOT NULL) <> (created_by_key_id IS NOT NULL))
);

CREATE TRIGGER generate_api_key_id_trigger
    BEFORE INSERT ON api_keys
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('api_key_seq');

CREATE INDEX idx_api_keys_active_by_org
    ON api_keys(org_id) WHERE revoked_at IS NULL;
CREATE INDEX idx_api_keys_jti ON api_keys(jti);

COMMENT ON TABLE api_keys IS 'API keys for public API authentication';
COMMENT ON COLUMN api_keys.jti IS 'JWT ID — revocation handle referenced by api_key JWTs';
COMMENT ON COLUMN api_keys.created_by IS 'User who minted this key via session auth. Mutually exclusive with created_by_key_id.';
COMMENT ON COLUMN api_keys.created_by_key_id IS 'Parent API key that minted this key via keys:admin scope. Mutually exclusive with created_by.';
COMMENT ON COLUMN api_keys.scopes IS 'Subset of: assets:read, assets:write, locations:read, locations:write, scans:read, scans:write, keys:admin';
```

- [ ] **Step 2: Apply and verify**

```bash
psql "$PG_URL_LOCAL" -f backend/migrations/000009_bulk_import_and_api_keys.up.sql
psql "$PG_URL_LOCAL" -c "\d trakrf.bulk_import_jobs"
psql "$PG_URL_LOCAL" -c "\d trakrf.api_keys"
```

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/000009_bulk_import_and_api_keys.up.sql
git commit -m "$(cat <<'EOF'
feat(migrations): 000009 bulk_import_jobs + api_keys (TRA-720)

bulk_import_jobs: tags_created counter (000025), corrected valid_row_counts
(000026), RLS. api_keys: self-FK created_by_key_id with mutual-exclusion
CHECK (000029). No RLS on api_keys — auth reads before session GUC set
(legacy 000027 rationale).

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 12: Write migration `000010_stored_procedures.up.sql`

**Files:**
- Create: `backend/migrations/000010_stored_procedures.up.sql`

- [ ] **Step 1: Write the migration**

`backend/migrations/000010_stored_procedures.up.sql`:
```sql
-- TRA-720 — Higher-order stored procedures. Defined last because they reference
-- tables created in 000003-000009.
--
-- process_tag_scans():  AFTER-INSERT trigger on tag_scans. Auto-creates
--                       locations/scan_devices/scan_points/assets/tags from
--                       MQTT message contents, then writes derived asset_scans.
--                       Body from legacy 000037 with all INT widened to BIGINT.
-- create_asset_with_tags():    transactional asset+tags insert, from legacy 000043.
-- create_location_with_tags(): transactional location+tags insert, from legacy 000036.
--
-- ----------------------------------------------------------------------------
-- NOTE ON process_tag_scans ARCHITECTURE
-- ----------------------------------------------------------------------------
-- Trigger-driven ingestion is a known interim. Each incoming MQTT message
-- fires N INSERTs into adjacent tables; this scales adequately for moderate
-- traffic but is not the long-term shape. Deferred until customer traffic
-- justifies a redesign (likely a dedicated ingestion service, not PG triggers).

SET search_path = trakrf, public;

-- ============================================================================
-- process_tag_scans
-- ============================================================================
CREATE OR REPLACE FUNCTION trakrf.process_tag_scans() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    topic_org_id BIGINT;
BEGIN
    SELECT o.id INTO topic_org_id
    FROM organizations o
    WHERE o.identifier = split_part(NEW.message_topic, '/', 1);

    IF topic_org_id IS NULL THEN
        RAISE NOTICE 'Could not find organization for topic: %', NEW.message_topic;
        RETURN NEW;
    END IF;

    INSERT INTO locations (org_id, external_key, name)
    SELECT DISTINCT
        topic_org_id,
        t.tag ->> 'capturePointName',
        t.tag ->> 'capturePointName' || ' (auto-created from scan)'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (
        SELECT 1 FROM locations l
        WHERE l.org_id = topic_org_id AND l.external_key = t.tag ->> 'capturePointName'
    );

    INSERT INTO scan_devices (org_id, identifier, name, type)
    SELECT DISTINCT
        topic_org_id,
        NEW.message_data ->> 'rfidReaderName',
        NEW.message_data ->> 'rfidReaderName' || ' (auto-created from scan)',
        'rfid_reader'
    WHERE NOT EXISTS (
        SELECT 1 FROM scan_devices d
        WHERE d.org_id = topic_org_id AND d.identifier = NEW.message_data ->> 'rfidReaderName'
    );

    INSERT INTO scan_points (org_id, scan_device_id, location_id, identifier, name, antenna_port)
    SELECT DISTINCT
        topic_org_id,
        (SELECT id FROM scan_devices WHERE org_id = topic_org_id AND identifier = NEW.message_data ->> 'rfidReaderName'),
        l.id,
        t.tag ->> 'capturePointName',
        t.tag ->> 'capturePointName' || ' (auto-created from scan)',
        (t.tag ->> 'antennaPort')::INT
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN locations l ON l.org_id = topic_org_id AND l.external_key = t.tag ->> 'capturePointName'
    WHERE NOT EXISTS (
        SELECT 1 FROM scan_points sp
        WHERE sp.org_id = topic_org_id AND sp.identifier = t.tag ->> 'capturePointName'
    );

    INSERT INTO assets (org_id, external_key, name)
    SELECT DISTINCT
        topic_org_id,
        t.tag ->> 'epc',
        t.tag ->> 'epc' || ' (auto-created from scan)'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    WHERE NOT EXISTS (
        SELECT 1 FROM assets a
        WHERE a.org_id = topic_org_id AND a.external_key = t.tag ->> 'epc'
    )
    AND NOT EXISTS (
        SELECT 1 FROM tags i
        WHERE i.org_id = topic_org_id AND i.value = t.tag ->> 'epc'
    );

    INSERT INTO tags (org_id, asset_id, type, value)
    SELECT DISTINCT
        topic_org_id, a.id, 'rfid', t.tag ->> 'epc'
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN assets a ON a.org_id = topic_org_id AND a.external_key = t.tag ->> 'epc'
    WHERE NOT EXISTS (
        SELECT 1 FROM tags i
        WHERE i.org_id = topic_org_id AND i.value = t.tag ->> 'epc'
    );

    INSERT INTO asset_scans (timestamp, org_id, asset_id, location_id, scan_point_id)
    SELECT
        to_timestamp((t.tag ->> 'timeStampOfRead')::BIGINT / 1000000.0),
        topic_org_id,
        a.id,
        sp.location_id,
        sp.id
    FROM jsonb_array_elements(NEW.message_data -> 'tags') AS t(tag)
    JOIN scan_points sp ON sp.org_id = topic_org_id AND sp.identifier = t.tag ->> 'capturePointName'
    JOIN assets a ON a.org_id = topic_org_id AND a.external_key = t.tag ->> 'epc'
    ON CONFLICT (timestamp, org_id, asset_id) DO NOTHING;

    RETURN NEW;
EXCEPTION
    WHEN OTHERS THEN
        RAISE WARNING 'Error processing tag_scan: %', SQLERRM;
        RETURN NEW;
END;
$$;

CREATE TRIGGER trigger_process_tag_scans
    AFTER INSERT ON tag_scans
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.process_tag_scans();

COMMENT ON FUNCTION trakrf.process_tag_scans() IS
    'Auto-create entities from MQTT messages and populate asset_scans. TRA-720 interim: trigger-driven ingestion is due for redesign at scale.';

-- ============================================================================
-- create_asset_with_tags (post-000043: no current_location_id)
-- ============================================================================
CREATE OR REPLACE FUNCTION trakrf.create_asset_with_tags(
    p_org_id BIGINT,
    p_external_key VARCHAR(255),
    p_name VARCHAR(255),
    p_description TEXT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_tags JSONB
) RETURNS TABLE (asset_id BIGINT, tag_ids BIGINT[]) AS $$
DECLARE
    v_asset_id BIGINT;
    v_tag_ids BIGINT[] := '{}';
    v_tag JSONB;
    v_new_id BIGINT;
BEGIN
    INSERT INTO trakrf.assets (
        org_id, external_key, name, description,
        valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_external_key, p_name, p_description,
        p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_asset_id;

    IF p_tags IS NOT NULL AND jsonb_array_length(p_tags) > 0 THEN
        FOR v_tag IN SELECT * FROM jsonb_array_elements(p_tags) LOOP
            INSERT INTO trakrf.tags (org_id, type, value, asset_id, is_active)
            VALUES (
                p_org_id,
                COALESCE(v_tag->>'type', 'rfid'),
                v_tag->>'value',
                v_asset_id,
                TRUE
            )
            RETURNING id INTO v_new_id;
            v_tag_ids := array_append(v_tag_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_asset_id, v_tag_ids;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- create_location_with_tags (post-000036)
-- ============================================================================
CREATE OR REPLACE FUNCTION trakrf.create_location_with_tags(
    p_org_id BIGINT,
    p_external_key VARCHAR(255),
    p_name VARCHAR(255),
    p_description TEXT,
    p_parent_location_id BIGINT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_tags JSONB
) RETURNS TABLE (location_id BIGINT, tag_ids BIGINT[]) AS $$
DECLARE
    v_location_id BIGINT;
    v_tag_ids BIGINT[] := '{}';
    v_tag JSONB;
    v_new_id BIGINT;
BEGIN
    INSERT INTO trakrf.locations (
        org_id, external_key, name, description,
        parent_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_external_key, p_name, p_description,
        p_parent_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_location_id;

    IF p_tags IS NOT NULL AND jsonb_array_length(p_tags) > 0 THEN
        FOR v_tag IN SELECT * FROM jsonb_array_elements(p_tags) LOOP
            INSERT INTO trakrf.tags (org_id, type, value, location_id, is_active)
            VALUES (
                p_org_id,
                COALESCE(v_tag->>'type', 'rfid'),
                v_tag->>'value',
                v_location_id,
                TRUE
            )
            RETURNING id INTO v_new_id;
            v_tag_ids := array_append(v_tag_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_location_id, v_tag_ids;
END;
$$ LANGUAGE plpgsql;
```

- [ ] **Step 2: Apply and verify**

```bash
psql "$PG_URL_LOCAL" -f backend/migrations/000010_stored_procedures.up.sql
psql "$PG_URL_LOCAL" -c "\df trakrf.process_tag_scans trakrf.create_asset_with_tags trakrf.create_location_with_tags"
```

Smoke-test end-to-end: ingest a sample MQTT message and confirm cascade creates rows.
```bash
psql "$PG_URL_LOCAL" <<'EOF'
SET search_path = trakrf, public;
INSERT INTO organizations (name, identifier) VALUES ('Sample Org', 'sample-org');
INSERT INTO tag_scans (created_at, message_topic, message_data) VALUES (
    NOW(),
    'sample-org/cs463-001/scan',
    '{
        "rfidReaderName": "cs463-001",
        "tags": [
            {"epc": "E2000017240000000000001", "capturePointName": "dock-1", "antennaPort": "1", "timeStampOfRead": "1716729600000000"}
        ]
    }'::jsonb
);
SELECT external_key FROM locations WHERE org_id = (SELECT id FROM organizations WHERE identifier = 'sample-org');
SELECT external_key FROM assets WHERE org_id = (SELECT id FROM organizations WHERE identifier = 'sample-org');
SELECT COUNT(*) FROM asset_scans;
EOF
```

Expected: 1 location (`dock-1`), 1 asset (the EPC), 1 asset_scan row.

- [ ] **Step 3: Commit**

```bash
git add backend/migrations/000010_stored_procedures.up.sql
git commit -m "$(cat <<'EOF'
feat(migrations): 000010 higher-order stored procedures (TRA-720)

process_tag_scans (legacy 000037, BIGINT widened), create_asset_with_tags
(legacy 000043, BIGINT widened), create_location_with_tags (legacy 000036,
BIGINT widened). Includes leading comment flagging trigger-driven
ingestion as known interim.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 13: Create `pre-tra-720` tag, delete legacy migration files, write schema-diff recipe, validate

**Why this sequencing:** The schema-diff recipe applies the *new* stack via golang-migrate against `backend/migrations/`. If legacy files are still present in that directory, the duplicate version numbers (legacy 000001 vs new 000001) cause migrate to fail. So legacy files must leave the working tree before the diff runs. The `pre-tra-720` tag preserves them for the diff and for git-history reference.

**Files:**
- Delete: `backend/migrations/0000{1..4}*_*.{up,down}.sql` (all 88 legacy files)
- Modify: `backend/justfile` (add db-diff-old-vs-new recipe)
- Create: `backend/database/test/expected_diff_allowlist.txt`

- [ ] **Step 1: Create the `pre-tra-720` git tag**

The tag should point at the last commit before TRA-720 work began. The worktree was created off `main`, so the merge-base is the right anchor:

```bash
PRE_SHA=$(git merge-base HEAD origin/main)
echo "Tagging $PRE_SHA as pre-tra-720"
# Sanity check — should show legacy migration files at this SHA:
git show --stat "$PRE_SHA" -- backend/migrations/ | head -3
git tag -a pre-tra-720 "$PRE_SHA" -m "Final state of pre-TRA-720 (44-migration) stack — see TRA-720 PR for the clean-stack rewrite."
```

If `origin/main` has moved since the worktree was created, use `git log --format='%H %s' --first-parent | grep -v 'TRA-720' | head -1` to find the most recent non-TRA-720 commit, or pass the SHA manually if you know it.

- [ ] **Step 2: Delete legacy migration files**

```bash
cd backend/migrations
ls 000{01..44}_*.{up,down}.sql 2>/dev/null   # list to be deleted (88 files)
git rm 000{01..44}_*.{up,down}.sql
ls    # verify only 000001-000010_*.up.sql remain (no .down.sql in the new foundation)
```

Commit the deletion:
```bash
git commit -m "$(cat <<'EOF'
chore(migrations): remove 44 legacy migration files (TRA-720)

The new 10-file stack (000001-000010) is the canonical schema.
Legacy files preserved via the pre-tra-720 tag.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

- [ ] **Step 3: Add the recipe to backend/justfile**

Append to `backend/justfile`:
```make
# TRA-720: schema-diff between the legacy 44-migration stack (pre-tra-720 tag)
# and the new 10-file stack. Both are applied to ephemeral databases;
# pg_dump --schema-only outputs are diffed.
#
# Requires the pre-tra-720 git tag to exist (Task 13 Step 3 creates it).
# Requires PG_URL_LOCAL to be a Postgres URL pointing at the local docker DB,
# with sufficient privileges to CREATE/DROP databases.
#
# Usage: just backend db-diff-old-vs-new
db-diff-old-vs-new:
    #!/usr/bin/env bash
    set -euo pipefail
    REPO_ROOT="$(git rev-parse --show-toplevel)"
    OLD_DB="trakrf_diff_old"
    NEW_DB="trakrf_diff_new"
    LEGACY_WORKTREE="/tmp/tra-720-legacy-migrations"

    if ! git -C "$REPO_ROOT" rev-parse pre-tra-720 >/dev/null 2>&1; then
        echo "ERROR: pre-tra-720 tag not found. Create it: git tag pre-tra-720 <sha>" >&2
        exit 1
    fi

    # Derive per-DB URLs by string-replacing the dbname in PG_URL_LOCAL.
    # PG_URL_LOCAL format: postgres://user:pwd@host:port/<dbname>?<query>
    SOURCE_DB=$(echo "$PG_URL_LOCAL" | sed -E 's|^postgres://[^/]+/([^?]+).*|\1|')
    OLD_URL="${PG_URL_LOCAL/\/${SOURCE_DB}/\/${OLD_DB}}"
    NEW_URL="${PG_URL_LOCAL/\/${SOURCE_DB}/\/${NEW_DB}}"

    psql "$PG_URL_LOCAL" -c "DROP DATABASE IF EXISTS $OLD_DB;"
    psql "$PG_URL_LOCAL" -c "DROP DATABASE IF EXISTS $NEW_DB;"
    psql "$PG_URL_LOCAL" -c "CREATE DATABASE $OLD_DB;"
    psql "$PG_URL_LOCAL" -c "CREATE DATABASE $NEW_DB;"
    psql "$PG_URL_LOCAL" -c "ALTER DATABASE $NEW_DB SET app.obfuscation_key = '6f626675736361746f72746573746b657920303132333435363738396162636465';"

    # Extract legacy migrations to a sibling worktree.
    rm -rf "$LEGACY_WORKTREE"
    git -C "$REPO_ROOT" worktree add --detach "$LEGACY_WORKTREE" pre-tra-720

    migrate -path "$LEGACY_WORKTREE/backend/migrations" -database "$OLD_URL" up
    migrate -path "$REPO_ROOT/backend/migrations" -database "$NEW_URL" up

    pg_dump --schema-only --no-owner --no-privileges -d "$OLD_URL" > /tmp/old_schema.sql
    pg_dump --schema-only --no-owner --no-privileges -d "$NEW_URL" > /tmp/new_schema.sql
    diff -u /tmp/old_schema.sql /tmp/new_schema.sql > /tmp/schema_diff.txt || true

    git -C "$REPO_ROOT" worktree remove --force "$LEGACY_WORKTREE"

    echo
    echo "Diff saved to /tmp/schema_diff.txt ($(wc -l < /tmp/schema_diff.txt) lines)"
    echo "Allowlist at $REPO_ROOT/backend/database/test/expected_diff_allowlist.txt"
    echo
    echo "Review the diff manually. Any line outside the allowlist categories is a regression."
```

- [ ] **Step 4: Write the allowlist documentation**

`backend/database/test/expected_diff_allowlist.txt`:
```
TRA-720 SCHEMA DIFF — EXPECTED CATEGORIES OF CHANGE
====================================================

The following categories are intentional and pass review. Anything OUTSIDE
these categories represents a regression that requires investigation.

1. integer → bigint
   Every surrogate PK / FK column on entity tables widens from int4 to int8.
   Affected: organizations.id, users.id+last_org_id, org_users.org_id+user_id,
   org_invitations.id+org_id+invited_by, password_reset_tokens.id+user_id,
   locations.id+org_id+parent_location_id, scan_devices.id+org_id,
   scan_points.id+org_id+scan_device_id+location_id,
   assets.id+org_id, tags.id+org_id+asset_id+location_id,
   bulk_import_jobs.id+org_id, api_keys.id+org_id+created_by+created_by_key_id,
   asset_scans.org_id+asset_id+location_id+scan_point_id.

2. Removed: public.generate_hashed_id
3. Removed: trakrf.generate_permuted_id
4. Added:   trakrf.generate_obfuscated_id (TRIGGER), trakrf._feistel_encrypt (pure)
5. Added:   trakrf.update_updated_at_column body unchanged but now also lives in trakrf schema only
6. RLS policy quals: (current_setting(...))::integer → (current_setting(...))::bigint
7. Added column: tag_scans.id BIGINT IDENTITY (TRA-836 fold-in)
   PK shape change: (created_at, message_topic) → (created_at, id)
8. Added extension: pgcrypto (was implicit on Cloud, explicit here)
9. Removed extension: ltree
10. Function bodies (process_tag_scans, create_asset_with_tags, create_location_with_tags):
    INT params/locals → BIGINT params/locals. Return TABLE column types BIGINT.
11. ID-related comments mention "keyed Feistel" instead of "hashed ID" / "permuted ID".

Diff lines NOT covered by these categories are regressions.
```

- [ ] **Step 5: Run the schema diff**

```bash
just backend db-diff-old-vs-new
# Review /tmp/schema_diff.txt manually.
less /tmp/schema_diff.txt
```

Expected: every diff line falls into one of the 11 categories in the allowlist. If a diff doesn't fit any category, it's a regression — investigate and fix the new migration. Iterate Tasks 4–12 + this step as needed until the diff matches the allowlist.

- [ ] **Step 6: Commit the recipe + allowlist**

```bash
git add backend/justfile backend/database/test/expected_diff_allowlist.txt
git commit -m "$(cat <<'EOF'
test(migrations): schema-diff recipe for TRA-720 clean-stack vs legacy

just backend db-diff-old-vs-new applies both stacks to ephemeral DBs,
pg_dump --schema-only, diffs. Reviewer compares against allowlist of
11 expected categories. Any line outside is a regression.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 14: Run full Go test suite against the new stack

**Files:**
- N/A (verification only)

- [ ] **Step 1: Reset DB and apply full new stack via golang-migrate**

```bash
just database reset    # answer 'yes'
just database up
# Set the obfuscation key on the local DB (matches Go test vector key)
psql "$PG_URL_LOCAL" -c "ALTER DATABASE $(psql $PG_URL_LOCAL -At -c 'SELECT current_database()') SET app.obfuscation_key = '6f626675736361746f72746573746b657920303132333435363738396162636465';"
# Apply all 10 migrations:
migrate -path backend/migrations -database "$PG_URL_LOCAL" up
```

Expected: `10/u <name>` lines for each migration applied without error.

- [ ] **Step 2: Run integration tests**

```bash
cd backend && just test-integration
```

Expected: all tests PASS. If any fail, the most likely causes are:
- Go code casting query results to int32 (must be int64 — Go `int` works on linux/amd64)
- Stored procedure signature mismatch (BIGINT vs INT in pgx call)
- Missing GUC: `app.current_org_id` set as `int` and cast at function call site — should still work but verify

- [ ] **Step 3: Run contract tests**

```bash
just test-contract
```

Expected: all PASS. The schemathesis test seed uses natural-key idempotency so it works against either schema.

- [ ] **Step 4: Commit any test fixes needed**

If tests required fixes, commit them:
```bash
git add <paths>
git commit -m "$(cat <<'EOF'
fix(tests): adjust integration tests for new schema (TRA-720)

<one-line summary of what changed>

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

If no fixes needed, no commit. Move on.

---

## Task 15: Go cleanup — remove `SurrogateIDMax` and offset cap

**Files:**
- Modify: `backend/internal/util/httputil/parseid.go`
- Modify: `backend/internal/util/httputil/listparams.go`

- [ ] **Step 1: Read and patch `parseid.go`**

```bash
grep -n "SurrogateIDMax\|MaxInt32" backend/internal/util/httputil/parseid.go
```

Open the file and remove:
- The `SurrogateIDMax` constant declaration
- Any cap reference in `ParseSurrogateID`; change to call `ParsePathInt(field, raw, 1, math.MaxInt64)`.

If `ParsePathInt` doesn't accept 64-bit upper, change it to. Update import to `math/big` only if needed (likely not — `math.MaxInt64` is a constant).

- [ ] **Step 2: Read and patch `listparams.go`**

```bash
grep -n "SurrogateIDMax\|2147483647" backend/internal/util/httputil/listparams.go
```

Remove the offset cap branch and the conditional error return for offset > SurrogateIDMax.

- [ ] **Step 3: Run unit tests**

```bash
cd backend && go test ./internal/util/httputil/...
```

Expected: all pass. If a test asserts the old cap behaviour, update or delete that test (it tests behaviour we're explicitly removing).

- [ ] **Step 4: Commit**

```bash
git add backend/internal/util/httputil/parseid.go backend/internal/util/httputil/listparams.go backend/internal/util/httputil/*_test.go
git commit -m "$(cat <<'EOF'
refactor(httputil): remove int32 ceiling caps (TRA-720)

SurrogateIDMax constant and offset cap branch existed solely to bridge
the wire (int64) / storage (int4) divergence from TRA-719's BB35 B7 fix.
Storage is now bigint; the divergence is closed; the caps are obsolete.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 16: Go cleanup — remove `validate:max=2147483647` tags

**Files:**
- Modify: `backend/internal/models/location/location.go`
- Search for and modify any other model files with the tag

- [ ] **Step 1: Find all sites**

```bash
grep -rn 'validate:"[^"]*max=2147483647' backend/internal/models/
```

Expected matches: at least `location.go` lines 42 and 106. Any other model fields with the same tag pattern: list them.

- [ ] **Step 2: Remove the cap from each tag**

Pattern to replace:
- From: `validate:"omitempty,min=1,max=2147483647"`
- To:   `validate:"omitempty,min=1"`

Apply to every match found in Step 1.

- [ ] **Step 3: Run model tests**

```bash
cd backend && go test ./internal/models/...
```

Expected: all pass.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/models/
git commit -m "$(cat <<'EOF'
refactor(models): drop max=2147483647 validate tags (TRA-720)

These caps existed to enforce the int32 storage ceiling. Storage is now
bigint; the caps would unnecessarily reject valid IDs in the new range.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 17: Go cleanup — remove swag `maximum(2147483647) format(int32)` annotations

**Files:**
- Modify: Several handler files with swag annotations

- [ ] **Step 1: Find all sites**

```bash
grep -rn "maximum(2147483647)" backend/internal/handlers/
```

Expected: ~40 sites across various handlers (users.go, orgs/*.go, locations.go, assets/*.go, reports/*.go).

- [ ] **Step 2: Remove the qualifiers from each annotation**

For each match, remove the ` maximum(2147483647)` and ` format(int32)` substrings from the `@Param` line. Example before/after:
```
// Before: // @Param id path int true "User ID" minimum(1) maximum(2147483647) format(int32)
// After:  // @Param id path int true "User ID" minimum(1)
```

- [ ] **Step 3: Regenerate the OpenAPI spec**

```bash
cd backend && just api-spec
# Verify the public spec no longer references the cap or int32 format:
grep -c "2147483647\|format: int32" docs/api/openapi.public.yaml
```

The committed public spec lives at `docs/api/openapi.public.yaml` (and `.json`). Internal swag output is at `backend/docs/swagger.json` (input to api-spec), and `backend/internal/handlers/swaggerspec/openapi.internal.{json,yaml}` (gitignored, embedded into the binary). Expected count after Task 17: 0 references to the cap or `format: int32` on path/query params.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handlers/ backend/docs/
git commit -m "$(cat <<'EOF'
refactor(handlers): drop swag maximum(2147483647) format(int32) (TRA-720)

These annotations declared the spec to int32-bound surrogate IDs. With
bigint storage and int64 wire format, they were already misleading even
under TRA-719's wire-only fix; now they're flat wrong.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 18: Go cleanup — remove `markSurrogateIDsInt64` postprocess

**Files:**
- Modify: `backend/internal/tools/apispec/postprocess.go`

- [ ] **Step 1: Identify the call site and helpers**

```bash
grep -n "markSurrogateIDsInt64\|isSurrogateIDName" backend/internal/tools/apispec/postprocess.go
```

Expected:
- `markSurrogateIDsInt64(doc)` — call site in `Postprocess` (or similar) function.
- `func markSurrogateIDsInt64(doc *openapi3.T) { ... }` — function body.
- `func isSurrogateIDName(name string) bool { ... }` — helper used only by `markSurrogateIDsInt64`.

- [ ] **Step 2: Remove**

Delete:
- The call to `markSurrogateIDsInt64(doc)` from the postprocess chain.
- The function `markSurrogateIDsInt64` and its body (entire function).
- The helper `isSurrogateIDName` (no longer referenced).
- Any imports the deletions made unused.

- [ ] **Step 3: Regenerate spec and run tests**

```bash
cd backend && just gen-spec
go test ./internal/tools/apispec/...
```

Expected: tests pass. Spec generation succeeds. The generated spec now declares surrogate IDs with whatever the natural Go-type-to-spec mapping produces (typically `format: int64` for `int` on 64-bit platforms, without an explicit `maximum`).

- [ ] **Step 4: Commit**

```bash
git add backend/internal/tools/apispec/
git commit -m "$(cat <<'EOF'
refactor(apispec): remove markSurrogateIDsInt64 postprocess (TRA-720)

This postprocess existed to bridge the wire / storage divergence from
TRA-719's BB35 B7 fix. Storage is now bigint; natural Go-type-to-spec
mapping produces correct int64 types without the manual override.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 19: Go cleanup — revert "must be a positive integer ≤ %d" error messages and `info.description` paragraph

**Files:**
- Modify: error-message source(s) — find via grep
- Modify: `info.description` source (likely in `apispec/emit.go` or similar)

- [ ] **Step 1: Verify the three error message sites**

Per pre-flight inspection (subject to drift since plan authoring; re-grep before edit):

- `backend/internal/handlers/locations/locations.go:971` — `"parent_id %q must be a positive integer ≤ %d", s, httputil.SurrogateIDMax`
- `backend/internal/handlers/reports/current_locations.go:138` — `"location_id %q must be a positive integer ≤ %d"`
- `backend/internal/handlers/reports/current_locations.go:154` — `"asset_id %q must be a positive integer ≤ %d"`

Confirm with:
```bash
grep -rn "must be a positive integer ≤" backend/internal/
```

- [ ] **Step 2: Revert each to plain positive-integer wording**

For each match:
- From: `fmt.Sprintf("<field> %q must be a positive integer ≤ %d", s, httputil.SurrogateIDMax)`
- To:   `fmt.Sprintf("<field> %q must be a positive integer", s)`

Note that `httputil.SurrogateIDMax` has already been removed in Task 15 — if Task 15 was completed in order, the code currently fails to compile until this task is finished.

- [ ] **Step 3: Remove the `info.description` "Surrogate ID width" paragraph**

The source is `backend/internal/tools/apispec/postprocess.go` around lines 778–800. Locate via:
```bash
grep -n "Surrogate ID width" backend/internal/tools/apispec/postprocess.go
```

Remove:
- The `const marker = "Surrogate ID width"` declaration
- The `policy := "Surrogate ID width: declared \`format: int64\` on the wire " + ...` paragraph
- The function/block that injects it into `doc.Info.Description`

Also remove any test fixtures asserting the paragraph's presence (search `apispec/*_test.go`).

- [ ] **Step 4: Regenerate spec and run tests**

```bash
cd backend && just api-spec
go test ./...
# Confirm the paragraph is gone from the committed spec:
grep -c "Surrogate ID width" docs/api/openapi.public.yaml
```

Expected: count = 0.

- [ ] **Step 5: Commit**

```bash
git add backend/
git commit -m "$(cat <<'EOF'
refactor(spec): revert wire/storage divergence error messages and info.description (TRA-720)

Removes the "≤ %d" upper-bound text from validation errors and the
"Surrogate ID width" paragraph from the OpenAPI info.description.
Both existed only to document the wire (int64) vs storage (int4)
divergence which no longer exists.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 20: Full local test pass

**Files:**
- N/A (verification only)

- [ ] **Step 1: Reset, apply, test**

```bash
just database reset    # answer 'yes'
just database up
psql "$PG_URL_LOCAL" -c "ALTER DATABASE $(psql $PG_URL_LOCAL -At -c 'SELECT current_database()') SET app.obfuscation_key = '6f626675736361746f72746573746b657920303132333435363738396162636465';"
migrate -path backend/migrations -database "$PG_URL_LOCAL" up

cd backend && just lint
cd backend && go test ./...
cd backend && just test-integration
just test-contract
```

Each step expected to pass clean. Fix any failures and commit the fix as its own commit before continuing.

- [ ] **Step 2: Manual smoke test in browser**

```bash
just dev    # or whatever brings up backend + frontend stack
```

In the browser:
1. Sign up a new account.
2. Log in.
3. Create a location.
4. Create an asset.
5. Attach a tag (RFID value) to the asset.
6. Run a CSV bulk import.
7. Verify all IDs in URLs are in the `[2^50, 2^51)` range (`>= 1125899906842624`).

If anything is broken, file a fix commit before proceeding.

- [ ] **Step 3: Commit any final fixes (if applicable)**

```bash
git add <paths>
git commit -m "..."
```

---

## Task 21: Apply and validate on GKE preview CNPG

**Files:**
- N/A (operational task, no code changes)

- [ ] **Step 1: Confirm GKE preview psql access**

```bash
# Per discussion with infra, the env var should be PG_URL_GKE_PREVIEW or similar.
psql "$PG_URL_GKE_PREVIEW" -c "SELECT current_database(), version();"
```

If access not ready yet, mark this task as blocked-pending-infra and skip to Task 22. The local pass in Task 20 is the primary gate.

- [ ] **Step 2: Drop schema and re-apply new stack**

```bash
psql "$PG_URL_GKE_PREVIEW" -c "DROP SCHEMA IF EXISTS trakrf CASCADE;"
psql "$PG_URL_GKE_PREVIEW" -c "DROP EXTENSION IF EXISTS pgcrypto;"
psql "$PG_URL_GKE_PREVIEW" -c "DROP EXTENSION IF EXISTS timescaledb CASCADE;"
psql "$PG_URL_GKE_PREVIEW" -c "ALTER DATABASE $(psql $PG_URL_GKE_PREVIEW -At -c 'SELECT current_database()') SET app.obfuscation_key = '<GKE_PREVIEW_KEY_FROM_SECRETS>';"
migrate -path backend/migrations -database "$PG_URL_GKE_PREVIEW" up
```

- [ ] **Step 3: Run tests against GKE preview**

```bash
PG_URL="$PG_URL_GKE_PREVIEW" cd backend && go test -tags=integration -p 1 ./...
```

Expected: all PASS.

- [ ] **Step 4: Smoke test the deployed app against GKE preview**

If the GKE preview deployment already points the backend at this DB, hit the URL in a browser and run the same flow as Task 20 step 2. Otherwise, point a local backend at it:

```bash
PG_URL="$PG_URL_GKE_PREVIEW" cd backend && just dev
```

- [ ] **Step 5: Commit any GKE-specific fixes**

```bash
git add <paths>
git commit -m "..."
```

---

## Task 22: Add migrations README

Legacy file deletion already happened in Task 13. This task only adds the documentation.

**Files:**
- Create: `backend/migrations/README.md`

- [ ] **Step 1: Write the migrations README**

`backend/migrations/README.md`:
```markdown
# Migrations

This directory contains the canonical schema definition as a set of
versioned SQL files applied in numeric order by golang-migrate.

## Layout

The 10 foundational files (`000001`–`000010`) define the schema by concern,
not chronology. Each file is up-only. Future incremental changes
(`000011`+) follow the conventional up+down pattern.

## History

The pre-TRA-720 stack contained 44 migration files representing schema
evolution: tenant model pivots, column renames, denormalization removals.
Those files were collapsed into this clean stack as part of TRA-720 / the
GKE/CNPG cutover (TRA-810).

To inspect the pre-TRA-720 stack:

    git checkout pre-tra-720 -- backend/migrations
    ls backend/migrations          # see the 88 legacy files

Or browse via the tag on GitHub: <https://github.com/trakrf/platform/releases/tag/pre-tra-720>

## Conventions

- **Up-only foundation.** Files `000001`–`000010` have no down-migration.
  They are the schema baseline; rolling them back means dropping the
  schema entirely.
- **Up+down for increments.** Any migration added after `000010` follows
  the conventional pattern (`000011_<topic>.up.sql` and
  `000011_<topic>.down.sql`).
- **Idempotent where possible.** `CREATE EXTENSION IF NOT EXISTS`,
  `CREATE SCHEMA IF NOT EXISTS`, etc. — guards against double-apply on
  recovery scenarios.

## Required GUC

`trakrf.generate_obfuscated_id()` reads `app.obfuscation_key` via
`current_setting()`. The key must be set on the target database before
any insert hits a Feistel trigger:

    ALTER DATABASE <db> SET app.obfuscation_key = '<64-hex-char-secret>';

This is normally handled at CNPG provisioning time; see TRA-810 for the
data cutover sequence.
```

- [ ] **Step 2: Commit**

```bash
git add backend/migrations/README.md
git commit -m "$(cat <<'EOF'
docs(migrations): README explaining the clean-stack rewrite (TRA-720)

Documents the up-only foundation convention and points at the
pre-tra-720 tag for legacy migration history.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Task 23: Push branch and prepare PR

**Files:**
- N/A (push + PR creation)

- [ ] **Step 1: Push the branch**

```bash
git push -u origin worktree-tra-720-bigint-migration:feat/tra-720-clean-schema-stack
```

(or whichever target branch name matches the project convention)

- [ ] **Step 2: Create the PR**

```bash
gh pr create --title "TRA-720: clean schema stack with bigint, unified Feistel, tag_scans surrogate key" --body "$(cat <<'EOF'
## Summary

- Replace 44-file migration stack with 10 clean files defining the canonical end-state schema
- All surrogate PK/FK columns widened to BIGINT
- Single keyed-Feistel ID generator (`trakrf.generate_obfuscated_id`) replaces legacy `generate_hashed_id` + `generate_permuted_id`
- New IDs land in `[2^50, 2^51)` — disjoint from migrated 31-bit IDs
- `tag_scans` gains surrogate id BIGINT IDENTITY (folds TRA-836)
- Go-side cleanup: drop `SurrogateIDMax`, validate caps, swag annotations, `markSurrogateIDsInt64` postprocess, error message customizations, `info.description` paragraph
- Legacy migrations preserved in git via `pre-tra-720` tag

## Test plan

- [x] Local stack applies cleanly to empty Postgres+TimescaleDB
- [x] Go reference Feistel matches PL/pgSQL via blessed vectors at `backend/internal/obfuscatedid/testdata/vectors.json`
- [x] `just backend db-diff-old-vs-new` diff matches allowlist
- [x] `cd backend && just test-integration` green
- [x] `just test-contract` green
- [x] Manual smoke (signup → login → asset/location/tag CRUD → bulk import) green
- [ ] Same suite green on GKE preview CNPG (Task 21; mark in PR once complete)

## Handoff to TRA-810

This PR delivers the schema only. TRA-810 owns the data move and cutover:

1. Provision empty `trakrf` database on target CNPG (preview, then prod).
2. Set `ALTER DATABASE trakrf SET app.obfuscation_key = '<secret>'`.
3. Apply this migration stack (automatic on app boot via golang-migrate).
4. Develop and prove the FDW pull script using GKE preview ← Cloud preview as the test bench.
5. Execute against prod CNPG: maintenance window, FDW pull, verify, advance the two BIGSERIAL sequences past their migrated max IDs.
6. Cutover connect strings, soak with Cloud in read-only fallback (24-48h), decommission Cloud.

## Cloud-coexistence note

This PR breaks the embedded-migration contract with Cloud envs (Cloud preview / Cloud prod are at legacy schema_migrations.version = 44; new stack restarts at 1). Before this PR merges, Cloud envs must be pinned to `pre-tra-720` or have `MIGRATIONS_MODE=disabled` deployed. Confirm with infra.

## Closes / fixes

- Closes TRA-720
- Folds in TRA-836 (closes-via-rewrite)

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

- [ ] **Step 3: Push the `pre-tra-720` tag**

```bash
git push origin pre-tra-720
```

- [ ] **Step 4: Comment on TRA-810 with handoff details**

Use Linear's `save_comment` (or the MCP tool) to post the same handoff section from the PR body as a comment on TRA-810. This ensures the cutover team has the operational sequence captured in their ticket regardless of PR-discovery context.

- [ ] **Step 5: Update TRA-836 status**

Mark TRA-836 as resolved with link to this PR ("Folded into TRA-720 via PR #<N>; closes on merge").

---

## Self-review checklist (engineer runs before declaring done)

After completing all tasks, verify:

1. **Spec coverage.** Re-read [the design doc](../specs/2026-05-26-tra-720-clean-schema-stack-design.md) sections 1–6 and confirm each commitment has a corresponding completed task above.

2. **Schema diff allowlist.** Every diff line from `just backend db-diff-old-vs-new` falls into one of the 11 allowlist categories. Any uncategorized diff = investigate.

3. **Test suite.** All of these green:
   - `cd backend && just test-integration`
   - `just test-contract`
   - `go test ./internal/obfuscatedid/...` (Go-side parity)
   - `./backend/database/test/run_feistel_parity.sh` (SQL-side parity)

4. **Manual smoke.** Browser flow on local + GKE preview, IDs in `[2^50, 2^51)` range visible in URLs.

5. **Cloud pinning confirmed with infra.** Pre-merge — verify Cloud envs are pinned to `pre-tra-720` so this merge doesn't break them.

6. **TRA-836 status.** Marked as folded-in-via-TRA-720.

If all checks pass: PR ready for review.
