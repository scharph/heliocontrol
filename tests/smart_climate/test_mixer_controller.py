from custom_components.smart_climate.controller.mixer import MixerController, MixerInput, MixerMode


def test_heating_opens_when_flow_too_cold():
    c = MixerController(kp=10, ki=0, deadband=0.2)
    o = c.calculate(MixerInput(35, 30, "heating"), 30)
    assert o.pump and o.mode == MixerMode.HEATING and o.three_point_command == 1 and o.valve_position > 0


def test_heating_closes_when_too_hot():
    c = MixerController(kp=10, ki=0, deadband=0.2)
    c.position = 50
    o = c.calculate(MixerInput(35, 40, "heating"), 30)
    assert o.three_point_command == -1 and o.valve_position < 50


def test_cooling_reverses_process_direction():
    c = MixerController(kp=10, ki=0, deadband=0.2)
    c.position = 50
    o = c.calculate(MixerInput(18, 22, "cooling"), 30)
    assert o.mode == MixerMode.COOLING and o.three_point_command == 1


def test_deadband_stops_three_point_motor():
    c = MixerController(deadband=0.3)
    o = c.calculate(MixerInput(35, 34.8, "heating"), 30)
    assert o.three_point_command == 0


def test_safety_closes_and_stops_pump():
    c = MixerController()
    c.position = 70
    o = c.calculate(MixerInput(35, 55, "heating", safety_trip=True), 30)
    assert o.mode == MixerMode.SAFETY and not o.pump and o.three_point_command == -1 and o.valve_position == 0
