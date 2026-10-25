import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { Api } from '../core/api';
import { fmtNumber } from '../core/format';
import { I18n, TPipe, TxPipe } from '../core/i18n';
import { Insights, Series } from '../core/models';
import { Toasts } from '../core/toast';
import { ChartSeries, ChartView, TimeChart } from '../ui/time-chart';

const historyPoints = [
  { ref: 'mp:0', id: 0, color: '#2563eb', label: { de: 'Außentemperatur', en: 'Outdoor' } },
  { ref: 'mp:3', id: 3, color: '#dc2626', label: { de: 'Vorlauf', en: 'Flow' } },
  { ref: 'mp:4', id: 4, color: '#ea580c', label: { de: 'Rücklauf', en: 'Return' } },
  { ref: 'mp:57', id: 57, color: '#6b7280', label: { de: 'Rücklauf Soll', en: 'Return target' } },
];
const ranges = [
  { key: '24 h', ms: 24 * 3600_000 },
  { key: '7 d', ms: 7 * 24 * 3600_000 },
  { key: '30 d', ms: 30 * 24 * 3600_000 },
];

/** Efficiency insights from the continuous history. */
@Component({
  selector: 'hc-insights',
  imports: [TPipe, TxPipe, TimeChart],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="stack">
      <div class="row">
        <h1>{{ 'nav_insights' | t }}</h1>
        <span class="spacer"></span>
        <label class="field">
          {{ 'ins_days' | t }}
          <select (change)="setDays(+val($event))">
            @for (d of [7, 14, 30, 60]; track d) {
              <option [value]="d" [selected]="d === days()">{{ d }}</option>
            }
          </select>
        </label>
      </div>

      <div class="card stack">
        <h2>{{ 'ins_hints' | t }}</h2>
        @if (!hasData()) {
          <div class="muted">{{ 'ins_collecting' | t }}</div>
        } @else {
          @for (h of insights()?.hints ?? []; track $index) {
            <div class="banner" [class.warn]="h.level === 'warn'" [class.info]="h.level === 'info'">
              <div>
                <strong>{{ h.title | tx }}</strong>
                <div>{{ h.detail | tx }}</div>
                @if (h.tips?.length) {
                  <details class="tips">
                    <summary>{{ 'ins_tips' | t }} ({{ h.tips!.length }})</summary>
                    <ul>
                      @for (tip of h.tips; track tip.source + $index) {
                        <li>
                          {{ tip.text | tx }}
                          <a [href]="tip.source" target="_blank" rel="noopener noreferrer">{{ 'ins_source' | t }}</a>
                        </li>
                      }
                    </ul>
                    <div class="muted small">{{ 'ins_tips_note' | t }}</div>
                  </details>
                }
              </div>
            </div>
          } @empty {
            <div class="muted">✓ {{ 'ins_no_hints' | t }}</div>
          }
        }
      </div>

      <div class="card stack">
        <div class="row">
          <h2>{{ 'ins_history' | t }}</h2>
          <span class="spacer"></span>
          @for (r of ranges; track r.key) {
            <button class="small" [class.primary]="range() === r.ms" (click)="setRange(r.ms)">{{ r.key }}</button>
          }
        </div>
        <hc-time-chart [series]="chartSeries()" [view]="view()" />
      </div>

      <div class="card scroll">
        <table class="table">
          <thead>
            <tr>
              <th>{{ 'ins_date' | t }}</th>
              <th class="r">{{ 'ins_starts' | t }}</th>
              <th class="r">{{ 'ins_runtime' | t }}</th>
              <th class="r">{{ 'ins_cycle' | t }}</th>
              <th class="r">{{ 'ins_outdoor' | t }}</th>
              <th class="r">{{ 'ins_flow' | t }}</th>
              <th class="r">{{ 'ins_return' | t }}</th>
              <th class="r">{{ 'ins_spread' | t }}</th>
              <th class="r">{{ 'ins_deviation' | t }}</th>
              <th class="r" [title]="'ins_dhw_interrupts' | t">{{ 'ins_dhw' | t }}</th>
              <th class="r">{{ 'ins_cop' | t }}</th>
              <th class="r">{{ 'ins_coverage' | t }}</th>
            </tr>
          </thead>
          <tbody>
            @for (d of rows(); track d.date) {
              <tr [class.thin]="d.coverage < 0.5">
                <td class="num">{{ day(d.date) }}</td>
                <td class="r num" [title]="d.startsSource === 'counter' ? ('ins_counter' | t) : ''">
                  {{ n(d.starts, 0) }}
                </td>
                <td class="r num">{{ d.runtimeHours === undefined ? '–' : n(d.runtimeHours, 1) + ' h' }}</td>
                <td class="r num">{{ d.avgCycleMinutes === undefined ? '–' : n(d.avgCycleMinutes, 0) + ' min' }}</td>
                <td class="r num">{{ deg(d.outdoor) }}</td>
                <td class="r num">{{ deg(d.flowRunning) }}</td>
                <td class="r num">{{ deg(d.returnRunning) }}</td>
                <td class="r num">{{ d.spread === undefined ? '–' : n(d.spread, 1) + ' K' }}</td>
                <td class="r num">{{ d.deviation === undefined ? '–' : (d.deviation > 0 ? '+' : '') + n(d.deviation, 1) + ' K' }}</td>
                <td class="r num" [title]="d.dhwInterrupts === undefined ? '' : ('ins_dhw_interrupts' | t) + ': ' + d.dhwInterrupts">
                  {{ d.dhwHours === undefined ? '–' : n(d.dhwHours, 1) + ' h' }}
                  @if (d.dhwInterrupts) {
                    <span class="muted">({{ d.dhwInterrupts }}×)</span>
                  }
                </td>
                <td class="r num">{{ n(d.cop, 2) }}</td>
                <td class="r num muted">{{ n(d.coverage * 100, 0) }} %</td>
              </tr>
            }
          </tbody>
        </table>
        <div class="muted small note">{{ 'ins_running_note' | t }}</div>
      </div>
    </div>
  `,
  styles: `
    .scroll { overflow: auto; }
    .small { font-size: 12px; }
    .note { padding-top: 8px; }
    tr.thin td { opacity: 0.55; }
    .tips { margin-top: 6px; }
    .tips summary { cursor: pointer; font-size: 13px; }
    .tips ul { margin: 6px 0; padding-left: 18px; }
    .tips li { margin-bottom: 4px; }
    .tips a { margin-left: 4px; font-size: 12px; }
  `,
})
export class InsightsPage {
  private api = inject(Api);
  private i18n = inject(I18n);
  private toasts = inject(Toasts);
  protected readonly ranges = ranges;

  protected days = signal(14);
  protected insights = signal<Insights | null>(null);
  protected range = signal(ranges[0].ms);
  private series = signal<Series[]>([]);
  private until = signal(Date.now());

  protected rows = computed(() => [...(this.insights()?.days ?? [])].reverse());
  protected hasData = computed(() => (this.insights()?.days ?? []).some((d) => d.coverage > 0));
  protected chartSeries = computed<ChartSeries[]>(() =>
    historyPoints.map((p) => {
      const s = this.series().find((x) => x.pointId === p.id);
      return {
        id: p.id,
        label: this.i18n.text(p.label),
        color: p.color,
        unit: '°C',
        visible: true,
        points: s ? s.t.map((t, i) => ({ x: t, y: Math.round(s.v[i] * 10) / 10 })) : [],
      };
    }),
  );
  protected view = computed<ChartView>(() => ({ from: this.until() - this.range(), to: this.until(), interpolate: true }));

  constructor() {
    this.load();
    this.loadHistory();
  }

  private async load() {
    try {
      this.insights.set(await this.api.insights(this.days()));
    } catch (e) {
      this.toasts.error(e);
    }
  }

  private async loadHistory() {
    const to = Date.now();
    this.until.set(to);
    try {
      this.series.set(await this.api.history(historyPoints.map((p) => p.ref), to - this.range(), to));
    } catch (e) {
      this.toasts.error(e);
    }
  }

  protected setDays(d: number) {
    this.days.set(d);
    this.load();
  }
  protected setRange(ms: number) {
    this.range.set(ms);
    this.loadHistory();
  }
  protected val(e: Event) {
    return (e.target as HTMLSelectElement).value;
  }
  protected n(v: number | undefined, d: number) {
    return v === undefined ? '–' : fmtNumber(v, this.i18n.locale(), d);
  }
  protected deg(v: number | undefined) {
    return v === undefined ? '–' : `${fmtNumber(v, this.i18n.locale(), 1)} °C`;
  }
  protected day(date: string) {
    return new Date(date + 'T12:00:00').toLocaleDateString(this.i18n.locale(), { weekday: 'short', day: '2-digit', month: '2-digit' });
  }
}
