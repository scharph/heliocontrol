"""Buttons: refresh, reset fault, sync clock."""

from __future__ import annotations

from homeassistant.components.button import ButtonEntity
from homeassistant.core import HomeAssistant
from homeassistant.exceptions import HomeAssistantError
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback

from .api import HeliocontrolError
from .coordinator import HeliocontrolConfigEntry
from .entity import DescribedEntity, language, of_component


async def async_setup_entry(
    hass: HomeAssistant, entry: HeliocontrolConfigEntry, async_add_entities: AddConfigEntryEntitiesCallback
) -> None:
    coordinator = entry.runtime_data
    lang = language(hass)
    async_add_entities(ActionButton(coordinator, d, lang) for d in of_component(coordinator, "button"))


class ActionButton(DescribedEntity, ButtonEntity):
    async def async_press(self) -> None:
        client = self.coordinator.client
        actions = {"refresh": client.refresh, "reset_fault": client.reset_fault, "sync_clock": client.sync_clock}
        action = actions.get(self.desc.get("action", ""))
        if action is None:
            raise HomeAssistantError(f"unknown action {self.desc.get('action')}")
        try:
            await action()
        except HeliocontrolError as err:
            raise HomeAssistantError(str(err)) from err
        await self.coordinator.async_request_refresh()
