"""Next departure (timestamp) and minutes until you must leave."""

from __future__ import annotations

from datetime import datetime
from typing import Any

from homeassistant.components.sensor import SensorDeviceClass, SensorEntity
from homeassistant.const import UnitOfTime
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback
from homeassistant.util import dt as dt_util

from .coordinator import BusConfigEntry
from .entity import BusLineEntity, selected_lines

PARALLEL_UPDATES = 0


async def async_setup_entry(hass: HomeAssistant, entry: BusConfigEntry, async_add_entities: AddConfigEntryEntitiesCallback) -> None:
    coordinator = entry.runtime_data
    entities: list[SensorEntity] = []
    for lid in selected_lines(entry):
        entities += [BusNextSensor(coordinator, lid, "next"), BusLeaveInSensor(coordinator, lid, "leave_in")]
    async_add_entities(entities)


def _time(value: Any) -> datetime | None:
    return dt_util.parse_datetime(value) if isinstance(value, str) else None


MODE_ICONS = {"tram": "mdi:tram", "bus": "mdi:bus", "trolley": "mdi:bus-electric", "train": "mdi:train"}


class BusNextSensor(BusLineEntity, SensorEntity):
    """When the next departure leaves (the next one still catchable, given an origin)."""

    _attr_device_class = SensorDeviceClass.TIMESTAMP
    # These change on every poll; keeping them out of the recorder saves a lot of database rows.
    _unrecorded_attributes = frozenset(
        {
            "next_departures",
            "changes",
            "arrive_at",
            "minutes",
            "delay_s",
            "realtime",
            "leave_in_min",
            "verdict",
            "catchable",
            "walk_min",
            "walk_approx",
            "order",
            "url",
            "error",
        }
    )

    @property
    def icon(self) -> str | None:
        return MODE_ICONS.get(self.line.get("mode", ""))

    @property
    def native_value(self) -> datetime | None:
        nxt = self.line.get("next")
        return _time(nxt.get("time")) if nxt else None

    @property
    def extra_state_attributes(self) -> dict[str, Any]:
        line = self.line
        nxt = line.get("next") or {}
        return {
            "line": line.get("line"),
            "mode": line.get("mode"),
            "label": line.get("label"),
            "kind": line.get("kind", "line"),
            "lines": line.get("lines", []),
            "order": self.coordinator.order(self.line_id),
            "walk_from": line.get("walk_from"),
            # A trip's changes (stop names) and, for the next departure, its onward rides and arrival.
            "via": line.get("via", []),
            "changes": nxt.get("changes", []),
            "arrive_at": nxt.get("arrive_at"),
            "headsign": nxt.get("headsign") or line.get("headsign"),
            # The saved line's direction, or a trip's destination stop (headsign is the next departure's).
            "destination": line.get("headsign"),
            "stop_name": line.get("stop_name"),
            "platform": line.get("platform"),
            "minutes": nxt.get("minutes"),
            "delay_s": nxt.get("delay_s"),
            "realtime": nxt.get("realtime"),
            "next_departures": line.get("next_departures", []),
            "walk_min": line.get("walk_min"),
            "walk_approx": line.get("walk_approx"),
            "verdict": line.get("verdict"),
            "leave_in_min": line.get("leave_in_min"),
            "catchable": line.get("catchable"),
            "url": self.coordinator.client.base_url + (line.get("path") or ""),
            "error": line.get("error"),  # e.g. "departures unavailable" when the upstream feed failed
        }


class BusLeaveInSensor(BusLineEntity, SensorEntity):
    """Minutes until you must leave for the next catchable departure (needs an origin)."""

    _attr_device_class = SensorDeviceClass.DURATION
    _attr_native_unit_of_measurement = UnitOfTime.MINUTES
    _attr_suggested_display_precision = 0

    @property
    def native_value(self) -> int | None:
        value = self.line.get("leave_in_min")
        return value if isinstance(value, int) else None
