# Smart Climate for Home Assistant

A modular heating/cooling control system built from four controller types. Each one is a separate config entry; add them from **Settings → Devices & services → Add integration → Smart Climate**.

| Controller | Count | What it does |
|---|---|---|
| **Room** | one per room | Comfort/Eco/Preheat/Protection modes, proportional heat/cool demand, PWM switching, learned heating rate, automatic shading |
| **Building** | one | Aggregates room demand (area- and occupancy-weighted), drives heating/cooling stages with hysteresis and anti-cycling |
| **Temperature** | one | Calculates flow and buffer setpoints from building demand, optionally weather compensated |
| **Mixer** | one | PI flow-temperature control for an analog (0–100 %) or three-point valve, pump control, over-temperature shutdown |

Data flows from room to building, then to temperature, then to mixer. Each controller recalculates every 30 s. Rooms also recalculate immediately when one of their input entities changes.

## Room controller

- **Modes**, by priority: Off → Protection (window open) → Comfort (manual target, presence or active schedule) → Preheat → Eco.
- **Schedule & preheat**: with a `schedule` helper, Comfort applies during its windows and Eco outside them. Before the next window the room switches to Preheat. The lead time comes from the learned heating rate. Example: 20.5 °C → 22 °C at 90 min/K means preheat starts 135 min early.
- **Learning**: during strong (≥ 75 % demand), uninterrupted heating, a `min/K` sample is recorded once the room has warmed ≥ 0.4 K over ≥ 10 min. Samples outside 5–720 min/K are discarded. The median of the last eight is used and persisted.
- **Heating + cooling**: optional cooler with its own comfort target and a neutral zone in between. Eco widens the cooling target, Protection suppresses cooling, and heating and cooling never run together.
- **Fail-safe**: if the temperature sensor or an actuator is unavailable, both outputs switch off and the room drops out of building aggregation.
- **Switch protection**: writes are idempotent. Turn-on is limited by `min_switch_seconds` (default 120 s); turn-off is always allowed.

### Shading

Optional per room: one or more covers sharing one facade orientation. Shading starts when the sun is on the facade (`sun.sun` azimuth/elevation within the tolerance) and the room is above the shading temperature. Wind alarm and open windows force covers open.

- *Maximum daylight*: cover at 15 %, slat tilt derived from the solar profile angle and the slat width/spacing.
- *Maximum cooling*: cover and slats nearly closed.
- **Manual override**: if a cover settles at a position the automation did not command, that cover is left alone until the sun leaves the facade. `smart_climate.clear_manual_override` resets it immediately.

## Changing settings

Open the controller under **Settings → Devices & services → Smart Climate → Configure**. The form has the same fields as setup, filled in with the current values. Every field except the name can be changed, including the entities used. Clearing an optional entity removes it, e.g. remove the cooler to make a room heat-only. Saving restarts that controller with the new settings; learned heating data and the room's target temperature are kept.

## Dashboard card

The integration ships a card and loads it on every dashboard automatically. No resource needs to be added. In a dashboard, *Add card → Smart Climate*, then pick a controller device. The card adapts to the controller type:

- **Room**: current/target temperature with −/+ and on/off, mode, heating/cooling demand, window, preheat, learned rate, shading.
- **Building**: plant mode, heating/cooling demand, stage 2, decision reason.
- **Temperature**: flow and buffer targets, reason.
- **Mixer**: mode, valve position, error, reason.

Tapping a value opens its entity. YAML: `type: custom:smart-climate-card`, `device: <device id>`, optional `title`.

## Services

| Service | Effect |
|---|---|
| `smart_climate.clear_manual_override` | Resets manual target temperature and cover overrides (all rooms, or one via `entry_id`) |
| `smart_climate.recalculate` | Recalculates all controllers immediately |

## Installation

Copy `custom_components/smart_climate` into your Home Assistant `custom_components/` folder and restart. Heaters, coolers, plant stages, pumps and three-point relays must be `switch` entities. Setpoints and analog valve outputs must be `number` entities.

## Code layout

```
custom_components/smart_climate/
├── controller/      pure control logic, no Home Assistant imports, fully unit tested
├── runtime/         one runtime per controller type: reads HA state, runs the controller, writes actuators
│   ├── base.py      shared lifecycle: 30 s timer, recalculation lock, entity push updates
│   ├── room.py      room loop (+ shading.py for covers)
│   └── building.py, temperature.py, mixer.py
├── config.py        typed, validated view of config entry data + options
├── hass_io.py       state reads and idempotent switch/number/cover writes
├── entity.py        shared entity base (device info, push updates)
└── climate.py, sensor.py, binary_sensor.py, config_flow.py
```

## Development

```bash
pip install -e ".[test]"
pytest
```

`tests/test_integration.py` runs the integration inside a real Home Assistant instance (via `pytest-homeassistant-custom-component`). The other tests cover the pure controllers.

## Known gaps

- `pump_overrun_minutes` can be configured but is not applied yet.
- All covers in a room share one facade geometry.

## Safety

Software control is supplemental. Keep hardware temperature limiters, frost protection and manufacturer-required interlocks independent from Home Assistant. Do not use this integration as the sole safety device of a heating or cooling system.

## Changelog

### 1.1.0: refactor + options flow

- Options flow for all four controller types (edit settings after setup without re-adding the controller).
- Runtime code split per controller type behind a common base class. Typed config objects replace scattered `data.get()` calls.
- Entities are push-updated after every control cycle instead of polled. The climate entity no longer runs its own timer.
- Recalculations are serialised, so state-change events and the timer can no longer interleave actuator writes.
- Learning data is saved with a delayed write instead of on every cycle.
- Fixed: shading's own cover moves were detected as manual overrides, and covers were re-commanded every cycle.
- Fixed: three-point mixer could briefly energise OPEN and CLOSE together when reversing.
- Services are registered once at integration setup and validate their input; `entry_id` gets a config-entry picker.
- Fixed: config entries from flow version 1 failed to load (missing migration).
- Fixed: building aggregation ignored room area changed via options.
- Mixer PI step uses real elapsed time instead of a fixed 30 s.
- Complete English/German translations for all config steps, entities, enum states and services.
- HVAC modes offered by the climate entity match the room (`heat` or `heat_cool`, plus `off`).

### 1.0.0: production hardening
Options merged into runtime config, restored room state, fail-safe on unavailable entities, idempotent actuator writes, minimum switching interval, services.

### 0.9: heating mixer controller · 0.8: room cooling, flow temperature controller · 0.7: building climate controller · 0.5: manual override, shading strategies, slat geometry, multiple covers · 0.4: shading controller · 0.3: schedule & predictive preheating
