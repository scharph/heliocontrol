"""Runtime for the Intelligent Temperature Controller (flow/buffer targets)."""

from __future__ import annotations

from homeassistant.core import HomeAssistant

from .. import hass_io
from ..config import TemperatureConfig
from ..const import ENTRY_TYPE_TEMPERATURE
from ..controller.temperature_controller import TemperatureInput, TemperatureOutput
from .base import ControllerRuntime, find_runtime
from .building import BuildingRuntime


class TemperatureRuntime(ControllerRuntime):
    entry_type = ENTRY_TYPE_TEMPERATURE
    model = "Intelligent Temperature Controller"

    def __init__(self, hass: HomeAssistant, entry_id: str, config: TemperatureConfig) -> None:
        super().__init__(hass, entry_id)
        self.config = config
        self.controller = config.build_controller()
        self.output = TemperatureOutput(None, None, "startup")

    async def _async_update(self) -> None:
        building = find_runtime(self.hass, BuildingRuntime)
        if building is None:
            self.output = TemperatureOutput(None, None, "no_building_controller")
            return
        plant = building.output
        self.output = self.controller.calculate(
            TemperatureInput(
                mode=plant.mode,
                heating_demand=plant.heating_demand,
                cooling_demand=plant.cooling_demand,
                outdoor_temperature=hass_io.state_float(self.hass, self.config.outdoor_sensor),
            )
        )
        await hass_io.set_number(self.hass, self.config.flow_setpoint, self.output.flow_target)
        await hass_io.set_number(self.hass, self.config.buffer_setpoint, self.output.buffer_target)
