# deploy/edge — TrakRF demo-box runtime

Rootless Podman quadlets for the offline demo box (`trakrf-demo`). Hosts the
backend + Timescale + Mosquitto + a Traefik TLS edge, all systemd-managed.
Design specs: `docs/superpowers/specs/2026-06-07-deploy-edge-design.md`,
`docs/superpowers/specs/2026-06-13-srv-trakrf-runtime-layout-design.md` (TRA-988).

Tim drives the demo from his laptop at **`https://app.demo.trakrf.id`** over the
Slate WiFi. Break-glass = a shell over the tailnet (`systemctl --user`,
`journalctl --user -u <svc>`, `podman`).

## Runtime layout — `/srv/trakrf` (TRA-988)

The running stack reads **only** from `/srv/trakrf`, never from this git working
tree. The repo is the source of truth; `install.sh` deploys it. This means a
branch switch / pull / `reset` on the checkout can never pull live config out
from under the services.

```
/srv/trakrf/
  quadlets/   *.container, trakrf.network   → symlinked into ~/.config/containers/systemd/
  config/     mosquitto/mosquitto.conf, traefik/{traefik,dynamic}.yaml
  scripts/    trakrf-backup.sh
  systemd/    trakrf-backup.{service,timer}  → symlinked into ~/.config/systemd/user/
  secrets/    .env  cloudflared.env  mosquitto/passwd  traefik/certs/   (chmod 600; hand-placed; never in git)
  backups/    trakrf-YYYYMMDD-HHMMSS.sql.gz  (daily pg_dump)
```

TimescaleDB data stays on the Podman named volume `timescale_data`.

## Repo layout (source of truth)

| Path | What |
|---|---|
| `quadlets/*.container`, `*.network` | the 5 services + user network (bind-mount from `/srv/trakrf`) |
| `config/mosquitto/mosquitto.conf` | broker config (plain `:1883`, basic auth) |
| `config/traefik/{traefik,dynamic}.yaml` | edge static + dynamic config |
| `scripts/trakrf-backup.sh` | `pg_dump` → `/srv/trakrf/backups` |
| `systemd/trakrf-backup.{service,timer}` | daily backup user timer |
| `install.sh` | deploy `config/`+`quadlets/`+`scripts/`+`systemd/` → `/srv/trakrf`, link + reload units, enable backup timer |
| `db-init.sh` | one-time DB bootstrap (trakrf schema, search_path, obfuscation key) |
| `smoke-test.sh` | broker→subscriber→ingest proof |
| `secrets/*.example` | templates; real secrets live only on the box under `/srv/trakrf/secrets/` |

## First-time bring-up (fresh box)

```bash
# 1. Host prereqs (one time)
sudo apt-get install -y podman mosquitto-clients rsync
loginctl enable-linger "$USER"
echo 'net.ipv4.ip_unprivileged_port_start=443' | sudo tee /etc/sysctl.d/99-trakrf-rootless-ports.conf
sudo sysctl --system            # lets rootless Traefik bind :443

# 2. Runtime root (one time)
sudo mkdir -p /srv/trakrf && sudo chown "$(id -un):$(id -gn)" /srv/trakrf
mkdir -p /srv/trakrf/secrets/mosquitto /srv/trakrf/secrets/traefik/certs

# 3. Secrets -> /srv/trakrf/secrets/.env  (runtime reads here)
cp deploy/edge/secrets/.env.example /srv/trakrf/secrets/.env
PGPW=$(openssl rand -hex 16); MQPW=$(openssl rand -hex 12)
sed -i "s|POSTGRES_PASSWORD=CHANGEME|POSTGRES_PASSWORD=$PGPW|;s|postgres://postgres:CHANGEME@|postgres://postgres:$PGPW@|" /srv/trakrf/secrets/.env
sed -i "s|mqtt://trakrf-mqtt:CHANGEME@|mqtt://trakrf-mqtt:$MQPW@|" /srv/trakrf/secrets/.env
sed -i "s|JWT_SECRET=CHANGEME|JWT_SECRET=$(openssl rand -hex 32)|" /srv/trakrf/secrets/.env
sed -i "s|OBFUSCATION_KEY=CHANGEME|OBFUSCATION_KEY=$(openssl rand -hex 32)|" /srv/trakrf/secrets/.env
# broker passwd (hashed) — same value as MQTT_URL above
touch /srv/trakrf/secrets/mosquitto/passwd
podman run --rm -v "/srv/trakrf/secrets/mosquitto/passwd:/passwd:Z" \
  --entrypoint mosquitto_passwd docker.io/library/eclipse-mosquitto:2.0.21 -b /passwd trakrf-mqtt "$MQPW"
chmod 600 /srv/trakrf/secrets/mosquitto/passwd
# rootless: hand the file to the container's mosquitto uid (1883) so the broker can read
# it at 0600 (mosquitto runs as 1883, not container-root). Re-run after any passwd change.
podman unshare chown 1883:1883 /srv/trakrf/secrets/mosquitto/passwd

# 4. Deploy + start (Timescale must be up before db-init/migrate)
deploy/edge/install.sh                 # config+quadlets+scripts+systemd -> /srv/trakrf; links units; enables backup timer
systemctl --user start timescaledb.service
deploy/edge/db-init.sh                 # schema + search_path + obfuscation key
systemctl --user start traefik.service # pulls up migrate -> backend via deps
systemctl --user enable --now podman-auto-update.timer

# 5. Verify
curl -fsS http://127.0.0.1:8080/health
deploy/edge/smoke-test.sh
```

On a box whose volume is already initialized, a reboot self-starts everything
(linger + `Restart=always` + `[Install] WantedBy=default.target`).

## Migrating an existing box (deploy/edge bind-mounts → /srv/trakrf)

Reversible, no hardware needed; the old `deploy/edge` working-tree config stays
in place as instant rollback until verified.

```bash
sudo mkdir -p /srv/trakrf && sudo chown "$(id -un):$(id -gn)" /srv/trakrf
# seed secrets from the live box, by hand (install.sh never touches secrets/)
mkdir -p /srv/trakrf/secrets/mosquitto /srv/trakrf/secrets/traefik
cp deploy/edge/.env             /srv/trakrf/secrets/.env
cp deploy/edge/cloudflared.env  /srv/trakrf/secrets/cloudflared.env
cp deploy/edge/mosquitto/passwd /srv/trakrf/secrets/mosquitto/passwd
cp -a deploy/edge/traefik/certs /srv/trakrf/secrets/traefik/certs
chmod -R go-rwx /srv/trakrf/secrets

deploy/edge/install.sh                                  # repoints quadlet symlinks at /srv/trakrf
systemctl --user start trakrf-backup.service            # smoke-test one dump
# restart one at a time, verifying each:
for u in timescaledb mosquitto backend traefik cloudflared; do
  systemctl --user restart "$u".service; sleep 5
  podman ps --format '{{.Names}} {{.Status}}' | grep "$u"
done
```

**Rollback** a single service to the old paths: point its symlink back and reload —
```bash
ln -sf "$PWD/deploy/edge/quadlets/<unit>.container" ~/.config/containers/systemd/<unit>.container
systemctl --user daemon-reload && systemctl --user restart <unit>.service
```

## Backups

`trakrf-backup.timer` runs `scripts/trakrf-backup.sh` daily (04:00, `Persistent=true`)
→ `pg_dump | gzip` to `/srv/trakrf/backups/trakrf-<UTC>.sql.gz`, keeping the last
14. Run on demand: `systemctl --user start trakrf-backup.service`. Restore:
`gunzip -c /srv/trakrf/backups/<file>.sql.gz | podman exec -i timescaledb psql -U postgres -d postgres`.

## TLS cert (`app.demo.trakrf.id`)

Issued out-of-band via Let's Encrypt **Cloudflare DNS-01** (the box is offline at
the venue, so no runtime ACME). Scoped name, **not** the `*.trakrf.id` wildcard.

```bash
export CLOUDFLARE_DNS_API_TOKEN=<cloudflare DNS-edit token for trakrf.id>
podman run --rm -e CLOUDFLARE_DNS_API_TOKEN -v "/srv/trakrf/secrets/traefik/lego:/.lego:Z" \
  docker.io/goacme/lego:latest run --accept-tos --email admin@trakrf.id \
  --dns cloudflare --domains app.demo.trakrf.id
cp /srv/trakrf/secrets/traefik/lego/certificates/app.demo.trakrf.id.{crt,key} /srv/trakrf/secrets/traefik/certs/
chmod 600 /srv/trakrf/secrets/traefik/certs/app.demo.trakrf.id.key
systemctl --user restart traefik.service
```

LE certs last 90 days — re-run during pre-event prep (the box has uplink then).

## Updates

Tracks the floating `ghcr.io/trakrf/backend:preview` tag via `AutoUpdate=registry`
+ `podman-auto-update.timer`. Updates pull only when the box has uplink (prep /
between-demos on house WiFi) — **never during a demo** (box is offline). Migrate
runs before serve on every update (backend `Requires=migrate`). Stay hands-off on
`preview` during demo windows. **Caution:** `preview` is a moving tag — changes
merged while the box is offline (e.g. shipping) land on next uplink. Pinning to a
stable tag is a tracked follow-up. *Next iteration:* a `demo` tag that defaults to
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

- Unclean-shutdown resilience (rootless port-forward wedge after a hard power
  loss) + clean-shutdown via power button — developed/plug-pull-tested on a home
  box, separate tickets.
- Pin off the floating `:preview` tag (see Updates) — separate ticket.
- gnome-kiosk deprioritized (laptop-driven demos); reopen for a trade-show booth.
- Prometheus/Grafana = TRA-908 fast-follow (+2 quadlets).
