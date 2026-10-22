"""Bus: Tallinn departures from a self-hosted Bus server as sensors."""

from __future__ import annotations

from pathlib import Path

from homeassistant.components.frontend import add_extra_js_url
from homeassistant.components.http import StaticPathConfig
from homeassistant.const import CONF_TOKEN, CONF_URL, CONF_VERIFY_SSL, Platform
from homeassistant.core import HomeAssistant
from homeassistant.helpers import config_validation as cv, device_registry as dr
from homeassistant.helpers.aiohttp_client import async_get_clientsession
from homeassistant.helpers.typing import ConfigType
from homeassistant.loader import async_get_integration

from .api import BusClient
from .const import DOMAIN
from .coordinator import BusConfigEntry, BusCoordinator
from .entity import selected_lines

PLATFORMS = [Platform.BINARY_SENSOR, Platform.SENSOR]

CONFIG_SCHEMA = cv.config_entry_only_config_schema(DOMAIN)

# The Lovelace card ships inside the integration (release zips include it) and
# is loaded on every dashboard, so it never has to be added as a resource.
CARD_FILE = Path(__file__).parent / "www" / "bus-card.js"
CARD_URL = "/bus_card/bus-card.js"


async def async_setup(hass: HomeAssistant, config: ConfigType) -> bool:
    """Serve the bundled bus-card and load it in the frontend."""
    if hass.http is not None and await hass.async_add_executor_job(CARD_FILE.is_file):
        version = (await async_get_integration(hass, DOMAIN)).version
        await hass.http.async_register_static_paths([StaticPathConfig(CARD_URL, str(CARD_FILE), cache_headers=False)])
        add_extra_js_url(hass, f"{CARD_URL}?v={version}")  # the version busts browser caches on upgrade
    return True


async def async_setup_entry(hass: HomeAssistant, entry: BusConfigEntry) -> bool:
    """Set up a Bus server connection."""
    session = async_get_clientsession(hass, verify_ssl=entry.data.get(CONF_VERIFY_SSL, True))
    client = BusClient(session, entry.data[CONF_URL], entry.data[CONF_TOKEN], entry.data.get(CONF_VERIFY_SSL, True))
    coordinator = BusCoordinator(hass, entry, client)
    await coordinator.async_config_entry_first_refresh()
    entry.runtime_data = coordinator
    _remove_stale_devices(hass, entry)
    entry.async_on_unload(entry.add_update_listener(_reload))
    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)
    return True


async def async_unload_entry(hass: HomeAssistant, entry: BusConfigEntry) -> bool:
    """Unload a config entry."""
    return await hass.config_entries.async_unload_platforms(entry, PLATFORMS)


async def _reload(hass: HomeAssistant, entry: BusConfigEntry) -> None:
    """Options changed (lines, interval, origin): rebuild entities.

    Reauth and reconfigure update the entry's data and reload it themselves.
    """
    if entry.options != entry.runtime_data.options:
        await hass.config_entries.async_reload(entry.entry_id)


def _remove_stale_devices(hass: HomeAssistant, entry: BusConfigEntry) -> None:
    """Drop devices (and their entities) of lines deselected in the options or deleted on the server."""
    keep = {f"{entry.unique_id}_{lid}" for lid in selected_lines(entry)}
    registry = dr.async_get(hass)
    for device in dr.async_entries_for_config_entry(registry, entry.entry_id):
        if not any(domain == DOMAIN and ident in keep for domain, ident in device.identifiers):
            registry.async_update_device(device.id, remove_config_entry_id=entry.entry_id)
