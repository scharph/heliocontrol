import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { Api } from '../core/api';
import { fmtNumber } from '../core/format';
import { I18n, TPipe } from '../core/i18n';
import { Point } from '../core/models';
import { State } from '../core/state';
import { Toasts } from '../core/toast';

/** All measuring and setup points of the controller, with names from the controller itself. */
@Component({
  selector: 'hc-points',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="stack">
      <div class="row">
        <h1>{{ 'nav_points' | t }}</h1>
        <span class="spacer"></span>
        <button (click)="read()" [disabled]="!state.online() || busy()">⟳ {{ 'points_read' | t }}</button>
      </div>
      <div class="tabs">
        <button [class.active]="kind() === 'mp'" (click)="setKind('mp')">{{ 'points_mp' | t }} ({{ mp().length }})</button>
        <button [class.active]="kind() === 'sp'" (click)="setKind('sp')">{{ 'points_sp' | t }} ({{ sp().length }})</button>
      </div>
      <div class="row">
        <input type="search" [placeholder]="'rec_search' | t" (input)="filter.set(val($event))" style="width: 260px" />
        <label class="check"><input type="checkbox" [checked]="showUnused()" (change)="showUnused.set(!showUnused())" />{{ 'points_show_unused' | t }}</label>
      </div>
      @if (editable()) {
        <div class="banner warn">{{ 'points_expert_hint' | t }}</div>
      }
      @if (!list().length) {
        <div class="card muted">{{ 'points_empty' | t }}</div>
      } @else {
        <div class="card scroll">
          <table class="table">
            <thead>
              <tr>
                <th>{{ kind().toUpperCase() }}</th>
                <th>{{ 'points_name' | t }}</th>
                <th class="r">{{ 'points_value' | t }}</th>
                <th class="r">{{ 'points_limits' | t }}</th>
                <th>{{ 'points_flags' | t }}</th>
              </tr>
            </thead>
            <tbody>
              @for (p of rows(); track p.id) {
                <tr [class.unused]="!p.used">
                  <td class="mono muted">{{ p.id }}</td>
                  <td>{{ p.name }}</td>
                  <td class="r num">
                    @if (editable() && p.writable) {
                      <input
                        type="number"
                        class="num"
                        [value]="current(p)"
                        [min]="p.min"
                        [max]="p.max"
                        (change)="write(p, $event)"
                        [disabled]="busy()"
                        style="width: 100px; text-align: right"
                      />
                    } @else {
                      {{ fmt(current(p)) }}
                    }
                  </td>
                  <td class="r num muted">{{ p.min }} … {{ p.max }}</td>
                  <td class="small">
                    @if (p.used) {
                      <span class="badge ok">{{ 'points_used' | t }}</span>
                    }
                    @if (p.writable) {
                      <span class="badge">{{ 'points_writable' | t }}</span>
                    }
                  </td>
                </tr>
              }
            </tbody>
          </table>
        </div>
      }
    </div>
  `,
  styles: `
    .scroll { max-height: calc(100vh - 260px); overflow: auto; padding: 0; }
    tr.unused td { opacity: 0.55; }
    .small { font-size: 12px; }
  `,
})
export class PointsPage {
  protected state = inject(State);
  private api = inject(Api);
  private toasts = inject(Toasts);
  private i18n = inject(I18n);

  protected kind = signal<'mp' | 'sp'>('mp');
  protected mp = signal<Point[]>([]);
  protected sp = signal<Point[]>([]);
  protected filter = signal('');
  protected showUnused = signal(false);
  protected busy = signal(false);

  protected list = computed(() => (this.kind() === 'mp' ? this.mp() : this.sp()));
  protected rows = computed(() => {
    const f = this.filter().toLowerCase();
    return this.list().filter((p) => (this.showUnused() || p.used) && (!f || p.name.toLowerCase().includes(f) || String(p.id) === f));
  });
  protected editable = computed(() => this.kind() === 'sp' && this.state.canWrite() && !!this.state.snapshot()?.expertMode);

  constructor() {
    this.api.points('mp').then((p) => this.mp.set(p), () => undefined);
    this.api.points('sp').then((p) => this.sp.set(p), () => undefined);
  }

  protected val(e: Event) {
    return (e.target as HTMLInputElement).value;
  }
  protected fmt(v: number | undefined) {
    return fmtNumber(v, this.i18n.locale(), 3);
  }

  /** Live value if polled, else the value read with the point list. */
  protected current(p: Point): number {
    const live = p.kind === 'mp' ? this.state.mp(p.id) : this.state.sp(p.id);
    return live ?? p.value;
  }

  protected setKind(k: 'mp' | 'sp') {
    this.kind.set(k);
  }

  protected async read() {
    this.busy.set(true);
    try {
      const pts = await this.api.points(this.kind(), true);
      (this.kind() === 'mp' ? this.mp : this.sp).set(pts);
    } catch (e) {
      this.toasts.error(e);
    } finally {
      this.busy.set(false);
    }
  }

  protected async write(p: Point, e: Event) {
    const el = e.target as HTMLInputElement;
    const v = el.valueAsNumber;
    if (Number.isNaN(v)) return;
    if (!confirm(`SP ${p.id} ${p.name}: ${this.fmt(this.current(p))} → ${v}?`)) {
      el.value = String(this.current(p));
      return;
    }
    this.busy.set(true);
    try {
      const r = await this.api.writeSP(p.id, v);
      this.state.setSP(p.id, r.value);
      this.sp.update((l) => l.map((x) => (x.id === p.id ? { ...x, value: r.value } : x)));
      this.toasts.ok(`SP ${p.id}: ${r.value}`);
    } catch (err) {
      this.toasts.error(err);
      el.value = String(this.current(p));
    } finally {
      this.busy.set(false);
    }
  }
}
