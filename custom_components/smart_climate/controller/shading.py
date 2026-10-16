"""Pure automatic shading logic, independent from Home Assistant."""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import StrEnum
from math import atan, cos, degrees, radians, tan


def angular_distance(a: float, b: float) -> float:
    return abs((a - b + 180.0) % 360.0 - 180.0)


class ShadingStrategy(StrEnum):
    MAXIMUM_DAYLIGHT = "maximum_daylight"
    MAXIMUM_COOLING = "maximum_cooling"


@dataclass(slots=True, frozen=True)
class ShadingInput:
    temperature: float
    shading_temperature: float
    sun_azimuth: float
    sun_elevation: float
    window_azimuth: float
    azimuth_tolerance: float = 85.0
    window_open: bool = False
    wind_alarm: bool = False
    enabled: bool = True
    manual_override: bool = False
    strategy: ShadingStrategy = ShadingStrategy.MAXIMUM_DAYLIGHT
    slat_width_mm: float = 80.0
    slat_spacing_mm: float = 70.0


@dataclass(slots=True, frozen=True)
class ShadingOutput:
    demand: bool
    sun_on_window: bool
    cover_position: int
    tilt_position: int | None
    reason: str


def solar_profile_angle(elevation: float, relative_azimuth: float) -> float:
    """Vertical solar profile angle on the window plane, in degrees."""
    c = max(0.05, cos(radians(relative_azimuth)))
    return degrees(atan(tan(radians(max(0.0, elevation))) / c))


def daylight_tilt(profile_angle: float, width_mm: float, spacing_mm: float) -> int:
    """Approximate HA tilt (0 closed, 100 open) that blocks direct sun.

    Geometry uses the solar profile angle and the slat width/spacing ratio.
    It intentionally errs slightly toward shading. The output is hardware-neutral
    and can later be calibrated per cover.
    """
    ratio = max(0.2, min(2.0, spacing_mm / max(1.0, width_mm)))
    # High sun can use more open slats; wider/tighter slats need less closure.
    open_pct = profile_angle * ratio
    return int(round(max(5.0, min(90.0, open_pct))))


class ShadingController:
    def calculate(self, data: ShadingInput) -> ShadingOutput:
        if not data.enabled:
            return ShadingOutput(False, False, 100, None, "disabled")
        if data.wind_alarm:
            return ShadingOutput(False, False, 100, None, "wind_alarm")
        if data.window_open:
            return ShadingOutput(False, False, 100, None, "window_open")

        incidence = angular_distance(data.sun_azimuth, data.window_azimuth)
        sun_on_window = data.sun_elevation > 0 and incidence <= data.azimuth_tolerance
        demand = sun_on_window and data.temperature >= data.shading_temperature

        if data.manual_override:
            return ShadingOutput(demand, sun_on_window, -1, None, "manual_override")
        if not demand:
            return ShadingOutput(False, sun_on_window, 100, None, "no_demand")

        if data.strategy == ShadingStrategy.MAXIMUM_COOLING:
            return ShadingOutput(True, True, 5, 5, "maximum_cooling")

        profile = solar_profile_angle(data.sun_elevation, incidence)
        tilt = daylight_tilt(profile, data.slat_width_mm, data.slat_spacing_mm)
        return ShadingOutput(True, True, 15, tilt, "maximum_daylight")


@dataclass(slots=True)
class ManualOverrideTracker:
    """Track which covers were moved by hand rather than by the automation.

    The runtime records every position it commands. A cover that settles
    (i.e. is not opening/closing) at a different position was moved manually,
    and automation is suspended for it until the sun leaves the facade.
    Only the position is compared: some cover integrations reset the slat tilt
    while repositioning, which would otherwise look like a manual change.
    """

    tolerance: int = 2
    _commanded: dict[str, int] = field(default_factory=dict)
    _overridden: set[str] = field(default_factory=set)

    def commanded(self, cover: str, position: int) -> None:
        self._commanded[cover] = position

    def observe(self, cover: str, old_position: int | None, new_position: int | None, moving: bool) -> None:
        if moving or new_position is None:
            return
        expected = self._commanded.get(cover)
        if expected is None:
            manual = old_position is not None and old_position != new_position
        else:
            manual = abs(new_position - expected) > self.tolerance
        if manual:
            self._overridden.add(cover)
            self._commanded.pop(cover, None)

    def is_overridden(self, cover: str, sun_on_window: bool) -> bool:
        # Loxone-like behaviour: a manual override lasts until the current
        # solar period on this facade ends.
        if not sun_on_window:
            self._overridden.discard(cover)
        return cover in self._overridden

    def clear(self) -> None:
        self._overridden.clear()

    @property
    def overridden(self) -> frozenset[str]:
        return frozenset(self._overridden)
