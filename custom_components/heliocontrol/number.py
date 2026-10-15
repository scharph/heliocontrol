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
from .entity import ItemEntity, component, items


async def async_setup_entry(
    hass: HomeAssistant, entry: HeliocontrolConfigEntry, async_add_entities: AddConfigEntryEntitiesCallback
) -> None:
    coordinator = entry.runtime_data
    snap = coordinator.data or {}
    read_only = bool(snap.get("readOnly", True))
    lang = "de" if (hass.config.language or "").startswith("de") else "en"
    async_add_entities(ItemNumber(coordinator, it, lang) for it in items(snap) if component(it, read_only) == "number")


class ItemNumber(ItemEntity, NumberEntity):
    _attr_mode = NumberMode.BOX

    def __init__(self, coordinator: HeliocontrolCoordinator, item: dict[str, Any], lang: str) -> None:
        super().__init__(coordinator, item, lang)
        self._attr_native_min_value = float(item.get("min") or 0)
        self._attr_native_max_value = float(item.get("max") or 0)
        self._attr_native_step = float(item.get("step") or 1)
        unit = item.get("unit") or None
        if unit == "°C":
            self._attr_device_class = NumberDeviceClass.TEMPERATURE
            self._attr_native_unit_of_measurement = UnitOfTemperature.CELSIUS
        else:
            self._attr_native_unit_of_measurement = unit

    @property
    def native_value(self) -> float | None:
        return self.raw()

    async def async_set_native_value(self, value: float) -> None:
        try:
            await self.coordinator.client.write_sp(self.item["id"], value)
        except HeliocontrolError as err:
            raise HomeAssistantError(str(err)) from err
        await self.coordinator.async_request_refresh()
