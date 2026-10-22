"""Config flow (server URL + API token) and options flow (lines, interval, origin)."""

from __future__ import annotations

from collections.abc import Mapping
from typing import Any
from urllib.parse import urlparse

import voluptuous as vol

from homeassistant.config_entries import ConfigFlow, ConfigFlowResult, OptionsFlow
from homeassistant.const import CONF_TOKEN, CONF_URL, CONF_VERIFY_SSL
from homeassistant.core import callback
from homeassistant.helpers.aiohttp_client import async_get_clientsession
from homeassistant.helpers.selector import (
    EntitySelector,
    EntitySelectorConfig,
    NumberSelector,
    NumberSelectorConfig,
    NumberSelectorMode,
    SelectOptionDict,
    SelectSelector,
    SelectSelectorConfig,
    TextSelector,
    TextSelectorConfig,
    TextSelectorType,
)

from .api import BusAuthError, BusClient, BusError, normalize_url
from .const import (
    CONF_LINES,
    CONF_ORIGIN,
    CONF_SCAN_INTERVAL,
    DEFAULT_SCAN_INTERVAL,
    DOMAIN,
    MAX_SCAN_INTERVAL,
    MIN_SCAN_INTERVAL,
)
from .coordinator import BusConfigEntry


class BusConfigFlow(ConfigFlow, domain=DOMAIN):
    """Connect to a Bus server with a personal API token."""

    VERSION = 1

    async def _validate(self, data: dict[str, Any]) -> tuple[dict[str, Any] | None, dict[str, str]]:
        """Returns the token's user, or form errors."""
        client = BusClient(
            async_get_clientsession(self.hass, verify_ssl=data.get(CONF_VERIFY_SSL, True)),
            data[CONF_URL],
            data[CONF_TOKEN],
            data.get(CONF_VERIFY_SSL, True),
        )
        try:
            me = await client.me()
        except BusAuthError:
            return None, {CONF_TOKEN: "invalid_auth"}
        except BusError:
            return None, {"base": "cannot_connect"}
        user = me.get("user") if isinstance(me, dict) else None
        if not isinstance(user, dict) or "username" not in user:
            return None, {"base": "not_bus"}
        return user, {}

    async def async_step_user(self, user_input: dict[str, Any] | None = None) -> ConfigFlowResult:
        errors: dict[str, str] = {}
        if user_input is not None:
            user_input[CONF_URL] = normalize_url(user_input[CONF_URL])
            user, errors = await self._validate(user_input)
            if user is not None:
                host = urlparse(user_input[CONF_URL]).netloc
                await self.async_set_unique_id(f"{host}:{user['username']}".lower())
                self._abort_if_unique_id_configured(updates={CONF_URL: user_input[CONF_URL], CONF_TOKEN: user_input[CONF_TOKEN]})
                return self.async_create_entry(title=f"{user['username']} @ {host}", data=user_input)
        schema = vol.Schema(
            {
                vol.Required(CONF_URL, default=(user_input or {}).get(CONF_URL, "")): TextSelector(
                    TextSelectorConfig(type=TextSelectorType.URL)
                ),
                vol.Required(CONF_TOKEN): TextSelector(TextSelectorConfig(type=TextSelectorType.PASSWORD)),
                vol.Required(CONF_VERIFY_SSL, default=(user_input or {}).get(CONF_VERIFY_SSL, True)): bool,
            }
        )
        return self.async_show_form(
            step_id="user", data_schema=schema, errors=errors, description_placeholders={"example_url": "https://bus.example.com"}
        )

    async def async_step_reauth(self, entry_data: Mapping[str, Any]) -> ConfigFlowResult:
        """The token was revoked: ask for a new one."""
        return await self.async_step_reauth_confirm()

    async def async_step_reauth_confirm(self, user_input: dict[str, Any] | None = None) -> ConfigFlowResult:
        entry = self._get_reauth_entry()
        errors: dict[str, str] = {}
        if user_input is not None:
            data = {**entry.data, CONF_TOKEN: user_input[CONF_TOKEN]}
            user, errors = await self._validate(data)
            if user is not None:
                host = urlparse(data[CONF_URL]).netloc
                await self.async_set_unique_id(f"{host}:{user['username']}".lower())
                self._abort_if_unique_id_mismatch(reason="wrong_account")
                return self.async_update_reload_and_abort(entry, data=data)
        return self.async_show_form(
            step_id="reauth_confirm",
            data_schema=vol.Schema({vol.Required(CONF_TOKEN): TextSelector(TextSelectorConfig(type=TextSelectorType.PASSWORD))}),
            description_placeholders={"url": entry.data[CONF_URL]},
            errors=errors,
        )

    @staticmethod
    @callback
    def async_get_options_flow(config_entry: BusConfigEntry) -> OptionsFlow:
        return BusOptionsFlow()


class BusOptionsFlow(OptionsFlow):
    """Pick the saved lines to import, the update interval and the walking origin."""

    async def async_step_init(self, user_input: dict[str, Any] | None = None) -> ConfigFlowResult:
        if user_input is not None:
            if not user_input.get(CONF_ORIGIN):
                user_input.pop(CONF_ORIGIN, None)
            user_input[CONF_SCAN_INTERVAL] = int(user_input[CONF_SCAN_INTERVAL])
            return self.async_create_entry(data=user_input)

        coordinator = self.config_entry.runtime_data
        await coordinator.async_request_refresh()
        lines = coordinator.data.lines if coordinator.data else {}
        choices = [
            SelectOptionDict(
                value=lid,
                label=f"{line.get('label') or line.get('line')} · {line.get('line')} → {line.get('headsign')} ({line.get('stop_name')})",
            )
            for lid, line in lines.items()
        ]
        opts = self.config_entry.options
        selected = [lid for lid in opts.get(CONF_LINES, list(lines)) if lid in lines]
        origin = {"suggested_value": opts.get(CONF_ORIGIN)} if opts.get(CONF_ORIGIN) else {}
        schema = vol.Schema(
            {
                vol.Optional(CONF_LINES, default=selected): SelectSelector(SelectSelectorConfig(options=choices, multiple=True)),
                vol.Required(CONF_SCAN_INTERVAL, default=opts.get(CONF_SCAN_INTERVAL, DEFAULT_SCAN_INTERVAL)): NumberSelector(
                    NumberSelectorConfig(
                        min=MIN_SCAN_INTERVAL, max=MAX_SCAN_INTERVAL, step=5, unit_of_measurement="s", mode=NumberSelectorMode.BOX
                    )
                ),
                vol.Optional(CONF_ORIGIN, description=origin): EntitySelector(
                    EntitySelectorConfig(domain=["zone", "person", "device_tracker"])
                ),
            }
        )
        return self.async_show_form(step_id="init", data_schema=schema)
