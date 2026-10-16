"""Pure room heating/cooling control logic, independent from Home Assistant."""

from __future__ import annotations

from dataclasses import dataclass
from enum import StrEnum


class RoomMode(StrEnum):
    COMFORT = "comfort"
    ECO = "eco"
    PREHEAT = "preheat"
    PROTECTION = "protection"
    OFF = "off"


@dataclass(slots=True, frozen=True)
class RoomInput:
    temperature: float
    target_temperature: float
    window_open: bool = False
    enabled: bool = True
    mode: RoomMode = RoomMode.COMFORT
    cooling_target: float | None = None


@dataclass(slots=True, frozen=True)
class RoomOutput:
    heating_demand: float
    heating: bool
    effective_target: float
    mode: RoomMode = RoomMode.COMFORT
    cooling_demand: float = 0.0
    cooling: bool = False

    @classmethod
    def idle(cls, target: float, mode: RoomMode) -> RoomOutput:
        return cls(0.0, False, target, mode, 0.0, False)


def select_room_mode(
    *,
    enabled: bool,
    window_open: bool,
    manual_target: bool,
    occupied: bool,
    schedule_active: bool,
    preheat: bool,
) -> RoomMode:
    """Priority: off > window protection > manual/presence/schedule comfort > preheat > eco."""
    if not enabled:
        return RoomMode.OFF
    if window_open:
        return RoomMode.PROTECTION
    if manual_target or occupied or schedule_active:
        return RoomMode.COMFORT
    if preheat:
        return RoomMode.PREHEAT
    return RoomMode.ECO


def heating_target(mode: RoomMode, comfort: float, eco: float, protection: float) -> float:
    """Comfort and Preheat both heat towards the (possibly user-adjusted) comfort target."""
    if mode == RoomMode.PROTECTION:
        return protection
    if mode == RoomMode.ECO:
        return eco
    return comfort


def cooling_target(mode: RoomMode, cooling: float, comfort: float, eco: float) -> float:
    """Eco widens the comfort band symmetrically on the cooling side."""
    if mode == RoomMode.ECO:
        return cooling + max(0.0, comfort - eco)
    return cooling


def _proportional(error: float, deadband: float, band: float) -> float:
    return 0.0 if error <= deadband else min(1.0, max(0.0, error / band))


class RoomController:
    """Proportional heat/cool controller with a neutral zone between targets."""

    def __init__(self, proportional_band: float = 2.0, deadband: float = 0.15) -> None:
        self.proportional_band = max(0.1, proportional_band)
        self.deadband = max(0.0, deadband)

    def calculate(self, data: RoomInput) -> RoomOutput:
        if not data.enabled:
            return RoomOutput.idle(data.target_temperature, RoomMode.OFF)
        heat = _proportional(data.target_temperature - data.temperature, self.deadband, self.proportional_band)
        cool = 0.0
        if data.cooling_target is not None and data.mode != RoomMode.PROTECTION:
            cool = _proportional(data.temperature - data.cooling_target, self.deadband, self.proportional_band)
        # Heating always wins if misconfigured targets overlap.
        if heat > 0:
            cool = 0.0
        return RoomOutput(heat, heat > 0, data.target_temperature, data.mode, cool, cool > 0)
