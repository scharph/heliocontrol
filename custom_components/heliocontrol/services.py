"""Services: temporary setpoints that heliocontrol restores automatically."""

from __future__ import annotations

from typing import Any

import voluptuous as vol

from homeassistant.config_entries import ConfigEntryState
from homeassistant.const import ATTR_ENTITY_ID
from homeassistant.core import HomeAssistant, ServiceCall, callback
from homeassistant.exceptions import HomeAssistantError, ServiceValidationError
from homeassistant.helpers import config_validation as cv, entity_registry as er

from .api import HeliocontrolError
from .const import DOMAIN

SERVICE_SET_TEMPORARY = "set_temporary"
SERVICE_CANCEL_TEMPORARY = "cancel_temporary"
ATTR_VALUE = "value"
ATTR_DURATION = "duration"

SET_TEMPORARY_SCHEMA = vol.Schema(
    {
        vol.Required(ATTR_ENTITY_ID): cv.entity_ids,
        vol.Required(ATTR_VALUE): vol.Any(vol.Coerce(float), cv.string),
        vol.Required(ATTR_DURATION): vol.All(vol.Coerce(float), vol.Range(min=1, max=1440)),
    }
)
CANCEL_TEMPORARY_SCHEMA = vol.Schema({vol.Required(ATTR_ENTITY_ID): cv.entity_ids})


def _resolve(hass: HomeAssistant, entity_id: str) -> tuple[Any, dict[str, Any]]:
    """Map an entity of this integration to its coordinator and description."""
    entry = er.async_get(hass).async_get(entity_id)
    if entry is None or entry.platform != DOMAIN or not entry.config_entry_id:
        raise ServiceValidationError(f"{entity_id} is not a heliocontrol entity")
    config_entry = hass.config_entries.async_get_entry(entry.config_entry_id)
    if config_entry is None or config_entry.state is not ConfigEntryState.LOADED:
        raise ServiceValidationError(f"{entity_id}: the heliocontrol server is not loaded")
    coordinator = config_entry.runtime_data
    key = entry.unique_id.removeprefix(f"{config_entry.unique_id}_")
    desc = next((d for d in coordinator.entities if d["key"] == key), None)
    if desc is None or desc.get("kind") != "sp":
        raise ServiceValidationError(f"{entity_id} is not a setting that can be changed temporarily")
    return coordinator, desc


def _value(desc: dict[str, Any], value: float | str) -> float:
    """Numbers pass through; for modes an option label (German or English) is accepted."""
    if isinstance(value, (int, float)):
        return float(value)
    for i, opt in enumerate(desc.get("options") or []):
        if value in (opt.get("de"), opt.get("en")):
            return float(i + int(desc.get("offset") or 0))
    try:
        return float(value)
    except ValueError as err:
        raise ServiceValidationError(f"unknown value {value!r}") from err


@callback
def async_register_services(hass: HomeAssistant) -> None:
    async def set_temporary(call: ServiceCall) -> None:
        for entity_id in call.data[ATTR_ENTITY_ID]:
            coordinator, desc = _resolve(hass, entity_id)
            try:
                await coordinator.client.set_temporary(
                    desc["id"], _value(desc, call.data[ATTR_VALUE]), call.data[ATTR_DURATION], source="Home Assistant"
                )
            except HeliocontrolError as err:
                raise HomeAssistantError(f"{entity_id}: {err}") from err
            await coordinator.async_request_refresh()

    async def cancel_temporary(call: ServiceCall) -> None:
        for entity_id in call.data[ATTR_ENTITY_ID]:
            coordinator, desc = _resolve(hass, entity_id)
            try:
                await coordinator.client.cancel_temporary(desc["id"])
            except HeliocontrolError as err:
                raise HomeAssistantError(f"{entity_id}: {err}") from err
            await coordinator.async_request_refresh()

    hass.services.async_register(DOMAIN, SERVICE_SET_TEMPORARY, set_temporary, schema=SET_TEMPORARY_SCHEMA)
    hass.services.async_register(DOMAIN, SERVICE_CANCEL_TEMPORARY, cancel_temporary, schema=CANCEL_TEMPORARY_SCHEMA)
