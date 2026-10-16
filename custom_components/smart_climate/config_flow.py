"""Config and options flows: one menu, one form per controller type."""

from __future__ import annotations

from typing import Any

import voluptuous as vol
from homeassistant.config_entries import ConfigEntry, ConfigFlow, ConfigFlowResult, OptionsFlow
from homeassistant.const import CONF_NAME
from homeassistant.core import callback
from homeassistant.helpers import selector

from .config import ShadingConfig, merged_entry_data
from .const import (
    CONF_AREA_M2,
    CONF_AZIMUTH_TOLERANCE,
    CONF_BUFFER_OFFSET,
    CONF_BUFFER_SETPOINT,
    CONF_COMFORT_TEMP,
    CONF_COOL_FLOW_MAX,
    CONF_COOL_FLOW_MIN,
    CONF_COOLER,
    CONF_COOLING_STAGE1,
    CONF_COOLING_STAGE2,
    CONF_COOLING_TEMP,
    CONF_ECO_TEMP,
    CONF_ENTRY_TYPE,
    CONF_FLOW_SETPOINT,
    CONF_HEAT_FLOW_MAX,
    CONF_HEAT_FLOW_MIN,
    CONF_HEATER,
    CONF_HEATING_STAGE1,
    CONF_HEATING_STAGE2,
    CONF_HVAC_DEADBAND,
    CONF_MIN_OFF_MINUTES,
    CONF_MIN_RUN_MINUTES,
    CONF_MIXER_CLOSE,
    CONF_MIXER_DEADBAND,
    CONF_MIXER_FLOW_SENSOR,
    CONF_MIXER_KI,
    CONF_MIXER_KP,
    CONF_MIXER_MAX_TEMP,
    CONF_MIXER_OPEN,
    CONF_MIXER_POSITION,
    CONF_MIXER_PUMP,
    CONF_OUTDOOR_TEMP_SENSOR,
    CONF_PRESENCE_SENSOR,
    CONF_PROTECTION_TEMP,
    CONF_PUMP_OVERRUN,
    CONF_PWM_PERIOD,
    CONF_SCHEDULE,
    CONF_SHADING_COVER,
    CONF_SHADING_COVERS,
    CONF_SHADING_STRATEGY,
    CONF_SHADING_TEMP,
    CONF_SLAT_SPACING,
    CONF_SLAT_WIDTH,
    CONF_STAGE2_THRESHOLD,
    CONF_START_THRESHOLD,
    CONF_STOP_THRESHOLD,
    CONF_TEMPERATURE_SENSOR,
    CONF_WEATHER_COMPENSATION,
    CONF_WIND_ALARM,
    CONF_WINDOW_AZIMUTH,
    CONF_WINDOW_SENSORS,
    DEFAULT_AREA_M2,
    DEFAULT_AZIMUTH_TOLERANCE,
    DEFAULT_BUFFER_OFFSET,
    DEFAULT_COMFORT_TEMP,
    DEFAULT_COOL_FLOW_MAX,
    DEFAULT_COOL_FLOW_MIN,
    DEFAULT_COOLING_TEMP,
    DEFAULT_ECO_TEMP,
    DEFAULT_HEAT_FLOW_MAX,
    DEFAULT_HEAT_FLOW_MIN,
    DEFAULT_HVAC_DEADBAND,
    DEFAULT_MIN_OFF_MINUTES,
    DEFAULT_MIN_RUN_MINUTES,
    DEFAULT_MIXER_DEADBAND,
    DEFAULT_MIXER_KI,
    DEFAULT_MIXER_KP,
    DEFAULT_MIXER_MAX_TEMP,
    DEFAULT_PROTECTION_TEMP,
    DEFAULT_PUMP_OVERRUN,
    DEFAULT_PWM_PERIOD,
    DEFAULT_SHADING_STRATEGY,
    DEFAULT_SHADING_TEMP,
    DEFAULT_SLAT_SPACING,
    DEFAULT_SLAT_WIDTH,
    DEFAULT_STAGE2_THRESHOLD,
    DEFAULT_START_THRESHOLD,
    DEFAULT_STOP_THRESHOLD,
    DEFAULT_WEATHER_COMPENSATION,
    DEFAULT_WINDOW_AZIMUTH,
    DOMAIN,
    ENTRY_TYPE_BUILDING,
    ENTRY_TYPE_MIXER,
    ENTRY_TYPE_ROOM,
    ENTRY_TYPE_TEMPERATURE,
)
from .controller.shading import ShadingStrategy


def _entity(domain: str, *, device_class: str | None = None, multiple: bool = False) -> selector.EntitySelector:
    config = selector.EntitySelectorConfig(domain=domain, multiple=multiple)
    if device_class:
        config["device_class"] = device_class
    return selector.EntitySelector(config)


def _number(minimum: float, maximum: float) -> vol.All:
    return vol.All(vol.Coerce(float), vol.Range(min=minimum, max=maximum))


_TEMPERATURE_SENSOR = _entity("sensor", device_class="temperature")


def room_schema() -> vol.Schema:
    return vol.Schema(
        {
            vol.Required(CONF_NAME): str,
            vol.Required(CONF_TEMPERATURE_SENSOR): _TEMPERATURE_SENSOR,
            vol.Required(CONF_HEATER): _entity("switch"),
            vol.Optional(CONF_COOLER): _entity("switch"),
            vol.Optional(CONF_AREA_M2, default=DEFAULT_AREA_M2): _number(1, 1000),
            vol.Optional(CONF_WINDOW_SENSORS, default=[]): _entity("binary_sensor", multiple=True),
            vol.Optional(CONF_PRESENCE_SENSOR): _entity("binary_sensor"),
            vol.Optional(CONF_SCHEDULE): _entity("schedule"),
            vol.Optional(CONF_COMFORT_TEMP, default=DEFAULT_COMFORT_TEMP): vol.Coerce(float),
            vol.Optional(CONF_COOLING_TEMP, default=DEFAULT_COOLING_TEMP): vol.Coerce(float),
            vol.Optional(CONF_HVAC_DEADBAND, default=DEFAULT_HVAC_DEADBAND): _number(0.1, 5.0),
            vol.Optional(CONF_ECO_TEMP, default=DEFAULT_ECO_TEMP): vol.Coerce(float),
            vol.Optional(CONF_PROTECTION_TEMP, default=DEFAULT_PROTECTION_TEMP): vol.Coerce(float),
            vol.Optional(CONF_PWM_PERIOD, default=DEFAULT_PWM_PERIOD): vol.All(
                vol.Coerce(int), vol.Range(min=5, max=120)
            ),
            vol.Optional(CONF_SHADING_COVERS, default=[]): _entity("cover", multiple=True),
            vol.Optional(CONF_WINDOW_AZIMUTH, default=DEFAULT_WINDOW_AZIMUTH): _number(0, 360),
            vol.Optional(CONF_SHADING_TEMP, default=DEFAULT_SHADING_TEMP): vol.Coerce(float),
            vol.Optional(CONF_AZIMUTH_TOLERANCE, default=DEFAULT_AZIMUTH_TOLERANCE): _number(1, 180),
            vol.Optional(CONF_WIND_ALARM): _entity("binary_sensor"),
            vol.Optional(CONF_SHADING_STRATEGY, default=DEFAULT_SHADING_STRATEGY): vol.In(
                [strategy.value for strategy in ShadingStrategy]
            ),
            vol.Optional(CONF_SLAT_WIDTH, default=DEFAULT_SLAT_WIDTH): _number(20, 150),
            vol.Optional(CONF_SLAT_SPACING, default=DEFAULT_SLAT_SPACING): _number(10, 150),
        }
    )


def building_schema() -> vol.Schema:
    return vol.Schema(
        {
            vol.Required(CONF_NAME, default="Building Climate Controller"): str,
            vol.Required(CONF_HEATING_STAGE1): _entity("switch"),
            vol.Optional(CONF_HEATING_STAGE2): _entity("switch"),
            vol.Optional(CONF_COOLING_STAGE1): _entity("switch"),
            vol.Optional(CONF_COOLING_STAGE2): _entity("switch"),
            vol.Optional(CONF_START_THRESHOLD, default=DEFAULT_START_THRESHOLD): _number(1, 100),
            vol.Optional(CONF_STOP_THRESHOLD, default=DEFAULT_STOP_THRESHOLD): _number(0, 99),
            vol.Optional(CONF_STAGE2_THRESHOLD, default=DEFAULT_STAGE2_THRESHOLD): _number(1, 100),
            vol.Optional(CONF_MIN_RUN_MINUTES, default=DEFAULT_MIN_RUN_MINUTES): _number(0, 240),
            vol.Optional(CONF_MIN_OFF_MINUTES, default=DEFAULT_MIN_OFF_MINUTES): _number(0, 240),
        }
    )


def temperature_schema() -> vol.Schema:
    return vol.Schema(
        {
            vol.Required(CONF_NAME, default="Intelligent Temperature Controller"): str,
            vol.Optional(CONF_OUTDOOR_TEMP_SENSOR): _TEMPERATURE_SENSOR,
            vol.Optional(CONF_FLOW_SETPOINT): _entity("number"),
            vol.Optional(CONF_BUFFER_SETPOINT): _entity("number"),
            vol.Optional(CONF_HEAT_FLOW_MIN, default=DEFAULT_HEAT_FLOW_MIN): vol.Coerce(float),
            vol.Optional(CONF_HEAT_FLOW_MAX, default=DEFAULT_HEAT_FLOW_MAX): vol.Coerce(float),
            vol.Optional(CONF_COOL_FLOW_MIN, default=DEFAULT_COOL_FLOW_MIN): vol.Coerce(float),
            vol.Optional(CONF_COOL_FLOW_MAX, default=DEFAULT_COOL_FLOW_MAX): vol.Coerce(float),
            vol.Optional(CONF_BUFFER_OFFSET, default=DEFAULT_BUFFER_OFFSET): vol.Coerce(float),
            vol.Optional(CONF_WEATHER_COMPENSATION, default=DEFAULT_WEATHER_COMPENSATION): vol.Coerce(float),
        }
    )


def mixer_schema() -> vol.Schema:
    return vol.Schema(
        {
            vol.Required(CONF_NAME, default="Heating Mixer Controller"): str,
            vol.Required(CONF_MIXER_FLOW_SENSOR): _TEMPERATURE_SENSOR,
            vol.Optional(CONF_MIXER_POSITION): _entity("number"),
            vol.Optional(CONF_MIXER_OPEN): _entity("switch"),
            vol.Optional(CONF_MIXER_CLOSE): _entity("switch"),
            vol.Required(CONF_MIXER_PUMP): _entity("switch"),
            vol.Optional(CONF_MIXER_KP, default=DEFAULT_MIXER_KP): _number(0, 100),
            vol.Optional(CONF_MIXER_KI, default=DEFAULT_MIXER_KI): _number(0, 10),
            vol.Optional(CONF_MIXER_DEADBAND, default=DEFAULT_MIXER_DEADBAND): _number(0.05, 5),
            vol.Optional(CONF_MIXER_MAX_TEMP, default=DEFAULT_MIXER_MAX_TEMP): vol.Coerce(float),
            vol.Optional(CONF_PUMP_OVERRUN, default=DEFAULT_PUMP_OVERRUN): _number(0, 60),
        }
    )


SCHEMAS = {
    ENTRY_TYPE_ROOM: room_schema,
    ENTRY_TYPE_BUILDING: building_schema,
    ENTRY_TYPE_TEMPERATURE: temperature_schema,
    ENTRY_TYPE_MIXER: mixer_schema,
}


def options_schema(entry_type: str) -> vol.Schema:
    """The setup form without the name: the title and unique ID stay fixed."""
    schema = SCHEMAS[entry_type]()
    return vol.Schema({key: value for key, value in schema.schema.items() if key.schema != CONF_NAME})


def room_unique_id(name: str) -> str:
    return "room_" + name.strip().lower().replace(" ", "_")


class SmartClimateConfigFlow(ConfigFlow, domain=DOMAIN):
    VERSION = 2

    @staticmethod
    @callback
    def async_get_options_flow(config_entry: ConfigEntry) -> SmartClimateOptionsFlow:
        return SmartClimateOptionsFlow()

    async def async_step_user(self, user_input: dict[str, Any] | None = None) -> ConfigFlowResult:
        return self.async_show_menu(step_id="user", menu_options=["room", "building", "temperature", "mixer"])

    async def async_step_room(self, user_input: dict[str, Any] | None = None) -> ConfigFlowResult:
        if user_input is None:
            return self.async_show_form(step_id="room", data_schema=room_schema())
        return await self._async_create(ENTRY_TYPE_ROOM, room_unique_id(user_input[CONF_NAME]), user_input)

    async def async_step_building(self, user_input: dict[str, Any] | None = None) -> ConfigFlowResult:
        if user_input is None:
            return self.async_show_form(step_id="building", data_schema=building_schema())
        return await self._async_create(ENTRY_TYPE_BUILDING, "building_controller", user_input)

    async def async_step_temperature(self, user_input: dict[str, Any] | None = None) -> ConfigFlowResult:
        if user_input is None:
            return self.async_show_form(step_id="temperature", data_schema=temperature_schema())
        return await self._async_create(ENTRY_TYPE_TEMPERATURE, "temperature_controller", user_input)

    async def async_step_mixer(self, user_input: dict[str, Any] | None = None) -> ConfigFlowResult:
        if user_input is None:
            return self.async_show_form(step_id="mixer", data_schema=mixer_schema())
        return await self._async_create(ENTRY_TYPE_MIXER, "heating_mixer_controller", user_input)

    async def _async_create(self, entry_type: str, unique_id: str, user_input: dict[str, Any]) -> ConfigFlowResult:
        await self.async_set_unique_id(unique_id)
        self._abort_if_unique_id_configured()
        return self.async_create_entry(title=user_input[CONF_NAME], data={**user_input, CONF_ENTRY_TYPE: entry_type})


class SmartClimateOptionsFlow(OptionsFlow):
    """Edit every setting of an existing controller. Saving reloads the entry."""

    async def async_step_init(self, user_input: dict[str, Any] | None = None) -> ConfigFlowResult:
        entry = self.config_entry
        entry_type = entry.data.get(CONF_ENTRY_TYPE, ENTRY_TYPE_ROOM)
        schema = options_schema(entry_type)

        if user_input is not None:
            # A cleared optional entity is simply missing from user_input. Store it
            # as None explicitly, otherwise the value from entry.data would win again.
            options = {key.schema: None for key in schema.schema if key.schema not in user_input}
            options.update(user_input)
            if entry_type == ENTRY_TYPE_ROOM:
                # The V0.4 single-cover key is folded into shading_covers below.
                options[CONF_SHADING_COVER] = None
            return self.async_create_entry(data=options)

        current = merged_entry_data(entry.data, entry.options)
        if entry_type == ENTRY_TYPE_ROOM:
            current[CONF_SHADING_COVERS] = list(ShadingConfig.from_mapping(current).covers)
        suggested = {key: value for key, value in current.items() if value is not None}
        return self.async_show_form(
            step_id="init",
            data_schema=self.add_suggested_values_to_schema(schema, suggested),
            description_placeholders={"name": entry.title},
        )
