package service

import (
	"context"
	"fmt"
	"time"

	"heliocontrol/internal/protocol"
	"heliocontrol/internal/store"
)

// recorder samples measuring points at a fixed interval into a recording
// (homeControl's "WEB-Online" recorder). One live recording runs at a time.
type recorder struct {
	rec    store.Recording
	ids    []int
	paused bool
	cancel context.CancelFunc
}

// Sample is published for every recorded tick.
type Sample struct {
	RecordingID int64           `json:"recordingId"`
	T           int64           `json:"t"`
	Values      map[int]float64 `json:"values"`
}

func (s *Service) StartRecording(ctx context.Context, name string, ids []int, interval time.Duration) (store.Recording, error) {
	if len(ids) == 0 {
		return store.Recording{}, invalid("select at least one point")
	}
	if interval < time.Second || interval > 24*time.Hour {
		return store.Recording{}, invalid("interval must be between 1 s and 24 h")
	}
	if _, err := s.Client(); err != nil {
		return store.Recording{}, err
	}
	s.mu.Lock()
	if s.rec != nil {
		s.mu.Unlock()
		return store.Recording{}, invalid("a recording is already running")
	}
	s.rec = &recorder{} // reserve
	s.mu.Unlock()

	if name == "" {
		name = fmt.Sprintf("WEB-Online %s", time.Now().Format("02.01.2006 15:04"))
	}
	rec := store.Recording{Name: name, Source: "live", State: "recording", Started: time.Now(), IntervalMS: interval.Milliseconds()}
	for i, id := range ids {
		rec.Points = append(rec.Points, s.recordingPoint(id, i))
	}
	if err := s.store.CreateRecording(ctx, &rec); err != nil {
		s.mu.Lock()
		s.rec = nil
		s.mu.Unlock()
		return rec, err
	}
	rctx, cancel := context.WithCancel(context.Background())
	r := &recorder{rec: rec, ids: ids, cancel: cancel}
	s.mu.Lock()
	s.rec = r
	s.snap.Recording = &r.rec
	s.mu.Unlock()
	go s.record(rctx, r, interval)
	s.event("info", "Aufzeichnung gestartet", fmt.Sprintf("%s, %d Punkte, Intervall %s", name, len(ids), interval))
	s.publishRecording()
	return rec, nil
}

func (s *Service) record(ctx context.Context, r *recorder, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		s.mu.RLock()
		paused := r.paused
		s.mu.RUnlock()
		if !paused {
			s.sample(ctx, r)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Service) sample(ctx context.Context, r *recorder) {
	c, err := s.Client()
	if err != nil {
		return // offline: skip this tick
	}
	vals, err := c.ReadValues(ctx, r.ids)
	if err != nil || len(vals) == 0 {
		return
	}
	now := time.Now()
	samples := make([]store.Sample, 0, len(vals))
	for id, v := range vals {
		samples = append(samples, store.Sample{PointID: id, Time: now, Value: v})
	}
	if err := s.store.AddSamples(ctx, r.rec.ID, samples); err != nil {
		s.log.Warn("store samples", "err", err)
		return
	}
	s.mu.Lock()
	r.rec.Samples += int64(len(samples))
	s.mu.Unlock()
	s.Bus.Publish(Event{Type: "sample", Data: Sample{RecordingID: r.rec.ID, T: now.UnixMilli(), Values: vals}})
}

func (s *Service) PauseRecording(pause bool) error {
	s.mu.Lock()
	r := s.rec
	if r == nil || r.cancel == nil {
		s.mu.Unlock()
		return invalid("no recording is running")
	}
	r.paused = pause
	r.rec.State = map[bool]string{true: "paused", false: "recording"}[pause]
	s.mu.Unlock()
	_ = s.store.SetRecordingState(context.Background(), r.rec.ID, r.rec.State, nil)
	s.publishRecording()
	return nil
}

func (s *Service) StopRecording() error {
	s.mu.Lock()
	r := s.rec
	if r == nil || r.cancel == nil {
		s.mu.Unlock()
		return invalid("no recording is running")
	}
	s.rec = nil
	s.snap.Recording = nil
	s.mu.Unlock()
	r.cancel()
	now := time.Now()
	_ = s.store.SetRecordingState(context.Background(), r.rec.ID, "stopped", &now)
	s.event("info", "Aufzeichnung beendet", fmt.Sprintf("%s, %d Werte", r.rec.Name, r.rec.Samples))
	s.publishRecording()
	return nil
}

func (s *Service) publishRecording() {
	s.mu.RLock()
	var cur *store.Recording
	if s.rec != nil && s.rec.cancel != nil {
		c := s.rec.rec
		cur = &c
	}
	s.mu.RUnlock()
	s.Bus.Publish(Event{Type: "recording", Data: cur})
}

// ActiveRecordingID returns the id of the running recording, or 0.
func (s *Service) ActiveRecordingID() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.rec == nil {
		return 0
	}
	return s.rec.rec.ID
}

// ImportTrend stores trend records read from a homeControl trend file (.htd).
func (s *Service) ImportTrend(ctx context.Context, name string, recs []protocol.TrendRecord) (store.Recording, error) {
	if len(recs) == 0 {
		return store.Recording{}, invalid("the file contains no trend records")
	}
	rec, err := s.saveTrend(ctx, recs, name, "import")
	if err == nil {
		s.event("info", "Datei importiert", fmt.Sprintf("%s, %d Werte", name, rec.Samples))
	}
	return rec, err
}

// ImportRecording stores a recording read from a WEB-Online file (.hwo).
func (s *Service) ImportRecording(ctx context.Context, rec store.Recording, samples []store.Sample) (store.Recording, error) {
	if len(samples) == 0 {
		return rec, invalid("the file contains no values")
	}
	rec.Source, rec.State = "import", "stopped"
	for i := range rec.Points {
		if rec.Points[i].Color == "" {
			rec.Points[i].Color = palette[i%len(palette)]
		}
	}
	if err := s.store.CreateRecording(ctx, &rec); err != nil {
		return rec, err
	}
	if err := s.store.AddSamples(ctx, rec.ID, samples); err != nil {
		return rec, err
	}
	rec.Samples = int64(len(samples))
	s.event("info", "Datei importiert", fmt.Sprintf("%s, %d Werte", rec.Name, rec.Samples))
	return rec, nil
}
