from custom_components.smart_climate.controller.climate_controller import (
    ClimateController,
    PlantInput,
    PlantMode,
    RoomDemand,
)


def test_area_weighted_heat_demand():
    c = ClimateController(start_threshold=0.3, min_off_seconds=0, min_run_seconds=0)
    o = c.calculate(PlantInput((RoomDemand(0.8, 0, 40), RoomDemand(0.2, 0, 10))))
    assert o.mode == PlantMode.HEATING
    assert round(o.heating_demand, 2) == 0.68


def test_restart_lockout():
    c = ClimateController(start_threshold=0.3, min_off_seconds=300, min_run_seconds=0)
    o = c.calculate(PlantInput((RoomDemand(0.9, 0, 20),), PlantMode.IDLE, 0, 60))
    assert o.mode == PlantMode.IDLE
    assert o.reason == "restart_lockout"


def test_stage2():
    c = ClimateController(start_threshold=0.3, stage2_threshold=0.75, min_off_seconds=0, min_run_seconds=0)
    o = c.calculate(PlantInput((RoomDemand(0.9, 0, 20),)))
    assert o.stage2 is True
