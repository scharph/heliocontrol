"""Numbers: room targets, heating curve, hot water temperatures."""

from __future__ import annotations

from typing import Any

from homeassistant.components.number import NumberDeviceClass, NumberEntity, NumberMode
from homeassistant.const import UnitOfTemperature
from homeassistant.core import HomeAssistant
from homeassistant.exceptions import HomeAssistantError
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback

from .api import HeliocontrolError
from .coordinator import HeliocontrolConfigEntry, HeliocontrolCoordinator
from .entity import ValueEntity, language, of_component


async def async_setup_entry(
    hass: HomeAssistant, entry: HeliocontrolConfigEntry, async_add_entities: AddConfigEntryEntitiesCallback
) -> None:
    coordinator = entry.runtime_data
    lang = language(hass)
    async_add_entities(ValueNumber(coordinator, d, lang) for d in of_component(coordinator, "number"))


class ValueNumber(ValueEntity, NumberEntity):
    _attr_mode = NumberMode.BOX

    def __init__(self, coordinator: HeliocontrolCoordinator, desc: dict[str, Any], lang: str) -> None:
        super().__init__(coordinator, desc, lang)
        self._attr_native_min_value = float(desc.get("min") or 0)
        self._attr_native_max_value = float(desc.get("max") or 0)
        self._attr_native_step = float(desc.get("step") or 1)
        if desc.get("deviceClass") == "temperature":
            self._attr_device_class = NumberDeviceClass.TEMPERATURE
            self._attr_native_unit_of_measurement = UnitOfTemperature.CELSIUS
        else:
            self._attr_native_unit_of_measurement = desc.get("unit") or None

    @property
    def native_value(self) -> float | None:
        return self.raw()

    async def async_set_native_value(self, value: float) -> None:
        try:
            await self.coordinator.client.write_sp(self.desc["id"], value)
        except HeliocontrolError as err:
            raise HomeAssistantError(str(err)) from err
        await self.coordinator.async_request_refresh()
