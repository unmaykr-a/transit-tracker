"""Polls every saved line in one request per interval."""

from __future__ import annotations

from dataclasses import dataclass
from datetime import timedelta
import logging
from typing import Any

from homeassistant.config_entries import ConfigEntry
from homeassistant.const import ATTR_LATITUDE, ATTR_LONGITUDE
from homeassistant.core import HomeAssistant
from homeassistant.exceptions import ConfigEntryAuthFailed
from homeassistant.helpers.update_coordinator import DataUpdateCoordinator, UpdateFailed

from .api import TramAuthError, TramClient, TramError
from .const import CONF_ORIGIN, CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL, DOMAIN

_LOGGER = logging.getLogger(__name__)


@dataclass
class TramData:
    """One poll: saved lines keyed by id (as a string), and whether walking times were computed."""

    lines: dict[str, dict[str, Any]]
    has_origin: bool


type TramConfigEntry = ConfigEntry[TramCoordinator]


class TramCoordinator(DataUpdateCoordinator[TramData]):
    """Fetches /api/v1/ha/lines with the origin entity's current position."""

    config_entry: TramConfigEntry

    def __init__(self, hass: HomeAssistant, entry: TramConfigEntry, client: TramClient) -> None:
        super().__init__(
            hass,
            _LOGGER,
            config_entry=entry,
            name=DOMAIN,
            update_interval=timedelta(seconds=entry.options.get(CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL)),
        )
        self.client = client

    def origin(self) -> tuple[float, float] | None:
        """Position of the origin entity (zone, person or device tracker), if it has one."""
        entity_id = self.config_entry.options.get(CONF_ORIGIN)
        if not entity_id:
            return None
        state = self.hass.states.get(entity_id)
        if state is None:
            return None
        lat, lon = state.attributes.get(ATTR_LATITUDE), state.attributes.get(ATTR_LONGITUDE)
        if not isinstance(lat, (int, float)) or not isinstance(lon, (int, float)):
            return None
        return float(lat), float(lon)

    async def _async_update_data(self) -> TramData:
        try:
            res = await self.client.lines(self.origin())
        except TramAuthError as err:
            raise ConfigEntryAuthFailed(str(err)) from err
        except TramError as err:
            raise UpdateFailed(str(err)) from err
        return TramData(lines={str(line["id"]): line for line in res.get("lines", [])}, has_origin=bool(res.get("has_origin")))
