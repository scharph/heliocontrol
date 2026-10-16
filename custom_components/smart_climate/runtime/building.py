"""Runtime for the building-wide Climate Controller."""

from __future__ import annotations

from homeassistant.core import HomeAssistant
from homeassistant.util import dt as dt_util

from .. import hass_io
from ..config import BuildingConfig
from ..const import ENTRY_TYPE_BUILDING
from ..controller.climate_controller import PlantInput, PlantMode, PlantOutput, RoomDemand
from .base import ControllerRuntime, iter_runtimes
from .room import RoomRuntime


class BuildingRuntime(ControllerRuntime):
    entry_type = ENTRY_TYPE_BUILDING
    model = "Building Climate Controller"

    def __init__(self, hass: HomeAssistant, entry_id: str, config: BuildingConfig) -> None:
        super().__init__(hass, entry_id)
        self.config = config
        self.controller = config.build_controller()
        self.output = PlantOutput(PlantMode.IDLE, 0.0, 0.0, False, "startup")
        now = dt_util.utcnow()
        self._mode_since = now
        self._off_since = now  # restart lockout also applies right after startup

    def _room_demands(self) -> tuple[RoomDemand, ...]:
        """Demand of every available room; unavailable rooms are excluded."""
        return tuple(
            RoomDemand(
                heating=room.output.heating_demand,
                cooling=room.output.cooling_demand,
                area_m2=room.config.area_m2,
                occupied=room.inputs.occupied,
            )
            for room in iter_runtimes(self.hass, RoomRuntime)
            if room.available
        )

    async def _async_update(self) -> None:
        now = dt_util.utcnow()
        previous = self.output.mode
        self.output = self.controller.calculate(
            PlantInput(
                rooms=self._room_demands(),
                previous_mode=previous,
                seconds_in_mode=(now - self._mode_since).total_seconds(),
                seconds_since_off=(now - self._off_since).total_seconds(),
            )
        )
        if self.output.mode != previous:
            self._mode_since = now
            if self.output.mode == PlantMode.IDLE:
                self._off_since = now

        heat = self.output.mode == PlantMode.HEATING
        cool = self.output.mode == PlantMode.COOLING
        await self._set_stages(heat, heat and self.output.stage2, cool, cool and self.output.stage2)

    async def _async_shutdown(self) -> None:
        await self._set_stages(False, False, False, False)

    async def _set_stages(self, heat1: bool, heat2: bool, cool1: bool, cool2: bool) -> None:
        cfg = self.config
        await hass_io.set_switch(self.hass, cfg.heating_stage1, heat1)
        await hass_io.set_switch(self.hass, cfg.heating_stage2, heat2)
        await hass_io.set_switch(self.hass, cfg.cooling_stage1, cool1)
        await hass_io.set_switch(self.hass, cfg.cooling_stage2, cool2)
