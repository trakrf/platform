# Deploying `mqtt-rpcd` to a CS463 reader

`mqtt-rpcd` is the on-reader MQTT-RPC control daemon (built from `/mqtt-rpc`). It
subscribes to `{publish_topic}/command/power`, drives the CS463's localhost
HTTP/servlet config API, and replies on the caller's `src` topic. See
`/mqtt-rpc/README.md` for the contract.

## Install
```sh
./install.sh root@<reader-ip>
# then set READER_API_PASS in /opt/trakrf/mqtt-rpcd.env on the reader
```
This cross-builds a static armv7 binary, ships it to `/opt/trakrf/mqtt-rpcd`,
installs the systemd unit, and enables the service.

## Notes
- The daemon auto-discovers the broker from the reader's own CloudServer config
  (it reuses the same endpoint/credential the reader already publishes reads to),
  using a **distinct** MQTT client id (`<reader>-rpc`) so it never evicts the
  reader's reads connection.
- A CSL firmware reflash wipes `/opt` — re-run `install.sh` to redeploy.
- You cannot `scp` over a running binary (`ETXTBSY`); `install.sh` stops the
  service first.
