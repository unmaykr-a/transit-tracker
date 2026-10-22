"""Fixtures: a fake Bus server answering /api/v1/me and /api/v1/ha/lines."""

from __future__ import annotations

from datetime import timedelta
from typing import Any

import pytest

from homeassistant.util import dt as dt_util

URL = "https://bus.example.com"
TOKEN = "bus_secret"


@pytest.fixture(autouse=True)
def auto_enable_custom_integrations(enable_custom_integrations):
    """Load custom_components/bus in every test."""
    yield


def line(lid: int, label: str, *, leave_in: int | None = 4, catchable: bool | None = True, minutes: int = 9) -> dict[str, Any]:
    now = dt_util.utcnow().replace(microsecond=0)
    deps = [
        {
            "time": (now + timedelta(minutes=minutes + 8 * i)).isoformat(),
            "minutes": minutes + 8 * i,
            "delay_s": 60 * i,
            "realtime": True,
            "headsign": "Kopli",
            "verdict": "green" if i == 0 else "neutral",
            "leave_in_min": None if leave_in is None else leave_in + 8 * i,
        }
        for i in range(5)
    ]
    return {
        "id": lid,
        "label": label,
        "line": "T4",
        "mode": "tram",
        "route_id": "tram:T4",
        "headsign": "Kopli",
        "stop_id": "1234-1",
        "stop_name": "Hobujaama",
        "platform": "A",
        "path": "/lines/tram/T4?stop=1234-1",
        "next": deps[0],
        "next_departures": deps,
        "walk_min": 4,
        "walk_approx": False,
        "verdict": "green",
        "leave_in_min": leave_in,
        "catchable": catchable,
        "unavailable": False,
    }


@pytest.fixture
def lines_payload() -> dict[str, Any]:
    return {"lines": [line(1, "Work"), line(2, "Gym", leave_in=None, catchable=None)], "has_origin": True}


@pytest.fixture
def bus_server(aioclient_mock, lines_payload):
    """Registers the server's answers; tests may override them."""
    aioclient_mock.get(f"{URL}/api/v1/me", json={"user": {"id": 1, "username": "andri", "role": "user"}})
    aioclient_mock.get(f"{URL}/api/v1/ha/lines", json=lines_payload)
    return aioclient_mock
