// heliocli is a diagnostic tool for a Heliotherm web control.
//
//	heliocli probe -host 10.0.0.5 [-port 4001] [-out captures/probe.txt]
//
// probe connects in READ-ONLY mode (writes are refused inside the client and
// never reach the wire), runs a fixed list of read commands and records the raw
// replies, which is how the reply formats of a real controller are verified.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"heliocontrol/internal/device"
	"heliocontrol/internal/protocol"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "probe" {
		fmt.Fprintln(os.Stderr, "usage: heliocli probe -host HOST [-port 4001] [-out FILE] [-full]")
		os.Exit(2)
	}
	fs := flag.NewFlagSet("probe", flag.ExitOnError)
	host := fs.String("host", "", "controller / LAN modem address")
	port := fs.Int("port", 4001, "TCP port")
	out := fs.String("out", "captures/probe.txt", "transcript file")
	full := fs.Bool("full", false, "also read all measuring and setup points (~RM / ~RS)")
	verbose := fs.Bool("v", false, "log every frame")
	fs.Parse(os.Args[2:])
	if *host == "" {
		fmt.Fprintln(os.Stderr, "-host is required")
		os.Exit(2)
	}
	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	if err := probe(*host, *port, *out, *full, log); err != nil {
		log.Error("probe failed", "err", err)
		os.Exit(1)
	}
}

func probe(host string, port int, out string, full bool, log *slog.Logger) error {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c, err := device.Dial(ctx, device.TransportConfig{Type: "tcp", Host: host, Port: port}, log)
	if err != nil {
		return err
	}
	c.SetReadOnly(true)
	defer c.Close()

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "# heliocli probe %s:%d at %s (read-only)\n", host, port, time.Now().Format(time.RFC3339))

	run := func(cmd string, n int, gap time.Duration) []string {
		start := time.Now()
		replies, err := c.RequestMany(ctx, cmd, n, gap)
		fmt.Fprintf(f, "\n> %s   (%d replies, %s)\n", cmd, len(replies), time.Since(start).Round(time.Millisecond))
		for _, r := range replies {
			fmt.Fprintf(f, "< %s\n", r)
		}
		if err != nil {
			fmt.Fprintf(f, "! %v\n", err)
			log.Warn("command failed", "cmd", cmd, "err", err)
		}
		return replies
	}
	count := func(replies []string) int {
		if len(replies) == 0 {
			return 0
		}
		n, _ := protocol.ParseCount(replies[0])
		return n
	}

	if r := run(protocol.CmdLogin, 1, 2*time.Second); len(r) == 0 {
		return fmt.Errorf("no answer to login; is another program (homeControl) connected?")
	}
	run(protocol.CmdReadID, 1, 2*time.Second)
	run(protocol.CmdClock, 1, 2*time.Second)
	for _, id := range []int{0, 3, 4, 31, 56} {
		run(protocol.ReadMP(id), 1, 2*time.Second)
	}
	for _, id := range []int{9, 10, 11, 13, 69, 222, 263} {
		run(protocol.ReadSP(id), 1, 2*time.Second)
	}
	run(protocol.ReadValues([]int{0, 3, 4, 57}), 4, 2*time.Second)
	if n := count(run(protocol.CmdTrendCount, 1, 2*time.Second)); n > 0 {
		run(protocol.ReadTrend([]int{0, 1, 2}), 3, 2*time.Second)
	}
	// time programs through the library (which matches replies to requests)
	tps, err := c.TimePrograms(ctx)
	fmt.Fprintf(f, "\n# time programs in use: %d (err=%v)\n", len(tps), err)
	for _, tp := range tps {
		evs, err := c.TimeProgramEvents(ctx, tp)
		fmt.Fprintf(f, "  %+v: %d events (err=%v)\n", tp, len(evs), err)
		for _, ev := range evs {
			if ev.Day == 0 {
				fmt.Fprintf(f, "    day %d event %d: state %d %s-%s\n", ev.Day, ev.Event, ev.State, ev.Begin, ev.End)
			}
		}
	}
	run(protocol.ReadTimeProgramEvent(0, 0, 0), 1, 2*time.Second)
	if full {
		for _, kind := range []protocol.Kind{protocol.MP, protocol.SP} {
			start := time.Now()
			pts, err := c.ReadAll(ctx, kind, nil)
			fmt.Fprintf(f, "\n# all %s points: %d in %s (err=%v)\n", kind, len(pts), time.Since(start).Round(time.Millisecond), err)
			for _, p := range pts {
				fmt.Fprintf(f, "  %s %3d %-30s val=%v min=%v max=%v used=%v writable=%v\n", kind, p.ID, p.Name, p.Val, p.Min, p.Max, p.US, p.WR)
			}
		}
	}
	fmt.Fprintln(f, "\n# done")
	log.Info("probe finished", "transcript", out)
	fmt.Println(strings.TrimSpace(out))
	return nil
}
