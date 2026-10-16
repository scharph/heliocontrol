"""Smart Climate: modular room, building, flow-temperature and mixer control."""

from __future__ import annotations

from pathlib import Path

import voluptuous as vol
from homeassistant.const import Platform
from homeassistant.core import HomeAssistant, ServiceCall
from homeassistant.helpers import config_validation as cv
from homeassistant.helpers.typing import ConfigType
from homeassistant.loader import async_get_integration

from .config import merged_entry_data
from .const import (
    ATTR_ENTRY_ID,
    CONF_ENTRY_TYPE,
    DOMAIN,
    ENTRY_TYPE_ROOM,
    SERVICE_CLEAR_MANUAL_OVERRIDE,
    SERVICE_RECALCULATE,
)
from .runtime import ControllerRuntime, RoomRuntime, SmartClimateConfigEntry, create_runtime, iter_runtimes

PLATFORMS = [Platform.CLIMATE, Platform.SENSOR, Platform.BINARY_SENSOR]

CONFIG_SCHEMA = cv.config_entry_only_config_schema(DOMAIN)

CARD_URL = f"/{DOMAIN}/smart-climate-card.js"

_CLEAR_OVERRIDE_SCHEMA = vol.Schema({vol.Optional(ATTR_ENTRY_ID): cv.string})


async def async_setup(hass: HomeAssistant, config: ConfigType) -> bool:
    """Register integration-wide services once, independent of config entries."""

    async def clear_manual_override(call: ServiceCall) -> None:
        entry_id = call.data.get(ATTR_ENTRY_ID)
        for room in iter_runtimes(hass, RoomRuntime):
            if entry_id and room.entry_id != entry_id:
                continue
            room.clear_manual_override()
            await room.async_recalculate()

    async def recalculate(call: ServiceCall) -> None:
        for runtime in iter_runtimes(hass, ControllerRuntime):
            await runtime.async_recalculate()

    hass.services.async_register(
        DOMAIN, SERVICE_CLEAR_MANUAL_OVERRIDE, clear_manual_override, schema=_CLEAR_OVERRIDE_SCHEMA
    )
    hass.services.async_register(DOMAIN, SERVICE_RECALCULATE, recalculate)
    if "frontend" in hass.config.components:
        await _async_register_card(hass)
    return True


async def _async_register_card(hass: HomeAssistant) -> None:
    """Serve the Lovelace card and load it on every dashboard (version busts the browser cache)."""
    from homeassistant.components.frontend import add_extra_js_url
    from homeassistant.components.http import StaticPathConfig

    integration = await async_get_integration(hass, DOMAIN)
    await hass.http.async_register_static_paths(
        [StaticPathConfig(CARD_URL, str(Path(__file__).parent / "frontend" / "smart-climate-card.js"), False)]
    )
    add_extra_js_url(hass, f"{CARD_URL}?v={integration.version}")


async def async_setup_entry(hass: HomeAssistant, entry: SmartClimateConfigEntry) -> bool:
    runtime = create_runtime(hass, entry.entry_id, merged_entry_data(entry.data, entry.options))
    entry.runtime_data = runtime
    entry.async_on_unload(entry.add_update_listener(_async_reload_entry))
    await runtime.async_start()
    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)
    return True


async def async_unload_entry(hass: HomeAssistant, entry: SmartClimateConfigEntry) -> bool:
    if unloaded := await hass.config_entries.async_unload_platforms(entry, PLATFORMS):
        await entry.runtime_data.async_stop()
    return unloaded


async def async_migrate_entry(hass: HomeAssistant, entry: SmartClimateConfigEntry) -> bool:
    """v1 entries (before the building controller existed) were always rooms."""
    if entry.version > 2:
        return False  # downgrade from a newer release
    if entry.version == 1:
        hass.config_entries.async_update_entry(entry, data={CONF_ENTRY_TYPE: ENTRY_TYPE_ROOM, **entry.data}, version=2)
    return True


async def _async_reload_entry(hass: HomeAssistant, entry: SmartClimateConfigEntry) -> None:
    await hass.config_entries.async_reload(entry.entry_id)
