"""Typed views on config entry data.

Every runtime used to read raw ``dict`` values with inline ``float(data.get(...))``
calls. The dataclasses here parse a config entry once (``data`` merged with
``options``) so defaults and type coercion live in exactly one place.
"""

from __future__ import annotations

from collections.abc import Mapping
from dataclasses import dataclass
from typing import Any

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
    CONF_FLOW_SETPOINT,
    CONF_HEAT_FLOW_MAX,
    CONF_HEAT_FLOW_MIN,
    CONF_HEATER,
    CONF_HEATING_STAGE1,
    CONF_HEATING_STAGE2,
    CONF_HVAC_DEADBAND,
    CONF_MIN_OFF_MINUTES,
    CONF_MIN_RUN_MINUTES,
    CONF_MIN_SWITCH_SECONDS,
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
    DEFAULT_MIN_SWITCH_SECONDS,
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
)
from .controller.climate_controller import ClimateController
from .controller.shading import ShadingStrategy
from .controller.temperature_controller import IntelligentTemperatureController


def merged_entry_data(data: Mapping[str, Any], options: Mapping[str, Any]) -> dict[str, Any]:
    """Options override the values entered during the initial config flow."""
    return {**data, **options}


def _float(data: Mapping[str, Any], key: str, default: float) -> float:
    value = data.get(key)
    return float(default if value is None else value)


def _optional_entity(data: Mapping[str, Any], key: str) -> str | None:
    return data.get(key) or None


def _entity_list(data: Mapping[str, Any], key: str) -> tuple[str, ...]:
    return tuple(data.get(key) or ())


@dataclass(frozen=True, slots=True)
class ShadingConfig:
    covers: tuple[str, ...]
    window_azimuth: float
    shading_temperature: float
    azimuth_tolerance: float
    wind_alarm: str | None
    strategy: ShadingStrategy
    slat_width_mm: float
    slat_spacing_mm: float

    @property
    def enabled(self) -> bool:
        return bool(self.covers)

    @classmethod
    def from_mapping(cls, data: Mapping[str, Any]) -> ShadingConfig:
        covers = list(_entity_list(data, CONF_SHADING_COVERS))
        legacy = data.get(CONF_SHADING_COVER)
        if legacy and legacy not in covers:
            covers.append(legacy)
        return cls(
            covers=tuple(covers),
            window_azimuth=_float(data, CONF_WINDOW_AZIMUTH, DEFAULT_WINDOW_AZIMUTH),
            shading_temperature=_float(data, CONF_SHADING_TEMP, DEFAULT_SHADING_TEMP),
            azimuth_tolerance=_float(data, CONF_AZIMUTH_TOLERANCE, DEFAULT_AZIMUTH_TOLERANCE),
            wind_alarm=_optional_entity(data, CONF_WIND_ALARM),
            strategy=ShadingStrategy(data.get(CONF_SHADING_STRATEGY, DEFAULT_SHADING_STRATEGY)),
            slat_width_mm=_float(data, CONF_SLAT_WIDTH, DEFAULT_SLAT_WIDTH),
            slat_spacing_mm=_float(data, CONF_SLAT_SPACING, DEFAULT_SLAT_SPACING),
        )


@dataclass(frozen=True, slots=True)
class RoomConfig:
    temperature_sensor: str
    heater: str
    cooler: str | None
    area_m2: float
    window_sensors: tuple[str, ...]
    presence_sensor: str | None
    schedule: str | None
    comfort_temperature: float
    cooling_temperature: float
    hvac_deadband: float
    eco_temperature: float
    protection_temperature: float
    pwm_period_seconds: float
    min_switch_seconds: float
    shading: ShadingConfig

    @classmethod
    def from_mapping(cls, data: Mapping[str, Any]) -> RoomConfig:
        return cls(
            temperature_sensor=data[CONF_TEMPERATURE_SENSOR],
            heater=data[CONF_HEATER],
            cooler=_optional_entity(data, CONF_COOLER),
            area_m2=_float(data, CONF_AREA_M2, DEFAULT_AREA_M2),
            window_sensors=_entity_list(data, CONF_WINDOW_SENSORS),
            presence_sensor=_optional_entity(data, CONF_PRESENCE_SENSOR),
            schedule=_optional_entity(data, CONF_SCHEDULE),
            comfort_temperature=_float(data, CONF_COMFORT_TEMP, DEFAULT_COMFORT_TEMP),
            cooling_temperature=_float(data, CONF_COOLING_TEMP, DEFAULT_COOLING_TEMP),
            hvac_deadband=_float(data, CONF_HVAC_DEADBAND, DEFAULT_HVAC_DEADBAND),
            eco_temperature=_float(data, CONF_ECO_TEMP, DEFAULT_ECO_TEMP),
            protection_temperature=_float(data, CONF_PROTECTION_TEMP, DEFAULT_PROTECTION_TEMP),
            pwm_period_seconds=_float(data, CONF_PWM_PERIOD, DEFAULT_PWM_PERIOD) * 60.0,
            min_switch_seconds=_float(data, CONF_MIN_SWITCH_SECONDS, DEFAULT_MIN_SWITCH_SECONDS),
            shading=ShadingConfig.from_mapping(data),
        )

    @property
    def watched_entities(self) -> list[str]:
        """Entities whose state changes should trigger a recalculation."""
        ids = [self.temperature_sensor, *self.window_sensors]
        ids += [e for e in (self.presence_sensor, self.schedule, self.shading.wind_alarm) if e]
        if self.shading.enabled:
            ids += [*self.shading.covers, "sun.sun"]
        return ids


@dataclass(frozen=True, slots=True)
class BuildingConfig:
    heating_stage1: str | None
    heating_stage2: str | None
    cooling_stage1: str | None
    cooling_stage2: str | None
    start_threshold: float
    stop_threshold: float
    stage2_threshold: float
    min_run_seconds: float
    min_off_seconds: float

    @classmethod
    def from_mapping(cls, data: Mapping[str, Any]) -> BuildingConfig:
        return cls(
            heating_stage1=_optional_entity(data, CONF_HEATING_STAGE1),
            heating_stage2=_optional_entity(data, CONF_HEATING_STAGE2),
            cooling_stage1=_optional_entity(data, CONF_COOLING_STAGE1),
            cooling_stage2=_optional_entity(data, CONF_COOLING_STAGE2),
            # Thresholds are entered in percent, the controller works with 0..1.
            start_threshold=_float(data, CONF_START_THRESHOLD, DEFAULT_START_THRESHOLD) / 100,
            stop_threshold=_float(data, CONF_STOP_THRESHOLD, DEFAULT_STOP_THRESHOLD) / 100,
            stage2_threshold=_float(data, CONF_STAGE2_THRESHOLD, DEFAULT_STAGE2_THRESHOLD) / 100,
            min_run_seconds=_float(data, CONF_MIN_RUN_MINUTES, DEFAULT_MIN_RUN_MINUTES) * 60,
            min_off_seconds=_float(data, CONF_MIN_OFF_MINUTES, DEFAULT_MIN_OFF_MINUTES) * 60,
        )

    def build_controller(self) -> ClimateController:
        return ClimateController(
            start_threshold=self.start_threshold,
            stop_threshold=self.stop_threshold,
            stage2_threshold=self.stage2_threshold,
            min_run_seconds=self.min_run_seconds,
            min_off_seconds=self.min_off_seconds,
        )


@dataclass(frozen=True, slots=True)
class TemperatureConfig:
    outdoor_sensor: str | None
    flow_setpoint: str | None
    buffer_setpoint: str | None
    heat_flow_min: float
    heat_flow_max: float
    cool_flow_min: float
    cool_flow_max: float
    buffer_offset: float
    weather_compensation: float

    @classmethod
    def from_mapping(cls, data: Mapping[str, Any]) -> TemperatureConfig:
        return cls(
            outdoor_sensor=_optional_entity(data, CONF_OUTDOOR_TEMP_SENSOR),
            flow_setpoint=_optional_entity(data, CONF_FLOW_SETPOINT),
            buffer_setpoint=_optional_entity(data, CONF_BUFFER_SETPOINT),
            heat_flow_min=_float(data, CONF_HEAT_FLOW_MIN, DEFAULT_HEAT_FLOW_MIN),
            heat_flow_max=_float(data, CONF_HEAT_FLOW_MAX, DEFAULT_HEAT_FLOW_MAX),
            cool_flow_min=_float(data, CONF_COOL_FLOW_MIN, DEFAULT_COOL_FLOW_MIN),
            cool_flow_max=_float(data, CONF_COOL_FLOW_MAX, DEFAULT_COOL_FLOW_MAX),
            buffer_offset=_float(data, CONF_BUFFER_OFFSET, DEFAULT_BUFFER_OFFSET),
            weather_compensation=_float(data, CONF_WEATHER_COMPENSATION, DEFAULT_WEATHER_COMPENSATION),
        )

    def build_controller(self) -> IntelligentTemperatureController:
        return IntelligentTemperatureController(
            heat_min=self.heat_flow_min,
            heat_max=self.heat_flow_max,
            cool_min=self.cool_flow_min,
            cool_max=self.cool_flow_max,
            buffer_offset=self.buffer_offset,
            weather_compensation=self.weather_compensation,
        )


@dataclass(frozen=True, slots=True)
class MixerConfig:
    flow_sensor: str
    position_number: str | None
    open_switch: str | None
    close_switch: str | None
    pump: str | None
    kp: float
    ki: float
    deadband: float
    max_temperature: float
    pump_overrun_minutes: float  # configurable, not yet applied by the runtime

    @property
    def analog_valve(self) -> bool:
        return self.position_number is not None

    @classmethod
    def from_mapping(cls, data: Mapping[str, Any]) -> MixerConfig:
        return cls(
            flow_sensor=data[CONF_MIXER_FLOW_SENSOR],
            position_number=_optional_entity(data, CONF_MIXER_POSITION),
            open_switch=_optional_entity(data, CONF_MIXER_OPEN),
            close_switch=_optional_entity(data, CONF_MIXER_CLOSE),
            pump=_optional_entity(data, CONF_MIXER_PUMP),
            kp=_float(data, CONF_MIXER_KP, DEFAULT_MIXER_KP),
            ki=_float(data, CONF_MIXER_KI, DEFAULT_MIXER_KI),
            deadband=_float(data, CONF_MIXER_DEADBAND, DEFAULT_MIXER_DEADBAND),
            max_temperature=_float(data, CONF_MIXER_MAX_TEMP, DEFAULT_MIXER_MAX_TEMP),
            pump_overrun_minutes=_float(data, CONF_PUMP_OVERRUN, DEFAULT_PUMP_OVERRUN),
        )
