package service

import (
	"context"
	"fmt"
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
}

// Hint is a rule-based observation.
type Hint struct {
	Level  string       `json:"level"` // info, warn
	Title  catalog.Text `json:"title"`
	Detail catalog.Text `json:"detail"`
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
		for _, id := range []int{mpCompressor, mpCompressorStarts, mpOutdoor, mpFlow, mpReturn, mpReturnTarget} {
			upTo := to
			if id == mpCompressorStarts {
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
		if c := pts[mpCompressorStarts]; len(c) >= 2 {
			first, ok1 := c.valueAt(from)
			last, ok2 := c.valueAt(end)
			if !ok1 && len(c) > 0 {
				first, ok1 = c[0].Value, true
			}
			if ok1 && ok2 && last >= first {
				d.Starts, d.StartsSource = ptr(int(last-first)), "counter"
			}
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
		out.Days = append(out.Days, d)
	}
	out.Hints = hints(out.Days)
	return out, nil
}

// hints applies simple rules to the last 7 days with enough data.
func hints(days []DayInsight) []Hint {
	var n, starts int
	var runtime, outdoor, flow, spread float64
	var nFlow, nSpread int
	for i := len(days) - 1; i >= 0 && i >= len(days)-7; i-- {
		d := days[i]
		if d.Coverage < 0.5 || d.Starts == nil || d.RuntimeHours == nil {
			continue
		}
		n++
		starts += *d.Starts
		runtime += *d.RuntimeHours
		if d.Outdoor != nil {
			outdoor += *d.Outdoor
		}
		if d.FlowRunning != nil {
			flow += *d.FlowRunning
			nFlow++
		}
		if d.Spread != nil {
			spread += *d.Spread
			nSpread++
		}
	}
	out := []Hint{}
	if n == 0 {
		return out
	}
	perDay := float64(starts) / float64(n)
	avgOutdoor := outdoor / float64(n)
	if starts > 0 {
		cycle := runtime * 60 / float64(starts)
		if perDay >= 12 && cycle < 15 {
			out = append(out, Hint{Level: "warn",
				Title: catalog.Text{DE: "Verdichter taktet häufig", EN: "Compressor cycles often"},
				Detail: catalog.Text{
					DE: fmt.Sprintf("Ø %.0f Starts pro Tag mit Ø %.0f min Laufzeit. Kurze Zyklen kosten Effizienz und verschleißen den Verdichter. Häufige Ursachen: Heizkurve in der Übergangszeit zu hoch, zu kleine Hysterese, zu wenig Volumenstrom oder fehlender Puffer.", perDay, cycle),
					EN: fmt.Sprintf("Avg. %.0f starts per day with avg. %.0f min runtime. Short cycles cost efficiency and wear the compressor. Common causes: heating curve too high in mild weather, small hysteresis, low flow rate or no buffer tank.", perDay, cycle)}})
		}
	}
	if nFlow > 0 && avgOutdoor >= 5 {
		if f := flow / float64(nFlow); f >= 40 {
			out = append(out, Hint{Level: "info",
				Title: catalog.Text{DE: "Vorlauf hoch für die Außentemperatur", EN: "Flow temperature high for the outdoor temperature"},
				Detail: catalog.Text{
					DE: fmt.Sprintf("Bei Ø %.0f °C außen läuft der Vorlauf mit Ø %.0f °C. Jedes Grad weniger bringt rund 2,5 %% Effizienz. Die Heizkurve schrittweise (1–2 K, über mehrere Tage) senken und die Räume beobachten.", avgOutdoor, f),
					EN: fmt.Sprintf("At avg. %.0f °C outdoors the flow runs at avg. %.0f °C. Each degree less gains about 2.5 %% efficiency. Lower the heating curve step by step (1–2 K over several days) while watching room temperatures.", avgOutdoor, f)}})
		}
	}
	if nSpread > 0 {
		if sp := spread / float64(nSpread); sp > 8 {
			out = append(out, Hint{Level: "info",
				Title: catalog.Text{DE: "Große Spreizung", EN: "Large temperature spread"},
				Detail: catalog.Text{
					DE: fmt.Sprintf("Vorlauf und Rücklauf liegen im Betrieb Ø %.1f K auseinander (üblich: 3–7 K). Das deutet auf zu wenig Volumenstrom hin (Pumpenstufe, Ventile, hydraulischer Abgleich).", sp),
					EN: fmt.Sprintf("Flow and return differ by avg. %.1f K while running (typical: 3–7 K). This points to a low flow rate (pump setting, valves, hydraulic balancing).", sp)}})
		}
	}
	return out
}
