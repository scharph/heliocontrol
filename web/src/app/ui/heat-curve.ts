import { ChangeDetectionStrategy, Component, computed, inject, input, output, signal } from '@angular/core';
import { fmtNumber } from '../core/format';
import { I18n, TPipe } from '../core/i18n';

export interface CurveParams {
  limit: number; // heating limit (outdoor °C)
  atLimit: number; // return target at the heating limit
  at0: number; // ... at 0 °C
  atMinus15: number; // ... at -15 °C
  boost: number; // K added during boost periods
  reduction: number; // K added during reduced periods (negative)
}

/** Return target for an outdoor temperature (same as catalog.HeatCurveTarget on the server). */
export function curveTarget(p: CurveParams, at: number, shift = 0): number {
  let v: number;
  if (at >= p.limit) v = p.atLimit;
  else if (at >= 0) v = p.at0 + ((p.atLimit - p.at0) * at) / p.limit;
  else if (at >= -15) v = p.at0 + ((p.atMinus15 - p.at0) * -at) / 15;
  else v = p.atMinus15;
  return v + shift;
}

/** Chart geometry: the full chart, and a compact one for dashboard widgets. */
const full = {
  W: 560,
  H: 300,
  PAD: { l: 44, r: 16, t: 16, b: 34 },
  xTicks: [25, 20, 15, 10, 5, 0, -5, -10, -15, -20],
  yTicks: [10, 20, 30, 40, 50, 60],
};
const compact = {
  W: 360,
  H: 190,
  PAD: { l: 26, r: 8, t: 8, b: 20 },
  xTicks: [20, 10, 0, -10, -20],
  yTicks: [20, 40, 60],
};
const X0 = 25; // left edge: warm
const X1 = -20; // right edge: cold
const Y0 = 10;
const Y1 = 65;

@Component({
  selector: 'hc-heat-curve',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <svg
      [class.compact]="compact()"
      [class.dragging]="dragging() !== null"
      [attr.viewBox]="'0 0 ' + g().W + ' ' + g().H"
      (pointermove)="pointerMove($event)"
      (pointerup)="endDrag($event)"
      (pointercancel)="dragging.set(null)"
      (mouseleave)="cursor.set(null)"
      role="img"
      [attr.aria-label]="'heat_curve' | t"
    >
      <!-- grid -->
      @for (x of g().xTicks; track x) {
        <line class="grid" [attr.x1]="sx(x)" [attr.x2]="sx(x)" [attr.y1]="g().PAD.t" [attr.y2]="g().H - g().PAD.b" />
        <text class="tick" [attr.x]="sx(x)" [attr.y]="g().H - g().PAD.b + 16" text-anchor="middle">{{ x }}</text>
      }
      @for (y of g().yTicks; track y) {
        <line class="grid" [attr.x1]="g().PAD.l" [attr.x2]="g().W - g().PAD.r" [attr.y1]="sy(y)" [attr.y2]="sy(y)" />
        <text class="tick" [attr.x]="g().PAD.l - 6" [attr.y]="sy(y) + 4" text-anchor="end">{{ y }}</text>
      }
      @if (!compact()) {
        <text class="axis" [attr.x]="g().W - g().PAD.r" [attr.y]="g().H - 4" text-anchor="end">{{ 'heat_curve_outdoor' | t }} °C</text>
        <text class="axis" x="4" y="12">{{ 'heat_curve_target' | t }} °C</text>
      }

      <polyline class="boost" [attr.points]="boostLine()" />
      <polyline class="reduced" [attr.points]="reducedLine()" />
      <polyline class="normal" [attr.points]="normalLine()" />
      @for (p of knots(); track $index) {
        @if (canDrag()) {
          <circle class="knot draggable" [attr.cx]="p[0]" [attr.cy]="p[1]" r="6" (pointerdown)="startDrag($index, $event)">
            <title>{{ 'heat_curve_drag' | t }}</title>
          </circle>
        } @else {
          <circle class="knot" [attr.cx]="p[0]" [attr.cy]="p[1]" [attr.r]="compact() ? 3 : 4" />
        }
      }

      @if (now(); as n) {
        <line class="now" [attr.x1]="sx(n.at)" [attr.x2]="sx(n.at)" [attr.y1]="g().PAD.t" [attr.y2]="g().H - g().PAD.b" />
        <circle class="nowdot" [attr.cx]="sx(n.at)" [attr.cy]="sy(n.target)" [attr.r]="compact() ? 4 : 5" />
      }
      @if (cursor(); as c) {
        <line class="cursor" [attr.x1]="sx(c.at)" [attr.x2]="sx(c.at)" [attr.y1]="g().PAD.t" [attr.y2]="g().H - g().PAD.b" />
        <circle class="knot" [attr.cx]="sx(c.at)" [attr.cy]="sy(c.normal)" r="3" />
      }
    </svg>
    @if (!compact()) {
      <div class="legend">
        <span><i class="normal"></i>{{ 'heat_curve_normal' | t }}</span>
        <span><i class="reduced"></i>{{ 'heat_curve_reduced' | t }}</span>
        <span><i class="boost"></i>{{ 'heat_curve_boost' | t }}</span>
        <span class="spacer"></span>
        @if (cursor(); as c) {
          <span class="num">
            {{ fmt(c.at, 1) }} °C → <b>{{ fmt(c.normal, 1) }} °C</b>
            <span class="muted">({{ fmt(c.reduced, 1) }} / {{ fmt(c.boost, 1) }})</span>
          </span>
        } @else if (now(); as n) {
          <span class="num">{{ 'heat_curve_now' | t }}: {{ fmt(n.at, 1) }} °C → <b>{{ fmt(n.target, 1) }} °C</b></span>
        }
      </div>
    }
  `,
  styles: `
    :host { display: block; }
    svg { width: 100%; height: auto; display: block; user-select: none; cursor: crosshair; }
    svg.compact { cursor: inherit; }
    svg.dragging { cursor: grabbing; }
    .knot.draggable { cursor: grab; touch-action: none; stroke-width: 2.5; }
    .knot.draggable:hover, svg.dragging .knot.draggable { fill: var(--accent); }
    svg.compact .tick { font-size: 10px; }
    svg.compact polyline.normal { stroke-width: 2; }
    .grid { stroke: var(--border); stroke-width: 1; }
    .tick, .axis { fill: var(--muted); font-size: 11px; }
    polyline { fill: none; stroke-width: 2.5; stroke-linejoin: round; }
    .normal { stroke: var(--text); }
    .reduced { stroke: #3b82f6; stroke-width: 1.5; stroke-dasharray: 5 3; }
    .boost { stroke: #ef4444; stroke-width: 1.5; stroke-dasharray: 5 3; }
    .knot { fill: var(--surface); stroke: var(--text); stroke-width: 2; }
    .now { stroke: var(--accent); stroke-width: 1.5; stroke-dasharray: 3 3; }
    .nowdot { fill: var(--accent); }
    .cursor { stroke: var(--muted); stroke-width: 1; }
    .legend { display: flex; gap: 14px; flex-wrap: wrap; font-size: 12px; align-items: center; margin-top: 6px; }
    .legend i { display: inline-block; width: 18px; height: 0; border-top: 2.5px solid; margin-right: 6px; vertical-align: middle; }
    .legend i.normal { border-color: var(--text); }
    .legend i.reduced { border-color: #3b82f6; border-top-style: dashed; }
    .legend i.boost { border-color: #ef4444; border-top-style: dashed; }
  `,
})
export class HeatCurveChart {
  readonly params = input.required<CurveParams>();
  readonly outdoor = input<number | undefined>(undefined);
  readonly target = input<number | undefined>(undefined);
  /** Small chart without axis titles, legend and hover readout (dashboard widget). */
  readonly compact = input(false);
  /** The knots can be dragged; each move emits the changed parameters. */
  readonly editable = input(false);
  readonly edit = output<Partial<CurveParams>>();

  private i18n = inject(I18n);
  protected g = computed(() => (this.compact() ? compact : full));
  protected canDrag = computed(() => this.editable() && !this.compact());
  /** Index of the dragged knot: 0 heating limit, 1 at 0 °C, 2 at -15 °C. */
  protected dragging = signal<number | null>(null);
  protected cursor = signal<{ at: number; normal: number; reduced: number; boost: number } | null>(null);

  protected sx(at: number) {
    const { W, PAD } = this.g();
    return PAD.l + ((X0 - at) / (X0 - X1)) * (W - PAD.l - PAD.r);
  }
  protected sy(v: number) {
    const { H, PAD } = this.g();
    const c = Math.max(Y0, Math.min(Y1, v));
    return H - PAD.b - ((c - Y0) / (Y1 - Y0)) * (H - PAD.t - PAD.b);
  }

  private line(shift: number) {
    const p = this.params();
    const xs = [X0, Math.min(X0, p.limit), 0, -15, X1];
    return xs.map((x) => `${this.sx(x)},${this.sy(curveTarget(p, x, shift))}`).join(' ');
  }
  protected normalLine = computed(() => this.line(0));
  protected reducedLine = computed(() => this.line(this.params().reduction));
  protected boostLine = computed(() => this.line(this.params().boost));
  protected knots = computed(() => {
    const p = this.params();
    return [
      [this.sx(Math.min(X0, p.limit)), this.sy(p.atLimit)],
      [this.sx(0), this.sy(p.at0)],
      [this.sx(-15), this.sy(p.atMinus15)],
    ];
  });
  protected now = computed(() => {
    const at = this.outdoor();
    if (at === undefined) return null;
    return { at, target: this.target() ?? curveTarget(this.params(), at) };
  });

  /** Pointer position in chart units: outdoor temperature and target. */
  private toChart(e: PointerEvent, svg: SVGSVGElement) {
    const { W, H, PAD } = this.g();
    const r = svg.getBoundingClientRect();
    const x = ((e.clientX - r.left) / r.width) * W;
    const y = ((e.clientY - r.top) / r.height) * H;
    return {
      at: X0 - ((x - PAD.l) / (W - PAD.l - PAD.r)) * (X0 - X1),
      v: Y0 + ((H - PAD.b - y) / (H - PAD.t - PAD.b)) * (Y1 - Y0),
    };
  }

  protected startDrag(i: number, e: PointerEvent) {
    const svg = (e.currentTarget as SVGElement).ownerSVGElement;
    if (!svg) return;
    e.preventDefault();
    svg.setPointerCapture(e.pointerId);
    this.dragging.set(i);
    this.cursor.set(null);
  }

  protected pointerMove(e: PointerEvent) {
    const svg = e.currentTarget as SVGSVGElement;
    const i = this.dragging();
    if (i === null) {
      this.hover(e, svg);
      return;
    }
    const { at, v } = this.toChart(e, svg);
    const target = Math.round(Math.max(Y0, Math.min(Y1, v)) * 2) / 2;
    if (i === 0) this.edit.emit({ limit: Math.round(Math.max(1, Math.min(X0, at))), atLimit: target });
    else if (i === 1) this.edit.emit({ at0: target });
    else this.edit.emit({ atMinus15: target });
  }

  /** The release point counts as the last move (some inputs send no moves in between). */
  protected endDrag(e: PointerEvent) {
    if (this.dragging() === null) return;
    this.pointerMove(e);
    this.dragging.set(null);
  }

  private hover(e: PointerEvent, svg: SVGSVGElement) {
    if (this.compact()) return;
    let at = this.toChart(e, svg).at;
    at = Math.round(Math.max(X1, Math.min(X0, at)) * 2) / 2;
    const p = this.params();
    this.cursor.set({ at, normal: curveTarget(p, at), reduced: curveTarget(p, at, p.reduction), boost: curveTarget(p, at, p.boost) });
  }

  protected fmt(v: number, d: number) {
    return fmtNumber(v, this.i18n.locale(), d);
  }
}
