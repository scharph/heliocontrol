"""Diagnostic sensors for all Smart Climate controller types."""

from __future__ import annotations

from collections.abc import Callable
from dataclasses import dataclass
from typing import Any

from homeassistant.components.sensor import (
    SensorDeviceClass,
    SensorEntity,
    SensorEntityDescription,
    SensorStateClass,
)
from homeassistant.const import PERCENTAGE, EntityCategory, UnitOfTemperature, UnitOfTime
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback
from homeassistant.helpers.typing import StateType

from .const import ENTRY_TYPE_BUILDING, ENTRY_TYPE_MIXER, ENTRY_TYPE_ROOM, ENTRY_TYPE_TEMPERATURE
from .controller.climate_controller import PlantMode
from .controller.mixer import MixerMode
from .controller.room import RoomMode
from .entity import SmartClimateEntity
from .runtime import SmartClimateConfigEntry


@dataclass(frozen=True, kw_only=True)
class SmartClimateSensorDescription(SensorEntityDescription):
    value_fn: Callable[[Any], StateType]


def _percent(fraction: float) -> float:
    return round(fraction * 100, 1)


def _percent_sensor(key: str, translation_key: str, value_fn: Callable[[Any], float]) -> SmartClimateSensorDescription:
    return SmartClimateSensorDescription(
        key=key,
        translation_key=translation_key,
        native_unit_of_measurement=PERCENTAGE,
        state_class=SensorStateClass.MEASUREMENT,
        suggested_display_precision=0,
        value_fn=value_fn,
    )


def _reason_sensor(key: str, translation_key: str) -> SmartClimateSensorDescription:
    return SmartClimateSensorDescription(
        key=key,
        translation_key=translation_key,
        entity_category=EntityCategory.DIAGNOSTIC,
        value_fn=lambda rt: rt.output.reason,
    )


def _flow_temperature_sensor(key: str, value_fn: Callable[[Any], float | None]) -> SmartClimateSensorDescription:
    return SmartClimateSensorDescription(
        key=key,
        translation_key=key,
        device_class=SensorDeviceClass.TEMPERATURE,
        native_unit_of_measurement=UnitOfTemperature.CELSIUS,
        state_class=SensorStateClass.MEASUREMENT,
        value_fn=value_fn,
    )


# Keys are part of each entity's unique_id and must stay stable.
ROOM_SENSORS: tuple[SmartClimateSensorDescription, ...] = (
    _percent_sensor("heating_demand", "heating_demand", lambda rt: _percent(rt.output.heating_demand)),
    _percent_sensor("cooling_demand", "cooling_demand", lambda rt: _percent(rt.output.cooling_demand)),
    SmartClimateSensorDescription(
        key="heating_rate",
        translation_key="heating_rate",
        native_unit_of_measurement="min/°C",
        suggested_display_precision=1,
        value_fn=lambda rt: rt.learned_heating_rate,
    ),
    SmartClimateSensorDescription(
        key="preheat_minutes",
        translation_key="preheat_minutes",
        device_class=SensorDeviceClass.DURATION,
        native_unit_of_measurement=UnitOfTime.MINUTES,
        suggested_display_precision=0,
        value_fn=lambda rt: round(rt.predicted_preheat_minutes, 1),
    ),
    SmartClimateSensorDescription(
        key="room_mode",
        translation_key="room_mode",
        device_class=SensorDeviceClass.ENUM,
        options=[mode.value for mode in RoomMode],
        value_fn=lambda rt: rt.mode.value,
    ),
    SmartClimateSensorDescription(
        key="shading_reason",
        translation_key="shading_reason",
        entity_category=EntityCategory.DIAGNOSTIC,
        value_fn=lambda rt: rt.shading.output.reason,
    ),
    SmartClimateSensorDescription(
        key="shading_tilt",
        translation_key="shading_tilt",
        native_unit_of_measurement=PERCENTAGE,
        value_fn=lambda rt: rt.shading.output.tilt_position,
    ),
)

BUILDING_SENSORS: tuple[SmartClimateSensorDescription, ...] = (
    SmartClimateSensorDescription(
        key="plant_mode",
        translation_key="plant_mode",
        device_class=SensorDeviceClass.ENUM,
        options=[mode.value for mode in PlantMode],
        value_fn=lambda rt: rt.output.mode.value,
    ),
    _percent_sensor("building_heat", "plant_heating_demand", lambda rt: _percent(rt.output.heating_demand)),
    _percent_sensor("building_cool", "plant_cooling_demand", lambda rt: _percent(rt.output.cooling_demand)),
    _reason_sensor("building_reason", "plant_reason"),
)

TEMPERATURE_SENSORS: tuple[SmartClimateSensorDescription, ...] = (
    _flow_temperature_sensor("flow_target", lambda rt: rt.output.flow_target),
    _flow_temperature_sensor("buffer_target", lambda rt: rt.output.buffer_target),
    _reason_sensor("temperature_reason", "temperature_reason"),
)

MIXER_SENSORS: tuple[SmartClimateSensorDescription, ...] = (
    SmartClimateSensorDescription(
        key="mixer_mode",
        translation_key="mixer_mode",
        device_class=SensorDeviceClass.ENUM,
        options=[mode.value for mode in MixerMode],
        value_fn=lambda rt: rt.output.mode.value,
    ),
    _percent_sensor("mixer_position", "mixer_position", lambda rt: round(rt.output.valve_position, 1)),
    SmartClimateSensorDescription(
        key="mixer_error",
        translation_key="mixer_error",
        native_unit_of_measurement=UnitOfTemperature.CELSIUS,
        state_class=SensorStateClass.MEASUREMENT,
        suggested_display_precision=2,
        value_fn=lambda rt: round(rt.output.error, 2),
    ),
    _reason_sensor("mixer_reason", "mixer_reason"),
)

SENSORS_BY_ENTRY_TYPE = {
    ENTRY_TYPE_ROOM: ROOM_SENSORS,
    ENTRY_TYPE_BUILDING: BUILDING_SENSORS,
    ENTRY_TYPE_TEMPERATURE: TEMPERATURE_SENSORS,
    ENTRY_TYPE_MIXER: MIXER_SENSORS,
}


async def async_setup_entry(
    hass: HomeAssistant, entry: SmartClimateConfigEntry, async_add_entities: AddConfigEntryEntitiesCallback
) -> None:
    descriptions = SENSORS_BY_ENTRY_TYPE[entry.runtime_data.entry_type]
    async_add_entities(SmartClimateSensor(entry, description) for description in descriptions)


class SmartClimateSensor(SmartClimateEntity, SensorEntity):
    entity_description: SmartClimateSensorDescription

    def __init__(self, entry: SmartClimateConfigEntry, description: SmartClimateSensorDescription) -> None:
        super().__init__(entry, description.key)
        self.entity_description = description

    @property
    def native_value(self) -> StateType:
        return self.entity_description.value_fn(self.runtime)
