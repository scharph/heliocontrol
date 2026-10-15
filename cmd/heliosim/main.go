// heliosim runs a simulated Heliotherm web control for development.
//
//	heliosim -listen 127.0.0.1:4001 [-fault] [-plant 0x2121]
package main

import (
	"flag"
	"log/slog"
	"net"
	"os"
	"strconv"

	"heliocontrol/internal/sim"
)

func main() {
	listen := flag.String("listen", "127.0.0.1:4001", "address to listen on")
	fault := flag.Bool("fault", false, "start with an active fault")
	plant := flag.String("plant", "", "installation bit mask (SP 9), e.g. 532 or 0x2121")
	debug := flag.Bool("debug", false, "log every command")
	flag.Parse()
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	o := sim.DefaultOptions()
	o.Fault = *fault
	if *plant != "" {
		v, err := strconv.ParseInt(*plant, 0, 32)
		if err != nil {
			log.Error("bad -plant", "err", err)
			os.Exit(2)
		}
		o.PlantType = int(v)
	}
	l, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Error("listen", "err", err)
		os.Exit(1)
	}
	log.Info("simulated web control listening", "addr", l.Addr(), "plantType", o.PlantType)
	if err := sim.New(o, log).Serve(l); err != nil {
		log.Error("serve", "err", err)
		os.Exit(1)
	}
}
