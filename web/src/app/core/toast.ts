import { Injectable, signal } from '@angular/core';
import { errorMessage } from './api';

export interface Toast {
  id: number;
  kind: 'ok' | 'err' | 'info';
  text: string;
}

@Injectable({ providedIn: 'root' })
export class Toasts {
  readonly items = signal<Toast[]>([]);
  private next = 1;

  show(kind: Toast['kind'], text: string, ms = 4000) {
    const t = { id: this.next++, kind, text };
    this.items.update((l) => [...l, t]);
    setTimeout(() => this.dismiss(t.id), ms);
  }
  ok(text: string) {
    this.show('ok', text);
  }
  error(e: unknown) {
    this.show('err', errorMessage(e), 7000);
  }
  dismiss(id: number) {
    this.items.update((l) => l.filter((t) => t.id !== id));
  }
}
