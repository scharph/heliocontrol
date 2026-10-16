"""Constants for the Smart Climate integration."""

from __future__ import annotations

from datetime import timedelta
from typing import Final

DOMAIN: Final = "smart_climate"
MANUFACTURER: Final = "Smart Climate"

UPDATE_INTERVAL: Final = timedelta(seconds=30)
STORAGE_VERSION: Final = 1
STORAGE_SAVE_DELAY_SECONDS: Final = 10

# --- Config entry types -------------------------------------------------------
CONF_ENTRY_TYPE: Final = "entry_type"
ENTRY_TYPE_ROOM: Final = "room"
ENTRY_TYPE_BUILDING: Final = "building"
ENTRY_TYPE_TEMPERATURE: Final = "temperature_controller"
ENTRY_TYPE_MIXER: Final = "mixer_controller"

# --- Room controller ----------------------------------------------------------
CONF_TEMPERATURE_SENSOR: Final = "temperature_sensor"
CONF_HEATER: Final = "heater"
CONF_COOLER: Final = "cooler"
CONF_AREA_M2: Final = "area_m2"
CONF_WINDOW_SENSORS: Final = "window_sensors"
CONF_PRESENCE_SENSOR: Final = "presence_sensor"
CONF_SCHEDULE: Final = "comfort_schedule"
CONF_COMFORT_TEMP: Final = "comfort_temperature"
CONF_COOLING_TEMP: Final = "cooling_temperature"
CONF_HVAC_DEADBAND: Final = "hvac_deadband"
CONF_ECO_TEMP: Final = "eco_temperature"
CONF_PROTECTION_TEMP: Final = "protection_temperature"
CONF_PWM_PERIOD: Final = "pwm_period_minutes"
CONF_MIN_SWITCH_SECONDS: Final = "min_switch_seconds"

DEFAULT_AREA_M2: Final = 20.0
DEFAULT_COMFORT_TEMP: Final = 22.0
DEFAULT_COOLING_TEMP: Final = 24.0
DEFAULT_HVAC_DEADBAND: Final = 0.3
DEFAULT_ECO_TEMP: Final = 19.0
DEFAULT_PROTECTION_TEMP: Final = 5.0
DEFAULT_PWM_PERIOD: Final = 30
DEFAULT_MIN_SWITCH_SECONDS: Final = 120

# --- Room shading -------------------------------------------------------------
CONF_SHADING_COVER: Final = "shading_cover"  # V0.4 single-cover key, still read
CONF_SHADING_COVERS: Final = "shading_covers"
CONF_WINDOW_AZIMUTH: Final = "window_azimuth"
CONF_SHADING_TEMP: Final = "shading_temperature"
CONF_AZIMUTH_TOLERANCE: Final = "azimuth_tolerance"
CONF_WIND_ALARM: Final = "wind_alarm"
CONF_SHADING_STRATEGY: Final = "shading_strategy"
CONF_SLAT_WIDTH: Final = "slat_width_mm"
CONF_SLAT_SPACING: Final = "slat_spacing_mm"

DEFAULT_WINDOW_AZIMUTH: Final = 180.0
DEFAULT_SHADING_TEMP: Final = 23.5
DEFAULT_AZIMUTH_TOLERANCE: Final = 85.0
DEFAULT_SHADING_STRATEGY: Final = "maximum_daylight"
DEFAULT_SLAT_WIDTH: Final = 80.0
DEFAULT_SLAT_SPACING: Final = 70.0

# --- Building climate controller ---------------------------------------------
CONF_HEATING_STAGE1: Final = "heating_stage1"
CONF_HEATING_STAGE2: Final = "heating_stage2"
CONF_COOLING_STAGE1: Final = "cooling_stage1"
CONF_COOLING_STAGE2: Final = "cooling_stage2"
CONF_START_THRESHOLD: Final = "start_threshold"
CONF_STOP_THRESHOLD: Final = "stop_threshold"
CONF_STAGE2_THRESHOLD: Final = "stage2_threshold"
CONF_MIN_RUN_MINUTES: Final = "min_run_minutes"
CONF_MIN_OFF_MINUTES: Final = "min_off_minutes"

DEFAULT_START_THRESHOLD: Final = 30
DEFAULT_STOP_THRESHOLD: Final = 15
DEFAULT_STAGE2_THRESHOLD: Final = 75
DEFAULT_MIN_RUN_MINUTES: Final = 10
DEFAULT_MIN_OFF_MINUTES: Final = 5

# --- Intelligent temperature controller --------------------------------------
CONF_OUTDOOR_TEMP_SENSOR: Final = "outdoor_temperature_sensor"
CONF_FLOW_SETPOINT: Final = "flow_setpoint"
CONF_BUFFER_SETPOINT: Final = "buffer_setpoint"
CONF_HEAT_FLOW_MIN: Final = "heat_flow_min"
CONF_HEAT_FLOW_MAX: Final = "heat_flow_max"
CONF_COOL_FLOW_MIN: Final = "cool_flow_min"
CONF_COOL_FLOW_MAX: Final = "cool_flow_max"
CONF_BUFFER_OFFSET: Final = "buffer_offset"
CONF_WEATHER_COMPENSATION: Final = "weather_compensation"

DEFAULT_HEAT_FLOW_MIN: Final = 25.0
DEFAULT_HEAT_FLOW_MAX: Final = 40.0
DEFAULT_COOL_FLOW_MIN: Final = 16.0
DEFAULT_COOL_FLOW_MAX: Final = 20.0
DEFAULT_BUFFER_OFFSET: Final = 3.0
DEFAULT_WEATHER_COMPENSATION: Final = 0.25

# --- Heating mixer controller -------------------------------------------------
CONF_MIXER_FLOW_SENSOR: Final = "mixer_flow_temperature_sensor"
CONF_MIXER_POSITION: Final = "mixer_position"
CONF_MIXER_OPEN: Final = "mixer_open"
CONF_MIXER_CLOSE: Final = "mixer_close"
CONF_MIXER_PUMP: Final = "mixer_pump"
CONF_MIXER_KP: Final = "mixer_kp"
CONF_MIXER_KI: Final = "mixer_ki"
CONF_MIXER_DEADBAND: Final = "mixer_deadband"
CONF_MIXER_MAX_TEMP: Final = "mixer_max_temperature"
CONF_PUMP_OVERRUN: Final = "pump_overrun_minutes"

DEFAULT_MIXER_KP: Final = 8.0
DEFAULT_MIXER_KI: Final = 0.08
DEFAULT_MIXER_DEADBAND: Final = 0.25
DEFAULT_MIXER_MAX_TEMP: Final = 50.0
DEFAULT_PUMP_OVERRUN: Final = 3.0

# --- Services -----------------------------------------------------------------
SERVICE_CLEAR_MANUAL_OVERRIDE: Final = "clear_manual_override"
SERVICE_RECALCULATE: Final = "recalculate"
ATTR_ENTRY_ID: Final = "entry_id"
