from custom_components.smart_climate.controller.climate_controller import PlantMode
from custom_components.smart_climate.controller.temperature_controller import (
    IntelligentTemperatureController,
    TemperatureInput,
)


def test_heating_target_rises_with_demand():
    c = IntelligentTemperatureController(weather_compensation=0)
    low = c.calculate(TemperatureInput(PlantMode.HEATING, 0.2, 0, 10))
    high = c.calculate(TemperatureInput(PlantMode.HEATING, 0.8, 0, 10))
    assert high.flow_target > low.flow_target


def test_cooling_target_falls_with_demand():
    c = IntelligentTemperatureController()
    low = c.calculate(TemperatureInput(PlantMode.COOLING, 0, 0.2, 25))
    high = c.calculate(TemperatureInput(PlantMode.COOLING, 0, 0.8, 25))
    assert high.flow_target < low.flow_target


def test_idle_has_no_target():
    o = IntelligentTemperatureController().calculate(TemperatureInput(PlantMode.IDLE, 0, 0))
    assert o.flow_target is None and o.buffer_target is None
