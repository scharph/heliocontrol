import {
  AfterViewInit,
  ChangeDetectionStrategy,
  Component,
  ElementRef,
  OnDestroy,
  effect,
  inject,
  input,
  output,
  untracked,
  viewChild,
} from '@angular/core';
import {
  Chart,
  ChartDataset,
  Legend,
  LineController,
  LineElement,
  LinearScale,
  Plugin,
  PointElement,
  Tooltip,
} from 'chart.js';
import { I18n } from '../core/i18n';

Chart.register(LineController, LineElement, PointElement, LinearScale, Tooltip, Legend);

export interface ChartSeries {
  id: number;
  label: string;
  color: string;
  unit: string;
  visible: boolean;
  points: { x: number; y: number }[];
}

export interface ChartView {
  from?: number; // ms
  to?: number;
  yMin?: number;
  yMax?: number;
  relativeTo?: number; // show time relative to this instant
  interpolate: boolean;
  cursorA?: number;
  cursorB?: number;
}

/** Line chart over time (x = unix ms) with two cursors, drawn by a small plugin. */
@Component({
  selector: 'hc-time-chart',
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `<div class="wrap"><canvas #canvas></canvas></div>`,
  styles: `
    .wrap { position: relative; height: 420px; }
    @media (max-width: 720px) { .wrap { height: 300px; } }
  `,
})
export class TimeChart implements AfterViewInit, OnDestroy {
  readonly series = input.required<ChartSeries[]>();
  readonly view = input.required<ChartView>();
  /** Emitted on click with the clicked time; shift = cursor B. */
  readonly pick = output<{ t: number; shift: boolean }>();

  private canvas = viewChild.required<ElementRef<HTMLCanvasElement>>('canvas');
  private i18n = inject(I18n);
  private chart?: Chart<'line', { x: number; y: number }[]>;

  constructor() {
    effect(() => {
      const s = this.series();
      const v = this.view();
      this.i18n.lang();
      untracked(() => this.render(s, v));
    });
  }

  ngAfterViewInit() {
    this.render(this.series(), this.view());
  }

  ngOnDestroy() {
    this.chart?.destroy();
  }

  /** Append live points without rebuilding the chart. */
  append(id: number, x: number, y: number) {
    const ds = this.chart?.data.datasets.find((d) => (d as { pid?: number }).pid === id);
    if (!ds) return;
    ds.data.push({ x, y });
    this.chart?.update('none');
  }

  /** span: visible time range in ms (decides whether dates are shown). */
  private fmtTick(ms: number, v: ChartView, span: number): string {
    if (v.relativeTo !== undefined) {
      const s = Math.round((ms - v.relativeTo) / 1000);
      const h = Math.floor(s / 3600);
      const m = Math.floor((s % 3600) / 60);
      return `${h}:${String(m).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`;
    }
    const d = new Date(ms);
    return span > 36 * 3600_000
      ? d.toLocaleString(this.i18n.locale(), { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
      : d.toLocaleTimeString(this.i18n.locale(), { hour: '2-digit', minute: '2-digit' });
  }

  private render(series: ChartSeries[], v: ChartView) {
    const el = this.canvas()?.nativeElement;
    if (!el) return;
    const css = getComputedStyle(document.documentElement);
    const grid = css.getPropertyValue('--border').trim() || '#ddd';
    const text = css.getPropertyValue('--muted').trim() || '#666';
    const datasets: ChartDataset<'line', { x: number; y: number }[]>[] = series.map((s) => ({
      label: s.unit ? `${s.label} [${s.unit}]` : s.label,
      data: s.points.slice(),
      borderColor: s.color,
      backgroundColor: s.color,
      hidden: !s.visible,
      borderWidth: 1.5,
      pointRadius: v.interpolate ? 0 : 1.5,
      showLine: v.interpolate,
      stepped: false,
      tension: 0,
      pid: s.id,
    })) as ChartDataset<'line', { x: number; y: number }[]>[];

    const cursors: Plugin<'line'> = {
      id: 'cursors',
      afterDatasetsDraw: (chart) => {
        const { ctx, chartArea, scales } = chart;
        const draw = (t: number | undefined, color: string, label: string) => {
          if (t === undefined) return;
          const x = scales['x'].getPixelForValue(t);
          if (x < chartArea.left || x > chartArea.right) return;
          ctx.save();
          ctx.strokeStyle = color;
          ctx.setLineDash([4, 3]);
          ctx.beginPath();
          ctx.moveTo(x, chartArea.top);
          ctx.lineTo(x, chartArea.bottom);
          ctx.stroke();
          ctx.fillStyle = color;
          ctx.font = '12px system-ui';
          ctx.fillText(label, x + 4, chartArea.top + 12);
          ctx.restore();
        };
        const view = this.view();
        draw(view.cursorA, '#c2410c', 'A');
        draw(view.cursorB, '#2563eb', 'B');
      },
    };

    if (this.chart) {
      this.chart.data.datasets = datasets;
      const x = this.chart.options.scales!['x']!;
      const y = this.chart.options.scales!['y']!;
      x.min = v.from;
      x.max = v.to;
      y.min = v.yMin;
      y.max = v.yMax;
      this.chart.update('none');
      return;
    }
    this.chart = new Chart<'line', { x: number; y: number }[]>(el, {
      type: 'line',
      data: { datasets },
      plugins: [cursors],
      options: {
        animation: false,
        responsive: true,
        maintainAspectRatio: false,
        parsing: false,
        normalized: true,
        interaction: { mode: 'nearest', axis: 'x', intersect: false },
        scales: {
          x: {
            type: 'linear',
            min: v.from,
            max: v.to,
            grid: { color: grid },
            ticks: {
              color: text,
              maxRotation: 0,
              autoSkip: true,
              callback: (val, _i, ticks) => {
                const span = ticks.length > 1 ? Number(ticks[ticks.length - 1].value) - Number(ticks[0].value) : 0;
                return this.fmtTick(Number(val), this.view(), span);
              },
            },
          },
          y: { min: v.yMin, max: v.yMax, grid: { color: grid }, ticks: { color: text } },
        },
        plugins: {
          legend: { display: false },
          tooltip: {
            callbacks: {
              title: (items) => (items.length ? this.fmtTick(items[0].parsed.x ?? 0, this.view(), Infinity) : ''),
            },
          },
        },
        onClick: (evt, _els, chart) => {
          const x = evt.x;
          if (x === null || x === undefined) return;
          const t = chart.scales['x'].getValueForPixel(x);
          const native = evt.native as MouseEvent | null;
          if (t !== undefined) this.pick.emit({ t, shift: !!native?.shiftKey });
        },
      },
    });
  }
}
