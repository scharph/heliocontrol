import { ChangeDetectionStrategy, Component, inject } from '@angular/core';
import { RouterOutlet } from '@angular/router';
import { I18n } from './core/i18n';
import { Toasts } from './core/toast';

@Component({
  selector: 'app-root',
  imports: [RouterOutlet],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <router-outlet />
    <div class="toasts" aria-live="polite">
      @for (t of toasts.items(); track t.id) {
        <div class="toast" [class]="t.kind" (click)="toasts.dismiss(t.id)">{{ t.text }}</div>
      }
    </div>
  `,
  styles: `
    .toasts {
      position: fixed;
      right: 16px;
      bottom: 16px;
      display: flex;
      flex-direction: column;
      gap: 8px;
      z-index: 100;
      max-width: min(420px, calc(100vw - 32px));
    }
    .toast {
      padding: 10px 14px;
      border-radius: 8px;
      background: var(--surface);
      border: 1px solid var(--border);
      box-shadow: 0 4px 16px rgb(0 0 0 / 15%);
      cursor: pointer;
      &.ok { border-left: 4px solid var(--ok); }
      &.err { border-left: 4px solid var(--err); }
      &.info { border-left: 4px solid var(--info); }
    }
  `,
})
export class App {
  protected toasts = inject(Toasts);
  constructor() {
    const i18n = inject(I18n);
    document.documentElement.lang = i18n.lang();
    try {
      const theme = localStorage.getItem('theme');
      if (theme === 'light' || theme === 'dark') document.documentElement.dataset['theme'] = theme;
    } catch {
      /* ignore */
    }
  }
}
