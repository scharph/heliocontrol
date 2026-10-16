"""Demand-adaptive flow/buffer temperature controller."""

from dataclasses import dataclass

from .climate_controller import PlantMode


@dataclass(frozen=True, slots=True)
class TemperatureInput:
    mode: PlantMode
    heating_demand: float
    cooling_demand: float
    outdoor_temperature: float | None = None


@dataclass(frozen=True, slots=True)
class TemperatureOutput:
    flow_target: float | None
    buffer_target: float | None
    reason: str


class IntelligentTemperatureController:
    def __init__(
        self, heat_min=25.0, heat_max=40.0, cool_min=16.0, cool_max=20.0, buffer_offset=3.0, weather_compensation=0.25
    ):
        self.heat_min, self.heat_max = heat_min, heat_max
        self.cool_min, self.cool_max = cool_min, cool_max
        self.buffer_offset = buffer_offset
        self.weather_compensation = weather_compensation

    def calculate(self, i: TemperatureInput) -> TemperatureOutput:
        if i.mode == PlantMode.HEATING:
            demand = max(0.0, min(1.0, i.heating_demand))
            target = self.heat_min + (self.heat_max - self.heat_min) * demand
            if i.outdoor_temperature is not None:
                # colder outdoor air raises target, mild weather lowers it; bounded below/above.
                target += max(-4.0, min(4.0, (10.0 - i.outdoor_temperature) * self.weather_compensation))
            target = max(self.heat_min, min(self.heat_max, target))
            return TemperatureOutput(round(target, 1), round(target + self.buffer_offset, 1), "heating_demand")
        if i.mode == PlantMode.COOLING:
            demand = max(0.0, min(1.0, i.cooling_demand))
            # More cooling demand means a colder flow target.
            target = self.cool_max - (self.cool_max - self.cool_min) * demand
            target = max(self.cool_min, min(self.cool_max, target))
            return TemperatureOutput(round(target, 1), round(target - self.buffer_offset, 1), "cooling_demand")
        return TemperatureOutput(None, None, "idle")
