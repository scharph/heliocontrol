"""Binary sensors: pumps, compressor, valves, fault."""

from __future__ import annotations

from typing import Any

from homeassistant.components.binary_sensor import BinarySensorDeviceClass, BinarySensorEntity
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback

from .coordinator import HeliocontrolConfigEntry, HeliocontrolCoordinator
from .entity import ValueEntity, language, of_component


async def async_setup_entry(
    hass: HomeAssistant, entry: HeliocontrolConfigEntry, async_add_entities: AddConfigEntryEntitiesCallback
) -> None:
    coordinator = entry.runtime_data
    lang = language(hass)
    async_add_entities(ValueBinarySensor(coordinator, d, lang) for d in of_component(coordinator, "binary_sensor"))


class ValueBinarySensor(ValueEntity, BinarySensorEntity):
    def __init__(self, coordinator: HeliocontrolCoordinator, desc: dict[str, Any], lang: str) -> None:
        super().__init__(coordinator, desc, lang)
        if desc.get("deviceClass"):
            try:
                self._attr_device_class = BinarySensorDeviceClass(desc["deviceClass"])
            except ValueError:
                pass

    @property
    def is_on(self) -> bool | None:
        v = self.raw()
        return None if v is None else v != 0
