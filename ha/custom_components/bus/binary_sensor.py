"""Whether the next departure can still be made from the origin."""

from __future__ import annotations

from homeassistant.components.binary_sensor import BinarySensorEntity
from homeassistant.core import HomeAssistant
from homeassistant.helpers.entity_platform import AddConfigEntryEntitiesCallback

from .coordinator import BusConfigEntry
from .entity import BusLineEntity, selected_lines

PARALLEL_UPDATES = 0


async def async_setup_entry(hass: HomeAssistant, entry: BusConfigEntry, async_add_entities: AddConfigEntryEntitiesCallback) -> None:
    coordinator = entry.runtime_data
    async_add_entities(BusCatchableSensor(coordinator, lid, "catchable") for lid in selected_lines(entry))


class BusCatchableSensor(BusLineEntity, BinarySensorEntity):
    """On while a listed departure can still be caught; unknown without an origin."""

    @property
    def is_on(self) -> bool | None:
        value = self.line.get("catchable")
        return value if isinstance(value, bool) else None
