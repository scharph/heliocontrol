"""Shading part of a room: sun geometry in, cover positions out."""

from __future__ import annotations

from homeassistant.components.cover import ATTR_CURRENT_POSITION, CoverState
from homeassistant.core import Event, EventStateChangedData, HomeAssistant, State

from .. import hass_io
from ..config import ShadingConfig
from ..controller.shading import ManualOverrideTracker, ShadingController, ShadingInput, ShadingOutput

NOT_CONFIGURED = ShadingOutput(False, False, 100, None, "not_configured")
_MOVING = frozenset({CoverState.OPENING, CoverState.CLOSING})


def _position(state: State | None) -> int | None:
    if state is None:
        return None
    value = state.attributes.get(ATTR_CURRENT_POSITION)
    return int(value) if isinstance(value, (int, float)) else None


class RoomShading:
    def __init__(self, hass: HomeAssistant, config: ShadingConfig) -> None:
        self.hass = hass
        self.config = config
        self.controller = ShadingController()
        self.overrides = ManualOverrideTracker()
        self.output: ShadingOutput = NOT_CONFIGURED

    def handle_cover_event(self, event: Event[EventStateChangedData]) -> None:
        """Feed cover state changes into manual-override detection."""
        entity_id = event.data["entity_id"]
        if entity_id not in self.config.covers:
            return
        new_state = event.data["new_state"]
        if new_state is None:
            return
        self.overrides.observe(
            entity_id,
            old_position=_position(event.data["old_state"]),
            new_position=_position(new_state),
            moving=new_state.state in _MOVING,
        )

    async def async_update(self, *, temperature: float, window_open: bool, enabled: bool) -> None:
        if not self.config.enabled:
            self.output = NOT_CONFIGURED
            return
        sun = hass_io.sun_position(self.hass)
        if sun is None:
            return  # keep the last decision until the sun entity is back

        cfg = self.config
        # All covers of a room share one facade geometry, so a single
        # calculation serves every cover that is not manually overridden.
        self.output = self.controller.calculate(
            ShadingInput(
                temperature=temperature,
                shading_temperature=cfg.shading_temperature,
                sun_azimuth=sun[0],
                sun_elevation=sun[1],
                window_azimuth=cfg.window_azimuth,
                azimuth_tolerance=cfg.azimuth_tolerance,
                window_open=window_open,
                wind_alarm=hass_io.is_on(self.hass, cfg.wind_alarm),
                enabled=enabled,
                strategy=cfg.strategy,
                slat_width_mm=cfg.slat_width_mm,
                slat_spacing_mm=cfg.slat_spacing_mm,
            )
        )
        for cover in cfg.covers:
            if self.overrides.is_overridden(cover, self.output.sun_on_window):
                continue
            self.overrides.commanded(cover, self.output.cover_position)
            await hass_io.set_cover(self.hass, cover, self.output.cover_position, self.output.tilt_position)

    def clear_manual_override(self) -> None:
        self.overrides.clear()
