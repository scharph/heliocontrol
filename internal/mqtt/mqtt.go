// Package mqtt bridges the heat pump to Home Assistant using MQTT discovery:
// every value on the UI pages becomes an entity (sensor, binary sensor, select,
// number, button) attached to one "Heliotherm" device.
package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"heliocontrol/internal/config"
	"heliocontrol/internal/haentity"
	"heliocontrol/internal/service"
)

type Bridge struct {
	svc *service.Service
	log *slog.Logger

	mu        sync.Mutex
	cfg       config.MQTT
	readOnly  bool
	client    paho.Client
	stop      context.CancelFunc
	entities  map[string]haentity.Entity // by key
	published map[string]bool            // discovery topics currently announced
	sig       string                     // signature of the announced entity set
}

func New(svc *service.Service, log *slog.Logger) *Bridge {
	return &Bridge{svc: svc, log: log.With("component", "mqtt"), entities: map[string]haentity.Entity{}, published: map[string]bool{}}
}

// Configure (re)starts the bridge with new settings; a disabled config stops it.
func (b *Bridge) Configure(c config.Config) {
	b.mu.Lock()
	same := b.cfg == c.MQTT && b.readOnly == c.IsReadOnly() && (b.client != nil) == c.MQTT.Enabled
	b.mu.Unlock()
	if same {
		return
	}
	b.Close()
	b.mu.Lock()
	b.cfg, b.readOnly = c.MQTT, c.IsReadOnly()
	b.mu.Unlock()
	if !c.MQTT.Enabled {
		return
	}
	b.start()
}

func (b *Bridge) Close() {
	b.mu.Lock()
	cl, stop := b.client, b.stop
	b.client, b.stop = nil, nil
	b.sig = ""
	clear(b.published)
	b.mu.Unlock()
	if stop != nil {
		stop()
	}
	if cl != nil && cl.IsConnected() {
		cl.Publish(b.availabilityTopic(), 1, true, "offline").WaitTimeout(time.Second)
		cl.Disconnect(500)
	}
}

func (b *Bridge) availabilityTopic() string { return b.cfg.TopicPrefix + "/status" }

func (b *Bridge) start() {
	cfg := b.cfg
	opts := paho.NewClientOptions().
		AddBroker(cfg.Broker).
		SetClientID(cfg.ClientID).
		SetUsername(cfg.Username).
		SetPassword(cfg.Password).
		SetWill(b.availabilityTopic(), "offline", 1, true).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(10 * time.Second).
		SetOnConnectHandler(func(c paho.Client) {
			b.log.Info("connected to broker", "broker", cfg.Broker)
			b.mu.Lock()
			b.sig = "" // force re-announcement
			b.mu.Unlock()
			c.Subscribe(cfg.TopicPrefix+"/+/+/set", 1, b.onCommand)
			// Home Assistant announces restarts; re-send discovery then.
			c.Subscribe(cfg.DiscoveryPrefix+"/status", 1, func(_ paho.Client, m paho.Message) {
				if string(m.Payload()) == "online" {
					b.mu.Lock()
					b.sig = ""
					b.mu.Unlock()
					b.sync(b.svc.Snapshot())
				}
			})
			b.sync(b.svc.Snapshot())
		}).
		SetConnectionLostHandler(func(_ paho.Client, err error) { b.log.Warn("broker connection lost", "err", err) })
	client := paho.NewClient(opts)
	ctx, cancel := context.WithCancel(context.Background())
	b.mu.Lock()
	b.client, b.stop = client, cancel
	b.mu.Unlock()
	client.Connect() // retries in the background
	go b.loop(ctx)
}

func (b *Bridge) loop(ctx context.Context) {
	events, unsub := b.svc.Bus.Subscribe()
	defer unsub()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-events:
			if !ok {
				return
			}
			if e.Type == "state" || e.Type == "values" {
				b.sync(b.svc.Snapshot())
			}
		}
	}
}

func (b *Bridge) publish(topic string, retained bool, payload any) {
	b.mu.Lock()
	c := b.client
	b.mu.Unlock()
	if c == nil || !c.IsConnected() {
		return
	}
	var data []byte
	switch p := payload.(type) {
	case string:
		data = []byte(p)
	case []byte:
		data = p
	default:
		data, _ = json.Marshal(p)
	}
	c.Publish(topic, 1, retained, data)
}

func (b *Bridge) nodeID(snap service.Snapshot) string {
	return fmt.Sprintf("heliotherm_%d", snap.DeviceID)
}

func (b *Bridge) baseTopic(snap service.Snapshot) string {
	return fmt.Sprintf("%s/%d", b.cfg.TopicPrefix, snap.DeviceID)
}

// sync announces entities when the set changed and publishes current states.
func (b *Bridge) sync(snap service.Snapshot) {
	b.mu.Lock()
	connected := b.client != nil && b.client.IsConnected()
	b.mu.Unlock()
	if !connected {
		return
	}
	online := snap.Status == service.Online
	b.publish(b.availabilityTopic(), true, map[bool]string{true: "online", false: "offline"}[online])
	if snap.DeviceID == 0 {
		return
	}
	b.announce(snap)
	if online && !snap.Updated.IsZero() {
		b.publishStates(snap)
	}
}

func (b *Bridge) announce(snap service.Snapshot) {
	b.mu.Lock()
	readOnly, cfg := b.readOnly, b.cfg
	b.mu.Unlock()
	list := haentity.Build(snap.Panels, readOnly)
	keys := make([]string, len(list))
	ents := make(map[string]haentity.Entity, len(list))
	for i, e := range list {
		keys[i] = e.Key
		ents[e.Key] = e
	}
	sig := fmt.Sprintf("%d|%s|%v|%s|%s", snap.DeviceID, snap.Firmware, readOnly, cfg.Language, strings.Join(keys, ","))
	b.mu.Lock()
	if sig == b.sig {
		b.mu.Unlock()
		return
	}
	b.sig, b.entities = sig, ents
	old := b.published
	b.published = map[string]bool{}
	b.mu.Unlock()

	node, base := b.nodeID(snap), b.baseTopic(snap)
	dev := map[string]any{
		"identifiers":   []string{node},
		"name":          "Heliotherm",
		"manufacturer":  "Heliotherm",
		"model":         "web control",
		"sw_version":    snap.Firmware,
		"serial_number": strconv.Itoa(snap.DeviceID),
	}
	for _, e := range list {
		p := discovery(e, base, cfg.Language)
		p["unique_id"] = node + "_" + e.Key
		p["object_id"] = node + "_" + e.Key
		p["device"] = dev
		p["availability_topic"] = b.availabilityTopic()
		topic := fmt.Sprintf("%s/%s/%s/%s/config", cfg.DiscoveryPrefix, e.Component, node, e.Key)
		b.publish(topic, true, p)
		b.mu.Lock()
		b.published[topic] = true
		b.mu.Unlock()
	}
	// remove entities that disappeared (e.g. a page is no longer present)
	b.mu.Lock()
	var stale []string
	for t := range old {
		if !b.published[t] {
			stale = append(stale, t)
		}
	}
	b.mu.Unlock()
	for _, t := range stale {
		b.publish(t, true, "")
	}
	b.log.Info("announced Home Assistant entities", "count", len(list), "readOnly", readOnly)
}

// discovery renders the MQTT discovery payload of an entity.
func discovery(e haentity.Entity, base, lang string) map[string]any {
	p := map[string]any{"name": haentity.Label(e.Name, lang)}
	if e.Component != "button" {
		p["state_topic"] = base + "/" + e.Key + "/state"
	}
	if e.Icon != "" {
		p["icon"] = e.Icon
	}
	if e.Category != "" {
		p["entity_category"] = e.Category
	}
	if e.DeviceClass != "" {
		p["device_class"] = e.DeviceClass
	}
	if e.Unit != "" && e.Display != "enum" {
		p["unit_of_measurement"] = e.Unit
	}
	if e.StateClass != "" {
		p["state_class"] = e.StateClass
	}
	if e.Precision > 0 {
		p["suggested_display_precision"] = e.Precision
	}
	switch e.Component {
	case "binary_sensor":
		p["payload_on"], p["payload_off"] = "ON", "OFF"
	case "select":
		p["command_topic"] = base + "/" + e.Key + "/set"
		var opts []string
		for _, v := range e.Selectable {
			opts = append(opts, haentity.Label(e.Options[v-e.Offset], lang))
		}
		p["options"] = opts
	case "number":
		p["command_topic"] = base + "/" + e.Key + "/set"
		p["min"], p["max"], p["step"], p["mode"] = e.Min, e.Max, e.Step, "box"
	case "button":
		p["command_topic"] = base + "/" + e.Key + "/set"
	case "sensor":
		if e.Display == "enum" {
			var opts []string
			for _, o := range e.Options {
				opts = append(opts, haentity.Label(o, lang))
			}
			p["options"] = opts
		}
	}
	return p
}

func (b *Bridge) publishStates(snap service.Snapshot) {
	b.mu.Lock()
	ents, lang := b.entities, b.cfg.Language
	b.mu.Unlock()
	base := b.baseTopic(snap)
	for key, e := range ents {
		if e.Component == "button" {
			continue
		}
		if st, ok := haentity.State(e, snap.MP, snap.SP, snap.Firmware, lang); ok {
			b.publish(base+"/"+key+"/state", true, haentity.FormatState(st))
		}
	}
}

func (b *Bridge) onCommand(_ paho.Client, m paho.Message) {
	parts := strings.Split(m.Topic(), "/")
	if len(parts) < 3 {
		return
	}
	key, payload := parts[len(parts)-2], strings.TrimSpace(string(m.Payload()))
	b.mu.Lock()
	e, ok := b.entities[key]
	b.mu.Unlock()
	if !ok {
		b.log.Warn("command for unknown entity", "key", key)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var err error
	switch e.Component {
	case "button":
		switch e.Action {
		case "refresh":
			err = b.svc.Refresh(ctx)
		case "reset_fault":
			err = b.svc.ResetFault(ctx)
		case "sync_clock":
			err = b.svc.SyncClock(ctx)
		}
	case "select":
		var v int
		if v, err = haentity.OptionValue(e, payload); err == nil {
			_, err = b.svc.WriteSP(ctx, e.ID, float64(v))
		}
	case "number":
		var v float64
		if v, err = strconv.ParseFloat(payload, 64); err == nil {
			_, err = b.svc.WriteSP(ctx, e.ID, v)
		}
	default:
		b.log.Warn("command for read-only entity", "key", key)
		return
	}
	if err != nil {
		b.log.Warn("command failed", "key", key, "payload", payload, "err", err)
	}
	b.sync(b.svc.Snapshot()) // publish the real state (also reverts HA's optimistic UI)
}
