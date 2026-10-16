"""Building-wide heat/cool demand aggregation and plant state machine."""

from __future__ import annotations

from dataclasses import dataclass
from enum import StrEnum


class PlantMode(StrEnum):
    IDLE = "idle"
    HEATING = "heating"
    COOLING = "cooling"


@dataclass(slots=True, frozen=True)
class RoomDemand:
    heating: float = 0.0
    cooling: float = 0.0
    area_m2: float = 1.0
    occupied: bool = False


@dataclass(slots=True, frozen=True)
class PlantInput:
    rooms: tuple[RoomDemand, ...]
    previous_mode: PlantMode = PlantMode.IDLE
    seconds_in_mode: float = 0.0
    seconds_since_off: float = 1e9


@dataclass(slots=True, frozen=True)
class PlantOutput:
    mode: PlantMode
    heating_demand: float
    cooling_demand: float
    stage2: bool
    reason: str


@dataclass(slots=True)
class ClimateController:
    start_threshold: float = 0.30
    stop_threshold: float = 0.15
    stage2_threshold: float = 0.75
    min_run_seconds: float = 600.0
    min_off_seconds: float = 300.0
    occupied_weight: float = 1.15

    def _weighted(self, rooms: tuple[RoomDemand, ...], attr: str) -> float:
        if not rooms:
            return 0.0
        total_w = 0.0
        value = 0.0
        for room in rooms:
            weight = max(room.area_m2, 0.1) * (self.occupied_weight if room.occupied else 1.0)
            total_w += weight
            value += max(0.0, min(1.0, getattr(room, attr))) * weight
        return value / total_w if total_w else 0.0

    def calculate(self, inp: PlantInput) -> PlantOutput:
        heat = self._weighted(inp.rooms, "heating")
        cool = self._weighted(inp.rooms, "cooling")
        dominant = PlantMode.HEATING if heat >= cool else PlantMode.COOLING
        demand = max(heat, cool)

        if inp.previous_mode != PlantMode.IDLE and inp.seconds_in_mode < self.min_run_seconds:
            mode = inp.previous_mode
            reason = "minimum_run_time"
        elif inp.previous_mode != PlantMode.IDLE and demand <= self.stop_threshold:
            mode = PlantMode.IDLE
            reason = "below_stop_threshold"
        elif inp.previous_mode == PlantMode.IDLE and demand < self.start_threshold:
            mode = PlantMode.IDLE
            reason = "below_start_threshold"
        elif inp.previous_mode == PlantMode.IDLE and inp.seconds_since_off < self.min_off_seconds:
            mode = PlantMode.IDLE
            reason = "restart_lockout"
        else:
            mode = dominant
            reason = "demand"

        active_demand = heat if mode == PlantMode.HEATING else cool if mode == PlantMode.COOLING else 0.0
        return PlantOutput(mode, heat, cool, active_demand >= self.stage2_threshold, reason)
