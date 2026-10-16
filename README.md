# heliocontrol

A self-hosted replacement for **Heliotherm homeControl**, the discontinued
Windows tool for Heliotherm heat pumps with the *web control* controller.
heliocontrol has a Go server, an Angular web UI, and a Home Assistant
integration (native custom component or MQTT discovery).

- **No license key needed.** homeControl's license only gated the Windows app;
  the controller doesn't check anything.
- **Read-only by default.** Nothing is written to the heat pump until you
  switch read-only mode off in the settings.
- The protocol is documented in [docs/protocol.md](docs/protocol.md). It is
  *not* Modbus but a proprietary ASCII protocol on TCP port 4001.

## Features (homeControl → heliocontrol)

| homeControl | heliocontrol |
|---|---|
| Tabs: *Grundeinstellung*, *Mischerkreis 1/2*, *Energiequelle*, *Anlagenschema*, *Energiezählung* | **Overview**. Pages are chosen from the installation bits (SP 9) like homeControl did, and points the controller marks as unused are hidden. A *Diagnostics* page lists all other used measuring points with the controller's own names. |
| Operating mode, room target, heating curve with cursor | Editable, with an interactive heating-curve chart (normal / reduced / boost) |
| Hot water target / minimum | On the installation page |
| Auto refresh | Live values via server-sent events (poll interval configurable) |
| *Zeitprogramme* (copy day, save/load file, transfer changes) | **Time programs**: week view, per-day editor, copy/paste days, JSON export/import, writes only changed events |
| *WEB-Online* recorder (data points, interval, pause/stop, cursors A/B, value axis, open .hwo/.htd) | **WEB-Online**: same, plus CSV export; imports homeControl `.hwo` and `.htd` files |
| *Trenddaten* (read and delete, edit trend points: alarm, call remote, interval/hysteresis, end) | **Trend data**. Downloads become recordings you can open in WEB-Online. |
| *Entstören* | Fault banner with reset button |
| *Zeit synchronisieren* | Settings → clock |
| *Messpunkte initialisieren* | Automatic on first connect (names and limits are cached) |
| Serial number check | Optional `expectedId` |
| Password lock | Optional login password (bcrypt). The old app's hard-coded `SuperUser` bypass is not reproduced. |
| — | **Home Assistant** via MQTT discovery |
| — | **All points** page (all 96 MP / 392 SP) and an expert mode to change any writable setup point |
| — | Read-only mode, German/English, dark mode, phone layout |

## Beyond homeControl

These features follow from the [design review](docs/design-review.md), which
also explains the heating concepts behind them.

- **Temporary values.** Change a setpoint for a while (e.g. hot water +8 K for
  60 min), and heliocontrol restores the original automatically, even after a
  restart. Available on the overview page, via `POST /api/overrides`, and as the
  Home Assistant service `heliocontrol.set_temporary`. This is the
  controller-side equivalent of SG-Ready "recommended on", for PV surplus or
  cheap hours.
- **Write budget.** Each setup point may be changed at most 48 times per day
  (configurable), which protects the controller's EEPROM from runaway
  automations. Writing the current value again sends nothing.
- **Insights.** Always-on history (stored on change, kept 400 days) gives
  compressor starts per day (exact, from the controller's start counter),
  runtime, cycle length, flow, return and spread while running, and the
  deviation from the return target. Hints point out short cycling or a heating
  curve that is higher than necessary.
- **Lighter polling.** Measuring points come from one multi-read; settings are
  re-read every 10 minutes, after writes, and on *Refresh*. That cuts the bus
  time per poll from 2–3 s to a few hundred ms.

Example: store PV surplus in the hot water tank (Home Assistant):

```yaml
automation:
  - alias: Hot water boost on PV surplus
    triggers:
      - trigger: numeric_state
        entity_id: sensor.grid_export_power
        above: 2500
        for: { minutes: 10 }
    actions:
      - action: heliocontrol.set_temporary
        target:
          entity_id: number.heliotherm_hot_water_target
        data: { value: 56, duration: 60 }
```

## Findings

What was learned while building heliocontrol: by decompiling homeControl 2.1,
by running read-only probes against a real web control (firmware 2.07D), and in
the [design review](docs/design-review.md).

### About homeControl and the controller

- **It's not Modbus.** The controller speaks a proprietary ASCII protocol
  (`~MP,NR=3;`, `~SP,NR=13,VAL=1;` …) inside a small binary frame with its own
  checksum, on TCP port 4001 of the LAN modem or over RS232. The frame codec is
  byte-identical to homeControl's `DataFrame.dll` on 409 test cases. Details:
  [docs/protocol.md](docs/protocol.md).
- **No security at all.** Login (`~LIN;`) takes no password, and nothing is
  encrypted, so anyone who can reach port 4001 can change every setting.
  Forwarding that port through the router, as homeControl's manual suggests for
  remote access, exposes the heat pump to the internet.
- **The license is cosmetic.** homeControl checks the license code against a
  hash of the serial number entered in the app, offline. The controller never
  sees it. homeControl's own password lock has a hard-coded bypass (`SuperUser`).
- **One session at a time.** The modem refuses a second connection, so
  homeControl, heliocontrol and any other tool cannot run in parallel.
- **No request IDs.** Replies aren't tagged, and counted requests (`~RM`,
  `~PRL`, `~PRD`) keep streaming records after the first answer. A request sent
  too early receives leftovers of the previous one, so heliocontrol matches
  every reply to its request by prefix.
- **The controller describes itself.** Every point reply carries a name, its
  limits, "used" and "writable" flags. SP 9 holds the installation bit mask as
  its value and the firmware version as its name. Text is ISO-8859-1.
- **Firmware differs from homeControl's assumptions:**
  - The operating mode only goes up to 7 (Party) on 2.07D.
  - MP 25 is the hot water priority valve, not a pump.
  - MP 83/89 are power values, and MP 88 is the inverter frequency.
  - Mixer circuit 1 has its own enable flag (SP 199).
  - The controller's own limits differ from homeControl's hard-coded ones; the
    heating limit, for example, goes to 45 °C, not 24 °C.
- **Timing (real device):** one point read takes 100–200 ms. The multi-read
  `~MR` fetches 4 values in about 130 ms. Reading all 96 measuring points takes
  10 s, and all 392 setup points 23 s.
- **Bugs in homeControl:**
  - The brine and water source tabs show the heating pump (MP 22) as compressor
    and source pump.
  - Trend end times are sent in a 12-hour clock.

### About the design (fixed on this branch)

- **Polling occupied the bus.** Re-reading ~20 settings every 15 s kept the
  controller busy 15–20 % of the time, so polling is now tiered.
- **Automations could wear out the controller.** Parameters most likely live in
  EEPROM (assumed; there is no datasheet), hence the write budget and the
  skipping of no-op writes.
- **"Change it for a while" had no safe form.** Restores were lost on restarts,
  and automations could fight over a value, hence restart-safe temporary values.
- **Entity mapping lived in two places (Go and Python) and had drifted.** It is
  now built once by the server. Writing the shared tests caught a real bug: the
  outdoor temperature (point 0) vanished in the Python integration.
- **Without history there was no way to see efficiency**, hence the continuous
  history and the Insights page.

### About heating with this heat pump

- **The free efficiency levers come first:** a lower flow temperature (about
  2.5 % better efficiency per degree), long instead of short compressor cycles,
  and hydraulic balancing.
  [42watt](https://42watt.de/magazin/waermepumpe-effizienz),
  [reduco](https://reduco.ai/blog/heizung/vorlauftemperatur-senken)
  The Insights page measures exactly these.
- **Heat storage with PV surplus.** Modern heat pumps take an *SG-Ready* signal
  to store heat when power is cheap or self-generated.
  [IDM](https://www.idm-energie.at/en/heat-pump-installation/smart-grid-connection/),
  [field study](https://www.sciencedirect.com/science/article/pii/S2667113123000153)
  The web control 2.07D has no such input in its protocol. Temporary values give
  the same effect (see the automation example above).
- **Dynamic tariffs.** Hourly spot-price tariffs exist in Austria, and
  simulations of price-aware heat pump control report roughly 13–50 % lower
  costs.
  [checkeverything.at](https://www.checkeverything.at/de/blog/dynamische-stromtarife-2026),
  [Springer 2026](https://link.springer.com/chapter/10.1007/978-3-032-03098-6_14)
  This uses the same mechanism, driven by tariff integrations in Home Assistant.
- **Predictive control** (forecast-based pre-heating) is not implemented yet.
  It needs a thermal model of the house, which the new history makes possible
  to learn.

## Quick start

```sh
cp heliocontrol.example.yaml heliocontrol.yaml   # set device.host
docker compose up -d                              # or: make all && bin/heliocontrol
```

Open <http://localhost:8080>. With Docker, everything lives in `./data/`: put
`heliocontrol.yaml` there (or start without one and set the heat pump address
under *Settings*, which writes the file). The container fixes the ownership of
`./data` on start, so it may also be created by Docker.

To try it without a heat pump:

```sh
make run-sim          # simulated controller + server, writes allowed
```

## Home Assistant

There are two ways to connect Home Assistant. Use one of them, not both, or
every entity shows up twice.

### Custom integration (recommended)

`custom_components/heliocontrol` is a native integration that talks to the
heliocontrol server's API and live event stream. It needs no MQTT broker.

1. Install it. Either:
   - **HACS:** *Custom repositories* → this repository, category *Integration*, or
   - **manually:** copy `custom_components/heliocontrol` into your Home Assistant
     `config/custom_components/`.
2. Restart Home Assistant.
3. Go to *Settings → Devices & services → Add integration → Heliotherm (heliocontrol)*.
4. Enter the server URL, e.g. `http://192.168.1.10:8080`, plus the password if
   you enabled access protection.

You get the same entities as the MQTT variant below, named in Home Assistant's
language (German or English). Updates are pushed live, with a 60 s polling
fallback. When the pages or read-only mode change on the server, the entry
reloads by itself.

### MQTT discovery

Enable MQTT in *Settings* (broker URL, user, password). The heat pump appears
as the device **Heliotherm** with:

| Type | Entities |
|---|---|
| Sensors | Temperatures, pressures, operating hours, compressor state, request, energy, SPF, firmware |
| Binary sensors | Pumps, compressor, valves, fault |
| Selects | Operating mode (main and mixer circuits) |
| Numbers | Room target, heating curve, hot water temperatures |
| Buttons | Refresh, reset fault, sync clock |

In read-only mode, selects and numbers are published as plain sensors and the
write buttons are omitted. Home Assistant then doesn't offer controls that
would be refused.

**Topics:**
- Discovery: `homeassistant/<component>/heliotherm_<id>/<key>/config`
- State: `heliocontrol/<id>/<key>/state`
- Commands: `heliocontrol/<id>/<key>/set`
- Availability: `heliocontrol/status`

## Safety

- **Read-only mode is enforced twice:** in the API, and again in the protocol
  client, which refuses any command not on an explicit allowlist of read
  commands before it reaches the network (`internal/protocol/readonly.go`).
- **Writes are checked** against the limits the controller reports. Modes the
  controller only reports are refused (e.g. *main switch*).
- **Only one connection at a time:** the controller handles one session. Don't
  run homeControl, or a second heliocontrol, against the same modem.
- **Keep port 4001 local:** the protocol has no authentication. Use a VPN
  instead of forwarding the port.

## Development

```
cmd/heliocontrol    server
cmd/heliosim        simulated controller (same protocol, for tests and UI work)
cmd/heliocli        `heliocli probe -host …`: read-only capture of raw replies
internal/frame      binary envelope (golden tests against the original DataFrame.dll)
internal/protocol   commands, reply parsing, read-only allowlist
internal/device     connection, request/reply matching, keep-alive, typed operations
internal/catalog    point → page map (homeControl's tab layout as data), heating curve
internal/service    session, tiered polling, write guard, temporary values,
                    history + insights, recorder, trend/time-program jobs, event bus
internal/store      SQLite (pure Go): log, point cache, recordings, history, overrides
internal/haentity   Home Assistant entity descriptions (shared by MQTT and the integration)
internal/mqtt       Home Assistant MQTT discovery bridge
custom_components/  Home Assistant custom integration (Python)
internal/api        REST + SSE + auth, embeds the UI
internal/legacy     homeControl .htd / .hwo import
web/                Angular 22 app (zoneless, standalone, signals)
```

`make test` runs the Go tests (including end-to-end tests against the
simulator and an embedded MQTT broker) and the web unit tests. `make test-ha`
runs the Home Assistant integration tests (`tests/homeassistant`): they build
the server and the simulator, start both, and drive the integration inside a
Home Assistant test instance (needs Go, plus Python with
`pytest-homeassistant-custom-component`; Linux or macOS). `make dev`
starts the Angular dev server with a proxy to the API on :8080.

`tools/golden/golden.ps1` regenerates `internal/frame/testdata/golden.txt` from
homeControl's `DataFrame.dll` (Windows PowerShell).

## Notes on fidelity

- **Bug fixes:**
  - homeControl's brine and water source tabs bound the compressor and source-pump
    indicators to MP 22 (the heating pump). heliocontrol uses MP 30 / MP 24.
  - homeControl sent trend end times with a 12-hour clock. heliocontrol sends
    24-hour times, which matches what the controller reports.
- **Firmware-dependent labels:** on firmware 2.07D, MP 25 is the hot water priority valve,
  MP 83/89 are power values, and MP 88 is the inverter frequency. The catalog
  follows the controller where homeControl's labels didn't match.
- **Not reimplemented:** licensing, the online updater, the bootloader/firmware update
  commands, modem phone numbers, and the old `.fwo`/`.fwt` formats of homeControl 1.x.
