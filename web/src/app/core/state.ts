import { Injectable, computed, inject, signal } from '@angular/core';
import { Subject } from 'rxjs';
import { Api } from './api';
import { Item, Job, LogEntry, Override, Recording, Sample, Snapshot, Values } from './models';

/** Live state of the heat pump, kept current by the server's event stream. */
@Injectable({ providedIn: 'root' })
export class State {
  private api = inject(Api);
  private es?: EventSource;
  private retry?: ReturnType<typeof setTimeout>;

  readonly snapshot = signal<Snapshot | null>(null);
  readonly streamConnected = signal(false);
  readonly log = signal<LogEntry[]>([]);
  /** Live samples of the running recording. */
  readonly samples = new Subject<Sample>();

  readonly status = computed(() => this.snapshot()?.status ?? 'offline');
  readonly online = computed(() => this.status() === 'online');
  readonly readOnly = computed(() => this.snapshot()?.readOnly ?? true);
  readonly panels = computed(() => this.snapshot()?.panels ?? []);
  readonly job = computed<Job | null>(() => this.snapshot()?.job ?? null);
  readonly recording = computed<Recording | null>(() => this.snapshot()?.recording ?? null);
  readonly canWrite = computed(() => this.online() && !this.readOnly());
  readonly overrides = computed<Override[]>(() => this.snapshot()?.overrides ?? []);

  start() {
    if (this.es) return;
    this.api.log().then((l) => this.log.set(l), () => undefined);
    const es = new EventSource('/api/events');
    this.es = es;
    es.onopen = () => this.streamConnected.set(true);
    es.onerror = () => {
      this.streamConnected.set(false);
      es.close();
      this.es = undefined;
      clearTimeout(this.retry);
      this.retry = setTimeout(() => this.start(), 3000);
    };
    const on = <T>(type: string, fn: (d: T) => void) =>
      es.addEventListener(type, (e) => fn(JSON.parse((e as MessageEvent).data) as T));
    on<Snapshot>('state', (s) => this.snapshot.set(s));
    on<Values>('values', (v) =>
      this.snapshot.update((s) => (s ? { ...s, mp: { ...s.mp, ...v.mp }, sp: { ...s.sp, ...v.sp }, updated: v.updated } : s)),
    );
    on<Job | null>('job', (j) => this.snapshot.update((s) => (s ? { ...s, job: j } : s)));
    on<Recording | null>('recording', (r) => this.snapshot.update((s) => (s ? { ...s, recording: r } : s)));
    on<Override[]>('overrides', (o) => this.snapshot.update((s) => (s ? { ...s, overrides: o } : s)));
    on<LogEntry>('log', (e) => this.log.update((l) => [e, ...l].slice(0, 500)));
    on<Sample>('sample', (x) => {
      this.samples.next(x);
      this.snapshot.update((s) =>
        s?.recording && s.recording.id === x.recordingId
          ? { ...s, recording: { ...s.recording, samples: s.recording.samples + Object.keys(x.values).length } }
          : s,
      );
    });
  }

  stop() {
    clearTimeout(this.retry);
    this.es?.close();
    this.es = undefined;
  }

  /** Current value of a catalog item (ratios are computed). */
  value(it: Item): number | undefined {
    const s = this.snapshot();
    if (!s) return undefined;
    if (it.display === 'ratio') {
      const n = s.mp[String(it.num)];
      const d = s.mp[String(it.den)];
      return n === undefined || d === undefined || d === 0 ? undefined : n / d;
    }
    return (it.kind === 'mp' ? s.mp : s.sp)[String(it.id)];
  }

  sp(id: number): number | undefined {
    return this.snapshot()?.sp[String(id)];
  }

  mp(id: number): number | undefined {
    return this.snapshot()?.mp[String(id)];
  }

  override(sp: number): Override | undefined {
    return this.overrides().find((o) => o.sp === sp);
  }

  /** Optimistically store a setup point value after a successful write. */
  setSP(id: number, v: number) {
    this.snapshot.update((s) => (s ? { ...s, sp: { ...s.sp, [String(id)]: v } } : s));
  }
}
