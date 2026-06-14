# readerd — on-reader control daemon deploy

`readerd` is the TrakRF reader-control daemon. It runs **on the CS463 reader**
itself, subscribes to the reader's MQTT JSON-RPC control topic, and translates
the neutral `readerrpc` contract into the reader's localhost HTTP/servlet API
(tx-power get/set, status, capabilities). It ships as the `readerd` subcommand
of the same single `server` binary used by the backend.

## 1. Cross-build the binary

The CS463 is a 32-bit ARMv7 box. Build a fully static binary from `backend/`:

```sh
cd backend
GOOS=linux GOARCH=arm GOARM=7 CGO_ENABLED=0 go build -o ./server .
file ./server   # -> ELF 32-bit LSB executable, ARM, EABI5 ... statically linked
```

`CGO_ENABLED=0` is what makes it static — no glibc on the reader to depend on.

## 2. Install onto a reader

```sh
cd deploy/edge/readerd
./install.sh root@<reader-ip>
```

`install.sh`:

1. creates `/opt/trakrf` on the reader,
2. copies the cross-built binary to `/opt/trakrf/server`,
3. installs `readerd.service` to `/etc/systemd/system/`,
4. seeds `/opt/trakrf/readerd.env` from the example **only if absent** (so a
   redeploy never clobbers a configured password),
5. runs `systemctl daemon-reload && systemctl enable --now readerd`.

It uses your existing ssh setup (key auth recommended via `ssh-copy-id`). If the
reader only accepts a password, wrap the call with `sshpass` — see the comment
at the top of `install.sh`. Override the binary path with `SERVER_BIN=...`.

After install, set the reader API password:

```sh
ssh root@<reader-ip> 'vi /opt/trakrf/readerd.env'   # set READER_API_PASS
ssh root@<reader-ip> 'systemctl restart readerd'
ssh root@<reader-ip> 'systemctl status readerd'
```

## 3. Configuration

The only **required** setting is `READER_API_PASS` (the reader's servlet API
password). Everything else auto-discovers.

### Broker auto-discovery

On startup the daemon reads the reader's own EmbeddedGlassFish config:

- `…/config/CloudServer` — it selects the MQTT entry named `TrakRF MQTT`
  (override with `READERD_CLOUDSERVER_ID`) and derives the broker URL, CA cert
  path, base topic, and RPC client id from it. This means the daemon shares one
  source of truth with the CloudServer connection the reader already uses to
  publish reads — no duplicate broker config to keep in sync.
- `…/config/EventListCS463` — it picks the single enabled event as the
  inventory event to re-arm after a power change (override with
  `READERD_EVENT_ID`).

To bypass auto-discovery entirely, set `READERD_BROKER_URL` (plus the matching
`READERD_BASE_TOPIC` / `READERD_CA_CERT` / `READERD_RPC_CLIENT_ID` overrides).

See `readerd.env.example` for the full list, including `READER_API_URL`,
`READER_API_USER`, and `READERD_ANTENNA_COUNT`.

## 4. Redeploying after a firmware reflash

A CSL firmware reflash **wipes `/opt`**, including `/opt/trakrf/server` and
`/opt/trakrf/readerd.env`. After reflashing a reader, just re-run
`./install.sh <host>` (and re-set `READER_API_PASS`, since the env file is gone)
to redeploy.
