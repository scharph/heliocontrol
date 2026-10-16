"""Selects: operating modes."""

from __future__ import annotations

from typing import Any

from homeassistant.components.select import SelectEntity
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
    async_add_entities(ValueSelect(coordinator, d, lang) for d in of_component(coordinator, "select"))


class ValueSelect(ValueEntity, SelectEntity):
    def __init__(self, coordinator: HeliocontrolCoordinator, desc: dict[str, Any], lang: str) -> None:
        super().__init__(coordinator, desc, lang)
        offset = int(desc.get("offset") or 0)
        labels = self.option_labels()
        # the current state must be among the options even if it cannot be chosen
        self._attr_options = labels
        allowed = set(desc.get("selectable") or [])
        self.values = {lbl: i + offset for i, lbl in enumerate(labels) if i + offset in allowed}

    @property
    def current_option(self) -> str | None:
        return self.option_for(self.raw())

    def value_of(self, option: str) -> int:
        if option not in self.values:
            raise HomeAssistantError(f"'{option}' cannot be selected")
        return self.values[option]

    async def async_select_option(self, option: str) -> None:
        try:
            await self.coordinator.client.write_sp(self.desc["id"], self.value_of(option))
        except HeliocontrolError as err:
            raise HomeAssistantError(str(err)) from err
        await self.coordinator.async_request_refresh()
