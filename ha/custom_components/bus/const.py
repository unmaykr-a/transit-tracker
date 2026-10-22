"""Constants for the Bus integration."""

from datetime import timedelta

DOMAIN = "bus"

CONF_LINES = "lines"
CONF_ORIGIN = "origin_entity"
CONF_SCAN_INTERVAL = "scan_interval"

DEFAULT_SCAN_INTERVAL = 30
MIN_SCAN_INTERVAL = 15
MAX_SCAN_INTERVAL = 600

# The server returns up to five departures per line; the card shows one to five.
DEPARTURES = 5

DEFAULT_INTERVAL = timedelta(seconds=DEFAULT_SCAN_INTERVAL)
