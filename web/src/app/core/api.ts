import { HttpClient, HttpErrorResponse } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import {
  ClockInfo,
  LogEntry,
  Point,
  Recording,
  Series,
  Settings,
  Snapshot,
  TimeProgram,
  TimeProgramEvent,
  TrendConfig,
  TrendPoint,
  Values,
} from './models';

/** Error message of a failed API call (the server answers {"error": "..."}). */
export function errorMessage(e: unknown): string {
  if (e instanceof HttpErrorResponse) {
    if (e.status === 0) return 'Server nicht erreichbar / server unreachable';
    const body = e.error as { error?: string } | string | null;
    if (body && typeof body === 'object' && body.error) return body.error;
    return `${e.status} ${e.statusText}`;
  }
  return e instanceof Error ? e.message : String(e);
}

@Injectable({ providedIn: 'root' })
export class Api {
  private http = inject(HttpClient);

  private get<T>(url: string) {
    return firstValueFrom(this.http.get<T>(url));
  }
  private post<T>(url: string, body: unknown = {}) {
    return firstValueFrom(this.http.post<T>(url, body));
  }
  private put<T>(url: string, body: unknown) {
    return firstValueFrom(this.http.put<T>(url, body));
  }

  auth() {
    return this.get<{ required: boolean; authenticated: boolean }>('/api/auth');
  }
  login(password: string) {
    return this.post<void>('/api/login', { password });
  }
  logout() {
    return this.post<void>('/api/logout');
  }
  version() {
    return this.get<{ version: string }>('/api/version');
  }

  state() {
    return this.get<Snapshot>('/api/state');
  }
  connect() {
    return this.post<void>('/api/connect');
  }
  disconnect() {
    return this.post<void>('/api/disconnect');
  }
  refresh() {
    return this.post<Values>('/api/refresh');
  }

  points(kind: 'mp' | 'sp', refresh = false) {
    return this.get<Point[]>(`/api/points/${kind}${refresh ? '?refresh=1' : ''}`);
  }
  initPoints() {
    return this.post<void>('/api/points/init');
  }
  writeSP(id: number, value: number) {
    return this.put<{ value: number }>(`/api/sp/${id}`, { value });
  }
  override(id: number, value: number, auto: boolean) {
    return this.post<void>(`/api/mp/${id}/override`, { value, auto });
  }
  resetFault() {
    return this.post<void>('/api/fault/reset');
  }
  clock() {
    return this.get<ClockInfo>('/api/clock');
  }
  syncClock() {
    return this.post<void>('/api/clock/sync');
  }

  timePrograms() {
    return this.get<TimeProgram[]>('/api/timeprograms');
  }
  saveTimeProgram(id: number, events: TimeProgramEvent[]) {
    return this.put<{ written: number }>(`/api/timeprograms/${id}`, { events });
  }

  trendPoints() {
    return this.get<TrendPoint[]>('/api/trend/points');
  }
  setTrendConfig(ids: number[], config: TrendConfig) {
    return this.put<void>('/api/trend/points', { ids, config });
  }
  trendCount() {
    return this.get<{ count: number }>('/api/trend/count');
  }
  downloadTrend(del: boolean) {
    return this.post<Recording>('/api/trend/download', { delete: del });
  }

  recordings() {
    return this.get<Recording[]>('/api/recordings');
  }
  startRecording(name: string, points: number[], intervalSeconds: number) {
    return this.post<Recording>('/api/recordings', { name, points, intervalSeconds });
  }
  recordingAction(action: 'pause' | 'resume' | 'stop') {
    return this.post<void>(`/api/recordings/active/${action}`);
  }
  samples(id: number, from?: number, to?: number) {
    const q = new URLSearchParams();
    if (from) q.set('from', String(from));
    if (to) q.set('to', String(to));
    return this.get<Series[]>(`/api/recordings/${id}/samples?${q}`);
  }
  renameRecording(id: number, name: string) {
    return firstValueFrom(this.http.patch<void>(`/api/recordings/${id}`, { name }));
  }
  deleteRecording(id: number) {
    return firstValueFrom(this.http.delete<void>(`/api/recordings/${id}`));
  }
  importFile(file: File) {
    const fd = new FormData();
    fd.append('file', file);
    return this.post<Recording>('/api/recordings/import', fd);
  }

  log() {
    return this.get<LogEntry[]>('/api/log');
  }
  clearLog() {
    return firstValueFrom(this.http.delete<void>('/api/log'));
  }

  settings() {
    return this.get<Settings>('/api/settings');
  }
  saveSettings(s: Settings) {
    return this.put<Settings>('/api/settings', s);
  }
  setPassword(current: string, next: string) {
    return this.post<void>('/api/settings/password', { current, new: next });
  }
  serialPorts() {
    return this.get<string[]>('/api/serialports');
  }
}
