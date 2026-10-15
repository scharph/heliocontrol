// heliocontrol is a web server for Heliotherm heat pumps with a "web control"
// controller: it replaces the Windows tool homeControl and bridges the heat
// pump to Home Assistant over MQTT.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"heliocontrol/internal/api"
	"heliocontrol/internal/config"
	"heliocontrol/internal/mqtt"
	"heliocontrol/internal/service"
	"heliocontrol/internal/store"
	"heliocontrol/web"
)

var version = "dev"

func main() {
	cfgPath := flag.String("config", "heliocontrol.yaml", "configuration file (created on first save)")
	debug := flag.Bool("debug", false, "log protocol traffic")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	if err := run(*cfgPath, log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(cfgPath string, log *slog.Logger) error {
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return err
	}
	c := cfg.Get()
	if err := os.MkdirAll(c.DataDir, 0o755); err != nil {
		return err
	}
	st, err := store.Open(filepath.Join(c.DataDir, "heliocontrol.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	svc := service.New(cfg, st, log)
	go svc.Run(ctx)

	bridge := mqtt.New(svc, log)
	bridge.Configure(c)
	defer bridge.Close()

	srv := api.New(svc, cfg, st, web.FS(), version, log)
	srv.OnSettings = func(old, cur config.Config) {
		svc.SettingsChanged(old, cur)
		bridge.Configure(cur)
	}
	hs := &http.Server{Addr: c.Listen, Handler: srv.Handler(), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		hs.Shutdown(sctx)
	}()
	log.Info("heliocontrol started", "version", version, "listen", c.Listen, "device", c.Device.TransportConfig.String(), "readOnly", c.IsReadOnly())
	if err := hs.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	svc.Disconnect()
	time.Sleep(200 * time.Millisecond) // let the logout go out
	return nil
}
