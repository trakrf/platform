# `deploy/edge/` — TrakRF Demo-Box Runtime — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the TrakRF fixed-reader stack as rootless Podman quadlets on the demo box (`trakrf-demo`) and prove the ingest→geofence→fire chain on simulated MQTT, then add the Traefik TLS edge for Tim's laptop.

**Architecture:** Five rootless Podman quadlets (Timescale, Mosquitto, a one-shot migrate, backend, Traefik) on one user network, systemd-managed with `enable-linger`. Tracks `ghcr.io/trakrf/backend:preview` (public, multi-arch) via `AutoUpdate=registry`. Migrate runs before serve on every start via `Requires=`/`After=`. Traefik terminates TLS for `app.demo.trakrf.id` with a statically-provisioned LE cert.

**Tech Stack:** Podman 4.9 quadlets + systemd `--user`, TimescaleDB, eclipse-mosquitto, Traefik v3, Go backend (`/server migrate|serve`), lego (Cloudflare DNS-01).

**Spec:** `docs/superpowers/specs/2026-06-07-deploy-edge-design.md`. **Branch:** `feat/tra-898-deploy-edge` (exists). **Box facts:** Zorin 18.1, amd64, passwordless sudo, on tailnet; podman NOT yet installed; repo at `/home/mike/platform`.

**Conventions:** Quadlet source lives in `deploy/edge/quadlets/`; an install script symlinks them into `~/.config/containers/systemd/`. Volume paths in quadlets use `%h/platform/deploy/edge/...` (`%h`=`/home/mike`). Secrets live in gitignored `deploy/edge/.env` (`EnvironmentFile=`).

---

### Task 0: Scaffold `deploy/edge/` + gitignore

**Files:**
- Create: `deploy/edge/.gitignore`, `deploy/edge/.env.example`, `deploy/edge/README.md` (stub)

- [ ] **Step 1: Create directory tree**

Run: `mkdir -p deploy/edge/quadlets deploy/edge/mosquitto deploy/edge/traefik/certs`

- [ ] **Step 2: Write `deploy/edge/.gitignore`**

```gitignore
# Secrets and generated artifacts — never commit
.env
mosquitto/passwd
traefik/certs/
```

- [ ] **Step 3: Write `deploy/edge/.env.example`** (placeholders only, no real secrets)

```dotenv
# Postgres (local Timescale container)
POSTGRES_PASSWORD=CHANGEME
POSTGRES_DB=postgres
PG_URL=postgres://postgres:CHANGEME@timescaledb:5432/postgres?sslmode=disable&options=-c%20search_path%3Dtrakrf,public
# MQTT (local Mosquitto, basic auth) — password must match mosquitto/passwd
MQTT_URL=mqtt://trakrf-mqtt:CHANGEME@mosquitto:1883
MQTT_TOPIC=trakrf.id/#
MQTT_CLIENT_ID=trakrf-demo-box
# Backend
JWT_SECRET=CHANGEME
JWT_EXPIRATION=3600
BACKEND_PORT=8080
BACKEND_LOG_LEVEL=info
```

- [ ] **Step 4: Write `deploy/edge/README.md` stub** (filled in Task 11)

```markdown
# deploy/edge — TrakRF demo-box runtime

Rootless Podman quadlets for the offline demo box. See the design spec at
`docs/superpowers/specs/2026-06-07-deploy-edge-design.md`. Runbook below (WIP).
```

- [ ] **Step 5: Commit**

```bash
git add deploy/edge/.gitignore deploy/edge/.env.example deploy/edge/README.md
git commit -m "feat(tra-898): scaffold deploy/edge tree + gitignore"
```

---

### Task 1: Install Podman + enable linger

**Files:** none (system prep)

- [ ] **Step 1: Install podman + mosquitto-clients (for `mosquitto_pub` in the smoke test)**

Run: `sudo apt-get update && sudo apt-get install -y podman mosquitto-clients`

- [ ] **Step 2: Verify podman**

Run: `podman --version`
Expected: `podman version 4.9.x`

- [ ] **Step 3: Enable linger so user units start at boot without login**

Run: `loginctl enable-linger mike && loginctl show-user mike -p Linger`
Expected: `Linger=yes`

- [ ] **Step 4: Create the quadlet drop-in dir**

Run: `mkdir -p ~/.config/containers/systemd`

- [ ] **Step 5: Confirm the backend image entrypoint** (we assume `/server`; verify the `Exec=` args)

Run: `podman pull ghcr.io/trakrf/backend:preview && podman inspect ghcr.io/trakrf/backend:preview --format '{{.Config.Entrypoint}} {{.Config.Cmd}}'`
Expected: entrypoint contains `/server` (so `Exec=migrate` → `/server migrate`). If the entrypoint differs, adjust `Exec=` in Tasks 5/6 accordingly.

No commit (system state).

---

### Task 2: Network + Timescale quadlet

**Files:**
- Create: `deploy/edge/quadlets/trakrf.network`, `deploy/edge/quadlets/timescaledb.container`, `deploy/edge/install.sh`

- [ ] **Step 1: Write `deploy/edge/quadlets/trakrf.network`**

```ini
[Unit]
Description=TrakRF demo internal network

[Network]
NetworkName=trakrf
```

- [ ] **Step 2: Write `deploy/edge/quadlets/timescaledb.container`**

```ini
[Unit]
Description=TimescaleDB (demo box)

[Container]
ContainerName=timescaledb
Image=docker.io/timescale/timescaledb-ha:pg17.9-ts2.26.4
Network=trakrf.network
Volume=timescale_data:/var/lib/postgresql/data
EnvironmentFile=%h/platform/deploy/edge/.env
PublishPort=127.0.0.1:5432:5432
HealthCmd=pg_isready -U postgres
HealthInterval=10s
HealthTimeout=5s
HealthRetries=5

[Service]
Restart=always

[Install]
WantedBy=default.target
```

- [ ] **Step 3: Write `deploy/edge/install.sh`** (symlinks quadlets, reloads systemd)

```bash
#!/usr/bin/env bash
# Symlink deploy/edge quadlets into the rootless systemd user dir and reload.
set -euo pipefail
SRC="$(cd "$(dirname "$0")/quadlets" && pwd)"
DEST="$HOME/.config/containers/systemd"
mkdir -p "$DEST"
for f in "$SRC"/*.container "$SRC"/*.network; do
  [ -e "$f" ] || continue
  ln -sf "$f" "$DEST/$(basename "$f")"
done
systemctl --user daemon-reload
echo "Linked quadlets:"; ls -l "$DEST"
```

- [ ] **Step 4: Make `.env` from the example with a real Postgres password**

Run:
```bash
cp deploy/edge/.env.example deploy/edge/.env
PGPW=$(openssl rand -hex 16)
sed -i "s|POSTGRES_PASSWORD=CHANGEME|POSTGRES_PASSWORD=$PGPW|" deploy/edge/.env
sed -i "s|postgres://postgres:CHANGEME@|postgres://postgres:$PGPW@|" deploy/edge/.env
```

- [ ] **Step 5: Install + start Timescale**

Run:
```bash
chmod +x deploy/edge/install.sh && deploy/edge/install.sh
systemctl --user start timescaledb.service
```

- [ ] **Step 6: Verify Timescale is healthy**

Run: `sleep 8 && podman healthcheck run timescaledb && podman exec timescaledb pg_isready -U postgres`
Expected: `... accepting connections`

- [ ] **Step 7: Commit**

```bash
git add deploy/edge/quadlets/trakrf.network deploy/edge/quadlets/timescaledb.container deploy/edge/install.sh
git commit -m "feat(tra-898): network + timescale quadlets + install script"
```

---

### Task 3: Mosquitto config + quadlet + auth

**Files:**
- Create: `deploy/edge/mosquitto/mosquitto.conf`, `deploy/edge/quadlets/mosquitto.container`

- [ ] **Step 1: Write `deploy/edge/mosquitto/mosquitto.conf`** (prod chart, stripped to plain 1883 + basic auth)

```conf
per_listener_settings true
listener 1883 0.0.0.0
allow_anonymous false
password_file /mosquitto/config/passwd
persistence false
persistence_location /mosquitto/data/
log_dest stdout
```

- [ ] **Step 2: Write `deploy/edge/quadlets/mosquitto.container`**

```ini
[Unit]
Description=Mosquitto broker (demo box)

[Container]
ContainerName=mosquitto
Image=docker.io/library/eclipse-mosquitto:2.0.21
Network=trakrf.network
Volume=%h/platform/deploy/edge/mosquitto/mosquitto.conf:/mosquitto/config/mosquitto.conf:ro,Z
Volume=%h/platform/deploy/edge/mosquitto/passwd:/mosquitto/config/passwd:ro,Z
PublishPort=1883:1883

[Service]
Restart=always

[Install]
WantedBy=default.target
```

- [ ] **Step 3: Generate a box-local MQTT password and write it into `.env` (MQTT_URL)**

Run:
```bash
MQPW=$(openssl rand -hex 12)
sed -i "s|mqtt://trakrf-mqtt:CHANGEME@|mqtt://trakrf-mqtt:$MQPW@|" deploy/edge/.env
echo "$MQPW" > /tmp/mqpw   # temp, for the next step; deleted after
```

- [ ] **Step 4: Create the hashed `passwd` file via the mosquitto image**

Run:
```bash
touch deploy/edge/mosquitto/passwd
podman run --rm -v "$PWD/deploy/edge/mosquitto/passwd:/passwd:Z" \
  --entrypoint mosquitto_passwd docker.io/library/eclipse-mosquitto:2.0.21 \
  -b /passwd trakrf-mqtt "$(cat /tmp/mqpw)"
rm -f /tmp/mqpw
```
Expected: `deploy/edge/mosquitto/passwd` contains one `trakrf-mqtt:$7$...` line.

- [ ] **Step 5: Install + start Mosquitto**

Run: `deploy/edge/install.sh && systemctl --user start mosquitto.service`

- [ ] **Step 6: Verify auth works (good creds publish; anon is rejected)**

Run:
```bash
PW=$(grep -oP 'trakrf-mqtt:\K[^@]+' <<<"$(grep MQTT_URL deploy/edge/.env)")
mosquitto_pub -h 127.0.0.1 -p 1883 -u trakrf-mqtt -P "$PW" -t 'trakrf.id/test' -m hi && echo "AUTH OK"
mosquitto_pub -h 127.0.0.1 -p 1883 -t 'trakrf.id/test' -m hi; echo "anon exit=$? (expect non-zero)"
```
Expected: `AUTH OK`, then a connection-refused/not-authorized non-zero exit for the anonymous attempt.

- [ ] **Step 7: Commit** (passwd is gitignored)

```bash
git add deploy/edge/mosquitto/mosquitto.conf deploy/edge/quadlets/mosquitto.container
git commit -m "feat(tra-898): mosquitto quadlet + basic-auth config"
```

---

### Task 4: Finalize backend secrets (`JWT_SECRET`)

**Files:** modifies gitignored `deploy/edge/.env` (no commit)

- [ ] **Step 1: Generate JWT secret**

Run: `sed -i "s|JWT_SECRET=CHANGEME|JWT_SECRET=$(openssl rand -hex 32)|" deploy/edge/.env`

- [ ] **Step 2: Verify `.env` has no remaining CHANGEME**

Run: `! grep -q CHANGEME deploy/edge/.env && echo "all secrets set"`
Expected: `all secrets set`

No commit (`.env` is gitignored).

---

### Task 5: Migrate one-shot quadlet

**Files:**
- Create: `deploy/edge/quadlets/migrate.container`

- [ ] **Step 1: Write `deploy/edge/quadlets/migrate.container`** (waits for DB, idempotent, re-runs on every backend start)

```ini
[Unit]
Description=TrakRF DB migrate (runs before backend, every start)
After=timescaledb.service
Requires=timescaledb.service

[Container]
ContainerName=trakrf-migrate
Image=ghcr.io/trakrf/backend:preview
Network=trakrf.network
EnvironmentFile=%h/platform/deploy/edge/.env
Exec=migrate
AutoUpdate=registry

[Service]
Type=oneshot
RemainAfterExit=no
# Wait up to 60s for Postgres to accept connections before migrating.
ExecStartPre=/bin/sh -c 'for i in $(seq 1 30); do podman exec timescaledb pg_isready -U postgres -q && exit 0; sleep 2; done; exit 1'
```

- [ ] **Step 2: Install + run migrate**

Run: `deploy/edge/install.sh && systemctl --user start migrate.service`

- [ ] **Step 3: Verify migrations applied (schema present)**

Run: `podman exec timescaledb psql -U postgres -c "SELECT count(*) FROM schema_migrations;" && podman exec timescaledb psql -U postgres -c "\dt trakrf.*" | head`
Expected: a non-error `schema_migrations` count and a list of `trakrf.*` tables (e.g. `asset_scans`, `tag_scans`, `assets`, `tags`, `scan_devices`).

- [ ] **Step 4: Verify idempotent re-run is a no-op**

Run: `systemctl --user start migrate.service && systemctl --user show migrate.service -p ExecMainStatus`
Expected: `ExecMainStatus=0` (success; migrate short-circuits at current version).

- [ ] **Step 5: Commit**

```bash
git add deploy/edge/quadlets/migrate.container
git commit -m "feat(tra-898): migrate one-shot quadlet (out-of-band, idempotent)"
```

---

### Task 6: Backend (serve) quadlet

**Files:**
- Create: `deploy/edge/quadlets/backend.container`

- [ ] **Step 1: Write `deploy/edge/quadlets/backend.container`**

```ini
[Unit]
Description=TrakRF backend (serve)
After=migrate.service mosquitto.service
Requires=migrate.service

[Container]
ContainerName=backend
Image=ghcr.io/trakrf/backend:preview
Network=trakrf.network
EnvironmentFile=%h/platform/deploy/edge/.env
Exec=serve
PublishPort=127.0.0.1:8080:8080
AutoUpdate=registry

[Service]
Restart=always

[Install]
WantedBy=default.target
```

- [ ] **Step 2: Install + start backend**

Run: `deploy/edge/install.sh && systemctl --user start backend.service`

- [ ] **Step 3: Verify `/health`**

Run: `sleep 5 && curl -fsS http://127.0.0.1:8080/health && echo " HEALTH OK"`
Expected: a JSON health/build-info body + `HEALTH OK`.

- [ ] **Step 4: Verify the MQTT subscriber connected to the local broker**

Run: `podman logs backend 2>&1 | grep -iE 'mqtt|subscrib' | tail`
Expected: a log line indicating the subscriber connected (NOT "MQTT subscriber disabled (MQTT_URL unset)").

- [ ] **Step 5: Commit**

```bash
git add deploy/edge/quadlets/backend.container
git commit -m "feat(tra-898): backend serve quadlet"
```

---

### Task 7: Simulated-MQTT smoke test (the proof)

**Files:**
- Create: `deploy/edge/smoke-test.sh`

- [ ] **Step 1: Discover the seeding mechanism + API routes**

Run:
```bash
ls backend/database/seeds 2>/dev/null; ls database 2>/dev/null
grep -rniE 'seed|demo.?data' backend/internal/cmd backend/main.go | head
# Find create endpoints for org/asset/tag/scan_point/alarm device:
grep -rniE 'POST|Handle(Func)?|router\.|chi\.|mux\.' backend/internal/cmd/serve/router.go | grep -iE 'asset|tag|scan|org|alarm|device|login|token' | head -40
```
Note the exact create routes + the auth method (JWT login or a seed command). Prefer an existing seed/demo-data path (TRA-904) if present; otherwise seed via authenticated API calls. Record the routes you'll use in the script comments.

- [ ] **Step 2: Write `deploy/edge/smoke-test.sh`** — seed a minimal fixture **via the API** (so the geofence in-memory armed-EPC set refreshes), publish a synthetic CS463 read, assert.

```bash
#!/usr/bin/env bash
# Proves ingest -> resolve -> geofence -> fire on simulated MQTT.
# Seeds via the backend API (NOT raw SQL) so TRA-901's in-memory armed-EPC set refreshes.
set -euo pipefail
BASE=${BASE:-http://127.0.0.1:8080}
EPC=${EPC:-000000000000000000010023}
BOUNDARY=${BOUNDARY:-door-1}
MQPW=$(grep -oP 'trakrf-mqtt:\K[^@]+' deploy/edge/.env)

# 1) Auth + seed fixture via API. EXACT endpoints/bodies filled from Task 7 Step 1 discovery:
#    - obtain JWT (login or seed token)
#    - create org, asset, tag(value=$EPC, armed), scan_device + scan_point(name=$BOUNDARY, boundary),
#      mock alarm device (HTTP target -> a local sink or a no-op).
#    (Implement using the discovered routes; keep each call `curl -fsS`.)

# 2) Publish a synthetic CS463 read for the armed EPC at the boundary.
NOW_US=$(( $(date +%s) * 1000000 ))
PAYLOAD=$(printf '{"tags":[{"epc":"%s","timeStampOfRead":%d,"antennaPort":1,"capturePointName":"%s","rssi":"-55"}]}' "$EPC" "$NOW_US" "$BOUNDARY")
mosquitto_pub -h 127.0.0.1 -p 1883 -u trakrf-mqtt -P "$MQPW" -t "trakrf.id/$BOUNDARY" -m "$PAYLOAD"

# 3) Assert: a row landed in asset_scans for the EPC.
sleep 2
ROWS=$(podman exec timescaledb psql -U postgres -tAc \
  "SELECT count(*) FROM trakrf.asset_scans WHERE epc = '$EPC';")
echo "asset_scans rows for $EPC: $ROWS"
[ "$ROWS" -ge 1 ] || { echo "FAIL: no asset_scans row"; exit 1; }

# 4) Assert: geofence fired the alarm dispatch (check backend logs for the trigger).
podman logs backend 2>&1 | grep -iE 'geofence|alarm|fire|dispatch' | tail
echo "SMOKE TEST PASS"
```

- [ ] **Step 3: Flesh out the seeding block** using the routes discovered in Step 1 (replace the comment in section 1 with real `curl -fsS` calls). Each call must succeed; no placeholders.

- [ ] **Step 4: Run the smoke test**

Run: `chmod +x deploy/edge/smoke-test.sh && ./deploy/edge/smoke-test.sh`
Expected: `asset_scans rows for … : 1` (or more), a geofence/alarm log line, and `SMOKE TEST PASS`.

- [ ] **Step 5: Commit**

```bash
git add deploy/edge/smoke-test.sh
git commit -m "feat(tra-898): simulated-MQTT smoke test (ingest->geofence->fire proof)"
```

---

### Task 8: Traefik edge with a self-signed cert (proxy wiring)

**Files:**
- Create: `deploy/edge/traefik/traefik.yaml`, `deploy/edge/traefik/dynamic.yaml`, `deploy/edge/quadlets/traefik.container`

- [ ] **Step 1: Write `deploy/edge/traefik/traefik.yaml`** (static; file provider; no ACME)

```yaml
entryPoints:
  websecure:
    address: ":443"
providers:
  file:
    filename: /etc/traefik/dynamic.yaml
    watch: true
log:
  level: INFO
```

- [ ] **Step 2: Write `deploy/edge/traefik/dynamic.yaml`**

```yaml
http:
  routers:
    app:
      rule: "Host(`app.demo.trakrf.id`)"
      entryPoints: [websecure]
      service: backend
      tls: {}
  services:
    backend:
      loadBalancer:
        servers:
          - url: "http://backend:8080"
tls:
  certificates:
    - certFile: /certs/app.demo.trakrf.id.crt
      keyFile: /certs/app.demo.trakrf.id.key
```

- [ ] **Step 3: Write `deploy/edge/quadlets/traefik.container`**

```ini
[Unit]
Description=Traefik TLS edge
After=backend.service
Requires=backend.service

[Container]
ContainerName=traefik
Image=docker.io/library/traefik:v3.3
Network=trakrf.network
Volume=%h/platform/deploy/edge/traefik/traefik.yaml:/etc/traefik/traefik.yaml:ro,Z
Volume=%h/platform/deploy/edge/traefik/dynamic.yaml:/etc/traefik/dynamic.yaml:ro,Z
Volume=%h/platform/deploy/edge/traefik/certs:/certs:ro,Z
PublishPort=443:443

[Service]
Restart=always

[Install]
WantedBy=default.target
```

- [ ] **Step 4: Generate a temporary self-signed cert** (real LE cert in Task 9)

Run:
```bash
openssl req -x509 -newkey rsa:2048 -nodes -days 30 \
  -keyout deploy/edge/traefik/certs/app.demo.trakrf.id.key \
  -out   deploy/edge/traefik/certs/app.demo.trakrf.id.crt \
  -subj "/CN=app.demo.trakrf.id" -addext "subjectAltName=DNS:app.demo.trakrf.id"
```

- [ ] **Step 5: Install + start Traefik**

Run: `deploy/edge/install.sh && systemctl --user start traefik.service`

- [ ] **Step 6: Verify the proxy reaches the backend over TLS**

Run: `curl -k -fsS --resolve app.demo.trakrf.id:443:127.0.0.1 https://app.demo.trakrf.id/health && echo " EDGE OK"`
Expected: the backend health body + `EDGE OK` (`-k` accepts the self-signed cert for now).

- [ ] **Step 7: Commit** (certs/ is gitignored)

```bash
git add deploy/edge/traefik/traefik.yaml deploy/edge/traefik/dynamic.yaml deploy/edge/quadlets/traefik.container
git commit -m "feat(tra-898): traefik TLS edge (self-signed placeholder cert)"
```

---

### Task 9: Real LE cert via Cloudflare DNS-01 (gated on CF token)

**Files:** writes PEMs into gitignored `deploy/edge/traefik/certs/`

> **Gate:** needs a Cloudflare API token. If not yet available, skip this task — Task 8's self-signed cert keeps the edge functional for proxy validation; the real cert is a pre-ship prep step. Web BLE from Tim's laptop requires this task complete.

- [ ] **Step 1: Locate / set the Cloudflare token**

Run: `env | grep -iE 'CLOUDFLARE|CF_' || grep -riE 'cloudflare|CF_API' /home/mike/infra/.env.local 2>/dev/null | head`
Set `CF_DNS_API_TOKEN` in the shell from the discovered value (a DNS-edit-scoped token for `trakrf.id`).

- [ ] **Step 2: Issue the cert with lego (containerized) via DNS-01**

Run:
```bash
mkdir -p deploy/edge/traefik/lego
podman run --rm -e CF_DNS_API_TOKEN \
  -v "$PWD/deploy/edge/traefik/lego:/.lego:Z" \
  docker.io/goacme/lego:latest \
  --email "admin@trakrf.id" --dns cloudflare \
  --domains app.demo.trakrf.id --accept-tos run
```
Expected: certificate + key written under `deploy/edge/traefik/lego/certificates/`.

- [ ] **Step 3: Place PEMs where Traefik reads them**

Run:
```bash
cp deploy/edge/traefik/lego/certificates/app.demo.trakrf.id.crt deploy/edge/traefik/certs/app.demo.trakrf.id.crt
cp deploy/edge/traefik/lego/certificates/app.demo.trakrf.id.key deploy/edge/traefik/certs/app.demo.trakrf.id.key
```

- [ ] **Step 4: Reload Traefik (file provider watches, but restart to be sure) + verify a valid cert**

Run:
```bash
systemctl --user restart traefik.service && sleep 3
curl -fsS --resolve app.demo.trakrf.id:443:127.0.0.1 https://app.demo.trakrf.id/health && echo " VALID-CERT EDGE OK"
```
Expected: success **without** `-k` → the cert chains to a public CA. (Add `lego` to `.gitignore`.)

- [ ] **Step 5: Commit gitignore update**

```bash
printf 'traefik/lego/\n' >> deploy/edge/.gitignore
git add deploy/edge/.gitignore
git commit -m "chore(tra-898): gitignore lego cert workdir"
```

---

### Task 10: Enable all units at boot + reboot validation

**Files:** none (systemd enablement)

- [ ] **Step 1: Enable the long-running services** (`[Install]` already sets `WantedBy=default.target`, but confirm presets after daemon-reload)

Run:
```bash
systemctl --user daemon-reload
systemctl --user list-unit-files | grep -E 'timescaledb|mosquitto|backend|traefik'
```
Expected: each `*.service` present (quadlet-generated, enabled via `[Install]`).

- [ ] **Step 2: Reboot and confirm the stack self-starts** (linger + boot ordering)

Run: `sudo reboot` — then after reconnect (over tailnet):
```bash
systemctl --user is-active timescaledb mosquitto backend traefik
curl -k -fsS --resolve app.demo.trakrf.id:443:127.0.0.1 https://app.demo.trakrf.id/health && echo " POST-REBOOT OK"
```
Expected: all `active`, health OK. (This proves `enable-linger` + `Restart=always` + ordering.)

No commit.

---

### Task 11: README runbook + finalize

**Files:**
- Modify: `deploy/edge/README.md`

- [ ] **Step 1: Write the runbook** — cover: prerequisites (podman, linger), `install.sh`, secrets (`.env` from `.env.example`, `mosquitto_passwd`, `openssl` for JWT), start order, `smoke-test.sh`, the **update model** (tracks `:preview` via `podman auto-update`; `systemctl --user start podman-auto-update.timer`; migrate runs before serve automatically), the **promote/next-iteration** note (`demo`→`prod` tracking), the **cert** procedure (lego DNS-01, drop PEMs, restart traefik), and the **demo-laptop checklist** (Chrome/Edge/Opera; Chrome Secure DNS off; URL `https://app.demo.trakrf.id`). Include the Slate dnsmasq record requirement (`app.demo.trakrf.id → box reserved IP`).

- [ ] **Step 2: Enable the auto-update timer**

Run: `systemctl --user enable --now podman-auto-update.timer && systemctl --user list-timers | grep auto-update`
Expected: the timer is listed/active.

- [ ] **Step 3: Commit + open PR**

```bash
git add deploy/edge/README.md
git commit -m "docs(tra-898): deploy/edge runbook + enable podman-auto-update timer"
git push -u origin feat/tra-898-deploy-edge
gh pr create --fill --base main
```

---

## Self-Review

**Spec coverage:** quadlet stack (T2,3,5,6), rootless+linger (T1), basic-auth broker (T3), track `:preview`+auto-update (T6 images / T11 timer), migrate-on-update (T5 + backend `Requires=`), simulated-MQTT proof (T7), Traefik edge + static cert (T8), LE cert via CF DNS-01 (T9), Slate-DNS + laptop checklist (T11 docs; Slate-side config tracked on TRA-898), boot/reboot resilience (T10). Deferred items (kiosk, Prom/Grafana, real Shelly, hardware) are intentionally out of this plan per the spec.

**Known execution-time discovery (concrete steps, not placeholders):** backend image entrypoint (T1 S5), exact seed/API routes for the smoke fixture (T7 S1→S3), Cloudflare token location (T9 S1). Each is a real command that resolves the unknown before the dependent step.

**Type/name consistency:** container names (`timescaledb`, `mosquitto`, `trakrf-migrate`, `backend`, `traefik`) are referenced consistently across quadlet `Network`/`After`/`Requires`, `podman exec`, and the smoke test. `Exec=migrate`/`Exec=serve` assume entrypoint `/server` (verified T1 S5). `.env` keys match `backend/internal/ingest/config.go` (`MQTT_URL`, `MQTT_TOPIC`) and the compose/`.env.example` (`PG_URL`, `JWT_SECRET`, `BACKEND_PORT`).
