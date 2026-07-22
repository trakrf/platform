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

**Verified GPIO mapping on cs463-212** (sysfs, readable over SSH — useful for
programmatic verification without hardware indicators):

| Function | sysfs line |
|---|---|
| GPO1 | `gpio205` |
| GPO2 | `gpio2` |
| GPO3 | `gpio175` |
| GPO4 | `gpio176` |
| GPI 1–4 | `gpio203`, `gpio46`, `gpio7`, `gpio8` |
