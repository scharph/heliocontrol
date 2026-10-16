"""Base entity shared by all Smart Climate platforms."""

from __future__ import annotations

from homeassistant.helpers.device_registry import DeviceInfo
from homeassistant.helpers.entity import Entity

from .const import DOMAIN, MANUFACTURER
from .runtime import ControllerRuntime, SmartClimateConfigEntry


class SmartClimateEntity(Entity):
    """Pushes state whenever its runtime finished a control cycle."""

    _attr_has_entity_name = True
    _attr_should_poll = False

    def __init__(self, entry: SmartClimateConfigEntry, unique_suffix: str) -> None:
        self.runtime: ControllerRuntime = entry.runtime_data
        self._attr_unique_id = f"{entry.entry_id}_{unique_suffix}"
        self._attr_device_info = DeviceInfo(
            identifiers={(DOMAIN, entry.entry_id)},
            name=entry.title,
            manufacturer=MANUFACTURER,
            model=self.runtime.model,
        )

    async def async_added_to_hass(self) -> None:
        await super().async_added_to_hass()
        self.async_on_remove(self.runtime.async_add_listener(self.async_write_ha_state))
