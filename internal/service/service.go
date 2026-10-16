// Package service is the heart of the server: it keeps one session to the heat
// pump, polls the values shown in the UI, records data, runs long jobs (trend
// download, point initialization) and publishes everything on an event bus.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"sync"
	"time"

	"heliocontrol/internal/catalog"
	"heliocontrol/internal/config"
	"heliocontrol/internal/device"
	"heliocontrol/internal/protocol"
	"heliocontrol/internal/store"
)

// Connection states.
const (
	Offline    = "offline"
	Connecting = "connecting"
	Online     = "online"
	Failed     = "error"
)

// Snapshot is the current state shown by the UI.
type Snapshot struct {
	Status       string               `json:"status"`
	Error        string               `json:"error,omitempty"`
	Target       string               `json:"target"`
	ReadOnly     bool                 `json:"readOnly"`
	ExpertMode   bool                 `json:"expertMode"`
	DeviceID     int                  `json:"deviceId,omitempty"`
	Firmware     string               `json:"firmware,omitempty"`
	Installation catalog.Installation `json:"installation"`
	Panels       []catalog.Panel      `json:"panels"`
	MP           map[int]float64      `json:"mp"`
	SP           map[int]float64      `json:"sp"`
	Updated      time.Time            `json:"updated,omitzero"`
	PollSeconds  int                  `json:"pollSeconds"`
	Job          *Job                 `json:"job,omitempty"`
	Recording    *store.Recording     `json:"recording,omitempty"`
	Overrides    []store.Override     `json:"overrides"`
}

// Job is a long-running device operation with progress.
type Job struct {
	Name  string `json:"name"`
	Done  int    `json:"done"`
	Total int    `json:"total"`
}

type Service struct {
	cfg   *config.Store
	store *store.Store
	log   *slog.Logger
	Bus   *Bus

	mu     sync.RWMutex
	client *device.Client
	snap   Snapshot
	meta   map[protocol.Kind]map[int]protocol.Point
	want   bool
	wake   chan struct{}
	jobs   chan struct{} // one long job at a time; others wait their turn
	rec    *recorder
	cancel context.CancelFunc
	// lastSettings is when the (slowly changing) settings were last read
	lastSettings time.Time
	budget       writeBudget
	history      historyRecorder
	overrides    map[int]store.Override // temporary setpoints by SP
}

var (
	ErrOffline  = errors.New("not connected to the heat pump")
	ErrReadOnly = errors.New("read-only mode: writing to the heat pump is disabled in the settings")
	ErrBusy     = errors.New("another operation is running")
)

func New(cfg *config.Store, st *store.Store, log *slog.Logger) *Service {
	s := &Service{cfg: cfg, store: st, log: log, Bus: NewBus(), wake: make(chan struct{}, 1), jobs: make(chan struct{}, 1), overrides: map[int]store.Override{},
		meta: map[protocol.Kind]map[int]protocol.Point{protocol.MP: {}, protocol.SP: {}}}
	s.snap = Snapshot{Status: Offline, MP: map[int]float64{}, SP: map[int]float64{}}
	s.snap.Panels = catalog.Resolve(catalog.Installation{})
	return s
}

// Run drives the connection until ctx is cancelled.
func (s *Service) Run(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	if err := s.store.StopDangling(ctx); err != nil {
		s.log.Warn("close interrupted recordings", "err", err)
	}
	if ovs, err := s.store.Overrides(ctx); err == nil {
		s.mu.Lock()
		for _, o := range ovs {
			s.overrides[o.SP] = o
		}
		s.mu.Unlock()
	}
	go s.overrideLoop(ctx)
	go s.historyLoop(ctx)
	for _, kind := range []protocol.Kind{protocol.MP, protocol.SP} {
		if pts, err := s.store.Points(ctx, kind); err == nil {
			for _, p := range pts {
				s.meta[kind][p.ID] = p
			}
		}
	}
	s.want = s.cfg.Get().Device.AutoConnect
	s.refreshSettings()
	backoff := 5 * time.Second
	for ctx.Err() == nil {
		s.mu.RLock()
		want := s.want
		s.mu.RUnlock()
		if !want {
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
			}
			continue
		}
		err := s.session(ctx)
		if ctx.Err() != nil {
			return
		}
		s.mu.RLock()
		want = s.want
		s.mu.RUnlock()
		if err != nil && want {
			s.setStatus(Failed, err.Error())
			s.event("error", "Verbindung unterbrochen", err.Error())
			select {
			case <-ctx.Done():
				return
			case <-s.wake:
			case <-time.After(backoff):
			}
			backoff = min(2*backoff, time.Minute)
			continue
		}
		backoff = 5 * time.Second
	}
}

// Connect / Disconnect set whether a session is wanted.
func (s *Service) Connect() {
	s.mu.Lock()
	s.want = true
	s.mu.Unlock()
	s.poke()
}

func (s *Service) Disconnect() {
	s.mu.Lock()
	s.want = false
	c := s.client
	s.mu.Unlock()
	if c != nil {
		c.Close()
	}
	s.poke()
}

// Reconnect drops the session so new connection settings take effect.
func (s *Service) Reconnect() {
	s.mu.RLock()
	c := s.client
	s.mu.RUnlock()
	if c != nil {
		c.Close()
	}
	s.refreshSettings()
	s.poke()
}

func (s *Service) poke() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Service) refreshSettings() {
	cfg := s.cfg.Get()
	s.mu.Lock()
	s.snap.Target = cfg.Device.TransportConfig.String()
	s.snap.ReadOnly = cfg.IsReadOnly()
	s.snap.ExpertMode = cfg.ExpertMode
	s.snap.PollSeconds = cfg.PollSeconds
	if s.client != nil {
		s.client.SetReadOnly(cfg.IsReadOnly())
	}
	s.mu.Unlock()
	s.publishState()
}

// SettingsChanged applies changed configuration.
func (s *Service) SettingsChanged(old, cur config.Config) {
	s.refreshSettings()
	if old.Device.TransportConfig != cur.Device.TransportConfig || old.Device.ExpectedID != cur.Device.ExpectedID {
		s.Reconnect()
	}
}

func (s *Service) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	c := s.snap
	c.MP = maps.Clone(s.snap.MP)
	c.SP = maps.Clone(s.snap.SP)
	c.Overrides = s.overrideList()
	return c
}

func (s *Service) setStatus(status, msg string) {
	s.mu.Lock()
	s.snap.Status, s.snap.Error = status, msg
	s.mu.Unlock()
	s.publishState()
}

func (s *Service) publishState() { s.Bus.Publish(Event{Type: "state", Data: s.Snapshot()}) }

func (s *Service) event(level, title, detail string) {
	e := store.Event{Level: level, Title: title, Detail: detail}
	if err := s.store.AddEvent(context.Background(), &e); err != nil {
		s.log.Warn("store event", "err", err)
	}
	s.Bus.Publish(Event{Type: "log", Data: e})
	s.log.Info(title, "level", level, "detail", detail)
}

// Client returns the live session or ErrOffline.
func (s *Service) Client() (*device.Client, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.client == nil || s.snap.Status != Online {
		return nil, ErrOffline
	}
	return s.client, nil
}

// session connects, logs in and polls until the connection fails.
func (s *Service) session(ctx context.Context) error {
	cfg := s.cfg.Get()
	s.setStatus(Connecting, "")
	dctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	c, err := device.Dial(dctx, cfg.Device.TransportConfig, s.log)
	cancel()
	if err != nil {
		return fmt.Errorf("connect %s: %w", cfg.Device.TransportConfig, err)
	}
	c.SetReadOnly(cfg.IsReadOnly())
	defer c.Close()
	if err := c.Login(ctx); err != nil {
		return err
	}
	id, err := c.ReadID(ctx)
	if err != nil {
		return fmt.Errorf("read controller id: %w", err)
	}
	if cfg.Device.ExpectedID != 0 && id != cfg.Device.ExpectedID {
		s.mu.Lock()
		s.want = false
		s.mu.Unlock()
		return fmt.Errorf("controller id %d does not match the configured id %d", id, cfg.Device.ExpectedID)
	}
	conf, err := c.ReadPoints(ctx, protocol.SP, catalog.ConfigPoints)
	if err != nil {
		return fmt.Errorf("read installation: %w", err)
	}
	inst := catalog.Installation{
		PlantType: int(conf[protocol.PlantTypeSP].Val),
		Mixer1:    conf[199].Val > 0,
		Mixer2:    conf[222].Val > 0,
		Energy:    conf[263].Val > 0,
	}
	firmware := strings.TrimSpace(conf[protocol.PlantTypeSP].Name) // SP 9 carries the firmware version as its name

	s.mu.Lock()
	s.client = c
	s.lastSettings = time.Time{} // first poll of a session reads everything
	s.snap.DeviceID, s.snap.Firmware, s.snap.Installation = id, firmware, inst
	s.snap.Status, s.snap.Error = Online, ""
	s.mu.Unlock()
	s.rebuildPanels()
	s.event("connect", "Verbunden", fmt.Sprintf("%s, Regler %d, Firmware %s", cfg.Device.TransportConfig, id, firmware))
	defer func() {
		s.mu.Lock()
		s.client = nil
		if s.snap.Status == Online {
			s.snap.Status = Offline
		}
		s.mu.Unlock()
		s.publishState()
		s.event("disconnect", "Verbindung getrennt", "")
	}()

	s.mu.RLock()
	needInit := len(s.meta[protocol.MP]) == 0 || len(s.meta[protocol.SP]) == 0
	s.mu.RUnlock()

	failures := 0
	for {
		err := s.poll(ctx, c, false)
		if err == nil && needInit {
			// names and limits of all points; after the first poll so values show up quickly
			needInit = false
			go func() {
				if err := s.InitPoints(ctx); err != nil && !errors.Is(err, ErrBusy) {
					s.event("warn", "Datenpunkte konnten nicht initialisiert werden", err.Error())
				}
			}()
		}
		if err != nil {
			if errors.Is(err, device.ErrClosed) || ctx.Err() != nil {
				return err
			}
			if failures++; failures >= 3 {
				return fmt.Errorf("polling failed: %w", err)
			}
			s.log.Warn("poll failed", "err", err)
		} else {
			failures = 0
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-c.Done():
			return c.Err()
		case <-time.After(time.Duration(s.cfg.Get().PollSeconds) * time.Second):
		}
	}
}

// Values is the compact value payload sent on every poll.
type Values struct {
	MP      map[int]float64 `json:"mp"`
	SP      map[int]float64 `json:"sp"`
	Updated time.Time       `json:"updated"`
}

func (s *Service) Values() Values {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Values{MP: maps.Clone(s.snap.MP), SP: maps.Clone(s.snap.SP), Updated: s.snap.Updated}
}

// rebuildPanels resolves the pages for the installation, hides items whose
// point the controller reports as unused, applies the controller's limits and
// adds a diagnostics page with all other used measuring points.
func (s *Service) rebuildPanels() {
	s.mu.Lock()
	panels := catalog.Resolve(s.snap.Installation)
	onPage := map[int]bool{}
	for i := range panels {
		items := panels[i].Items[:0:0]
		for _, it := range panels[i].Items {
			if it.Display != catalog.Ratio {
				m, known := s.meta[it.Kind][it.ID]
				if known && !m.US {
					continue
				}
				if known && it.Writable && m.Max > m.Min {
					it.Min, it.Max = m.Min, m.Max
				}
				if known && it.Display == catalog.Enum && m.Max > 0 && int(m.Max)+1 < len(it.Options) {
					it.Options = it.Options[:int(m.Max)+1-it.Offset]
				}
				if it.Kind == protocol.MP {
					onPage[it.ID] = true
				}
			}
			items = append(items, it)
		}
		panels[i].Items = items
	}
	var diag []catalog.Item
	ids := slices.Sorted(maps.Keys(s.meta[protocol.MP]))
	for _, id := range ids {
		m := s.meta[protocol.MP][id]
		if !m.US || onPage[id] {
			continue
		}
		diag = append(diag, diagItem(m))
	}
	if len(diag) > 0 {
		panels = append(panels, catalog.Panel{ID: "diagnostics", Title: catalog.Text{DE: "Diagnose", EN: "Diagnostics"},
			Subtitle: catalog.Text{DE: "Weitere Messpunkte des Reglers", EN: "Further measuring points of the controller"}, Items: diag})
	}
	s.snap.Panels = panels
	s.mu.Unlock()
	s.publishState()
}

// diagItem derives a display item from the controller's own point description.
func diagItem(m protocol.Point) catalog.Item {
	name := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(m.Name), ":"))
	it := catalog.Item{Key: fmt.Sprintf("mp_%d", m.ID), Kind: protocol.MP, ID: m.ID, Label: catalog.Text{DE: name, EN: name}, Display: catalog.Value,
		HA: catalog.HA{Component: "sensor", StateClass: "measurement", Category: "diagnostic"}}
	lower := strings.ToLower(name)
	switch {
	case m.Min == 0 && m.Max == 1:
		it.Display = catalog.Bool
		it.HA = catalog.HA{Component: "binary_sensor", DeviceClass: "running", Category: "diagnostic"}
	case strings.HasPrefix(lower, "temp") || strings.Contains(lower, "temp.") || strings.Contains(lower, "sollwert"):
		it.Unit, it.HA.DeviceClass = "°C", "temperature"
	case strings.Contains(lower, "(bar)") || strings.Contains(lower, "druck"):
		it.Unit, it.HA.DeviceClass = "bar", "pressure"
	case strings.Contains(lower, "(%)") || strings.HasPrefix(lower, "ao") || strings.Contains(lower, "ventil"):
		it.Unit = "%"
	case strings.Contains(lower, "(kwh)"):
		it.Unit, it.HA.DeviceClass, it.HA.StateClass = "kWh", "energy", "total_increasing"
	case strings.Contains(lower, "(w)"):
		it.Unit, it.HA.DeviceClass = "W", "power"
	case strings.Contains(lower, "frequenz"):
		it.Unit, it.HA.DeviceClass = "Hz", "frequency"
	}
	return it
}

// PointMeta returns cached metadata for all points of a kind.
func (s *Service) PointMeta(kind protocol.Kind) []protocol.Point {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := slices.Collect(maps.Values(s.meta[kind]))
	slices.SortFunc(out, func(a, b protocol.Point) int { return a.ID - b.ID })
	return out
}

// runJob serializes long operations (waiting for a running one to finish)
// and reports their progress.
func (s *Service) runJob(ctx context.Context, name string, fn func(progress func(done, total int)) error) error {
	select {
	case s.jobs <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.jobs }()
	set := func(j *Job) {
		s.mu.Lock()
		s.snap.Job = j
		s.mu.Unlock()
		s.Bus.Publish(Event{Type: "job", Data: j})
	}
	set(&Job{Name: name})
	defer set(nil)
	last := time.Time{}
	return fn(func(done, total int) {
		if time.Since(last) > 250*time.Millisecond || done == total {
			last = time.Now()
			set(&Job{Name: name, Done: done, Total: total})
		}
	})
}

// InitPoints reads names and limits of all points (homeControl's "initialize
// measuring points") and caches them.
func (s *Service) InitPoints(ctx context.Context) error {
	c, err := s.Client()
	if err != nil {
		return err
	}
	return s.runJob(ctx, "init-points", func(progress func(int, int)) error {
		for _, kind := range []protocol.Kind{protocol.MP, protocol.SP} {
			pts, err := c.ReadAll(ctx, kind, progress)
			if err != nil {
				return err
			}
			if err := s.store.SavePoints(ctx, pts); err != nil {
				return err
			}
			s.mu.Lock()
			for _, p := range pts {
				s.meta[kind][p.ID] = p
				if kind == protocol.MP {
					s.snap.MP[p.ID] = p.Val
				} else {
					s.snap.SP[p.ID] = p.Val
				}
			}
			s.mu.Unlock()
		}
		s.rebuildPanels()
		s.event("info", "Datenpunkte initialisiert", fmt.Sprintf("%d Messpunkte, %d Setuppunkte", len(s.meta[protocol.MP]), len(s.meta[protocol.SP])))
		return nil
	})
}

// ReadAllPoints reads every point of a kind fresh from the controller (for the
// "all points" page).
func (s *Service) ReadAllPoints(ctx context.Context, kind protocol.Kind) ([]protocol.Point, error) {
	c, err := s.Client()
	if err != nil {
		return nil, err
	}
	var pts []protocol.Point
	err = s.runJob(ctx, "read-"+string(kind), func(progress func(int, int)) error {
		pts, err = c.ReadAll(ctx, kind, progress)
		return err
	})
	if err != nil {
		return nil, err
	}
	_ = s.store.SavePoints(ctx, pts)
	s.mu.Lock()
	for _, p := range pts {
		s.meta[kind][p.ID] = p
	}
	s.mu.Unlock()
	return pts, nil
}
