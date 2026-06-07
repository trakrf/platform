# `deploy/edge/` — TrakRF demo-box runtime stack — Design

- **Date:** 2026-06-07
- **Ticket:** TRA-898 (parent epic TRA-897 — Frederick Health fixed-reader asset-egress geofence demo)
- **Status:** Approved design, pre-implementation
- **Author/driver:** Mike (infra; built and run on the box itself)

## Context & goal

A self-contained, offline demo box — HP EliteDesk, **Zorin OS 18.1 (Ubuntu 24.04, amd64), hostname `trakrf-demo`** — running the full TrakRF fixed-reader stack for **conference-room demos**. Tim drives the demo from **his own laptop over the Slate WiFi**; there is no on-box kiosk (see Decisions). The box is set up at Mike's, ships to Tim, and runs **fully offline at the venue** (relies on no network). Mike remote-maintains it over Tailscale during the build/test phase only.

The application pipeline (MQTT ingest → geofence rules engine → fire Shelly) is **already merged and amd64-capable** — TRA-899/900/901/903/909 are all Done. This work is the **runtime that hosts those images on the box**, plus the TLS edge Tim's laptop needs. It lives at `deploy/edge/` in the platform (app) repo.

This session: **author `deploy/edge/` AND bring it up live on the box**, proving the ingest→geofence→fire chain on simulated MQTT before any hardware is attached.

## Scope

**In scope**
- Rootless **Podman quadlets** for: TimescaleDB, Mosquitto, a one-shot DB migrate, the backend, and a **Caddy TLS edge**.
- A local **Mosquitto** broker config (plain `:1883`, basic auth).
- **Digest-pinned** backend image + `podman auto-update` discipline.
- **TLS edge** for the laptop surface (Web Bluetooth requires a secure context off-localhost).
- A **simulated-MQTT smoke test** that proves the full chain.
- A `README.md` runbook (bring-up, promote/digest-pin, laptop checklist).

**Out of scope (deferred, with reasons)**
- **gnome-kiosk / auto-login / cold-start splash** — deprioritized: Tim confirmed conference-room demos are laptop-driven, not kiosk. Reopen only for a trade-show booth.
- **Prometheus + Grafana** — TRA-908 fast-follow; +2 quadlets when wanted, not demo-gating.
- **Real Shelly HTTP wiring + RSSI/doorway tuning** — hardware steps (CS463/Shelly bring-up), done after this stack is proven.
- **Physical hardware bring-up & Slate network config** — tracked on TRA-898's hardware checklist; the Slate-side DNS entry (below) is the one cross-cutting item.

## Architecture

Five containers as **rootless Podman quadlets** under user `mike` (systemd `--user` units in `~/.config/containers/systemd/`), joined on one user network `trakrf` so they resolve each other by container name. `loginctl enable-linger mike` so the stack starts at boot with no interactive login. This mirrors the prod composition (Mosquitto + Timescale + backend-with-subscriber) with the cloud scaffolding (TLS-on-broker, CNPG, LB, cert-manager, exporter) stripped.

```
                 Slate LAN (192.168.8.0/24)
Tim's laptop ──HTTPS──> [caddy :443]  app.demo.trakrf.id
   (Chrome)                  │ reverse-proxy
                             ▼
                       [backend :8080] ──serve──┐
                             │  ▲                │ subscribes/publishes
            migrate (oneshot)│  │ PG_URL         ▼
                             ▼  │           [mosquitto :1883]
                       [timescaledb :5432]   (basic auth)
CS463 / GL-S10 / Moko ──MQTT (trakrf.id/#)──────┘  (hardware, later)
```

## Components — `deploy/edge/`

| File | Purpose |
|---|---|
| `trakrf.network` | user network; backend reaches `timescaledb`/`mosquitto`/`caddy` by name |
| `timescaledb.container` | `timescale/timescaledb-ha:pg17.9-ts2.26.4`; named volume for PGDATA; `pg_isready` healthcheck |
| `mosquitto.container` | `eclipse-mosquitto:2.0.21`; mounts `mosquitto.conf` + `passwd` |
| `mosquitto.conf` | plain `listener 1883`, `allow_anonymous false`, `password_file`, `persistence false`, `log_dest stdout` |
| `migrate.container` | **oneshot** (`Type=oneshot`, `RemainAfterExit`); pinned backend digest running `migrate`; `After=` TS-healthy, `Before=` backend |
| `backend.container` | same pinned digest running `serve`; env wired local; `:8080`; `After=` migrate + mosquitto |
| `caddy.container` | `caddy:2`; terminates TLS for `app.demo.trakrf.id` → `backend:8080`; mounts cert + `Caddyfile` |
| `.env.example` / `.env` (gitignored) | secrets via systemd `EnvironmentFile=` |
| `smoke-test.sh` | seed fixture + publish synthetic read + assert (the proof) |
| `README.md` | bring-up + promote/digest-pin runbook + demo-laptop checklist |

### Backend env (repointed cloud → box)
- `PG_URL=postgres://postgres:<pw>@timescaledb:5432/postgres?sslmode=disable&options=-c search_path=trakrf,public`
- `MQTT_URL=mqtt://trakrf-mqtt:<box-pw>@mosquitto:1883` (was `mqtts://…@mqtt.preview.gke.trakrf.id:8883`)
- `MQTT_TOPIC=trakrf.id/#`, `MQTT_CLIENT_ID=trakrf-demo-box`
- `JWT_SECRET=<generated fresh>` (unset in source env), `JWT_EXPIRATION=3600`, `BACKEND_PORT=8080`, `BACKEND_LOG_LEVEL=info`

### Migrations
The backend ships distinct `migrate` and `serve` subcommands; prod helm invokes both separately (no DDL at runtime — TRA-85). The box mirrors this with a `migrate` one-shot quadlet ordered after Timescale is healthy and before `serve`. This also validates the full migration set against a vanilla Timescale image (a TRA-898 scope item).

### Secrets
`POSTGRES_PASSWORD`, the box-local Mosquitto password, and `JWT_SECRET` live in a gitignored `deploy/edge/.env` consumed via `EnvironmentFile=`. `.env.example` is committed. Rationale: simplest for a sealed, offline, single-tenant box where break-glass is a shell. Podman secrets are the hygienic upgrade if ever wanted.

### Image lifecycle / promote
- `podman login ghcr.io` (using `GH_TOKEN`) → pull `ghcr.io/trakrf/backend:preview` → resolve & **pin `Image=ghcr.io/trakrf/backend@sha256:<digest>`** in the migrate + backend quadlets.
- `AutoUpdate=registry` label + enable `podman-auto-update.timer`. While digest-pinned this is a no-op; **promote = deliberately re-pin** the current `preview` digest, vet, pull. The box only has a GHCR uplink during pre-event prep, never at the venue.
- Requires TRA-909 multi-arch (Done) so `preview` carries an amd64 image.

### TLS edge & DNS (laptop secure-context requirement)
Tim demos the **CS108 handheld over Web Bluetooth from his laptop**. Web Bluetooth (+ clipboard/Web-Share) require a **secure context**, and the laptop hits the box at a non-localhost LAN origin — so **HTTPS is mandatory** (the localhost exemption does not apply). Web BLE is **Chromium-only** (Chrome/Edge/Opera; not Firefox/Safari/iOS).
- **Cert:** Let's Encrypt via **Cloudflare DNS-01** for the scoped name `app.demo.trakrf.id` — **not** the `*.trakrf.id` wildcard (don't ship the wildcard private key on a box that lives at Tim's; scope the blast radius). DNS-01 is offline-issuable (proves domain control via TXT; box need not be reachable).
- **Resolution at the venue:** authoritative **Slate dnsmasq** record `app.demo.trakrf.id → <box reserved IP>`. This resolves offline with no upstream — caching is the wrong tool for a name we own. An optional Cloudflare gray-cloud (DNS-only) A record to the private IP is convenient for build/test resolution but must NOT be the venue's only path (it reintroduces an internet dependency).
- **Termination:** Caddy quadlet → `backend:8080`. Caddy serves a **statically-provisioned** cert (`tls <cert> <key>` in the Caddyfile) — **not** runtime ACME. Reasons: (1) the stock `caddy:2` image has no DNS-provider plugin, so Caddy can't do Cloudflare DNS-01 itself without a custom build; (2) the box is offline at the venue, so runtime ACME renewal would fail. Issue/renew the cert **out-of-band during prep** (lego/certbot with the Cloudflare DNS-01 plugin, or reuse the infra cert-manager output), drop the PEMs, Caddy just serves them.
- **Demo-laptop checklist:** Chrome/Edge/Opera on a desktop OS (not iPad); **Chrome "Secure DNS" Off** so queries hit the Slate resolver rather than DoH-bypassing it.

### Cold-start & ordering
systemd owns lifecycle: `Restart=always`, boot-start via linger, ordering via `After=`/`Wants=` (Timescale + Mosquitto have no deps; migrate after TS-healthy; backend after migrate + mosquitto; caddy after backend). No kiosk health-gate splash is needed (no kiosk).

## Testing — the proof

A committed `smoke-test.sh` that, on the live box:
1. Seeds a **minimal fixture via the backend API** (not raw SQL): a demo org, one asset, an armed-EPC tag, a boundary `scan_point`, and a mock alarm device. Use the API so the geofence engine's **in-memory armed-EPC set refreshes** on the CRUD (TRA-901 — raw SQL inserts would leave the in-memory set stale and the alarm wouldn't fire).
2. `mosquitto_pub` a synthetic **CS463 payload** to the local broker on a `trakrf.id/...` topic. The CS463 shape (verified against live preview traffic, per `backend/internal/ingest/parser_cs463.go`):
   ```json
   {"tags":[{"epc":"<armed-epc>","timeStampOfRead":<µs>,"antennaPort":1,"capturePointName":"<boundary>","rssi":"-55"}]}
   ```
3. Asserts: a row lands in `asset_scans` for the armed EPC **and** the geofence engine fires the alarm dispatch (Shelly mocked — real HTTP wiring is the hardware step). This exercises ingest → resolve → geofence → fire on real hardware.

## Live bring-up sequence (this session)

1. `sudo apt install -y podman` (passwordless sudo confirmed available).
2. `loginctl enable-linger mike`; create `~/.config/containers/systemd/`.
3. Author `deploy/edge/` files; link/copy quadlets into the systemd user dir.
4. `podman login ghcr.io` (GH_TOKEN); pull `:preview`; resolve digest; pin.
5. Generate `passwd` (`mosquitto_passwd`) + `.env` (JWT via `openssl rand`).
6. `systemctl --user daemon-reload`; start `timescaledb` (await healthy) → `mosquitto` → `migrate` (oneshot) → `backend`; `curl localhost:8080/health`.
7. Run `smoke-test.sh`; confirm the chain green.
8. Caddy/LE edge: issue the `app.demo.trakrf.id` cert **out-of-band** via Cloudflare DNS-01 (needs a CF API token — see Prerequisites), drop the PEMs where Caddy reads them, start `caddy`, verify `https://app.demo.trakrf.id` from a LAN client.

## Open prerequisites

- **Cloudflare API token** for the DNS-01 cert issuance (step 8). The core stack + proof (steps 1–7) need nothing external beyond the GHCR pull. If the token isn't in `.env.local`/infra, Caddy can come up on its internal CA to validate proxy wiring, with the real LE cert swapped in during prep.

## Decisions log (the why)

- **Podman quadlets** over Docker Compose and over k8s/Helm/Argo: at 3–5 containers, Compose orchestration adds no value and `podman-compose` lags the spec; quadlets give systemd-native lifecycle + `podman auto-update`, no daemon/shim. Argo is a non-starter offline. Prod parity is the identical GHCR images, not the orchestrator.
- **No Portainer:** every break-glass path reduces to Mike getting a shell over the tailnet; the runtime is optimized for that, not a GUI Tim won't open.
- **Mosquitto basic auth, not anonymous:** confidentiality is moot here (demo data, no TLS on the LAN listener), but auth cheaply blocks *accidental/casual* publishers from injecting spoofed reads that trip the alarm mid-demo. Same username `trakrf-mqtt` as prod for config parity, but a **fresh box-local password** (don't reuse the preview cred on a plaintext listener).
- **No TLS on the broker:** offline single-LAN, Slate is the perimeter (no WAN forward); also sidesteps the TRA-827 TLS-1.2/GL-S10 constraint.
- **TLS on the app is mandatory** (Web BLE secure context from the laptop); scoped LE cert + Slate authoritative DNS, per above.
- **Kiosk dropped** for conference-room demos (Tim's call); reopen for a booth.

## Repo home

`deploy/edge/` in the **app repo** (not the separate infra repo): convention here is build + run-composition lives with the app (Dockerfiles, `docker-compose.yaml`, `railway.json` are in-repo) while the infra repo is cloud-only (Terraform/Helm/Cloudflare). The quadlets are the appliance analog of the local `docker-compose.yaml` and change in lockstep with app composition. Refactor to a dedicated `trakrf-edge` repo only if this productizes into a fleet.
