import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { Api } from '../core/api';
import { fmtTime } from '../core/format';
import { I18n, TPipe } from '../core/i18n';
import { LogEntry } from '../core/models';
import { State } from '../core/state';
import { Toasts } from '../core/toast';

const icons: Record<LogEntry['level'], string> = { info: 'ℹ', warn: '⚠', error: '✖', connect: '🔌', disconnect: '⏻' };

@Component({
  selector: 'hc-log',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="stack">
      <div class="row">
        <h1>{{ 'nav_log' | t }}</h1>
        <span class="spacer"></span>
        <button (click)="clear()" [disabled]="!state.log().length">{{ 'log_clear' | t }}</button>
      </div>
      <div class="card list">
        @for (e of state.log(); track e.id) {
          <div class="entry" [class]="e.level">
            <span class="icon">{{ icon(e) }}</span>
            <span class="time num muted">{{ fmt(e.time) }}</span>
            <span class="title">{{ e.title }}</span>
            <span class="detail muted">{{ e.detail }}</span>
          </div>
        } @empty {
          <div class="muted">{{ 'log_empty' | t }}</div>
        }
      </div>
    </div>
  `,
  styles: `
    .list { padding: 4px 0; }
    .entry {
      display: grid;
      grid-template-columns: 28px 160px minmax(160px, auto) 1fr;
      gap: 8px;
      padding: 6px 12px;
      border-bottom: 1px solid var(--border);
      align-items: baseline;
      &:last-child { border-bottom: none; }
      &.error .title { color: var(--err); }
      &.warn .title { color: var(--warn); }
    }
    .detail { overflow-wrap: anywhere; }
    @media (max-width: 720px) { .entry { grid-template-columns: 24px 1fr; } .detail, .time { grid-column: 2; } }
  `,
})
export class LogPage {
  protected state = inject(State);
  private api = inject(Api);
  private toasts = inject(Toasts);
  private i18n = inject(I18n);

  protected icon(e: LogEntry) {
    return icons[e.level] ?? '•';
  }
  protected fmt(t: string) {
    return fmtTime(t, this.i18n.locale());
  }
  protected async clear() {
    try {
      await this.api.clearLog();
      this.state.log.set([]);
    } catch (e) {
      this.toasts.error(e);
    }
  }
}
