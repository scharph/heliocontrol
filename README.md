# heliocontrol

A self-hosted replacement for **Heliotherm homeControl**, the discontinued
Windows tool for Heliotherm heat pumps with the *web control* controller.
heliocontrol has a Go server, an Angular web UI, and a Home Assistant
integration over MQTT.

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

## Quick start

```sh
cp heliocontrol.example.yaml heliocontrol.yaml   # set device.host
docker compose up -d                              # or: make all && bin/heliocontrol
```

Open <http://localhost:8080>. With Docker, put `heliocontrol.yaml` into `./data/`.

To try it without a heat pump:

```sh
make run-sim          # simulated controller + server, writes allowed
```

## Home Assistant

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
internal/service    session, polling, recorder, trend/time-program jobs, event bus
internal/store      SQLite (pure Go): log, point cache, recordings
internal/mqtt       Home Assistant bridge
internal/api        REST + SSE + auth, embeds the UI
internal/legacy     homeControl .htd / .hwo import
web/                Angular 22 app (zoneless, standalone, signals)
```

`make test` runs the Go tests (including end-to-end tests against the
simulator and an embedded MQTT broker) and the web unit tests. `make dev`
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
