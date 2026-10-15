import { Item } from './models';

export function fmtNumber(v: number | undefined, locale: string, maxDigits = 2): string {
  if (v === undefined || Number.isNaN(v)) return '–';
  return v.toLocaleString(locale, { maximumFractionDigits: maxDigits });
}

export function fmtDuration(seconds: number | undefined): string {
  if (seconds === undefined) return '–';
  const s = Math.max(0, Math.round(seconds));
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  const pad = (n: number) => String(n).padStart(2, '0');
  return (d ? `${d}d ` : '') + `${pad(h)}:${pad(m)}:${pad(s % 60)}`;
}

export function fmtTime(iso: string | number | undefined, locale: string, withDate = true): string {
  if (iso === undefined || iso === '') return '–';
  const d = new Date(iso);
  return withDate
    ? d.toLocaleString(locale, { dateStyle: 'short', timeStyle: 'medium' })
    : d.toLocaleTimeString(locale);
}

/** Decimal places that fit an item's step. */
export function decimals(it: Item): number {
  if (!it.step || it.step >= 1) return 0;
  return Math.min(3, Math.ceil(-Math.log10(it.step) - 1e-9));
}

export function clamp(v: number, min?: number, max?: number) {
  if (min !== undefined && v < min) return min;
  if (max !== undefined && v > max) return max;
  return v;
}

/** "HH:MM" <-> minutes; 24:00 = 1440. */
export function toMinutes(s: string): number {
  const [h, m] = s.split(':').map(Number);
  return h * 60 + m;
}
export function fromMinutes(n: number): string {
  return `${String(Math.floor(n / 60)).padStart(2, '0')}:${String(n % 60).padStart(2, '0')}`;
}

export function download(name: string, data: string, type = 'application/json') {
  const a = document.createElement('a');
  a.href = URL.createObjectURL(new Blob([data], { type }));
  a.download = name;
  a.click();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
}
