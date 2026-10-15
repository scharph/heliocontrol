package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"heliocontrol/internal/catalog"
	"heliocontrol/internal/device"
	"heliocontrol/internal/protocol"
	"heliocontrol/internal/store"
)

// ValidationError is returned for requests that are refused before anything
// is sent to the heat pump.
type ValidationError struct{ Msg string }

func (e ValidationError) Error() string { return e.Msg }

func invalid(format string, a ...any) error { return ValidationError{fmt.Sprintf(format, a...)} }

// writableClient returns the session if writing is allowed.
func (s *Service) writableClient() (*device.Client, error) {
	if s.cfg.Get().IsReadOnly() {
		return nil, ErrReadOnly
	}
	return s.Client()
}

// WriteSP changes a setup point. Points on the UI pages may always be changed;
// any other writable point only in expert mode. Values are checked against the
// limits the controller reports.
func (s *Service) WriteSP(ctx context.Context, id int, v float64) (float64, error) {
	c, err := s.writableClient()
	if err != nil {
		return 0, err
	}
	s.mu.RLock()
	item, onPage := catalog.FindWritable(s.snap.Panels, id)
	meta, known := s.meta[protocol.SP][id]
	s.mu.RUnlock()
	switch {
	case onPage:
		if item.Display == catalog.Enum && slices.Contains(item.NotSelectable, int(v)) {
			return 0, invalid("value %v cannot be selected for %s", v, item.Label.EN)
		}
		if item.Max > item.Min && (v < item.Min || v > item.Max) {
			return 0, invalid("value %v outside %v..%v", v, item.Min, item.Max)
		}
	case s.cfg.Get().ExpertMode && known && meta.WR:
	default:
		return 0, invalid("setup point %d may not be changed (enable expert mode for points outside the pages)", id)
	}
	if known && meta.Max > meta.Min && (v < meta.Min || v > meta.Max) {
		return 0, invalid("value %v outside the controller limits %v..%v", v, meta.Min, meta.Max)
	}
	if known && !meta.WR {
		return 0, invalid("setup point %d is read-only on the controller", id)
	}
	got, err := c.WriteSP(ctx, id, v)
	if err != nil {
		s.event("error", "Einstellung nicht gespeichert", fmt.Sprintf("SP %d = %v: %v", id, v, err))
		return 0, err
	}
	s.mu.Lock()
	s.snap.SP[id] = got
	s.mu.Unlock()
	name := meta.Name
	if onPage {
		name = item.Label.DE
	}
	s.event("info", "Einstellung gespeichert", fmt.Sprintf("%s (SP %d) = %v", name, id, got))
	s.Bus.Publish(Event{Type: "values", Data: s.Values()})
	if slices.Contains(catalog.ConfigPoints, id) {
		go s.Reconnect() // installation changed: pages must be rebuilt
	}
	return got, nil
}

// ResetFault acknowledges an active fault ("Entstören", SP 14 = 1).
func (s *Service) ResetFault(ctx context.Context) error {
	c, err := s.writableClient()
	if err != nil {
		return err
	}
	if err := c.ResetFault(ctx); err != nil {
		s.event("error", "Störung konnte nicht quittiert werden", err.Error())
		return err
	}
	s.event("info", "Störung quittiert", "")
	go s.Refresh(context.WithoutCancel(ctx))
	return nil
}

// Override forces a measuring point to a manual value (expert mode only).
func (s *Service) Override(ctx context.Context, id int, v float64, auto bool) error {
	if !s.cfg.Get().ExpertMode {
		return invalid("overrides require expert mode")
	}
	c, err := s.writableClient()
	if err != nil {
		return err
	}
	if err := c.SetOverride(ctx, id, v, auto); err != nil {
		return err
	}
	if auto {
		s.event("info", "Handbetrieb aufgehoben", fmt.Sprintf("MP %d", id))
	} else {
		s.event("warn", "Messpunkt übersteuert", fmt.Sprintf("MP %d = %v", id, v))
	}
	return nil
}

type ClockInfo struct {
	Device time.Time `json:"device"`
	Server time.Time `json:"server"`
	Offset float64   `json:"offsetSeconds"` // device - server
}

func (s *Service) Clock(ctx context.Context) (ClockInfo, error) {
	c, err := s.Client()
	if err != nil {
		return ClockInfo{}, err
	}
	t, err := c.ReadClock(ctx)
	if err != nil {
		return ClockInfo{}, err
	}
	now := time.Now()
	return ClockInfo{Device: t, Server: now, Offset: t.Sub(now).Seconds()}, nil
}

// SyncClock sets the controller clock to the server time.
func (s *Service) SyncClock(ctx context.Context) error {
	c, err := s.writableClient()
	if err != nil {
		return err
	}
	if err := c.SetClock(ctx, time.Now()); err != nil {
		s.event("error", "Zeit konnte nicht synchronisiert werden", err.Error())
		return err
	}
	s.event("info", "Zeit synchronisiert", time.Now().Format("02.01.2006 15:04:05"))
	return nil
}

// --- time programs ----------------------------------------------------------

type TimeProgram struct {
	protocol.TimeProgram
	Events []protocol.TimeProgramEvent `json:"events"`
}

func (s *Service) TimePrograms(ctx context.Context) ([]TimeProgram, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	var out []TimeProgram
	err = s.runJob(ctx, "time-programs", func(progress func(int, int)) error {
		tps, err := c.TimePrograms(ctx)
		if err != nil {
			return err
		}
		out = nil
		for i, tp := range tps {
			evs, err := c.TimeProgramEvents(ctx, tp)
			if err != nil {
				return err
			}
			out = append(out, TimeProgram{TimeProgram: tp, Events: evs})
			progress(i+1, len(tps))
		}
		return nil
	})
	return out, err
}

// ValidateDay checks that a day's events cover 00:00-24:00 without gaps, in
// steps of the program's step size (unused trailing events are 24:00-24:00).
func ValidateDay(tp protocol.TimeProgram, evs []protocol.TimeProgramEvent) error {
	if len(evs) != tp.EAD {
		return invalid("day needs %d events, got %d", tp.EAD, len(evs))
	}
	step := protocol.ClockTime(max(tp.STE, 1))
	var prev protocol.ClockTime
	for i, ev := range evs {
		if ev.Event != i {
			return invalid("events must be numbered 0..%d in order", tp.EAD-1)
		}
		if ev.State < 0 || ev.State >= max(tp.NOS, 2) {
			return invalid("event %d: state %d out of range", i, ev.State)
		}
		if ev.Begin != prev || ev.End < ev.Begin || ev.End > 1440 {
			return invalid("event %d: %s-%s does not continue the previous period", i, ev.Begin, ev.End)
		}
		if ev.End%step != 0 && ev.End != 1440 {
			return invalid("event %d: end %s is not on the %d-minute grid", i, ev.End, step)
		}
		prev = ev.End
	}
	if prev != 1440 {
		return invalid("the last event must end at 24:00")
	}
	return nil
}

// SaveTimeProgram writes the events that differ from the controller's state.
func (s *Service) SaveTimeProgram(ctx context.Context, id int, events []protocol.TimeProgramEvent) (int, error) {
	c, err := s.writableClient()
	if err != nil {
		return 0, err
	}
	tps, err := c.TimePrograms(ctx)
	if err != nil {
		return 0, err
	}
	i := slices.IndexFunc(tps, func(tp protocol.TimeProgram) bool { return tp.ID == id })
	if i < 0 {
		return 0, invalid("unknown time program %d", id)
	}
	tp := tps[i]
	byDay := map[int][]protocol.TimeProgramEvent{}
	for _, ev := range events {
		if ev.Program != id || ev.Day < 0 || ev.Day >= tp.NOD {
			return 0, invalid("event for wrong program or day: %+v", ev)
		}
		byDay[ev.Day] = append(byDay[ev.Day], ev)
	}
	for day, evs := range byDay {
		slices.SortFunc(evs, func(a, b protocol.TimeProgramEvent) int { return a.Event - b.Event })
		if err := ValidateDay(tp, evs); err != nil {
			return 0, fmt.Errorf("day %d: %w", day, err)
		}
	}
	current, err := c.TimeProgramEvents(ctx, tp)
	if err != nil {
		return 0, err
	}
	written := 0
	for _, ev := range events {
		k := slices.IndexFunc(current, func(o protocol.TimeProgramEvent) bool { return o.Day == ev.Day && o.Event == ev.Event })
		if k >= 0 && current[k] == ev {
			continue
		}
		if err := c.WriteTimeProgramEvent(ctx, ev); err != nil {
			s.event("error", "Zeitprogramm nicht vollständig gespeichert", fmt.Sprintf("%s: %v", tp.Name, err))
			return written, err
		}
		written++
	}
	s.event("info", "Zeitprogramm gespeichert", fmt.Sprintf("%s: %d Änderungen übertragen", tp.Name, written))
	return written, nil
}

// --- trend buffer -------------------------------------------------------------

// TrendPoint is a measuring point that can be recorded in the controller's trend buffer.
type TrendPoint struct {
	ID     int                  `json:"id"`
	Name   string               `json:"name"`
	Min    float64              `json:"min"`
	Max    float64              `json:"max"`
	Config protocol.TrendConfig `json:"config"`
}

func trendConfig(p protocol.Point) protocol.TrendConfig {
	return protocol.TrendConfig{Alarm: p.ERF, CallRemote: p.RT1, Enabled: p.TRF, Type: p.TRT, Hysteresis: p.TRHV, Interval: p.TRI, End: p.TE}
}

// TrendPoints reads the trend configuration of homeControl's trend points that
// are used in this installation.
func (s *Service) TrendPoints(ctx context.Context) ([]TrendPoint, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	var ids []int
	for _, id := range catalog.TrendPoints {
		if m, ok := s.meta[protocol.MP][id]; !ok || m.US {
			ids = append(ids, id)
		}
	}
	s.mu.RUnlock()
	var out []TrendPoint
	err = s.runJob(ctx, "trend-points", func(progress func(int, int)) error {
		for i, id := range ids {
			p, err := c.ReadPoint(ctx, protocol.MP, id)
			if err != nil {
				if ctx.Err() != nil || errors.Is(err, device.ErrClosed) {
					return err
				}
				continue
			}
			if p.US {
				out = append(out, TrendPoint{ID: id, Name: p.Name, Min: p.Min, Max: p.Max, Config: trendConfig(p)})
			}
			progress(i+1, len(ids))
		}
		return nil
	})
	return out, err
}

func (s *Service) SetTrendConfig(ctx context.Context, ids []int, cfg protocol.TrendConfig) error {
	c, err := s.writableClient()
	if err != nil {
		return err
	}
	cfg.End = cfg.End.Local() // the controller clock runs on local time
	if cfg.Enabled && cfg.End.Before(time.Now()) {
		return invalid("the recording end must be in the future")
	}
	if cfg.Type != 0 && cfg.Type != 1 {
		return invalid("trend type must be 0 (interval) or 1 (hysteresis)")
	}
	for _, id := range ids {
		if !slices.Contains(catalog.TrendPoints, id) {
			return invalid("MP %d is not a trend point", id)
		}
	}
	for _, id := range ids {
		if err := c.WriteTrendConfig(ctx, id, cfg); err != nil {
			s.event("error", "Trendeinstellung nicht gespeichert", fmt.Sprintf("MP %d: %v", id, err))
			return err
		}
	}
	s.event("info", "Trendeinstellungen gespeichert", fmt.Sprintf("%d Punkte", len(ids)))
	return nil
}

func (s *Service) TrendCount(ctx context.Context) (int, error) {
	c, err := s.Client()
	if err != nil {
		return 0, err
	}
	return c.TrendCount(ctx)
}

// DownloadTrend reads the controller's trend buffer into a new recording.
// Deleting the buffer afterwards (and pausing it during the download) changes
// the controller and is refused in read-only mode.
func (s *Service) DownloadTrend(ctx context.Context, deleteAfter bool) (store.Recording, error) {
	pause := s.cfg.Get().TrendPauseDuringDownload
	var c *device.Client
	var err error
	if deleteAfter || pause {
		c, err = s.writableClient()
	} else {
		c, err = s.Client()
	}
	if err != nil {
		return store.Recording{}, err
	}
	var rec store.Recording
	err = s.runJob(ctx, "trend-download", func(progress func(int, int)) error {
		recs, err := c.TrendDownload(ctx, pause, deleteAfter, progress)
		if err != nil && len(recs) == 0 {
			return err
		}
		rec, err = s.saveTrend(ctx, recs, fmt.Sprintf("Trend %s", time.Now().Format("02.01.2006 15:04")), "trend")
		return err
	})
	if err == nil {
		s.event("info", "Trenddaten gelesen", fmt.Sprintf("%d Werte", rec.Samples))
	}
	return rec, err
}

func (s *Service) saveTrend(ctx context.Context, recs []protocol.TrendRecord, name, source string) (store.Recording, error) {
	rec := store.Recording{Name: name, Source: source, State: "stopped", Started: time.Now()}
	seen := map[int]bool{}
	for _, r := range recs {
		if r.Time.Before(rec.Started) {
			rec.Started = r.Time
		}
		if !seen[r.MPID] {
			seen[r.MPID] = true
			rec.Points = append(rec.Points, s.recordingPoint(r.MPID, len(rec.Points)))
		}
	}
	end := rec.Started
	for _, r := range recs {
		if r.Time.After(end) {
			end = r.Time
		}
	}
	rec.Ended = &end
	if err := s.store.CreateRecording(ctx, &rec); err != nil {
		return rec, err
	}
	samples := make([]store.Sample, len(recs))
	for i, r := range recs {
		reason := r.Reason
		samples[i] = store.Sample{PointID: r.MPID, Time: r.Time, Value: r.Value, Reason: &reason}
	}
	if err := s.store.AddSamples(ctx, rec.ID, samples); err != nil {
		return rec, err
	}
	rec.Samples = int64(len(samples))
	return rec, nil
}

var palette = []string{"#e6194b", "#3cb44b", "#4363d8", "#f58231", "#911eb4", "#46f0f0", "#f032e6", "#bcf60c", "#008080", "#9a6324", "#800000", "#808000", "#000075", "#808080"}

func (s *Service) recordingPoint(id, n int) store.RecordingPoint {
	s.mu.RLock()
	m := s.meta[protocol.MP][id]
	unit := ""
	for _, p := range s.snap.Panels { // the catalog knows units better than name heuristics
		for _, it := range p.Items {
			if unit == "" && it.Kind == protocol.MP && it.ID == id {
				unit = it.Unit
			}
		}
	}
	s.mu.RUnlock()
	name := m.Name
	if name == "" {
		name = fmt.Sprintf("MP %d", id)
	}
	if unit == "" {
		unit = diagItem(m).Unit
	}
	return store.RecordingPoint{ID: id, Name: name, Unit: unit, Color: palette[n%len(palette)]}
}
