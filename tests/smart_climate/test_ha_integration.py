"""End-to-end tests running the integration inside a real Home Assistant instance."""

from __future__ import annotations

import pytest
from homeassistant.components.climate import HVACMode
from homeassistant.config_entries import ConfigEntryState
from homeassistant.core import HomeAssistant
from homeassistant.helpers import entity_registry as er
from pytest_homeassistant_custom_component.common import MockConfigEntry, async_mock_service

from custom_components.smart_climate.const import DOMAIN

ROOM_DATA = {
    "name": "Office",
    "entry_type": "room",
    "temperature_sensor": "sensor.office_temperature",
    "heater": "switch.office_heater",
    "window_sensors": ["binary_sensor.office_window"],
    "area_m2": 20.0,
}


@pytest.fixture
def switch_calls(hass: HomeAssistant):
    """Record switch service calls and mirror them into the state machine."""
    calls: list[tuple[str, str]] = []

    async def handle(call):
        entity_id = call.data["entity_id"]
        entity_id = entity_id[0] if isinstance(entity_id, list) else entity_id
        on = call.service == "turn_on"
        calls.append((entity_id, "on" if on else "off"))
        hass.states.async_set(entity_id, "on" if on else "off")

    hass.services.async_register("switch", "turn_on", handle)
    hass.services.async_register("switch", "turn_off", handle)
    return calls


async def _setup(hass: HomeAssistant, data: dict, unique_id: str, version: int = 2) -> MockConfigEntry:
    entry = MockConfigEntry(domain=DOMAIN, title=data["name"], data=data, unique_id=unique_id, version=version)
    entry.add_to_hass(hass)
    assert await hass.config_entries.async_setup(entry.entry_id)
    await hass.async_block_till_done()
    return entry


def _room_states(hass: HomeAssistant, temperature: str = "18.0", heater: str = "off") -> None:
    hass.states.async_set("sensor.office_temperature", temperature)
    hass.states.async_set("switch.office_heater", heater)
    hass.states.async_set("binary_sensor.office_window", "off")


async def test_room_heats_and_creates_entities(hass: HomeAssistant, switch_calls) -> None:
    _room_states(hass)
    entry = await _setup(hass, ROOM_DATA, "room_office")

    assert ("switch.office_heater", "on") in switch_calls
    climate = hass.states.get("climate.office_room_controller")
    assert climate is not None
    assert climate.state == HVACMode.HEAT  # no cooler configured
    assert climate.attributes["hvac_action"] == "heating"
    assert hass.states.get("sensor.office_heating_demand").state == "100.0"
    assert hass.states.get("sensor.office_room_mode").state == "comfort"

    # unique_ids are unchanged from v1.0.0
    registry = er.async_get(hass)
    ids = {e.unique_id for e in er.async_entries_for_config_entry(registry, entry.entry_id)}
    assert {
        f"{entry.entry_id}_{s}" for s in ("climate", "heating_demand", "heating_active", "window_protection")
    } <= ids


async def test_window_and_unavailable_sensor_fail_safe(hass: HomeAssistant, switch_calls) -> None:
    _room_states(hass)
    await _setup(hass, ROOM_DATA, "room_office")

    hass.states.async_set("binary_sensor.office_window", "on")
    await hass.async_block_till_done()
    assert hass.states.get("sensor.office_room_mode").state == "protection"
    assert hass.states.get("binary_sensor.office_window_protection").state == "on"
    assert hass.states.get("switch.office_heater").state == "off"

    hass.states.async_set("binary_sensor.office_window", "off")
    hass.states.async_set("sensor.office_temperature", "unavailable")
    await hass.async_block_till_done()
    assert hass.states.get("climate.office_room_controller").state == "unavailable"
    assert hass.states.get("switch.office_heater").state == "off"


async def test_set_temperature_off_and_persistence(hass: HomeAssistant, switch_calls, hass_storage) -> None:
    _room_states(hass, temperature="21.0")
    entry = await _setup(hass, ROOM_DATA, "room_office")

    await hass.services.async_call(
        "climate",
        "set_temperature",
        {"entity_id": "climate.office_room_controller", "temperature": 23.5},
        blocking=True,
    )
    assert hass.states.get("climate.office_room_controller").attributes["temperature"] == 23.5
    await hass.services.async_call(
        "climate", "set_hvac_mode", {"entity_id": "climate.office_room_controller", "hvac_mode": "off"}, blocking=True
    )
    assert hass.states.get("climate.office_room_controller").state == "off"
    assert hass.states.get("switch.office_heater").state == "off"

    assert await hass.config_entries.async_unload(entry.entry_id)
    stored = hass_storage[f"{DOMAIN}.{entry.entry_id}.learning"]["data"]
    assert stored["target_temperature"] == 23.5
    assert stored["enabled"] is False

    assert await hass.config_entries.async_setup(entry.entry_id)
    await hass.async_block_till_done()
    climate = hass.states.get("climate.office_room_controller")
    assert climate.state == "off"
    assert climate.attributes["temperature"] == 23.5


async def test_shading_moves_cover_once_and_respects_manual_override(hass: HomeAssistant, switch_calls) -> None:
    cover_calls = async_mock_service(hass, "cover", "set_cover_position")
    _room_states(hass, temperature="26.0")
    hass.states.async_set("sun.sun", "above_horizon", {"azimuth": 180.0, "elevation": 45.0})
    hass.states.async_set("cover.office", "open", {"current_position": 100, "supported_features": 15})
    data = {**ROOM_DATA, "shading_covers": ["cover.office"], "shading_temperature": 23.5}
    entry = await _setup(hass, data, "room_office")
    runtime = entry.runtime_data

    assert [c.data["position"] for c in cover_calls] == [15]
    assert hass.states.get("binary_sensor.office_shading_demand").state == "on"

    # The cover reaches the commanded position: not a manual override.
    hass.states.async_set("cover.office", "closing", {"current_position": 50, "supported_features": 15})
    hass.states.async_set("cover.office", "open", {"current_position": 15, "supported_features": 15})
    await hass.async_block_till_done()
    assert not runtime.shading.overrides.overridden
    assert len(cover_calls) == 1  # idempotent: already in place

    # Someone opens it by hand: automation backs off.
    hass.states.async_set("cover.office", "open", {"current_position": 80, "supported_features": 15})
    await hass.async_block_till_done()
    assert runtime.shading.overrides.overridden == {"cover.office"}
    assert len(cover_calls) == 1

    await hass.services.async_call(DOMAIN, "clear_manual_override", {}, blocking=True)
    assert not runtime.shading.overrides.overridden
    assert cover_calls[-1].data["position"] == 15


async def test_building_aggregates_rooms_and_switches_plant(hass: HomeAssistant, switch_calls) -> None:
    _room_states(hass, temperature="18.0")
    hass.states.async_set("switch.boiler", "off")
    await _setup(hass, ROOM_DATA, "room_office")
    building = await _setup(
        hass,
        {"name": "House", "entry_type": "building", "heating_stage1": "switch.boiler", "min_off_minutes": 0},
        "building_controller",
    )
    runtime = building.runtime_data
    assert runtime.output.heating_demand == 1.0
    assert runtime.output.mode == "heating"
    assert hass.states.get("switch.boiler").state == "on"
    assert hass.states.get("binary_sensor.house_heating").state == "on"
    assert hass.states.get("sensor.house_plant_mode").state == "heating"

    # An unavailable room is excluded from the aggregation.
    hass.states.async_set("sensor.office_temperature", "unavailable")
    await hass.async_block_till_done()
    await runtime.async_recalculate()
    assert runtime.output.heating_demand == 0.0


async def test_services_survive_unloading_one_of_several_entries(hass: HomeAssistant, switch_calls) -> None:
    _room_states(hass)
    hass.states.async_set("sensor.bed_temperature", "20")
    hass.states.async_set("switch.bed_heater", "off")
    office = await _setup(hass, ROOM_DATA, "room_office")
    await _setup(
        hass,
        {**ROOM_DATA, "name": "Bed", "temperature_sensor": "sensor.bed_temperature", "heater": "switch.bed_heater"},
        "room_bed",
    )
    assert await hass.config_entries.async_unload(office.entry_id)
    assert office.state is ConfigEntryState.NOT_LOADED
    assert hass.services.has_service(DOMAIN, "recalculate")
    await hass.services.async_call(DOMAIN, "recalculate", {}, blocking=True)


async def test_full_chain_and_mixer_relays_never_both_on(hass: HomeAssistant, switch_calls) -> None:
    _room_states(hass, temperature="18.0")
    hass.states.async_set("sensor.flow", "30")
    for sw in ("switch.boiler", "switch.pump", "switch.open", "switch.close"):
        hass.states.async_set(sw, "off")
    hass.states.async_set("switch.close", "on")  # valve was closing before

    await _setup(hass, ROOM_DATA, "room_office")
    await _setup(
        hass,
        {"name": "House", "entry_type": "building", "heating_stage1": "switch.boiler", "min_off_minutes": 0},
        "building_controller",
    )
    temperature = await _setup(hass, {"name": "Flow", "entry_type": "temperature_controller"}, "temperature_controller")
    mixer = await _setup(
        hass,
        {
            "name": "Mixer",
            "entry_type": "mixer_controller",
            "mixer_flow_temperature_sensor": "sensor.flow",
            "mixer_pump": "switch.pump",
            "mixer_open": "switch.open",
            "mixer_close": "switch.close",
        },
        "heating_mixer_controller",
    )
    assert temperature.runtime_data.output.flow_target == 40.0
    assert hass.states.get("sensor.flow_flow_target").state == "40.0"
    assert mixer.runtime_data.output.three_point_command == 1
    assert hass.states.get("switch.open").state == "on"
    assert hass.states.get("switch.pump").state == "on"

    hass.states.async_set("sensor.flow", "55")  # above the 50 °C safety limit
    await mixer.runtime_data.async_recalculate()
    assert hass.states.get("sensor.mixer_mixer_mode").state == "safety"
    assert hass.states.get("switch.close").state == "on"
    assert hass.states.get("switch.pump").state == "off"

    relays = {"switch.open": "off", "switch.close": "on"}
    for entity_id, state in switch_calls:
        if entity_id in relays:
            relays[entity_id] = state
            assert list(relays.values()) != ["on", "on"], "OPEN and CLOSE energised together"


async def test_version_1_room_entry_is_migrated(hass: HomeAssistant, switch_calls) -> None:
    _room_states(hass)
    legacy = {k: v for k, v in ROOM_DATA.items() if k != "entry_type"}
    entry = await _setup(hass, legacy, "room_office", version=1)
    assert entry.version == 2
    assert entry.data["entry_type"] == "room"
    assert hass.states.get("climate.office_room_controller") is not None


async def _options(hass: HomeAssistant, entry: MockConfigEntry, user_input: dict) -> dict:
    flow = await hass.config_entries.options.async_init(entry.entry_id)
    assert flow["type"] == "form" and flow["step_id"] == "init"
    result = await hass.config_entries.options.async_configure(flow["flow_id"], user_input)
    assert result["type"] == "create_entry"
    await hass.async_block_till_done()
    return flow


async def test_options_flow_prefills_and_reloads_room(hass: HomeAssistant, switch_calls) -> None:
    _room_states(hass)
    hass.states.async_set("switch.office_cooler", "off")
    data = {**ROOM_DATA, "cooler": "switch.office_cooler", "eco_temperature": 18.0, "shading_cover": "cover.legacy"}
    entry = await _setup(hass, data, "room_office")
    old_runtime = entry.runtime_data

    flow = await hass.config_entries.options.async_init(entry.entry_id)
    fields = {str(k): k for k in flow["data_schema"].schema}
    assert "name" not in fields
    assert fields["eco_temperature"].description["suggested_value"] == 18.0
    assert fields["cooler"].description["suggested_value"] == "switch.office_cooler"
    assert fields["shading_covers"].description["suggested_value"] == ["cover.legacy"]
    hass.config_entries.options.async_abort(flow["flow_id"])

    # Change eco temperature, clear the cooler and the legacy cover.
    await _options(
        hass,
        entry,
        {"temperature_sensor": "sensor.office_temperature", "heater": "switch.office_heater", "eco_temperature": 17.0},
    )
    runtime = entry.runtime_data
    assert runtime is not old_runtime  # entry was reloaded
    assert runtime.config.eco_temperature == 17.0
    assert runtime.config.cooler is None
    assert runtime.config.shading.covers == ()
    assert runtime.config.comfort_temperature == 22.0  # untouched field keeps its default
    assert hass.states.get("climate.office_room_controller").attributes["hvac_modes"] == ["heat", "off"]
    assert entry.title == "Office"


async def test_options_flow_for_building(hass: HomeAssistant, switch_calls) -> None:
    hass.states.async_set("switch.boiler", "off")
    entry = await _setup(
        hass, {"name": "House", "entry_type": "building", "heating_stage1": "switch.boiler"}, "building_controller"
    )
    await _options(hass, entry, {"heating_stage1": "switch.boiler", "start_threshold": 50, "min_run_minutes": 3})
    assert entry.runtime_data.config.start_threshold == 0.5
    assert entry.runtime_data.config.min_run_seconds == 180
