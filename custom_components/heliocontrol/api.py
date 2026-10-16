"""Client for the heliocontrol server API (REST + server-sent events)."""

from __future__ import annotations

from collections.abc import AsyncIterator
import json
from typing import Any

import aiohttp


class HeliocontrolError(Exception):
    """The server could not be reached or answered with an error."""


class AuthError(HeliocontrolError):
    """The server requires a (different) password."""


class HeliocontrolClient:
    """Talks to one heliocontrol server."""

    def __init__(self, session: aiohttp.ClientSession, url: str, password: str | None = None) -> None:
        self._session = session
        self.url = url.rstrip("/")
        self._password = password or ""

    async def _request(self, method: str, path: str, body: Any = None, *, retry_auth: bool = True) -> Any:
        try:
            async with self._session.request(
                method, self.url + path, json=body, timeout=aiohttp.ClientTimeout(total=120)
            ) as resp:
                if resp.status == 401 and retry_auth and path != "/api/login":
                    await self.login()
                    return await self._request(method, path, body, retry_auth=False)
                if resp.status == 401:
                    raise AuthError("login required")
                text = await resp.text()
                if resp.status >= 400:
                    try:
                        msg = json.loads(text).get("error", text)
                    except ValueError:
                        msg = text
                    raise HeliocontrolError(f"{resp.status}: {msg}")
                return json.loads(text) if text else None
        except (aiohttp.ClientError, TimeoutError) as err:
            raise HeliocontrolError(f"cannot reach {self.url}: {err}") from err

    async def login(self) -> None:
        """Log in if the server is password protected."""
        auth = await self._request("GET", "/api/auth", retry_auth=False)
        if not auth.get("required") or auth.get("authenticated"):
            return
        if not self._password:
            raise AuthError("the server is password protected")
        try:
            await self._request("POST", "/api/login", {"password": self._password}, retry_auth=False)
        except HeliocontrolError as err:
            raise AuthError(str(err)) from err

    async def state(self) -> dict[str, Any]:
        return await self._request("GET", "/api/state")

    async def entities(self) -> list[dict[str, Any]]:
        """Home Assistant entity descriptions built by the server (internal/haentity)."""
        return await self._request("GET", "/api/ha/entities")

    async def set_temporary(self, sp_id: int, value: float, minutes: float, source: str = "") -> dict[str, Any]:
        return await self._request(
            "POST", "/api/overrides", {"sp": sp_id, "value": value, "minutes": minutes, "source": source}
        )

    async def cancel_temporary(self, sp_id: int) -> None:
        await self._request("DELETE", f"/api/overrides/{sp_id}")

    async def write_sp(self, sp_id: int, value: float) -> float:
        res = await self._request("PUT", f"/api/sp/{sp_id}", {"value": value})
        return res["value"]

    async def refresh(self) -> None:
        await self._request("POST", "/api/refresh")

    async def reset_fault(self) -> None:
        await self._request("POST", "/api/fault/reset")

    async def sync_clock(self) -> None:
        await self._request("POST", "/api/clock/sync")

    async def events(self) -> AsyncIterator[tuple[str, Any]]:
        """Yield (event type, data) from the live event stream until it ends."""
        timeout = aiohttp.ClientTimeout(total=None, sock_connect=15, sock_read=90)  # server pings every 25 s
        async with self._session.get(self.url + "/api/events", timeout=timeout) as resp:
            if resp.status == 401:
                raise AuthError("login required")
            if resp.status != 200:
                raise HeliocontrolError(f"event stream: HTTP {resp.status}")
            event, data = "message", []
            async for raw in resp.content:
                line = raw.decode("utf-8").rstrip("\r\n")
                if not line:
                    if data:
                        yield event, json.loads("\n".join(data))
                    event, data = "message", []
                elif line.startswith(":"):
                    continue  # keep-alive comment
                elif line.startswith("event:"):
                    event = line[6:].strip()
                elif line.startswith("data:"):
                    data.append(line[5:].lstrip())
