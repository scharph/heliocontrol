import { ChangeDetectionStrategy, Component, computed, inject, input, signal } from '@angular/core';
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

const W = 560;
const H = 300;
const PAD = { l: 44, r: 16, t: 16, b: 34 };
const X0 = 25; // left edge: warm
const X1 = -20; // right edge: cold
const Y0 = 10;
const Y1 = 65;

@Component({
  selector: 'hc-heat-curve',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <svg [attr.viewBox]="'0 0 ' + W + ' ' + H" (mousemove)="hover($event)" (mouseleave)="cursor.set(null)" role="img" [attr.aria-label]="'heat_curve' | t">
      <!-- grid -->
      @for (x of xTicks; track x) {
        <line class="grid" [attr.x1]="sx(x)" [attr.x2]="sx(x)" [attr.y1]="PAD.t" [attr.y2]="H - PAD.b" />
        <text class="tick" [attr.x]="sx(x)" [attr.y]="H - PAD.b + 16" text-anchor="middle">{{ x }}</text>
      }
      @for (y of yTicks; track y) {
        <line class="grid" [attr.x1]="PAD.l" [attr.x2]="W - PAD.r" [attr.y1]="sy(y)" [attr.y2]="sy(y)" />
        <text class="tick" [attr.x]="PAD.l - 6" [attr.y]="sy(y) + 4" text-anchor="end">{{ y }}</text>
      }
      <text class="axis" [attr.x]="W - PAD.r" [attr.y]="H - 4" text-anchor="end">{{ 'heat_curve_outdoor' | t }} °C</text>
      <text class="axis" x="4" y="12">{{ 'heat_curve_target' | t }} °C</text>

      <polyline class="boost" [attr.points]="boostLine()" />
      <polyline class="reduced" [attr.points]="reducedLine()" />
      <polyline class="normal" [attr.points]="normalLine()" />
      @for (p of knots(); track $index) {
        <circle class="knot" [attr.cx]="p[0]" [attr.cy]="p[1]" r="4" />
      }

      @if (now(); as n) {
        <line class="now" [attr.x1]="sx(n.at)" [attr.x2]="sx(n.at)" [attr.y1]="PAD.t" [attr.y2]="H - PAD.b" />
        <circle class="nowdot" [attr.cx]="sx(n.at)" [attr.cy]="sy(n.target)" r="5" />
      }
      @if (cursor(); as c) {
        <line class="cursor" [attr.x1]="sx(c.at)" [attr.x2]="sx(c.at)" [attr.y1]="PAD.t" [attr.y2]="H - PAD.b" />
        <circle class="knot" [attr.cx]="sx(c.at)" [attr.cy]="sy(c.normal)" r="3" />
      }
    </svg>
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
  `,
  styles: `
    :host { display: block; }
    svg { width: 100%; height: auto; display: block; user-select: none; cursor: crosshair; }
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

  private i18n = inject(I18n);
  protected readonly W = W;
  protected readonly H = H;
  protected readonly PAD = PAD;
  protected readonly xTicks = [25, 20, 15, 10, 5, 0, -5, -10, -15, -20];
  protected readonly yTicks = [10, 20, 30, 40, 50, 60];
  protected cursor = signal<{ at: number; normal: number; reduced: number; boost: number } | null>(null);

  protected sx(at: number) {
    return PAD.l + ((X0 - at) / (X0 - X1)) * (W - PAD.l - PAD.r);
  }
  protected sy(v: number) {
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

  protected hover(e: MouseEvent) {
    const svg = e.currentTarget as SVGSVGElement;
    const r = svg.getBoundingClientRect();
    const x = ((e.clientX - r.left) / r.width) * W;
    let at = X0 - ((x - PAD.l) / (W - PAD.l - PAD.r)) * (X0 - X1);
    at = Math.round(Math.max(X1, Math.min(X0, at)) * 2) / 2;
    const p = this.params();
    this.cursor.set({ at, normal: curveTarget(p, at), reduced: curveTarget(p, at, p.reduction), boost: curveTarget(p, at, p.boost) });
  }

  protected fmt(v: number, d: number) {
    return fmtNumber(v, this.i18n.locale(), d);
  }
}
