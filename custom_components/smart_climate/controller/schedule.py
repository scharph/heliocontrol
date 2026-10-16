"""Schedule/preheating helpers independent from Home Assistant."""

from __future__ import annotations

from datetime import datetime


def is_preheat_window(now: datetime, next_comfort: datetime | None, preheat_minutes: float) -> bool:
    """Return True when now is inside the calculated preheat window."""
    if next_comfort is None or preheat_minutes <= 0:
        return False
    seconds = (next_comfort - now).total_seconds()
    return 0 < seconds <= preheat_minutes * 60.0
