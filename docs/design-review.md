# Design review (October 2026)

This review covers heliocontrol after its first working version: Go server,
Angular UI, MQTT bridge, and the Home Assistant integration. It was run against
a real web control (firmware 2.07D) in read-only mode. Each finding says what
was changed on the `refactor/design-review` branch, or why it was left.

## 1. What works well and stays

- **Layers:** `frame` → `protocol` → `device` → `service` → `api` / `mqtt`.
  Each layer is testable on its own, and the simulator makes the whole stack
  testable without hardware.
- **The catalog as data.** The point-to-page map is data, not UI code. The UI,
  the MQTT bridge and the HA integration all render from it.
- **Read-only enforced in two places** (API and protocol allowlist). This is the
  most important safety property and was verified on the real device: of 68
  commands sent, all were reads.
- **Prefix-matched replies.** The controller has no request IDs and streams
  records after counted requests; matching each reply to its request fixed
  real mix-ups.

## 2. Findings

### 2.1 Polling occupies the controller bus — *fixed*

Every poll read all ~20 setup points of the pages one by one. Measured on the
real controller, a single `~SP` read takes 100–200 ms. That made **2–3 s of
every 15 s poll** (15–20 % of the time) a busy bus, during which user actions
and recorder samples had to wait. Most of these points are settings that only
change when someone changes them.

**Change:** polling is tiered.

| Tier | What | When |
|---|---|---|
| Fast | All measuring points via one `~MR` multi-read (~130 ms), plus the few volatile setup points (compressor state SP 10, running time SP 11, operating hours) | Every poll |
| Slow | All other setup points (settings) | Every 10 minutes, after our own writes, and on demand |

Settings changed at the controller's own panel therefore show up within 10
minutes, or immediately via *Refresh*.

*Verified against the simulator, which counts the commands it receives: a fast
poll reads 2 setup points instead of ~22. A real-device measurement is still to
do. The controller accepts one session at a time, and it was in use by the
production instance when this was written.*

### 2.2 Writes have no protection against automations — *fixed*

Home Assistant automations can write setpoints at any rate. Controllers of this
generation most likely keep parameters in EEPROM, which has limited write
cycles. That is an assumption (no datasheet), but it's the conventional design.
A misconfigured automation ("set room target on every PV change") could write
thousands of times a day. Writing the same value again also causes
unnecessary bus traffic and log noise.

**Change:** a write guard in the service.

- **No-op writes** (value equals the current one) are acknowledged without
  touching the controller.
- **A daily write budget per setup point** (default 48, configurable) refuses
  further writes with a clear error. People editing in the UI never hit it;
  runaway automations do.

### 2.3 No safe way to change a value *temporarily* — *new*

The interesting modern control strategies (section 3) all mean "raise or lower a
setpoint for a while, then go back". Doing that with plain writes from Home
Assistant is fragile:

- If HA restarts, the restore never happens.
- Two automations can fight over the same value.
- Nobody remembers the original value.

**Change: temporary setpoints.** `POST /api/overrides {sp, value, minutes}`
stores the original value, writes the new one, and restores it automatically
when the time runs out (or on `DELETE`).

- **Restart-safe:** overrides are persisted, and expired ones are restored after
  a restart.
- **One override per point:** a new one replaces the old, but keeps the
  *original* value.
- **Exposed everywhere:** Home Assistant service `heliocontrol.set_temporary`,
  and on the overview page with a countdown and cancel.

This is the controller-side equivalent of an *SG-Ready "recommended on"*
signal, implemented with the setpoints this controller does have.

### 2.4 Entity mapping is implemented twice — *fixed*

The MQTT bridge (Go) and the HA integration (Python) each turned catalog
items into Home Assistant entities, and they had already drifted apart: only
the Python side prefixed mixer circuit names.

**Change:** `internal/haentity` builds the entity list once (component, name in
both languages, unit, device class, state class, options, limits, category).

- The MQTT bridge renders it.
- The server publishes it at `GET /api/ha/entities`, and the Python integration
  consumes it, so most mapping logic left the Python code.

### 2.5 There is no history, so no insight — *new*

homeControl only recorded on demand (WEB-Online) or through the controller's
small trend buffer. Without continuous history, nobody can see whether the
heat pump short-cycles or whether the heating curve is too high. Yet those two
are the biggest free efficiency levers (section 3).

**Change: always-on history** of the page values every minute (≈30 points ≈
45k rows per day), with configurable retention (default 400 days), plus
compressor start/stop tracking.

An **Insights** page shows per day:

- compressor starts, runtime and average cycle length
- outdoor, flow and return averages
- the deviation of the actual return temperature from the curve target

Rule-based hints point out short-cycling and an over-high heating curve.
Cycle counts come from the controller's own counter (MP 41, compressor
starts) where available. That counter is exact; sampling would miss short
cycles.

### 2.6 Smaller items

| Item | Status |
|---|---|
| `store.Recording(id)` loaded every recording to find one | fixed (direct query) |
| `Service` mixes session, polling, panels, jobs, recorder | split into files by concern (`poll.go`, `writes.go`, `overrides.go`, `history.go`); one type remains, because they share one session and lock |
| Browsers allow 6 HTTP/1.1 connections per host, and each open UI tab holds one SSE stream | documented; would need HTTP/2 (TLS) |
| No metrics endpoint | left out: HA already records history |

## 3. Heating concepts and how they apply here

Current practice for heat pumps has moved from "fixed curve, fixed schedule"
to *using the building and hot water tank as storage*, timed to cheap or
self-generated electricity, and to *running as low and as long as possible*.

### 3.1 Efficiency first: low flow temperature, long cycles

- **Lower flow temperature:** guides cite about 2.5 % better seasonal
  performance per degree of lower flow temperature. They recommend lowering the
  heating curve step by step over several days while watching room
  temperatures. [42watt](https://42watt.de/magazin/waermepumpe-effizienz),
  [reduco](https://reduco.ai/blog/heizung/vorlauftemperatur-senken)
- **Hydraulic balancing:** often credited with 5–10 % on its own.
- **Avoid short-cycling:** frequent cycling points to oversizing or wrong
  control settings. [42watt – Regelung](https://42watt.de/magazin/warmepumpen-regelung)

**In heliocontrol:**
- The Insights page measures starts per day, cycle length and curve deviation,
  and suggests lowering the curve when return temperatures run consistently
  above need.
- The heating-curve editor already shows the effect of a change.

Turning these numbers into a feedback loop needs room temperatures. The
controller's room sensor inputs (MP 16–19) are unused in this installation,
and room thermostats in Home Assistant could provide them later.

### 3.2 Load shifting with PV surplus (SG-Ready style)

Modern heat pumps have an *SG-Ready* input: an energy manager signals
"recommended on" when PV surplus or cheap power is available, and the heat pump
raises its setpoints to store heat.
[IDM – Smart Grid](https://www.idm-energie.at/en/heat-pump-installation/smart-grid-connection/),
[Solimpeks – SG-Ready](https://solimpeks.com/en/knowledge/glossary/sg-ready)

A hot water tank stores a few kWh this way (≈3.5 kWh for 300 l and +10 K).
Studies report noticeable self-consumption gains, even though the interface
can't follow PV output precisely.
[ScienceDirect field study](https://www.sciencedirect.com/science/article/pii/S2667113123000153)

Open-source energy managers (evcc, Home Assistant blueprints) drive heat pumps
this way.
[evcc + Bosch/Buderus](https://bosch-buderus-wp.github.io/en/docs/smarthome/evcc/),
[HA SG-Ready controller](https://github.com/ShortyMF/Home-Assistant-SG-Ready-Heat-Pump-Control)

**In heliocontrol:** the web control 2.07D exposes no SG-Ready state in its
protocol (only the utility-block input MP 37). *Temporary setpoints* give the
same effect without hardware: when HA sees surplus, it calls
`heliocontrol.set_temporary` for e.g. hot water target +8 K for 60 min.
heliocontrol restores the value afterwards, even across restarts.

### 3.3 Dynamic electricity tariffs

Spot-price tariffs (hourly or quarter-hourly from EPEX Spot, smart meter
required) are available in Austria. One utility even sells a heat-pump-specific
dynamic tariff. Simulations of rule-based SG-Ready strategies with dynamic
prices, setback and PV report cost reductions of roughly 13–50 %, depending on
climate.
[checkeverything.at](https://www.checkeverything.at/de/blog/dynamische-stromtarife-2026),
[smartENERGY smartHEAT](https://smartenergy.at/fileadmin/user_upload/downloads/smartENERGY_Tarifblatt_smartHEAT.pdf),
[Springer 2026](https://link.springer.com/chapter/10.1007/978-3-032-03098-6_14)

**In heliocontrol:** the same primitive works with prices. Raise hot water or
room target (or switch to *Party*/boost) in the cheapest hours, and lower it
(reduction) in the most expensive ones. The write budget and automatic restore
make this safe to automate. The price logic belongs in HA, where tariff
integrations already exist.

### 3.4 Predictive control (weather and price forecasts)

Research and commercial energy managers plan the next 24 h from PV, price and
weather forecasts. That is model-predictive control: it pre-heats when it's
cheap, and when the outdoor air is warmer, for air-source pumps' COP.
[Springer 2026](https://link.springer.com/chapter/10.1007/978-3-032-03098-6_14)

**In heliocontrol:** not implemented. It needs a thermal model of the house,
which the new history makes possible to estimate later: heat-up and cool-down
rates per outdoor temperature. The building blocks (history, temporary
setpoints, time programs with boost/reduce states) are now in place.

## 4. Roadmap (not on this branch)

1. **Room temperature input from Home Assistant**, to tune the heating curve
   from measured comfort instead of rules of thumb.
2. **Energy estimate** without a meter: compressor runtime × an
   inverter-power estimate (MP 94 current output) as a rough kWh figure.
3. **Forecast-based pre-heating** using the learned heat-up and cool-down rates.
4. **HTTP/2** behind a TLS reverse proxy, to remove the SSE connection limit.
