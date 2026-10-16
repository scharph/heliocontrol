from custom_components.smart_climate.controller.shading import (
    ShadingController,
    ShadingInput,
    ShadingStrategy,
    daylight_tilt,
    solar_profile_angle,
)


def test_shades_hot_south_window():
    out = ShadingController().calculate(ShadingInput(26, 23.5, 180, 45, 180))
    assert out.demand is True
    assert out.cover_position == 15
    assert out.tilt_position is not None


def test_no_shade_when_sun_behind_window():
    out = ShadingController().calculate(ShadingInput(26, 23.5, 0, 45, 180))
    assert out.demand is False


def test_wind_alarm_opens_cover():
    out = ShadingController().calculate(ShadingInput(30, 23.5, 180, 45, 180, wind_alarm=True))
    assert out.cover_position == 100
    assert out.reason == "wind_alarm"


def test_manual_override_suppresses_motion():
    out = ShadingController().calculate(ShadingInput(30, 23.5, 180, 45, 180, manual_override=True))
    assert out.reason == "manual_override"
    assert out.cover_position == -1


def test_maximum_cooling_closes_more_than_daylight():
    ctl = ShadingController()
    daylight = ctl.calculate(ShadingInput(30, 23.5, 180, 45, 180, strategy=ShadingStrategy.MAXIMUM_DAYLIGHT))
    cooling = ctl.calculate(ShadingInput(30, 23.5, 180, 45, 180, strategy=ShadingStrategy.MAXIMUM_COOLING))
    assert cooling.cover_position < daylight.cover_position
    assert cooling.tilt_position <= daylight.tilt_position


def test_profile_geometry_changes_with_sun_height():
    low = solar_profile_angle(20, 0)
    high = solar_profile_angle(60, 0)
    assert high > low
    assert daylight_tilt(high, 80, 70) > daylight_tilt(low, 80, 70)
