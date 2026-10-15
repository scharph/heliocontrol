"""Entities built from the pages ("panels") the heliocontrol server describes."""

from __future__ import annotations

from collections.abc import Iterator
from typing import Any

from homeassistant.const import EntityCategory
from homeassistant.helpers.device_registry import DeviceInfo
from homeassistant.helpers.update_coordinator import CoordinatorEntity

from .const import DOMAIN
from .coordinator import HeliocontrolCoordinator


def items(snapshot: dict[str, Any]) -> Iterator[dict[str, Any]]:
    """All page items, each key once (the same value can appear on several pages).

    Items of the mixer circuit pages get the page title as "prefix" so that
    e.g. the three operating modes can be told apart.
    """
    seen: set[str] = set()
    for panel in snapshot.get("panels") or []:
        prefix = panel.get("title") if str(panel.get("id", "")).startswith("mixer") else None
        for it in panel.get("items") or []:
            if it["key"] not in seen:
                seen.add(it["key"])
                yield {**it, "prefix": prefix} if prefix else it


def component(it: dict[str, Any], read_only: bool) -> str:
    """The HA platform of an item; controls become sensors in read-only mode."""
    comp = (it.get("ha") or {}).get("component", "")
    if read_only and comp in ("select", "number"):
        return "sensor"
    return comp


class HeliocontrolEntity(CoordinatorEntity[HeliocontrolCoordinator]):
    """Base for all heliocontrol entities."""

    _attr_has_entity_name = True

    def __init__(self, coordinator: HeliocontrolCoordinator, key: str, name: str | None) -> None:
        super().__init__(coordinator)
        entry = coordinator.config_entry
        snap = coordinator.data or {}
        self._attr_unique_id = f"{entry.unique_id}_{key}"
        if name is not None:  # otherwise the translation_key provides the name
            self._attr_name = name
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
    def lang(self) -> str:
        return "de" if (self.hass.config.language or "").startswith("de") else "en"

    @property
    def online(self) -> bool:
        return bool(self.coordinator.data) and self.coordinator.data.get("status") == "online"

    @property
    def available(self) -> bool:
        return super().available and self.online


class ItemEntity(HeliocontrolEntity):
    """An entity for one page item."""

    def __init__(self, coordinator: HeliocontrolCoordinator, item: dict[str, Any], lang: str) -> None:
        name = item["label"].get(lang) or item["label"]["en"]
        if item.get("prefix"):
            name = f"{item['prefix'].get(lang) or item['prefix']['en']} {name}"
        super().__init__(coordinator, item["key"], name)
        self.item = item
        self._lang = lang
        ha = item.get("ha") or {}
        if ha.get("icon"):
            self._attr_icon = ha["icon"]
        category = ha.get("category")
        if category == "diagnostic":
            self._attr_entity_category = EntityCategory.DIAGNOSTIC
        elif category == "config" and ha.get("component") in ("number", "select"):
            self._attr_entity_category = EntityCategory.CONFIG

    def raw(self) -> float | None:
        """Current value; ratios are computed from two measuring points."""
        snap = self.coordinator.data or {}
        it = self.item
        if it.get("display") == "ratio":
            mp = snap.get("mp") or {}
            n, d = mp.get(str(it.get("num"))), mp.get(str(it.get("den")))
            return None if n is None or not d else round(n / d, 2)
        src = snap.get(it["kind"]) or {}
        return src.get(str(it["id"]))

    def option_labels(self) -> list[str]:
        return [o.get(self._lang) or o["en"] for o in self.item.get("options") or []]

    def option_for(self, value: float | None) -> str | None:
        if value is None:
            return None
        labels = self.option_labels()
        i = int(value) - int(self.item.get("offset") or 0)
        return labels[i] if 0 <= i < len(labels) else None

    @property
    def available(self) -> bool:
        return super().available and self.raw() is not None
