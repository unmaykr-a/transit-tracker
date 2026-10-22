"""The bundled Lovelace card is served and loaded without adding a resource."""

from __future__ import annotations

from pathlib import Path

import pytest

from homeassistant.components.frontend import DATA_EXTRA_MODULE_URL
from homeassistant.core import HomeAssistant
from homeassistant.setup import async_setup_component

import custom_components.bus as bus


async def test_card_is_registered(hass: HomeAssistant, tmp_path: Path, monkeypatch: pytest.MonkeyPatch, hass_client) -> None:
    card = tmp_path / "bus-card.js"
    card.write_text("customElements.define('bus-card', class extends HTMLElement {});")
    monkeypatch.setattr(bus, "CARD_FILE", card)
    assert await async_setup_component(hass, "bus", {})
    urls = list(hass.data[DATA_EXTRA_MODULE_URL].urls)
    assert len(urls) == 1 and urls[0].startswith("/bus_card/bus-card.js?v=")
    res = await (await hass_client()).get("/bus_card/bus-card.js")
    assert res.status == 200 and "bus-card" in await res.text()


async def test_no_card_file(hass: HomeAssistant, tmp_path: Path, monkeypatch: pytest.MonkeyPatch) -> None:
    monkeypatch.setattr(bus, "CARD_FILE", tmp_path / "missing.js")
    assert await async_setup_component(hass, "bus", {})
    assert not [u for u in hass.data[DATA_EXTRA_MODULE_URL].urls if "bus-card" in u]
