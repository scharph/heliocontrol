package service

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"heliocontrol/internal/config"
	"heliocontrol/internal/protocol"
	"heliocontrol/internal/store"
)

func TestInsightsFromHistory(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := config.Load(filepath.Join(dir, "c.yaml"))
	st, err := store.Open(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	// yesterday: the compressor runs 10 min every 30 min (48 starts, 8 h),
	// 8 °C outdoors, flow 45 °C, return 40 °C, return target 39 °C
	now := time.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -1)
	var rows []store.HistoryRow
	add := func(id int, at time.Time, v float64) {
		rows = append(rows, store.HistoryRow{Kind: protocol.MP, ID: id, Time: at, Value: v})
	}
	for m := 0; m < 24*60; m += 5 {
		at := day.Add(time.Duration(m) * time.Minute)
		on := 0.0
		if m%30 < 10 {
			on = 1
		}
		add(mpCompressor, at, on)
		add(mpOutdoor, at, 8)
		add(mpFlow, at, 45)
		add(mpReturn, at, 40)
		add(mpReturnTarget, at, 39)
	}
	if err := st.AddHistory(ctx, rows); err != nil {
		t.Fatal(err)
	}

	in, err := svc.Insights(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	d := in.Days[0]
	if d.Date != day.Format(time.DateOnly) || d.Coverage < 0.99 {
		t.Fatalf("day %+v", d)
	}
	if *d.Starts != 48 || d.StartsSource != "samples" || *d.RuntimeHours != 8 || *d.AvgCycleMin != 10 {
		t.Fatalf("cycles: starts %v runtime %v cycle %v", *d.Starts, *d.RuntimeHours, *d.AvgCycleMin)
	}
	if *d.Outdoor != 8 || *d.FlowRunning != 45 || *d.Spread != 5 || *d.DeviationAbove != 1 {
		t.Fatalf("temperatures %+v", d)
	}
	if len(in.Hints) != 2 || in.Hints[0].Level != "warn" {
		t.Fatalf("hints %+v", in.Hints)
	}

	// the controller's start counter wins over sampled transitions
	var counter []store.HistoryRow
	for m := 0; m <= 24*60; m += 15 {
		counter = append(counter, store.HistoryRow{Kind: protocol.MP, ID: mpCompressorStarts, Time: day.Add(time.Duration(m) * time.Minute), Value: 1000 + float64(m/30)*2})
	}
	st.AddHistory(ctx, counter)
	in, _ = svc.Insights(ctx, 2)
	if d := in.Days[0]; *d.Starts != 96 || d.StartsSource != "counter" {
		t.Fatalf("counter: %v %s", *d.Starts, d.StartsSource)
	}
}

func TestHistoryStoresChangesOnly(t *testing.T) {
	svc, _, ctx := newTestService(t, true)
	from := time.Now().Add(-time.Hour)
	c, _ := svc.Client()
	for range 3 {
		if err := svc.poll(ctx, c, false); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := svc.store.HistoryPoints(ctx, store.PointRef{Kind: protocol.SP, ID: 13}, from, time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 { // the operating mode did not change
		t.Fatalf("mode rows %d", len(rows))
	}
	if rows, _ := svc.store.HistoryPoints(ctx, store.PointRef{Kind: protocol.MP, ID: 0}, from, time.Now().Add(time.Minute)); len(rows) == 0 {
		t.Fatal("outdoor temperature not recorded")
	}
}

func TestInsightsRequestsAndEnergy(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := config.Load(filepath.Join(dir, "c.yaml"))
	st, err := store.Open(filepath.Join(dir, "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx := context.Background()

	// yesterday: the compressor runs all day; heating, except hot water
	// 06:00–07:00 and 18:00–18:30 (two interruptions); 48 kWh delivered for 12 kWh
	now := time.Now()
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location()).AddDate(0, 0, -1)
	var rows []store.HistoryRow
	add := func(id int, at time.Time, v float64) {
		rows = append(rows, store.HistoryRow{Kind: protocol.MP, ID: id, Time: at, Value: v})
	}
	for m := 0; m <= 24*60; m += 15 {
		at := day.Add(time.Duration(m) * time.Minute)
		req := float64(requestHeating)
		if (m >= 6*60 && m < 7*60) || (m >= 18*60 && m < 18*60+30) {
			req = requestDHW
		}
		if m < 24*60 {
			add(mpCompressor, at, 1)
			add(mpOutdoor, at, 2)
			add(mpRequest, at, req)
		}
		add(mpHeatOut, at, 1000+float64(m)/30)
		add(mpHeatIn, at, 500+float64(m)/120)
	}
	if err := st.AddHistory(ctx, rows); err != nil {
		t.Fatal(err)
	}
	in, err := svc.Insights(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	d := in.Days[0]
	if d.DHWInterrupts == nil || *d.DHWInterrupts != 2 || *d.DHWHours != 1.5 || *d.HeatingHours != 22.5 {
		t.Fatalf("requests %+v", d)
	}
	if *d.HeatOutKWh != 48 || *d.HeatInKWh != 12 || *d.COP != 4 {
		t.Fatalf("energy out %v in %v cop %v", *d.HeatOutKWh, *d.HeatInKWh, *d.COP)
	}
}

func TestHintRules(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	i := func(v int) *int { return &v }
	day := func(outdoor, flow, spread float64, starts int, runtime float64) DayInsight {
		return DayInsight{Coverage: 1, Starts: i(starts), RuntimeHours: f(runtime), Outdoor: f(outdoor), FlowRunning: f(flow), Spread: f(spread)}
	}
	titles := func(hs []Hint) []string {
		var out []string
		for _, h := range hs {
			out = append(out, h.Title.EN)
		}
		return out
	}
	has := func(hs []Hint, title string) bool { return slices.Contains(titles(hs), title) }

	// mild, low curve, 10 starts of 15 min: output above demand, not "cycles often"
	var days []DayInsight
	for range 7 {
		days = append(days, day(9, 32, 1.5, 10, 2.5))
	}
	hs := hints(days, hintSettings{})
	if !has(hs, "Output above demand") || has(hs, "Compressor cycles often") || !has(hs, "Very small temperature spread") {
		t.Fatalf("oversized: %v", titles(hs))
	}
	for _, h := range hs {
		if h.Title.EN == "Output above demand" && len(h.Tips) == 0 {
			t.Fatal("expected community tips")
		}
	}

	// hot water interrupts heating twice a day; heating on warm days above the limit
	days = days[:0]
	for range 7 {
		d := day(16, 30, 5, 3, 6)
		d.DHWInterrupts, d.HeatingHours = i(2), f(4)
		days = append(days, d)
	}
	hs = hints(days, hintSettings{HeatingLimit: f(15), CurveAtLimit: f(22)})
	if !has(hs, "Hot water interrupts heating") || !has(hs, "Heating above the heating limit") {
		t.Fatalf("dhw/limit: %v", titles(hs))
	}

	// SPF comparison of two weeks
	days = days[:0]
	for w := range 2 {
		for range 7 {
			d := day(3, 33, 5, 4, 8)
			d.HeatInKWh, d.HeatOutKWh = f(10), f(30+float64(w)*10) // 3.0 → 4.0
			days = append(days, d)
		}
	}
	hs = hints(days, hintSettings{})
	if !has(hs, "SPF improved") {
		t.Fatalf("spf: %v", titles(hs))
	}
}
