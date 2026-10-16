from datetime import UTC, datetime, timedelta

from custom_components.smart_climate.controller.learning import HeatingLearner, preheat_minutes
from custom_components.smart_climate.controller.pwm import pwm_is_on
from custom_components.smart_climate.controller.room import RoomController, RoomInput, RoomMode
from custom_components.smart_climate.controller.schedule import is_preheat_window


def test_full_demand_when_far_below_target():
    out = RoomController().calculate(RoomInput(19.0, 22.0))
    assert out.heating_demand == 1.0
    assert out.heating


def test_no_heat_inside_deadband():
    out = RoomController().calculate(RoomInput(21.9, 22.0))
    assert out.heating_demand == 0.0


def test_window_flag_does_not_bypass_explicit_protection_target():
    out = RoomController().calculate(RoomInput(3.0, 5.0, window_open=True, mode=RoomMode.PROTECTION))
    assert out.heating_demand == 1.0


def test_pwm():
    assert pwm_is_on(0.5, 10, 100)
    assert not pwm_is_on(0.5, 60, 100)


def test_learning_uses_median_of_last_eight():
    learner = HeatingLearner()
    for value in [100, 80, 90, 95, 85, 110, 70, 88, 92]:
        assert learner.add_sample(value)
    assert len(learner.samples) == 8
    assert learner.learned_rate == 89.0


def test_preheat_prediction():
    assert preheat_minutes(20.5, 22.0, 90.0) == 135.0


def test_protection_mode_can_hold_frost_target():
    out = RoomController().calculate(RoomInput(3.0, 5.0, window_open=True, mode=RoomMode.PROTECTION))
    assert out.heating_demand == 1.0
    assert out.mode == RoomMode.PROTECTION


def test_preheat_window_starts_at_learned_lead_time():
    now = datetime(2026, 10, 16, 4, 15, tzinfo=UTC)
    comfort = now + timedelta(minutes=135)
    assert is_preheat_window(now, comfort, 135)
    assert not is_preheat_window(now - timedelta(minutes=1), comfort, 135)


def test_preheat_window_requires_future_event():
    now = datetime(2026, 10, 16, 6, 31, tzinfo=UTC)
    assert not is_preheat_window(now, now - timedelta(minutes=1), 135)


def test_cooling_demand_when_above_cooling_target():
    out = RoomController().calculate(RoomInput(27.0, 22.0, cooling_target=24.0))
    assert out.cooling_demand == 1.0
    assert out.cooling
    assert not out.heating


def test_neutral_zone_has_no_heat_or_cool():
    out = RoomController().calculate(RoomInput(23.0, 22.0, cooling_target=24.0))
    assert out.heating_demand == 0.0
    assert out.cooling_demand == 0.0


def test_protection_disables_active_cooling():
    out = RoomController().calculate(
        RoomInput(30.0, 5.0, window_open=True, mode=RoomMode.PROTECTION, cooling_target=24.0)
    )
    assert out.cooling_demand == 0.0
