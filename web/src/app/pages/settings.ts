import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { Api } from '../core/api';
import { fmtNumber, fmtTime } from '../core/format';
import { I18n, Lang, TPipe } from '../core/i18n';
import { ClockInfo, Settings } from '../core/models';
import { State } from '../core/state';
import { Toasts } from '../core/toast';

@Component({
  selector: 'hc-settings',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="stack">
      <div class="row">
        <h1>{{ 'nav_settings' | t }}</h1>
        <span class="spacer"></span>
        <button class="primary" (click)="save()" [disabled]="!draft() || busy()">{{ 'save' | t }}</button>
      </div>
      @if (draft(); as s) {
        <div class="cols">
          <section class="card stack">
            <h2>{{ 'set_connection' | t }}</h2>
            <label class="field">
              {{ 'set_type' | t }}
              <select [value]="s.device.type" (change)="dev({ type: $any(val($event)) })">
                <option value="tcp">{{ 'set_type_tcp' | t }}</option>
                <option value="serial">{{ 'set_type_serial' | t }}</option>
              </select>
            </label>
            @if (s.device.type === 'tcp') {
              <div class="row">
                <label class="field grow">
                  {{ 'set_host' | t }}
                  <input [value]="s.device.host" (input)="dev({ host: val($event).trim() })" placeholder="10.0.0.5" />
                </label>
                <label class="field">
                  {{ 'set_port' | t }}
                  <input type="number" [value]="s.device.port" (input)="dev({ port: num($event) })" style="width: 90px" />
                </label>
              </div>
            } @else {
              <div class="row">
                <label class="field grow">
                  {{ 'set_serial_port' | t }}
                  <input [value]="s.device.serialPort" (input)="dev({ serialPort: val($event) })" list="ports" placeholder="COM1 / /dev/ttyUSB0" />
                  <datalist id="ports">
                    @for (p of ports(); track p) {
                      <option [value]="p"></option>
                    }
                  </datalist>
                </label>
                <label class="field">
                  {{ 'set_baud' | t }}
                  <select [value]="s.device.baud" (change)="dev({ baud: +val($event) })">
                    @for (b of [9600, 19200, 38400, 57600]; track b) {
                      <option [value]="b" [selected]="b === s.device.baud">{{ b }}</option>
                    }
                  </select>
                </label>
              </div>
            }
            <label class="field">
              {{ 'set_expected_id' | t }}
              <input type="number" [value]="s.device.expectedId" (input)="dev({ expectedId: num($event) })" />
            </label>
            <label class="check"><input type="checkbox" [checked]="s.device.autoConnect" (change)="dev({ autoConnect: !s.device.autoConnect })" />{{ 'set_auto_connect' | t }}</label>
            <label class="field">
              {{ 'set_poll' | t }}
              <input type="number" min="10" [value]="s.pollSeconds" (input)="set({ pollSeconds: num($event) })" style="width: 90px" />
            </label>
          </section>

          <section class="card stack">
            <h2>{{ 'set_safety' | t }}</h2>
            <label class="check"><input type="checkbox" [checked]="s.readOnly" (change)="toggleReadOnly()" /><strong>{{ 'set_read_only' | t }}</strong></label>
            <label class="check"><input type="checkbox" [checked]="s.expertMode" (change)="set({ expertMode: !s.expertMode })" />{{ 'set_expert' | t }}</label>
            <label class="field">
              {{ 'set_write_budget' | t }}
              <input type="number" min="-1" [value]="s.writeBudgetPerDay || 48" (input)="set({ writeBudgetPerDay: num($event) })" style="width: 90px" />
            </label>
            <label class="field">
              {{ 'set_history_days' | t }}
              <input type="number" min="1" [value]="s.historyDays" (input)="set({ historyDays: num($event) })" style="width: 90px" />
            </label>
            <label class="check">
              <input type="checkbox" [checked]="s.trendPauseDuringDownload" (change)="set({ trendPauseDuringDownload: !s.trendPauseDuringDownload })" />{{
                'set_trend_pause' | t
              }}
            </label>
          </section>

          <section class="card stack">
            <h2>{{ 'set_mqtt' | t }}</h2>
            <div class="muted small">{{ 'set_mqtt_hint' | t }}</div>
            <label class="check"><input type="checkbox" [checked]="s.mqtt.enabled" (change)="mq({ enabled: !s.mqtt.enabled })" />{{ 'set_mqtt_enabled' | t }}</label>
            <label class="field">
              {{ 'set_mqtt_broker' | t }}
              <input [value]="s.mqtt.broker" (input)="mq({ broker: val($event).trim() })" placeholder="tcp://homeassistant.local:1883" />
            </label>
            <div class="row">
              <label class="field grow">
                {{ 'set_mqtt_user' | t }}
                <input [value]="s.mqtt.username" (input)="mq({ username: val($event) })" autocomplete="off" />
              </label>
              <label class="field grow">
                {{ 'set_mqtt_password' | t }}
                <input type="password" [value]="s.mqtt.password ?? ''" (input)="mq({ password: val($event) })" autocomplete="new-password"
                  [placeholder]="s.mqttPasswordSet ? ('set_mqtt_password_keep' | t) : ''" />
              </label>
            </div>
            <label class="field">
              {{ 'set_mqtt_language' | t }}
              <select (change)="mq({ language: $any(val($event)) })">
                <option value="en" [selected]="s.mqtt.language !== 'de'">English</option>
                <option value="de" [selected]="s.mqtt.language === 'de'">Deutsch</option>
              </select>
            </label>
            <div class="row">
              <label class="field grow">
                {{ 'set_mqtt_discovery' | t }}
                <input [value]="s.mqtt.discoveryPrefix" (input)="mq({ discoveryPrefix: val($event) })" />
              </label>
              <label class="field grow">
                {{ 'set_mqtt_topic' | t }}
                <input [value]="s.mqtt.topicPrefix" (input)="mq({ topicPrefix: val($event) })" />
              </label>
            </div>
          </section>

          <section class="card stack">
            <h2>{{ 'set_clock' | t }}</h2>
            <div class="row">
              <button (click)="readClock()" [disabled]="!state.online()">{{ 'set_clock_read' | t }}</button>
              <button (click)="syncClock()" [disabled]="!state.canWrite()">{{ 'set_clock_sync' | t }}</button>
            </div>
            @if (clock(); as c) {
              <table class="table">
                <tr><td>{{ 'set_clock_device' | t }}</td><td class="num">{{ fmt(c.device) }}</td></tr>
                <tr><td>{{ 'set_clock_server' | t }}</td><td class="num">{{ fmt(c.server) }}</td></tr>
                <tr><td>{{ 'set_clock_diff' | t }}</td><td class="num">{{ diff(c.offsetSeconds) }} s</td></tr>
              </table>
            }
          </section>

          <section class="card stack">
            <h2>{{ 'set_points' | t }}</h2>
            <div class="muted small">{{ 'set_points_hint' | t }}</div>
            <div><button (click)="initPoints()" [disabled]="!state.online() || !!state.job()">{{ 'set_points_init' | t }}</button></div>
          </section>

          <section class="card stack">
            <h2>{{ 'set_password' | t }}</h2>
            @if (s.passwordSet) {
              <label class="field">
                {{ 'set_password_current' | t }}
                <input type="password" [value]="pwCurrent()" (input)="pwCurrent.set(val($event))" autocomplete="current-password" />
              </label>
            }
            <label class="field">
              {{ 'set_password_new' | t }}
              <input type="password" [value]="pwNew()" (input)="pwNew.set(val($event))" autocomplete="new-password" />
            </label>
            <div><button (click)="setPassword()">{{ 'set_password_set' | t }}</button></div>
          </section>

          <section class="card stack">
            <h2>{{ 'set_appearance' | t }}</h2>
            <div class="row">
              <label class="field">
                {{ 'set_language' | t }}
                <select [value]="i18n.lang()" (change)="i18n.setLang($any(val($event)))">
                  <option value="de">Deutsch</option>
                  <option value="en">English</option>
                </select>
              </label>
              <label class="field">
                {{ 'set_theme' | t }}
                <select [value]="theme()" (change)="setTheme(val($event))">
                  <option value="auto">{{ 'set_theme_auto' | t }}</option>
                  <option value="light">{{ 'set_theme_light' | t }}</option>
                  <option value="dark">{{ 'set_theme_dark' | t }}</option>
                </select>
              </label>
            </div>
          </section>
        </div>
      } @else {
        <div class="muted">{{ 'loading' | t }}</div>
      }
    </div>
  `,
  styles: `
    .cols { display: grid; grid-template-columns: repeat(auto-fill, minmax(340px, 1fr)); gap: 16px; align-items: start; }
    .grow { flex: 1; }
    .small { font-size: 12px; }
  `,
})
export class SettingsPage {
  protected state = inject(State);
  protected i18n = inject(I18n);
  private api = inject(Api);
  private toasts = inject(Toasts);

  protected draft = signal<Settings | null>(null);
  protected ports = signal<string[]>([]);
  protected busy = signal(false);
  protected clock = signal<ClockInfo | null>(null);
  protected pwCurrent = signal('');
  protected pwNew = signal('');
  protected theme = signal(document.documentElement.dataset['theme'] ?? 'auto');

  constructor() {
    this.load();
    this.api.serialPorts().then((p) => this.ports.set(p), () => undefined);
  }

  private async load() {
    try {
      this.draft.set(await this.api.settings());
    } catch (e) {
      this.toasts.error(e);
    }
  }

  protected val(e: Event): string {
    return (e.target as HTMLInputElement).value;
  }
  protected num(e: Event): number {
    const v = (e.target as HTMLInputElement).valueAsNumber;
    return Number.isNaN(v) ? 0 : v;
  }
  protected fmt(t: string) {
    return fmtTime(t, this.i18n.locale());
  }
  protected diff(s: number) {
    return fmtNumber(s, this.i18n.locale(), 0);
  }

  protected set(p: Partial<Settings>) {
    this.draft.update((s) => (s ? { ...s, ...p } : s));
  }
  protected dev(p: Partial<Settings['device']>) {
    this.draft.update((s) => (s ? { ...s, device: { ...s.device, ...p } } : s));
  }
  protected mq(p: Partial<Settings['mqtt']>) {
    this.draft.update((s) => (s ? { ...s, mqtt: { ...s.mqtt, ...p } } : s));
  }

  protected toggleReadOnly() {
    const s = this.draft();
    if (!s) return;
    if (s.readOnly && !confirm(this.i18n.t('set_read_only_off_confirm'))) {
      // re-render the checkbox in its old state
      this.draft.set({ ...s });
      return;
    }
    this.set({ readOnly: !s.readOnly });
  }

  protected async save() {
    const s = this.draft();
    if (!s) return;
    this.busy.set(true);
    try {
      this.draft.set(await this.api.saveSettings(s));
      this.toasts.ok(this.i18n.t('saved'));
    } catch (e) {
      this.toasts.error(e);
    } finally {
      this.busy.set(false);
    }
  }

  protected async readClock() {
    try {
      this.clock.set(await this.api.clock());
    } catch (e) {
      this.toasts.error(e);
    }
  }

  protected async syncClock() {
    try {
      await this.api.syncClock();
      await this.readClock();
    } catch (e) {
      this.toasts.error(e);
    }
  }

  protected async initPoints() {
    try {
      await this.api.initPoints();
    } catch (e) {
      this.toasts.error(e);
    }
  }

  protected async setPassword() {
    try {
      await this.api.setPassword(this.pwCurrent(), this.pwNew());
      this.pwCurrent.set('');
      this.pwNew.set('');
      await this.load();
      this.toasts.ok(this.i18n.t('saved'));
    } catch (e) {
      this.toasts.error(e);
    }
  }

  protected setTheme(t: string) {
    this.theme.set(t);
    const root = document.documentElement;
    if (t === 'auto') delete root.dataset['theme'];
    else root.dataset['theme'] = t;
    try {
      localStorage.setItem('theme', t);
    } catch {
      /* ignore */
    }
  }

  protected setLang(l: Lang) {
    this.i18n.setLang(l);
  }
}
