// Package legacy reads files saved by homeControl: trend data (*.htd, a
// DS_TrendData DataSet) and WEB-Online recordings (*.hwo, a DS_Heliograph
// DataSet). Both are .NET DataSet.WriteXml documents: one element per row,
// named after the table, with one child element per column.
package legacy

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"
	"time"

	"heliocontrol/internal/protocol"
	"heliocontrol/internal/store"
)

// parseTime accepts the xs:dateTime forms DataSet writes (with or without
// fraction / zone offset). Times without zone are local.
func parseTime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05.9999999", "2006-01-02T15:04:05"} {
		loc := time.Local
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("bad timestamp %q", s)
}

type trendFile struct {
	Rows []struct {
		ID        int     `xml:"ID"`
		MPID      int     `xml:"MP_ID"`
		Timestamp string  `xml:"Timestamp"`
		Value     float64 `xml:"Value"`
		Reason    int     `xml:"Reason"`
	} `xml:"TrendDataLog"`
}

// ReadTrend parses a homeControl trend file (*.htd).
func ReadTrend(r io.Reader) ([]protocol.TrendRecord, error) {
	var f trendFile
	if err := xml.NewDecoder(r).Decode(&f); err != nil {
		return nil, fmt.Errorf("not a homeControl trend file: %w", err)
	}
	out := make([]protocol.TrendRecord, 0, len(f.Rows))
	for _, row := range f.Rows {
		t, err := parseTime(row.Timestamp)
		if err != nil {
			return nil, err
		}
		out = append(out, protocol.TrendRecord{ID: row.ID, MPID: row.MPID, Time: t, Value: row.Value, Reason: row.Reason})
	}
	return out, nil
}

type heliographFile struct {
	Points []struct {
		PointID int     `xml:"PointID"`
		Name    string  `xml:"Name"`
		Min     float64 `xml:"Min"`
		Max     float64 `xml:"Max"`
		Color   int64   `xml:"Color"`
	} `xml:"RecordingPointsTemp"`
	Values []struct {
		PointID   int     `xml:"PointID"`
		TimeInSec int64   `xml:"TimeInSec"`
		Value     float64 `xml:"Value"`
	} `xml:"RecordedValuesTemp"`
	Props []struct {
		Title string `xml:"Title"`
		Start string `xml:"Start"`
	} `xml:"RecorderProperties"`
}

// ReadRecording parses a WEB-Online recording (*.hwo). Sample times are
// stored relative to the recording start.
func ReadRecording(r io.Reader, name string) (store.Recording, []store.Sample, error) {
	var f heliographFile
	if err := xml.NewDecoder(r).Decode(&f); err != nil {
		return store.Recording{}, nil, fmt.Errorf("not a homeControl WEB-Online file: %w", err)
	}
	rec := store.Recording{Name: name, Started: time.Now()}
	if len(f.Props) > 0 {
		if f.Props[0].Title != "" {
			rec.Name = f.Props[0].Title
		}
		if t, err := parseTime(f.Props[0].Start); err == nil {
			rec.Started = t
		}
	}
	for _, p := range f.Points {
		c := uint32(p.Color) // .NET ARGB
		rec.Points = append(rec.Points, store.RecordingPoint{ID: p.PointID, Name: p.Name,
			Color: fmt.Sprintf("#%02x%02x%02x", (c>>16)&0xff, (c>>8)&0xff, c&0xff)})
	}
	samples := make([]store.Sample, 0, len(f.Values))
	var last int64
	for _, v := range f.Values {
		samples = append(samples, store.Sample{PointID: v.PointID, Time: rec.Started.Add(time.Duration(v.TimeInSec) * time.Second), Value: v.Value})
		last = max(last, v.TimeInSec)
	}
	end := rec.Started.Add(time.Duration(last) * time.Second)
	rec.Ended = &end
	return rec, samples, nil
}
