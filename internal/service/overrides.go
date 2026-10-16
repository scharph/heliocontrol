package service

import (
	"context"
	"fmt"
	"slices"
	"time"

	"heliocontrol/internal/protocol"
	"heliocontrol/internal/store"
)

// Temporary setpoints ("overrides") change a setup point for a limited time
// and restore the original value automatically - also after a restart. They
// are the building block for PV-surplus and price-driven control: the
// controller has no SG-Ready input in its protocol, but raising e.g. the hot
// water target for an hour has the same effect.

const (
	minOverride = time.Minute
	maxOverride = 24 * time.Hour
)

// SetTemporary sets setup point sp to value for d. If the point already has a
// temporary value, it is replaced but the original value is kept.
func (s *Service) SetTemporary(ctx context.Context, sp int, value float64, d time.Duration, source string) (store.Override, error) {
	if d < minOverride || d > maxOverride {
		return store.Override{}, invalid("duration must be between 1 minute and 24 hours")
	}
	c, err := s.writableClient()
	if err != nil {
		return store.Override{}, err
	}
	if _, err := s.checkWritable(sp, value); err != nil {
		return store.Override{}, err
	}
	s.mu.RLock()
	existing, active := s.overrides[sp]
	s.mu.RUnlock()
	original := existing.Original
	if !active {
		cur, err := c.ReadPoint(ctx, protocol.SP, sp)
		if err != nil {
			return store.Override{}, fmt.Errorf("read current value: %w", err)
		}
		original = cur.Val
	}
	if _, err := s.checkWritable(sp, original); err != nil {
		return store.Override{}, fmt.Errorf("the current value %v could not be restored later: %w", original, err)
	}
	got, err := s.writeSP(ctx, sp, value, false)
	if err != nil {
		return store.Override{}, err
	}
	now := time.Now()
	o := store.Override{SP: sp, Value: got, Original: original, Until: now.Add(d), Created: now, Source: source}
	if err := s.store.SaveOverride(ctx, o); err != nil {
		// the value is written but would never be restored: undo it
		_, _ = s.writeSP(context.WithoutCancel(ctx), sp, original, true)
		return store.Override{}, fmt.Errorf("store temporary value: %w", err)
	}
	s.mu.Lock()
	s.overrides[sp] = o
	s.mu.Unlock()
	s.event("info", "Temporärer Wert gesetzt", fmt.Sprintf("SP %d = %v bis %s (danach %v)%s", sp, got, o.Until.Format("15:04"), original, sourceSuffix(source)))
	s.publishOverrides()
	return o, nil
}

func sourceSuffix(src string) string {
	if src == "" {
		return ""
	}
	return ", " + src
}

// CancelTemporary restores the original value now.
func (s *Service) CancelTemporary(ctx context.Context, sp int) error {
	s.mu.RLock()
	o, ok := s.overrides[sp]
	s.mu.RUnlock()
	if !ok {
		return invalid("setup point %d has no temporary value", sp)
	}
	return s.restore(ctx, o, "vorzeitig beendet")
}

func (s *Service) restore(ctx context.Context, o store.Override, why string) error {
	if _, err := s.writeSP(ctx, o.SP, o.Original, true); err != nil {
		return fmt.Errorf("restore SP %d to %v: %w", o.SP, o.Original, err)
	}
	s.dropOverride(o.SP, "")
	s.event("info", "Temporärer Wert zurückgesetzt", fmt.Sprintf("SP %d = %v (%s)", o.SP, o.Original, why))
	return nil
}

// dropOverride forgets a temporary value without restoring it.
func (s *Service) dropOverride(sp int, why string) {
	s.mu.Lock()
	_, ok := s.overrides[sp]
	delete(s.overrides, sp)
	s.mu.Unlock()
	if !ok {
		return
	}
	if err := s.store.DeleteOverride(context.Background(), sp); err != nil {
		s.log.Warn("delete override", "sp", sp, "err", err)
	}
	if why != "" {
		s.event("info", "Temporärer Wert beendet", fmt.Sprintf("SP %d %s", sp, why))
	}
	s.publishOverrides()
}

// HasOverrides reports whether temporary values are active (they need write
// access to be restored, so read-only mode cannot be switched on meanwhile).
func (s *Service) HasOverrides() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.overrides) > 0
}

func (s *Service) overrideList() []store.Override {
	out := make([]store.Override, 0, len(s.overrides))
	for _, o := range s.overrides {
		out = append(out, o)
	}
	slices.SortFunc(out, func(a, b store.Override) int { return a.Until.Compare(b.Until) })
	return out
}

// Overrides lists the active temporary values.
func (s *Service) Overrides() []store.Override {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.overrideList()
}

func (s *Service) publishOverrides() { s.Bus.Publish(Event{Type: "overrides", Data: s.Overrides()}) }

// overrideLoop restores expired temporary values. While the heat pump is
// unreachable they are retried until it is back.
func (s *Service) overrideLoop(ctx context.Context) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	warned := map[int]bool{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.restoreExpired(ctx, warned)
		}
	}
}

// restoreExpired restores every temporary value whose time is up. Failures are
// logged once per point (in warned) and retried on the next call.
func (s *Service) restoreExpired(ctx context.Context, warned map[int]bool) {
	now := time.Now()
	for _, o := range s.Overrides() {
		if o.Until.After(now) {
			continue
		}
		if _, err := s.Client(); err != nil {
			return // offline: try again later
		}
		if err := s.restore(ctx, o, "abgelaufen"); err != nil {
			if !warned[o.SP] {
				warned[o.SP] = true
				s.event("error", "Temporärer Wert konnte nicht zurückgesetzt werden", err.Error())
			}
			continue
		}
		delete(warned, o.SP)
	}
}
