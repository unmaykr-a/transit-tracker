"""Config and options flow."""

from __future__ import annotations

from homeassistant import config_entries
from homeassistant.const import CONF_TOKEN, CONF_URL, CONF_VERIFY_SSL
from homeassistant.core import HomeAssistant
from homeassistant.data_entry_flow import FlowResultType
from pytest_homeassistant_custom_component.common import MockConfigEntry

from custom_components.bus.const import CONF_LINES, CONF_ORIGIN, CONF_SCAN_INTERVAL, DOMAIN

from .conftest import TOKEN, URL


async def test_user_flow(hass: HomeAssistant, bus_server) -> None:
    result = await hass.config_entries.flow.async_init(DOMAIN, context={"source": config_entries.SOURCE_USER})
    assert result["type"] is FlowResultType.FORM
    result = await hass.config_entries.flow.async_configure(
        result["flow_id"], {CONF_URL: "bus.example.com/", CONF_TOKEN: TOKEN, CONF_VERIFY_SSL: True}
    )
    assert result["type"] is FlowResultType.CREATE_ENTRY
    assert result["options"] == {CONF_ORIGIN: "zone.home"}  # walk from home by default
    assert result["title"] == "andri @ bus.example.com"
    assert result["data"][CONF_URL] == URL
    # The token is sent as a bearer header.
    assert bus_server.mock_calls[0][3]["Authorization"] == f"Bearer {TOKEN}"


async def test_user_flow_errors(hass: HomeAssistant, aioclient_mock) -> None:
    aioclient_mock.get(f"{URL}/api/v1/me", status=401)
    result = await hass.config_entries.flow.async_init(DOMAIN, context={"source": config_entries.SOURCE_USER})
    result = await hass.config_entries.flow.async_configure(
        result["flow_id"], {CONF_URL: URL, CONF_TOKEN: "bus_bad", CONF_VERIFY_SSL: True}
    )
    assert result["errors"] == {CONF_TOKEN: "invalid_auth"}

    aioclient_mock.clear_requests()
    aioclient_mock.get(f"{URL}/api/v1/me", status=502)
    result = await hass.config_entries.flow.async_configure(result["flow_id"], {CONF_URL: URL, CONF_TOKEN: TOKEN, CONF_VERIFY_SSL: True})
    assert result["errors"] == {"base": "cannot_connect"}

    aioclient_mock.clear_requests()
    aioclient_mock.get(f"{URL}/api/v1/me", json={"hello": "world"})
    result = await hass.config_entries.flow.async_configure(result["flow_id"], {CONF_URL: URL, CONF_TOKEN: TOKEN, CONF_VERIFY_SSL: True})
    assert result["errors"] == {"base": "not_bus"}


async def test_duplicate_account_aborts(hass: HomeAssistant, bus_server) -> None:
    MockConfigEntry(domain=DOMAIN, unique_id="bus.example.com:andri", data={CONF_URL: URL, CONF_TOKEN: "old"}).add_to_hass(hass)
    result = await hass.config_entries.flow.async_init(DOMAIN, context={"source": config_entries.SOURCE_USER})
    result = await hass.config_entries.flow.async_configure(result["flow_id"], {CONF_URL: URL, CONF_TOKEN: TOKEN, CONF_VERIFY_SSL: True})
    assert result["type"] is FlowResultType.ABORT
    assert result["reason"] == "already_configured"


async def test_options_flow(hass: HomeAssistant, bus_server) -> None:
    entry = MockConfigEntry(
        domain=DOMAIN, unique_id="bus.example.com:andri", data={CONF_URL: URL, CONF_TOKEN: TOKEN, CONF_VERIFY_SSL: True}
    )
    entry.add_to_hass(hass)
    assert await hass.config_entries.async_setup(entry.entry_id)
    await hass.async_block_till_done()

    result = await hass.config_entries.options.async_init(entry.entry_id)
    assert result["type"] is FlowResultType.FORM
    options = result["data_schema"].schema[CONF_LINES].config["options"]
    assert [o["value"] for o in options] == ["1", "2"]
    result = await hass.config_entries.options.async_configure(
        result["flow_id"], {CONF_LINES: ["1"], CONF_SCAN_INTERVAL: 45.0, CONF_ORIGIN: "zone.home"}
    )
    assert result["type"] is FlowResultType.CREATE_ENTRY
    assert entry.options == {CONF_LINES: ["1"], CONF_SCAN_INTERVAL: 45, CONF_ORIGIN: "zone.home"}
    await hass.async_block_till_done()
    # Reloaded with one line only.
    assert hass.states.get("sensor.bus_work_next") is not None
    assert hass.states.get("sensor.bus_gym_next") is None


async def test_options_all_lines_follow_new_lines(hass: HomeAssistant, bus_server) -> None:
    """Every line selected stores no list, so lines saved later in Bus appear too."""
    entry = MockConfigEntry(
        domain=DOMAIN, unique_id="bus.example.com:andri", data={CONF_URL: URL, CONF_TOKEN: TOKEN, CONF_VERIFY_SSL: True}
    )
    entry.add_to_hass(hass)
    assert await hass.config_entries.async_setup(entry.entry_id)
    await hass.async_block_till_done()
    result = await hass.config_entries.options.async_init(entry.entry_id)
    result = await hass.config_entries.options.async_configure(result["flow_id"], {CONF_LINES: ["1", "2"], CONF_SCAN_INTERVAL: 30})
    assert result["type"] is FlowResultType.CREATE_ENTRY
    assert CONF_LINES not in entry.options


async def test_options_while_not_loaded(hass: HomeAssistant, aioclient_mock) -> None:
    """The server is down at start: Configure explains instead of crashing."""
    aioclient_mock.get(f"{URL}/api/v1/ha/lines", exc=TimeoutError)
    entry = MockConfigEntry(
        domain=DOMAIN, unique_id="bus.example.com:andri", data={CONF_URL: URL, CONF_TOKEN: TOKEN, CONF_VERIFY_SSL: True}
    )
    entry.add_to_hass(hass)
    await hass.config_entries.async_setup(entry.entry_id)
    await hass.async_block_till_done()
    assert entry.state is config_entries.ConfigEntryState.SETUP_RETRY
    result = await hass.config_entries.options.async_init(entry.entry_id)
    assert result["type"] is FlowResultType.ABORT and result["reason"] == "not_loaded"


async def test_reconfigure(hass: HomeAssistant, bus_server) -> None:
    entry = MockConfigEntry(
        domain=DOMAIN, unique_id="bus.example.com:andri", data={CONF_URL: URL, CONF_TOKEN: TOKEN, CONF_VERIFY_SSL: True}
    )
    entry.add_to_hass(hass)
    assert await hass.config_entries.async_setup(entry.entry_id)
    await hass.async_block_till_done()
    result = await entry.start_reconfigure_flow(hass)
    assert result["type"] is FlowResultType.FORM and result["step_id"] == "reconfigure"
    result = await hass.config_entries.flow.async_configure(
        result["flow_id"], {CONF_URL: URL + "/", CONF_TOKEN: "bus_new", CONF_VERIFY_SSL: False}
    )
    assert result["type"] is FlowResultType.ABORT and result["reason"] == "reconfigure_successful"
    assert entry.data == {CONF_URL: URL, CONF_TOKEN: "bus_new", CONF_VERIFY_SSL: False}
