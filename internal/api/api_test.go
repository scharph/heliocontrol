package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"heliocontrol/internal/config"
	"heliocontrol/internal/service"
	"heliocontrol/internal/sim"
	"heliocontrol/internal/store"
)

func setup(t *testing.T, mods ...func(*config.Config)) (*httptest.Server, *config.Store) {
	t.Helper()
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { l.Close() })
	dev := sim.New(sim.DefaultOptions(), nil)
	dev.Latency = 0
	go dev.Serve(l)
	host, port, _ := net.SplitHostPort(l.Addr().String())
	p, _ := strconv.Atoi(port)

	dir := t.TempDir()
	cfg, _ := config.Load(filepath.Join(dir, "c.yaml"))
	cfg.Update(func(c *config.Config) {
		c.Device.Host, c.Device.Port, c.Device.AutoConnect = host, p, true
		c.MQTT.Password = "secret"
		for _, m := range mods {
			m(c)
		}
	})
	st, _ := store.Open(filepath.Join(dir, "db"))
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := service.New(cfg, st, log)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go svc.Run(ctx)
	deadline := time.Now().Add(15 * time.Second)
	for svc.Snapshot().Updated.IsZero() {
		if time.Now().After(deadline) {
			t.Fatal("service did not come online")
		}
		time.Sleep(50 * time.Millisecond)
	}
	web := fstest.MapFS{"index.html": {Data: []byte("<app-root></app-root>")}, "main.js": {Data: []byte("x")}}
	api := New(svc, cfg, st, web, "test", log)
	api.HtREST = true
	srv := httptest.NewServer(api.Handler())
	t.Cleanup(srv.Close)
	return srv, cfg
}

func do(t *testing.T, c *http.Client, method, url, body string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(res.Body)
	res.Body.Close()
	return res, string(b)
}

func TestReadOnlyAndSettings(t *testing.T) {
	srv, _ := setup(t)
	c := srv.Client()
	// default config is read-only: writes are refused with 403
	if res, body := do(t, c, "PUT", srv.URL+"/api/sp/69", `{"value":21}`); res.StatusCode != http.StatusForbidden {
		t.Fatalf("write in read-only mode: %d %s", res.StatusCode, body)
	}
	// secrets are not returned
	_, body := do(t, c, "GET", srv.URL+"/api/settings", "")
	if strings.Contains(body, "secret") || !strings.Contains(body, `"mqttPasswordSet":true`) {
		t.Fatalf("settings leak or flag missing: %s", body)
	}
	var st struct{ Panels []struct{ ID string } }
	_, body = do(t, c, "GET", srv.URL+"/api/state", "")
	json.Unmarshal([]byte(body), &st)
	if len(st.Panels) == 0 {
		t.Fatal("no panels in state")
	}
	// SPA routes fall back to the app shell, unknown API routes do not
	if res, body := do(t, c, "GET", srv.URL+"/overview/plant", ""); res.StatusCode != 200 || !strings.Contains(body, "app-root") {
		t.Fatalf("spa fallback: %d %s", res.StatusCode, body)
	}
	if res, _ := do(t, c, "GET", srv.URL+"/api/nope", ""); res.StatusCode != 404 {
		t.Fatalf("unknown api route: %d", res.StatusCode)
	}
}

func TestPasswordProtection(t *testing.T) {
	srv, _ := setup(t)
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar}
	if res, body := do(t, c, "POST", srv.URL+"/api/settings/password", `{"new":"geheim123"}`); res.StatusCode != 204 {
		t.Fatalf("set password: %d %s", res.StatusCode, body)
	}
	// the client that set it stays logged in
	if res, _ := do(t, c, "GET", srv.URL+"/api/state", ""); res.StatusCode != 200 {
		t.Fatalf("own session: %d", res.StatusCode)
	}
	anon := &http.Client{}
	if res, _ := do(t, anon, "GET", srv.URL+"/api/state", ""); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous access: %d", res.StatusCode)
	}
	if res, _ := do(t, anon, "GET", srv.URL+"/", ""); res.StatusCode != 200 {
		t.Fatalf("app shell must stay reachable for the login page: %d", res.StatusCode)
	}
	if res, _ := do(t, anon, "POST", srv.URL+"/api/login", `{"password":"wrong"}`); res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong password: %d", res.StatusCode)
	}
	jar2, _ := cookiejar.New(nil)
	c2 := &http.Client{Jar: jar2}
	if res, _ := do(t, c2, "POST", srv.URL+"/api/login", `{"password":"geheim123"}`); res.StatusCode != 204 {
		t.Fatalf("login: %d", res.StatusCode)
	}
	if res, _ := do(t, c2, "GET", srv.URL+"/api/state", ""); res.StatusCode != 200 {
		t.Fatalf("after login: %d", res.StatusCode)
	}
}

func TestHtREST(t *testing.T) {
	srv, _ := setup(t, func(c *config.Config) { off := false; c.ReadOnly = &off })
	c := srv.Client()
	_, body := do(t, c, "GET", srv.URL+"/api/v1/param/", "")
	var all map[string]float64
	if err := json.Unmarshal([]byte(body), &all); err != nil || all["Betriebsart"] != 1 || all["HKR Soll_Raum"] != 20 || all["Stoerung"] != 0 {
		t.Fatalf("param list: %s", body)
	}
	if _, body := do(t, c, "GET", srv.URL+"/api/v1/param/?HKR%20Soll_Raum&Betriebsart", ""); body != `{"Betriebsart":1,"HKR Soll_Raum":20}`+"\n" {
		t.Fatalf("subset: %s", body)
	}
	if _, body := do(t, c, "GET", srv.URL+"/api/v1/fastquery/", ""); strings.Contains(body, "Betriebsart") || !strings.Contains(body, `"Temp. Aussen"`) {
		t.Fatalf("fastquery must return measuring points only: %s", body)
	}
	if res, body := do(t, c, "GET", srv.URL+"/api/v1/param/Nope", ""); res.StatusCode != 404 {
		t.Fatalf("unknown name: %d %s", res.StatusCode, body)
	}
	if res, body := do(t, c, "PUT", srv.URL+"/api/v1/param/HKR%20Soll_Raum", `{"value":21.5}`); res.StatusCode != 200 || body != `{"value":21.5}`+"\n" {
		t.Fatalf("write: %d %s", res.StatusCode, body)
	}
	if _, body := do(t, c, "GET", srv.URL+"/api/v1/param/HKR%20Soll_Raum", ""); body != `{"value":21.5}`+"\n" {
		t.Fatalf("read back: %s", body)
	}
	if res, _ := do(t, c, "PUT", srv.URL+"/api/v1/param/Temp.%20Aussen", `{"value":1}`); res.StatusCode != 400 {
		t.Fatalf("measuring point write: %d", res.StatusCode)
	}
	// multi write stops at the first failure (99 is out of range), keeping earlier writes
	res, body := do(t, c, "PUT", srv.URL+"/api/v1/param/", `{"Betriebsart":3,"WW Normaltemp.":99}`)
	if res.StatusCode != 400 || !strings.Contains(body, `"written":{"Betriebsart":3}`) {
		t.Fatalf("multi write: %d %s", res.StatusCode, body)
	}

	// Basic auth once a password is set
	do(t, c, "POST", srv.URL+"/api/settings/password", `{"new":"geheim123"}`)
	u := strings.Replace(srv.URL, "http://", "http://loxone:geheim123@", 1)
	if res, body := do(t, c, "GET", u+"/api/v1/param/Betriebsart", ""); res.StatusCode != 200 {
		t.Fatalf("basic auth: %d %s", res.StatusCode, body)
	}
	res, _ = do(t, c, "GET", srv.URL+"/api/v1/param/Betriebsart", "")
	if res.StatusCode != 401 || res.Header.Get("WWW-Authenticate") == "" {
		t.Fatalf("no credentials: %d %v", res.StatusCode, res.Header)
	}
	if res, _ := do(t, c, "GET", strings.Replace(u, "geheim123", "wrong", 1)+"/api/state", ""); res.StatusCode != 401 {
		t.Fatalf("wrong basic password: %d", res.StatusCode)
	}
}

func TestHtRESTOffByDefault(t *testing.T) {
	cfg, _ := config.Load(filepath.Join(t.TempDir(), "c.yaml"))
	h := New(nil, cfg, nil, fstest.MapFS{}, "test", slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/v1/param/", nil))
	if rec.Code != 404 {
		t.Fatalf("HtREST endpoints must be off unless enabled: %d", rec.Code)
	}
}
