# Deploying `mqtt-rpcd` to a CS463 reader

`mqtt-rpcd` is the on-reader MQTT-RPC control daemon (built from `/mqtt-rpc`). It
subscribes to `<base>/rpc`, drives the CS463's localhost HTTP/servlet config API,
and replies on the caller's `src` topic. See `/mqtt-rpc/README.md` for the
contract.

## Install (SSH, once per reader)
```sh
./install.sh root@<reader-ip>
# then set READER_API_USER / READER_API_PASS in /opt/trakrf/mqtt-rpcd.env
./set-recover-password.sh root@<reader-ip>   # the on-site operator's password
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
| `/opt/trakrf/recovery/auto-reboots.json` | the daemon's automatic-reboot count for a wedged vendor app (created on first use; see `/mqtt-rpc/README.md` "Reader-API health and wedge recovery") |
| `/opt/trakrf/mqtt-rpcd.env` | credentials — seeded once, never touched by updates |
| user `recover`, `/etc/sudoers.d/trakrf-recover`, a `Match User recover` block in `/etc/ssh/sshd_config` | the on-site operator's recovery login (below); starts locked |

After this, every later update can arrive over RPC alone (`Daemon.Update`; see
`/mqtt-rpc/README.md` "Remote update"). A reader installed before versioned
binaries keeps its old binary as version `legacy` until it ages out of retention
(newest 3 versions are kept); `legacy` has no updater, so rolling back to it ends
remote updates for that reader.

`install.sh` is safe to interrupt and re-run: an SSH drop at any point leaves a
runnable `/opt/trakrf/mqtt-rpcd`, and the rollback guard is stopped and disarmed
before the new binary moves in (`mqtt-rpcd-guard place`). That stops the daemon
too, since it runs the guard on every start; `install.sh` starts the daemon and
the guard timer again however it ends.

## The golden CloudServer
The one entity the daemon cannot create is `TrakRF mqtt-rpc MQTT Server`. It
holds the broker credential and a trust bundle, so it is set up by hand when the
reader is commissioned. The reader's own app publishes reads through it. A
wrong bundle or TLS version stops reads while the daemon stays connected and
everything looks healthy. A wrong `cleanSession` or `qos` is drift from the
validated config:

| Setting | Golden | Why |
|---|---|---|
| `serverCertFile` bundle | **trust anchors only**, a self-signed root **first** | CSL loads only the **first** certificate in the file. A bundle that leads with the broker's issuing intermediate works until the broker's certificate is renewed under a sibling intermediate, and then every handshake fails. Let's Encrypt rotates between its intermediates routinely. Lead with `ISRG Root X1`; any cross-signs after it are harmless. Never put a leaf or an intermediate first. |
| the bundle file itself | present | If the config names a file that has been deleted, the app publishes nothing **and logs nothing**. |
| `cleanSession` | `true` | |
| `qos` | `0` | Reads are fire-and-forget. A buffered backlog replayed late reads as presence that is no longer true. |
| `sslVersion` | `TLSv1.2` | It is the newest the CS463 offers, so it is a ceiling, not a choice. **Never make the broker require TLS 1.3** while CS463s are in the fleet: every reader drops at once, without a word. |

Check a bundle before uploading it. The first `subject` must equal its
`issuer`, and the broker's chain must verify against that first certificate
alone, the way CSL will use it:

```sh
openssl x509 -in bundle.pem -noout -subject -issuer -enddate
awk '/BEGIN CERT/{n++} n==1' bundle.pem > first.pem
openssl s_client -connect <broker>:8883 -servername <broker> -CAfile first.pem \
  -verify_return_error </dev/null | grep 'Verify return code'   # 0 (ok)
```

The daemon checks the same things. Every 30 s it reads the entry and the bundle.
On every broker handshake it also verifies the chain the broker presented
against the bundle's first certificate alone. That catches a broker-side change
the reader never sees, such as a renewal under another intermediate or a
cross-sign the broker stops sending. A fault is logged once and published as
`health.cloudserver_fault` in the retained `<base>/status`. The daemon reports
and never rewrites the entry. A firmware update that replaces
`/opt/EmbeddedGlassFish` can revert the entry, and the check sees that too.

If the bundle is missing or holds no certificate, the daemon trusts the
reader's system roots for its own broker connection instead of exiting. It
stays reachable only if those roots include the broker's root, which has not
been verified on every firmware image.

## Bringing a reader onto a release before it leaves (while SSH still works)
Per reader, from a machine on the reader's network with this repo, Go and
python3. The reader must already be commissioned: golden CloudServer, the
`trakrf` web user, and `READER_API_USER=trakrf` / `READER_API_PASS` in
`/opt/trakrf/mqtt-rpcd.env`.

1. **Link:** `cat /sys/class/net/eth0/speed` on the reader reads `1000`. Anything
   less is a bad cable pair, not a bad port.
2. **Install:** `./install.sh root@<reader>` from `main`. It installs the daemon,
   the rollback guard and the `recover` login, and ends with
   `rollback guard armed`.
3. **Recover password:** `./set-recover-password.sh root@<reader>`. Keep each
   reader's password with its other credentials; it is the one you give the
   site's IT.
4. **Release:** `./update-to-release.sh root@<reader> <version>` (default:
   the newest `mqtt-rpcd-*` release). It takes the reader off the local build
   from step 2 onto the exact CI-signed binary, over RPC, and waits for
   `OK: … committed`, about 4 minutes. This also proves the remote-update path
   on this reader: GitHub download, signature check, broker round trip.
5. **Operator path, once per site network:** `ssh recover@<reader>` with that
   password. It should report the release as already intact, then reboot the
   reader cleanly. Wait for it to come back (2–7 min) and check
   `/opt/trakrf/update/recover.log`.
6. **Before shipping:** `systemctl poweroff`, never a power pull.

**Network:** remote updates and `recover` need outbound HTTPS (443) to
`github.com`, `api.github.com` and `release-assets.githubusercontent.com`, as well
as the broker's port (8883). A site whose firewall allows destinations by name
must allow those three names too.

A release goes to readers only after it has updated a bench reader to a newer
build ("Release rule" in `/mqtt-rpc/README.md`).

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
4. **On-site operator, no developer needed.** From any computer on the reader's
   network:
   ```sh
   ssh recover@<reader-ip>
   ```
   and enter that reader's recover password. The login runs one fixed command and
   then logs out; there is no shell. It downloads the newest `mqtt-rpcd-*` release
   from GitHub, checks it against its manifest's sha256, switches to it, and then
   reboots the reader cleanly. The reboot happens even if nothing could be
   downloaded, because a clean reboot also clears a wedged vendor app. The reader is
   back in about 5 minutes. Each step is printed for the operator to read back, and
   kept in `/opt/trakrf/update/recover.log` (the last run; the CS463 journal does not
   survive the reboot).
   - It checks the hash but not the signature: the reader's OpenSSL is too old for
     Ed25519. The URL is fixed to this repo's GitHub releases and never comes from
     the broker.
   - When the newest release is itself the problem, publish a fixed release first,
     then ask the operator to run it.
   - Set or change the password with `./set-recover-password.sh root@<reader-ip>`.
     It reads the password from the terminal, or from stdin.
5. **Reinstall.** `./install.sh` again.

## Notes
- The daemon auto-discovers the broker from the reader's own CloudServer config
  (it reuses the same endpoint/credential the reader already publishes reads to),
  using a **distinct** MQTT client id (`<reader>-rpc`) so it never evicts the
  reader's reads connection.
- A CSL firmware reflash wipes `/opt` — re-run `install.sh` to redeploy.
- Each version is uploaded under its own file name and switched in by renaming
  the symlink, so neither `install.sh` nor an update writes over the running
  binary (`ETXTBSY`).
