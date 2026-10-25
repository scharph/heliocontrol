import { ChangeDetectionStrategy, Component, computed, inject, signal } from '@angular/core';
import { Api } from '../core/api';
import { download, fromMinutes, toMinutes } from '../core/format';
import { I18n, Key, TPipe } from '../core/i18n';
import { TimeProgram, TimeProgramEvent } from '../core/models';
import { State } from '../core/state';
import { Toasts } from '../core/toast';

const colors2 = ['#9ca3af', '#22c55e']; // off, on
const colors3 = ['#22c55e', '#3b82f6', '#ef4444']; // normal, reduced, boost

/** The editable fields of an event (day and index stay with the slot). */
const pick = ({ state, begin, end }: TimeProgramEvent) => ({ state, begin, end });

@Component({
  selector: 'hc-time-programs',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="stack">
      <div class="row">
        <h1>{{ 'nav_timeprograms' | t }}</h1>
        <span class="spacer"></span>
        <button (click)="load()" [disabled]="!state.online() || loading()">{{ loading() ? ('loading' | t) : ('load' | t) }}</button>
      </div>
      @if (!programs().length) {
        <div class="card muted">{{ 'tp_load_hint' | t }}</div>
      } @else {
        <div class="tabs">
          @for (p of programs(); track p.id) {
            <button [class.active]="p.id === sel()?.id" (click)="selectProgram(p.id)">
              {{ p.name }}
              @if (dirtyPrograms().has(p.id)) {
                <span class="badge warn">●</span>
              }
            </button>
          }
        </div>

        @if (sel(); as p) {
          <div class="card stack">
            <div class="row">
              <div class="legend">
                @for (s of stateList(); track s.value) {
                  <span><i [style.background]="s.color"></i>{{ s.label }}</span>
                }
              </div>
              <span class="spacer"></span>
              <button class="small" (click)="exportProgram()">⭳ {{ 'tp_export' | t }}</button>
              <label class="btn small">⭱ {{ 'tp_import' | t }}<input type="file" accept=".json" hidden (change)="importProgram($event)" /></label>
            </div>
            <div class="week">
              <div></div>
              <div class="hours">
                @for (h of hours; track h) {
                  <span [style.left.%]="(h / 24) * 100">{{ h }}</span>
                }
              </div>
              @for (d of days(); track d) {
                <button class="dayname" [class.active]="d === day()" (click)="day.set(d)">{{ dayKey(d) | t }}</button>
                <div class="bar" [class.active]="d === day()" (click)="day.set(d)">
                  @for (ev of dayEvents(d); track ev.event) {
                    @if (toMinutes(ev.end) > toMinutes(ev.begin)) {
                      <div
                        class="seg"
                        [style.left.%]="(toMinutes(ev.begin) / 1440) * 100"
                        [style.width.%]="((toMinutes(ev.end) - toMinutes(ev.begin)) / 1440) * 100"
                        [style.background]="color(ev.state)"
                        [title]="ev.begin + '–' + ev.end + ' ' + stateLabel(ev.state)"
                      ></div>
                    }
                  }
                </div>
              }
            </div>
          </div>

          <div class="card stack">
            <div class="row">
              <h2>{{ dayKey(day()) | t }}</h2>
              <span class="spacer"></span>
              <button class="small" (click)="copyDay()">⧉ {{ 'tp_copy_day' | t }}</button>
              @if (clipboard(); as c) {
                <span class="muted small">{{ 'tp_paste' | t }}:</span>
                @for (d of days(); track d) {
                  @if (d !== c.day || c.program !== p.id) {
                    <button class="small" (click)="paste(d)">{{ dayShort(d) }}</button>
                  }
                }
                <button class="small" (click)="pasteAll()">{{ 'rec_window_all' | t }}</button>
                <button class="small" (click)="clipboard.set(null)">✕</button>
              }
            </div>
            <table class="table">
              <thead>
                <tr>
                  <th>{{ 'tp_day' | t }}</th>
                  <th>#</th>
                  <th>{{ 'tp_state' | t }}</th>
                  <th>{{ 'tp_begin' | t }}</th>
                  <th>{{ 'tp_end' | t }}</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                @for (ev of dayEvents(day()); track ev.event) {
                  <tr [class.unused]="ev.begin === '24:00'">
                    <td class="dayshort">{{ dayShort(ev.day) }}</td>
                    <td class="muted">{{ ev.event + 1 }}</td>
                    <td>
                      <select [value]="ev.state" (change)="setState(ev, $event)" [disabled]="ev.begin === '24:00'">
                        @for (s of stateList(); track s.value) {
                          <option [value]="s.value" [selected]="s.value === ev.state">{{ s.label }}</option>
                        }
                      </select>
                      <i class="swatch" [style.background]="color(ev.state)"></i>
                    </td>
                    <td>
                      @if (ev.begin === '24:00') {
                        <span class="num">{{ ev.begin }}</span>
                      } @else if (ev.event === 0) {
                        @if (canInsertBefore(ev)) {
                          <select [value]="ev.begin" (change)="setFirstBegin(ev, $event)" [title]="'tp_first_begin' | t">
                            @for (t of firstBeginTimes(ev); track t) {
                              <option [value]="t" [selected]="t === ev.begin">{{ t }}</option>
                            }
                          </select>
                        } @else {
                          <span class="num" [title]="'tp_split_full' | t">{{ ev.begin }}</span>
                        }
                      } @else {
                        <select [value]="ev.begin" (change)="setBegin(ev, $event)">
                          @for (t of beginTimes(ev); track t) {
                            <option [value]="t" [selected]="t === ev.begin">{{ t }}</option>
                          }
                        </select>
                      }
                    </td>
                    <td>
                      <select [value]="ev.end" (change)="setEnd(ev, $event)" [disabled]="ev.begin === '24:00'">
                        @for (t of endTimes(ev); track t) {
                          <option [value]="t" [selected]="t === ev.end">{{ t }}</option>
                        }
                      </select>
                    </td>
                    <td class="actions">
                      @if (ev.begin === '24:00') {
                        <span class="muted small">{{ 'tp_unused' | t }}</span>
                      } @else {
                        <button
                          class="small"
                          (click)="split(ev)"
                          [disabled]="!canSplit(ev)"
                          [title]="dayFull(ev.day) ? ('tp_split_full' | t) : ''"
                        >
                          ⊕ {{ 'tp_split' | t }}
                        </button>
                        <button class="small" (click)="remove(ev)" [disabled]="usedCount(ev.day) < 2" [title]="'tp_remove' | t">✕</button>
                      }
                    </td>
                  </tr>
                }
              </tbody>
            </table>
            <div class="muted small">{{ 'tp_gapless_hint' | t }}</div>
            <div class="row">
              <span class="spacer"></span>
              @if (dirtyPrograms().has(p.id)) {
                <button (click)="undo()">{{ 'reset' | t }}</button>
              }
              <button class="primary" (click)="transfer()" [disabled]="!dirtyPrograms().has(p.id) || !state.canWrite() || saving()">
                {{ 'tp_transfer' | t }}
              </button>
            </div>
            @if (state.readOnly()) {
              <div class="banner info">{{ 'read_only_hint' | t }}</div>
            }
          </div>
        }
      }
    </div>
  `,
  styles: `
    .legend { display: flex; gap: 12px; font-size: 12px; }
    .legend i, .swatch { display: inline-block; width: 12px; height: 12px; border-radius: 3px; margin-right: 4px; vertical-align: -2px; }
    .swatch { margin-left: 6px; }
    .week { display: grid; grid-template-columns: 110px 1fr; gap: 6px 10px; align-items: center; }
    .hours { position: relative; height: 16px; font-size: 11px; color: var(--muted); }
    .hours span { position: absolute; transform: translateX(-50%); }
    .hours span:first-child { transform: none; }
    .hours span:last-child { transform: translateX(-100%); }
    .dayname { border: none; background: none; text-align: left; padding: 4px; &.active { font-weight: 700; color: var(--accent); } }
    .bar {
      position: relative;
      height: 26px;
      background: var(--surface-2);
      border-radius: 6px;
      overflow: hidden;
      cursor: pointer;
      &.active { outline: 2px solid var(--accent); }
    }
    .seg { position: absolute; top: 0; bottom: 0; border-right: 1px solid var(--surface); }
    tr.unused td { opacity: 0.5; }
    .small { font-size: 12px; }
    select { min-width: 110px; }
    .actions { white-space: nowrap; }
    .dayshort { font-weight: 600; }
    .actions button + button { margin-left: 4px; }
  `,
})
export class TimeProgramsPage {
  protected state = inject(State);
  private api = inject(Api);
  private toasts = inject(Toasts);
  private i18n = inject(I18n);

  protected readonly hours = [0, 3, 6, 9, 12, 15, 18, 21, 24];
  protected readonly toMinutes = toMinutes;
  protected loading = signal(false);
  protected saving = signal(false);
  private original = signal<TimeProgram[]>([]);
  protected programs = signal<TimeProgram[]>([]);
  private selId = signal<number | null>(null);
  protected day = signal(0);
  protected clipboard = signal<{ program: number; day: number; events: TimeProgramEvent[] } | null>(null);

  protected sel = computed(() => this.programs().find((p) => p.id === this.selId()) ?? this.programs()[0]);
  protected days = computed(() => Array.from({ length: this.sel()?.days ?? 7 }, (_, i) => i));
  protected dirtyPrograms = computed(() => {
    const out = new Set<number>();
    for (const p of this.programs()) {
      const o = this.original().find((x) => x.id === p.id);
      if (o && JSON.stringify(o.events) !== JSON.stringify(p.events)) out.add(p.id);
    }
    return out;
  });
  protected stateList = computed(() => {
    const n = this.sel()?.states === 2 ? 2 : 3;
    return Array.from({ length: n }, (_, i) => ({ value: i, label: this.stateLabel(i), color: this.color(i) }));
  });

  protected dayKey(d: number): Key {
    return `day_${d}` as Key;
  }
  protected dayShort(d: number) {
    return this.i18n.t(this.dayKey(d)).slice(0, 2);
  }
  protected stateLabel(s: number) {
    const n = this.sel()?.states === 2 ? 2 : 3;
    return this.i18n.t(`st${n}_${Math.min(s, n - 1)}` as Key);
  }
  protected color(s: number) {
    return (this.sel()?.states === 2 ? colors2 : colors3)[s] ?? '#999';
  }

  protected dayEvents(d: number): TimeProgramEvent[] {
    return (this.sel()?.events ?? []).filter((e) => e.day === d).sort((a, b) => a.event - b.event);
  }

  protected async load() {
    this.loading.set(true);
    try {
      const tps = await this.api.timePrograms();
      this.original.set(structuredClone(tps));
      this.programs.set(tps);
    } catch (e) {
      this.toasts.error(e);
    } finally {
      this.loading.set(false);
    }
  }

  protected selectProgram(id: number) {
    this.selId.set(id);
  }

  /** Valid end times of an event: from begin + step to 24:00. */
  protected endTimes(ev: TimeProgramEvent): string[] {
    const step = this.sel()?.step || 15;
    const b = toMinutes(ev.begin);
    const out: string[] = [];
    for (let m = b + step; m < 1440; m += step) out.push(fromMinutes(m));
    out.push('24:00');
    if (!out.includes(ev.end)) out.unshift(ev.end);
    return out;
  }

  private updateDay(d: number, fn: (evs: TimeProgramEvent[]) => void) {
    const p = this.sel();
    if (!p) return;
    this.programs.update((ps) =>
      ps.map((x) => {
        if (x.id !== p.id) return x;
        const events = x.events.map((e) => ({ ...e }));
        const evs = events.filter((e) => e.day === d).sort((a, b) => a.event - b.event);
        fn(evs);
        return { ...x, events };
      }),
    );
  }

  protected setState(ev: TimeProgramEvent, e: Event) {
    const v = Number((e.target as HTMLSelectElement).value);
    this.updateDay(ev.day, (evs) => (evs[ev.event].state = v));
  }

  /**
   * Valid begin times of an event after the first: after the previous begin,
   * before 24:00. The controller stores a day without gaps, so a begin is the
   * previous period's end.
   */
  protected beginTimes(ev: TimeProgramEvent): string[] {
    const prev = this.dayEvents(ev.day)[ev.event - 1];
    if (!prev) return [ev.begin];
    return this.endTimes(prev).filter((t) => t !== '24:00' || t === ev.begin);
  }

  protected setBegin(ev: TimeProgramEvent, e: Event) {
    this.moveEnd(ev.day, ev.event - 1, (e.target as HTMLSelectElement).value);
  }

  protected setEnd(ev: TimeProgramEvent, e: Event) {
    this.moveEnd(ev.day, ev.event, (e.target as HTMLSelectElement).value);
  }

  /** Changing an end time moves the following periods, as in homeControl. */
  private moveEnd(day: number, event: number, end: string) {
    const step = this.sel()?.step || 15;
    this.updateDay(day, (evs) => {
      evs[event].end = end;
      for (let i = event + 1; i < evs.length; i++) {
        evs[i].begin = evs[i - 1].end;
        if (toMinutes(evs[i].end) < toMinutes(evs[i].begin) || i === evs.length - 1) {
          evs[i].end = i === evs.length - 1 ? '24:00' : fromMinutes(Math.min(1440, toMinutes(evs[i].begin) + step));
        }
      }
    });
  }

  protected usedCount(day: number) {
    return this.dayEvents(day).filter((e) => e.begin !== '24:00').length;
  }

  /** All events of the day are in use, so none can be added. */
  protected dayFull(day: number) {
    const evs = this.dayEvents(day);
    return evs.length > 0 && evs[evs.length - 1].begin !== '24:00';
  }

  /** A later begin for the first period needs a free slot for the new 00:00 period. */
  protected canInsertBefore(ev: TimeProgramEvent) {
    const step = this.sel()?.step || 15;
    return !this.dayFull(ev.day) && toMinutes(ev.end) - toMinutes(ev.begin) >= 2 * step;
  }

  /** 00:00, or a later begin up to one step before the end. */
  protected firstBeginTimes(ev: TimeProgramEvent): string[] {
    const step = this.sel()?.step || 15;
    const out = ['00:00'];
    for (let m = step; m < toMinutes(ev.end); m += step) out.push(fromMinutes(m));
    return out;
  }

  /**
   * The day must start at 00:00, so a later begin inserts a period before it.
   * It takes the state of the day's last period: the night continues.
   */
  protected setFirstBegin(ev: TimeProgramEvent, e: Event) {
    const begin = (e.target as HTMLSelectElement).value;
    if (begin === '00:00' || !this.canInsertBefore(ev)) return;
    this.updateDay(ev.day, (evs) => {
      const used = evs.filter((x) => x.begin !== '24:00');
      const night = used[used.length - 1].state;
      for (let j = evs.length - 1; j > 0; j--) Object.assign(evs[j], pick(evs[j - 1]));
      Object.assign(evs[0], { state: night, begin: '00:00', end: begin });
      evs[1].begin = begin;
    });
  }

  protected canSplit(ev: TimeProgramEvent) {
    const step = this.sel()?.step || 15;
    return ev.begin !== '24:00' && !this.dayFull(ev.day) && toMinutes(ev.end) - toMinutes(ev.begin) >= 2 * step;
  }

  /** Splits a period in two halves with the same state; later periods move down one slot. */
  protected split(ev: TimeProgramEvent) {
    if (!this.canSplit(ev)) return;
    const step = this.sel()?.step || 15;
    const b = toMinutes(ev.begin);
    const mid = fromMinutes(b + Math.floor((toMinutes(ev.end) - b) / 2 / step) * step);
    this.updateDay(ev.day, (evs) => {
      const i = ev.event;
      for (let j = evs.length - 1; j > i + 1; j--) Object.assign(evs[j], pick(evs[j - 1]));
      Object.assign(evs[i + 1], { state: evs[i].state, begin: mid, end: evs[i].end });
      evs[i].end = mid;
    });
  }

  /** Removes a period: the previous one (or, for the first, the next one) takes over its time. */
  protected remove(ev: TimeProgramEvent) {
    if (this.usedCount(ev.day) < 2) return;
    this.updateDay(ev.day, (evs) => {
      const i = ev.event;
      const removed = pick(evs[i]);
      for (let j = i; j < evs.length - 1; j++) Object.assign(evs[j], pick(evs[j + 1]));
      Object.assign(evs[evs.length - 1], { begin: '24:00', end: '24:00' });
      if (i > 0) evs[i - 1].end = removed.end;
      else evs[0].begin = '00:00';
    });
  }

  protected copyDay() {
    const p = this.sel();
    if (p) this.clipboard.set({ program: p.id, day: this.day(), events: this.dayEvents(this.day()).map((e) => ({ ...e })) });
  }

  protected paste(d: number) {
    const c = this.clipboard();
    if (!c) return;
    this.updateDay(d, (evs) => {
      evs.forEach((e, i) => {
        const src = c.events[i];
        if (src) Object.assign(e, { state: src.state, begin: src.begin, end: src.end });
      });
    });
  }

  protected pasteAll() {
    for (const d of this.days()) {
      const c = this.clipboard();
      if (c && (d !== c.day || c.program !== this.sel()?.id)) this.paste(d);
    }
  }

  protected undo() {
    const p = this.sel();
    const o = this.original().find((x) => x.id === p?.id);
    if (o) this.programs.update((ps) => ps.map((x) => (x.id === o.id ? structuredClone(o) : x)));
  }

  protected async transfer() {
    const p = this.sel();
    if (!p) return;
    this.saving.set(true);
    try {
      const r = await this.api.saveTimeProgram(p.id, p.events);
      this.toasts.ok(r.written ? `${p.name}: ${r.written} ${this.i18n.t('tp_written')}` : this.i18n.t('tp_no_changes'));
      this.original.update((os) => os.map((o) => (o.id === p.id ? structuredClone(p) : o)));
    } catch (e) {
      this.toasts.error(e);
    } finally {
      this.saving.set(false);
    }
  }

  protected exportProgram() {
    const p = this.sel();
    if (p) download(`timeprogram-${p.name}.json`, JSON.stringify(p, null, 2));
  }

  protected async importProgram(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    input.value = '';
    const p = this.sel();
    if (!file || !p) return;
    try {
      const data = JSON.parse(await file.text()) as TimeProgram;
      if (data.days !== p.days || data.eventsPerDay !== p.eventsPerDay || data.states !== p.states) {
        throw new Error('Program layout does not match / Programmaufbau passt nicht');
      }
      this.programs.update((ps) => ps.map((x) => (x.id === p.id ? { ...x, events: data.events.map((ev) => ({ ...ev, program: p.id })) } : x)));
    } catch (err) {
      this.toasts.error(err);
    }
  }
}
