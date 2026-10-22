"""Entities, origin handling, availability and reauth."""

from __future__ import annotations

from datetime import timedelta

from freezegun.api import FrozenDateTimeFactory

from homeassistant.config_entries import SOURCE_REAUTH, ConfigEntryState
from homeassistant.const import CONF_TOKEN, CONF_URL, CONF_VERIFY_SSL, STATE_ON, STATE_UNAVAILABLE, STATE_UNKNOWN
from homeassistant.core import HomeAssistant
from homeassistant.setup import async_setup_component
from pytest_homeassistant_custom_component.common import MockConfigEntry, async_fire_time_changed

from custom_components.bus.const import CONF_ORIGIN, DOMAIN
from custom_components.bus.diagnostics import async_get_config_entry_diagnostics

from .conftest import TOKEN, URL, line


async def setup_entry(hass: HomeAssistant, **options) -> MockConfigEntry:
    entry = MockConfigEntry(
        domain=DOMAIN, unique_id="bus.example.com:andri", data={CONF_URL: URL, CONF_TOKEN: TOKEN, CONF_VERIFY_SSL: True}, options=options
    )
    entry.add_to_hass(hass)
    assert await hass.config_entries.async_setup(entry.entry_id)
    await hass.async_block_till_done()
    return entry


async def test_entities(hass: HomeAssistant, bus_server) -> None:
    await setup_entry(hass)
    nxt = hass.states.get("sensor.bus_work_next")
    assert nxt is not None and nxt.state not in (STATE_UNKNOWN, STATE_UNAVAILABLE)
    assert nxt.attributes["device_class"] == "timestamp"
    attrs = nxt.attributes
    assert attrs["line"] == "T4" and attrs["stop_name"] == "Hobujaama" and attrs["platform"] == "A"
    assert attrs["minutes"] == 9 and attrs["verdict"] == "green" and attrs["walk_min"] == 4 and attrs["leave_in_min"] == 4
    assert len(attrs["next_departures"]) == 5
    assert attrs["url"] == f"{URL}/lines/tram/T4?stop=1234-1"

    leave = hass.states.get("sensor.bus_work_leave_in")
    assert leave.state == "4" and leave.attributes["unit_of_measurement"] == "min"
    assert hass.states.get("binary_sensor.bus_work_catchable").state == STATE_ON

    # A line without an origin-based verdict: leave-in and catchable are unknown.
    assert hass.states.get("sensor.bus_gym_leave_in").state == STATE_UNKNOWN
    assert hass.states.get("binary_sensor.bus_gym_catchable").state == STATE_UNKNOWN


async def test_origin_is_sent(hass: HomeAssistant, bus_server) -> None:
    await async_setup_component(hass, "zone", {})
    hass.states.async_set("person.andri", "home", {"latitude": 59.4405, "longitude": 24.7375})
    await setup_entry(hass, **{CONF_ORIGIN: "person.andri"})
    url = str(bus_server.mock_calls[-1][1])
    assert "origin=59.440500,24.737500" in url and "departures=5" in url


async def test_origin_from_zone_without_gps(hass: HomeAssistant, bus_server) -> None:
    """A person tracked without coordinates (e.g. by the router) walks from the zone they are in."""
    await async_setup_component(hass, "zone", {"zone": [{"name": "Work", "latitude": 59.43, "longitude": 24.75, "radius": 100}]})
    hass.states.async_set("person.andri", "Work", {"source": "device_tracker.router"})
    await setup_entry(hass, **{CONF_ORIGIN: "person.andri"})
    assert "origin=59.430000,24.750000" in str(bus_server.mock_calls[-1][1])

    hass.states.async_set("person.andri", "not_home", {})
    await hass.config_entries.async_entries("bus")[0].runtime_data.async_refresh()
    assert "origin=" not in str(bus_server.mock_calls[-1][1])


async def test_updates_and_unavailable(hass: HomeAssistant, bus_server, freezer: FrozenDateTimeFactory) -> None:
    await setup_entry(hass)
    bus_server.clear_requests()
    bus_server.get(f"{URL}/api/v1/ha/lines", json={"lines": [line(1, "Work", leave_in=2, minutes=7)], "has_origin": True})
    freezer.tick(timedelta(seconds=31))
    async_fire_time_changed(hass)
    await hass.async_block_till_done()
    assert hass.states.get("sensor.bus_work_leave_in").state == "2"
    # Line 2 was deleted on the server: its entities go unavailable.
    assert hass.states.get("sensor.bus_gym_next").state == STATE_UNAVAILABLE

    bus_server.clear_requests()
    bus_server.get(f"{URL}/api/v1/ha/lines", status=500)
    freezer.tick(timedelta(seconds=31))
    async_fire_time_changed(hass)
    await hass.async_block_till_done()
    assert hass.states.get("sensor.bus_work_next").state == STATE_UNAVAILABLE


async def test_revoked_token_starts_reauth(hass: HomeAssistant, aioclient_mock) -> None:
    aioclient_mock.get(f"{URL}/api/v1/ha/lines", status=401)
    entry = MockConfigEntry(
        domain=DOMAIN, unique_id="bus.example.com:andri", data={CONF_URL: URL, CONF_TOKEN: TOKEN, CONF_VERIFY_SSL: True}
    )
    entry.add_to_hass(hass)
    await hass.config_entries.async_setup(entry.entry_id)
    await hass.async_block_till_done()
    assert entry.state is ConfigEntryState.SETUP_ERROR
    flows = hass.config_entries.flow.async_progress()
    assert len(flows) == 1 and flows[0]["context"]["source"] == SOURCE_REAUTH

    aioclient_mock.clear_requests()
    aioclient_mock.get(f"{URL}/api/v1/me", json={"user": {"id": 1, "username": "andri"}})
    aioclient_mock.get(f"{URL}/api/v1/ha/lines", json={"lines": [line(1, "Work")], "has_origin": False})
    result = await hass.config_entries.flow.async_configure(flows[0]["flow_id"], {CONF_TOKEN: "bus_new"})
    await hass.async_block_till_done()
    assert result["reason"] == "reauth_successful"
    assert entry.data[CONF_TOKEN] == "bus_new"
    assert entry.state is ConfigEntryState.LOADED


async def test_diagnostics_redact_token(hass: HomeAssistant, bus_server) -> None:
    entry = await setup_entry(hass)
    diag = await async_get_config_entry_diagnostics(hass, entry)
    assert diag["entry"]["data"][CONF_TOKEN] == "**REDACTED**"
    assert TOKEN not in str(diag)
    assert "1" in diag["data"]["lines"]
