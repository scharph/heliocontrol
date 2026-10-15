package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strconv"
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
