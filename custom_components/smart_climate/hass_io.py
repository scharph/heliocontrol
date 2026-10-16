"""Thin, idempotent read/write helpers on top of the Home Assistant state machine."""

from __future__ import annotations

from datetime import UTC, datetime

from homeassistant.components.cover import ATTR_CURRENT_POSITION, ATTR_CURRENT_TILT_POSITION, CoverEntityFeature
from homeassistant.const import ATTR_ENTITY_ID, STATE_ON, STATE_UNAVAILABLE, STATE_UNKNOWN
from homeassistant.core import HomeAssistant
from homeassistant.util import dt as dt_util

_INVALID_STATES = frozenset({STATE_UNAVAILABLE, STATE_UNKNOWN, "none", "None", ""})
_NUMBER_TOLERANCE = 0.05


def is_available(hass: HomeAssistant, entity_id: str | None) -> bool:
    if not entity_id:
        return False
    state = hass.states.get(entity_id)
    return state is not None and state.state not in _INVALID_STATES


def is_on(hass: HomeAssistant, entity_id: str | None) -> bool:
    if not entity_id:
        return False
    state = hass.states.get(entity_id)
    return state is not None and state.state == STATE_ON


def any_on(hass: HomeAssistant, entity_ids: tuple[str, ...] | list[str]) -> bool:
    return any(is_on(hass, entity_id) for entity_id in entity_ids)


def state_float(hass: HomeAssistant, entity_id: str | None) -> float | None:
    if not entity_id:
        return None
    state = hass.states.get(entity_id)
    if state is None:
        return None
    try:
        return float(state.state)
    except (TypeError, ValueError):
        return None


def attribute_float(hass: HomeAssistant, entity_id: str, attribute: str) -> float | None:
    state = hass.states.get(entity_id)
    if state is None:
        return None
    try:
        return float(state.attributes[attribute])
    except (KeyError, TypeError, ValueError):
        return None


def attribute_datetime(hass: HomeAssistant, entity_id: str, attribute: str) -> datetime | None:
    state = hass.states.get(entity_id)
    value = state.attributes.get(attribute) if state is not None else None
    if not value:
        return None
    parsed = value if isinstance(value, datetime) else dt_util.parse_datetime(str(value))
    if parsed is not None and parsed.tzinfo is None:
        parsed = parsed.replace(tzinfo=UTC)
    return parsed


def sun_position(hass: HomeAssistant) -> tuple[float, float] | None:
    """Return ``(azimuth, elevation)`` from ``sun.sun``."""
    azimuth = attribute_float(hass, "sun.sun", "azimuth")
    elevation = attribute_float(hass, "sun.sun", "elevation")
    if azimuth is None or elevation is None:
        return None
    return azimuth, elevation


async def set_switch(hass: HomeAssistant, entity_id: str | None, on: bool) -> bool:
    """Switch an entity on/off unless it already is. Returns True if a call was made."""
    if not entity_id:
        return False
    state = hass.states.get(entity_id)
    if state is not None and state.state == ("on" if on else "off"):
        return False
    await hass.services.async_call(
        "switch", "turn_on" if on else "turn_off", {ATTR_ENTITY_ID: entity_id}, blocking=True
    )
    return True


async def set_number(hass: HomeAssistant, entity_id: str | None, value: float | None) -> None:
    """Write a ``number`` value unless it is already (nearly) set."""
    if not entity_id or value is None:
        return
    value = round(value, 1)
    current = state_float(hass, entity_id)
    if current is not None and abs(current - value) < _NUMBER_TOLERANCE:
        return
    await hass.services.async_call("number", "set_value", {ATTR_ENTITY_ID: entity_id, "value": value}, blocking=True)


async def set_cover(hass: HomeAssistant, entity_id: str, position: int, tilt: int | None) -> None:
    """Move a cover (and its slats if supported) unless already in place."""
    state = hass.states.get(entity_id)
    if state is None:
        return
    if state.attributes.get(ATTR_CURRENT_POSITION) != position:
        await hass.services.async_call(
            "cover", "set_cover_position", {ATTR_ENTITY_ID: entity_id, "position": position}, blocking=True
        )
    features = int(state.attributes.get("supported_features", 0))
    if (
        tilt is not None
        and features & CoverEntityFeature.SET_TILT_POSITION
        and state.attributes.get(ATTR_CURRENT_TILT_POSITION) != tilt
    ):
        await hass.services.async_call(
            "cover", "set_cover_tilt_position", {ATTR_ENTITY_ID: entity_id, "tilt_position": tilt}, blocking=True
        )
