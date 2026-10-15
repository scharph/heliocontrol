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

func setup(t *testing.T) (*httptest.Server, *config.Store) {
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
	srv := httptest.NewServer(New(svc, cfg, st, web, "test", log).Handler())
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
