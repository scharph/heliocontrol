from datetime import UTC, datetime, timedelta

from custom_components.smart_climate.controller.learning import HeatingPhaseTracker
from custom_components.smart_climate.controller.room import (
    RoomMode,
    cooling_target,
    heating_target,
    select_room_mode,
)
from custom_components.smart_climate.controller.shading import ManualOverrideTracker

T0 = datetime(2026, 10, 16, 6, 0, tzinfo=UTC)


def _mode(**overrides):
    args = dict(
        enabled=True, window_open=False, manual_target=False, occupied=False, schedule_active=False, preheat=False
    )
    args.update(overrides)
    return select_room_mode(**args)


def test_mode_priority():
    assert _mode(enabled=False, window_open=True, occupied=True) == RoomMode.OFF
    assert _mode(window_open=True, occupied=True) == RoomMode.PROTECTION
    assert _mode(occupied=True, preheat=True) == RoomMode.COMFORT
    assert _mode(manual_target=True) == RoomMode.COMFORT
    assert _mode(schedule_active=True) == RoomMode.COMFORT
    assert _mode(preheat=True) == RoomMode.PREHEAT
    assert _mode() == RoomMode.ECO


def test_targets():
    assert heating_target(RoomMode.PREHEAT, 22.5, 19, 5) == 22.5
    assert heating_target(RoomMode.ECO, 22.5, 19, 5) == 19
    assert heating_target(RoomMode.PROTECTION, 22.5, 19, 5) == 5
    assert cooling_target(RoomMode.COMFORT, 24, 22, 19) == 24
    assert cooling_target(RoomMode.ECO, 24, 22, 19) == 27


def test_phase_tracker_produces_sample_and_restarts():
    t = HeatingPhaseTracker()
    assert t.update(20.0, T0, heating=True) is None
    assert t.update(20.2, T0 + timedelta(minutes=10), heating=True) is None  # rise too small
    sample = t.update(20.5, T0 + timedelta(minutes=45), heating=True)
    assert sample == 90.0
    assert t.start_time == T0 + timedelta(minutes=45)


def test_phase_tracker_resets_when_heating_stops():
    t = HeatingPhaseTracker()
    t.update(20.0, T0, heating=True)
    assert t.update(20.1, T0 + timedelta(minutes=5), heating=False) is None
    assert t.start_time is None


def test_override_ignores_own_moves_and_detects_manual_ones():
    o = ManualOverrideTracker()
    o.commanded("cover.a", 15)
    o.observe("cover.a", 100, 60, moving=True)  # still travelling
    o.observe("cover.a", 60, 15, moving=False)  # arrived where we sent it
    assert not o.is_overridden("cover.a", sun_on_window=True)
    o.observe("cover.a", 15, 70, moving=False)  # someone moved it
    assert o.is_overridden("cover.a", sun_on_window=True)
    assert not o.is_overridden("cover.a", sun_on_window=False)  # solar period over


def test_override_without_command_needs_real_position_change():
    o = ManualOverrideTracker()
    o.observe("cover.a", 50, 50, moving=False)  # attribute-only update
    assert not o.is_overridden("cover.a", True)
    o.observe("cover.a", 50, 20, moving=False)
    assert o.is_overridden("cover.a", True)
