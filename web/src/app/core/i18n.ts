import { Injectable, Pipe, PipeTransform, inject, signal } from '@angular/core';
import { Text } from './models';

export type Lang = 'de' | 'en';

const dict = {
  nav_overview: ['Übersicht', 'Overview'],
  nav_timeprograms: ['Zeitprogramme', 'Time programs'],
  nav_recorder: ['WEB-Online', 'WEB-Online'],
  nav_trend: ['Trenddaten', 'Trend data'],
  nav_points: ['Alle Datenpunkte', 'All points'],
  nav_log: ['Meldungen', 'Messages'],
  nav_settings: ['Einstellungen', 'Settings'],
  status_online: ['Verbunden', 'Connected'],
  status_offline: ['Getrennt', 'Disconnected'],
  status_connecting: ['Verbinde…', 'Connecting…'],
  status_error: ['Fehler', 'Error'],
  connect: ['Verbinden', 'Connect'],
  disconnect: ['Trennen', 'Disconnect'],
  refresh: ['Aktualisieren', 'Refresh'],
  logout: ['Abmelden', 'Log out'],
  read_only: ['Nur lesen', 'Read-only'],
  read_only_hint: [
    'Nur-Lese-Modus: Es werden keine Änderungen an die Wärmepumpe gesendet. Umschalten in den Einstellungen.',
    'Read-only mode: nothing is written to the heat pump. Change this in the settings.',
  ],
  expert: ['Experte', 'Expert'],
  controller: ['Regler', 'Controller'],
  firmware: ['Firmware', 'Firmware'],
  updated: ['Aktualisiert', 'Updated'],
  save: ['Speichern', 'Save'],
  cancel: ['Abbrechen', 'Cancel'],
  reset: ['Zurücksetzen', 'Reset'],
  delete: ['Löschen', 'Delete'],
  rename: ['Umbenennen', 'Rename'],
  load: ['Laden', 'Load'],
  loading: ['Lade…', 'Loading…'],
  saved: ['Gespeichert', 'Saved'],
  close: ['Schließen', 'Close'],
  yes: ['Ja', 'Yes'],
  no: ['Nein', 'No'],
  on: ['Ein', 'On'],
  off: ['Aus', 'Off'],
  stream_lost: ["Verbindung zum Server unterbrochen – verbinde neu …", "Connection to the server lost – reconnecting …"],
  offline_hint: ['Keine Verbindung zur Wärmepumpe.', 'Not connected to the heat pump.'],
  fault_active: ['Störung aktiv', 'Fault active'],
  fault_reset: ['Entstören', 'Reset fault'],
  heat_curve: ['Heizkurve', 'Heating curve'],
  heat_curve_save: ['Heizkurve übertragen', 'Apply heating curve'],
  heat_curve_outdoor: ['Außentemperatur', 'Outdoor temperature'],
  heat_curve_target: ['Rücklauf Soll', 'Return target'],
  heat_curve_normal: ['Normal', 'Normal'],
  heat_curve_reduced: ['Absenken', 'Reduced'],
  heat_curve_boost: ['Aufheizen', 'Boost'],
  heat_curve_now: ['Aktuell', 'Now'],
  changed: ['geändert', 'changed'],
  job_init_points: ['Datenpunkte werden initialisiert', 'Initializing points'],
  job_read_mp: ['Messpunkte werden gelesen', 'Reading measuring points'],
  job_read_sp: ['Setuppunkte werden gelesen', 'Reading setup points'],
  job_trend_download: ['Trenddaten werden gelesen', 'Reading trend data'],
  job_trend_points: ['Trendpunkte werden gelesen', 'Reading trend points'],
  job_time_programs: ['Zeitprogramme werden gelesen', 'Reading time programs'],
  // time programs
  tp_load_hint: [
    'Die Zeitprogramme werden aus dem Regler gelesen (dauert einige Sekunden).',
    'Time programs are read from the controller (takes a few seconds).',
  ],
  tp_transfer: ['Änderungen übertragen', 'Transfer changes'],
  tp_copy_day: ['Tag kopieren', 'Copy day'],
  tp_paste: ['Einfügen', 'Paste'],
  tp_export: ['Exportieren', 'Export'],
  tp_import: ['Importieren', 'Import'],
  tp_state: ['Status', 'State'],
  tp_begin: ['Beginn', 'Begin'],
  tp_end: ['Ende', 'End'],
  tp_unused: ['nicht genutzt', 'unused'],
  tp_split: ['Teilen', 'Split'],
  tp_remove: ['Entfernen', 'Remove'],
  tp_split_full: ['Alle Zeiträume des Tages sind belegt', 'All periods of the day are in use'],
  tp_gapless_hint: [
    'Der Regler speichert einen Tag lückenlos von 00:00 bis 24:00: jeder Zeitraum beginnt, wo der vorige endet. Wer den Beginn ändert, verschiebt das Ende des vorigen Zeitraums. Mit „Teilen“ kommt ein Zeitraum dazu.',
    'The controller stores a day without gaps from 00:00 to 24:00: each period begins where the previous one ends. Changing a begin moves the end of the previous period. "Split" adds a period.',
  ],
  tp_no_changes: ['Keine Änderungen.', 'No changes.'],
  tp_written: ['Übertragen', 'Transferred'],
  day_0: ['Montag', 'Monday'],
  day_1: ['Dienstag', 'Tuesday'],
  day_2: ['Mittwoch', 'Wednesday'],
  day_3: ['Donnerstag', 'Thursday'],
  day_4: ['Freitag', 'Friday'],
  day_5: ['Samstag', 'Saturday'],
  day_6: ['Sonntag', 'Sunday'],
  st2_0: ['Aus', 'Off'],
  st2_1: ['Ein', 'On'],
  st3_0: ['Normal', 'Normal'],
  st3_1: ['Absenken', 'Reduced'],
  st3_2: ['Aufheizen', 'Boost'],
  // recorder
  rec_points: ['Datenpunkte', 'Data points'],
  rec_add_points: ['Datenpunkte wählen', 'Choose data points'],
  rec_interval: ['Intervall', 'Interval'],
  rec_name: ['Name', 'Name'],
  rec_start: ['Aufnahme starten', 'Start recording'],
  rec_pause: ['Pause', 'Pause'],
  rec_resume: ['Fortsetzen', 'Resume'],
  rec_stop: ['Stoppen', 'Stop'],
  rec_recordings: ['Aufzeichnungen', 'Recordings'],
  rec_import: ['Datei importieren (.hwo / .htd)', 'Import file (.hwo / .htd)'],
  rec_csv: ['CSV', 'CSV'],
  rec_none: ['Noch keine Aufzeichnungen.', 'No recordings yet.'],
  rec_select: ['Aufzeichnung auswählen', 'Select a recording'],
  rec_running: ['Aufnahme läuft', 'Recording'],
  rec_paused: ['Pausiert', 'Paused'],
  rec_samples: ['Werte', 'values'],
  rec_source_live: ['Live', 'Live'],
  rec_source_trend: ['Trend', 'Trend'],
  rec_source_import: ['Import', 'Import'],
  rec_window: ['Zeitbereich', 'Time range'],
  rec_window_all: ['Alles', 'All'],
  rec_value_axis: ['Wertachse', 'Value axis'],
  rec_auto: ['Automatisch', 'Automatic'],
  rec_cursor: ['Cursor', 'Cursors'],
  rec_cursor_hint: ['Klick: Cursor A, Umschalt+Klick: Cursor B', 'Click: cursor A, Shift+click: cursor B'],
  rec_delta: ['Differenz', 'Difference'],
  rec_search: ['Suchen…', 'Search…'],
  rec_relative: ['Relative Zeit', 'Relative time'],
  rec_confirm_delete: ['Aufzeichnung löschen?', 'Delete recording?'],
  sec: ['Sekunden', 'seconds'],
  min: ['Minuten', 'minutes'],
  hour: ['Stunden', 'hours'],
  // trend
  trend_buffer: ['Trendspeicher im Regler', 'Trend buffer in the controller'],
  trend_entries: ['Einträge', 'entries'],
  trend_count: ['Anzahl lesen', 'Read count'],
  trend_download: ['Trenddaten lesen', 'Read trend data'],
  trend_delete_after: ['Danach im Regler löschen', 'Delete in the controller afterwards'],
  trend_points: ['Trendpunkte', 'Trend points'],
  trend_points_hint: [
    'Welche Messpunkte der Regler im Trendspeicher aufzeichnet.',
    'Which measuring points the controller records in its trend buffer.',
  ],
  trend_edit: ['Gewählte bearbeiten', 'Edit selected'],
  trend_type: ['Trendaufzeichnung', 'Trend recording'],
  trend_type_off: ['Ausschalten', 'Off'],
  trend_type_interval: ['Intervall', 'Interval'],
  trend_type_hyst: ['Hysterese', 'Hysteresis'],
  trend_interval_s: ['Intervall (s)', 'Interval (s)'],
  trend_hyst: ['Hysterese', 'Hysteresis'],
  trend_end: ['Aufzeichnungsende', 'Recording end'],
  trend_alarm: ['Alarm', 'Alarm'],
  trend_call: ['Fernwartung anrufen', 'Call remote service'],
  trend_downloaded: ['Gelesen – in WEB-Online öffnen', 'Read – open in WEB-Online'],
  // points
  points_mp: ['Messpunkte', 'Measuring points'],
  points_sp: ['Setuppunkte', 'Setup points'],
  points_read: ['Aus Regler lesen', 'Read from controller'],
  points_show_unused: ['Ungenutzte anzeigen', 'Show unused'],
  points_value: ['Wert', 'Value'],
  points_name: ['Name', 'Name'],
  points_limits: ['Grenzen', 'Limits'],
  points_flags: ['Eigenschaften', 'Properties'],
  points_used: ['genutzt', 'used'],
  points_writable: ['schreibbar', 'writable'],
  points_empty: [
    'Noch keine Datenpunkte bekannt. „Aus Regler lesen" lädt Namen und Grenzen aller Punkte.',
    'No points known yet. "Read from controller" loads names and limits of all points.',
  ],
  points_expert_hint: [
    'Expertenmodus: Setuppunkte können direkt geändert werden. Falsche Werte können die Anlage beschädigen.',
    'Expert mode: setup points can be changed directly. Wrong values can damage the installation.',
  ],
  // log
  log_clear: ['Protokoll leeren', 'Clear log'],
  log_empty: ['Keine Meldungen.', 'No messages.'],
  // settings
  set_connection: ['Verbindung', 'Connection'],
  set_type: ['Schnittstelle', 'Interface'],
  set_type_tcp: ['LAN-Modem (TCP)', 'LAN modem (TCP)'],
  set_type_serial: ['Seriell (RS232)', 'Serial (RS232)'],
  set_host: ['Adresse', 'Address'],
  set_port: ['Port', 'Port'],
  set_serial_port: ['Anschluss', 'Port'],
  set_baud: ['Baudrate', 'Baud rate'],
  set_expected_id: ['Regler-Nummer prüfen (0 = aus)', 'Check controller number (0 = off)'],
  set_auto_connect: ['Beim Start verbinden', 'Connect on start'],
  set_poll: ['Aktualisierung alle … Sekunden', 'Refresh every … seconds'],
  set_safety: ['Sicherheit', 'Safety'],
  set_read_only: ['Nur-Lese-Modus (nichts an die Wärmepumpe schreiben)', 'Read-only mode (never write to the heat pump)'],
  set_read_only_off_confirm: [
    'Nur-Lese-Modus ausschalten? Danach können Einstellungen der Wärmepumpe geändert werden.',
    'Turn off read-only mode? Heat pump settings can then be changed.',
  ],
  set_expert: ['Expertenmodus (alle schreibbaren Setuppunkte ändern)', 'Expert mode (change any writable setup point)'],
  set_trend_pause: [
    'Trendaufzeichnung während des Lesens anhalten',
    'Pause trend recording while reading the buffer',
  ],
  set_mqtt: ['Home Assistant (MQTT)', 'Home Assistant (MQTT)'],
  set_mqtt_enabled: ['MQTT aktivieren', 'Enable MQTT'],
  set_mqtt_broker: ['Broker', 'Broker'],
  set_mqtt_user: ['Benutzer', 'Username'],
  set_mqtt_password: ['Passwort', 'Password'],
  set_mqtt_password_keep: ['(unverändert)', '(unchanged)'],
  set_mqtt_discovery: ['Discovery-Präfix', 'Discovery prefix'],
  set_mqtt_topic: ['Topic-Präfix', 'Topic prefix'],
  set_mqtt_hint: [
    'Die Wärmepumpe erscheint in Home Assistant automatisch als Gerät „Heliotherm".',
    'The heat pump appears in Home Assistant automatically as device "Heliotherm".',
  ],
  set_password: ['Zugangsschutz', 'Access protection'],
  set_password_current: ['Aktuelles Passwort', 'Current password'],
  set_password_new: ['Neues Passwort (leer = Schutz aus)', 'New password (empty = no protection)'],
  set_password_set: ['Passwort setzen', 'Set password'],
  set_clock: ['Uhrzeit', 'Clock'],
  set_clock_read: ['Uhrzeit lesen', 'Read clock'],
  set_clock_sync: ['Zeit synchronisieren', 'Synchronize time'],
  set_clock_device: ['Regler', 'Controller'],
  set_clock_server: ['Server', 'Server'],
  set_clock_diff: ['Abweichung', 'Difference'],
  set_points: ['Datenpunkte', 'Data points'],
  set_points_init: ['Datenpunkte initialisieren', 'Initialize data points'],
  set_points_hint: [
    'Liest Namen und Grenzen aller Punkte aus dem Regler (für Beschriftungen und Prüfungen).',
    'Reads names and limits of all points from the controller (for labels and checks).',
  ],
  set_appearance: ['Darstellung', 'Appearance'],
  set_language: ['Sprache', 'Language'],
  set_theme: ['Design', 'Theme'],
  set_theme_auto: ['System', 'System'],
  set_theme_light: ['Hell', 'Light'],
  set_theme_dark: ['Dunkel', 'Dark'],
  // temporary setpoints
  tmp_title: ['Vorübergehende Werte', 'Temporary values'],
  tmp_set: ['Vorübergehend', 'Temporarily'],
  tmp_for: ['für', 'for'],
  tmp_minutes: ['Minuten', 'minutes'],
  tmp_until: ['bis', 'until'],
  tmp_then: ['danach', 'then'],
  tmp_end: ['Beenden', 'End'],
  tmp_apply: ['Setzen', 'Apply'],
  tmp_hint: [
    'Der vorherige Wert wird automatisch wiederhergestellt – auch nach einem Neustart.',
    'The previous value is restored automatically – also after a restart.',
  ],
  // insights
  nav_insights: ['Auswertung', 'Insights'],
  ins_hints: ['Hinweise', 'Hints'],
  ins_no_hints: ['Keine Auffälligkeiten in den letzten Tagen.', 'Nothing remarkable in the last days.'],
  ins_collecting: [
    'Die Auswertung braucht Verlaufsdaten. heliocontrol zeichnet laufend auf, aussagekräftig wird es nach ein bis zwei Tagen.',
    'Insights need history. heliocontrol records continuously; the numbers become meaningful after a day or two.',
  ],
  ins_days: ['Tage', 'Days'],
  ins_date: ['Tag', 'Day'],
  ins_starts: ['Starts', 'Starts'],
  ins_runtime: ['Laufzeit', 'Runtime'],
  ins_cycle: ['Ø Zyklus', 'Avg. cycle'],
  ins_outdoor: ['Ø Außen', 'Avg. outdoor'],
  ins_flow: ['Vorlauf*', 'Flow*'],
  ins_return: ['Rücklauf*', 'Return*'],
  ins_spread: ['Spreizung*', 'Spread*'],
  ins_deviation: ['Abw. Soll*', 'Dev. target*'],
  ins_running_note: ['* Mittelwerte bei laufendem Verdichter', '* averages while the compressor runs'],
  ins_counter: ['aus Schaltzähler des Reglers', 'from the controller\'s start counter'],
  ins_coverage: ['Datenabdeckung', 'Data coverage'],
  ins_history: ['Verlauf', 'History'],
  ins_dhw: ['Warmwasser', 'Hot water'],
  ins_dhw_interrupts: ['Unterbrechungen der Heizung durch Warmwasser', 'Heating interrupted by hot water'],
  ins_cop: ['AZ Heizen', 'SPF heating'],
  ins_tips: ['Erfahrungen aus der Community', 'Community experience'],
  ins_tips_note: [
    'Forenberichte anderer Heliotherm-Besitzer, keine Herstellerangaben. Parameter im Servicemenü vom Fachbetrieb ändern lassen.',
    'Forum reports from other Heliotherm owners, not manufacturer advice. Have the installer change service-menu parameters.',
  ],
  ins_source: ['Quelle', 'Source'],
  set_write_budget: ['Max. Änderungen je Setuppunkt und Tag (−1 = unbegrenzt)', 'Max. changes per setup point and day (−1 = unlimited)'],
  set_history_days: ['Verlauf aufbewahren (Tage)', 'Keep history (days)'],
  set_mqtt_language: ['Sprache in Home Assistant', 'Language in Home Assistant'],
  login_title: ['Anmelden', 'Log in'],
  login_password: ['Passwort', 'Password'],
} satisfies Record<string, [string, string]>;

export type Key = keyof typeof dict;

@Injectable({ providedIn: 'root' })
export class I18n {
  readonly lang = signal<Lang>(I18n.initial());

  private static initial(): Lang {
    try {
      const v = localStorage.getItem('lang');
      if (v === 'de' || v === 'en') return v;
    } catch {
      /* storage unavailable */
    }
    return navigator.language.toLowerCase().startsWith('de') ? 'de' : 'en';
  }

  setLang(l: Lang) {
    this.lang.set(l);
    document.documentElement.lang = l;
    try {
      localStorage.setItem('lang', l);
    } catch {
      /* ignore */
    }
  }

  t(key: Key): string {
    return dict[key][this.lang() === 'de' ? 0 : 1];
  }

  text(t: Text | undefined | null): string {
    if (!t) return '';
    return this.lang() === 'de' ? t.de : t.en;
  }

  locale(): string {
    return this.lang() === 'de' ? 'de-DE' : 'en-GB';
  }
}

/** {{ 'key' | t }} - impure so it follows language changes. */
@Pipe({ name: 't', pure: false })
export class TPipe implements PipeTransform {
  private i18n = inject(I18n);
  transform(key: Key): string {
    return this.i18n.t(key);
  }
}

/** {{ text | tx }} for server-provided {de, en} labels. */
@Pipe({ name: 'tx', pure: false })
export class TxPipe implements PipeTransform {
  private i18n = inject(I18n);
  transform(t: Text | undefined | null): string {
    return this.i18n.text(t);
  }
}
