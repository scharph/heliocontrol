"""Selects: operating modes."""

from __future__ import annotations

from typing import Any

from homeassistant.components.select import SelectEntity
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
    async_add_entities(ItemSelect(coordinator, it, lang) for it in items(snap) if component(it, read_only) == "select")


class ItemSelect(ItemEntity, SelectEntity):
    def __init__(self, coordinator: HeliocontrolCoordinator, item: dict[str, Any], lang: str) -> None:
        super().__init__(coordinator, item, lang)
        offset = int(item.get("offset") or 0)
        blocked = set(item.get("notSelectable") or [])
        labels = self.option_labels()
        # the current state must be among the options even if it cannot be chosen
        self._attr_options = labels
        self._selectable = {label: i + offset for i, label in enumerate(labels) if i + offset not in blocked}

    @property
    def current_option(self) -> str | None:
        return self.option_for(self.raw())

    async def async_select_option(self, option: str) -> None:
        if option not in self._selectable:
            raise HomeAssistantError(f"'{option}' cannot be selected")
        try:
            await self.coordinator.client.write_sp(self.item["id"], self._selectable[option])
        except HeliocontrolError as err:
            raise HomeAssistantError(str(err)) from err
        await self.coordinator.async_request_refresh()
