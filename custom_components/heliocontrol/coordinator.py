"""Keeps the heliocontrol snapshot current: live events with a polling fallback."""

from __future__ import annotations

import asyncio
from datetime import timedelta
import logging
from typing import Any

from homeassistant.config_entries import ConfigEntry
from homeassistant.core import HomeAssistant
from homeassistant.exceptions import ConfigEntryAuthFailed
from homeassistant.helpers.update_coordinator import DataUpdateCoordinator, UpdateFailed

from .api import AuthError, HeliocontrolClient, HeliocontrolError
from .const import DOMAIN, FALLBACK_POLL_SECONDS

_LOGGER = logging.getLogger(__name__)

type HeliocontrolConfigEntry = ConfigEntry[HeliocontrolCoordinator]


def structure(snapshot: dict[str, Any]) -> tuple:
    """What the entity set depends on; a change requires reloading the entry."""
    keys = sorted({it["key"] for p in snapshot.get("panels") or [] for it in p.get("items") or []})
    return (snapshot.get("deviceId"), bool(snapshot.get("readOnly", True)), tuple(keys))


class HeliocontrolCoordinator(DataUpdateCoordinator[dict[str, Any]]):
    """Holds the server snapshot (status, pages, values)."""

    config_entry: HeliocontrolConfigEntry

    def __init__(self, hass: HomeAssistant, entry: HeliocontrolConfigEntry, client: HeliocontrolClient) -> None:
        super().__init__(
            hass,
            _LOGGER,
            config_entry=entry,
            name=DOMAIN,
            update_interval=timedelta(seconds=FALLBACK_POLL_SECONDS),
        )
        self.client = client

    async def _async_update_data(self) -> dict[str, Any]:
        try:
            return await self.client.state()
        except AuthError as err:
            raise ConfigEntryAuthFailed(str(err)) from err
        except HeliocontrolError as err:
            raise UpdateFailed(str(err)) from err

    def _apply(self, event: str, data: Any) -> None:
        cur = self.data or {}
        if event == "state":
            self.async_set_updated_data(data)
        elif event == "values" and cur:
            self.async_set_updated_data(
                {
                    **cur,
                    "mp": {**(cur.get("mp") or {}), **(data.get("mp") or {})},
                    "sp": {**(cur.get("sp") or {}), **(data.get("sp") or {})},
                    "updated": data.get("updated"),
                }
            )

    async def listen(self) -> None:
        """Follow the server's event stream forever, reconnecting with backoff."""
        delay = 5
        while True:
            try:
                async for event, data in self.client.events():
                    delay = 5
                    self._apply(event, data)
            except asyncio.CancelledError:
                raise
            except AuthError:
                try:
                    await self.client.login()
                except HeliocontrolError as err:
                    _LOGGER.warning("heliocontrol login failed: %s", err)
            except (HeliocontrolError, ValueError) as err:
                _LOGGER.debug("heliocontrol event stream ended: %s", err)
            except Exception:  # noqa: BLE001 - keep the listener alive
                _LOGGER.exception("unexpected error in heliocontrol event stream")
            await asyncio.sleep(delay)
            delay = min(delay * 2, 120)
