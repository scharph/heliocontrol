// Types mirroring the JSON of the Go server (internal/service, catalog, protocol, store).

export interface Text {
  de: string;
  en: string;
}

export type Display = 'value' | 'bool' | 'enum' | 'duration' | 'fault' | 'ratio';

export interface Item {
  key: string;
  kind: 'mp' | 'sp';
  id: number;
  label: Text;
  unit?: string;
  display: Display;
  options?: Text[];
  offset?: number;
  writable?: boolean;
  min?: number;
  max?: number;
  step?: number;
  notSelectable?: number[];
  num?: number;
  den?: number;
}

export interface HeatCurve {
  boost: number;
  reduction: number;
  limit: number;
  atLimit: number;
  at0: number;
  atMinus15: number;
}

export interface Panel {
  id: string;
  title: Text;
  subtitle?: Text;
  items: Item[];
  heatCurve?: HeatCurve;
}

export interface Job {
  name: string;
  done: number;
  total: number;
}

export interface RecordingPoint {
  id: number;
  name: string;
  unit: string;
  color: string;
}

export interface Recording {
  id: number;
  name: string;
  source: 'live' | 'trend' | 'import';
  state: 'recording' | 'paused' | 'stopped';
  started: string;
  ended?: string;
  intervalMs: number;
  points: RecordingPoint[];
  samples: number;
}

export type ValueMap = Record<string, number>;

export interface Snapshot {
  status: 'offline' | 'connecting' | 'online' | 'error';
  error?: string;
  target: string;
  readOnly: boolean;
  expertMode: boolean;
  deviceId?: number;
  firmware?: string;
  installation: { plantType: number; mixer1: boolean; mixer2: boolean; energy: boolean };
  panels: Panel[];
  mp: ValueMap;
  sp: ValueMap;
  updated?: string;
  pollSeconds: number;
  job?: Job | null;
  recording?: Recording | null;
  overrides: Override[];
}

/** A temporary setpoint that heliocontrol restores automatically. */
export interface Override {
  sp: number;
  value: number;
  original: number;
  until: string;
  created: string;
  source?: string;
}

export interface DayInsight {
  date: string;
  coverage: number;
  starts?: number;
  startsSource?: "counter" | "samples";
  runtimeHours?: number;
  avgCycleMinutes?: number;
  outdoor?: number;
  flowRunning?: number;
  returnRunning?: number;
  spread?: number;
  targetRunning?: number;
  deviation?: number;
  heatingHours?: number;
  dhwHours?: number;
  dhwInterrupts?: number;
  heatOutKWh?: number;
  heatInKWh?: number;
  cop?: number;
}

export interface Tip {
  text: Text;
  source: string;
}

export interface Hint {
  level: "info" | "warn";
  title: Text;
  detail: Text;
  tips?: Tip[];
}

export interface Insights {
  days: DayInsight[];
  hints: Hint[];
}

export interface Values {
  mp: ValueMap;
  sp: ValueMap;
  updated: string;
}

export interface Point {
  kind: 'mp' | 'sp';
  id: number;
  nr: number;
  name: string;
  value: number;
  min: number;
  max: number;
  used: boolean;
  writable: boolean;
  acs: number;
}

export interface LogEntry {
  id: number;
  time: string;
  level: 'info' | 'warn' | 'error' | 'connect' | 'disconnect';
  title: string;
  detail?: string;
}

export interface Series {
  pointId: number;
  t: number[];
  v: number[];
}

export interface Sample {
  recordingId: number;
  t: number;
  values: ValueMap;
}

export interface TimeProgramEvent {
  program: number;
  day: number;
  event: number;
  state: number;
  begin: string; // HH:MM
  end: string; // HH:MM, 24:00 = end of day
}

export interface TimeProgram {
  id: number;
  name: string;
  eventsPerDay: number;
  states: number;
  step: number;
  days: number;
  used: boolean;
  events: TimeProgramEvent[];
}

export interface TrendConfig {
  alarm: boolean;
  callRemote: boolean;
  enabled: boolean;
  type: number; // 0 interval, 1 hysteresis
  hysteresis: number;
  interval: number;
  end: string;
}

export interface TrendPoint {
  id: number;
  name: string;
  min: number;
  max: number;
  config: TrendConfig;
}

export interface ClockInfo {
  device: string;
  server: string;
  offsetSeconds: number;
}

export interface Settings {
  listen: string;
  dataDir: string;
  device: {
    type: 'tcp' | 'serial';
    host: string;
    port: number;
    serialPort: string;
    baud: number;
    dataBits: number;
    stopBits: number;
    expectedId: number;
    autoConnect: boolean;
  };
  pollSeconds: number;
  writeBudgetPerDay: number;
  historyDays: number;
  trendPauseDuringDownload: boolean;
  readOnly: boolean;
  expertMode: boolean;
  mqtt: {
    enabled: boolean;
    broker: string;
    username: string;
    password?: string;
    clientId: string;
    discoveryPrefix: string;
    topicPrefix: string;
    language: "de" | "en";
  };
  passwordSet: boolean;
  mqttPasswordSet: boolean;
}
