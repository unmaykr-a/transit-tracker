"""Diagnostics download, with the token redacted."""

from __future__ import annotations

from dataclasses import asdict
from typing import Any

from homeassistant.components.diagnostics import async_redact_data
from homeassistant.const import CONF_TOKEN
from homeassistant.core import HomeAssistant

from .coordinator import BusConfigEntry

TO_REDACT = {CONF_TOKEN}


async def async_get_config_entry_diagnostics(hass: HomeAssistant, entry: BusConfigEntry) -> dict[str, Any]:
    coordinator = entry.runtime_data
    return {
        "entry": {"data": async_redact_data(dict(entry.data), TO_REDACT), "options": dict(entry.options)},
        "origin": coordinator.origin() is not None,
        "last_update_success": coordinator.last_update_success,
        "data": asdict(coordinator.data) if coordinator.data else None,
    }
