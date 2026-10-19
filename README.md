# heliocontrol

A self-hosted replacement for **Heliotherm homeControl**, the discontinued
Windows tool for Heliotherm heat pumps with the *web control* controller.
heliocontrol has a Go server, an Angular web UI, and a Home Assistant
integration (native custom component or MQTT discovery). It talks the
controller's proprietary ASCII protocol on TCP port 4001 (*not* Modbus), documented in
[docs/protocol.md](docs/protocol.md). (No HtRest needed)

![heliocontrol overview with heating curve](docs/screenshot.png)

## Quick start

Ready-to-use binaries for Linux, macOS, and Windows (amd64 and arm64) are
attached to each published GitHub release. Download and extract the archive
for your platform, then run `./heliocontrol` (or `heliocontrol.exe` on Windows)
in a writable directory and open <http://localhost:8080>. The web UI is
included; set the heat pump address under *Settings*, or copy
`heliocontrol.example.yaml` to `heliocontrol.yaml` and edit it before starting.
`checksums.txt` contains SHA-256 checksums for the archives.

Alternatively, build from source or use Docker:

```sh
docker compose up -d --build
```

Open <http://localhost:8080> and set the heat pump address under *Settings*.
For installation on a server, see [Server setup with Docker Compose](#server-setup-with-docker-compose).

Nothing is written to the heat pump until you switch read-only mode off in the
settings. To try it without a heat pump:

```sh
make run-sim          # simulated controller + server, writes allowed
```

## Server setup with Docker Compose

You need Docker Engine with the Docker Compose plugin, Git, and a server that
can reach the heat pump's LAN modem on TCP port 4001.

1. Clone the repository on the server:

   ```sh
   git clone https://github.com/scharph/heliocontrol.git
   cd heliocontrol
   ```

2. In `docker-compose.yml`, set `TZ` to the heat pump's local timezone
   (default: `Europe/Vienna`). The controller clock and trend timestamps use
   local time. The default port mapping serves the web UI on port 8080; change
   the left side of `8080:8080` if that port is already in use.

3. Build and start the server in the background:

   ```sh
   docker compose up -d --build
   ```

   Compose builds the Go server and web UI from source, so no local Go or Node
   installation is needed. The service restarts automatically after a server
   reboot, provided Docker starts on boot.

4. Open `http://<server-ip>:8080` in your browser. Under *Settings*, enter the
   heat pump's LAN modem address and port `4001`, and enable automatic
   connection. Leave read-only mode enabled while checking the live values.
   You can also enable access protection with a password in *Settings*.
   For access from outside your LAN, use a VPN (see [Safety](#safety)).

Configuration, the SQLite database, and recordings persist in `./data/`, which
is mounted at `/data` in the container. Settings saves the configuration as
`./data/heliocontrol.yaml`. To configure it before the first start instead:

```sh
mkdir -p data
cp heliocontrol.example.yaml data/heliocontrol.yaml
# Edit data/heliocontrol.yaml and set device.host to the LAN modem address.
```

The container fixes ownership of `./data` on startup and then runs as the
unprivileged `helio` user. Back up this directory; stop the service before
copying it to get a consistent database backup.

Run these commands from the repository directory:

```sh
docker compose logs -f heliocontrol   # follow server logs
docker compose stop                  # stop without removing the container
docker compose down                  # remove the container; ./data is retained
```

To update, back up `./data`, then pull the latest source and rebuild:

```sh
git pull --ff-only
docker compose up -d --build
```

## Where homeControl's features went

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
| — | **All points** page (all 96 MP / 392 SP) and an expert mode to change any writable setup point |

Not reimplemented: licensing, the online updater, the bootloader/firmware
update commands, modem phone numbers, and the old `.fwo`/`.fwt` formats of
homeControl 1.x.

## What's better than the old homeControl.exe

| | homeControl 2.1 (Windows) | heliocontrol |
|---|---|---|
| **Runs on** | One Windows PC with .NET 4.0, open the whole time | A server (Docker, Linux, Windows, NAS); use it from any browser, phone included |
| **License** | License code per heat pump serial number, from the installer | None needed (the old check never involved the controller) |
| **Access protection** | Local password with a hard-coded `SuperUser` bypass | Optional login with a bcrypt-hashed password, session cookies |
| **Remote access** | Port 4001 forwarded through the router, so the heat pump was open to the internet without a password | Port 4001 stays in the LAN; reach the web UI via VPN or Home Assistant |
| **Safety** | Every change goes straight to the heat pump | Read-only by default (enforced twice); writes checked against the controller's limits; daily write limit per setting; writing the current value again sends nothing |
| **Smart home** | None | Home Assistant: native integration or MQTT discovery, with sensors, mode selects, setpoints, buttons |
| **Temporary changes** | Not possible | "Hot water +8 K for 60 min" is restored automatically, even after a restart; the basis for PV surplus and dynamic tariffs |
| **History** | Only on demand (WEB-Online) or the controller's small trend buffer | Always-on history (400 days), stored on change |
| **Efficiency analysis** | None | Insights: compressor starts, runtime, cycle length, temperatures while running, deviation from target, hints for short cycling or a high heating curve |
| **Data shown** | Fixed tabs; some labels wrong for newer firmware | Same pages, built from the controller's own descriptions; unused points hidden; a diagnostics page with 28 extra values (hot gas, subcooling, inverter …); all 488 points browsable |
| **Live updates** | Manual refresh, or a timer that re-read everything | Values pushed to the browser; settings read rarely, so the controller's line is mostly free |
| **Reliability** | Replies could mix up after timeouts; garbled umlauts | Each reply matched to its request; correct text decoding; reconnect with backoff |
| **Data files** | Proprietary `.hwo` / `.htd` | Imports those, exports CSV, and stores recordings in a database |
| **Bugs fixed** | Brine/water tabs show the heating pump as compressor; trend end times in a 12-hour clock | Fixed |
| **Languages, look** | German/English, fixed window | German/English, dark mode, phone layout |
| **Tested** | — | Protocol checked against the original DLL, end-to-end tests with a simulated heat pump, MQTT broker and Home Assistant, verified read-only on a real controller |

## Beyond homeControl

These features came out of the [design review](docs/design-review.md), which
also explains the heating concepts behind them.

- **Temporary values.** Change a setpoint for a while (e.g. hot water +8 K for
  60 min), and heliocontrol restores the original automatically, even after a
  restart, so automations can't lose a restore or fight over a value.
  Available on the overview page, via `POST /api/overrides`, and as the Home
  Assistant service `heliocontrol.set_temporary`.
- **Write budget.** Parameters most likely live in EEPROM (assumed; there is no
  datasheet), so each setup point may be changed at most 48 times per day
  (configurable). Writing the current value again sends nothing.
- **Insights.** The always-on history gives compressor starts per day (exact,
  from the controller's start counter), runtime, cycle length, flow, return and
  spread while running, and the deviation from the return target. Hints point
  out short cycling or a heating curve that is higher than necessary.
- **Tiered polling.** Re-reading ~20 settings every 15 s used to keep the
  controller busy 15–20 % of the time. Now measuring points come from one
  multi-read, and settings are re-read every 10 minutes, after writes, and on
  *Refresh*: a few hundred ms of bus time per poll instead of 2–3 s.

### Heating concepts

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
  The web control 2.07D has no such input in its protocol; temporary values
  give the same effect (example below).
- **Dynamic tariffs.** Hourly spot-price tariffs exist in Austria, and
  simulations of price-aware heat pump control report roughly 13–50 % lower
  costs.
  [checkeverything.at](https://www.checkeverything.at/de/blog/dynamische-stromtarife-2026),
  [Springer 2026](https://link.springer.com/chapter/10.1007/978-3-032-03098-6_14)
  Same mechanism, driven by tariff integrations in Home Assistant.
- **Predictive control** (forecast-based pre-heating) is not implemented yet.
  It needs a thermal model of the house, which the history makes possible to
  learn.

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

## Home Assistant

There are two ways to connect Home Assistant. Use one of them, not both, or
every entity shows up twice. Both get their entity list from the server
(`internal/haentity`), so they always match.

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

Entities are named in Home Assistant's language (German or English). Updates
are pushed live, with a 60 s polling fallback. When the pages or read-only mode
change on the server, the entry reloads by itself.

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
write buttons are omitted (this applies to the custom integration too). Home
Assistant then doesn't offer controls that would be refused.

**Topics:**
- Discovery: `homeassistant/<component>/heliotherm_<id>/<key>/config`
- State: `heliocontrol/<id>/<key>/state`
- Commands: `heliocontrol/<id>/<key>/set`
- Availability: `heliocontrol/status`

### Smart Climate

`custom_components/smart_climate` is a separate helper integration for
room/building/flow-temperature/mixer control. It doesn't depend on
heliocontrol. Install it manually by copying the folder; see
[docs/smart-climate.md](docs/smart-climate.md).

## Safety

- **Read-only mode is enforced twice:** in the API, and again in the protocol
  client, which refuses any command not on an explicit allowlist of read
  commands before it reaches the network (`internal/protocol/readonly.go`).
- **Writes are checked** against the limits the controller reports. Modes the
  controller only reports are refused (e.g. *main switch*).
- **Only one connection at a time:** the modem refuses a second session. Don't
  run homeControl, or a second heliocontrol, against the same modem.
- **Keep port 4001 local:** login (`~LIN;`) takes no password and nothing is
  encrypted, so anyone who can reach the port can change every setting.
  Forwarding it through the router, as homeControl's manual suggests, exposes
  the heat pump to the internet. Use a VPN instead.

## Findings

What was learned by decompiling homeControl 2.1 and by running read-only probes
against a real web control (firmware 2.07D):

- **The protocol** is ASCII (`~MP,NR=3;`, `~SP,NR=13,VAL=1;` …) inside a small
  binary frame with its own checksum, on TCP port 4001 of the LAN modem or over
  RS232. heliocontrol's frame codec is byte-identical to homeControl's
  `DataFrame.dll` on 409 test cases.
- **The license is cosmetic.** homeControl checks the license code offline
  against a hash of the serial number entered in the app. The controller never
  sees it.
- **No request IDs.** Replies aren't tagged, and counted requests (`~RM`,
  `~PRL`, `~PRD`) keep streaming records after the first answer. A request sent
  too early receives leftovers of the previous one, so heliocontrol matches
  every reply to its request by prefix.
- **The controller describes itself.** Every point reply carries a name, its
  limits, "used" and "writable" flags. SP 9 holds the installation bit mask as
  its value and the firmware version as its name. Text is ISO-8859-1.
- **Firmware 2.07D differs from homeControl's assumptions**, and the catalog
  follows the controller:
  - The operating mode only goes up to 7 (Party).
  - MP 25 is the hot water priority valve, not a pump.
  - MP 83/89 are power values, and MP 88 is the inverter frequency.
  - Mixer circuit 1 has its own enable flag (SP 199).
  - The controller's own limits differ from homeControl's hard-coded ones; the
    heating limit, for example, goes to 45 °C, not 24 °C.
- **Timing:** one point read takes 100–200 ms. The multi-read `~MR` fetches 4
  values in about 130 ms. Reading all 96 measuring points takes 10 s, and all
  392 setup points 23 s.
- **Bugs in homeControl**, fixed in heliocontrol:
  - The brine and water source tabs bind the compressor and source-pump
    indicators to MP 22 (the heating pump); heliocontrol uses MP 30 / MP 24.
  - Trend end times are sent in a 12-hour clock; heliocontrol sends 24-hour
    times, which matches what the controller reports.

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
runs the Home Assistant integration tests (`tests/homeassistant`, plus
the Smart Climate tests in `tests/smart_climate`): the heliocontrol ones build
the server and the simulator, start both, and drive the integration inside a
Home Assistant test instance (needs Go, plus Python with
`pytest-homeassistant-custom-component`; Linux or macOS). `make dev`
starts the Angular dev server with a proxy to the API on :8080.

`tools/golden/golden.ps1` regenerates `internal/frame/testdata/golden.txt` from
homeControl's `DataFrame.dll` (Windows PowerShell).

## License

This project is licensed under the MIT License — see the [LICENSE](LICENSE)
file for details.

## Disclaimer

**WARNING:** This software allows you to control your heat pump. Incorrect
settings could damage your heat pump or cause it to operate inefficiently.
Use at your own risk!

The author does not provide any guarantee or warranty concerning correctness,
functionality, or performance and does not accept any liability for damage
caused by this software.
