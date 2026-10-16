"""Climate entity for room controllers."""

from __future__ import annotations

from typing import Any

from homeassistant.components.climate import ClimateEntity, ClimateEntityFeature, HVACAction, HVACMode
from homeassistant.const import ATTR_TEMPERATURE, UnitOfTemperature
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback

from .entity import SmartClimateEntity
from .runtime import RoomRuntime, SmartClimateConfigEntry


async def async_setup_entry(
    hass: HomeAssistant, entry: SmartClimateConfigEntry, async_add_entities: AddConfigEntryEntitiesCallback
) -> None:
    if isinstance(entry.runtime_data, RoomRuntime):
        async_add_entities([SmartRoomClimate(entry)])


class SmartRoomClimate(SmartClimateEntity, ClimateEntity):
    runtime: RoomRuntime

    _attr_translation_key = "room_controller"
    _attr_temperature_unit = UnitOfTemperature.CELSIUS
    _attr_supported_features = (
        ClimateEntityFeature.TARGET_TEMPERATURE | ClimateEntityFeature.TURN_ON | ClimateEntityFeature.TURN_OFF
    )

    def __init__(self, entry: SmartClimateConfigEntry) -> None:
        super().__init__(entry, "climate")
        # The room switches between heating and cooling on its own; the only
        # user choice is on/off. Offer HEAT_COOL only when a cooler exists.
        self._active_mode = HVACMode.HEAT_COOL if self.runtime.has_cooling else HVACMode.HEAT
        self._attr_hvac_modes = [self._active_mode, HVACMode.OFF]

    @property
    def available(self) -> bool:
        return self.runtime.available

    @property
    def current_temperature(self) -> float | None:
        return self.runtime.inputs.temperature

    @property
    def target_temperature(self) -> float:
        return self.runtime.target_temperature

    @property
    def hvac_mode(self) -> HVACMode:
        return self._active_mode if self.runtime.enabled else HVACMode.OFF

    @property
    def hvac_action(self) -> HVACAction:
        if not self.runtime.enabled:
            return HVACAction.OFF
        if self.runtime.output.heating:
            return HVACAction.HEATING
        if self.runtime.output.cooling:
            return HVACAction.COOLING
        return HVACAction.IDLE

    async def async_set_temperature(self, **kwargs: Any) -> None:
        if (temperature := kwargs.get(ATTR_TEMPERATURE)) is not None:
            await self.runtime.async_set_target_temperature(float(temperature))

    async def async_set_hvac_mode(self, hvac_mode: HVACMode) -> None:
        await self.runtime.async_set_enabled(hvac_mode != HVACMode.OFF)

    async def async_turn_on(self) -> None:
        await self.runtime.async_set_enabled(True)

    async def async_turn_off(self) -> None:
        await self.runtime.async_set_enabled(False)
