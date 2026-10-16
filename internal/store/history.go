package store

import (
	"context"
	"fmt"
	"strings"
	"time"

	"heliocontrol/internal/protocol"
)

// The continuous history stores point values on change (with a periodic
// heartbeat, see service/history.go), so a value is valid from its timestamp
// until the next row of the same point.

const historySchema = `
CREATE TABLE IF NOT EXISTS history (
	kind  TEXT NOT NULL,
	id    INTEGER NOT NULL,
	t     INTEGER NOT NULL,
	value REAL NOT NULL
);
CREATE INDEX IF NOT EXISTS history_by_point ON history (kind, id, t);
CREATE INDEX IF NOT EXISTS history_by_time ON history (t);
`

// HistoryRow is one stored value.
type HistoryRow struct {
	Kind  protocol.Kind
	ID    int
	Time  time.Time
	Value float64
}

func (s *Store) AddHistory(ctx context.Context, rows []HistoryRow) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO history (kind, id, t, value) VALUES (?,?,?,?)`)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := stmt.ExecContext(ctx, r.Kind, r.ID, r.Time.UnixMilli(), r.Value); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// PruneHistory deletes rows older than before.
func (s *Store) PruneHistory(ctx context.Context, before time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM history WHERE t < ?`, before.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// PointRef names a point in history queries ("mp:3", "sp:10").
type PointRef struct {
	Kind protocol.Kind `json:"kind"`
	ID   int           `json:"id"`
}

func (p PointRef) String() string { return fmt.Sprintf("%s:%d", p.Kind, p.ID) }

// ParsePointRef parses "mp:3" / "sp:10".
func ParsePointRef(s string) (PointRef, error) {
	k, id, ok := strings.Cut(strings.TrimSpace(s), ":")
	var p PointRef
	if !ok || (k != string(protocol.MP) && k != string(protocol.SP)) {
		return p, fmt.Errorf("bad point %q (want mp:N or sp:N)", s)
	}
	if _, err := fmt.Sscan(id, &p.ID); err != nil {
		return p, fmt.Errorf("bad point %q", s)
	}
	p.Kind = protocol.Kind(k)
	return p, nil
}

// HistoryPoints returns the raw rows of one point in [from, to), plus the last
// row before from (the value valid at from).
func (s *Store) HistoryPoints(ctx context.Context, p PointRef, from, to time.Time) ([]HistoryRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT t, value FROM (SELECT t, value FROM history WHERE kind = ? AND id = ? AND t < ? ORDER BY t DESC LIMIT 1)
		UNION ALL
		SELECT t, value FROM history WHERE kind = ? AND id = ? AND t >= ? AND t < ?
		ORDER BY t`, p.Kind, p.ID, from.UnixMilli(), p.Kind, p.ID, from.UnixMilli(), to.UnixMilli())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryRow
	for rows.Next() {
		var t int64
		r := HistoryRow{Kind: p.Kind, ID: p.ID}
		if err := rows.Scan(&t, &r.Value); err != nil {
			return nil, err
		}
		r.Time = time.UnixMilli(t)
		out = append(out, r)
	}
	return out, rows.Err()
}

// HistorySeries returns a point's values in [from, to), averaged into at most
// maxPoints buckets so long ranges stay small.
func (s *Store) HistorySeries(ctx context.Context, p PointRef, from, to time.Time, maxPoints int) (Series, error) {
	bucket := max(to.Sub(from).Milliseconds()/int64(max(maxPoints, 1)), 60_000)
	rows, err := s.db.QueryContext(ctx, `
		SELECT (t / ?) * ? AS b, AVG(value) FROM history
		WHERE kind = ? AND id = ? AND t >= ? AND t < ?
		GROUP BY b ORDER BY b`, bucket, bucket, p.Kind, p.ID, from.UnixMilli(), to.UnixMilli())
	if err != nil {
		return Series{}, err
	}
	defer rows.Close()
	ser := Series{PointID: p.ID}
	for rows.Next() {
		var t int64
		var v float64
		if err := rows.Scan(&t, &v); err != nil {
			return ser, err
		}
		ser.T = append(ser.T, t)
		ser.V = append(ser.V, v)
	}
	return ser, rows.Err()
}
