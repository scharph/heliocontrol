package device

import (
	"context"
	"errors"
	"net"
	"strconv"
	"testing"
	"time"

	"heliocontrol/internal/protocol"
	"heliocontrol/internal/sim"
)

func startSim(t *testing.T, o sim.Options) (*sim.Device, TransportConfig) {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	d := sim.New(o, nil)
	d.Latency = 0
	go d.Serve(l)
	host, port, _ := net.SplitHostPort(l.Addr().String())
	p, _ := strconv.Atoi(port)
	return d, TransportConfig{Type: "tcp", Host: host, Port: p}
}

func TestClientAgainstSimulator(t *testing.T) {
	_, cfg := startSim(t, sim.DefaultOptions())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := Dial(ctx, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.Login(ctx); err != nil {
		t.Fatal(err)
	}
	if id, err := c.ReadID(ctx); err != nil || id != 123456 {
		t.Fatalf("id %d %v", id, err)
	}
	all, err := c.ReadAll(ctx, protocol.MP, nil)
	if err != nil || len(all) != 100 || all[3].Name != "Vorlauf" {
		t.Fatalf("ReadAll: %d %v", len(all), err)
	}
	vals, err := c.ReadValues(ctx, []int{0, 3, 4})
	if err != nil || len(vals) != 3 {
		t.Fatalf("ReadValues %v %v", vals, err)
	}
	got, err := c.WriteSP(ctx, 69, 21.5)
	if err != nil || got != 21.5 {
		t.Fatalf("WriteSP %v %v", got, err)
	}
	pts, err := c.ReadPoints(ctx, protocol.SP, []int{9, 13, 69})
	if err != nil || pts[69].Val != 21.5 || len(pts) != 3 {
		t.Fatalf("ReadPoints %+v %v", pts, err)
	}

	want := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := c.SetClock(ctx, want); err != nil {
		t.Fatal(err)
	}
	if clk, err := c.ReadClock(ctx); err != nil || clk.Sub(want).Abs() > 3*time.Second {
		t.Fatalf("clock %v %v", clk, err)
	}

	n, err := c.TrendCount(ctx)
	if err != nil || n == 0 {
		t.Fatalf("trend count %d %v", n, err)
	}
	recs, err := c.TrendDownload(ctx, true, false, nil)
	if err != nil || len(recs) != n || recs[0].ID != 0 {
		t.Fatalf("trend download %d/%d %v", len(recs), n, err)
	}

	tps, err := c.TimePrograms(ctx)
	if err != nil || len(tps) != 3 {
		t.Fatalf("time programs %+v %v", tps, err)
	}
	evs, err := c.TimeProgramEvents(ctx, tps[1])
	if err != nil || len(evs) != tps[1].EAD*tps[1].NOD {
		t.Fatalf("events %d %v", len(evs), err)
	}
	ev := evs[3]
	ev.State = 1 - ev.State
	if err := c.WriteTimeProgramEvent(ctx, ev); err != nil {
		t.Fatal(err)
	}
	evs2, _ := c.TimeProgramEvents(ctx, tps[1])
	if evs2[3].State != ev.State {
		t.Fatal("time program event not stored")
	}
}

func TestFaultReset(t *testing.T) {
	o := sim.DefaultOptions()
	o.Fault = true
	_, cfg := startSim(t, o)
	ctx := context.Background()
	c, err := Dial(ctx, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	p, _ := c.ReadPoint(ctx, protocol.MP, 31)
	if p.Val != 1 {
		t.Fatal("expected active fault")
	}
	if err := c.ResetFault(ctx); err != nil {
		t.Fatal(err)
	}
	if p, _ = c.ReadPoint(ctx, protocol.MP, 31); p.Val != 0 {
		t.Fatal("fault still active")
	}
}

func TestReadOnlyRefusesWrites(t *testing.T) {
	d, cfg := startSim(t, sim.DefaultOptions())
	ctx := context.Background()
	c, err := Dial(ctx, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	c.SetReadOnly(true)
	before := d.Handle(protocol.ReadSP(69))[0]
	if _, err := c.WriteSP(ctx, 69, 24); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("WriteSP: %v", err)
	}
	if err := c.SetClock(ctx, time.Now()); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("SetClock: %v", err)
	}
	if err := c.TrendStop(ctx); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("TrendStop: %v", err)
	}
	if _, err := c.TrendDownload(ctx, true, false, nil); !errors.Is(err, ErrReadOnly) {
		t.Fatalf("TrendDownload with pause: %v", err)
	}
	if after := d.Handle(protocol.ReadSP(69))[0]; after != before {
		t.Fatalf("setup point changed: %q -> %q", before, after)
	}
	// reads still work
	if err := c.Login(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.TrendDownload(ctx, false, false, nil); err != nil {
		t.Fatal(err)
	}
}

func TestDisconnectDetected(t *testing.T) {
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	defer l.Close()
	go func() {
		conn, err := l.Accept()
		if err == nil {
			conn.Close()
		}
	}()
	host, port, _ := net.SplitHostPort(l.Addr().String())
	p, _ := strconv.Atoi(port)
	c, err := Dial(context.Background(), TransportConfig{Host: host, Port: p}, nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-c.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("disconnect not detected")
	}
	if _, err := c.Request(context.Background(), protocol.CmdLogin, time.Second); err == nil {
		t.Fatal("expected error on closed client")
	}
}
