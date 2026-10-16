"""The heliocontrol custom integration against a real heliocontrol server."""

from __future__ import annotations

import asyncio

from homeassistant import config_entries
from homeassistant.const import CONF_PASSWORD
from homeassistant.core import HomeAssistant
from homeassistant.data_entry_flow import FlowResultType
from homeassistant.helpers import entity_registry as er
import pytest

from custom_components.heliocontrol.const import CONF_URL, DOMAIN


async def _setup(hass: HomeAssistant, url: str, password: str | None = None):
    result = await hass.config_entries.flow.async_init(DOMAIN, context={"source": config_entries.SOURCE_USER})
    assert result["type"] is FlowResultType.FORM
    data = {CONF_URL: url}
    if password:
        data[CONF_PASSWORD] = password
    result = await hass.config_entries.flow.async_configure(result["flow_id"], data)
    await hass.async_block_till_done()
    return result


def _entity(hass: HomeAssistant, platform: str, key: str) -> str | None:
    reg = er.async_get(hass)
    return reg.async_get_entity_id(platform, DOMAIN, f"123456_{key}")


async def _wait(cond, timeout=15.0):
    for _ in range(int(timeout / 0.1)):
        if cond():
            return
        await asyncio.sleep(0.1)
    raise AssertionError("condition not met")


async def test_read_only_setup(hass: HomeAssistant, server) -> None:
    result = await _setup(hass, server.url.removeprefix("http://"))  # scheme is optional
    assert result["type"] is FlowResultType.CREATE_ENTRY
    assert result["title"] == "Heliotherm 123456"
    entry = result["result"]
    assert entry.unique_id == "123456"
    assert entry.data[CONF_URL] == server.url

    outdoor = _entity(hass, "sensor", "outdoor")
    assert outdoor
    state = hass.states.get(outdoor)
    assert state.attributes["unit_of_measurement"] == "°C"
    assert state.attributes["device_class"] == "temperature"
    float(state.state)

    # read-only server: controls are plain sensors, write buttons are omitted
    assert _entity(hass, "select", "mode") is None
    mode = hass.states.get(_entity(hass, "sensor", "mode"))
    assert mode.state == "Automatic"
    assert "Main switch" in mode.attributes["options"]
    assert _entity(hass, "number", "room_target") is None
    assert _entity(hass, "button", "refresh")
    assert _entity(hass, "button", "reset_fault") is None

    pump = hass.states.get(_entity(hass, "binary_sensor", "heating_pump"))
    assert pump.state in ("on", "off")
    assert hass.states.get(_entity(hass, "sensor", "firmware")).state == "2.07D"
    assert hass.states.get(_entity(hass, "sensor", "cop_heating")).state not in ("unknown", "unavailable")

    # second setup of the same heat pump is refused
    again = await _setup(hass, server.url)
    assert again["type"] is FlowResultType.ABORT
    assert again["reason"] == "already_configured"

    assert await hass.config_entries.async_unload(entry.entry_id)


async def test_writes_and_live_updates(hass: HomeAssistant, writable_server) -> None:
    result = await _setup(hass, writable_server.url)
    entry = result["result"]

    mode = _entity(hass, "select", "mode")
    assert mode, "writable server exposes the mode as a select"
    await hass.services.async_call("select", "select_option", {"entity_id": mode, "option": "Summer"}, blocking=True)
    assert writable_server.get("/api/state")["sp"]["13"] == 3
    await _wait(lambda: hass.states.get(mode).state == "Summer")

    room = _entity(hass, "number", "room_target")
    await hass.services.async_call("number", "set_value", {"entity_id": room, "value": 21.5}, blocking=True)
    assert writable_server.get("/api/state")["sp"]["69"] == 21.5
    await _wait(lambda: hass.states.get(room).state == "21.5")

    # a value changed outside Home Assistant arrives through the event stream
    writable_server.post("/api/sp/69", {"value": 22}, method="PUT")
    await _wait(lambda: float(hass.states.get(room).state) == 22)

    # refused writes surface as errors
    with pytest.raises(Exception, match="cannot be selected"):
        await hass.services.async_call("select", "select_option", {"entity_id": mode, "option": "Main switch"}, blocking=True)

    # temporary setpoints: changed now, restored by heliocontrol later (or on cancel)
    dhw = _entity(hass, "number", "dhw_target")
    await hass.services.async_call(DOMAIN, "set_temporary", {"entity_id": dhw, "value": 55, "duration": 30}, blocking=True)
    overrides = writable_server.get("/api/overrides")
    assert [(o["sp"], o["value"], o["original"]) for o in overrides] == [(83, 55, 48)]
    await hass.services.async_call(DOMAIN, "cancel_temporary", {"entity_id": dhw}, blocking=True)
    assert writable_server.get("/api/overrides") == []
    assert writable_server.get("/api/state")["sp"]["83"] == 48
    await hass.services.async_call(DOMAIN, "set_temporary", {"entity_id": mode, "value": "Party", "duration": 60}, blocking=True)
    assert writable_server.get("/api/state")["sp"]["13"] == 7
    with pytest.raises(Exception, match="not a heliocontrol entity|not a setting"):
        await hass.services.async_call(
            DOMAIN, "set_temporary", {"entity_id": _entity(hass, "sensor", "outdoor"), "value": 1, "duration": 5}, blocking=True
        )

    assert _entity(hass, "button", "reset_fault")
    # readable, distinct entity ids (translation keys, mixer circuit prefix)
    assert _entity(hass, "button", "refresh") == "button.heliotherm_refresh"
    assert _entity(hass, "button", "reset_fault") == "button.heliotherm_reset_fault"
    assert _entity(hass, "sensor", "firmware") == "sensor.heliotherm_firmware"
    assert mode == "select.heliotherm_operating_mode"
    assert _entity(hass, "select", "mixer1_mode") == "select.heliotherm_mixer_circuit_1_operating_mode"
    assert await hass.config_entries.async_unload(entry.entry_id)


async def test_password_protected_server(hass: HomeAssistant, server) -> None:
    server.post("/api/settings/password", {"new": "geheim123"})
    result = await hass.config_entries.flow.async_init(DOMAIN, context={"source": config_entries.SOURCE_USER})
    result = await hass.config_entries.flow.async_configure(result["flow_id"], {CONF_URL: server.url})
    assert result["type"] is FlowResultType.FORM
    assert result["errors"] == {"base": "invalid_auth"}
    result = await hass.config_entries.flow.async_configure(
        result["flow_id"], {CONF_URL: server.url, CONF_PASSWORD: "geheim123"}
    )
    await hass.async_block_till_done()
    assert result["type"] is FlowResultType.CREATE_ENTRY
    assert hass.states.get(_entity(hass, "sensor", "outdoor")).state not in ("unknown", "unavailable")
    assert await hass.config_entries.async_unload(result["result"].entry_id)


async def test_unreachable_server(hass: HomeAssistant) -> None:
    result = await _setup(hass, "http://127.0.0.1:9")
    assert result["type"] is FlowResultType.FORM
    assert result["errors"] == {"base": "cannot_connect"}
