"""Heating/cooling mixer controller.

The controller is deliberately independent from Home Assistant.  It calculates a
pump request and either a 0..100 % valve position or a signed three-point valve
command (-1 close, 0 stop, +1 open).
"""

from __future__ import annotations

from dataclasses import dataclass
from enum import StrEnum


class MixerMode(StrEnum):
    IDLE = "idle"
    HEATING = "heating"
    COOLING = "cooling"
    SAFETY = "safety"


@dataclass(slots=True, frozen=True)
class MixerInput:
    target: float | None
    actual: float | None
    plant_mode: str = "idle"
    enabled: bool = True
    safety_trip: bool = False


@dataclass(slots=True, frozen=True)
class MixerOutput:
    mode: MixerMode
    pump: bool
    valve_position: float
    three_point_command: int
    error: float
    reason: str


class MixerController:
    """PI controller with output limiting and a three-point deadband."""

    def __init__(self, kp: float = 8.0, ki: float = 0.08, deadband: float = 0.25):
        self.kp = kp
        self.ki = ki
        self.deadband = deadband
        self.integral = 0.0
        self.position = 0.0

    def reset(self) -> None:
        self.integral = 0.0

    def _stop(self, mode: MixerMode, command: int, reason: str) -> MixerOutput:
        self.reset()
        return MixerOutput(mode, False, 0.0, command, 0.0, reason)

    def calculate(self, data: MixerInput, dt_seconds: float = 30.0) -> MixerOutput:
        if not data.enabled:
            return self._stop(MixerMode.IDLE, 0, "disabled")
        if data.safety_trip:
            return self._stop(MixerMode.SAFETY, -1, "safety_trip")
        if data.target is None or data.actual is None:
            return self._stop(MixerMode.IDLE, 0, "missing_temperature")
        if data.plant_mode == "heating":
            mode = MixerMode.HEATING
        elif data.plant_mode == "cooling":
            mode = MixerMode.COOLING
        else:
            return self._stop(MixerMode.IDLE, -1, "plant_idle")

        error = data.target - data.actual
        # A more-open mixing valve raises flow temperature in heating mode but
        # lowers it in cooling mode, so reverse the process error for cooling.
        process_error = error if mode == MixerMode.HEATING else -error
        self.integral += process_error * max(0.0, dt_seconds) / 60.0
        # anti-windup: integral contribution cannot exceed full output by itself
        if self.ki > 0:
            lim = 100.0 / self.ki
            self.integral = max(-lim, min(lim, self.integral))
        requested = self.position + self.kp * process_error + self.ki * self.integral
        self.position = max(0.0, min(100.0, requested))
        cmd = 0 if abs(process_error) <= self.deadband else (1 if process_error > 0 else -1)
        return MixerOutput(mode, True, self.position, cmd, error, "regulating" if cmd else "within_deadband")
