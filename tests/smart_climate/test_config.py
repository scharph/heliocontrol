from custom_components.smart_climate.config import BuildingConfig, RoomConfig, merged_entry_data


def test_room_defaults_and_legacy_cover():
    cfg = RoomConfig.from_mapping(
        {
            "temperature_sensor": "sensor.t",
            "heater": "switch.h",
            "shading_cover": "cover.old",
            "shading_covers": ["cover.new"],
        }
    )
    assert cfg.comfort_temperature == 22.0
    assert cfg.pwm_period_seconds == 30 * 60
    assert cfg.cooler is None
    assert cfg.shading.covers == ("cover.new", "cover.old")
    assert "sun.sun" in cfg.watched_entities


def test_options_override_data():
    merged = merged_entry_data(
        {"temperature_sensor": "sensor.t", "heater": "switch.h", "eco_temperature": 18}, {"eco_temperature": 17.5}
    )
    assert RoomConfig.from_mapping(merged).eco_temperature == 17.5


def test_building_percent_and_minutes_are_converted():
    cfg = BuildingConfig.from_mapping({"heating_stage1": "switch.h1", "start_threshold": 40, "min_run_minutes": 2})
    assert cfg.start_threshold == 0.4
    assert cfg.min_run_seconds == 120
