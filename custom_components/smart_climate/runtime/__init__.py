"""Home Assistant-facing runtimes, one per config entry type."""

from __future__ import annotations

from collections.abc import Mapping
from typing import Any

from homeassistant.core import HomeAssistant

from ..config import BuildingConfig, MixerConfig, RoomConfig, TemperatureConfig
from ..const import CONF_ENTRY_TYPE, ENTRY_TYPE_BUILDING, ENTRY_TYPE_MIXER, ENTRY_TYPE_ROOM, ENTRY_TYPE_TEMPERATURE
from .base import ControllerRuntime, SmartClimateConfigEntry, find_runtime, iter_runtimes
from .building import BuildingRuntime
from .mixer import MixerRuntime
from .room import RoomRuntime
from .temperature import TemperatureRuntime

__all__ = [
    "BuildingRuntime",
    "ControllerRuntime",
    "MixerRuntime",
    "RoomRuntime",
    "SmartClimateConfigEntry",
    "TemperatureRuntime",
    "create_runtime",
    "find_runtime",
    "iter_runtimes",
]


_FACTORIES = {
    ENTRY_TYPE_ROOM: (RoomRuntime, RoomConfig),
    ENTRY_TYPE_BUILDING: (BuildingRuntime, BuildingConfig),
    ENTRY_TYPE_TEMPERATURE: (TemperatureRuntime, TemperatureConfig),
    ENTRY_TYPE_MIXER: (MixerRuntime, MixerConfig),
}


def create_runtime(hass: HomeAssistant, entry_id: str, data: Mapping[str, Any]) -> ControllerRuntime:
    """Build the runtime for a config entry. Entries without a type are rooms (pre-V0.7)."""
    runtime_cls, config_cls = _FACTORIES[data.get(CONF_ENTRY_TYPE, ENTRY_TYPE_ROOM)]
    return runtime_cls(hass, entry_id, config_cls.from_mapping(data))
