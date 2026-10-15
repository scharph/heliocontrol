"""Binary sensors: pumps, compressor, valves, fault."""

from __future__ import annotations

from typing import Any

from homeassistant.components.binary_sensor import BinarySensorDeviceClass, BinarySensorEntity
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback

from .coordinator import HeliocontrolConfigEntry, HeliocontrolCoordinator
from .entity import ItemEntity, component, items


async def async_setup_entry(
    hass: HomeAssistant, entry: HeliocontrolConfigEntry, async_add_entities: AddConfigEntryEntitiesCallback
) -> None:
    coordinator = entry.runtime_data
    snap = coordinator.data or {}
    read_only = bool(snap.get("readOnly", True))
    lang = "de" if (hass.config.language or "").startswith("de") else "en"
    async_add_entities(
        ItemBinarySensor(coordinator, it, lang) for it in items(snap) if component(it, read_only) == "binary_sensor"
    )


class ItemBinarySensor(ItemEntity, BinarySensorEntity):
    def __init__(self, coordinator: HeliocontrolCoordinator, item: dict[str, Any], lang: str) -> None:
        super().__init__(coordinator, item, lang)
        device_class = (item.get("ha") or {}).get("deviceClass")
        if device_class:
            try:
                self._attr_device_class = BinarySensorDeviceClass(device_class)
            except ValueError:
                pass

    @property
    def is_on(self) -> bool | None:
        v = self.raw()
        return None if v is None else v != 0
