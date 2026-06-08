# deploy/edge — TrakRF demo-box runtime

Rootless Podman quadlets for the offline demo box (`trakrf-demo`). Hosts the
backend + Timescale + Mosquitto + a Traefik TLS edge, all systemd-managed.
Design spec: `docs/superpowers/specs/2026-06-07-deploy-edge-design.md`.

Tim drives the demo from his laptop at **`https://app.demo.trakrf.id`** over the
Slate WiFi. Break-glass = a shell over the tailnet (`systemctl --user`,
`journalctl --user -u <svc>`, `podman`).

## Layout

| Path | What |
|---|---|
| `quadlets/*.container`, `*.network` | the 5 services + user network (symlinked into `~/.config/containers/systemd/`) |
| `install.sh` | symlink quadlets + `systemctl --user daemon-reload` |
| `db-init.sh` | one-time DB bootstrap (trakrf schema, search_path, obfuscation key) |
| `mosquitto/mosquitto.conf` | broker config (plain `:1883`, basic auth) |
| `traefik/traefik.yaml`, `dynamic.yaml` | edge static + dynamic config |
| `smoke-test.sh` | broker→subscriber→ingest proof |
| `.env.example` | template; copy to `.env` (gitignored) and fill |

## First-time bring-up (fresh box)

```bash
# 1. Host prereqs (one time)
sudo apt-get install -y podman mosquitto-clients
loginctl enable-linger "$USER"
echo 'net.ipv4.ip_unprivileged_port_start=443' | sudo tee /etc/sysctl.d/99-trakrf-rootless-ports.conf
sudo sysctl --system            # lets rootless Traefik bind :443

# 2. Secrets -> .env
cp deploy/edge/.env.example deploy/edge/.env
PGPW=$(openssl rand -hex 16); MQPW=$(openssl rand -hex 12)
sed -i "s|POSTGRES_PASSWORD=CHANGEME|POSTGRES_PASSWORD=$PGPW|;s|postgres://postgres:CHANGEME@|postgres://postgres:$PGPW@|" deploy/edge/.env
sed -i "s|mqtt://trakrf-mqtt:CHANGEME@|mqtt://trakrf-mqtt:$MQPW@|" deploy/edge/.env
sed -i "s|JWT_SECRET=CHANGEME|JWT_SECRET=$(openssl rand -hex 32)|" deploy/edge/.env
sed -i "s|OBFUSCATION_KEY=CHANGEME|OBFUSCATION_KEY=$(openssl rand -hex 32)|" deploy/edge/.env
# broker passwd (hashed) — same value as MQTT_URL above
touch deploy/edge/mosquitto/passwd && chmod 600 deploy/edge/mosquitto/passwd
podman run --rm -v "$PWD/deploy/edge/mosquitto/passwd:/passwd:Z" \
  --entrypoint mosquitto_passwd docker.io/library/eclipse-mosquitto:2.0.21 -b /passwd trakrf-mqtt "$MQPW"

# 3. Install quadlets + start (Timescale must be up before db-init/migrate)
deploy/edge/install.sh
systemctl --user start timescaledb.service
deploy/edge/db-init.sh                 # schema + search_path + obfuscation key
systemctl --user start traefik.service # pulls up migrate -> backend via deps
systemctl --user enable --now podman-auto-update.timer

# 4. Verify
curl -fsS http://127.0.0.1:8080/health
deploy/edge/smoke-test.sh
```

On a box whose volume is already initialized, a reboot self-starts everything
(linger + `Restart=always` + `[Install] WantedBy=default.target`).

## TLS cert (`app.demo.trakrf.id`)

Issued out-of-band via Let's Encrypt **Cloudflare DNS-01** (the box is offline at
the venue, so no runtime ACME). Scoped name, **not** the `*.trakrf.id` wildcard.

```bash
export CLOUDFLARE_DNS_API_TOKEN=<cloudflare DNS-edit token for trakrf.id>
podman run --rm -e CLOUDFLARE_DNS_API_TOKEN -v "$PWD/deploy/edge/traefik/lego:/.lego:Z" \
  docker.io/goacme/lego:latest run --accept-tos --email admin@trakrf.id \
  --dns cloudflare --domains app.demo.trakrf.id
cp deploy/edge/traefik/lego/certificates/app.demo.trakrf.id.{crt,key} deploy/edge/traefik/certs/
chmod 600 deploy/edge/traefik/certs/app.demo.trakrf.id.key
systemctl --user restart traefik.service
```

LE certs last 90 days — re-run during pre-event prep (the box has uplink then).

## Updates

Tracks the floating `ghcr.io/trakrf/backend:preview` tag via `AutoUpdate=registry`
+ `podman-auto-update.timer`. Updates pull only when the box has uplink (prep /
between-demos on house WiFi) — **never during a demo** (box is offline). Migrate
runs before serve on every update (backend `Requires=migrate`). Stay hands-off on
`preview` during demo windows. *Next iteration:* a `demo` tag that defaults to
tracking `prod`, with manual `preview → demo` promotion.

## Network (Slate-side, separate from this box)

- DHCP reservation for the box (`192.168.8.10`); the Slate resolves
  **`app.demo.trakrf.id → 192.168.8.10`** (authoritative dnsmasq record) so the
  name works offline.

## Demo-laptop checklist

- **Chrome / Edge / Opera** on a desktop OS (Web Bluetooth is Chromium-only; not
  Firefox/Safari/iPad).
- **Chrome → Secure DNS: Off** so `app.demo.trakrf.id` resolves via the Slate.
- Browse to **`https://app.demo.trakrf.id`**.

## Known / follow-ups

- The simulated-MQTT smoke test proves broker→subscriber→ingest. Full
  `asset_scans` derivation + geofence **fire** need a registered
  scan_device/scan_point + output device — provisioned by **real CS463/Shelly
  onboarding** (or a demo-data fixture). Validate that path with hardware.
- gnome-kiosk deprioritized (laptop-driven demos); reopen for a trade-show booth.
- Prometheus/Grafana = TRA-908 fast-follow (+2 quadlets).
