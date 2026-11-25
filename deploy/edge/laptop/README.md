# deploy/edge/laptop — ThinkPad demo box host layer

One laptop is the whole demo kit: server, network and screen. The stack is
[`deploy/edge`](../README.md), unchanged. This directory adds what a laptop
needs around it. Tim's procedure is [`SETUP-CARD.md`](SETUP-CARD.md).

First unit: `trakrf-demo-1`, a ThinkPad T14s Gen 2a on Ubuntu 26.04. It serves
`app.demo-1.trakrf.id`; the EliteDesk kit keeps `app.demo.trakrf.id`, so the
two kits never claim one name.

## Shape

| Job | Hardware | Config |
|---|---|---|
| Uplink (venue, house, hotspot) | built-in `wlp3s0` (ath11k) | NM profiles `uplink-*` from `/etc/trakrf/uplinks.conf` |
| Demo LAN 192.168.8.0/24, box = `.10` | USB dongle (TP-Link Archer T3U, `rtw88_8822bu`) as AP + `enp2s0f0` | bridge `br-demo`, NM shared mode (DHCP .100–.199, DNS) |
| CS463 | Ethernet through an 802.3at PoE switch | on `br-demo`, so **Ethernet is never an uplink** |
| Remote access | Tailscale (`trakrf-demo-1`) | key-based OpenSSH over the tailnet; Tailscale SSH **off** |
| Screen | GDM autologin as `kiosk` | Chrome `--kiosk http://localhost:8080` |

The demo LAN copies the Slate it replaces (SSID `TrakRF`, same PSK, same
subnet and box address), so the CS463, Shelly and gateways that point at
`192.168.8.10:1883` need no reconfiguration.

**Tailscale outranks everything.** `setup-host.sh demo-lan` refuses to run
unless the Wi-Fi uplink already holds the default route, because it takes the
Ethernet port off uplink duty.

## Bring-up (fresh Ubuntu, as the stack user)

```bash
git clone https://github.com/trakrf/platform.git ~/platform && cd ~/platform

# 1. Uplinks first: one root-only line per network, never in git.
sudo install -d -m 700 /etc/trakrf
sudoedit /etc/trakrf/uplinks.conf   # name|ssid|psk|priority   (higher = preferred)

# 2. Host layer. Idempotent; any subset of steps can be named.
sudo deploy/edge/laptop/setup-host.sh
#   packages apparmor tailscale battery bios logind journald uplinks demo-lan kiosk

# 3. Tailscale login (interactive by design), then in the admin console:
#    disable key expiry for this machine.
sudo tailscale up --hostname=trakrf-demo-1     # no --ssh: see "Why" below

# 4. Stack: deploy/edge/README.md "First-time bring-up", steps 2-4, except:
systemctl --user mask cloudflared.service traefik.service   # see below
systemctl --user start mosquitto.service backend.service

# 5. Seed: the org plus its logins. Add people to /srv/trakrf/secrets/logins.conf
#    (email|name|role|superadmin); passwords land in logins.passwords.
deploy/edge/laptop/seed.sh

# 6. Verify, then reboot once (logind and BIOS settings apply at boot).
deploy/edge/smoke-test.sh
sudo reboot
```

### Why each piece is the way it is

- **cloudflared and Traefik are masked.** Tailscale is the remote path, and the
  kiosk browses `http://localhost:8080`. `localhost` is a secure context, so Web
  Bluetooth to the CS108 works without TLS. Unmask Traefik and issue an
  `app.demo-1.trakrf.id` cert (`deploy/edge/README.md` "TLS cert") only if a
  second laptop has to join.
- **`apparmor` step.** Ubuntu 26.04 ships a host profile for
  `/usr/sbin/mosquitto` that also confines the binary inside the rootless
  container, so the broker can't open `/mosquitto/config/mosquitto.conf`.
  `files/apparmor-local-mosquitto` grants the container paths through the
  profile's `local/` include. The EliteDesk ran 24.04 and never saw this.
- **Tailscale SSH is off.** With it on, tailscaled takes port 22 on the
  tailnet address, and the tailnet's `check` policy demands a browser approval
  per session, so nothing headless gets in. The EliteDesk hit exactly this.
  Off, Tailscale is only the network and key-based OpenSSH answers.
- **`seed.sh`.** `seed/demo-org.sql` creates the org (identifier = hostname,
  so `trakrf-demo-1`), Doorway 1, the CS463 and the alarm plug on their
  grandfathered `trakrf.id/…` topics (the API refuses those, so it is SQL), and
  turns on the entitlement. `seed/login.sql` runs once per line of the box-only
  `/srv/trakrf/secrets/logins.conf`: people's addresses stay out of this public
  repo. Each password is generated on first run into `logins.passwords`, which
  is where they're copied into the password manager from. Delete a line there
  and re-run to reset that password. Everything is keyed on natural keys (org
  identifier, `external_key`, topic, email), so re-runs are no-ops and the id
  triggers mint ids as usual.
- **Read history** is capped by the schema's own retention (`tag_scans` 7 days,
  `asset_scans` 365 days); the journal is capped at 500 MB.
- **Power.** Power key = clean poweroff. Lid on AC = nothing (the demo keeps
  running). Lid on battery = clean poweroff (being packed). BIOS
  `OnByAcAttach` boots the box when power is plugged in. The battery is held
  at 75–80 %.

## Break-glass

Over the tailnet: `ssh mike@trakrf-demo-1` (plain OpenSSH, key auth).

```bash
systemctl --user status backend mosquitto timescaledb
journalctl --user -u backend -f
nmcli con show --active                 # uplink-*, br-demo, br-demo-ap, br-demo-wired
cat /var/lib/NetworkManager/dnsmasq-br-demo.leases   # who joined the demo LAN
```

Pin a device to its old Slate address: add
`dhcp-host=<mac>,192.168.8.20,cs463` to
`/etc/NetworkManager/dnsmasq-shared.d/dhcp-hosts-trakrf.conf`, then
`sudo nmcli con up br-demo`.

At the screen: Alt+F4 closes Chrome, and it relaunches 10 s later unless
`/tmp/trakrf-kiosk.stop` exists. Ctrl+Alt+F3 gives a text console.

## Known gaps

- Broker credentials are this box's own. Devices that last talked to the
  EliteDesk's broker need its `trakrf-mqtt` password. The plan is to copy
  `/srv/trakrf/secrets/{.env,mosquitto/passwd}` across from the EliteDesk
  (then `podman unshare chown 1883:1883` the passwd file and restart
  mosquitto and the backend), rather than re-entering a new one on each device.
- Fixed readers being commissioned and validated offline (no network onsite)
  is a separate design, not this kit.
