import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { Api } from '../core/api';
import { fmtTime } from '../core/format';
import { I18n, TPipe } from '../core/i18n';
import { Recording, TrendConfig, TrendPoint } from '../core/models';
import { State } from '../core/state';
import { Toasts } from '../core/toast';

function toLocalInput(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return '';
  const pad = (n: number) => String(n).padStart(2, '0');
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

@Component({
  selector: 'hc-trend',
  imports: [TPipe, RouterLink],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="stack">
      <h1>{{ 'nav_trend' | t }}</h1>
      <div class="card stack">
        <h2>{{ 'trend_buffer' | t }}</h2>
        <div class="row">
          <button (click)="count()" [disabled]="!state.online()">{{ 'trend_count' | t }}</button>
          @if (entries() !== null) {
            <span class="badge info">{{ entries() }} {{ 'trend_entries' | t }}</span>
          }
          <span class="spacer"></span>
          <label class="check" [class.muted]="state.readOnly()">
            <input type="checkbox" [checked]="deleteAfter()" (change)="deleteAfter.set(!deleteAfter())" [disabled]="state.readOnly()" />
            {{ 'trend_delete_after' | t }}
          </label>
          <button class="primary" (click)="downloadTrend()" [disabled]="!state.online() || busy()">⭳ {{ 'trend_download' | t }}</button>
        </div>
        @if (lastDownload(); as r) {
          <div class="banner info">
            {{ r.samples }} {{ 'rec_samples' | t }} ·
            <a [routerLink]="['/recorder', r.id]">{{ 'trend_downloaded' | t }}</a>
          </div>
        }
      </div>

      <div class="card stack">
        <div class="row">
          <h2>{{ 'trend_points' | t }}</h2>
          <span class="spacer"></span>
          <button (click)="loadPoints()" [disabled]="!state.online() || busy()">{{ 'load' | t }}</button>
          <button class="primary" (click)="openEditor()" [disabled]="!selected().size">{{ 'trend_edit' | t }} ({{ selected().size }})</button>
        </div>
        <div class="muted small">{{ 'trend_points_hint' | t }}</div>
        @if (points().length) {
          <div class="scroll">
            <table class="table">
              <thead>
                <tr>
                  <th><input type="checkbox" [checked]="allSelected()" (change)="toggleAll()" /></th>
                  <th>MP</th>
                  <th>{{ 'points_name' | t }}</th>
                  <th>{{ 'trend_type' | t }}</th>
                  <th class="r">{{ 'trend_interval_s' | t }}</th>
                  <th class="r">{{ 'trend_hyst' | t }}</th>
                  <th>{{ 'trend_end' | t }}</th>
                  <th>{{ 'trend_alarm' | t }}</th>
                </tr>
              </thead>
              <tbody>
                @for (p of points(); track p.id) {
                  <tr (dblclick)="selectOnly(p.id)">
                    <td><input type="checkbox" [checked]="selected().has(p.id)" (change)="toggle(p.id)" /></td>
                    <td class="mono muted">{{ p.id }}</td>
                    <td>{{ p.name }}</td>
                    <td>
                      @if (!p.config.enabled) {
                        <span class="muted">{{ 'trend_type_off' | t }}</span>
                      } @else {
                        <span class="badge ok">{{ (p.config.type === 1 ? 'trend_type_hyst' : 'trend_type_interval') | t }}</span>
                      }
                    </td>
                    <td class="r num">{{ p.config.interval }}</td>
                    <td class="r num">{{ p.config.hysteresis }}</td>
                    <td class="num small">{{ fmt(p.config.end) }}</td>
                    <td>{{ p.config.alarm ? '✓' : '' }}{{ p.config.callRemote ? ' ☎' : '' }}</td>
                  </tr>
                }
              </tbody>
            </table>
          </div>
        }
      </div>

      @if (editing(); as ed) {
        <div class="modal-backdrop" (click)="editing.set(null)">
          <div class="card modal stack" (click)="$event.stopPropagation()">
            <h2>{{ 'trend_edit' | t }} ({{ selected().size }})</h2>
            <label class="field">
              {{ 'trend_type' | t }}
              <select [value]="mode()" (change)="setMode($event)">
                <option value="off">{{ 'trend_type_off' | t }}</option>
                <option value="interval">{{ 'trend_type_interval' | t }}</option>
                <option value="hyst">{{ 'trend_type_hyst' | t }}</option>
              </select>
            </label>
            <div class="row">
              <label class="field">
                {{ 'trend_interval_s' | t }}
                <input type="number" min="1" [value]="ed.interval" (input)="patch({ interval: num($event) })" [disabled]="mode() !== 'interval'" />
              </label>
              <label class="field">
                {{ 'trend_hyst' | t }}
                <input type="number" step="0.1" [value]="ed.hysteresis" (input)="patch({ hysteresis: num($event) })" [disabled]="mode() !== 'hyst'" />
              </label>
            </div>
            <label class="field">
              {{ 'trend_end' | t }}
              <input type="datetime-local" [value]="endLocal()" (input)="setEnd($event)" />
            </label>
            <label class="check"><input type="checkbox" [checked]="ed.alarm" (change)="patch({ alarm: !ed.alarm })" />{{ 'trend_alarm' | t }}</label>
            <label class="check">
              <input type="checkbox" [checked]="ed.callRemote" (change)="patch({ callRemote: !ed.callRemote })" [disabled]="!ed.alarm" />{{ 'trend_call' | t }}
            </label>
            @if (state.readOnly()) {
              <div class="banner info">{{ 'read_only_hint' | t }}</div>
            }
            <div class="row">
              <span class="spacer"></span>
              <button (click)="editing.set(null)">{{ 'cancel' | t }}</button>
              <button class="primary" (click)="saveConfig()" [disabled]="!state.canWrite() || busy()">{{ 'save' | t }}</button>
            </div>
          </div>
        </div>
      }
    </div>
  `,
  styles: `
    .small { font-size: 12px; }
    .scroll { max-height: 60vh; overflow: auto; }
    .modal-backdrop { position: fixed; inset: 0; background: rgb(0 0 0 / 40%); display: grid; place-items: center; z-index: 50; padding: 16px; }
    .modal { width: min(440px, 100%); }
  `,
})
export class TrendPage {
  protected state = inject(State);
  private api = inject(Api);
  private toasts = inject(Toasts);
  private i18n = inject(I18n);

  protected entries = signal<number | null>(null);
  protected deleteAfter = signal(false);
  protected busy = signal(false);
  protected lastDownload = signal<Recording | null>(null);
  protected points = signal<TrendPoint[]>([]);
  protected selected = signal(new Set<number>());
  protected editing = signal<TrendConfig | null>(null);

  protected allSelected = computed(() => this.points().length > 0 && this.selected().size === this.points().length);
  protected mode = computed(() => {
    const e = this.editing();
    if (!e || !e.enabled) return 'off';
    return e.type === 1 ? 'hyst' : 'interval';
  });
  protected endLocal = computed(() => toLocalInput(this.editing()?.end ?? ''));

  protected fmt(iso: string) {
    return fmtTime(iso, this.i18n.locale());
  }
  protected num(e: Event): number {
    const v = (e.target as HTMLInputElement).valueAsNumber;
    return Number.isNaN(v) ? 0 : v;
  }

  private async run<T>(fn: () => Promise<T>): Promise<T | undefined> {
    this.busy.set(true);
    try {
      return await fn();
    } catch (e) {
      this.toasts.error(e);
      return undefined;
    } finally {
      this.busy.set(false);
    }
  }

  protected async count() {
    const r = await this.run(() => this.api.trendCount());
    if (r) this.entries.set(r.count);
  }

  protected async downloadTrend() {
    const r = await this.run(() => this.api.downloadTrend(this.deleteAfter() && !this.state.readOnly()));
    if (r) {
      this.lastDownload.set(r);
      this.entries.set(this.deleteAfter() ? 0 : this.entries());
    }
  }

  protected async loadPoints() {
    const r = await this.run(() => this.api.trendPoints());
    if (r) this.points.set(r);
  }

  protected toggle(id: number) {
    this.selected.update((s) => {
      const n = new Set(s);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });
  }
  protected toggleAll() {
    this.selected.set(this.allSelected() ? new Set() : new Set(this.points().map((p) => p.id)));
  }
  protected selectOnly(id: number) {
    this.selected.set(new Set([id]));
    this.openEditor();
  }

  protected openEditor() {
    const first = this.points().find((p) => this.selected().has(p.id));
    if (!first) return;
    const cfg = { ...first.config };
    if (!cfg.end || new Date(cfg.end).getFullYear() < 2000) cfg.end = new Date(2099, 11, 31, 23, 59).toISOString();
    this.editing.set(cfg);
  }

  protected patch(p: Partial<TrendConfig>) {
    this.editing.update((e) => (e ? { ...e, ...p } : e));
  }
  protected setMode(e: Event) {
    const v = (e.target as HTMLSelectElement).value;
    this.patch({ enabled: v !== 'off', type: v === 'hyst' ? 1 : 0 });
  }
  protected setEnd(e: Event) {
    const v = (e.target as HTMLInputElement).value;
    if (v) this.patch({ end: new Date(v).toISOString() });
  }

  protected async saveConfig() {
    const cfg = this.editing();
    if (!cfg) return;
    const ids = [...this.selected()];
    const ok = await this.run(() => this.api.setTrendConfig(ids, cfg).then(() => true));
    if (ok) {
      this.points.update((ps) => ps.map((p) => (ids.includes(p.id) ? { ...p, config: { ...cfg } } : p)));
      this.editing.set(null);
      this.toasts.ok(this.i18n.t('saved'));
    }
  }
}
