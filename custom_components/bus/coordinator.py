"""Polls every saved line in one request per interval."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import timedelta
import logging
from typing import Any

from homeassistant.config_entries import ConfigEntry
from homeassistant.const import ATTR_LATITUDE, ATTR_LONGITUDE
from homeassistant.core import HomeAssistant, State
from homeassistant.exceptions import ConfigEntryAuthFailed
from homeassistant.helpers.update_coordinator import DataUpdateCoordinator, UpdateFailed

from .api import BusAuthError, BusClient, BusError
from .const import CONF_ORIGIN, CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL, DOMAIN

_LOGGER = logging.getLogger(__name__)


@dataclass
class BusData:
    """One poll: saved lines keyed by id (as a string), and whether walking times were computed."""

    lines: dict[str, dict[str, Any]]
    has_origin: bool


type BusConfigEntry = ConfigEntry[BusCoordinator]


def _position(state: State) -> tuple[float, float] | None:
    lat, lon = state.attributes.get(ATTR_LATITUDE), state.attributes.get(ATTR_LONGITUDE)
    if not isinstance(lat, (int, float)) or not isinstance(lon, (int, float)):
        return None
    # A tracker without a fix may report 0,0: treat it (and nonsense) as "no position" rather than failing the poll.
    if not (-90 <= lat <= 90 and -180 <= lon <= 180) or (lat == 0 and lon == 0):
        return None
    return float(lat), float(lon)


class BusCoordinator(DataUpdateCoordinator[BusData]):
    """Fetches /api/v1/ha/lines with the origin entity's current position."""

    config_entry: BusConfigEntry

    def __init__(self, hass: HomeAssistant, entry: BusConfigEntry, client: BusClient) -> None:
        super().__init__(
            hass,
            _LOGGER,
            config_entry=entry,
            name=DOMAIN,
            update_interval=timedelta(seconds=entry.options.get(CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL)),
        )
        self.client = client
        # The options this setup was built from: only their change needs a reload.
        self.options = dict(entry.options)

    def origin(self) -> tuple[float, float] | None:
        """Position of the origin entity (zone, person or device tracker), if it has one.

        A person or tracker without GPS coordinates (e.g. tracked by a router) is
        placed at the zone it is in. Without any position the server falls back
        to each line's own starting place, if one is set in Bus.
        """
        entity_id = self.config_entry.options.get(CONF_ORIGIN)
        if not entity_id:
            return None
        state = self.hass.states.get(entity_id)
        if state is None:
            return None
        if (pos := _position(state)) is not None:
            return pos
        zone = self.hass.states.get(f"zone.{state.state}")
        if zone is None:  # zones are named by their friendly name in a person's state
            zone = next((z for z in self.hass.states.async_all("zone") if z.name == state.state), None)
        return _position(zone) if zone is not None else None

    async def _async_update_data(self) -> BusData:
        try:
            res = await self.client.lines(self.origin())
        except BusAuthError as err:
            raise ConfigEntryAuthFailed(translation_domain=DOMAIN, translation_key="token_rejected") from err
        except BusError as err:
            raise UpdateFailed(
                translation_domain=DOMAIN, translation_key="update_failed", translation_placeholders={"error": str(err)}
            ) from err
        return BusData(lines={str(line["id"]): line for line in res.get("lines", [])}, has_origin=bool(res.get("has_origin")))

    def order(self, line_id: str) -> int:
        """Position of a line in the user's saved order (the server's)."""
        lines = list(self.data.lines) if self.data else []
        return lines.index(line_id) if line_id in lines else len(lines)
