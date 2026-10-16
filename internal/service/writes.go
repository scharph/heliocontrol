package service

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"heliocontrol/internal/catalog"
	"heliocontrol/internal/protocol"
)

// ErrWriteBudget is returned when a setup point was written too often today.
var ErrWriteBudget = errors.New("daily write limit reached for this setup point")

// writeBudget limits writes per setup point and day. Controller parameters
// most likely live in EEPROM with limited write cycles; the budget stops
// runaway automations without bothering people editing values by hand.
type writeBudget struct {
	mu     sync.Mutex
	day    string
	counts map[int]int
}

func (b *writeBudget) take(id, limit int) bool {
	if limit < 0 {
		return true // unlimited
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if today := time.Now().Format(time.DateOnly); b.day != today {
		b.day, b.counts = today, map[int]int{}
	}
	if b.counts[id] >= limit {
		return false
	}
	b.counts[id]++
	return true
}

func (b *writeBudget) used(id int) int {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.day != time.Now().Format(time.DateOnly) {
		return 0
	}
	return b.counts[id]
}

// WriteSP permanently changes a setup point. Points on the UI pages may always
// be changed; any other writable point only in expert mode. A permanent change
// replaces a temporary value on the same point.
func (s *Service) WriteSP(ctx context.Context, id int, v float64) (float64, error) {
	got, err := s.writeSP(ctx, id, v, false)
	if err == nil {
		s.dropOverride(id, "durch dauerhafte Änderung ersetzt")
	}
	return got, err
}

// checkWritable validates a write before anything is sent to the controller.
func (s *Service) checkWritable(id int, v float64) (name string, err error) {
	s.mu.RLock()
	item, onPage := catalog.FindWritable(s.snap.Panels, id)
	meta, known := s.meta[protocol.SP][id]
	s.mu.RUnlock()
	switch {
	case onPage:
		if item.Display == catalog.Enum && slices.Contains(item.NotSelectable, int(v)) {
			return "", invalid("value %v cannot be selected for %s", v, item.Label.EN)
		}
		if item.Max > item.Min && (v < item.Min || v > item.Max) {
			return "", invalid("value %v outside %v..%v", v, item.Min, item.Max)
		}
	case s.cfg.Get().ExpertMode && known && meta.WR:
	default:
		return "", invalid("setup point %d may not be changed (enable expert mode for points outside the pages)", id)
	}
	if known && meta.Max > meta.Min && (v < meta.Min || v > meta.Max) {
		return "", invalid("value %v outside the controller limits %v..%v", v, meta.Min, meta.Max)
	}
	if known && !meta.WR {
		return "", invalid("setup point %d is read-only on the controller", id)
	}
	if onPage {
		return item.Label.DE, nil
	}
	if meta.Name != "" {
		return meta.Name, nil
	}
	return fmt.Sprintf("SP %d", id), nil
}

// writeSP validates, skips no-op writes and enforces the write budget.
// restore marks the automatic restore of a temporary value: it must not be
// blocked by the budget.
func (s *Service) writeSP(ctx context.Context, id int, v float64, restore bool) (float64, error) {
	c, err := s.writableClient()
	if err != nil {
		return 0, err
	}
	name, err := s.checkWritable(id, v)
	if err != nil {
		return 0, err
	}
	// compare with the controller's current value, not the (possibly minutes old) cache
	if cur, err := c.ReadPoint(ctx, protocol.SP, id); err == nil && cur.Val == v {
		s.setSP(id, v)
		return v, nil
	}
	if !restore && !s.budget.take(id, s.cfg.Get().WriteBudget()) {
		s.event("warn", "Schreibzugriff abgelehnt", fmt.Sprintf("%s (SP %d): Tageslimit von %d Änderungen erreicht", name, id, s.cfg.Get().WriteBudget()))
		return 0, fmt.Errorf("%w (%d per day)", ErrWriteBudget, s.cfg.Get().WriteBudget())
	}
	got, err := c.WriteSP(ctx, id, v)
	if err != nil {
		s.event("error", "Einstellung nicht gespeichert", fmt.Sprintf("%s (SP %d) = %v: %v", name, id, v, err))
		return 0, err
	}
	s.setSP(id, got)
	s.event("info", "Einstellung gespeichert", fmt.Sprintf("%s (SP %d) = %v", name, id, got))
	if slices.Contains(catalog.ConfigPoints, id) {
		go s.Reconnect() // installation changed: pages must be rebuilt
	}
	return got, nil
}

func (s *Service) setSP(id int, v float64) {
	s.mu.Lock()
	s.snap.SP[id] = v
	s.mu.Unlock()
	s.Bus.Publish(Event{Type: "values", Data: s.Values()})
}
