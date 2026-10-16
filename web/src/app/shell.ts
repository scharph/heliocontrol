import { ChangeDetectionStrategy, Component, OnDestroy, computed, inject, signal } from '@angular/core';
import { Router, RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { Api } from './core/api';
import { fmtTime } from './core/format';
import { I18n, Key, TPipe } from './core/i18n';
import { State } from './core/state';
import { Toasts } from './core/toast';

@Component({
  selector: 'hc-shell',
  imports: [RouterOutlet, RouterLink, RouterLinkActive, TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="layout" [class.nav-open]="navOpen()">
      <aside>
        <div class="brand">
          <svg viewBox="0 0 32 32" width="28" height="28" aria-hidden="true">
            <rect width="32" height="32" rx="7" fill="var(--accent)" />
            <path d="M16 6c-3 4-6 7-6 11a6 6 0 0 0 12 0c0-2-1-4-2-5 0 2-1 3-2 3 0-3-1-6-2-9z" fill="#fff" />
          </svg>
          <div>
            <strong>heliocontrol</strong>
            <div class="muted small">Heliotherm web control</div>
          </div>
        </div>
        <nav (click)="navOpen.set(false)">
          @for (n of nav; track n.path) {
            <a [routerLink]="n.path" routerLinkActive="active">{{ n.label | t }}</a>
          }
        </nav>
        <div class="foot muted small">
          @if (version()) {
            <span>{{ version() }}</span>
          }
        </div>
      </aside>

      <div class="main">
        <header>
          <button class="icon menu" (click)="navOpen.set(!navOpen())" aria-label="Menu">☰</button>
          <span class="badge" [class]="statusClass()">
            <span class="dot"></span>{{ statusLabel() | t }}
          </span>
          @if (snap(); as s) {
            <span class="muted small hide-sm">{{ s.target }}</span>
            @if (s.deviceId) {
              <span class="muted small hide-sm">· {{ 'controller' | t }} {{ s.deviceId }}</span>
            }
            @if (s.firmware) {
              <span class="muted small hide-sm">· {{ 'firmware' | t }} {{ s.firmware }}</span>
            }
            @if (s.readOnly) {
              <span class="badge info" [title]="'read_only_hint' | t">🔒 {{ 'read_only' | t }}</span>
            }
            @if (s.expertMode && !s.readOnly) {
              <span class="badge warn">{{ 'expert' | t }}</span>
            }
          }
          <span class="spacer"></span>
          @if (updated(); as u) {
            <span class="muted small hide-sm">{{ 'updated' | t }} {{ u }}</span>
          }
          @if (state.online()) {
            <button (click)="refresh()" [disabled]="busy()">⟳ {{ 'refresh' | t }}</button>
            <button (click)="disconnect()">{{ 'disconnect' | t }}</button>
          } @else {
            <button class="primary" (click)="connect()" [disabled]="state.status() === 'connecting'">{{ 'connect' | t }}</button>
          }
          <button class="small" (click)="toggleLang()">{{ i18n.lang() === 'de' ? 'EN' : 'DE' }}</button>
          @if (authRequired()) {
            <button class="small" (click)="logout()">{{ 'logout' | t }}</button>
          }
        </header>
        @if (state.job(); as j) {
          <div class="jobbar">
            <span class="small">{{ jobLabel() }} {{ j.total ? j.done + ' / ' + j.total : '' }}</span>
            <div class="progress"><div [style.width.%]="j.total ? (100 * j.done) / j.total : 5"></div></div>
          </div>
        }
        @if (state.status() === 'error' && snap()?.error) {
          <div class="banner err" style="margin: 12px 16px 0">{{ snap()?.error }}</div>
        }
        @if (!state.streamConnected()) {
          <div class="banner warn" style="margin: 12px 16px 0">{{ 'stream_lost' | t }}</div>
        }
        <main><router-outlet /></main>
      </div>
    </div>
  `,
  styles: `
    .layout { display: grid; grid-template-columns: 220px 1fr; min-height: 100vh; }
    aside {
      background: var(--surface);
      border-right: 1px solid var(--border);
      display: flex;
      flex-direction: column;
      position: sticky;
      top: 0;
      height: 100vh;
    }
    .brand { display: flex; gap: 10px; align-items: center; padding: 16px; }
    .small { font-size: 12px; }
    nav { display: flex; flex-direction: column; padding: 8px; gap: 2px; }
    nav a {
      padding: 8px 12px;
      border-radius: 8px;
      color: var(--text);
      text-decoration: none;
      &:hover { background: var(--surface-2); }
      &.active { background: var(--accent-soft); color: var(--accent); font-weight: 600; }
    }
    .foot { margin-top: auto; padding: 16px; }
    .main { min-width: 0; display: flex; flex-direction: column; }
    header {
      display: flex;
      align-items: center;
      gap: 8px;
      flex-wrap: wrap;
      padding: 10px 16px;
      background: var(--surface);
      border-bottom: 1px solid var(--border);
      position: sticky;
      top: 0;
      z-index: 10;
    }
    .badge.online { background: var(--ok-soft); color: var(--ok); }
    .badge.connecting { background: var(--warn-soft); color: var(--warn); }
    .badge.error { background: var(--err-soft); color: var(--err); }
    .jobbar { padding: 8px 16px; display: flex; flex-direction: column; gap: 4px; background: var(--surface); border-bottom: 1px solid var(--border); }
    main { padding: 16px; max-width: 1400px; width: 100%; }
    .menu { display: none; }
    @media (max-width: 900px) {
      .layout { grid-template-columns: 1fr; }
      aside {
        position: fixed;
        z-index: 20;
        transform: translateX(-100%);
        transition: transform 0.2s;
        width: 240px;
        box-shadow: 0 0 24px rgb(0 0 0 / 20%);
      }
      .nav-open aside { transform: none; }
      .menu { display: inline-flex; }
      .hide-sm { display: none; }
      main { padding: 12px; }
    }
  `,
})
export class Shell implements OnDestroy {
  protected state = inject(State);
  protected i18n = inject(I18n);
  private api = inject(Api);
  private toasts = inject(Toasts);
  private router = inject(Router);

  protected navOpen = signal(false);
  protected busy = signal(false);
  protected version = signal('');
  protected authRequired = signal(false);
  protected snap = this.state.snapshot;

  protected nav: { path: string; label: Key }[] = [
    { path: '/overview', label: 'nav_overview' },
    { path: '/timeprograms', label: 'nav_timeprograms' },
    { path: '/insights', label: 'nav_insights' },
    { path: '/recorder', label: 'nav_recorder' },
    { path: '/trend', label: 'nav_trend' },
    { path: '/points', label: 'nav_points' },
    { path: '/log', label: 'nav_log' },
    { path: '/settings', label: 'nav_settings' },
  ];

  protected statusClass = computed(() => this.state.status());
  protected statusLabel = computed<Key>(() => `status_${this.state.status()}` as Key);
  protected updated = computed(() => {
    const u = this.snap()?.updated;
    return u ? fmtTime(u, this.i18n.locale(), false) : '';
  });
  protected jobLabel = computed(() => {
    const name = this.state.job()?.name ?? '';
    const key = `job_${name.replace(/-/g, '_')}` as Key;
    try {
      return this.i18n.t(key);
    } catch {
      return name;
    }
  });

  constructor() {
    this.state.start();
    this.api.version().then((v) => this.version.set(v.version), () => undefined);
    this.api.auth().then((a) => this.authRequired.set(a.required), () => undefined);
  }

  ngOnDestroy() {
    this.state.stop();
  }

  async refresh() {
    this.busy.set(true);
    try {
      await this.api.refresh();
    } catch (e) {
      this.toasts.error(e);
    } finally {
      this.busy.set(false);
    }
  }
  async connect() {
    try {
      await this.api.connect();
    } catch (e) {
      this.toasts.error(e);
    }
  }
  async disconnect() {
    try {
      await this.api.disconnect();
    } catch (e) {
      this.toasts.error(e);
    }
  }
  toggleLang() {
    this.i18n.setLang(this.i18n.lang() === 'de' ? 'en' : 'de');
  }
  async logout() {
    await this.api.logout().catch(() => undefined);
    this.state.stop();
    this.router.navigate(['/login']);
  }
}
