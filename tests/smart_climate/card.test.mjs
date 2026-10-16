// node --test tests/smart_climate/card.test.mjs
import assert from "node:assert/strict";
import test from "node:test";

globalThis.HTMLElement = class {};
globalThis.customElements = { get: () => true };
const { entitiesOf, kindOf } = await import("../../custom_components/smart_climate/frontend/smart-climate-card.js");

const entities = {
  a: { entity_id: "climate.office_room_controller", platform: "smart_climate", device_id: "d1", translation_key: "room_controller" },
  b: { entity_id: "sensor.office_room_mode", platform: "smart_climate", device_id: "d1", translation_key: "room_mode" },
  c: { entity_id: "sensor.house_plant_mode", platform: "smart_climate", device_id: "d2", translation_key: "plant_mode" },
  d: { entity_id: "sensor.other", platform: "other", device_id: "d1", translation_key: "plant_mode" },
};

test("maps one device's entities by translation key and detects its kind", () => {
  const ids = entitiesOf(entities, "d1");
  assert.deepEqual(ids, { room_controller: "climate.office_room_controller", room_mode: "sensor.office_room_mode" });
  assert.equal(kindOf(ids), "room");
  assert.equal(kindOf(entitiesOf(entities, "d2")), "building");
  assert.equal(kindOf({}), undefined);
});
