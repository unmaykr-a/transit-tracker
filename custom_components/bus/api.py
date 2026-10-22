"""Minimal client for the Bus server's read-only API."""

from __future__ import annotations

import asyncio
from typing import Any

import aiohttp

from .const import DEPARTURES

TIMEOUT = aiohttp.ClientTimeout(total=15)


class BusError(Exception):
    """The server could not be reached or answered unexpectedly."""


class BusAuthError(BusError):
    """The API token was rejected (revoked, or the user is disabled)."""


class BusClient:
    """Talks to /api/v1 with a personal API token."""

    def __init__(self, session: aiohttp.ClientSession, base_url: str, token: str, verify_ssl: bool = True) -> None:
        self._session = session
        self.base_url = normalize_url(base_url)
        self._token = token
        self._ssl = None if verify_ssl else False

    async def _get(self, path: str, params: dict[str, str] | None = None) -> Any:
        try:
            async with self._session.get(
                self.base_url + path,
                params=params,
                headers={"Authorization": f"Bearer {self._token}", "Accept": "application/json"},
                timeout=TIMEOUT,
                ssl=self._ssl,
            ) as res:
                if res.status in (401, 403):
                    raise BusAuthError(f"token rejected ({res.status})")
                if res.status != 200:
                    raise BusError(f"{path} answered {res.status}")
                return await res.json()
        except (aiohttp.ClientError, asyncio.TimeoutError, ValueError) as err:
            raise BusError(str(err) or type(err).__name__) from err

    async def me(self) -> dict[str, Any]:
        """The token's user; used to validate the config."""
        return await self._get("/api/v1/me")

    async def lines(self, origin: tuple[float, float] | None = None) -> dict[str, Any]:
        """Every saved line with its next departures, in one request."""
        params = {"departures": str(DEPARTURES)}
        if origin is not None:
            params["origin"] = f"{origin[0]:.6f},{origin[1]:.6f}"
        return await self._get("/api/v1/ha/lines", params)


def normalize_url(url: str) -> str:
    """https://bus.example.com/ → https://bus.example.com; adds https:// when missing."""
    url = url.strip().rstrip("/")
    if "://" not in url:
        url = "https://" + url
    return url
