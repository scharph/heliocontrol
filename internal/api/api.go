// Package api is the HTTP interface: a JSON REST API under /api, a
// server-sent event stream for live updates, and the embedded web UI.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"

	"heliocontrol/internal/config"
	"heliocontrol/internal/device"
	"heliocontrol/internal/legacy"
	"heliocontrol/internal/protocol"
	"heliocontrol/internal/service"
	"heliocontrol/internal/store"
)

type Server struct {
	svc        *service.Service
	cfg        *config.Store
	store      *store.Store
	log        *slog.Logger
	web        fs.FS
	version    string
	OnSettings func(old, cur config.Config)

	sessMu   sync.Mutex
	sessions map[string]time.Time
}

func New(svc *service.Service, cfg *config.Store, st *store.Store, web fs.FS, version string, log *slog.Logger) *Server {
	return &Server{svc: svc, cfg: cfg, store: st, web: web, version: version, log: log, sessions: map[string]time.Time{}}
}

const sessionCookie = "heliocontrol_session"

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	h := func(pattern string, fn func(w http.ResponseWriter, r *http.Request) error) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			if err := fn(w, r); err != nil {
				s.writeError(w, r, err)
			}
		})
	}
	h("GET /api/auth", s.authInfo)
	h("POST /api/login", s.login)
	h("POST /api/logout", s.logout)
	h("GET /api/version", func(w http.ResponseWriter, r *http.Request) error {
		return writeJSON(w, map[string]string{"version": s.version})
	})
	h("GET /api/state", func(w http.ResponseWriter, r *http.Request) error { return writeJSON(w, s.svc.Snapshot()) })
	mux.HandleFunc("GET /api/events", s.events)
	h("POST /api/connect", func(w http.ResponseWriter, r *http.Request) error { s.svc.Connect(); return noContent(w) })
	h("POST /api/disconnect", func(w http.ResponseWriter, r *http.Request) error { s.svc.Disconnect(); return noContent(w) })
	h("POST /api/refresh", func(w http.ResponseWriter, r *http.Request) error {
		if err := s.svc.Refresh(r.Context()); err != nil {
			return err
		}
		return writeJSON(w, s.svc.Values())
	})

	h("GET /api/points/{kind}", s.points)
	h("POST /api/points/init", func(w http.ResponseWriter, r *http.Request) error {
		go func() {
			if err := s.svc.InitPoints(context.Background()); err != nil {
				s.log.Warn("init points", "err", err)
			}
		}()
		return accepted(w)
	})
	h("PUT /api/sp/{id}", s.writeSP)
	h("POST /api/mp/{id}/override", s.override)
	h("POST /api/fault/reset", func(w http.ResponseWriter, r *http.Request) error {
		if err := s.svc.ResetFault(r.Context()); err != nil {
			return err
		}
		return noContent(w)
	})
	h("GET /api/clock", func(w http.ResponseWriter, r *http.Request) error {
		c, err := s.svc.Clock(r.Context())
		if err != nil {
			return err
		}
		return writeJSON(w, c)
	})
	h("POST /api/clock/sync", func(w http.ResponseWriter, r *http.Request) error {
		if err := s.svc.SyncClock(r.Context()); err != nil {
			return err
		}
		return noContent(w)
	})

	h("GET /api/timeprograms", func(w http.ResponseWriter, r *http.Request) error {
		tps, err := s.svc.TimePrograms(r.Context())
		if err != nil {
			return err
		}
		return writeJSON(w, tps)
	})
	h("PUT /api/timeprograms/{id}", s.saveTimeProgram)

	h("GET /api/trend/points", func(w http.ResponseWriter, r *http.Request) error {
		pts, err := s.svc.TrendPoints(r.Context())
		if err != nil {
			return err
		}
		return writeJSON(w, pts)
	})
	h("PUT /api/trend/points", s.setTrendConfig)
	h("GET /api/trend/count", func(w http.ResponseWriter, r *http.Request) error {
		n, err := s.svc.TrendCount(r.Context())
		if err != nil {
			return err
		}
		return writeJSON(w, map[string]int{"count": n})
	})
	h("POST /api/trend/download", s.downloadTrend)

	h("GET /api/recordings", func(w http.ResponseWriter, r *http.Request) error {
		recs, err := s.store.Recordings(r.Context())
		if err != nil {
			return err
		}
		return writeJSON(w, recs)
	})
	h("POST /api/recordings", s.startRecording)
	h("POST /api/recordings/active/{action}", s.recordingAction)
	h("POST /api/recordings/import", s.importFile)
	h("GET /api/recordings/{id}", s.recording)
	h("GET /api/recordings/{id}/samples", s.samples)
	h("GET /api/recordings/{id}/csv", s.csv)
	h("PATCH /api/recordings/{id}", s.renameRecording)
	h("DELETE /api/recordings/{id}", s.deleteRecording)

	h("GET /api/log", func(w http.ResponseWriter, r *http.Request) error {
		evs, err := s.store.Events(r.Context(), 500)
		if err != nil {
			return err
		}
		return writeJSON(w, evs)
	})
	h("DELETE /api/log", func(w http.ResponseWriter, r *http.Request) error {
		if err := s.store.ClearEvents(r.Context()); err != nil {
			return err
		}
		return noContent(w)
	})

	h("GET /api/settings", s.getSettings)
	h("PUT /api/settings", s.putSettings)
	h("POST /api/settings/password", s.setPassword)
	h("GET /api/serialports", func(w http.ResponseWriter, r *http.Request) error {
		ports := device.SerialPorts()
		if ports == nil {
			ports = []string{}
		}
		return writeJSON(w, ports)
	})

	mux.Handle("/", s.static())
	return s.authMiddleware(mux)
}

// --- helpers ------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, v any) error {
	w.Header().Set("Content-Type", "application/json")
	return json.NewEncoder(w).Encode(v)
}

func noContent(w http.ResponseWriter) error { w.WriteHeader(http.StatusNoContent); return nil }
func accepted(w http.ResponseWriter) error  { w.WriteHeader(http.StatusAccepted); return nil }

type httpError struct {
	code int
	msg  string
}

func (e httpError) Error() string { return e.msg }

func badRequest(format string, a ...any) error {
	return httpError{http.StatusBadRequest, fmt.Sprintf(format, a...)}
}

func decode(r *http.Request, v any) error {
	dec := json.NewDecoder(io.LimitReader(r.Body, 1<<20))
	if err := dec.Decode(v); err != nil {
		return badRequest("invalid request body: %v", err)
	}
	return nil
}

func pathInt(r *http.Request, name string) (int64, error) {
	n, err := strconv.ParseInt(r.PathValue(name), 10, 64)
	if err != nil {
		return 0, badRequest("invalid %s", name)
	}
	return n, nil
}

func (s *Server) writeError(w http.ResponseWriter, r *http.Request, err error) {
	code := http.StatusBadGateway // device communication failed
	var he httpError
	var ve service.ValidationError
	switch {
	case errors.As(err, &he):
		code = he.code
	case errors.As(err, &ve):
		code = http.StatusBadRequest
	case errors.Is(err, service.ErrReadOnly), errors.Is(err, device.ErrReadOnly):
		code = http.StatusForbidden
	case errors.Is(err, service.ErrOffline):
		code = http.StatusServiceUnavailable
	case errors.Is(err, service.ErrBusy):
		code = http.StatusConflict
	case errors.Is(err, store.ErrNotFound):
		code = http.StatusNotFound
	case errors.Is(err, context.Canceled):
		return
	}
	if code >= 500 {
		s.log.Warn("request failed", "path", r.URL.Path, "err", err)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// --- auth -------------------------------------------------------------------

func (s *Server) authRequired() bool { return s.cfg.Get().PasswordHash != "" }

func (s *Server) authenticated(r *http.Request) bool {
	if !s.authRequired() {
		return true
	}
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	s.sessMu.Lock()
	defer s.sessMu.Unlock()
	exp, ok := s.sessions[c.Value]
	if !ok || time.Now().After(exp) {
		delete(s.sessions, c.Value)
		return false
	}
	s.sessions[c.Value] = time.Now().Add(30 * 24 * time.Hour)
	return true
}

func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		open := !strings.HasPrefix(p, "/api/") || p == "/api/auth" || p == "/api/login" || p == "/api/version"
		if !open && !s.authenticated(r) {
			s.writeError(w, r, httpError{http.StatusUnauthorized, "login required"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) authInfo(w http.ResponseWriter, r *http.Request) error {
	return writeJSON(w, map[string]bool{"required": s.authRequired(), "authenticated": s.authenticated(r)})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) error {
	var body struct{ Password string }
	if err := decode(r, &body); err != nil {
		return err
	}
	hash := s.cfg.Get().PasswordHash
	if hash == "" {
		return noContent(w)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(body.Password)) != nil {
		time.Sleep(500 * time.Millisecond)
		return httpError{http.StatusUnauthorized, "wrong password"}
	}
	s.startSession(w)
	return noContent(w)
}

func (s *Server) startSession(w http.ResponseWriter) {
	b := make([]byte, 32)
	rand.Read(b)
	token := hex.EncodeToString(b)
	s.sessMu.Lock()
	s.sessions[token] = time.Now().Add(30 * 24 * time.Hour)
	s.sessMu.Unlock()
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 30 * 24 * 3600})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) error {
	if c, err := r.Cookie(sessionCookie); err == nil {
		s.sessMu.Lock()
		delete(s.sessions, c.Value)
		s.sessMu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	return noContent(w)
}

// --- events -----------------------------------------------------------------

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	ch, unsub := s.svc.Bus.Subscribe()
	defer unsub()
	send := func(e service.Event) bool {
		b, err := json.Marshal(e.Data)
		if err != nil {
			return true
		}
		if _, err := fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, b); err != nil {
			return false
		}
		fl.Flush()
		return true
	}
	send(service.Event{Type: "state", Data: s.svc.Snapshot()})
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-ch:
			if !ok || !send(e) {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

// --- points & writes ----------------------------------------------------------

func (s *Server) points(w http.ResponseWriter, r *http.Request) error {
	kind := protocol.Kind(r.PathValue("kind"))
	if kind != protocol.MP && kind != protocol.SP {
		return badRequest("kind must be mp or sp")
	}
	if r.URL.Query().Get("refresh") == "1" {
		pts, err := s.svc.ReadAllPoints(r.Context(), kind)
		if err != nil {
			return err
		}
		return writeJSON(w, pts)
	}
	return writeJSON(w, s.svc.PointMeta(kind))
}

func (s *Server) writeSP(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Value *float64 `json:"value"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.Value == nil {
		return badRequest("value is required")
	}
	got, err := s.svc.WriteSP(r.Context(), int(id), *body.Value)
	if err != nil {
		return err
	}
	return writeJSON(w, map[string]float64{"value": got})
}

func (s *Server) override(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Value float64 `json:"value"`
		Auto  bool    `json:"auto"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if err := s.svc.Override(r.Context(), int(id), body.Value, body.Auto); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) saveTimeProgram(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Events []protocol.TimeProgramEvent `json:"events"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	n, err := s.svc.SaveTimeProgram(r.Context(), int(id), body.Events)
	if err != nil {
		return err
	}
	return writeJSON(w, map[string]int{"written": n})
}

func (s *Server) setTrendConfig(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		IDs    []int                `json:"ids"`
		Config protocol.TrendConfig `json:"config"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if err := s.svc.SetTrendConfig(r.Context(), body.IDs, body.Config); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) downloadTrend(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Delete bool `json:"delete"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	rec, err := s.svc.DownloadTrend(r.Context(), body.Delete)
	if err != nil {
		return err
	}
	return writeJSON(w, rec)
}

// --- recordings -----------------------------------------------------------------

func (s *Server) startRecording(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Name            string  `json:"name"`
		Points          []int   `json:"points"`
		IntervalSeconds float64 `json:"intervalSeconds"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	rec, err := s.svc.StartRecording(r.Context(), body.Name, body.Points, time.Duration(body.IntervalSeconds*float64(time.Second)))
	if err != nil {
		return err
	}
	return writeJSON(w, rec)
}

func (s *Server) recordingAction(w http.ResponseWriter, r *http.Request) error {
	var err error
	switch r.PathValue("action") {
	case "pause":
		err = s.svc.PauseRecording(true)
	case "resume":
		err = s.svc.PauseRecording(false)
	case "stop":
		err = s.svc.StopRecording()
	default:
		return badRequest("unknown action")
	}
	if err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) recording(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	rec, err := s.store.Recording(r.Context(), id)
	if err != nil {
		return err
	}
	return writeJSON(w, rec)
}

func parseTimeParam(v string) time.Time {
	if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
		return time.UnixMilli(ms)
	}
	return time.Time{}
}

func (s *Server) samples(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	q := r.URL.Query()
	series, err := s.store.Samples(r.Context(), id, parseTimeParam(q.Get("from")), parseTimeParam(q.Get("to")))
	if err != nil {
		return err
	}
	return writeJSON(w, series)
}

// csv exports a recording in wide format: one row per timestamp, one column per point.
func (s *Server) csv(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	rec, err := s.store.Recording(r.Context(), id)
	if err != nil {
		return err
	}
	series, err := s.store.Samples(r.Context(), id, time.Time{}, time.Time{})
	if err != nil {
		return err
	}
	col := map[int]int{}
	header := []string{"time"}
	for i, p := range rec.Points {
		col[p.ID] = i
		name := p.Name
		if p.Unit != "" {
			name += " [" + p.Unit + "]"
		}
		header = append(header, fmt.Sprintf("%q", name))
	}
	rows := map[int64][]string{}
	var times []int64
	for _, ser := range series {
		c, ok := col[ser.PointID]
		if !ok {
			continue
		}
		for i, t := range ser.T {
			row, ok := rows[t]
			if !ok {
				row = make([]string, len(rec.Points))
				rows[t] = row
				times = append(times, t)
			}
			row[c] = strconv.FormatFloat(ser.V[i], 'f', -1, 64)
		}
	}
	slices.Sort(times)
	name := strings.Map(func(r rune) rune {
		if strings.ContainsRune(`\/:*?"<>|`, r) {
			return '_'
		}
		return r
	}, rec.Name)
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, name))
	fmt.Fprintln(w, strings.Join(header, ";"))
	for _, t := range times {
		fmt.Fprintf(w, "%s;%s\n", time.UnixMilli(t).Format("2006-01-02 15:04:05"), strings.Join(rows[t], ";"))
	}
	return nil
}

func (s *Server) renameRecording(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	var body struct {
		Name string `json:"name"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if strings.TrimSpace(body.Name) == "" {
		return badRequest("name is required")
	}
	if err := s.store.RenameRecording(r.Context(), id, strings.TrimSpace(body.Name)); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) deleteRecording(w http.ResponseWriter, r *http.Request) error {
	id, err := pathInt(r, "id")
	if err != nil {
		return err
	}
	if id == s.svc.ActiveRecordingID() {
		return badRequest("stop the recording first")
	}
	if err := s.store.DeleteRecording(r.Context(), id); err != nil {
		return err
	}
	return noContent(w)
}

func (s *Server) importFile(w http.ResponseWriter, r *http.Request) error {
	r.Body = http.MaxBytesReader(w, r.Body, 200<<20)
	f, hdr, err := r.FormFile("file")
	if err != nil {
		return badRequest("file is required: %v", err)
	}
	defer f.Close()
	name := strings.TrimSuffix(filepath.Base(hdr.Filename), path.Ext(hdr.Filename))
	var rec store.Recording
	switch strings.ToLower(path.Ext(hdr.Filename)) {
	case ".htd":
		recs, err := legacy.ReadTrend(f)
		if err != nil {
			return badRequest("%v", err)
		}
		rec, err = s.svc.ImportTrend(r.Context(), name, recs)
		if err != nil {
			return err
		}
	case ".hwo":
		rr, samples, err := legacy.ReadRecording(f, name)
		if err != nil {
			return badRequest("%v", err)
		}
		rec, err = s.svc.ImportRecording(r.Context(), rr, samples)
		if err != nil {
			return err
		}
	default:
		return badRequest("unsupported file type (expected .htd or .hwo)")
	}
	return writeJSON(w, rec)
}

// --- settings -------------------------------------------------------------------

type settingsView struct {
	config.Config
	PasswordSet     bool `json:"passwordSet"`
	MQTTPasswordSet bool `json:"mqttPasswordSet"`
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) error {
	c := s.cfg.Get()
	v := settingsView{Config: c, PasswordSet: c.PasswordHash != "", MQTTPasswordSet: c.MQTT.Password != ""}
	v.MQTT.Password = ""
	return writeJSON(w, v)
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request) error {
	var in config.Config
	if err := decode(r, &in); err != nil {
		return err
	}
	old := s.cfg.Get()
	cur, err := s.cfg.Update(func(c *config.Config) {
		c.Device = in.Device
		c.PollSeconds = in.PollSeconds
		c.TrendPauseDuringDownload = in.TrendPauseDuringDownload
		c.ExpertMode = in.ExpertMode
		if in.ReadOnly != nil {
			c.ReadOnly = in.ReadOnly
		}
		pw := c.MQTT.Password
		c.MQTT = in.MQTT
		if in.MQTT.Password == "" {
			c.MQTT.Password = pw
		}
	})
	if err != nil {
		return badRequest("%v", err)
	}
	if old.IsReadOnly() != cur.IsReadOnly() {
		s.log.Warn("read-only mode changed", "readOnly", cur.IsReadOnly())
	}
	if s.OnSettings != nil {
		s.OnSettings(old, cur)
	}
	return s.getSettings(w, r)
}

func (s *Server) setPassword(w http.ResponseWriter, r *http.Request) error {
	var body struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if h := s.cfg.Get().PasswordHash; h != "" && bcrypt.CompareHashAndPassword([]byte(h), []byte(body.Current)) != nil {
		return httpError{http.StatusForbidden, "current password is wrong"}
	}
	var hash string
	if body.New != "" {
		if len(body.New) < 6 {
			return badRequest("the password needs at least 6 characters")
		}
		b, err := bcrypt.GenerateFromPassword([]byte(body.New), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		hash = string(b)
	}
	if _, err := s.cfg.Update(func(c *config.Config) { c.PasswordHash = hash }); err != nil {
		return err
	}
	if hash == "" {
		return noContent(w)
	}
	s.sessMu.Lock()
	clear(s.sessions)
	s.sessMu.Unlock()
	s.startSession(w) // keep the user who set the password logged in
	return noContent(w)
}

// --- static UI ----------------------------------------------------------------

func (s *Server) static() http.Handler {
	files := http.FileServerFS(s.web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		if _, err := fs.Stat(s.web, "index.html"); err != nil {
			http.Error(w, "The web UI is not part of this build. Build it with: cd web && npm ci && npx ng build, then rebuild the server.", http.StatusNotFound)
			return
		}
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(s.web, p); err != nil {
			// client-side route: serve the app shell
			r = r.Clone(r.Context())
			r.URL.Path = "/"
			w.Header().Set("Cache-Control", "no-cache")
		} else if p == "index.html" {
			w.Header().Set("Cache-Control", "no-cache")
		} else {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}
