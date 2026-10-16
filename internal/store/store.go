// Package store persists the message log, the point name cache and recorded
// time series (live WEB-Online recordings, downloaded trend buffers and
// imported files) in SQLite.
package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"heliocontrol/internal/protocol"
)

type Store struct{ db *sql.DB }

const schema = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;
CREATE TABLE IF NOT EXISTS events (
	id     INTEGER PRIMARY KEY,
	ts     INTEGER NOT NULL,
	level  TEXT NOT NULL,
	title  TEXT NOT NULL,
	detail TEXT NOT NULL DEFAULT ''
);
CREATE TABLE IF NOT EXISTS points (
	kind     TEXT NOT NULL,
	id       INTEGER NOT NULL,
	name     TEXT NOT NULL,
	min      REAL NOT NULL,
	max      REAL NOT NULL,
	used     INTEGER NOT NULL,
	writable INTEGER NOT NULL,
	PRIMARY KEY (kind, id)
);
CREATE TABLE IF NOT EXISTS recordings (
	id          INTEGER PRIMARY KEY,
	name        TEXT NOT NULL,
	source      TEXT NOT NULL,
	state       TEXT NOT NULL,
	started     INTEGER NOT NULL,
	ended       INTEGER,
	interval_ms INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS recording_points (
	recording_id INTEGER NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
	point_id     INTEGER NOT NULL,
	name         TEXT NOT NULL,
	unit         TEXT NOT NULL DEFAULT '',
	color        TEXT NOT NULL DEFAULT '',
	PRIMARY KEY (recording_id, point_id)
);
CREATE TABLE IF NOT EXISTS samples (
	recording_id INTEGER NOT NULL REFERENCES recordings(id) ON DELETE CASCADE,
	point_id     INTEGER NOT NULL,
	t            INTEGER NOT NULL,
	value        REAL NOT NULL,
	reason       INTEGER
);
CREATE INDEX IF NOT EXISTS samples_by_recording ON samples (recording_id, t);
CREATE TABLE IF NOT EXISTS overrides (
	sp_id    INTEGER PRIMARY KEY,
	value    REAL NOT NULL,
	original REAL NOT NULL,
	until    INTEGER NOT NULL,
	created  INTEGER NOT NULL,
	source   TEXT NOT NULL DEFAULT ''
);
`

func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema + historySchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// --- message log -------------------------------------------------------------

type Event struct {
	ID     int64     `json:"id"`
	Time   time.Time `json:"time"`
	Level  string    `json:"level"` // info, warn, error, connect, disconnect
	Title  string    `json:"title"`
	Detail string    `json:"detail,omitempty"`
}

func (s *Store) AddEvent(ctx context.Context, e *Event) error {
	if e.Time.IsZero() {
		e.Time = time.Now()
	}
	res, err := s.db.ExecContext(ctx, `INSERT INTO events (ts, level, title, detail) VALUES (?,?,?,?)`, e.Time.UnixMilli(), e.Level, e.Title, e.Detail)
	if err != nil {
		return err
	}
	e.ID, _ = res.LastInsertId()
	// keep the log bounded
	_, _ = s.db.ExecContext(ctx, `DELETE FROM events WHERE id <= ? - 5000`, e.ID)
	return nil
}

func (s *Store) Events(ctx context.Context, limit int) ([]Event, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, ts, level, title, detail FROM events ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var ts int64
		if err := rows.Scan(&e.ID, &ts, &e.Level, &e.Title, &e.Detail); err != nil {
			return nil, err
		}
		e.Time = time.UnixMilli(ts)
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) ClearEvents(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM events`)
	return err
}

// --- point cache ------------------------------------------------------------

// SavePoints caches point metadata (names, limits) so data can be labelled
// while offline (homeControl's "initialize measuring points").
func (s *Store) SavePoints(ctx context.Context, pts []protocol.Point) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO points (kind, id, name, min, max, used, writable) VALUES (?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	for _, p := range pts {
		if _, err := stmt.ExecContext(ctx, p.Kind, p.ID, p.Name, p.Min, p.Max, p.US, p.WR); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Points(ctx context.Context, kind protocol.Kind) ([]protocol.Point, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, min, max, used, writable FROM points WHERE kind = ? ORDER BY id`, kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []protocol.Point{}
	for rows.Next() {
		p := protocol.Point{Kind: kind}
		if err := rows.Scan(&p.ID, &p.Name, &p.Min, &p.Max, &p.US, &p.WR); err != nil {
			return nil, err
		}
		p.NR = p.ID
		out = append(out, p)
	}
	return out, rows.Err()
}

// --- temporary setpoints ------------------------------------------------------

// Override is a temporary setup point value that is restored automatically.
type Override struct {
	SP       int       `json:"sp"`
	Value    float64   `json:"value"`
	Original float64   `json:"original"`
	Until    time.Time `json:"until"`
	Created  time.Time `json:"created"`
	Source   string    `json:"source,omitempty"`
}

func (s *Store) SaveOverride(ctx context.Context, o Override) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR REPLACE INTO overrides (sp_id, value, original, until, created, source) VALUES (?,?,?,?,?,?)`,
		o.SP, o.Value, o.Original, o.Until.UnixMilli(), o.Created.UnixMilli(), o.Source)
	return err
}

func (s *Store) DeleteOverride(ctx context.Context, sp int) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM overrides WHERE sp_id = ?`, sp)
	return err
}

func (s *Store) Overrides(ctx context.Context) ([]Override, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT sp_id, value, original, until, created, source FROM overrides ORDER BY until`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Override
	for rows.Next() {
		var o Override
		var until, created int64
		if err := rows.Scan(&o.SP, &o.Value, &o.Original, &until, &created, &o.Source); err != nil {
			return nil, err
		}
		o.Until, o.Created = time.UnixMilli(until), time.UnixMilli(created)
		out = append(out, o)
	}
	return out, rows.Err()
}

// --- recordings ---------------------------------------------------------------

type RecordingPoint struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Unit  string `json:"unit"`
	Color string `json:"color"`
}

type Recording struct {
	ID         int64            `json:"id"`
	Name       string           `json:"name"`
	Source     string           `json:"source"` // live, trend, import
	State      string           `json:"state"`  // recording, paused, stopped
	Started    time.Time        `json:"started"`
	Ended      *time.Time       `json:"ended,omitempty"`
	IntervalMS int64            `json:"intervalMs"`
	Points     []RecordingPoint `json:"points"`
	Samples    int64            `json:"samples"`
}

var ErrNotFound = errors.New("not found")

func (s *Store) CreateRecording(ctx context.Context, r *Recording) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var ended any
	if r.Ended != nil {
		ended = r.Ended.UnixMilli()
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO recordings (name, source, state, started, ended, interval_ms) VALUES (?,?,?,?,?,?)`,
		r.Name, r.Source, r.State, r.Started.UnixMilli(), ended, r.IntervalMS)
	if err != nil {
		return err
	}
	r.ID, _ = res.LastInsertId()
	for _, p := range r.Points {
		if _, err := tx.ExecContext(ctx, `INSERT OR REPLACE INTO recording_points (recording_id, point_id, name, unit, color) VALUES (?,?,?,?,?)`,
			r.ID, p.ID, p.Name, p.Unit, p.Color); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) SetRecordingState(ctx context.Context, id int64, state string, ended *time.Time) error {
	var e any
	if ended != nil {
		e = ended.UnixMilli()
	}
	_, err := s.db.ExecContext(ctx, `UPDATE recordings SET state = ?, ended = COALESCE(?, ended) WHERE id = ?`, state, e, id)
	return err
}

// StopDangling marks recordings left "recording"/"paused" by an earlier run
// (server stopped or crashed) as stopped, ending at their last sample.
func (s *Store) StopDangling(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE recordings SET state = 'stopped',
		ended = COALESCE(ended, (SELECT MAX(t) FROM samples WHERE recording_id = recordings.id), started)
		WHERE state != 'stopped'`)
	return err
}

func (s *Store) RenameRecording(ctx context.Context, id int64, name string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE recordings SET name = ? WHERE id = ?`, name, id)
	return err
}

func (s *Store) DeleteRecording(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM recordings WHERE id = ?`, id)
	return err
}

func (s *Store) Recordings(ctx context.Context) ([]Recording, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id, r.name, r.source, r.state, r.started, r.ended, r.interval_ms,
		(SELECT COUNT(*) FROM samples WHERE recording_id = r.id) FROM recordings r ORDER BY r.started DESC`)
	if err != nil {
		return nil, err
	}
	var out []Recording
	for rows.Next() {
		var r Recording
		var started int64
		var ended sql.NullInt64
		if err := rows.Scan(&r.ID, &r.Name, &r.Source, &r.State, &started, &ended, &r.IntervalMS, &r.Samples); err != nil {
			rows.Close()
			return nil, err
		}
		r.Started = time.UnixMilli(started)
		if ended.Valid {
			t := time.UnixMilli(ended.Int64)
			r.Ended = &t
		}
		out = append(out, r)
	}
	rows.Close()
	for i := range out {
		if out[i].Points, err = s.recordingPoints(ctx, out[i].ID); err != nil {
			return nil, err
		}
	}
	if out == nil {
		out = []Recording{}
	}
	return out, nil
}

func (s *Store) Recording(ctx context.Context, id int64) (Recording, error) {
	var r Recording
	var started int64
	var ended sql.NullInt64
	err := s.db.QueryRowContext(ctx, `SELECT id, name, source, state, started, ended, interval_ms,
		(SELECT COUNT(*) FROM samples WHERE recording_id = recordings.id) FROM recordings WHERE id = ?`, id).
		Scan(&r.ID, &r.Name, &r.Source, &r.State, &started, &ended, &r.IntervalMS, &r.Samples)
	if errors.Is(err, sql.ErrNoRows) {
		return r, ErrNotFound
	}
	if err != nil {
		return r, err
	}
	r.Started = time.UnixMilli(started)
	if ended.Valid {
		t := time.UnixMilli(ended.Int64)
		r.Ended = &t
	}
	r.Points, err = s.recordingPoints(ctx, id)
	return r, err
}

func (s *Store) recordingPoints(ctx context.Context, id int64) ([]RecordingPoint, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT point_id, name, unit, color FROM recording_points WHERE recording_id = ? ORDER BY point_id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RecordingPoint{}
	for rows.Next() {
		var p RecordingPoint
		if err := rows.Scan(&p.ID, &p.Name, &p.Unit, &p.Color); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type Sample struct {
	PointID int       `json:"p"`
	Time    time.Time `json:"-"`
	Value   float64   `json:"v"`
	Reason  *int      `json:"r,omitempty"`
}

func (s *Store) AddSamples(ctx context.Context, recordingID int64, samples []Sample) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO samples (recording_id, point_id, t, value, reason) VALUES (?,?,?,?,?)`)
	if err != nil {
		return err
	}
	for _, x := range samples {
		if _, err := stmt.ExecContext(ctx, recordingID, x.PointID, x.Time.UnixMilli(), x.Value, x.Reason); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Series is the column-oriented sample data of one point.
type Series struct {
	PointID int       `json:"pointId"`
	T       []int64   `json:"t"` // unix ms
	V       []float64 `json:"v"`
}

func (s *Store) Samples(ctx context.Context, recordingID int64, from, to time.Time) ([]Series, error) {
	q := `SELECT point_id, t, value FROM samples WHERE recording_id = ?`
	args := []any{recordingID}
	if !from.IsZero() {
		q += ` AND t >= ?`
		args = append(args, from.UnixMilli())
	}
	if !to.IsZero() {
		q += ` AND t <= ?`
		args = append(args, to.UnixMilli())
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY point_id, t`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Series{}
	for rows.Next() {
		var id int
		var t int64
		var v float64
		if err := rows.Scan(&id, &t, &v); err != nil {
			return nil, err
		}
		if len(out) == 0 || out[len(out)-1].PointID != id {
			out = append(out, Series{PointID: id})
		}
		last := &out[len(out)-1]
		last.T = append(last.T, t)
		last.V = append(last.V, v)
	}
	return out, rows.Err()
}
