"""Time proportional output helper."""

from __future__ import annotations


def pwm_is_on(demand: float, elapsed_seconds: float, period_seconds: float) -> bool:
    """Return whether the actuator should be on in the current PWM cycle."""
    if demand <= 0:
        return False
    if demand >= 1:
        return True
    if period_seconds <= 0:
        raise ValueError("period_seconds must be > 0")
    position = elapsed_seconds % period_seconds
    return position < period_seconds * demand
