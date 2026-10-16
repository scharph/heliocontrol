"""Runtime for one Intelligent Room Controller config entry."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import Any

from homeassistant.core import Event, EventStateChangedData, HomeAssistant, callback
from homeassistant.helpers.event import async_track_state_change_event
from homeassistant.helpers.storage import Store
from homeassistant.util import dt as dt_util

from .. import hass_io
from ..config import RoomConfig
from ..const import DOMAIN, ENTRY_TYPE_ROOM, STORAGE_SAVE_DELAY_SECONDS, STORAGE_VERSION
from ..controller.learning import HeatingLearner, HeatingPhaseTracker, preheat_minutes
from ..controller.pwm import pwm_is_on
from ..controller.room import (
    RoomController,
    RoomInput,
    RoomMode,
    RoomOutput,
    cooling_target,
    heating_target,
    select_room_mode,
)
from ..controller.schedule import is_preheat_window
from .base import ControllerRuntime
from .shading import RoomShading

LEARNING_MIN_DEMAND = 0.75


@dataclass(frozen=True, slots=True)
class RoomInputs:
    """One consistent snapshot of all HA states a room cycle depends on."""

    temperature: float | None
    window_open: bool
    occupied: bool
    schedule_active: bool
    next_comfort_start: datetime | None


class _SwitchGuard:
    """Idempotent switch with a minimum interval between starts.

    Turning off is always allowed (fail-safe); only turning on is rate limited.
    """

    def __init__(self, hass: HomeAssistant, entity_id: str | None, min_seconds: float) -> None:
        self.hass = hass
        self.entity_id = entity_id
        self.min_seconds = min_seconds
        self._last_change: datetime | None = None

    async def async_set(self, on: bool, now: datetime) -> None:
        if not self.entity_id:
            return
        if on and self._last_change is not None and (now - self._last_change).total_seconds() < self.min_seconds:
            return
        if await hass_io.set_switch(self.hass, self.entity_id, on):
            self._last_change = now


class RoomRuntime(ControllerRuntime):
    entry_type = ENTRY_TYPE_ROOM
    model = "Intelligent Room Controller"

    def __init__(self, hass: HomeAssistant, entry_id: str, config: RoomConfig) -> None:
        super().__init__(hass, entry_id)
        self.config = config
        self.controller = RoomController(deadband=config.hvac_deadband)
        self.shading = RoomShading(hass, config.shading)
        self.learner = HeatingLearner()
        self._phase = HeatingPhaseTracker()
        self._heater = _SwitchGuard(hass, config.heater, config.min_switch_seconds)
        self._cooler = _SwitchGuard(hass, config.cooler, config.min_switch_seconds)
        self._store: Store[dict[str, Any]] = Store(hass, STORAGE_VERSION, f"{DOMAIN}.{entry_id}.learning")

        # user-controlled state (persisted except manual_target)
        self.target_temperature = config.comfort_temperature
        self.enabled = True
        self.manual_target = False

        # results of the last cycle, read by entities and the building runtime
        self.inputs = RoomInputs(None, False, False, True, None)
        self.mode = RoomMode.COMFORT
        self.output = RoomOutput.idle(config.comfort_temperature, RoomMode.COMFORT)
        self.available = True

    # --- derived values ------------------------------------------------------
    @property
    def has_cooling(self) -> bool:
        return self.config.cooler is not None

    @property
    def learned_heating_rate(self) -> float | None:
        return self.learner.learned_rate

    @property
    def predicted_preheat_minutes(self) -> float:
        if self.inputs.temperature is None:
            return 0.0
        return preheat_minutes(self.inputs.temperature, self.config.comfort_temperature, self.learned_heating_rate)

    # --- user actions --------------------------------------------------------
    async def async_set_target_temperature(self, temperature: float) -> None:
        self.target_temperature = temperature
        self.manual_target = True
        self._schedule_save()
        await self.async_recalculate()

    async def async_set_enabled(self, enabled: bool) -> None:
        self.enabled = enabled
        self._schedule_save()
        await self.async_recalculate()

    def clear_manual_override(self) -> None:
        self.manual_target = False
        self.target_temperature = self.config.comfort_temperature
        self.shading.clear_manual_override()
        self._schedule_save()

    # --- lifecycle -----------------------------------------------------------
    async def _async_setup(self) -> None:
        stored = await self._store.async_load() or {}
        self.learner = HeatingLearner.from_dict(stored.get("learning", stored))
        self.target_temperature = float(stored.get("target_temperature", self.target_temperature))
        self.enabled = bool(stored.get("enabled", self.enabled))
        self._unsubs.append(
            async_track_state_change_event(self.hass, self.config.watched_entities, self._handle_state_change)
        )

    async def _async_shutdown(self) -> None:
        await self._store.async_save(self._storage_data())
        await self.hass.services.async_call("switch", "turn_off", {"entity_id": self.config.heater}, blocking=True)
        if self.config.cooler:
            await self.hass.services.async_call("switch", "turn_off", {"entity_id": self.config.cooler}, blocking=True)

    @callback
    def _handle_state_change(self, event: Event[EventStateChangedData]) -> None:
        self.shading.handle_cover_event(event)
        self.async_schedule_recalculate()

    def _storage_data(self) -> dict[str, Any]:
        return {
            "learning": self.learner.as_dict(),
            "target_temperature": self.target_temperature,
            "enabled": self.enabled,
        }

    def _schedule_save(self) -> None:
        self._store.async_delay_save(self._storage_data, STORAGE_SAVE_DELAY_SECONDS)

    # --- control loop --------------------------------------------------------
    def _read_inputs(self) -> RoomInputs:
        cfg = self.config
        schedule_on = hass_io.is_on(self.hass, cfg.schedule)
        return RoomInputs(
            temperature=hass_io.state_float(self.hass, cfg.temperature_sensor),
            window_open=hass_io.any_on(self.hass, cfg.window_sensors),
            occupied=hass_io.is_on(self.hass, cfg.presence_sensor),
            # Without a schedule the room is always in its comfort period.
            schedule_active=schedule_on if cfg.schedule else True,
            next_comfort_start=(
                hass_io.attribute_datetime(self.hass, cfg.schedule, "next_event")
                if cfg.schedule and not schedule_on
                else None
            ),
        )

    def _actuators_available(self) -> bool:
        cfg = self.config
        return hass_io.is_available(self.hass, cfg.heater) and (
            cfg.cooler is None or hass_io.is_available(self.hass, cfg.cooler)
        )

    def _select_mode(self, now: datetime) -> RoomMode:
        return select_room_mode(
            enabled=self.enabled,
            window_open=self.inputs.window_open,
            manual_target=self.manual_target,
            occupied=self.inputs.occupied,
            schedule_active=self.inputs.schedule_active,
            preheat=is_preheat_window(now, self.inputs.next_comfort_start, self.predicted_preheat_minutes),
        )

    async def _async_update(self) -> None:
        now = dt_util.utcnow()
        cfg = self.config
        self.inputs = self._read_inputs()
        self.mode = self._select_mode(now)
        target = heating_target(self.mode, self.target_temperature, cfg.eco_temperature, cfg.protection_temperature)
        temperature = self.inputs.temperature

        self.available = temperature is not None and self._actuators_available()
        if temperature is None or not self.available:
            self.output = RoomOutput.idle(target, self.mode)
            await self._heater.async_set(False, now)
            await self._cooler.async_set(False, now)
            return

        # Learning looks at the demand that was active during the elapsed interval.
        self._learn(temperature, now)

        self.output = self.controller.calculate(
            RoomInput(
                temperature=temperature,
                target_temperature=target,
                window_open=self.inputs.window_open,
                enabled=self.enabled,
                mode=self.mode,
                cooling_target=(
                    cooling_target(self.mode, cfg.cooling_temperature, cfg.comfort_temperature, cfg.eco_temperature)
                    if self.has_cooling
                    else None
                ),
            )
        )

        heat = pwm_is_on(self.output.heating_demand, now.timestamp(), cfg.pwm_period_seconds)
        cool = not heat and pwm_is_on(self.output.cooling_demand, now.timestamp(), cfg.pwm_period_seconds)
        await self._heater.async_set(heat, now)
        await self._cooler.async_set(cool, now)

        await self.shading.async_update(
            temperature=temperature, window_open=self.inputs.window_open, enabled=self.enabled
        )

    def _learn(self, temperature: float, now: datetime) -> None:
        heating = self.output.heating_demand >= LEARNING_MIN_DEMAND and not self.inputs.window_open
        sample = self._phase.update(temperature, now, heating)
        if sample is not None and self.learner.add_sample(sample):
            self._schedule_save()
