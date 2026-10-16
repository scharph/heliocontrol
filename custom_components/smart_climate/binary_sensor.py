"""Binary diagnostic sensors for rooms and the building controller."""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from typing import Any

from homeassistant.components.binary_sensor import BinarySensorEntity, BinarySensorEntityDescription
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback

from .const import ENTRY_TYPE_BUILDING, ENTRY_TYPE_ROOM
from .controller.climate_controller import PlantMode
from .entity import SmartClimateEntity
from .runtime import SmartClimateConfigEntry


@dataclass(frozen=True, kw_only=True)
class SmartClimateBinarySensorDescription(BinarySensorEntityDescription):
    is_on_fn: Callable[[Any], bool]


# Keys are part of each entity's unique_id and must stay stable.
ROOM_BINARY_SENSORS: tuple[SmartClimateBinarySensorDescription, ...] = (
    SmartClimateBinarySensorDescription(
        key="heating_active", translation_key="heating_active", is_on_fn=lambda rt: rt.output.heating
    ),
    SmartClimateBinarySensorDescription(
        key="cooling_active", translation_key="cooling_active", is_on_fn=lambda rt: rt.output.cooling
    ),
    SmartClimateBinarySensorDescription(
        key="window_protection", translation_key="window_protection", is_on_fn=lambda rt: rt.inputs.window_open
    ),
    SmartClimateBinarySensorDescription(
        key="shading_demand", translation_key="shading_demand", is_on_fn=lambda rt: rt.shading.output.demand
    ),
    SmartClimateBinarySensorDescription(
        key="sun_on_window", translation_key="sun_on_window", is_on_fn=lambda rt: rt.shading.output.sun_on_window
    ),
)

BUILDING_BINARY_SENSORS: tuple[SmartClimateBinarySensorDescription, ...] = (
    SmartClimateBinarySensorDescription(
        key="plant_heating", translation_key="plant_heating", is_on_fn=lambda rt: rt.output.mode == PlantMode.HEATING
    ),
    SmartClimateBinarySensorDescription(
        key="plant_cooling", translation_key="plant_cooling", is_on_fn=lambda rt: rt.output.mode == PlantMode.COOLING
    ),
    SmartClimateBinarySensorDescription(
        key="plant_stage2", translation_key="plant_stage2", is_on_fn=lambda rt: rt.output.stage2
    ),
)

BINARY_SENSORS_BY_ENTRY_TYPE = {
    ENTRY_TYPE_ROOM: ROOM_BINARY_SENSORS,
    ENTRY_TYPE_BUILDING: BUILDING_BINARY_SENSORS,
}


async def async_setup_entry(
    hass: HomeAssistant, entry: SmartClimateConfigEntry, async_add_entities: AddConfigEntryEntitiesCallback
) -> None:
    descriptions = BINARY_SENSORS_BY_ENTRY_TYPE.get(entry.runtime_data.entry_type, ())
    async_add_entities(SmartClimateBinarySensor(entry, description) for description in descriptions)


class SmartClimateBinarySensor(SmartClimateEntity, BinarySensorEntity):
    entity_description: SmartClimateBinarySensorDescription

    def __init__(self, entry: SmartClimateConfigEntry, description: SmartClimateBinarySensorDescription) -> None:
        super().__init__(entry, description.key)
        self.entity_description = description

    @property
    def is_on(self) -> bool:
        return self.entity_description.is_on_fn(self.runtime)
