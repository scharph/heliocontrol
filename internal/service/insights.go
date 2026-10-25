package service

import (
	"context"
	"math"
	"time"

	"heliocontrol/internal/catalog"
	"heliocontrol/internal/protocol"
	"heliocontrol/internal/store"
)

// Insights turn the history into the numbers that matter for efficiency:
// how often the compressor starts, how long it runs, and at which
// temperatures. Short cycling and an unnecessarily high flow temperature are
// the biggest free levers (see docs/design-review.md, section 3.1).

// DayInsight summarizes one calendar day. Nil fields had no data.
type DayInsight struct {
	Date           string   `json:"date"`
	Coverage       float64  `json:"coverage"` // share of the day with history (0..1)
	Starts         *int     `json:"starts,omitempty"`
	StartsSource   string   `json:"startsSource,omitempty"` // counter (MP 41) or samples
	RuntimeHours   *float64 `json:"runtimeHours,omitempty"`
	AvgCycleMin    *float64 `json:"avgCycleMinutes,omitempty"`
	Outdoor        *float64 `json:"outdoor,omitempty"`
	FlowRunning    *float64 `json:"flowRunning,omitempty"`   // mean flow temperature while the compressor runs
	ReturnRunning  *float64 `json:"returnRunning,omitempty"` // mean return temperature while the compressor runs
	Spread         *float64 `json:"spread,omitempty"`        // flow - return while running
	TargetRunning  *float64 `json:"targetRunning,omitempty"` // mean return target (MP 57) while running
	DeviationAbove *float64 `json:"deviation,omitempty"`     // return - target while running
	HeatingHours   *float64 `json:"heatingHours,omitempty"`  // compressor runtime for heating (request MP 56)
	DHWHours       *float64 `json:"dhwHours,omitempty"`      // compressor runtime for hot water
	DHWInterrupts  *int     `json:"dhwInterrupts,omitempty"` // heating requests interrupted by hot water
	HeatOutKWh     *float64 `json:"heatOutKWh,omitempty"`    // heating energy delivered (MP 52)
	HeatInKWh      *float64 `json:"heatInKWh,omitempty"`     // heating energy consumed (MP 53)
	COP            *float64 `json:"cop,omitempty"`           // HeatOutKWh / HeatInKWh
}

// Hint is a rule-based observation.
type Hint struct {
	Level  string       `json:"level"` // info, warn
	Title  catalog.Text `json:"title"`
	Detail catalog.Text `json:"detail"`
	Tips   []Tip        `json:"tips,omitempty"`
}

// Tip is an experience reported by other Heliotherm owners, with its source.
type Tip struct {
	Text   catalog.Text `json:"text"`
	Source string       `json:"source"`
}

type Insights struct {
	Days  []DayInsight `json:"days"`
	Hints []Hint       `json:"hints"`
}

// step is a step function from history rows: each value holds until the next row.
type step []store.HistoryRow

// valueAt returns the value valid at t.
func (s step) valueAt(t time.Time) (float64, bool) {
	v, ok := 0.0, false
	for _, r := range s {
		if r.Time.After(t) {
			break
		}
		v, ok = r.Value, true
	}
	return v, ok
}

// delta returns the increase of a counter between from and to.
func (s step) delta(from, to time.Time) (float64, bool) {
	if len(s) < 2 {
		return 0, false
	}
	first, ok := s.valueAt(from)
	if !ok {
		first = s[0].Value
	}
	last, ok := s.valueAt(to)
	if !ok || last < first {
		return 0, false
	}
	return last - first, true
}

// overlap returns how long, within [from, to), s satisfies fs while other
// satisfies fo.
func (s step) overlap(other step, from, to time.Time, fs, fo func(float64) bool) time.Duration {
	var d time.Duration
	s.segments(from, to, func(a, b time.Time, v float64) {
		if fs(v) {
			other.segments(a, b, func(oa, ob time.Time, w float64) {
				if fo(w) {
					d += ob.Sub(oa)
				}
			})
		}
	})
	return d
}

// valueBefore returns the value valid just before t (rows at t not included).
func (s step) valueBefore(t time.Time) (float64, bool) {
	v, ok := 0.0, false
	for _, r := range s {
		if !r.Time.Before(t) {
			break
		}
		v, ok = r.Value, true
	}
	return v, ok
}

// stale is how long a value is trusted without a new row (heartbeat + margin);
// beyond that the server was presumably not running.
const stale = historyHeartbeat + 5*time.Minute

// segments calls fn for each interval [a, b) in [from, to) with the value valid there.
func (s step) segments(from, to time.Time, fn func(a, b time.Time, v float64)) {
	for i, r := range s {
		a := r.Time
		b := to
		if i+1 < len(s) {
			b = s[i+1].Time
		}
		if lim := r.Time.Add(stale); b.After(lim) {
			b = lim
		}
		if a.Before(from) {
			a = from
		}
		if b.After(to) {
			b = to
		}
		if b.After(a) {
			fn(a, b, r.Value)
		}
	}
}

// mean returns the time-weighted mean over [from, to), optionally only where
// gate is non-zero.
func (s step) mean(from, to time.Time, gate step) (float64, bool) {
	var sum, dur float64
	s.segments(from, to, func(a, b time.Time, v float64) {
		if gate == nil {
			d := b.Sub(a).Seconds()
			sum, dur = sum+v*d, dur+d
			return
		}
		gate.segments(a, b, func(ga, gb time.Time, g float64) {
			if g != 0 {
				d := gb.Sub(ga).Seconds()
				sum, dur = sum+v*d, dur+d
			}
		})
	})
	if dur == 0 {
		return 0, false
	}
	return sum / dur, true
}

func ptr[T any](v T) *T { return &v }

func round1(v float64) float64 { return math.Round(v*10) / 10 }

// Insights computes the last days (today included) and hints from the last week.
func (s *Service) Insights(ctx context.Context, days int) (Insights, error) {
	days = min(max(days, 1), 90)
	load := func(id int, from, to time.Time) (step, error) {
		rows, err := s.store.HistoryPoints(ctx, store.PointRef{Kind: protocol.MP, ID: id}, from, to)
		return step(rows), err
	}
	now := time.Now()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	out := Insights{Days: []DayInsight{}, Hints: []Hint{}}
	for i := days - 1; i >= 0; i-- {
		from := today.AddDate(0, 0, -i)
		to := from.AddDate(0, 0, 1)
		end := to
		if end.After(now) {
			end = now
		}
		d := DayInsight{Date: from.Format(time.DateOnly)}
		pts := map[int]step{}
		for _, id := range []int{mpCompressor, mpCompressorStarts, mpOutdoor, mpFlow, mpReturn, mpReturnTarget, mpRequest, mpHeatOut, mpHeatIn} {
			upTo := to
			if id == mpCompressorStarts || id == mpHeatOut || id == mpHeatIn {
				upTo = to.Add(stale) // the counter value at midnight belongs to this day
			}
			st, err := load(id, from, upTo)
			if err != nil {
				return out, err
			}
			pts[id] = st
		}
		// coverage from the outdoor temperature, which is always recorded
		var covered time.Duration
		pts[mpOutdoor].segments(from, end, func(a, b time.Time, _ float64) { covered += b.Sub(a) })
		d.Coverage = round1(covered.Seconds()/to.Sub(from).Seconds()*100) / 100
		if covered == 0 {
			out.Days = append(out.Days, d)
			continue
		}
		comp := pts[mpCompressor]
		if len(comp) > 0 {
			var run time.Duration
			starts := 0
			prev, havePrev := comp.valueBefore(from)
			comp.segments(from, end, func(a, b time.Time, v float64) {
				if v != 0 {
					run += b.Sub(a)
				}
			})
			for _, r := range comp {
				if r.Time.Before(from) || !r.Time.Before(end) {
					continue
				}
				if r.Value != 0 && (!havePrev || prev == 0) {
					starts++
				}
				prev, havePrev = r.Value, true
			}
			d.RuntimeHours = ptr(math.Round(run.Hours()*100) / 100)
			d.Starts, d.StartsSource = ptr(starts), "samples"
		}
		// the controller's own counter is exact (sampling misses short cycles)
		if n, ok := pts[mpCompressorStarts].delta(from, end); ok {
			d.Starts, d.StartsSource = ptr(int(n)), "counter"
		}
		if d.Starts != nil && *d.Starts > 0 && d.RuntimeHours != nil {
			d.AvgCycleMin = ptr(round1(*d.RuntimeHours * 60 / float64(*d.Starts)))
		}
		if v, ok := pts[mpOutdoor].mean(from, end, nil); ok {
			d.Outdoor = ptr(round1(v))
		}
		if len(comp) > 0 {
			flow, okF := pts[mpFlow].mean(from, end, comp)
			ret, okR := pts[mpReturn].mean(from, end, comp)
			tgt, okT := pts[mpReturnTarget].mean(from, end, comp)
			if okF {
				d.FlowRunning = ptr(round1(flow))
			}
			if okR {
				d.ReturnRunning = ptr(round1(ret))
			}
			if okF && okR {
				d.Spread = ptr(round1(flow - ret))
			}
			if okT {
				d.TargetRunning = ptr(round1(tgt))
				if okR {
					d.DeviationAbove = ptr(round1(ret - tgt))
				}
			}
		}
		// what the compressor ran for, and how often hot water cut into heating
		if req := pts[mpRequest]; len(req) > 0 && len(comp) > 0 {
			on := func(v float64) bool { return v != 0 }
			is := func(r float64) func(float64) bool { return func(v float64) bool { return v == r } }
			d.HeatingHours = ptr(math.Round(comp.overlap(req, from, end, on, is(requestHeating)).Hours()*100) / 100)
			d.DHWHours = ptr(math.Round(comp.overlap(req, from, end, on, is(requestDHW)).Hours()*100) / 100)
			n := 0
			prev, havePrev := req.valueBefore(from)
			for _, r := range req {
				if r.Time.Before(from) || !r.Time.Before(end) {
					continue
				}
				if havePrev && prev == requestHeating && r.Value == requestDHW {
					n++
				}
				prev, havePrev = r.Value, true
			}
			d.DHWInterrupts = ptr(n)
		}
		heatOut, okO := pts[mpHeatOut].delta(from, end)
		heatIn, okI := pts[mpHeatIn].delta(from, end)
		if okO && okI {
			d.HeatOutKWh, d.HeatInKWh = ptr(round1(heatOut)), ptr(round1(heatIn))
			if heatIn >= 1 {
				d.COP = ptr(math.Round(heatOut/heatIn*100) / 100)
			}
		}
		out.Days = append(out.Days, d)
	}
	sp := s.Values().SP
	setting := func(id int) *float64 {
		if v, ok := sp[id]; ok {
			return ptr(v)
		}
		return nil
	}
	out.Hints = hints(out.Days, hintSettings{HeatingLimit: setting(spHeatingLimit), CurveAtLimit: setting(spCurveAtLimit)})
	return out, nil
}

