"""Sensors: temperatures, pressures, energy, states and other values."""

from __future__ import annotations

from typing import Any

from homeassistant.components.sensor import SensorDeviceClass, SensorEntity, SensorStateClass
from homeassistant.const import (
    PERCENTAGE,
    EntityCategory,
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
from .entity import HeliocontrolEntity, ItemEntity, component, items

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
    snap = coordinator.data or {}
    read_only = bool(snap.get("readOnly", True))
    lang = "de" if (hass.config.language or "").startswith("de") else "en"
    entities: list[SensorEntity] = [
        ItemSensor(coordinator, it, lang) for it in items(snap) if component(it, read_only) == "sensor"
    ]
    entities.append(FirmwareSensor(coordinator))
    async_add_entities(entities)


class ItemSensor(ItemEntity, SensorEntity):
    """A page value."""

    def __init__(self, coordinator: HeliocontrolCoordinator, item: dict[str, Any], lang: str) -> None:
        super().__init__(coordinator, item, lang)
        ha = item.get("ha") or {}
        display = item.get("display")
        unit = item.get("unit") or ""
        if display == "enum":
            self._attr_device_class = SensorDeviceClass.ENUM
            self._attr_options = self.option_labels()
            return
        self._attr_native_unit_of_measurement = UNITS.get(unit, unit or None)
        device_class = ha.get("deviceClass")
        # offsets in K are temperature differences, not temperatures
        if device_class and device_class != "enum" and not (device_class == "temperature" and unit != "°C"):
            try:
                self._attr_device_class = SensorDeviceClass(device_class)
            except ValueError:
                pass
        if ha.get("stateClass"):
            try:
                self._attr_state_class = SensorStateClass(ha["stateClass"])
            except ValueError:
                pass
        if display == "ratio":
            self._attr_suggested_display_precision = 2

    @property
    def native_value(self) -> float | str | None:
        v = self.raw()
        if self.item.get("display") == "enum":
            return self.option_for(v)
        return v


class FirmwareSensor(HeliocontrolEntity, SensorEntity):
    """Controller firmware version."""

    _attr_entity_category = EntityCategory.DIAGNOSTIC
    _attr_icon = "mdi:chip"
    _attr_translation_key = "firmware"

    def __init__(self, coordinator: HeliocontrolCoordinator) -> None:
        super().__init__(coordinator, "firmware", None)

    @property
    def native_value(self) -> str | None:
        return (self.coordinator.data or {}).get("firmware") or None
