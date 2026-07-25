# CS463 reader references

Vendor documentation for the CSL Intelligent Fixed Reader family (CS463), which
the `mqtt-rpc` on-reader daemon drives.

## Checked in here

- **`CSL_Intelligent_Fixed_Reader_Network_HTTP_API.pdf`** — the `/API` command
  surface (V1.4). Section 8 "IO Management" covers the GPIO commands. Checked in
  because the CS463 work repeatedly needs command-level detail that exists
  nowhere else in this repo, and the vendor's download URLs have moved before.

## Upstream — start here

**All CSL manuals:**
<https://github.com/cslrfid/CS463-CS203X-Product-Downloads/tree/main/Manuals>

That directory is the canonical source and covers the whole reader family. We
check in only the HTTP API PDF above; everything else is large and needed a
chapter at a time. Notable contents:

| Folder | What it's for |
|---|---|
| `1 - User Manual` | Hardware. **§5.13** GPIO pin/function table, **chapter 6** GPIO Ports Connection Guide |
| `3 - CSL HTTP API` | The `/API` command surface (same doc as the PDF here) |
| `4 - TCPIP Network Specificatoins` | Low-level network protocol *(vendor's typo, not ours)* |

**Read chapter 6 of the User Manual before wiring anything.** Worked examples 1
and 3 establish GPO polarity — see below.

**HTTP API demo app** — <https://github.com/cslrfid/CSL-HTTP-Demo>
(`CS463_HL_CS/CS463_HL_API.cs`). The reference implementation, and the fastest
way to resolve ambiguity about parameter names. It documented `directIOOutput`
and `importTagGroupCSV` more clearly than the manual did.

### Other CSL product families

CSL publishes a downloads repo per product line, same layout:

- **CS463 / CS203X** — fixed readers.
  <https://github.com/cslrfid/CS463-CS203X-Product-Downloads>
- **CS108** — handheld. **Already supported in this monorepo.**
  <https://github.com/cslrfid/CS108-Product-Downloads>
- **CS710S** — next-generation handheld, newer Impinj module.
  <https://github.com/cslrfid/CS710S-Product-Downloads>

**Fixed readers and handhelds take completely different paths through this
codebase** — they share no transport, no adapter, and no protocol code:

| | Fixed (CS463) | Handheld (CS108) |
|---|---|---|
| Lives in | `mqtt-rpc/` on-reader daemon + backend | `frontend/src/worker/cs108/` |
| Runs on | the reader / the server | the browser, in a web worker |
| Transport | HTTP `/API` + MQTT JSON-RPC | binary packet protocol over the worker bridge |
| Contract | `readerrpc` | `BaseReader` / comlink bridge |

So nothing in the `cs463` adapter is reusable for a handheld, and vice versa.
Adding **CS710S** would most likely extend the *worker* side alongside
`cs108/` — its packet protocol is the thing to compare, not this package.

## Facts worth knowing before you read either

**The GPO is a polarized switch, not a symmetric dry contact.** Current must
enter `GPO(+)` and exit `GPO(−)` — for GPO1, in at pin 4 and out at pin 14. Both
worked examples in chapter 6 show it this way.

Wired backwards, an internal body diode is forward-biased and conducts
continuously — regardless of the commanded state, and **even with the reader
powered off**. The usual checks are blind to it: a continuity test reads
open/closed correctly because the meter's test voltage sits below the diode's
forward threshold, and a manual jumper across the pins works fine because a hard
short bypasses the diode. The diagnostic that finds it is measuring the output
with the reader unplugged; a path that still conducts is passive.

**`directIOOutput` / `directIOInput` are sessionless.** They authenticate inline
and bypass the reader's single-root-session lock, so they work while an operator
has the web UI open. The session-bound equivalents (`runIO_output`,
`runIO_input`) do not.

**GPIO connector pinout** (user manual v2.1 §5.13; HD15 / DE-15):

| Function | (+) | (−) | Isolation |
|---|---|---|---|
| GPO1 | Pin 4 | Pin 14 | full |
| GPO2 | Pin 3 | Pin 13 | full |
| GPO3 | Pin 10 | **Pin 8** | (−) shared with GPO4 |
| GPO4 | Pin 9 | **Pin 8** | (−) shared with GPO3 |
| GPI1 | Pin 2 | Pin 12 | (−) shared with GPI3 |
| GPI2 | Pin 1 | Pin 11 | (−) shared with GPI4 |
| GPI3 | Pin 7 | Pin 12 | (−) shared with GPI1 |
| GPI4 | Pin 6 | Pin 11 | (−) shared with GPI2 |
| +12 V | Pin 5 | Pin 15 (`+12VGND`) | full |

**GPO3 and GPO4 share Pin 8.** The switches are still independent — verified on
cs463-212 with GPO3 commanded on: Pin 10 ↔ Pin 8 closed, Pin 9 ↔ Pin 8 open. But
anything that returns both channels through Pin 8 will couple them, so put
per-channel components in series with the `(+)` pin, which is never shared.

All GPOs are **Normal Open on power-up**. Maximum 2 A, opto-isolated switches
with an internal resettable fuse. Manual §6.2 asks for a series resistor sized
`V / 2 A` so a shorted load cannot damage the switch.

**Measured GPO characteristics** (cs463-212, 2026-07-25, GPO2 via 4.7 kΩ from a
23.88 V supply):

| | |
|---|---|
| On-state drop | **0.004 V** at 5.08 mA (`R_on` ≈ 0.79 Ω) |
| Off-state leakage | ≈ **17 MΩ** |

The on-state drop is negligible for low-current signalling — worth knowing when
budgeting headroom for anything driven through a GPO, because the reader
contributes essentially nothing. The manual does not specify either figure.

**Verified GPIO mapping on cs463-212** (sysfs, readable over SSH — useful for
programmatic verification without hardware indicators):

| Function | sysfs line |
|---|---|
| GPO1 | `gpio205` |
| GPO2 | `gpio2` |
| GPO3 | `gpio175` |
| GPO4 | `gpio176` |
| GPI 1–4 | `gpio203`, `gpio46`, `gpio7`, `gpio8` |

**Reader power.** 12 V DC via an externally-threaded 5.5 × 2.5 mm barrel jack
(the supplied adapter carries the mating female collar — the manual says *"When
using AC adaptor, please remember to screw tight"*), **or** PoE+ 802.3at from a
30 W port. Both appear on the unit's label. The manual gives no DC input voltage
range.

## Operational: the post-power-cycle wedge

Seen on cs463-212, 2026-07-25. **A power cycle can leave the reader with a fully
healthy-looking stack and a non-functional API.**

Symptoms — every one of these read normal:

- `systemctl is-active embeddedglassfish` → `active`
- `systemctl is-active mqtt-rpcd` → `active`, MQTT connected, RPC subscribed
- `GET /` → HTTP 200

While **every `/API` command returned HTTP 200 with an empty body**, and GPO
commands silently did nothing — no `<Ack>`, sysfs pin never moved. It is not an
auth failure: a bogus command, a wrong password, no credentials, and a valid
request all return *identically empty*. A working API answers those four
differently, which is the fastest way to tell the two apart.

The boot itself was unhealthy in ways nothing surfaced: the RTC started at
**2019-04-12**, and the daemon logged `crypto/rand: blocked for 60 seconds
waiting to read random data from the kernel`.

**Recovery:**

1. `systemctl restart embeddedglassfish`
2. Wait for `/API` to *actually answer* — roughly two minutes. **systemd "active"
   is not "serving"**, which is the whole trap.
3. `systemctl restart mqtt-rpcd`, so its golden-config reconcile runs against a
   healthy API rather than failing on a 502 and never retrying.

Tracked as TRA-1041 (content-validating health probe + reconcile retry). The
durable lesson: **assert on returned content, never on HTTP status or systemd
state.**

**Fetching the manual** (21 MB, not checked in):

```
curl -sL -o csl-manual.pdf \
  "https://raw.githubusercontent.com/cslrfid/CS463-CS203X-Product-Downloads/main/Manuals/1%20-%20User%20Manual/CSL-Intelligent-Fixed-Reader-User-Manual.pdf"
pdftotext -layout csl-manual.pdf manual.txt
```
