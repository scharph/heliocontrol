import { ChangeDetectionStrategy, Component, DestroyRef, computed, effect, inject, input, signal, untracked } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { Router, RouterLink } from '@angular/router';
import { Api } from '../core/api';
import { fmtNumber, fmtTime } from '../core/format';
import { I18n, Key, TPipe } from '../core/i18n';
import { Point, Recording, Series } from '../core/models';
import { State } from '../core/state';
import { Toasts } from '../core/toast';
import { ChartSeries, ChartView, TimeChart } from '../ui/time-chart';

const windows: { key: string; ms?: number }[] = [
  { key: 'all' },
  { key: '15 min', ms: 15 * 60_000 },
  { key: '1 h', ms: 3600_000 },
  { key: '6 h', ms: 6 * 3600_000 },
  { key: '24 h', ms: 24 * 3600_000 },
  { key: '7 d', ms: 7 * 24 * 3600_000 },
];

@Component({
  selector: 'hc-recorder',
  imports: [TPipe, TimeChart, RouterLink],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="layout">
      <div class="stack side">
        <div class="card stack">
          @if (state.recording(); as r) {
            <div class="row">
              <span class="badge" [class.ok]="r.state === 'recording'" [class.warn]="r.state === 'paused'">
                <span class="dot"></span>{{ (r.state === 'paused' ? 'rec_paused' : 'rec_running') | t }}
              </span>
              <a [routerLink]="['/recorder', r.id]">{{ r.name }}</a>
            </div>
            <div class="muted small">{{ r.samples }} {{ 'rec_samples' | t }} · {{ r.points.length }} {{ 'rec_points' | t }}</div>
            <div class="row">
              @if (r.state === 'paused') {
                <button (click)="action('resume')">▶ {{ 'rec_resume' | t }}</button>
              } @else {
                <button (click)="action('pause')">⏸ {{ 'rec_pause' | t }}</button>
              }
              <button class="danger" (click)="action('stop')">■ {{ 'rec_stop' | t }}</button>
            </div>
          } @else {
            <h2>{{ 'rec_add_points' | t }}</h2>
            @if (!points().length) {
              <div class="muted small">{{ 'points_empty' | t }}</div>
            } @else {
              <input type="search" [placeholder]="'rec_search' | t" (input)="filter.set(val($event))" />
              <div class="picklist">
                @for (p of filtered(); track p.id) {
                  <label class="check">
                    <input type="checkbox" [checked]="picked().has(p.id)" (change)="toggle(p.id)" />
                    <span class="mono muted">{{ p.id }}</span> {{ p.name }}
                  </label>
                }
              </div>
              <div class="muted small">{{ picked().size }} {{ 'rec_points' | t }}</div>
            }
            <div class="row">
              <label class="field">
                {{ 'rec_interval' | t }}
                <input type="number" min="1" [value]="interval()" (input)="interval.set(num($event) ?? 5)" style="width: 80px" />
              </label>
              <label class="field">
                &nbsp;
                <select [value]="unit()" (change)="unit.set(+val($event))">
                  <option [value]="1" [selected]="unit() === 1">{{ 'sec' | t }}</option>
                  <option [value]="60" [selected]="unit() === 60">{{ 'min' | t }}</option>
                  <option [value]="3600" [selected]="unit() === 3600">{{ 'hour' | t }}</option>
                </select>
              </label>
            </div>
            <label class="field">
              {{ 'rec_name' | t }}
              <input [value]="name()" (input)="name.set(val($event))" placeholder="WEB-Online …" />
            </label>
            <button class="primary" (click)="start()" [disabled]="!state.online() || !picked().size">● {{ 'rec_start' | t }}</button>
          }
        </div>

        <div class="card stack">
          <div class="row">
            <h2>{{ 'rec_recordings' | t }}</h2>
          </div>
          <label class="btn small">⭱ {{ 'rec_import' | t }}<input type="file" accept=".hwo,.htd" hidden (change)="importFile($event)" /></label>
          @if (!recordings().length) {
            <div class="muted small">{{ 'rec_none' | t }}</div>
          }
          <div class="reclist">
            @for (r of recordings(); track r.id) {
              <a class="rec" [class.active]="r.id === selected()?.id" [routerLink]="['/recorder', r.id]">
                <div class="row">
                  <span class="badge" [class.info]="r.source === 'trend'" [class.warn]="r.source === 'import'">{{ sourceKey(r) | t }}</span>
                  <strong class="name">{{ r.name }}</strong>
                </div>
                <div class="muted small">{{ fmt(r.started) }} · {{ r.samples }} {{ 'rec_samples' | t }}</div>
              </a>
            }
          </div>
        </div>
      </div>

      <div class="card stack main">
        @if (selected(); as r) {
          <div class="row">
            <h2>{{ r.name }}</h2>
            <span class="spacer"></span>
            <button class="small" (click)="rename(r)">{{ 'rename' | t }}</button>
            <a class="btn small" [href]="'/api/recordings/' + r.id + '/csv'">⭳ {{ 'rec_csv' | t }}</a>
            <button class="small danger" (click)="remove(r)" [disabled]="r.id === state.recording()?.id">{{ 'delete' | t }}</button>
          </div>
          <div class="row controls">
            <label class="field">
              {{ 'rec_window' | t }}
              <select [value]="windowKey()" (change)="windowKey.set(val($event))">
                @for (w of windows; track w.key) {
                  <option [value]="w.key" [selected]="w.key === windowKey()">{{ w.key === 'all' ? ('rec_window_all' | t) : w.key }}</option>
                }
              </select>
            </label>
            <label class="check"><input type="checkbox" [checked]="manualY()" (change)="manualY.set(!manualY())" />{{ 'rec_value_axis' | t }}</label>
            @if (manualY()) {
              <input type="number" [value]="yMin()" (input)="yMin.set(num($event) ?? 0)" style="width: 70px" />
              <span>…</span>
              <input type="number" [value]="yMax()" (input)="yMax.set(num($event) ?? 100)" style="width: 70px" />
            }
            <label class="check"><input type="checkbox" [checked]="relative()" (change)="relative.set(!relative())" />{{ 'rec_relative' | t }}</label>
            <label class="check"><input type="checkbox" [checked]="interpolate()" (change)="interpolate.set(!interpolate())" />〰</label>
            <span class="spacer"></span>
            <span class="muted small">{{ 'rec_cursor_hint' | t }}</span>
          </div>
          <hc-time-chart [series]="chartSeries()" [view]="view()" (pick)="onPick($event)" />
          <table class="table legend">
            <thead>
              <tr>
                <th></th>
                <th>{{ 'rec_points' | t }}</th>
                <th class="r">A {{ cursorA() ? fmtT(cursorA()!) : '' }}</th>
                <th class="r">B {{ cursorB() ? fmtT(cursorB()!) : '' }}</th>
                <th class="r">{{ 'rec_delta' | t }}</th>
              </tr>
            </thead>
            <tbody>
              @for (s of chartSeries(); track s.id) {
                <tr>
                  <td><input type="checkbox" [checked]="s.visible" (change)="toggleVisible(s.id)" [style.accent-color]="s.color" /></td>
                  <td><i class="sw" [style.background]="s.color"></i>{{ s.label }} <span class="muted">{{ s.unit }}</span></td>
                  <td class="r num">{{ at(s, cursorA()) }}</td>
                  <td class="r num">{{ at(s, cursorB()) }}</td>
                  <td class="r num">{{ delta(s) }}</td>
                </tr>
              }
            </tbody>
          </table>
        } @else {
          <div class="muted">{{ 'rec_select' | t }}</div>
        }
      </div>
    </div>
  `,
  styles: `
    .layout { display: grid; grid-template-columns: 320px minmax(0, 1fr); gap: 16px; align-items: start; }
    .small { font-size: 12px; }
    .picklist { max-height: 260px; overflow: auto; display: flex; flex-direction: column; gap: 2px; border: 1px solid var(--border); border-radius: 8px; padding: 6px; }
    .picklist label { font-size: 13px; }
    .reclist { display: flex; flex-direction: column; gap: 4px; max-height: 420px; overflow: auto; }
    .rec { display: block; padding: 8px; border-radius: 8px; color: var(--text); text-decoration: none; border: 1px solid transparent; }
    .rec:hover { background: var(--surface-2); }
    .rec.active { border-color: var(--accent); background: var(--accent-soft); }
    .rec .name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
    .controls { align-items: end; }
    .sw { display: inline-block; width: 10px; height: 10px; border-radius: 2px; margin-right: 6px; }
    .legend td { padding: 3px 8px; }
    @media (max-width: 1000px) { .layout { grid-template-columns: 1fr; } }
  `,
})
export class RecorderPage {
  /** Route parameter: recording id. */
  readonly id = input<string>();
  protected state = inject(State);
  private api = inject(Api);
  private toasts = inject(Toasts);
  private i18n = inject(I18n);
  private router = inject(Router);

  protected readonly windows = windows;
  protected points = signal<Point[]>([]);
  protected filter = signal('');
  protected picked = signal(new Set<number>());
  protected interval = signal(5);
  protected unit = signal(1);
  protected name = signal('');
  protected recordings = signal<Recording[]>([]);
  protected series = signal<Series[]>([]);
  protected hidden = signal(new Set<number>());
  protected windowKey = signal('all');
  protected manualY = signal(false);
  protected yMin = signal(0);
  protected yMax = signal(60);
  protected relative = signal(false);
  protected interpolate = signal(true);
  protected cursorA = signal<number | undefined>(undefined);
  protected cursorB = signal<number | undefined>(undefined);

  protected filtered = computed(() => {
    const f = this.filter().toLowerCase();
    return this.points().filter((p) => p.used && (!f || p.name.toLowerCase().includes(f) || String(p.id) === f));
  });
  protected selected = computed(() => {
    const id = Number(this.id());
    return this.recordings().find((r) => r.id === id);
  });
  protected chartSeries = computed<ChartSeries[]>(() => {
    const r = this.selected();
    if (!r) return [];
    const hidden = this.hidden();
    return r.points.map((p) => {
      const s = this.series().find((x) => x.pointId === p.id);
      return {
        id: p.id,
        label: p.name,
        color: p.color || '#888',
        unit: p.unit,
        visible: !hidden.has(p.id),
        points: s ? s.t.map((t, i) => ({ x: t, y: s.v[i] })) : [],
      };
    });
  });
  protected view = computed<ChartView>(() => {
    const r = this.selected();
    const w = windows.find((x) => x.key === this.windowKey());
    let to: number | undefined;
    let from: number | undefined;
    if (w?.ms && r) {
      to = r.ended && r.state === 'stopped' ? Date.parse(r.ended) : Date.now();
      from = to - w.ms;
    }
    return {
      from,
      to,
      yMin: this.manualY() ? this.yMin() : undefined,
      yMax: this.manualY() ? this.yMax() : undefined,
      relativeTo: this.relative() && r ? Date.parse(r.started) : undefined,
      interpolate: this.interpolate(),
      cursorA: this.cursorA(),
      cursorB: this.cursorB(),
    };
  });

  constructor() {
    this.api.points('mp').then((p) => this.points.set(p), () => undefined);
    this.loadRecordings();
    // load samples when the selection changes
    effect(() => {
      const r = this.selected();
      untracked(() => {
        this.cursorA.set(undefined);
        this.cursorB.set(undefined);
        this.series.set([]);
        if (r) this.api.samples(r.id).then((s) => this.series.set(s), (e) => this.toasts.error(e));
      });
    });
    // reload the list when a recording starts or stops
    effect(() => {
      this.state.recording()?.id;
      this.state.recording()?.state;
      untracked(() => this.loadRecordings());
    });
    // live samples of the selected recording
    this.state.samples.pipe(takeUntilDestroyed(inject(DestroyRef))).subscribe((x) => {
      if (x.recordingId !== this.selected()?.id) return;
      this.series.update((all) => {
        const out = all.map((s) => ({ ...s }));
        for (const [k, v] of Object.entries(x.values)) {
          const pid = Number(k);
          let s = out.find((q) => q.pointId === pid);
          if (!s) {
            s = { pointId: pid, t: [], v: [] };
            out.push(s);
          }
          s.t = [...s.t, x.t];
          s.v = [...s.v, v];
        }
        return out;
      });
    });
  }

  private async loadRecordings() {
    try {
      this.recordings.set(await this.api.recordings());
    } catch (e) {
      this.toasts.error(e);
    }
  }

  protected val(e: Event): string {
    return (e.target as HTMLInputElement).value;
  }
  protected num(e: Event): number | undefined {
    const v = (e.target as HTMLInputElement).valueAsNumber;
    return Number.isNaN(v) ? undefined : v;
  }
  protected fmt(iso: string) {
    return fmtTime(iso, this.i18n.locale());
  }
  protected fmtT(ms: number) {
    return fmtTime(ms, this.i18n.locale());
  }
  protected sourceKey(r: Recording): Key {
    return `rec_source_${r.source}` as Key;
  }

  protected toggle(id: number) {
    this.picked.update((s) => {
      const n = new Set(s);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });
  }
  protected toggleVisible(id: number) {
    this.hidden.update((s) => {
      const n = new Set(s);
      if (n.has(id)) n.delete(id);
      else n.add(id);
      return n;
    });
  }

  protected async start() {
    try {
      const r = await this.api.startRecording(this.name(), [...this.picked()], this.interval() * this.unit());
      await this.loadRecordings();
      this.router.navigate(['/recorder', r.id]);
    } catch (e) {
      this.toasts.error(e);
    }
  }

  protected async action(a: 'pause' | 'resume' | 'stop') {
    try {
      await this.api.recordingAction(a);
      if (a === 'stop') await this.loadRecordings();
    } catch (e) {
      this.toasts.error(e);
    }
  }

  protected async rename(r: Recording) {
    const name = prompt(this.i18n.t('rename'), r.name);
    if (!name) return;
    try {
      await this.api.renameRecording(r.id, name);
      await this.loadRecordings();
    } catch (e) {
      this.toasts.error(e);
    }
  }

  protected async remove(r: Recording) {
    if (!confirm(`${this.i18n.t('rec_confirm_delete')}\n${r.name}`)) return;
    try {
      await this.api.deleteRecording(r.id);
      await this.loadRecordings();
      this.router.navigate(['/recorder']);
    } catch (e) {
      this.toasts.error(e);
    }
  }

  protected async importFile(e: Event) {
    const input = e.target as HTMLInputElement;
    const f = input.files?.[0];
    input.value = '';
    if (!f) return;
    try {
      const r = await this.api.importFile(f);
      await this.loadRecordings();
      this.router.navigate(['/recorder', r.id]);
    } catch (err) {
      this.toasts.error(err);
    }
  }

  protected onPick(p: { t: number; shift: boolean }) {
    (p.shift ? this.cursorB : this.cursorA).set(p.t);
  }

  /** Value of a series at time t (nearest earlier sample). */
  private valueAt(s: ChartSeries, t: number | undefined): number | undefined {
    if (t === undefined || !s.points.length) return undefined;
    let lo = 0;
    let hi = s.points.length - 1;
    if (t < s.points[0].x) return undefined;
    while (lo < hi) {
      const mid = (lo + hi + 1) >> 1;
      if (s.points[mid].x <= t) lo = mid;
      else hi = mid - 1;
    }
    return s.points[lo].y;
  }
  protected at(s: ChartSeries, t: number | undefined) {
    const v = this.valueAt(s, t);
    return v === undefined ? '' : fmtNumber(v, this.i18n.locale(), 2);
  }
  protected delta(s: ChartSeries) {
    const a = this.valueAt(s, this.cursorA());
    const b = this.valueAt(s, this.cursorB());
    return a === undefined || b === undefined ? '' : fmtNumber(b - a, this.i18n.locale(), 2);
  }
}
