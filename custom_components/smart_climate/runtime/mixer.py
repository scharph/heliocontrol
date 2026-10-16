"""Runtime for the Heating Mixer Controller."""

from __future__ import annotations

import time

from homeassistant.core import HomeAssistant

from .. import hass_io
from ..config import MixerConfig
from ..const import ENTRY_TYPE_MIXER, UPDATE_INTERVAL
from ..controller.climate_controller import PlantMode
from ..controller.mixer import MixerController, MixerInput, MixerMode, MixerOutput
from .base import ControllerRuntime, find_runtime
from .building import BuildingRuntime
from .temperature import TemperatureRuntime

_MAX_DT_SECONDS = 300.0


class MixerRuntime(ControllerRuntime):
    entry_type = ENTRY_TYPE_MIXER
    model = "Heating Mixer Controller"

    def __init__(self, hass: HomeAssistant, entry_id: str, config: MixerConfig) -> None:
        super().__init__(hass, entry_id)
        self.config = config
        self.controller = MixerController(config.kp, config.ki, config.deadband)
        self.output = MixerOutput(MixerMode.IDLE, False, 0.0, 0, 0.0, "startup")
        self._last_run: float | None = None

    def _elapsed_seconds(self) -> float:
        """Real time since the last PI step (manual recalculations included)."""
        now = time.monotonic()
        dt = UPDATE_INTERVAL.total_seconds() if self._last_run is None else now - self._last_run
        self._last_run = now
        return min(max(dt, 0.0), _MAX_DT_SECONDS)

    async def _async_update(self) -> None:
        temperature = find_runtime(self.hass, TemperatureRuntime)
        building = find_runtime(self.hass, BuildingRuntime)
        target = temperature.output.flow_target if temperature else None
        plant_mode = building.output.mode if building else PlantMode.IDLE
        actual = hass_io.state_float(self.hass, self.config.flow_sensor)
        over_temperature = (
            actual is not None and plant_mode == PlantMode.HEATING and actual >= self.config.max_temperature
        )

        self.output = self.controller.calculate(
            MixerInput(target, actual, plant_mode, enabled=True, safety_trip=over_temperature),
            self._elapsed_seconds(),
        )
        await hass_io.set_switch(self.hass, self.config.pump, self.output.pump)
        if self.config.analog_valve:
            await hass_io.set_number(self.hass, self.config.position_number, self.output.valve_position)
        else:
            await self._drive_three_point(self.output.three_point_command)

    async def _drive_three_point(self, command: int) -> None:
        # Always release the opposite relay first so both are never on together.
        if command > 0:
            await hass_io.set_switch(self.hass, self.config.close_switch, False)
            await hass_io.set_switch(self.hass, self.config.open_switch, True)
        elif command < 0:
            await hass_io.set_switch(self.hass, self.config.open_switch, False)
            await hass_io.set_switch(self.hass, self.config.close_switch, True)
        else:
            await hass_io.set_switch(self.hass, self.config.open_switch, False)
            await hass_io.set_switch(self.hass, self.config.close_switch, False)

    async def _async_shutdown(self) -> None:
        await self._drive_three_point(0)
