package service

import (
	"context"
	"slices"
	"sync"
	"time"

	"heliocontrol/internal/catalog"
	"heliocontrol/internal/protocol"
	"heliocontrol/internal/store"
)

// The continuous history records the page values after every poll, but stores
// a value only when it changed or after historyHeartbeat. Most values are
// constant for long stretches (pumps, settings), so this keeps a year of
// history in the low hundreds of megabytes.

const historyHeartbeat = 15 * time.Minute

// Points the insights need in addition to the page values.
const (
	mpCompressor       = 30 // Verdichter (on/off)
	mpCompressorStarts = 41 // BSZ Verdichter Schaltungen (counter)
	mpOutdoor          = 0
	mpFlow             = 3
	mpReturn           = 4
	mpReturnTarget     = 57
	mpRequest          = 56 // Anforderung (enum)
	mpHeatOut          = 52 // Heizen abgegeben (kWh counter, energy page)
	mpHeatIn           = 53 // Heizen aufgenommen (kWh counter, energy page)

	requestHeating = 2
	requestDHW     = 3

	spHeatingLimit = 76 // Heizgrenze
	spCurveAtLimit = 80 // RL Soll bei Heizgrenze
)

type lastValue struct {
	v float64
	t time.Time
}

type historyRecorder struct {
	mu   sync.Mutex
	last map[store.PointRef]lastValue
}

// historyRefs lists the recorded points: measuring points of the pages
// (without the diagnostics page), the volatile and hour counter setup points,
// and the compressor start counter.
func (s *Service) historyRefs() []store.PointRef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []store.PointRef
	add := func(k protocol.Kind, id int) {
		r := store.PointRef{Kind: k, ID: id}
		if !slices.Contains(out, r) {
			out = append(out, r)
		}
	}
	for _, p := range s.snap.Panels {
		if p.ID == "diagnostics" {
			continue
		}
		for _, it := range p.Items {
			switch {
			case it.Display == catalog.Ratio:
				add(protocol.MP, it.Num)
				add(protocol.MP, it.Den)
			case it.Kind == protocol.MP:
				add(protocol.MP, it.ID)
			case it.Volatile || it.Unit == "h" || it.Display == catalog.Enum:
				add(protocol.SP, it.ID)
			}
		}
	}
	if m, ok := s.meta[protocol.MP][mpCompressorStarts]; !ok || m.US {
		add(protocol.MP, mpCompressorStarts)
	}
	return out
}

// recordHistory stores the values that changed (or are due for a heartbeat).
func (s *Service) recordHistory(ctx context.Context, now time.Time, mp, sp map[int]float64) {
	refs := s.historyRefs()
	h := &s.history
	h.mu.Lock()
	if h.last == nil {
		h.last = map[store.PointRef]lastValue{}
	}
	var rows []store.HistoryRow
	for _, r := range refs {
		src := mp
		if r.Kind == protocol.SP {
			src = sp
		}
		v, ok := src[r.ID]
		if !ok {
			continue
		}
		if l, seen := h.last[r]; seen && l.v == v && now.Sub(l.t) < historyHeartbeat {
			continue
		}
		h.last[r] = lastValue{v: v, t: now}
		rows = append(rows, store.HistoryRow{Kind: r.Kind, ID: r.ID, Time: now, Value: v})
	}
	h.mu.Unlock()
	if err := s.store.AddHistory(ctx, rows); err != nil {
		s.log.Warn("store history", "err", err)
	}
}

// historyLoop prunes the history once a day.
func (s *Service) historyLoop(ctx context.Context) {
	prune := func() {
		days := s.cfg.Get().HistoryDays
		n, err := s.store.PruneHistory(ctx, time.Now().AddDate(0, 0, -days))
		if err != nil {
			s.log.Warn("prune history", "err", err)
		} else if n > 0 {
			s.log.Info("pruned history", "rows", n, "keepDays", days)
		}
	}
	prune()
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			prune()
		}
	}
}

// History returns averaged series of points for charts.
func (s *Service) History(ctx context.Context, refs []store.PointRef, from, to time.Time, maxPoints int) ([]store.Series, error) {
	out := make([]store.Series, 0, len(refs))
	for _, r := range refs {
		ser, err := s.store.HistorySeries(ctx, r, from, to, maxPoints)
		if err != nil {
			return nil, err
		}
		if r.Kind == protocol.SP {
			ser.PointID = -r.ID // setup points are reported with negative ids
		}
		out = append(out, ser)
	}
	return out, nil
}
