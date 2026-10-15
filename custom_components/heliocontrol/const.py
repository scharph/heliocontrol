"""Constants for the heliocontrol integration."""

from homeassistant.const import Platform

DOMAIN = "heliocontrol"

CONF_URL = "url"

PLATFORMS: list[Platform] = [
    Platform.SENSOR,
    Platform.BINARY_SENSOR,
    Platform.SELECT,
    Platform.NUMBER,
    Platform.BUTTON,
]

# Fallback poll while the live event stream is down.
FALLBACK_POLL_SECONDS = 60
