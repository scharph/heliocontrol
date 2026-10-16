import { ChangeDetectionStrategy, Component, computed, effect, inject, input, signal } from '@angular/core';
import { Router } from '@angular/router';
import { Api } from '../core/api';
import { I18n, TPipe, TxPipe } from '../core/i18n';
import { HeatCurve, Item, Panel } from '../core/models';
import { State } from '../core/state';
import { Toasts } from '../core/toast';
import { CurveParams, HeatCurveChart } from '../ui/heat-curve';
import { ItemCard } from '../ui/item-card';

type CurveKey = keyof CurveParams;
const curveKeys: CurveKey[] = ['limit', 'atLimit', 'at0', 'atMinus15', 'boost', 'reduction'];

@Component({
  selector: 'hc-overview',
  imports: [TPipe, TxPipe, ItemCard, HeatCurveChart],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="stack">
      @if (!state.online()) {
        <div class="banner warn">{{ 'offline_hint' | t }}</div>
      }
      @if (fault()) {
        <div class="banner err">
          <strong>⚠ {{ 'fault_active' | t }}</strong>
          <span class="spacer"></span>
          <button class="primary" (click)="resetFault()" [disabled]="!state.canWrite()">{{ 'fault_reset' | t }}</button>
        </div>
      }

      @if (state.overrides().length) {
        <div class="card tmp-list">
          <h3>⏱ {{ 'tmp_title' | t }}</h3>
          @for (o of state.overrides(); track o.sp) {
            <div class="row">
              <strong>{{ spLabel(o.sp) }}</strong>
              <span class="num">{{ o.value }} → {{ o.original }}</span>
              <span class="muted">{{ 'tmp_until' | t }} {{ hhmm(o.until) }}{{ o.source ? ' · ' + o.source : '' }}</span>
              <span class="spacer"></span>
              <button class="small" (click)="endTemporary(o.sp)" [disabled]="!state.canWrite()">{{ 'tmp_end' | t }}</button>
            </div>
          }
          <div class="muted small">{{ 'tmp_hint' | t }}</div>
        </div>
      }

      <div class="tabs">
        @for (p of state.panels(); track p.id) {
          <button [class.active]="p.id === current()?.id" (click)="select(p)">
            {{ p.title | tx }}
            @if (p.subtitle) {
              <span class="muted sub">· {{ p.subtitle | tx }}</span>
            }
          </button>
        }
      </div>

      @if (current(); as p) {
        @if (p.heatCurve; as hc) {
          <div class="card curve">
            <div class="row">
              <h2>{{ 'heat_curve' | t }}</h2>
              <span class="spacer"></span>
              @if (curveDirty()) {
                <span class="badge warn">{{ 'changed' | t }}</span>
                <button class="small" (click)="resetCurve()">{{ 'reset' | t }}</button>
                <button class="small primary" (click)="applyCurve(hc)" [disabled]="!state.canWrite() || saving()">
                  {{ 'heat_curve_save' | t }}
                </button>
              }
            </div>
            <div class="curve-body">
              <hc-heat-curve [params]="curveDraft()" [outdoor]="state.mp(0)" [target]="curveDirty() ? undefined : returnTarget()" />
              <div class="params">
                @for (k of curveKeys; track k) {
                  @if (curveItem(hc, k); as it) {
                    <label class="field">
                      {{ it.label | tx }}
                      <div class="row nowrap">
                        <input
                          type="number"
                          class="num"
                          [class.changed]="curveDraft()[k] !== curveValues()[k]"
                          [min]="it.min ?? null"
                          [max]="it.max ?? null"
                          [step]="it.step ?? 1"
                          [value]="curveDraft()[k]"
                          (input)="setCurve(k, $event)"
                          [disabled]="!state.canWrite()"
                        />
                        <span class="muted">{{ it.unit }}</span>
                      </div>
                    </label>
                  }
                }
              </div>
            </div>
          </div>
        }

        <div class="grid">
          @for (it of items(); track it.key) {
            <hc-item [item]="it" />
          }
        </div>
      }
    </div>
  `,
  styles: `
    .sub { font-size: 12px; }
    .small { font-size: 12px; }
    .tmp-list { display: flex; flex-direction: column; gap: 6px; border-color: var(--warn); }
    .curve-body { display: grid; grid-template-columns: minmax(0, 1fr) 260px; gap: 16px; align-items: start; margin-top: 8px; }
    .params { display: grid; grid-template-columns: 1fr 1fr; gap: 10px; }
    .params input { width: 90px; }
    .params input.changed { border-color: var(--warn); background: var(--warn-soft); }
    .nowrap { flex-wrap: nowrap; }
    @media (max-width: 900px) { .curve-body { grid-template-columns: 1fr; } }
  `,
})
export class OverviewPage {
  /** Route parameter: selected page id. */
  readonly panel = input<string>();
  protected state = inject(State);
  private api = inject(Api);
  private toasts = inject(Toasts);
  private router = inject(Router);
  private i18n = inject(I18n);
  protected readonly curveKeys = curveKeys;
  protected saving = signal(false);

  protected current = computed<Panel | undefined>(() => {
    const ps = this.state.panels();
    return ps.find((p) => p.id === this.panel()) ?? ps[0];
  });

  /** Items shown as cards (the heating curve parameters are edited in the chart card). */
  protected items = computed(() => {
    const p = this.current();
    if (!p) return [];
    const curveIds = new Set(p.heatCurve ? curveKeys.map((k) => p.heatCurve![k]) : []);
    return p.items.filter((it) => !(it.kind === 'sp' && curveIds.has(it.id)));
  });

  protected fault = computed(() => !!this.state.mp(31));
  /** Measured target of the shown circuit (main: MP 57, mixer circuits: their own target). */
  protected returnTarget = computed(() => {
    const it = this.current()?.items.find((i) => i.kind === "mp" && (i.key === "return_target" || i.key.endsWith("_target")));
    return it ? this.state.mp(it.id) : undefined;
  });

  protected curveValues = computed<CurveParams>(() => {
    const hc = this.current()?.heatCurve;
    const v = (id: number | undefined, d: number) => (id === undefined ? d : (this.state.sp(id) ?? d));
    return {
      limit: v(hc?.limit, 18),
      atLimit: v(hc?.atLimit, 24),
      at0: v(hc?.at0, 30),
      atMinus15: v(hc?.atMinus15, 38),
      boost: v(hc?.boost, 2),
      reduction: v(hc?.reduction, -2),
    };
  });
  protected curveDraft = signal<CurveParams>({ limit: 18, atLimit: 24, at0: 30, atMinus15: 38, boost: 2, reduction: -2 });
  private touched = signal(false);
  protected curveDirty = computed(() => {
    const d = this.curveDraft();
    const v = this.curveValues();
    return curveKeys.some((k) => d[k] !== v[k]);
  });

  constructor() {
    // follow device values until the user starts editing
    effect(() => {
      const v = this.curveValues();
      if (!this.touched()) this.curveDraft.set(v);
    });
  }

  protected select(p: Panel) {
    this.touched.set(false);
    this.router.navigate(['/overview', p.id]);
  }

  protected curveItem(hc: HeatCurve, k: CurveKey): Item | undefined {
    return this.current()?.items.find((it) => it.kind === 'sp' && it.id === hc[k]);
  }

  protected setCurve(k: CurveKey, e: Event) {
    const v = (e.target as HTMLInputElement).valueAsNumber;
    if (Number.isNaN(v)) return;
    this.touched.set(true);
    this.curveDraft.update((d) => ({ ...d, [k]: v }));
  }

  protected resetCurve() {
    this.touched.set(false);
    this.curveDraft.set(this.curveValues());
  }

  protected async applyCurve(hc: HeatCurve) {
    const d = this.curveDraft();
    const v = this.curveValues();
    this.saving.set(true);
    let ok = 0;
    try {
      for (const k of curveKeys) {
        if (d[k] === v[k]) continue;
        const r = await this.api.writeSP(hc[k], d[k]);
        this.state.setSP(hc[k], r.value);
        ok++;
      }
      this.toasts.ok(`${this.i18n.t('heat_curve')}: ${ok} ${this.i18n.t('saved')}`);
      this.touched.set(false);
    } catch (e) {
      this.toasts.error(e);
    } finally {
      this.saving.set(false);
    }
  }

  /** Label of a setup point from any page. */
  protected spLabel(sp: number): string {
    for (const p of this.state.panels()) {
      const it = p.items.find((i) => i.kind === 'sp' && i.id === sp);
      if (it) return this.i18n.text(it.label);
    }
    return `SP ${sp}`;
  }

  protected hhmm(iso: string) {
    return new Date(iso).toLocaleTimeString(this.i18n.locale(), { hour: '2-digit', minute: '2-digit' });
  }

  protected async endTemporary(sp: number) {
    try {
      await this.api.cancelTemporary(sp);
    } catch (e) {
      this.toasts.error(e);
    }
  }

  protected async resetFault() {
    try {
      await this.api.resetFault();
      this.toasts.ok(this.i18n.t('fault_reset'));
    } catch (e) {
      this.toasts.error(e);
    }
  }
}
