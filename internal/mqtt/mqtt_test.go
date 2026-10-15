package mqtt

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"

	"heliocontrol/internal/config"
	"heliocontrol/internal/protocol"
	"heliocontrol/internal/service"
	"heliocontrol/internal/sim"
	"heliocontrol/internal/store"
)

func freeAddr(t *testing.T) string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	return l.Addr().String()
}

type recorder struct {
	mu   sync.Mutex
	msgs map[string]string
}

func (r *recorder) get(topic string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.msgs[topic]
	return v, ok
}

func setup(t *testing.T, readOnly bool) (*sim.Device, *recorder, paho.Client) {
	t.Helper()
	// broker
	brokerAddr := freeAddr(t)
	broker := mochi.New(nil)
	broker.AddHook(new(auth.AllowHook), nil)
	if err := broker.AddListener(listeners.NewTCP(listeners.Config{Type: "tcp", ID: "t", Address: brokerAddr})); err != nil {
		t.Fatal(err)
	}
	go broker.Serve()
	t.Cleanup(func() { broker.Close() })

	// simulated heat pump
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	t.Cleanup(func() { l.Close() })
	dev := sim.New(sim.DefaultOptions(), nil)
	dev.Latency = 0
	go dev.Serve(l)
	host, port, _ := net.SplitHostPort(l.Addr().String())
	p, _ := strconv.Atoi(port)

	dir := t.TempDir()
	cfg, _ := config.Load(filepath.Join(dir, "c.yaml"))
	c, err := cfg.Update(func(c *config.Config) {
		c.Device.Host, c.Device.Port, c.Device.AutoConnect = host, p, true
		c.ReadOnly = &readOnly
		c.MQTT = config.MQTT{Enabled: true, Broker: "tcp://" + brokerAddr, ClientID: "hc-test"}
	})
	if err != nil {
		t.Fatal(err)
	}
	st, _ := store.Open(filepath.Join(dir, "db"))
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := service.New(cfg, st, log)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go svc.Run(ctx)
	b := New(svc, log)
	b.Configure(c)
	t.Cleanup(b.Close)

	// observer
	rec := &recorder{msgs: map[string]string{}}
	obs := paho.NewClient(paho.NewClientOptions().AddBroker("tcp://" + brokerAddr).SetClientID("observer"))
	if tok := obs.Connect(); tok.Wait() && tok.Error() != nil {
		t.Fatal(tok.Error())
	}
	t.Cleanup(func() { obs.Disconnect(100) })
	obs.Subscribe("#", 1, func(_ paho.Client, m paho.Message) {
		rec.mu.Lock()
		rec.msgs[m.Topic()] = string(m.Payload())
		rec.mu.Unlock()
	}).Wait()
	return dev, rec, obs
}

func waitTopic(t *testing.T, r *recorder, topic string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		if v, ok := r.get(topic); ok && v != "" {
			return v
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("no message on %s", topic)
	return ""
}

func TestDiscoveryAndCommands(t *testing.T) {
	dev, rec, obs := setup(t, false)
	var outdoor map[string]any
	if err := json.Unmarshal([]byte(waitTopic(t, rec, "homeassistant/sensor/heliotherm_123456/outdoor/config")), &outdoor); err != nil {
		t.Fatal(err)
	}
	if outdoor["device_class"] != "temperature" || outdoor["unit_of_measurement"] != "°C" || outdoor["state_topic"] != "heliocontrol/123456/outdoor/state" {
		t.Fatalf("outdoor discovery: %v", outdoor)
	}
	var mode map[string]any
	json.Unmarshal([]byte(waitTopic(t, rec, "homeassistant/select/heliotherm_123456/mode/config")), &mode)
	opts, _ := mode["options"].([]any)
	if len(opts) == 0 || strings.Contains(waitTopic(t, rec, "homeassistant/select/heliotherm_123456/mode/config"), "Main switch") {
		t.Fatalf("mode options: %v", mode["options"])
	}
	waitTopic(t, rec, "heliocontrol/123456/outdoor/state")
	if s := waitTopic(t, rec, "heliocontrol/status"); s != "online" {
		t.Fatalf("availability %q", s)
	}
	if s := waitTopic(t, rec, "heliocontrol/123456/mode/state"); s != "Automatic" {
		t.Fatalf("mode state %q", s)
	}

	obs.Publish("heliocontrol/123456/room_target/set", 1, false, "21.5").Wait()
	deadline := time.Now().Add(10 * time.Second)
	for !strings.Contains(dev.Handle(protocol.ReadSP(69))[0], "VAL=21.5,") {
		if time.Now().After(deadline) {
			t.Fatal("room target not written via MQTT")
		}
		time.Sleep(50 * time.Millisecond)
	}
	obs.Publish("heliocontrol/123456/mode/set", 1, false, "Summer").Wait()
	deadline = time.Now().Add(10 * time.Second)
	for !strings.Contains(dev.Handle(protocol.ReadSP(13))[0], "VAL=3,") {
		if time.Now().After(deadline) {
			t.Fatal("mode not written via MQTT")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestReadOnlyPublishesNoControls(t *testing.T) {
	dev, rec, obs := setup(t, true)
	waitTopic(t, rec, "homeassistant/sensor/heliotherm_123456/mode/config") // select becomes a sensor
	waitTopic(t, rec, "homeassistant/sensor/heliotherm_123456/room_target/config")
	time.Sleep(300 * time.Millisecond)
	for _, topic := range []string{"homeassistant/select/heliotherm_123456/mode/config", "homeassistant/button/heliotherm_123456/reset_fault/config"} {
		if _, ok := rec.get(topic); ok {
			t.Fatalf("%s published in read-only mode", topic)
		}
	}
	before := dev.Handle(protocol.ReadSP(69))[0]
	obs.Publish("heliocontrol/123456/room_target/set", 1, false, "24").Wait()
	time.Sleep(500 * time.Millisecond)
	if after := dev.Handle(protocol.ReadSP(69))[0]; after != before {
		t.Fatal("write went through in read-only mode")
	}
}
