# TrakRF asset-egress demo — Tim's card

> **DRAFT for Mike + Tim review (2026-06-17).** This is the **operator** card —
> physical connections and the app only. No commands, no logins to the box.
> Anything not on this card = **call Mike**. Engineer detail lives in the
> companion `tra-904-demo-runbook.md`.

**The pitch in one line:** a tagged piece of equipment leaving through the door
sets off the strobe — and random tagged stuff walking by does *not*.

You only ever touch two things: the **physical gear** (plug it in, power it on)
and the **app in your browser** (`https://app.demo.trakrf.id`). That's it.

> **WiFi:** network **`TrakRF`**, password **`trakrf.id`**.

---

## A. Set up the gear — it ships pre-wired

The kit shipped with **everything already plugged into the surge protector** and
all the network cables connected. Your normal setup is tiny:

1. Set the pieces roughly in place — the **2–3 antennas at the doorway** (each aimed
   *across* the opening, not down the hall), the strobe near the door.
2. **Plug the surge protector into a wall outlet.** Everything powers on together.
3. *(Optional — only if the venue has wired internet)* plug the **orange** cable
   into the house router. The demo works fine without it.
4. Wait **~2 minutes** for the box and reader to boot. **Don't unplug anything.**

Here's how it's all connected, in case a cable comes loose or you need to repack:

```mermaid
flowchart TB
  wall([Wall outlet])
  surge["Surge protector<br/>(power strip + USB ports)"]
  slate{{"GL.iNet Slate (router)"}}
  hp["HP server (the box)"]
  cs463["CS463 reader"]
  mk107["Moko MK107"]
  gls10["GL-S10 gateway"]
  shelly["Shelly Plug Gen4"]
  siren["Alarm light / siren"]
  laptop([Your laptop])
  inet([House router / Internet])

  %% POWER (thick arrows)
  wall ==>|plug THIS into the wall| surge
  surge ==>|power block| hp
  surge ==>|power block| cs463
  surge ==>|direct| mk107
  surge ==>|direct| shelly
  surge ==>|blue/black USB-C| slate
  surge ==>|red USB cable| gls10
  shelly ==>|switched power| siren

  %% WIRED LAN (color-coded cables)
  slate ---|white cable| hp
  slate ---|purple cable| cs463
  slate ---|orange cable - WAN, optional| inet

  %% WIRELESS
  slate -. WiFi .- laptop
  slate -. WiFi .- gls10
  slate -. WiFi .- mk107
  slate -. WiFi .- shelly

  classDef power fill:#fff7ed,stroke:#fb923c;
  class surge,siren power;
  linkStyle 5 stroke:#3b82f6,stroke-width:2px;
  linkStyle 6 stroke:#ef4444,stroke-width:2px;
  linkStyle 8 stroke:#9ca3af,stroke-width:3px;
  linkStyle 9 stroke:#a855f7,stroke-width:3px;
  linkStyle 10 stroke:#f97316,stroke-width:3px;
```

**Cable colors:** white = router ↔ box · purple = router ↔ reader · orange =
router ↔ house internet (optional). **Thick arrows = power; dotted = WiFi.** The
alarm light/siren plugs into the **Shelly Plug**, so the Shelly switches its power.

---

## B. Turn it on / check it's alive  — every time, before an audience

1. On your **laptop**, connect to the Slate WiFi — network name **`TrakRF`**,
   password **`trakrf.id`**.
2. Open **Chrome** and go to **`https://app.demo.trakrf.id`**. Log in.
   - *Page won't load?* Re-check laptop WiFi is on the Slate. Still nothing →
     give the box another minute, reload. Still nothing → **call Mike**.
3. In the app, open **Settings → Live feed**. Hold a **tagged** item near the
   doorway antenna — you should see reads pop up in the list. **Reads = the
   reader is alive.**

   ![Live feed screen](img/tra904-live-feed.png)

4. Open **Settings → Outputs**, expand the door strobe, click **Test-fire**.
   The strobe should flash; it clears itself after a few seconds (or click
   **Reset (off)**).

   ![Output device with Test-fire and Reset](img/tra904-output-config.png)

✅ **You're green when:** the page loads, Live feed shows reads, and Test-fire
flashes the strobe. If all three work, the demo will work.

---

## B2. Dial in the range (transmit power)  — your tuning knob

Each antenna at the door has a **transmit-power slider** in the app. More power =
reads tags from farther away; less power = only catches tags right at the door.

1. In the app, open **Settings → Readers**, click your reader to expand it. You'll
   see a row per antenna, each with a **power slider (dBm)**.
2. Keep **Settings → Live feed** open in another tab while you test-walk.
3. Walk the tagged item through — including **in a bag / against your body**:
   - Doesn't catch the concealed item? **Slide power up.**
   - Catches tags from across the room / when just standing near? **Slide power down.**
4. Aim for: walking *through* always fires, standing *near* doesn't. Sweet spot
   found → leave it. (No save button — it applies live.)

   ![Antenna list with transmit-power sliders](img/tra904-antennas-power.png)

> This is the one setting you'll actively tune in the room. Everything else Mike
> pre-set. If you can't find a setting that works, **call Mike**.

---

## C. Run the demo

1. **Calm state** — strobe off, nothing happening. *"Nothing tracked is leaving."*
2. **The catch** — walk the **registered** item through the doorway at a normal
   pace. **Strobe flashes within ~1 second.** *"That just walked out — caught."*
3. **Reset** — clear the strobe (it may auto-clear; if not, **Outputs → Reset**).
4. **The point** — walk the **decoy** item (the one that's *not* registered)
   through. **Nothing happens.** *"Random tagged goods don't false-alarm — only
   your tracked assets do."*
5. **The real-world version** — carry the registered item in a **bag or against
   your body** and walk through. Catches most of the time. Honest line: *"~80–90%
   even concealed — and at replacement cost, catching most is the win."*
6. Reset and repeat as needed.

---

## D. If something looks wrong  (what YOU can do)

| You see… | Try this |
|---|---|
| Page won't load | Laptop on Slate WiFi? Reload. Wait 1 min. Still down → **call Mike**. |
| No reads in Live feed | Power-cycle the **reader** (unplug power, plug back, wait ~2 min). |
| Walking through doesn't fire | Reset the strobe and try again. Still nothing → **call Mike** (it's a settings tweak). |
| Strobe flashes for *everything* | **Call Mike** — needs a sensitivity tweak. |
| Strobe stuck ON | In the app: **Outputs → Reset**. |
| Strobe never flashes, even on Test-fire | Check the strobe is plugged in / powered. Still nothing → **call Mike**. |
| Box got unplugged / power blip | Plug it back in, power on, wait ~2 min, redo section **B**. |

**Golden rules**
- ✅ You handle: **plugging things in, powering on, clicking in the app.**
- 📞 Call Mike for: **anything that needs a setting changed, or that a power-cycle + wait doesn't fix.**
- 🚫 Never run software updates or connect the box to venue/hotel internet during a demo — it's meant to run on its own.
