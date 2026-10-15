package device

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"heliocontrol/internal/protocol"
)

// ReadID returns the controller ID (the heat pump serial number homeControl
// checks against its configuration).
func (c *Client) ReadID(ctx context.Context) (int, error) {
	r, err := c.RequestPrefix(ctx, protocol.CmdReadID, "~RID", 2*time.Second)
	if err != nil {
		return 0, err
	}
	return protocol.ParseCount(r)
}

// ReadPoint reads one measuring or setup point.
func (c *Client) ReadPoint(ctx context.Context, kind protocol.Kind, nr int) (protocol.Point, error) {
	cmd := protocol.ReadMP(nr)
	if kind == protocol.SP {
		cmd = protocol.ReadSP(nr)
	}
	r, err := c.RequestPrefix(ctx, cmd, fmt.Sprintf("~%s,NR=%d,", strings.ToUpper(string(kind)), nr), time.Second)
	if err != nil {
		return protocol.Point{}, err
	}
	p, err := protocol.ParsePoint(r)
	if err == nil && p.Kind != kind {
		err = fmt.Errorf("unexpected reply %q", r)
	}
	return p, err
}

// ReadPoints reads the given points one by one; points that fail are retried
// once (as homeControl does) and then skipped.
func (c *Client) ReadPoints(ctx context.Context, kind protocol.Kind, ids []int) (map[int]protocol.Point, error) {
	out := make(map[int]protocol.Point, len(ids))
	var missing []int
	for _, id := range ids {
		p, err := c.ReadPoint(ctx, kind, id)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, ErrClosed) {
				return out, err
			}
			missing = append(missing, id)
			continue
		}
		out[id] = p
	}
	for _, id := range missing {
		time.Sleep(100 * time.Millisecond)
		if p, err := c.ReadPoint(ctx, kind, id); err == nil {
			out[id] = p
		}
	}
	if len(out) == 0 && len(ids) > 0 {
		return out, fmt.Errorf("no %s point could be read", kind)
	}
	return out, nil
}

// ReadAll reads every point of a kind via ~RM / ~RS, then fills gaps
// individually. progress may be nil.
func (c *Client) ReadAll(ctx context.Context, kind protocol.Kind, progress func(done, total int)) ([]protocol.Point, error) {
	cmd, first, gap := protocol.CmdCountMP, time.Second, 4*time.Second
	if kind == protocol.SP {
		cmd, first, gap = protocol.CmdCountSP, 3*time.Second, 2*time.Second
	}
	total, replies, err := c.requestCounted(ctx, cmd, first, gap, progress)
	if err != nil && total == 0 {
		return nil, err
	}
	byID := map[int]protocol.Point{}
	for _, r := range replies {
		if p, err := protocol.ParsePoint(r); err == nil {
			byID[p.ID] = p
		}
	}
	for id := 0; id < total && len(byID) < total; id++ {
		if _, ok := byID[id]; ok {
			continue
		}
		p, err := c.ReadPoint(ctx, kind, id)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		byID[id] = p
		if progress != nil {
			progress(len(byID), total)
		}
	}
	out := make([]protocol.Point, 0, len(byID))
	for _, p := range byID {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b protocol.Point) int { return a.ID - b.ID })
	return out, nil
}

// ReadValues fetches current values of measuring points with the fast ~MR
// multi-read (used by the WEB-Online recorder). Requests are split so a single
// command stays below the controller's length limit.
func (c *Client) ReadValues(ctx context.Context, ids []int) (map[int]float64, error) {
	out := make(map[int]float64, len(ids))
	for len(ids) > 0 {
		n := 0
		for l := 4; n < len(ids); n++ { // "~MR" + ";"
			l += len(fmt.Sprint(ids[n])) + 1
			if l > maxCommandLen {
				break
			}
		}
		batch := ids[:n]
		ids = ids[n:]
		replies, err := c.RequestMany(ctx, protocol.ReadValues(batch), len(batch), time.Second)
		if err != nil {
			return out, err
		}
		for _, r := range replies {
			if v, err := protocol.ParseValue(r); err == nil {
				out[v.ID] = v.Val
			}
		}
	}
	return out, nil
}

// WriteSP stores a setup point and returns the value the controller reports
// back (it may clamp or reject the value).
func (c *Client) WriteSP(ctx context.Context, nr int, v float64) (float64, error) {
	prefix := fmt.Sprintf("~SP,NR=%d,", nr)
	if nr == 8 {
		prefix = "~" // SP 8 answers differently; homeControl accepts any reply for it
	}
	r, err := c.RequestPrefix(ctx, protocol.WriteSP(nr, v), prefix, 2*time.Second)
	if err != nil {
		return 0, err
	}
	if !strings.HasPrefix(r, "~SP,NR=") {
		if nr == 8 {
			return v, nil
		}
		return 0, fmt.Errorf("write SP %d rejected: %q", nr, r)
	}
	p, err := protocol.ParsePoint(r)
	if err != nil {
		return v, nil
	}
	return p.Val, nil
}

// SetOverride forces (or with auto=true releases) a measuring point value.
func (c *Client) SetOverride(ctx context.Context, nr int, v float64, auto bool) error {
	cmd := protocol.OverrideMP(nr, v)
	if auto {
		cmd = protocol.AutoMP(nr)
	}
	r, err := c.RequestPrefix(ctx, cmd, fmt.Sprintf("~MP,NR=%d,", nr), time.Second)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(r, "~MP") {
		return fmt.Errorf("override rejected: %q", r)
	}
	return nil
}

// ResetFault acknowledges the active fault ("Entstören").
func (c *Client) ResetFault(ctx context.Context) error {
	_, err := c.WriteSP(ctx, protocol.FaultResetSP, 1)
	return err
}

func (c *Client) ReadClock(ctx context.Context) (time.Time, error) {
	r, err := c.RequestPrefix(ctx, protocol.CmdClock, "~CLK", 2*time.Second)
	if err != nil {
		return time.Time{}, err
	}
	return protocol.ParseClock(r)
}

func (c *Client) SetClock(ctx context.Context, t time.Time) error {
	cmd := protocol.SetClock(t)
	r, err := c.RequestPrefix(ctx, cmd, "~CLK", 2*time.Second)
	if err != nil {
		return err
	}
	if !strings.Contains(r, strings.TrimSuffix(strings.TrimPrefix(cmd, "~CLK,"), ";")) {
		return fmt.Errorf("set clock rejected: %q", r)
	}
	return nil
}

// --- trend buffer -----------------------------------------------------------

func (c *Client) TrendCount(ctx context.Context) (int, error) {
	r, err := c.RequestPrefix(ctx, protocol.CmdTrendCount, "~SUM", time.Second)
	if err != nil {
		return 0, err
	}
	return protocol.ParseCount(r)
}

func (c *Client) expect(ctx context.Context, cmd, want string) error {
	_, err := c.RequestPrefix(ctx, cmd, want, time.Second)
	return err
}

func (c *Client) TrendStop(ctx context.Context) error {
	return c.expect(ctx, protocol.CmdTrendStop, "~TF,1")
}
func (c *Client) TrendRestart(ctx context.Context) error {
	return c.expect(ctx, protocol.CmdTrendRestart, "~TF,0")
}
func (c *Client) TrendDelete(ctx context.Context) error {
	return c.expect(ctx, protocol.CmdTrendDelete, "~OK")
}

// TrendDownload reads the whole trend buffer. With pause set, recording is
// stopped during the download and restarted afterwards (homeControl's "read
// all trend data" option; otherwise only records available at the start are
// read). With deleteAfter the buffer is cleared after a complete read.
func (c *Client) TrendDownload(ctx context.Context, pause, deleteAfter bool, progress func(done, total int)) ([]protocol.TrendRecord, error) {
	if pause {
		if err := c.TrendStop(ctx); err != nil {
			return nil, err
		}
		defer func() {
			rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
			defer cancel()
			_ = c.TrendRestart(rctx)
		}()
	}
	total, err := c.TrendCount(ctx)
	if err != nil {
		return nil, err
	}
	byID := make(map[int]protocol.TrendRecord, total)
	for next := 0; next < total; {
		batch := []int{}
		for l := 4; next < total && len(batch) < 10; next++ {
			l += len(fmt.Sprint(next)) + 1
			if l > maxCommandLen {
				break
			}
			batch = append(batch, next)
		}
		replies, err := c.RequestMany(ctx, protocol.ReadTrend(batch), len(batch), 1500*time.Millisecond)
		if err != nil && ctx.Err() != nil {
			return nil, ctx.Err()
		}
		for _, r := range replies {
			if rec, err := protocol.ParseTrendRecord(r); err == nil {
				byID[rec.ID] = rec
			}
		}
		if progress != nil {
			progress(len(byID), total)
		}
	}
	for id := 0; id < total && len(byID) < total; id++ {
		if _, ok := byID[id]; ok {
			continue
		}
		r, err := c.RequestPrefix(ctx, protocol.ReadTrend([]int{id}), fmt.Sprintf("~TA,%d,", id), time.Second)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue
		}
		if rec, err := protocol.ParseTrendRecord(r); err == nil {
			byID[rec.ID] = rec
		}
	}
	if len(byID) < total {
		return nil, fmt.Errorf("trend download incomplete: %d of %d records", len(byID), total)
	}
	out := make([]protocol.TrendRecord, 0, total)
	for _, r := range byID {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b protocol.TrendRecord) int { return a.ID - b.ID })
	if deleteAfter {
		if err := c.TrendDelete(ctx); err != nil {
			return out, fmt.Errorf("records read, but deleting the buffer failed: %w", err)
		}
	}
	return out, nil
}

func (c *Client) WriteTrendConfig(ctx context.Context, nr int, cfg protocol.TrendConfig) error {
	r, err := c.RequestPrefix(ctx, protocol.WriteTrendConfig(nr, cfg), fmt.Sprintf("~MP,NR=%d,", nr), time.Second)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(r, "~MP") {
		return fmt.Errorf("trend config rejected: %q", r)
	}
	return nil
}

// --- time programs ----------------------------------------------------------

// TimePrograms lists the time programs that are used in this installation.
func (c *Client) TimePrograms(ctx context.Context) ([]protocol.TimeProgram, error) {
	total, replies, err := c.requestCounted(ctx, protocol.CmdTimeProgramList, time.Second, 2*time.Second, nil)
	if err != nil && total == 0 {
		return nil, err
	}
	byID := map[int]protocol.TimeProgram{}
	for _, r := range replies {
		if tp, err := protocol.ParseTimeProgram(r); err == nil {
			byID[tp.ID] = tp
		}
	}
	for id := 0; id < total; id++ {
		if _, ok := byID[id]; ok {
			continue
		}
		r, err := c.RequestPrefix(ctx, protocol.ReadTimeProgram(id), fmt.Sprintf("~PRI%d,", id), time.Second)
		if err != nil {
			return nil, fmt.Errorf("time program %d: %w", id, err)
		}
		tp, err := protocol.ParseTimeProgram(r)
		if err != nil {
			return nil, err
		}
		byID[id] = tp
	}
	var out []protocol.TimeProgram
	for _, tp := range byID {
		if tp.US {
			out = append(out, tp)
		}
	}
	slices.SortFunc(out, func(a, b protocol.TimeProgram) int { return a.ID - b.ID })
	return out, nil
}

// TimeProgramEvents reads all switching events (days x events per day) of a program.
func (c *Client) TimeProgramEvents(ctx context.Context, tp protocol.TimeProgram) ([]protocol.TimeProgramEvent, error) {
	want := tp.EAD * tp.NOD
	// ~PRD answers with the program header (~PRI<id>,...) followed by one ~PRE
	// reply per event.
	cmd := protocol.ReadTimeProgramData(tp.ID)
	var replies []string
	c.txMu.Lock()
	for attempt := 0; ; attempt++ {
		_, err := c.exchangePrefix(ctx, cmd, fmt.Sprintf("~PRI%d,", tp.ID), time.Second)
		if err == nil {
			replies, _ = c.collect(ctx, cmd, want, time.Second)
			break
		}
		if attempt == 2 || ctx.Err() != nil {
			c.txMu.Unlock()
			return nil, fmt.Errorf("time program %d data: %w", tp.ID, err)
		}
	}
	c.txMu.Unlock()
	type key struct{ day, ev int }
	got := map[key]protocol.TimeProgramEvent{}
	for _, r := range replies {
		if ev, err := protocol.ParseTimeProgramEvent(r); err == nil && ev.Program == tp.ID {
			got[key{ev.Day, ev.Event}] = ev
		}
	}
	for d := 0; d < tp.NOD; d++ {
		for e := 0; e < tp.EAD; e++ {
			if _, ok := got[key{d, e}]; ok {
				continue
			}
			r, err := c.RequestPrefix(ctx, protocol.ReadTimeProgramEvent(tp.ID, d, e), fmt.Sprintf("~PRE,PR=%d,DAY=%d,EV=%d,", tp.ID, d, e), time.Second)
			if err != nil {
				return nil, fmt.Errorf("time program %d day %d event %d: %w", tp.ID, d, e, err)
			}
			ev, err := protocol.ParseTimeProgramEvent(r)
			if err != nil {
				return nil, err
			}
			got[key{d, e}] = ev
		}
	}
	out := make([]protocol.TimeProgramEvent, 0, want)
	for _, ev := range got {
		out = append(out, ev)
	}
	slices.SortFunc(out, func(a, b protocol.TimeProgramEvent) int {
		if a.Day != b.Day {
			return a.Day - b.Day
		}
		return a.Event - b.Event
	})
	return out, nil
}

// WriteTimeProgramEvent stores one switching event.
func (c *Client) WriteTimeProgramEvent(ctx context.Context, ev protocol.TimeProgramEvent) error {
	r, err := c.RequestPrefix(ctx, protocol.WriteTimeProgramEvent(ev), fmt.Sprintf("~PRE,PR=%d,DAY=%d,EV=%d,", ev.Program, ev.Day, ev.Event), time.Second)
	if err != nil {
		return err
	}
	if _, err := protocol.ParseTimeProgramEvent(r); err != nil {
		return fmt.Errorf("time program event rejected: %q", r)
	}
	return nil
}
