"""Learning helpers for room heating behaviour."""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime
from statistics import median


@dataclass(slots=True)
class HeatingLearner:
    """Keep the latest heating-rate samples in minutes per kelvin."""

    max_samples: int = 8
    samples: list[float] = field(default_factory=list)

    def add_sample(self, minutes_per_kelvin: float) -> bool:
        if not 5.0 <= minutes_per_kelvin <= 720.0:
            return False
        self.samples.append(float(minutes_per_kelvin))
        self.samples = self.samples[-self.max_samples :]
        return True

    @property
    def learned_rate(self) -> float | None:
        return median(self.samples) if self.samples else None

    def as_dict(self) -> dict:
        return {"heating_samples": list(self.samples)}

    @classmethod
    def from_dict(cls, data: dict | None) -> HeatingLearner:
        learner = cls()
        if data:
            for value in data.get("heating_samples", []):
                try:
                    learner.add_sample(float(value))
                except (TypeError, ValueError):
                    pass
        return learner


@dataclass(slots=True)
class HeatingPhaseTracker:
    """Detect strong, uninterrupted heating phases and turn them into rate samples.

    A sample (minutes per kelvin) is produced once the room has warmed by at least
    ``min_rise_k`` over at least ``min_minutes`` while heating was requested.
    """

    min_rise_k: float = 0.4
    min_minutes: float = 10.0
    start_time: datetime | None = None
    start_temperature: float | None = None

    def reset(self) -> None:
        self.start_time = None
        self.start_temperature = None

    def update(self, temperature: float, now: datetime, heating: bool) -> float | None:
        if self.start_time is None or self.start_temperature is None:
            if heating:
                self.start_time, self.start_temperature = now, temperature
            return None

        rise = temperature - self.start_temperature
        minutes = (now - self.start_time).total_seconds() / 60.0
        if rise >= self.min_rise_k and minutes >= self.min_minutes:
            # Start a fresh measurement window if heating continues.
            if heating:
                self.start_time, self.start_temperature = now, temperature
            else:
                self.reset()
            return minutes / rise
        if not heating:
            self.reset()
        return None


def preheat_minutes(current: float, target: float, rate_min_per_k: float | None) -> float:
    """Return predicted preheat time in minutes."""
    if rate_min_per_k is None or target <= current:
        return 0.0
    return max(0.0, (target - current) * rate_min_per_k)
