// Smart Climate card: one card per controller device (room, building, flow temperature, mixer).
// Served by the integration itself; no build step, no dependencies.

const DOMAIN = "smart_climate";
// The entity that identifies each controller type (translation keys from the integration).
const KINDS = { room_controller: "room", plant_mode: "building", flow_target: "temperature", mixer_mode: "mixer" };

const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);
const pct = (state) => Math.max(0, Math.min(100, parseFloat(state?.state) || 0));
const missing = (state) => !state || state.state === "unavailable" || state.state === "unknown";

// translation_key -> entity_id for all entities of one Smart Climate device
export function entitiesOf(entities, deviceId) {
  const ids = {};
  for (const e of Object.values(entities)) {
    if (e.platform === DOMAIN && e.device_id === deviceId && e.translation_key) ids[e.translation_key] = e.entity_id;
  }
  return ids;
}

export function kindOf(ids) {
  return Object.entries(KINDS).find(([key]) => key in ids)?.[1];
}

class SmartClimateCard extends HTMLElement {
  static getConfigForm() {
    return { schema: [{ name: "device", required: true, selector: { device: { integration: DOMAIN } } }] };
  }

  static getStubConfig(hass) {
    const entity = Object.values(hass.entities).find((e) => e.platform === DOMAIN && e.device_id);
    return { device: entity?.device_id ?? "" };
  }

  constructor() {
    super();
    this.attachShadow({ mode: "open" });
    this.shadowRoot.addEventListener("click", (ev) => this._click(ev));
  }

  setConfig(config) {
    if (!config.device) throw new Error("Select a Smart Climate device");
    this._config = config;
    this._seen = undefined;
  }

  getCardSize() {
    return 4;
  }

  set hass(hass) {
    this._hass = hass;
    this._ids = entitiesOf(hass.entities, this._config.device);
    // State objects are replaced on every change, so identity tells us whether to re-render.
    const states = Object.values(this._ids).map((id) => hass.states[id]);
    if (this._seen && states.length === this._seen.length && states.every((s, i) => s === this._seen[i])) return;
    this._seen = states;
    this._render();
  }

  _st(key) {
    return this._hass.states[this._ids[key]];
  }

  _fmt(key) {
    const state = this._st(key);
    return missing(state) ? "—" : esc(this._hass.formatEntityState(state));
  }

  _value(key, label) {
    return `<div class="value" data-more="${esc(this._ids[key])}"><span>${label}</span><b>${this._fmt(key)}</b></div>`;
  }

  _bar(key, label, color) {
    const state = this._st(key);
    return `<div class="bar" data-more="${esc(this._ids[key])}">
      <span>${label}</span><div class="track"><div style="width:${pct(state)}%;background:${color}"></div></div>
      <b>${this._fmt(key)}</b></div>`;
  }

  _chip(key) {
    const state = this._st(key);
    return `<span class="chip ${esc(state?.state)}" data-more="${esc(this._ids[key])}">${this._fmt(key)}</span>`;
  }

  _reason(key) {
    const state = this._st(key);
    return missing(state) ? "" : `<div class="reason" data-more="${esc(this._ids[key])}">${esc(state.state.replaceAll("_", " "))}</div>`;
  }

  _room() {
    const climate = this._st("room_controller");
    const on = climate && climate.state !== "off";
    const a = climate?.attributes ?? {};
    const cooling = (a.hvac_modes ?? []).includes("heat_cool");
    const shading = this._st("shading_reason");
    const badges = [
      this._st("window_protection")?.state === "on" ? `<span class="badge" data-more="${esc(this._ids.window_protection)}">🪟 Window open</span>` : "",
      pct(this._st("preheat_minutes")) > 0 ? `<span class="badge" data-more="${esc(this._ids.preheat_minutes)}">⏱ Preheat ${this._fmt("preheat_minutes")}</span>` : "",
      missing(this._st("heating_rate")) ? "" : `<span class="badge" data-more="${esc(this._ids.heating_rate)}">📈 ${this._fmt("heating_rate")}</span>`,
      this._st("sun_on_window")?.state === "on" ? `<span class="badge" data-more="${esc(this._ids.shading_tilt)}">☀️ Slats ${this._fmt("shading_tilt")}</span>` : "",
    ].join("");
    return `
      <div class="head">${this._chip("room_mode")}
        <button class="power ${on ? "on" : ""}" data-power title="On/off"><ha-icon icon="mdi:power"></ha-icon></button></div>
      <div class="temps" data-more="${esc(this._ids.room_controller)}">
        <div class="current">${a.current_temperature ?? "—"}<small>°C</small></div>
        <div class="target">
          <button data-step="-0.5" ${on ? "" : "disabled"}>−</button>
          <span>${a.temperature ?? "—"} °C</span>
          <button data-step="0.5" ${on ? "" : "disabled"}>+</button>
        </div>
      </div>
      ${this._bar("heating_demand", "Heat", "var(--state-climate-heat-color, #ff8100)")}
      ${cooling ? this._bar("cooling_demand", "Cool", "var(--state-climate-cool-color, #2b9af9)") : ""}
      ${badges ? `<div class="badges">${badges}</div>` : ""}
      ${shading && shading.state !== "not_configured" ? this._reason("shading_reason") : ""}`;
  }

  _building() {
    const stage2 = this._st("plant_stage2")?.state === "on";
    return `
      <div class="head">${this._chip("plant_mode")}${stage2 ? `<span class="badge">Stage 2</span>` : ""}</div>
      ${this._bar("plant_heating_demand", "Heat", "var(--state-climate-heat-color, #ff8100)")}
      ${this._bar("plant_cooling_demand", "Cool", "var(--state-climate-cool-color, #2b9af9)")}
      ${this._reason("plant_reason")}`;
  }

  _temperature() {
    return `
      <div class="values">${this._value("flow_target", "Flow")}${this._value("buffer_target", "Buffer")}</div>
      ${this._reason("temperature_reason")}`;
  }

  _mixer() {
    return `
      <div class="head">${this._chip("mixer_mode")}</div>
      ${this._bar("mixer_position", "Valve", "var(--primary-color)")}
      <div class="values">${this._value("mixer_error", "Error")}</div>
      ${this._reason("mixer_reason")}`;
  }

  _render() {
    const device = this._hass.devices?.[this._config.device];
    const title = esc(this._config.title ?? device?.name_by_user ?? device?.name ?? "Smart Climate");
    const kind = kindOf(this._ids);
    const body = kind ? this[`_${kind}`]() : `<div class="reason">No Smart Climate entities found for this device.</div>`;
    this.shadowRoot.innerHTML = `<style>${STYLE}</style><ha-card><div class="title">${title}</div>${body}</ha-card>`;
  }

  _click(ev) {
    const el = ev.target.closest("[data-step],[data-power],[data-more]");
    if (!el) return;
    ev.stopPropagation();
    const climate = this._ids.room_controller;
    if (el.dataset.step) {
      const target = this._st("room_controller")?.attributes.temperature;
      if (target == null) return;
      this._hass.callService("climate", "set_temperature", { entity_id: climate, temperature: target + parseFloat(el.dataset.step) });
    } else if ("power" in el.dataset) {
      const state = this._st("room_controller");
      const mode = state.state === "off" ? state.attributes.hvac_modes.find((m) => m !== "off") : "off";
      this._hass.callService("climate", "set_hvac_mode", { entity_id: climate, hvac_mode: mode });
    } else if (el.dataset.more) {
      this.dispatchEvent(new CustomEvent("hass-more-info", { detail: { entityId: el.dataset.more }, bubbles: true, composed: true }));
    }
  }
}

const STYLE = `
  ha-card { padding: 16px; display: flex; flex-direction: column; gap: 12px; }
  [data-more] { cursor: pointer; }
  .title { font-size: 1.2em; font-weight: 500; }
  .head { display: flex; align-items: center; gap: 8px; }
  .chip { padding: 2px 10px; border-radius: 12px; background: var(--secondary-background-color); font-size: .9em; }
  .chip.heating, .chip.comfort, .chip.preheat { background: var(--state-climate-heat-color, #ff8100); color: #fff; }
  .chip.cooling { background: var(--state-climate-cool-color, #2b9af9); color: #fff; }
  .chip.protection, .chip.safety { background: var(--error-color, #db4437); color: #fff; }
  .power { margin-left: auto; border: none; background: none; color: var(--disabled-text-color); cursor: pointer; }
  .power.on { color: var(--state-climate-heat-color, #ff8100); }
  .temps { display: flex; align-items: center; justify-content: space-between; }
  .current { font-size: 2.6em; line-height: 1; }
  .current small { font-size: .4em; color: var(--secondary-text-color); }
  .target { display: flex; align-items: center; gap: 8px; }
  .target button { width: 32px; height: 32px; border-radius: 50%; border: 1px solid var(--divider-color);
    background: none; color: var(--primary-text-color); font-size: 1.2em; cursor: pointer; }
  .target button:disabled { opacity: .4; cursor: default; }
  .bar { display: grid; grid-template-columns: 48px 1fr 56px; align-items: center; gap: 8px; }
  .bar b, .value b { text-align: right; font-weight: 500; }
  .track { height: 8px; border-radius: 4px; background: var(--secondary-background-color); overflow: hidden; }
  .track div { height: 100%; transition: width .5s; }
  .values { display: flex; gap: 24px; }
  .value { display: flex; flex-direction: column; }
  .value span, .bar span { color: var(--secondary-text-color); font-size: .9em; }
  .value b { font-size: 1.6em; text-align: left; }
  .badges { display: flex; flex-wrap: wrap; gap: 6px; }
  .badge { padding: 2px 8px; border-radius: 12px; border: 1px solid var(--divider-color); font-size: .85em; }
  .reason { color: var(--secondary-text-color); font-size: .85em; }
  .reason::first-letter { text-transform: uppercase; }
`;

if (!customElements.get("smart-climate-card")) {
  customElements.define("smart-climate-card", SmartClimateCard);
  window.customCards = window.customCards ?? [];
  window.customCards.push({
    type: "smart-climate-card",
    name: "Smart Climate",
    description: "Room, building, flow temperature or mixer controller",
  });
}
