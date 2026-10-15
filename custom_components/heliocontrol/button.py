"""Buttons: refresh, reset fault, sync clock."""

from __future__ import annotations

from collections.abc import Awaitable, Callable
from dataclasses import dataclass

from homeassistant.components.button import ButtonEntity, ButtonEntityDescription
from homeassistant.const import EntityCategory
from homeassistant.core import HomeAssistant
from homeassistant.exceptions import HomeAssistantError
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback

from .api import HeliocontrolClient, HeliocontrolError
from .coordinator import HeliocontrolConfigEntry, HeliocontrolCoordinator
from .entity import HeliocontrolEntity


@dataclass(frozen=True, kw_only=True)
class HeliocontrolButtonDescription(ButtonEntityDescription):
    press: Callable[[HeliocontrolClient], Awaitable[None]]
    writes: bool  # changes the heat pump (not offered in read-only mode)


BUTTONS = (
    HeliocontrolButtonDescription(
        key="refresh", translation_key="refresh", icon="mdi:refresh",
        entity_category=EntityCategory.DIAGNOSTIC, press=lambda c: c.refresh(), writes=False,
    ),
    HeliocontrolButtonDescription(
        key="reset_fault", translation_key="reset_fault", icon="mdi:alert-remove",
        press=lambda c: c.reset_fault(), writes=True,
    ),
    HeliocontrolButtonDescription(
        key="sync_clock", translation_key="sync_clock", icon="mdi:clock-check",
        entity_category=EntityCategory.CONFIG, press=lambda c: c.sync_clock(), writes=True,
    ),
)


async def async_setup_entry(
    hass: HomeAssistant, entry: HeliocontrolConfigEntry, async_add_entities: AddConfigEntryEntitiesCallback
) -> None:
    coordinator = entry.runtime_data
    read_only = bool((coordinator.data or {}).get("readOnly", True))
    async_add_entities(HeliocontrolButton(coordinator, d) for d in BUTTONS if not (read_only and d.writes))


class HeliocontrolButton(HeliocontrolEntity, ButtonEntity):
    entity_description: HeliocontrolButtonDescription

    def __init__(self, coordinator: HeliocontrolCoordinator, description: HeliocontrolButtonDescription) -> None:
        super().__init__(coordinator, description.key, None)
        self.entity_description = description

    async def async_press(self) -> None:
        try:
            await self.entity_description.press(self.coordinator.client)
        except HeliocontrolError as err:
            raise HomeAssistantError(str(err)) from err
        await self.coordinator.async_request_refresh()
