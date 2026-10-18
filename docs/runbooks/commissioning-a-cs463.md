# Commissioning a CS463

Takes a factory-fresh CSL CS463 to a reader that publishes reads to a TrakRF
environment, with `mqtt-rpcd` running on it. Reads only: GPO output devices are
a separate step on top of this one.

Every step here was executed on a real reader. Where a step has a command, it is
the command that ran. The daemon's own contract and golden config live in
[`mqtt-rpc/README.md`](../../mqtt-rpc/README.md), and its installer in
[`deploy/edge/mqtt-rpc/README.md`](../../deploy/edge/mqtt-rpc/README.md). This
page links to them rather than restating them.

**The order matters.** Register the reader in the platform (§6) before you build
its CloudServer (§7), because registration decides the MQTT topic. Build the
CloudServer before you install the daemon (§8), because the daemon exits without
it.

---

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
  `<router>`.
- **An operator machine** with Go (the installer cross-builds the daemon),
  `curl`, `openssl`, `mosquitto-clients`, and optionally `sshpass` for the first
  password login.
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
- **A password manager entry** to receive two new passwords in §2.

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

The reader allows **one root session at a time**, and the web UI's session
counts against the same slot. Log out when you are done, or the daemon's
reconcile in §8 is refused (`-10`, already logged in from another address).

---

## 1. Baseline

With `READER_PASS=csl` (the factory default, about to change):

| Read | Command | What to check |
|---|---|---|
| Firmware | `getReaderVersion` | **`rfid_firmware="2.6.38"` and `web_application="1.3.23"`** — the build the daemon is validated on |
| MAC, IP | `getNetworkConfig` | Ethernet MAC carries the CSL OUI `00:05:7B`; `dhcpmode="0"` (static) |
| Clock | `getDateTime` | `UTCDateTime` is correct (see §3) |
| Hostname | `ssh … hostname` | `csl-` + the last four MAC octets |

**Do not flash firmware as part of commissioning.** A CSL reflash wipes `/opt`,
where the daemon lives, and the adapter works around specific `1.3.23`
behaviour: for example, `setOperProfile` cannot set antenna enablement, so the
daemon writes that through the web servlet. A reader on a different build is a
bench exercise first, not a field one.

**The serial number is not exposed over `/API`** on this build; read it off the
label if the record needs it.

**Choose the external key now:** `cs463-` + the last three MAC octets, lower
case (MAC `00:05:7B:xx:yy:zz` → `cs463-xxyyzz`). It becomes the scan device's
key, the MQTT client id, and part of the topic, and it is derivable from the
reader itself without a label.

---

## 2. Replace both default passwords

The web/API `root` and the SSH `root` both ship as `csl`. They are separate
accounts; change both. The device has no local firewall, so these passwords are
the whole of its access control.

Generate two strong passwords, **save them to the password manager first**, then
apply them:

```sh
# SSH root: chpasswd reads stdin, so the password never reaches argv
printf 'root:%s\n' "$NEW_SSH_PASS" | ssh <reader> chpasswd

# Web/API root (logged in with the old password)
printf 'url = "%s?session_id=%s&command=setUserPassword&username=root&password=%s"\n' \
    "$B" "$S" "$NEW_WEB_PASS" | curl -s --config -
```

Keep the generated passwords alphanumeric: the web one travels in a query
string.

**Then prove the old one is dead, on every interface.** An `OK:` from the set
command is not evidence that `csl` stopped working:

| Interface | `csl` must give | New password must give |
|---|---|---|
| `/API` login | `Error code="-10" msg="Error: Password not correct!"` | `OK: session_id=…` |
| Web form `POST /Login` | `200` (the login page again) | `302` → `/home.html` |
| SSH, password auth only | `Permission denied` | a shell |

For SSH, force password auth so a key doesn't mask the result:
`ssh -o PubkeyAuthentication=no -o PreferredAuthentications=password <reader>`.

The web password is also the daemon's `READER_API_PASS` (§8).

---

## 3. SSH access and the clock

**SSH.** Reach the reader through the router and give it an alias, so the
installer's `install.sh <alias>` works unchanged:

```
Host cs463-xxyyzz
    HostName 192.168.25.160
    User root
    ProxyJump <router>
```

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

---

## 4. Back up the factory config

Before deleting anything, take a copy you can diff against or restore from:

```sh
ssh cs463-xxyyzz 'tar -C /opt/EmbeddedGlassFish -czf - config cert' > cs463-xxyyzz-factory.tgz
```

---

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

---

## 6. Register the reader in the platform

In the customer's org, **Scan Devices → Create**:

| Field | Value |
|---|---|
| Name | the external key, `cs463-xxyyzz` |
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

---

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
| `client_id` | the external key, `cs463-xxyyzz` — unique per physical unit; the broker evicts a duplicate |
| `username` / `password` | the environment's broker credential |
| `enable_ssl` / `ssl_version` | `true` / `TLSv1.2` |
| `two_way_authentication` | `false` |
| `topic` | the registered `publish_topic`, verbatim |
| `clean_session` | `false` |
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

After a power cycle the stack can look healthy while `/API` answers every
request with an empty body (see "the post-power-cycle wedge" in
[`docs/cs463-reader-references.md`](../cs463-reader-references.md)). Wait for
content, not for systemd: a healthy API answers a bogus command with an error.

```sh
until ssh cs463-xxyyzz 'curl -s "http://localhost/API?command=bogus"' | grep -q '<Error'; do sleep 10; done
```

## 8. Install `mqtt-rpcd`

**Connect the antenna to ANT 1 first, finger-tight, before anything below.** The
daemon's first reconcile creates `TrakRF mqtt-rpc Profile` as antenna-1-only at
30 dBm and arms it immediately. Transmitting into an open or badly seated port
trips the reader's reflected-power guard (§9) and halts inventory.

**Seed the env file before installing.** `install.sh` creates
`/opt/trakrf/mqtt-rpcd.env` from the example only if it is absent, then starts
the daemon at once. Writing it first means the first start already has the real
password:

```sh
printf 'READER_API_PASS=%s\n' "$NEW_WEB_PASS" \
  | ssh cs463-xxyyzz 'umask 077; mkdir -p /opt/trakrf; cat > /opt/trakrf/mqtt-rpcd.env'
```

Nothing else goes in it: the event self-bootstraps, and §7's exact name is the default.

Then, from an up-to-date `main` on the operator machine:

```sh
sh deploy/edge/mqtt-rpc/install.sh cs463-xxyyzz    # not executable: run it with sh
```

It ends with `mqtt-rpcd active (NRestarts=0)`, or fails with the journal if the
daemon could not stay up.

**Never run `/opt/trakrf/mqtt-rpcd --version`** (or any flag). The binary takes
no arguments and starts a second daemon, which fights the real one for its MQTT
client id. Read the version from the journal instead: every line carries
`"version":"…"`.

### Expect a slow first connection after a boot

`golden-config reconcile complete` and `readerd connecting` appear within
seconds, but the broker connection can take **minutes** after a reboot: the
kernel RNG isn't initialised, the TLS handshake blocks on it (`crypto/rand:
blocked for 60 seconds…` in the journal), and the firmware's MQTT client waits
the same way. Check the RNG rather than guessing:

```sh
ssh cs463-xxyyzz 'dmesg | grep "crng init done"; cat /proc/sys/kernel/random/entropy_avail'
```

Measured: about 96 s on one reader, and 6¾ minutes on an idle one fresh off a
reboot (`crng init done` at 388 s). It connects without intervention once the
RNG is up, with `connected; rpc subscribe…` in the journal and a retained
`{"online":true}` on `<base>/status`. The image has no `ss`; use
`netstat -tn | grep 8883`.

## 9. Verify

Green is not clean: each check below names the evidence to read.

### The reader is actually reading

Read the reader's own log. `systemctl`, the daemon's journal and
`Reader.GetStatus` (`"reading": true`) all looked healthy on a reader whose
inventory was halted:

```sh
ssh cs463-xxyyzz 'cd /opt/EmbeddedGlassFish/logs
  grep -a "MAC Error" CS463System.log | tail -3
  grep -a "DataRate" CS463System.log | tail -5'
```

- **`DataRate` is non-zero** with tags in the field: about 10/s with two tags on
  the antenna.
- **No new `MAC Error : 0x0309 port N, reflected power …`.** That is the
  reflected-power guard: the reader sent power into port N and most of it came
  back. **It halts the whole inventory, every port, until the port is disabled
  and the event re-armed.** The cause is physical: nothing on the port, a
  loose connector, or a bad cable or antenna. If two antennas trip on the same
  cable, suspect the cable. A reading just over the threshold (241 against 240)
  is a marginal match that a nudge tips over: 30 dBm is the daemon's default,
  not a requirement, so lower it with `Reader.SetOperProfile`, since reflected
  power falls with it. Then `systemctl restart mqtt-rpcd` to re-arm. A reboot
  changes nothing, and costs the slow reconnect in §8.

The vendor web UI shows the same thing live: **Events → Tag Inventory → Capture
Tags Raw**, with an `Ant #` column. Its stream (`/TagInventory`) is empty
while the inventory is halted.

### The port is the one you think it is

Check the antenna port in the published reads, not the label on the case. Every
tag in a read message carries `antennaPort`. With known tags on the antenna,
all of their reads should report the port you registered as the scan point.

### Reads reach the platform

Subscribe to the topic with the environment's credential, passed in a mosquitto
client config file (`$XDG_CONFIG_HOME/mosquitto_sub`) rather than argv:

```sh
mosquitto_sub -t '<topic>' -t '<topic>/#' -v
```

Read messages are `{"sequenceNumber":…,"numberOfTags":…,"tags":[{"epc","timeStampOfRead","antennaPort","rssi"}]}`,
with `timeStampOfRead` in epoch microseconds UTC. Check one: a 2019 date means
the clock (§3).

Then check that the backend ingested them, read-only:

```sql
SELECT message_topic, count(*), min(created_at), max(created_at)
  FROM trakrf.tag_scans
 WHERE created_at > now() - interval '30 minutes' GROUP BY 1;
```

The backend's `ingest_*` counters (`/metrics` on the backend pod) should move
together: `ingest_messages_total{result="received"}` and
`ingest_reads_parsed_total`. Tags that are not attached to an asset in that org
count as `ingest_reads_dropped_total{reason="no_asset"}` and produce no
`asset_scans`. That is correct behaviour, not a fault.

### RPC round-trip

Publish `{"id":1,"src":"<reply-topic>","method":"Reader.GetCapabilities"}` to
`<base>/rpc` and subscribe to `<reply-topic>`. The reply carries
`"reader_model":"csl_cs463","antennas":4`. `Reader.GetOperProfile` returns the
per-antenna enablement and power, and should show only the ports with something
connected. The scan device's **Antennas & Location** panel in the app uses the
same path; it shows "Reader did not respond" until this works.

### Daemon health

`systemctl show mqtt-rpcd -p NRestarts` is `0`. The journal has `golden-config
reconcile complete`, `connected; rpc subscribe…`, no `"level":"error"`, and no
`"level":"warn"` you can't account for (a network change, §11, leaves one).

## 10. Soak

Leave it reading overnight on the bench before it ships. Record at the start,
then compare in the morning:

| Measure | Start | Morning, pass if |
|---|---|---|
| `systemctl show mqtt-rpcd -p NRestarts` | `0` | still `0` |
| daemon `"level":"error"` / `"warn"` lines | count | no new ones |
| `MAC Error` lines in `CS463System.log` | count | no new ones |
| `DataRate` | ~10/s with test tags on the antenna | still non-zero |
| prod `tag_scans` for the topic | — | arriving every second, no gap of more than a few seconds |
| vendor web-app java RSS (`EmbeddedGlassFish.jar`, `VmRSS` in `/proc/<pid>/status`) | ~160 MB after boot | record it: the app freezes after long uptime |

## 11. Before it ships

### Switch Ethernet to DHCP

**Ship on DHCP, never on the factory static address.** `192.168.25.160` will
not exist on a customer network, and a reader that can't reach anything can
only be recovered by someone on site. DHCP at least has a chance of working out
of the box.

Ask the site's IT for, in order of preference: DHCP with a reservation for the
reader's MAC, plain DHCP, or (only if they insist) a static address with
gateway, DNS and optionally their NTP server. Either way, outbound **TCP 8883**
(broker) and **UDP 123** (NTP) must be allowed.

Switch while the reader is still on your bench router, so you can watch it move:

1. On the bench router, reserve an address for the reader's MAC, so you know
   where it will land (on OpenWrt: a `dhcp` `host` entry with `mac` and `ip`).
2. Run the switch **on the reader against `localhost`**, so the request does
   not depend on the address it is about to drop:

   ```sh
   ssh cs463-xxyyzz '… curl -s "http://localhost/API?session_id=$S&command=setNetworkConfig&type=ethernet&dhcpmode=1"'
   ```

   It takes effect immediately; no reboot. Your SSH session to the old address
   hangs, which is expected.
3. Find the lease on the router (`/tmp/dhcp.leases` on OpenWrt), and point the
   SSH alias and any tunnel at the new address.
4. Re-verify (§9). Both broker connections should be back from the new address
   (`netstat -tn | grep 8883` shows two); the daemon logs one `warn` for the
   socket that died with the old address and reconnects within seconds. Reads
   resumed after a 9-second gap when this was done.

### Final credential check

Re-run the table in §2: `csl` must be refused on the `/API`, the web form and
SSH, and the password manager must hold both passwords.
