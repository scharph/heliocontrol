package service

import (
	"context"
	"maps"
	"time"

	"heliocontrol/internal/catalog"
	"heliocontrol/internal/device"
	"heliocontrol/internal/protocol"
)

// settingsInterval is how often the settings (non-volatile setup points) are
// re-read. Reading one setup point takes 100-200 ms on a real controller, so
// reading all settings on every poll kept the bus busy 15-20 % of the time.
const settingsInterval = 10 * time.Minute

// Refresh reads everything now (the UI's "refresh" button).
func (s *Service) Refresh(ctx context.Context) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	return s.poll(ctx, c, true)
}

// poll reads all measuring points with one multi-read plus the volatile setup
// points; the settings only when due (or when full is set).
func (s *Service) poll(ctx context.Context, c *device.Client, full bool) error {
	s.mu.RLock()
	mps, sps := catalog.Points(s.snap.Panels)
	read := catalog.VolatilePoints(s.snap.Panels)
	full = full || time.Since(s.lastSettings) >= settingsInterval
	s.mu.RUnlock()
	if full {
		read = sps
	}
	vals, err := c.ReadValues(ctx, mps)
	if err != nil {
		return err
	}
	pts, err := c.ReadPoints(ctx, protocol.SP, read)
	if err != nil {
		return err
	}
	now := time.Now()
	s.mu.Lock()
	maps.Copy(s.snap.MP, vals)
	for id, p := range pts {
		s.snap.SP[id] = p.Val
		s.meta[protocol.SP][id] = mergeMeta(s.meta[protocol.SP][id], p)
	}
	s.snap.Updated = now
	if full {
		s.lastSettings = now
	}
	mpNow, spNow := maps.Clone(s.snap.MP), maps.Clone(s.snap.SP)
	s.mu.Unlock()
	s.Bus.Publish(Event{Type: "values", Data: s.Values()})
	s.recordHistory(ctx, now, mpNow, spNow)
	return nil
}

// mergeMeta keeps a cached name if a reply lacks one.
func mergeMeta(old, p protocol.Point) protocol.Point {
	if p.Name == "" {
		p.Name = old.Name
	}
	return p
}
