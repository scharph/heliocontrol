package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"heliocontrol/internal/config"
	"heliocontrol/internal/protocol"
	"heliocontrol/internal/sim"
	"heliocontrol/internal/store"
)

func newTestService(t *testing.T, readOnly bool) (*Service, *sim.Device, context.Context) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	dev := sim.New(sim.DefaultOptions(), nil)
	dev.Latency = 0
	go dev.Serve(l)
	host, port, _ := net.SplitHostPort(l.Addr().String())
	p, _ := strconv.Atoi(port)

	dir := t.TempDir()
	cfg, err := config.Load(filepath.Join(dir, "cfg.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cfg.Update(func(c *config.Config) {
		c.Device.Type, c.Device.Host, c.Device.Port, c.Device.AutoConnect = "tcp", host, p, true
		c.ReadOnly = &readOnly
	}); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(dir, "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	svc := New(cfg, st, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go svc.Run(ctx)
	waitFor(t, func() bool { s := svc.Snapshot(); return s.Status == Online && !s.Updated.IsZero() })
	return svc, dev, ctx
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timeout waiting for condition")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestServiceReadOnly(t *testing.T) {
	svc, dev, ctx := newTestService(t, true)
	snap := svc.Snapshot()
	if snap.DeviceID != 123456 || !snap.ReadOnly || len(snap.Panels) < 4 {
		t.Fatalf("snapshot %+v", snap)
	}
	if _, ok := snap.MP[3]; !ok {
		t.Fatal("flow temperature not polled")
	}
	before := dev.Handle(protocol.ReadSP(69))[0]
	if _, err := svc.WriteSP(ctx, 69, 23); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("expected read-only error, got %v", err)
	}
	if err := svc.ResetFault(ctx); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("reset fault: %v", err)
	}
	if err := svc.SyncClock(ctx); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("sync clock: %v", err)
	}
	if _, err := svc.DownloadTrend(ctx, true); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("trend download with delete: %v", err)
	}
	if after := dev.Handle(protocol.ReadSP(69))[0]; after != before {
		t.Fatal("device changed in read-only mode")
	}
	// reading operations work
	rec, err := svc.DownloadTrend(ctx, false)
	if err != nil || rec.Samples == 0 {
		t.Fatalf("trend download: %+v %v", rec, err)
	}
	tps, err := svc.TimePrograms(ctx)
	if err != nil || len(tps) != 3 || len(tps[0].Events) != 42 {
		t.Fatalf("time programs: %d %v", len(tps), err)
	}
	if _, err := svc.Clock(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestTieredPolling(t *testing.T) {
	svc, dev, ctx := newTestService(t, true)
	c, err := svc.Client()
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return len(svc.PointMeta(protocol.SP)) > 0 })
	before := dev.CommandCount("SP")
	if err := svc.poll(ctx, c, false); err != nil {
		t.Fatal(err)
	}
	if n := dev.CommandCount("SP") - before; n != 2 { // compressor state + running time
		t.Fatalf("fast poll read %d setup points, want 2", n)
	}
	before = dev.CommandCount("SP")
	if err := svc.Refresh(ctx); err != nil {
		t.Fatal(err)
	}
	if n := dev.CommandCount("SP") - before; n < 20 {
		t.Fatalf("full refresh read only %d setup points", n)
	}
}

func TestServiceWrites(t *testing.T) {
	svc, _, ctx := newTestService(t, false)
	waitFor(t, func() bool { return len(svc.PointMeta(protocol.SP)) > 0 }) // point init job
	got, err := svc.WriteSP(ctx, 69, 21.5)
	if err != nil || got != 21.5 || svc.Snapshot().SP[69] != 21.5 {
		t.Fatalf("write: %v %v", got, err)
	}
	var ve ValidationError
	if _, err := svc.WriteSP(ctx, 69, 99); !errors.As(err, &ve) {
		t.Fatalf("out of range should be refused: %v", err)
	}
	if _, err := svc.WriteSP(ctx, 13, 10); !errors.As(err, &ve) {
		t.Fatalf("main switch mode should be refused: %v", err)
	}
	if _, err := svc.WriteSP(ctx, 30, 1); !errors.As(err, &ve) {
		t.Fatalf("point outside pages needs expert mode: %v", err)
	}

	tps, err := svc.TimePrograms(ctx)
	if err != nil {
		t.Fatal(err)
	}
	evs := tps[1].Events
	day0 := evs[:tps[1].EAD]
	day0[1].State = 1 - day0[1].State
	n, err := svc.SaveTimeProgram(ctx, tps[1].ID, day0)
	if err != nil || n != 1 {
		t.Fatalf("save time program: %d %v", n, err)
	}
	bad := append([]protocol.TimeProgramEvent(nil), day0...)
	bad[2].Begin += 15
	if _, err := svc.SaveTimeProgram(ctx, tps[1].ID, bad); !errors.As(err, &ve) {
		t.Fatalf("gap should be refused: %v", err)
	}

	rec, err := svc.StartRecording(ctx, "test", []int{0, 3}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, func() bool { return svc.Snapshot().Recording != nil && svc.Snapshot().Recording.Samples >= 4 })
	if err := svc.StopRecording(); err != nil {
		t.Fatal(err)
	}
	series, err := svc.store.Samples(ctx, rec.ID, time.Time{}, time.Time{})
	if err != nil || len(series) != 2 {
		t.Fatalf("samples %v %v", series, err)
	}
}

func TestWriteBudgetAndNoOp(t *testing.T) {
	svc, dev, ctx := newTestService(t, false)
	waitFor(t, func() bool { return len(svc.PointMeta(protocol.SP)) > 0 })
	svc.cfg.Update(func(c *config.Config) { c.WriteBudgetPerDay = 2 })

	writes := func() int { return dev.CommandCount("SP") }
	if _, err := svc.WriteSP(ctx, 69, 21); err != nil {
		t.Fatal(err)
	}
	// same value again: acknowledged without a write (only the fresh read)
	before := writes()
	if _, err := svc.WriteSP(ctx, 69, 21); err != nil {
		t.Fatal(err)
	}
	if n := writes() - before; n != 1 {
		t.Fatalf("no-op write sent %d commands, want 1 read", n)
	}
	if _, err := svc.WriteSP(ctx, 69, 22); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.WriteSP(ctx, 69, 23); !errors.Is(err, ErrWriteBudget) {
		t.Fatalf("third change should exceed the budget of 2: %v", err)
	}
	if !strings.Contains(dev.Handle(protocol.ReadSP(69))[0], "VAL=22,") {
		t.Fatal("refused write reached the controller")
	}
}

func TestTemporarySetpoints(t *testing.T) {
	svc, dev, ctx := newTestService(t, false)
	waitFor(t, func() bool { return len(svc.PointMeta(protocol.SP)) > 0 })
	val := func() string { return dev.Handle(protocol.ReadSP(83))[0] }
	if !strings.Contains(val(), "VAL=48,") {
		t.Fatalf("unexpected start value %s", val())
	}
	o, err := svc.SetTemporary(ctx, 83, 55, time.Hour, "test")
	if err != nil || o.Original != 48 || o.Value != 55 {
		t.Fatalf("set: %+v %v", o, err)
	}
	// a second temporary value keeps the original
	if o, _ = svc.SetTemporary(ctx, 83, 58, time.Hour, "test"); o.Original != 48 || !strings.Contains(val(), "VAL=58,") {
		t.Fatalf("replace: %+v %s", o, val())
	}
	if _, err := svc.SetTemporary(ctx, 83, 55, time.Second, ""); err == nil {
		t.Fatal("too short duration accepted")
	}
	// expire it and let the restore run
	svc.mu.Lock()
	ov := svc.overrides[83]
	ov.Until = time.Now().Add(-time.Second)
	svc.overrides[83] = ov
	svc.mu.Unlock()
	svc.restoreExpired(ctx, map[int]bool{})
	if !strings.Contains(val(), "VAL=48,") || svc.HasOverrides() {
		t.Fatalf("not restored: %s", val())
	}

	// a permanent write replaces a temporary value without restoring it later
	svc.SetTemporary(ctx, 83, 52, time.Hour, "")
	svc.WriteSP(ctx, 83, 50)
	if svc.HasOverrides() || !strings.Contains(val(), "VAL=50,") {
		t.Fatal("permanent write did not end the temporary value")
	}
}

func TestTemporarySetpointSurvivesRestart(t *testing.T) {
	svc, dev, ctx := newTestService(t, false)
	waitFor(t, func() bool { return len(svc.PointMeta(protocol.SP)) > 0 })
	if _, err := svc.SetTemporary(ctx, 83, 55, time.Hour, ""); err != nil {
		t.Fatal(err)
	}
	// the persisted override, as a restarted server finds it
	ovs, err := svc.store.Overrides(ctx)
	if err != nil || len(ovs) != 1 || ovs[0].Original != 48 {
		t.Fatalf("persisted: %+v %v", ovs, err)
	}
	ovs[0].Until = time.Now().Add(-time.Minute)
	svc.store.SaveOverride(ctx, ovs[0])
	restarted := New(svc.cfg, svc.store, svc.log)
	rctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go restarted.Run(rctx)
	waitFor(t, func() bool { return restarted.Snapshot().Status == Online && restarted.HasOverrides() })
	restarted.restoreExpired(rctx, map[int]bool{})
	if !strings.Contains(dev.Handle(protocol.ReadSP(83))[0], "VAL=48,") {
		t.Fatal("expired override not restored after restart")
	}
}
