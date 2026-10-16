"""Sensors: temperatures, pressures, energy, states, firmware."""

from __future__ import annotations

from typing import Any

from homeassistant.components.sensor import SensorDeviceClass, SensorEntity, SensorStateClass
from homeassistant.const import (
    PERCENTAGE,
    UnitOfElectricCurrent,
    UnitOfElectricPotential,
    UnitOfEnergy,
    UnitOfFrequency,
    UnitOfPower,
    UnitOfPressure,
    UnitOfTemperature,
    UnitOfTime,
)
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback

from .coordinator import HeliocontrolConfigEntry, HeliocontrolCoordinator
from .entity import DescribedEntity, ValueEntity, language, of_component

UNITS = {
    "°C": UnitOfTemperature.CELSIUS,
    "bar": UnitOfPressure.BAR,
    "kWh": UnitOfEnergy.KILO_WATT_HOUR,
    "W": UnitOfPower.WATT,
    "kW": UnitOfPower.KILO_WATT,
    "V": UnitOfElectricPotential.VOLT,
    "A": UnitOfElectricCurrent.AMPERE,
    "Hz": UnitOfFrequency.HERTZ,
    "h": UnitOfTime.HOURS,
    "s": UnitOfTime.SECONDS,
    "%": PERCENTAGE,
}


async def async_setup_entry(
    hass: HomeAssistant, entry: HeliocontrolConfigEntry, async_add_entities: AddConfigEntryEntitiesCallback
) -> None:
    coordinator = entry.runtime_data
    lang = language(hass)
    entities: list[SensorEntity] = []
    for desc in of_component(coordinator, "sensor"):
        if desc.get("display") == "firmware":
            entities.append(FirmwareSensor(coordinator, desc, lang))
        else:
            entities.append(ValueSensor(coordinator, desc, lang))
    async_add_entities(entities)


class ValueSensor(ValueEntity, SensorEntity):
    def __init__(self, coordinator: HeliocontrolCoordinator, desc: dict[str, Any], lang: str) -> None:
        super().__init__(coordinator, desc, lang)
        if desc.get("display") == "enum":
            self._attr_device_class = SensorDeviceClass.ENUM
            self._attr_options = self.option_labels()
            return
        unit = desc.get("unit")
        if unit:
            self._attr_native_unit_of_measurement = UNITS.get(unit, unit)
        if desc.get("deviceClass"):
            try:
                self._attr_device_class = SensorDeviceClass(desc["deviceClass"])
            except ValueError:
                pass
        if desc.get("stateClass"):
            try:
                self._attr_state_class = SensorStateClass(desc["stateClass"])
            except ValueError:
                pass
        if desc.get("precision"):
            self._attr_suggested_display_precision = desc["precision"]

    @property
    def native_value(self) -> float | str | None:
        v = self.raw()
        return self.option_for(v) if self.desc.get("display") == "enum" else v


class FirmwareSensor(DescribedEntity, SensorEntity):
    @property
    def native_value(self) -> str | None:
        return (self.coordinator.data or {}).get("firmware") or None
