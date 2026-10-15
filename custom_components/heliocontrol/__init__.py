"""Heliotherm heat pumps through a heliocontrol server."""

from __future__ import annotations

import aiohttp

from homeassistant.const import CONF_PASSWORD
from homeassistant.core import HomeAssistant, callback
from homeassistant.exceptions import ConfigEntryAuthFailed, ConfigEntryNotReady
from homeassistant.helpers.aiohttp_client import async_create_clientsession

from .api import AuthError, HeliocontrolClient, HeliocontrolError
from .const import CONF_URL, PLATFORMS
from .coordinator import HeliocontrolConfigEntry, HeliocontrolCoordinator, structure


async def async_setup_entry(hass: HomeAssistant, entry: HeliocontrolConfigEntry) -> bool:
    """Set up a heliocontrol server."""
    # own session: the login cookie must not leak into HA's shared session;
    # unsafe=True keeps cookies for servers addressed by IP
    session = async_create_clientsession(hass, cookie_jar=aiohttp.CookieJar(unsafe=True))
    client = HeliocontrolClient(session, entry.data[CONF_URL], entry.data.get(CONF_PASSWORD))
    try:
        await client.login()
    except AuthError as err:
        raise ConfigEntryAuthFailed(str(err)) from err
    except HeliocontrolError as err:
        raise ConfigEntryNotReady(str(err)) from err

    coordinator = HeliocontrolCoordinator(hass, entry, client)
    await coordinator.async_config_entry_first_refresh()
    entry.runtime_data = coordinator

    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)

    entry.async_create_background_task(hass, coordinator.listen(), "heliocontrol events")

    # entities follow the server's pages and read-only mode: reload when they change
    initial = structure(coordinator.data)

    @callback
    def _check_structure() -> None:
        if coordinator.data and structure(coordinator.data) != initial and coordinator.data.get("deviceId"):
            hass.config_entries.async_schedule_reload(entry.entry_id)

    entry.async_on_unload(coordinator.async_add_listener(_check_structure))
    return True


async def async_unload_entry(hass: HomeAssistant, entry: HeliocontrolConfigEntry) -> bool:
    """Unload a config entry."""
    return await hass.config_entries.async_unload_platforms(entry, PLATFORMS)
