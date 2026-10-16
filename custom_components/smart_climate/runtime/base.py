"""Common lifecycle for all Smart Climate runtimes."""

from __future__ import annotations

import asyncio
from abc import ABC, abstractmethod
from collections.abc import Iterator
from datetime import datetime
from typing import ClassVar

from homeassistant.config_entries import ConfigEntry
from homeassistant.core import CALLBACK_TYPE, HomeAssistant, callback
from homeassistant.helpers.event import async_track_time_interval

from ..const import DOMAIN, UPDATE_INTERVAL

type SmartClimateConfigEntry = ConfigEntry[ControllerRuntime]


class ControllerRuntime(ABC):
    """Owns one config entry's control loop.

    Subclasses implement :meth:`_async_update`. The base class handles the
    periodic tick, serialises recalculations (state-change events and the timer
    can fire concurrently) and notifies entities after every cycle so they no
    longer need polling.
    """

    entry_type: ClassVar[str]
    model: ClassVar[str]

    def __init__(self, hass: HomeAssistant, entry_id: str) -> None:
        self.hass = hass
        self.entry_id = entry_id
        self._lock = asyncio.Lock()
        self._listeners: list[CALLBACK_TYPE] = []
        self._unsubs: list[CALLBACK_TYPE] = []

    # --- lifecycle -----------------------------------------------------------
    async def async_start(self) -> None:
        await self._async_setup()
        self._unsubs.append(async_track_time_interval(self.hass, self._async_tick, UPDATE_INTERVAL))
        await self.async_recalculate()

    async def async_stop(self) -> None:
        while self._unsubs:
            self._unsubs.pop()()
        async with self._lock:
            await self._async_shutdown()

    async def _async_setup(self) -> None:  # noqa: B027 - optional hook
        """Hook: load storage, subscribe to entities."""

    async def _async_shutdown(self) -> None:  # noqa: B027 - optional hook
        """Hook: bring actuators into a safe state."""

    # --- control loop --------------------------------------------------------
    async def async_recalculate(self) -> None:
        async with self._lock:
            await self._async_update()
        self._notify_listeners()

    @abstractmethod
    async def _async_update(self) -> None:
        """Read inputs, run the pure controller, write actuators."""

    async def _async_tick(self, _now: datetime) -> None:
        await self.async_recalculate()

    @callback
    def async_schedule_recalculate(self, *_args: object) -> None:
        """Event-handler friendly trigger."""
        self.hass.async_create_task(self.async_recalculate(), eager_start=True)

    # --- entity notifications -----------------------------------------------
    @callback
    def async_add_listener(self, update_callback: CALLBACK_TYPE) -> CALLBACK_TYPE:
        self._listeners.append(update_callback)

        @callback
        def remove() -> None:
            self._listeners.remove(update_callback)

        return remove

    @callback
    def _notify_listeners(self) -> None:
        for update_callback in list(self._listeners):
            update_callback()


def iter_runtimes[T: ControllerRuntime](hass: HomeAssistant, kind: type[T]) -> Iterator[T]:
    """All loaded runtimes of the given type."""
    for entry in hass.config_entries.async_entries(DOMAIN):
        runtime = getattr(entry, "runtime_data", None)
        if isinstance(runtime, kind):
            yield runtime


def find_runtime[T: ControllerRuntime](hass: HomeAssistant, kind: type[T]) -> T | None:
    return next(iter_runtimes(hass, kind), None)
