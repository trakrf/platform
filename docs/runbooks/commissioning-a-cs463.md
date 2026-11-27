# Commissioning a CS463

Takes a factory-fresh CSL CS463 to a reader that publishes reads to a TrakRF
environment, with `mqtt-rpcd` running on it. Reads only: an alarm on a GPO port
is a [separate step](cs463-alarm-output.md) on top of this one.

Every step here was executed on a real reader. Where a step has a command, it is
the command that ran. The daemon's own contract and golden config live in
[`mqtt-rpc/README.md`](../../mqtt-rpc/README.md), and its installer in
[`deploy/edge/mqtt-rpc/README.md`](../../deploy/edge/mqtt-rpc/README.md). This
page links to them rather than restating them.

**The order matters.** Move the reader to the address it ships with right
after SSH and the clock (§3), so later checks run on that address. Register it
in the platform (§6) before building its CloudServer (§7), because registration
decides the topic; build the CloudServer before installing the daemon (§8),
which exits without it.

**Plan on about 10 minutes from power-on to reads**, every boot. The reader is
ready when its retained status reads `api=healthy golden=applied` (§8), not
when a timer runs out.

## 0. Before you start

**The environment must already ingest MQTT.** Confirm the backend subscriber is
connected (`just ops logs <env>` shows `MQTT subscriber started`). A reader
publishing into an environment that isn't subscribed looks exactly like a broken
reader, and you will debug the wrong end.

You need:

- **A LAN with a `192.168.25.0/24` leg.** CSL readers ship with a static
  `192.168.25.160/24`, gateway `192.168.25.1`, WiFi off (user manual §5.5). Give
  the router a second address on that subnet and the reader is reachable without
  renumbering anything. This page assumes you reach that router over SSH as
  `<router>`. **Power up one factory-fresh reader per segment at a time:** they
  all claim the same address until §3 moves them.
- **An operator machine** with Go (the installer cross-builds the daemon),
  `python3`, `curl`, `openssl`, `mosquitto-clients`, and optionally `sshpass`
  for the first password login.
- **The broker CA bundle the reader will trust.** The reader verifies the broker
  against the PEM you upload in §7; it does not use a system store. Check the
  bundle against the live broker before you upload it:

  ```sh
  echo | openssl s_client -connect <broker-host>:8883 -servername <broker-host> \
      -CAfile ca.pem 2>/dev/null | grep 'Verify return'
  # Verify return code: 0 (ok)
  ```

- **The environment's broker credential** (`username` / `password` from the
  `trakrf-mosquitto-auth` secret in the environment's namespace). It is a shared
  credential; there is no per-reader one yet.
- **A password manager entry** to receive four new passwords (§2, §8).
- **The site's network details** (§3): DHCP or a static address, and its DNS
  servers.

### Talking to the `/API` without leaking a password

Every `/API` call is an HTTP GET with the password in the query string. Keep it
out of shell history and out of `ps` by feeding `curl` its URL on stdin:

```sh
B=http://<reader>/API          # through a tunnel: ssh -f -N -L 127.0.0.1:8160:192.168.25.160:80 <router>
login() {
  printf 'url = "%s?command=login&username=root&password=%s"\n' "$B" "$READER_PASS" \
    | curl -s --config - | grep -oE 'session_id=[^<]+' | cut -d= -f2
}
S=$(login)
curl -s "$B?session_id=$S&command=getReaderVersion"
curl -s "$B?session_id=$S&command=logout"
```

The reader allows **one session per user**, the web UI's included. Log out when
you are done, or your next `root` login is refused (`-10`, already logged in from
another address). The daemon has its own user (§8), so you never block it.

## 1. Baseline

With `READER_PASS=csl` (the factory default, about to change):

| Read | Command | What to check |
|---|---|---|
| Firmware | `getReaderVersion` | **`rfid_firmware="2.6.38"` and `web_application="1.3.23"`** — the build the daemon is validated on |
| MAC, IP | `getNetworkConfig` | Ethernet MAC carries the CSL OUI `00:05:7B`; `dhcpmode="0"` (static), and a stored DNS of `8.8.8.8` and `192.168.25.1` (§3) |
| Clock | `getDateTime` | `UTCDateTime` is correct (see §3) |
| Hostname | `ssh … hostname` | `csl-` + the last four MAC octets |
| Link | `ssh … cat /sys/class/net/eth0/speed` | **`1000`**, and no `Link is Down` in `journalctl -b -k` after boot. The PHY is gigabit, and a cable with one bad pair drops it to 10 Mbps after a few up/down cycles. Swap the cable before suspecting the reader: a bad cable looks exactly like a bad port |

**Do not flash firmware as part of commissioning.** A CSL reflash wipes `/opt`,
where the daemon lives, and the adapter works around specific `1.3.23`
behaviour: for example, `setOperProfile` cannot set antenna enablement, so the
daemon writes that through the web servlet. A reader on a different build is a
bench exercise first, not a field one.

**The serial number is not exposed over `/API`** on this build; read it off the
label if the record needs it.

**Choose the reader key now:** `cs463-` + the last three MAC octets, lower case
(`00:05:7B:xx:yy:zz` → `cs463-xxyyzz`). It becomes the reader ID, the MQTT client
id and the SSH alias, and is derivable from the reader without a label.

## 2. Replace both default passwords

The web/API `root` and the SSH `root` both ship as `csl`. They are separate
accounts; change both. The device has no local firewall, so these passwords are
the whole of its access control.

Generate three strong passwords (web root, SSH root, and the daemon's `trakrf`
user), **save them to the password manager first**, then apply them:

```sh
# SSH root: chpasswd reads stdin, so the password never reaches argv
printf 'root:%s\n' "$NEW_SSH_PASS" | ssh <reader> chpasswd

# Web/API root (logged in with the old password)
printf 'url = "%s?session_id=%s&command=setUserPassword&username=root&password=%s"\n' \
    "$B" "$S" "$NEW_WEB_PASS" | curl -s --config -
```

Keep the generated passwords alphanumeric: they travel in query strings.

**Then prove the old one is dead, on every interface.** An `OK:` from the set
command is not evidence that `csl` stopped working:

| Interface | `csl` must give | New password must give |
|---|---|---|
| `/API` login | `Error code="-10" msg="Error: Password not correct!"` | `OK: session_id=…` |
| Web form `POST /Login` | `200` (the login page again) | `302` → `/home.html` |
| SSH, password auth only | `Permission denied` | a shell |

For SSH, force password auth so a key doesn't mask the result:
`ssh -o PubkeyAuthentication=no -o PreferredAuthentications=password <reader>`.

**Then fix the users** ([CS463 user accounts](cs463-user-accounts.md)): delete
the factory `ExampleUser1PWDcsl` (password `csl`, which the root change leaves
working), add the daemon's `trakrf` user (§8), and an operator account if asked.

## 3. SSH, the clock and the address

**SSH.** Reach the reader through the router and give it an alias, so the
installer's `install.sh <alias>` works unchanged:

```
Host cs463-xxyyzz
    HostName 192.168.25.160
    User root
    ProxyJump <router>
    HostKeyAlias cs463-xxyyzz
```

`HostKeyAlias` keys the host key to the reader, not the address: every factory
reader is `192.168.25.160`, and the address changes below.

Install the operator's public key in `/home/root/.ssh/authorized_keys` (root's
home on this image is `/home/root`), then confirm `ssh -o BatchMode=yes
cs463-xxyyzz true` succeeds with no password.

**Reader ID.** Every unit ships as `CSLIntelligentReader`. Set it to the key
(`setReaderID&reader_id=cs463-xxyyzz&desc=`); the vendor UI shows it in the
header of every page, so a browser tab says which reader it is.

**Clock.** NTP is on from the factory (`time.nist.gov`, `pool.ntp.org`, UDP 123).
Check `UTCDateTime` from `getDateTime` anyway: TLS to the broker fails on a
wrong clock, and a fresh boot reports 2019 until NTP answers.

**Leave the timezone at UTC** (`setTimeZone&time_zone=00:00&dst=0`; it ships as
`GMT+08:00`). It only changes the vendor UI's display: the OS clock is UTC and
read timestamps are epoch-microseconds UTC, and localisation belongs in the
platform. This build's `/API` ignores the `dst` flag anyway.

### Move it to the address it ships with

**Never ship on the factory static address.** `192.168.25.160` will not exist
on a customer network, and a reader that can't reach anything can only be
recovered on site.

Ask the site's IT for the address plan (DHCP with a reservation for the
reader's MAC, plain DHCP, or a static address with mask and gateway) **and for
its DNS servers**. Outbound **TCP 8883** (broker) and **UDP 123** (NTP) must be
allowed, and **TCP 443** to GitHub for remote updates (the names are in the
[installer README](../../deploy/edge/mqtt-rpc/README.md)). If it goes silent on
site, send them the [network check](cs463-network-check.md).

**The factory DNS survives a switch to DHCP.** Every factory reader stores
`8.8.8.8` and `192.168.25.1` as its DNS servers. `dhcpmode=1` changes only the
mode flag: the stored servers stay configured in connman and override whatever
DHCP supplies. A site that allows DNS only to its own resolvers then gets a
reader with a good address that resolves nothing, so the broker name fails.
**Check the nameservers after every address change** (step 4).

Run the change **on the reader against `localhost`**, so the request does not
depend on the address it is about to drop. It takes effect immediately, with no
reboot, and your SSH session to the old address hangs, which is expected.

1. Make the new address reachable from the bench. For DHCP, reserve an address
   for the reader's MAC on the bench router (on OpenWrt: a `dhcp` `host` entry
   with `mac` and `ip`). For a site static address, give the bench router a
   further address on the site's subnet, the way it has one on
   `192.168.25.0/24`.
2. Send one `setNetworkConfig`. **For a static address, set the address and
   the DNS together in the same call**; it writes both connman and the vendor
   config, and the result survived a reboot on a bench reader:

   ```sh
   # static address with the site's DNS
   ssh cs463-xxyyzz '… curl -s "http://localhost/API?session_id=$S&command=setNetworkConfig&type=ethernet&dhcpmode=0&ip=<ip>&mask=<mask>&gateway=<gateway>&dns_server1=<dns1>&dns_server2=<dns2>"'
   # DHCP
   ssh cs463-xxyyzz '… curl -s "http://localhost/API?session_id=$S&command=setNetworkConfig&type=ethernet&dhcpmode=1"'
   ```

3. Point the SSH alias and any tunnel at the new address (for DHCP, the lease
   is in `/tmp/dhcp.leases` on OpenWrt) and confirm `ssh cs463-xxyyzz true`.
4. Read back what the reader now uses, and resolve the broker with it:

   ```sh
   ssh cs463-xxyyzz 'connmanctl services ethernet_00057b<mac6>_cable | grep -E "IPv4 =|Nameservers"
     nslookup <broker-host> | tail -1'
   ```

   `<mac6>` is the last three MAC octets, lower case. On a DHCP reader the
   nameservers are still the factory pair; that works only where the site lets
   `8.8.8.8` through.
5. Re-run the link check (§1). Switching later, under a running daemon, costs
   one `warn` and a reconnect.

## 4. Back up the factory config

Before deleting anything, take a copy you can diff against or restore from:

```sh
ssh cs463-xxyyzz 'tar -C /opt/EmbeddedGlassFish -czf - config cert' > cs463-xxyyzz-factory.tgz
```

## 5. Remove every publishing entity

The factory image carries demo CloudServers (HTTP, TCP, and an enabled MQTT
entry with a random client id) and the actions, heartbeats and notifications
that point at them. Left in place they collide with the golden config; a
leftover MQTT entry caused client-id collisions on an earlier reader.

**Only delete things that publish.** GPIO, display and USB examples publish
nothing; leave them alone, as the daemon does.

The reader refuses to delete anything still referenced, so map the references
first (`listServer`, `listEvent`, `listResultantAction`, `listHeartBeat`,
`listPowerUpNotification`, `listReaderErrorNotification`), then delete from the
leaves inward. The ids contain spaces and parentheses, so URL-encode them:

```sh
del() { curl -s -G "$B" --data-urlencode "session_id=$S" \
          --data-urlencode "command=$1" --data-urlencode "$2=$3"; echo; }
```

| Order | What | Command | Id parameter |
|---|---|---|---|
| 1 | Events whose `resultant_action` sends to a server | `delEvent` | `event_id` |
| 2 | Actions with a non-empty `server_id` | `delResultantAction` | `action_id` |
| 3 | Heartbeats with a non-empty `server_id` | `delHeartBeat` | `heart_beat_id` |
| 4 | Power-up notifications | `delPowerUpNotification` | `notification_id` |
| 5 | Reader-error notifications | `delReaderErrorNotification` | `notification_id` |
| 6 | Every CloudServer | `delServerID` | `server_id` |

On the factory image that came to four events, five actions, one heartbeat, one
power-up and one reader-error notification, and four servers. Heartbeats with
no server (the ARP and ping ones) stay.

**Done means:** `listServer` returns no `<Server>`; no action, heartbeat or
notification has a non-empty `server_id`; and
`/opt/EmbeddedGlassFish/cert/CloudServer/` is empty.

## 6. Register the reader in the platform

In the customer's org, **Scan Devices → Create**:

| Field | Value |
|---|---|
| Name | a label; the place reads best, e.g. `cs463-dock-door-3`. The topic is the unique key |
| Type | CSL CS463 |
| Transport | MQTT |
| Publish Topic | `<org_slug>/<place>` — the form pre-fills `<org_slug>/` and refuses anything else |
| Model / Description | `CS463`, and the MAC and hostname, so the record names the physical unit |

**The topic names the place, not the box.** Name it after where the reader is
mounted (`<org_slug>/dock-door-3`). A replacement reader then takes over the
same topic with nothing re-registered, and the device record carries which unit
is there now.

A trailing `/reads` is optional: the daemon and the backend both trim it to get
the base topic, so RPC is `<base>/rpc` and status `<base>/status` either way.
The backend subscribes to the exact topic, so those are never taken for reads.

Then add the reader's **location** and one **scan point per connected antenna**
(`antenna_port` = the firmware port), **with the scan point's location set**. A
scan point without a location still produces raw scans, but nothing downstream
can say where a read happened.

**Check it from the database rather than the UI** (read-only):

```sql
SELECT id, name, type, transport, publish_topic, is_active
  FROM trakrf.scan_devices WHERE org_id = <org_id>;
SELECT name, antenna_port, location_id, is_active
  FROM trakrf.scan_points WHERE scan_device_id = <scan_device_id>;
```

The backend subscribes as soon as the device is saved; no restart. Its log shows
`subscribed … topic=<topic>` followed by `topic registry reconciled added=1`.

## 7. The golden CloudServer

The daemon finds its broker by reading this entry, and the golden event
publishes through it by name. It is the one golden entity the daemon does not
create, because it carries the broker credential.

**The name must be exactly `TrakRF mqtt-rpc MQTT Server`.** The reader cannot
rename an entry, only delete and re-add it, and a whitespace typo here has cost
a reader before.

`setServerID`, sent with `-G --data-urlencode` per parameter (and the credential
via `curl --config -`, as in §0):

| Parameter | Value |
|---|---|
| `server_id` | `TrakRF mqtt-rpc MQTT Server` |
| `desc` | `Managed by TrakRF mqtt-rpc daemon` |
| `type` | `MQTT` |
| `server_ip` / `server_port` | the broker host / `8883` |
| `client_id` | the reader key, `cs463-xxyyzz` — unique per physical unit; the broker evicts a duplicate |
| `username` / `password` | the environment's broker credential |
| `enable_ssl` / `ssl_version` | `true` / `TLSv1.2` |
| `two_way_authentication` | `false` |
| `topic` | the registered `publish_topic`, verbatim |
| `clean_session` | `true` — reads are QoS 0 and fire-and-forget; nothing should be held for a reader across a disconnect |
| `qos` | `0` |

Then upload the CA bundle with `setServerCertificate`. The PEM is the **POST
body**; the parameters go in the query string. Do not use `curl -G` here: it
moves the body into the query string as well, and no `<Ack>` comes back.

```sh
U="$B?session_id=$S&command=setServerCertificate&server_id=TrakRF%20mqtt-rpc%20MQTT%20Server&serverCertFile=le-fullchain-plus-roots.pem"
curl -s -X POST --data-binary @ca.pem -H 'Content-Type: application/octet-stream' "$U"
```

**Verify what landed, not what was acknowledged:**

- `listServer` shows exactly one `<Server>`, with the fields above and
  `server_certificate="le-fullchain-plus-roots.pem"`.
- The PEM under `/opt/EmbeddedGlassFish/cert/CloudServer/TrakRF mqtt-rpc MQTT Server/`
  has the same `sha256sum` as your local bundle.
- The credential in `/opt/EmbeddedGlassFish/config/CloudServer` matches the
  secret (`grep -qF` in a script; never print it).

**Then reboot** (`ssh cs463-xxyyzz systemctl reboot`). A CloudServer that has
never connected is not wired up by a live toggle; only a reboot did it.

**Expect the vendor app to wedge on some boots.** The stack looks healthy
(systemd `active`, `GET /` 200) while `/API` answers every request with HTTP
200 and an empty body. Clean reboots cause it as well as power cuts, and in one
batch every reader powered up in a day did it on at least one boot. Wait for
content, not for systemd: a healthy API answers a bogus command with an error.

```sh
until ssh cs463-xxyyzz 'curl -s "http://localhost/API?command=bogus"' | grep -q '<Error'; do sleep 10; done
```

A boot takes about 3 minutes to serve. Once the daemon is installed (§8) it
clears a wedge by itself. Here, before it is, do it by hand if the body is
still empty after about 5 minutes: `systemctl restart embeddedglassfish`, then
wait about 2 more ("the post-power-cycle wedge" in
[`docs/cs463-reader-references.md`](../cs463-reader-references.md)).

## 8. Install `mqtt-rpcd`

**Connect the antenna to ANT 1 first, finger-tight, before anything below.** The
daemon's first reconcile creates `TrakRF mqtt-rpc Profile` as antenna-1-only at
30 dBm and arms it immediately. Transmitting into an open or badly seated port
trips the reader's reflected-power guard (§9) and halts inventory.

**Seed the env file before installing.** `install.sh` creates
`/opt/trakrf/mqtt-rpcd.env` from the example only if it is absent, then starts
the daemon at once, so the first start already uses the `trakrf` user from §2
(without `READER_API_USER` it logs in as `root`):

```sh
printf 'READER_API_USER=trakrf\nREADER_API_PASS=%s\n' "$TRAKRF_PASS" \
  | ssh cs463-xxyyzz 'umask 077; mkdir -p /opt/trakrf; cat > /opt/trakrf/mqtt-rpcd.env'
```

Nothing else goes in it: the event self-bootstraps, and §7's exact name is the default.

Then, from an up-to-date `main` on the operator machine, follow "Bringing a
reader onto a release before it leaves" in the
[installer README](../../deploy/edge/mqtt-rpc/README.md):

```sh
cd deploy/edge/mqtt-rpc
./install.sh cs463-xxyyzz               # ends: mqtt-rpcd <version> active (NRestarts=0); rollback guard armed
./set-recover-password.sh cs463-xxyyzz  # the on-site recovery login; save the password first
./update-to-release.sh cs463-xxyyzz     # ends: OK: <base> runs <version>, committed.  About 4 minutes
```

`install.sh` fails with the journal if the daemon could not stay up. The last
step replaces the local build with the newest signed release over the broker,
which also proves remote update works for this reader. **Do all three while SSH
still works:** after it ships, the broker and the `recover` login are the only
ways in.

**Never run `/opt/trakrf/mqtt-rpcd --version`** (or any flag). It takes no
arguments and starts a second daemon that fights the real one for its MQTT client
id. Read the version from the journal: every line carries `"version":"…"`.

### Ready means `api=healthy golden=applied`

The daemon publishes a retained status document on `<base>/status`. Read it
with `mqtt-rpcctl`, built from this repo, with the broker URL in the
environment (never in argv):

```sh
(cd mqtt-rpc && go build -o ../mqtt-rpcctl ./cmd/mqtt-rpcctl)
export MQTT_URL    # mqtts://<user>:<password>@<broker-host>:8883, read from a file
./mqtt-rpcctl status -base <base> -watch
# 18:25:10 online <version> api=healthy golden=applied
```

`-watch` prints one line per change. The reader is ready on
`online … api=healthy golden=applied`, with no `cloudserver_fault`.

**A wedged boot recovers without you.** After 6 minutes of an unhealthy `/API`
the line shows `api=degraded WEDGED recovery=app_restart`: the daemon restarts
the vendor app, which took about 1.5 minutes each time it was seen, and re-runs
the reconcile so inventory re-arms. Only if that fails does it reboot the
reader cleanly, at most twice. `EXHAUSTED` means it has stopped trying and a
person decides. The ladder is in "Reader-API health and wedge recovery" in
[`mqtt-rpc/README.md`](../../mqtt-rpc/README.md).

The daemon itself is on the broker within about half a minute of power-on (18
to 27 s measured), because it credits the hardware RNG to the kernel at start.
A daemon older than `v1.6.0` waits minutes for the kernel RNG instead
(`crypto/rand: blocked for 60 seconds…` in the journal), and so does the
vendor app's own MQTT client, which carries the reads. The image has no `ss`;
use `netstat -tn | grep 8883` to see both connections.

### Control without SSH

The same tool reaches a reader that has no SSH, which is every reader on site:

| Command | Does |
|---|---|
| `mqtt-rpcctl call -base <base> -method Reader.GetCapabilities` | any RPC; the reply is printed |
| `mqtt-rpcctl update -base <base> -manifest <manifest.json URL>` | moves the daemon to a signed release; a bad build rolls itself back |
| `mqtt-rpcctl reboot -base <base> -yes` | `Reader.Reboot`: a clean reboot |
| `mqtt-rpcctl shutdown -base <base> -yes` | `Reader.Shutdown`: a clean power-off. It stays off until someone power-cycles it on site |

## 9. Verify

Work through [Verifying and soaking a CS463](cs463-verify-and-soak.md). Green is
not clean, so each check there names the evidence to read:

- **The reader is actually reading:** a non-zero `DataRate` and no new
  `MAC Error : 0x0309` (the reflected-power guard) in the reader's own log.
  systemd, the daemon's journal and `Reader.GetStatus` all looked healthy on a
  reader whose inventory was halted.
- **The port is the one you think it is:** `antennaPort` in the published reads.
- **Reads reach the platform:** on the broker, then in `tag_scans`.
- **RPC round-trip:** `Reader.GetCapabilities` and `Reader.GetOperProfile`.
- **Daemon health:** `NRestarts=0`, a clean journal, `api=healthy golden=applied`.

## 10. Soak

Leave it reading overnight on the bench before it ships, against the baseline
table on the [same page](cs463-verify-and-soak.md#soak).

## 11. Before it ships

- **Reboot it once more** (`systemctl reboot`) and wait for
  `api=healthy golden=applied` and reads in the platform. One healthy boot is
  not enough evidence, and this one runs on the address it ships with.
- Re-run the §1 link check (still `1000`, no new drops), the §3 nameserver
  check, and the §2 table (`csl` refused on the `/API`, the web form and SSH).
- `listUsers` shows only `root`, `trakrf` and any operator account, and the
  password manager holds every password, the `recover` one included.
- **Power it down cleanly, never by pulling power:** `systemctl poweroff`, or
  `mqtt-rpcctl shutdown` (§8). From `v1.6.0` the daemon drives every GPO off
  when it stops and again when it starts, so a reader with an
  [alarm output](cs463-alarm-output.md) can be powered off as it is. An older
  daemon leaves an output that was on latched on, because the halted board
  keeps its last GPO state while it has power: update it first (§8).
