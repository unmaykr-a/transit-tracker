"""Base entity: one device per saved line."""

from __future__ import annotations

from typing import Any

from homeassistant.helpers.device_registry import DeviceEntryType, DeviceInfo
from homeassistant.helpers.update_coordinator import CoordinatorEntity

from .const import CONF_LINES, DOMAIN
from .coordinator import TramConfigEntry, TramCoordinator

MODE_NAMES = {"tram": "Tram", "bus": "Bus", "trolley": "Trolleybus", "train": "Train"}


def selected_lines(entry: TramConfigEntry) -> list[str]:
    """Line ids to create entities for: the options' selection, else every saved line."""
    data = entry.runtime_data.data
    available = list(data.lines) if data else []
    chosen = entry.options.get(CONF_LINES)
    return available if chosen is None else [lid for lid in chosen if lid in available]


class TramLineEntity(CoordinatorEntity[TramCoordinator]):
    """An entity about one saved line."""

    _attr_has_entity_name = True

    def __init__(self, coordinator: TramCoordinator, line_id: str, key: str) -> None:
        super().__init__(coordinator)
        self.line_id = line_id
        line = self.line
        entry = coordinator.config_entry
        self._attr_unique_id = f"{entry.unique_id}_{line_id}_{key}"
        self._attr_translation_key = key
        label = line.get("label") or line.get("line") or line_id
        self._attr_device_info = DeviceInfo(
            identifiers={(DOMAIN, f"{entry.unique_id}_{line_id}")},
            name=f"Tram {label}",
            manufacturer="Tram",
            model=f"{MODE_NAMES.get(line.get('mode', ''), 'Line')} {line.get('line', '')} → {line.get('headsign', '')}",
            entry_type=DeviceEntryType.SERVICE,
            configuration_url=coordinator.client.base_url + (line.get("path") or ""),
        )

    @property
    def line(self) -> dict[str, Any]:
        data = self.coordinator.data
        return data.lines.get(self.line_id, {}) if data else {}

    @property
    def available(self) -> bool:
        line = self.line
        return super().available and bool(line) and not line.get("unavailable")
