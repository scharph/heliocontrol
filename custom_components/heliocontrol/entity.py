"""Entities built from the server's entity descriptions (GET /api/ha/entities).

The server (internal/haentity) decides which entities exist and how they are
classified, for this integration and for the MQTT bridge alike; this module
only renders them.
"""

from __future__ import annotations

from typing import Any

from homeassistant.const import EntityCategory
from homeassistant.core import HomeAssistant
from homeassistant.helpers.device_registry import DeviceInfo
from homeassistant.helpers.update_coordinator import CoordinatorEntity

from .const import DOMAIN
from .coordinator import HeliocontrolCoordinator


def language(hass: HomeAssistant) -> str:
    return "de" if (hass.config.language or "").startswith("de") else "en"


def label(text: dict[str, str], lang: str) -> str:
    return text.get(lang) or text.get("en") or ""


def of_component(coordinator: HeliocontrolCoordinator, component: str) -> list[dict[str, Any]]:
    return [d for d in coordinator.entities if d.get("component") == component]


class DescribedEntity(CoordinatorEntity[HeliocontrolCoordinator]):
    """An entity described by the server."""

    _attr_has_entity_name = True

    def __init__(self, coordinator: HeliocontrolCoordinator, desc: dict[str, Any], lang: str) -> None:
        super().__init__(coordinator)
        entry = coordinator.config_entry
        snap = coordinator.data or {}
        self.desc = desc
        self._lang = lang
        self._attr_unique_id = f"{entry.unique_id}_{desc['key']}"
        self._attr_name = label(desc["name"], lang)
        if desc.get("icon"):
            self._attr_icon = desc["icon"]
        category = desc.get("category")
        if category == "diagnostic":
            self._attr_entity_category = EntityCategory.DIAGNOSTIC
        elif category == "config":
            self._attr_entity_category = EntityCategory.CONFIG
        self._attr_device_info = DeviceInfo(
            identifiers={(DOMAIN, str(entry.unique_id))},
            name="Heliotherm",
            manufacturer="Heliotherm",
            model="web control",
            sw_version=snap.get("firmware") or None,
            serial_number=str(snap["deviceId"]) if snap.get("deviceId") else None,
            configuration_url=coordinator.client.url,
        )

    @property
    def online(self) -> bool:
        return bool(self.coordinator.data) and self.coordinator.data.get("status") == "online"

    @property
    def available(self) -> bool:
        return super().available and self.online

    def raw(self) -> float | None:
        """Current value; ratios are computed from two measuring points."""
        snap = self.coordinator.data or {}
        d = self.desc
        if d.get("display") == "ratio":
            mp = snap.get("mp") or {}
            n, den = mp.get(str(d.get("num"))), mp.get(str(d.get("den")))
            return None if n is None or not den else round(n / den, 2)
        kind = d.get("kind")
        if kind not in ("mp", "sp"):
            return None
        return (snap.get(kind) or {}).get(str(d.get("id")))

    def option_labels(self) -> list[str]:
        return [label(o, self._lang) for o in self.desc.get("options") or []]

    def option_for(self, value: float | None) -> str | None:
        if value is None:
            return None
        labels = self.option_labels()
        i = int(value) - int(self.desc.get("offset") or 0)
        return labels[i] if 0 <= i < len(labels) else None


class ValueEntity(DescribedEntity):
    """A described entity backed by a point value."""

    @property
    def available(self) -> bool:
        return super().available and self.raw() is not None
