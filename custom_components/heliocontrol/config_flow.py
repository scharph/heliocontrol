"""Config flow: connect Home Assistant to a heliocontrol server."""

from __future__ import annotations

from collections.abc import Mapping
from typing import Any

import aiohttp
import voluptuous as vol

from homeassistant.config_entries import ConfigFlow, ConfigFlowResult
from homeassistant.const import CONF_PASSWORD

from .api import AuthError, HeliocontrolClient, HeliocontrolError
from .const import CONF_URL, DOMAIN


async def _probe(hass, url: str, password: str | None) -> dict[str, Any]:
    # a short-lived session of our own (HA forbids closing the ones it creates)
    async with aiohttp.ClientSession(cookie_jar=aiohttp.CookieJar(unsafe=True)) as session:
        client = HeliocontrolClient(session, url, password)
        await client.login()
        return await client.state()


def _normalize(url: str) -> str:
    url = url.strip().rstrip("/")
    if "://" not in url:
        url = "http://" + url
    return url


class HeliocontrolConfigFlow(ConfigFlow, domain=DOMAIN):
    """Ask for the server URL (and password, if the server has one)."""

    VERSION = 1

    async def async_step_user(self, user_input: dict[str, Any] | None = None) -> ConfigFlowResult:
        errors: dict[str, str] = {}
        if user_input is not None:
            url = _normalize(user_input[CONF_URL])
            password = user_input.get(CONF_PASSWORD) or None
            try:
                state = await _probe(self.hass, url, password)
            except AuthError:
                errors["base"] = "invalid_auth"
            except HeliocontrolError:
                errors["base"] = "cannot_connect"
            else:
                # the controller ID identifies the heat pump; fall back to the URL
                # while the server has not reached the heat pump yet
                await self.async_set_unique_id(str(state.get("deviceId") or url))
                self._abort_if_unique_id_configured(updates={CONF_URL: url})
                title = f"Heliotherm {state['deviceId']}" if state.get("deviceId") else "Heliotherm"
                return self.async_create_entry(title=title, data={CONF_URL: url, CONF_PASSWORD: password})
        return self.async_show_form(
            step_id="user",
            data_schema=self.add_suggested_values_to_schema(
                vol.Schema({vol.Required(CONF_URL): str, vol.Optional(CONF_PASSWORD): str}),
                user_input or {CONF_URL: "http://"},
            ),
            errors=errors,
        )

    async def async_step_reauth(self, entry_data: Mapping[str, Any]) -> ConfigFlowResult:
        return await self.async_step_reauth_confirm()

    async def async_step_reauth_confirm(self, user_input: dict[str, Any] | None = None) -> ConfigFlowResult:
        errors: dict[str, str] = {}
        entry = self._get_reauth_entry()
        if user_input is not None:
            try:
                await _probe(self.hass, entry.data[CONF_URL], user_input[CONF_PASSWORD])
            except AuthError:
                errors["base"] = "invalid_auth"
            except HeliocontrolError:
                errors["base"] = "cannot_connect"
            else:
                return self.async_update_reload_and_abort(entry, data_updates={CONF_PASSWORD: user_input[CONF_PASSWORD]})
        return self.async_show_form(
            step_id="reauth_confirm",
            data_schema=vol.Schema({vol.Required(CONF_PASSWORD): str}),
            errors=errors,
        )
