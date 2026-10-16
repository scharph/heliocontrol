import { ChangeDetectionStrategy, Component, computed, inject, input, linkedSignal, signal } from '@angular/core';
import { Api } from '../core/api';
import { clamp, decimals, fmtDuration, fmtNumber } from '../core/format';
import { I18n, TPipe } from '../core/i18n';
import { Item } from '../core/models';
import { State } from '../core/state';
import { Toasts } from '../core/toast';

/** One value of a page: read-only display or an editor for writable setup points. */
@Component({
  selector: 'hc-item',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="item" [class.bool-on]="isBool() && value() === 1" [class.fault]="isFault() && !!value()">
      <div class="label" [title]="titleText()">{{ label() }}</div>
      @switch (mode()) {
        @case ('select') {
          <select [value]="value() ?? ''" (change)="selectChanged($event)" [disabled]="!state.canWrite() || saving()">
            @if (value() === undefined) {
              <option value="">–</option>
            }
            @for (o of options(); track o.value) {
              <option [value]="o.value" [selected]="o.value === value()" [disabled]="o.disabled">{{ o.label }}</option>
            }
          </select>
        }
        @case ('number') {
          <div class="row edit">
            <input
              type="number"
              class="num"
              [min]="item().min ?? null"
              [max]="item().max ?? null"
              [step]="item().step ?? 1"
              [value]="draft() ?? ''"
              (input)="draft.set(num($event))"
              (keydown.enter)="save()"
              [disabled]="!state.canWrite() || saving()"
            />
            <span class="unit">{{ item().unit }}</span>
            @if (dirty()) {
              <button class="small primary" (click)="save()" [disabled]="saving()">{{ 'save' | t }}</button>
              <button class="small" (click)="draft.set(value())">✕</button>
            }
          </div>
          @if (item().min !== undefined && item().max !== undefined) {
            <div class="range muted">{{ item().min }} … {{ item().max }} {{ item().unit }}</div>
          }
        }
        @default {
          <div class="value num">
            @if (isBool()) {
              <span class="dot" [class.on]="value() === 1"></span>{{ value() === undefined ? '–' : value() ? ('on' | t) : ('off' | t) }}
            } @else if (isFault()) {
              {{ value() === undefined ? '–' : value() ? ('fault_active' | t) : '✓' }}
            } @else {
              {{ text() }}<span class="unit">{{ item().unit && item().display === 'value' ? ' ' + item().unit : '' }}</span>
            }
          </div>
        }
      }
      @if (item().writable) {
        @if (temporary(); as o) {
          <div class="tmp row">
            <span class="badge warn" [title]="'tmp_hint' | t">⏱ {{ 'tmp_until' | t }} {{ hhmm(o.until) }} · {{ 'tmp_then' | t }} {{ show(o.original) }}</span>
            <button class="small" (click)="endTemporary()" [disabled]="!state.canWrite() || saving()">{{ 'tmp_end' | t }}</button>
          </div>
        } @else if (state.canWrite()) {
          @if (tmpOpen()) {
            <div class="tmp row">
              @if (item().display === 'enum') {
                <select (change)="tmpValue.set(+val($event))">
                  @for (o of options(); track o.value) {
                    @if (!o.disabled) {
                      <option [value]="o.value" [selected]="o.value === tmpValue()">{{ o.label }}</option>
                    }
                  }
                </select>
              } @else {
                <input type="number" class="num short" [step]="item().step ?? 1" [value]="tmpValue() ?? ''" (input)="tmpValue.set(num($event))" />
              }
              <span class="muted">{{ 'tmp_for' | t }}</span>
              <input type="number" class="num short" min="1" max="1440" [value]="tmpMinutes()" (input)="tmpMinutes.set(num($event) ?? 60)" />
              <span class="muted">min</span>
              <button class="small primary" (click)="applyTemporary()" [disabled]="saving() || tmpValue() === undefined">{{ 'tmp_apply' | t }}</button>
              <button class="small" (click)="tmpOpen.set(false)">✕</button>
            </div>
          } @else {
            <button class="small link" (click)="openTemporary()">⏱ {{ 'tmp_set' | t }}</button>
          }
        }
      }
    </div>
  `,
  styles: `
    .item {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--radius);
      padding: 10px 12px;
      display: flex;
      flex-direction: column;
      gap: 4px;
      min-height: 72px;
    }
    .label { font-size: 12px; color: var(--muted); white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
    .value { font-size: 20px; font-weight: 600; display: flex; align-items: center; gap: 8px; }
    .unit { font-size: 13px; font-weight: 400; color: var(--muted); }
    .dot { width: 10px; height: 10px; background: var(--border); }
    .dot.on { background: var(--ok); box-shadow: 0 0 0 3px var(--ok-soft); }
    .fault { border-color: var(--err); .value { color: var(--err); } }
    .edit input { width: 96px; font-size: 16px; font-weight: 600; }
    .range { font-size: 11px; }
    select { font-size: 15px; }
    .tmp { gap: 4px; font-size: 12px; margin-top: 2px; }
    .tmp input.short { width: 64px; padding: 2px 6px; }
    .tmp select { font-size: 12px; padding: 2px 4px; }
    .link { border: none; background: none; padding: 0; color: var(--muted); align-self: flex-start; }
    .link:hover { color: var(--accent); }
  `,
})
export class ItemCard {
  readonly item = input.required<Item>();
  protected state = inject(State);
  private i18n = inject(I18n);
  private api = inject(Api);
  private toasts = inject(Toasts);

  protected saving = signal(false);
  protected value = computed(() => this.state.value(this.item()));
  protected draft = linkedSignal(() => this.value());
  protected dirty = computed(() => this.draft() !== undefined && this.draft() !== this.value());
  protected label = computed(() => this.i18n.text(this.item().label));
  protected titleText = computed(() => `${this.label()} (${this.item().kind.toUpperCase()} ${this.item().id})`);
  protected isBool = computed(() => this.item().display === 'bool');
  protected isFault = computed(() => this.item().display === 'fault');
  protected mode = computed(() => {
    const it = this.item();
    if (!it.writable) return 'show';
    return it.display === 'enum' ? 'select' : 'number';
  });
  protected options = computed(() => {
    const it = this.item();
    const off = it.offset ?? 0;
    return (it.options ?? []).map((o, i) => ({
      value: i + off,
      label: this.i18n.text(o),
      disabled: (it.notSelectable ?? []).includes(i + off),
    }));
  });
  protected text = computed(() => {
    const it = this.item();
    const v = this.value();
    if (v === undefined) return '–';
    switch (it.display) {
      case 'enum': {
        const o = it.options?.[v - (it.offset ?? 0)];
        return o ? this.i18n.text(o) : String(v);
      }
      case 'duration':
        return fmtDuration(v);
      case 'ratio':
        return fmtNumber(v, this.i18n.locale(), 2);
      default:
        return fmtNumber(v, this.i18n.locale(), Math.max(1, decimals(it)));
    }
  });

  protected temporary = computed(() => (this.item().kind === 'sp' ? this.state.override(this.item().id) : undefined));
  protected tmpOpen = signal(false);
  protected tmpValue = signal<number | undefined>(undefined);
  protected tmpMinutes = signal(60);

  protected val(e: Event): string {
    return (e.target as HTMLInputElement).value;
  }
  protected hhmm(iso: string) {
    return new Date(iso).toLocaleTimeString(this.i18n.locale(), { hour: '2-digit', minute: '2-digit' });
  }
  /** A value as the user sees it (option label for modes). */
  protected show(v: number): string {
    const it = this.item();
    if (it.display === 'enum') {
      const o = it.options?.[v - (it.offset ?? 0)];
      return o ? this.i18n.text(o) : String(v);
    }
    return `${fmtNumber(v, this.i18n.locale(), 2)}${it.unit ? ' ' + it.unit : ''}`;
  }

  protected openTemporary() {
    this.tmpValue.set(this.value());
    this.tmpOpen.set(true);
  }

  protected async applyTemporary() {
    const v = this.tmpValue();
    if (v === undefined) return;
    const it = this.item();
    this.saving.set(true);
    try {
      const o = await this.api.setTemporary(it.id, clamp(v, it.min, it.max), this.tmpMinutes());
      this.state.setSP(it.id, o.value);
      this.toasts.ok(`${this.label()}: ${this.show(o.value)} ${this.i18n.t('tmp_until')} ${this.hhmm(o.until)}`);
      this.tmpOpen.set(false);
    } catch (e) {
      this.toasts.error(e);
    } finally {
      this.saving.set(false);
    }
  }

  protected async endTemporary() {
    this.saving.set(true);
    try {
      await this.api.cancelTemporary(this.item().id);
    } catch (e) {
      this.toasts.error(e);
    } finally {
      this.saving.set(false);
    }
  }

  protected num(e: Event): number | undefined {
    const v = (e.target as HTMLInputElement).valueAsNumber;
    return Number.isNaN(v) ? undefined : v;
  }

  protected selectChanged(e: Event) {
    const el = e.target as HTMLSelectElement;
    // on failure the element must show the real value again
    this.write(Number(el.value), () => (el.value = String(this.value() ?? "")));
  }

  protected save() {
    const v = this.draft();
    if (v === undefined || !this.dirty()) return;
    const it = this.item();
    this.write(clamp(v, it.min, it.max));
  }

  private async write(v: number, onError?: () => void) {
    const it = this.item();
    this.saving.set(true);
    try {
      const r = await this.api.writeSP(it.id, v);
      this.state.setSP(it.id, r.value);
      this.toasts.ok(`${this.label()}: ${r.value}${it.unit ? ' ' + it.unit : ''}`);
    } catch (e) {
      this.toasts.error(e);
      this.draft.set(this.value());
      onError?.();
    } finally {
      this.saving.set(false);
    }
  }
}
