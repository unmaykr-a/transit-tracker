"""Tram: Tallinn departures from a self-hosted Tram server as sensors."""

from __future__ import annotations

from homeassistant.const import CONF_TOKEN, CONF_URL, CONF_VERIFY_SSL, Platform
from homeassistant.core import HomeAssistant
from homeassistant.helpers import device_registry as dr
from homeassistant.helpers.aiohttp_client import async_get_clientsession

from .api import TramClient
from .const import DOMAIN
from .coordinator import TramConfigEntry, TramCoordinator
from .entity import selected_lines

PLATFORMS = [Platform.BINARY_SENSOR, Platform.SENSOR]


async def async_setup_entry(hass: HomeAssistant, entry: TramConfigEntry) -> bool:
    """Set up a Tram server connection."""
    session = async_get_clientsession(hass, verify_ssl=entry.data.get(CONF_VERIFY_SSL, True))
    client = TramClient(session, entry.data[CONF_URL], entry.data[CONF_TOKEN], entry.data.get(CONF_VERIFY_SSL, True))
    coordinator = TramCoordinator(hass, entry, client)
    await coordinator.async_config_entry_first_refresh()
    entry.runtime_data = coordinator
    _remove_stale_devices(hass, entry)
    entry.async_on_unload(entry.add_update_listener(_reload))
    await hass.config_entries.async_forward_entry_setups(entry, PLATFORMS)
    return True


async def async_unload_entry(hass: HomeAssistant, entry: TramConfigEntry) -> bool:
    """Unload a config entry."""
    return await hass.config_entries.async_unload_platforms(entry, PLATFORMS)


async def _reload(hass: HomeAssistant, entry: TramConfigEntry) -> None:
    """Options changed (lines, interval, origin): rebuild entities."""
    await hass.config_entries.async_reload(entry.entry_id)


def _remove_stale_devices(hass: HomeAssistant, entry: TramConfigEntry) -> None:
    """Drop devices (and their entities) of lines deselected in the options or deleted on the server."""
    keep = {f"{entry.unique_id}_{lid}" for lid in selected_lines(entry)}
    registry = dr.async_get(hass)
    for device in dr.async_entries_for_config_entry(registry, entry.entry_id):
        if not any(domain == DOMAIN and ident in keep for domain, ident in device.identifiers):
            registry.async_update_device(device.id, remove_config_entry_id=entry.entry_id)
