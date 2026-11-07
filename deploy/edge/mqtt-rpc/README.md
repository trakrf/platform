# Deploying `mqtt-rpcd` to a CS463 reader

`mqtt-rpcd` is the on-reader MQTT-RPC control daemon (built from `/mqtt-rpc`). It
subscribes to `<base>/rpc`, drives the CS463's localhost HTTP/servlet config API,
and replies on the caller's `src` topic. See `/mqtt-rpc/README.md` for the
contract.

## Install (SSH, once per reader)
```sh
./install.sh root@<reader-ip>
# then set READER_API_USER / READER_API_PASS in /opt/trakrf/mqtt-rpcd.env
```
This cross-builds a static armv7 binary and installs:

| Path | What |
|---|---|
| `/opt/trakrf/versions/mqtt-rpcd-<version>` | one file per retained version |
| `/opt/trakrf/mqtt-rpcd` | symlink to the active version (systemd `ExecStart`) |
| `/opt/trakrf/mqtt-rpcd-guard` | rollback guard — **never updated over the air** |
| `/etc/systemd/system/mqtt-rpcd.service` | runs the guard as `ExecStartPre`, then the daemon |
| `/etc/systemd/system/mqtt-rpcd-guard.{service,timer}` | guard tick every 5 min |
| `/opt/trakrf/update/` | update state shared by the daemon and the guard |
| `/opt/trakrf/mqtt-rpcd.env` | credentials — seeded once, never touched by updates |

After this, every later update can arrive over RPC alone (`Daemon.Update`; see
`/mqtt-rpc/README.md` "Remote update"). A reader installed before versioned
binaries keeps its old binary as version `legacy`.

## Recovering from a bad update
1. **Automatic.** A build that crash-loops (more than 5 starts) or never commits
   (never answers an RPC through the broker within 15–20 min) is rolled back to
   the previous version by the guard.
2. **Over RPC.** A build that committed but misbehaves: send `Daemon.Update` with
   any other signed version — older versions are allowed.
3. **SSH, from a laptop on the reader's network.**
   ```sh
   ./rollback.sh root@<reader-ip>             # list versions, * = active
   ./rollback.sh root@<reader-ip> <version>   # switch and restart
   ```
   On the reader itself: `/opt/trakrf/mqtt-rpcd-guard list|rollback <version>`.
4. **Reinstall.** `./install.sh` again.

## Notes
- The daemon auto-discovers the broker from the reader's own CloudServer config
  (it reuses the same endpoint/credential the reader already publishes reads to),
  using a **distinct** MQTT client id (`<reader>-rpc`) so it never evicts the
  reader's reads connection.
- A CSL firmware reflash wipes `/opt` — re-run `install.sh` to redeploy.
- Each version is uploaded under its own file name and switched in by renaming
  the symlink, so neither `install.sh` nor an update writes over the running
  binary (`ETXTBSY`).
