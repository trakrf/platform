# CS463 network check

For a commissioned reader that works on one network and stays silent on
another. [`cs463-network-check.sh`](cs463-network-check.sh) runs on the reader,
changes nothing, and shows where the path to the broker stops. Anyone with the
reader's SSH or serial console can run it, so it can go to a site's IT.

## Run it

Get the script onto the reader in one of two ways:

- **Copy it over:** `scp cs463-network-check.sh root@<reader>:/tmp/`
- **Paste it into a session** (SSH or serial console), wrapped in a heredoc:
  `cat > /tmp/cs463-network-check.sh <<'EOF'`, then the script, then `EOF`.

Then:

```sh
tr -d '\r' < /tmp/cs463-network-check.sh | bash
```

`tr` strips any Windows line endings the file picked up in transit. It takes
about 8 s on a healthy network and up to about a minute when everything is
blocked, because each probe has its own timeout. To check against preview, set
`B=mqtt.preview.gke.trakrf.id` before `bash`.

It uses only what the stock firmware ships: `nslookup`, bash's `/dev/tcp`,
`openssl s_client`, `curl`, `ntpdate`, `connmanctl`. There is no `dig`, `ss` or
`nc -z`.

## Read it

| Output | Means | Fix |
|---|---|---|
| §3: the `127.0.0.1` lookup fails or gives another address, `8.8.8.8` is right | the site's DNS blocks or rewrites the name; or connman's DNS proxy is stuck (it has answered `10.0.0.1` for every name) | site allowlists the domain; else `systemctl restart connman` |
| §3: `127.0.0.1` right, `8.8.8.8` times out | normal for a site that forces its own DNS | nothing |
| §3: both lookups fail | the site's resolvers don't answer for the name, and public DNS is blocked | site allowlists the domain on its resolvers |
| §4: broker `:8883` blocked, `www.google.com:443` open | port block | site opens outbound TCP 8883 |
| §5: issuer is not Let's Encrypt / ISRG | a firewall is decrypting TLS; the reader trusts only the anchors it was given (§7 of the commissioning runbook) | site exempts the reader from decryption |
| §1 wrong date, or §7 fails | clock not synced, so TLS fails as "not yet valid" | site opens UDP 123, or the reader is pointed at the site's NTP server |
| §8: `UnknownHostException` / `connect timed out` / `PKIX` | the vendor client's view of the same failures | as above |

§6 is the site's public IP, which you need for the broker side below.

## The broker side

A reader that reaches the broker and then fails retries every 5–10 s, so even a
TLS or auth failure leaves a long run of lines from one public IP in the broker
log:

```sh
kubectl -n trakrf-prod logs deploy/trakrf-mosquitto -c mosquitto | grep -F '<site public IP>'
```

Failed handshakes log the IP but not the client id, so match on §6. No lines
from the site at all means the traffic never left their network: DNS, a port
block, or a reader that isn't trying. Ignore the steady `unexpected eof` lines
from the node IP: those are load balancer health checks. The pod log covers
only about two days.
