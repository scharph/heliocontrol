import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { Router } from '@angular/router';
import { Api, errorMessage } from '../core/api';
import { TPipe } from '../core/i18n';

@Component({
  selector: 'hc-login',
  imports: [TPipe],
  changeDetection: ChangeDetectionStrategy.OnPush,
  template: `
    <div class="wrap">
      <form class="card stack" (submit)="login($event)">
        <h1>heliocontrol</h1>
        <label class="field">
          {{ 'login_password' | t }}
          <input type="password" autocomplete="current-password" [value]="password()" (input)="password.set(val($event))" autofocus />
        </label>
        @if (error()) {
          <div class="banner err">{{ error() }}</div>
        }
        <button class="primary" type="submit" [disabled]="busy()">{{ 'login_title' | t }}</button>
      </form>
    </div>
  `,
  styles: `
    .wrap { min-height: 100vh; display: grid; place-items: center; padding: 16px; }
    form { width: min(340px, 100%); }
  `,
})
export class LoginPage {
  private api = inject(Api);
  private router = inject(Router);
  protected password = signal('');
  protected error = signal('');
  protected busy = signal(false);

  protected val(e: Event) {
    return (e.target as HTMLInputElement).value;
  }

  protected async login(e: Event) {
    e.preventDefault();
    this.busy.set(true);
    this.error.set('');
    try {
      await this.api.login(this.password());
      this.router.navigateByUrl('/');
    } catch (err) {
      this.error.set(errorMessage(err));
    } finally {
      this.busy.set(false);
    }
  }
}
