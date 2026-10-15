// Package mqtt bridges the heat pump to Home Assistant using MQTT discovery:
// every value on the UI pages becomes an entity (sensor, binary sensor, select,
// number, button) attached to one "Heliotherm" device.
package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"

	"heliocontrol/internal/catalog"
	"heliocontrol/internal/config"
	"heliocontrol/internal/protocol"
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
	entities  map[string]entity // by key
	published map[string]bool   // discovery topics currently announced
	sig       string            // signature of the announced entity set
}

// entity is one Home Assistant entity.
type entity struct {
	catalog.Item
	component string
}

func New(svc *service.Service, log *slog.Logger) *Bridge {
	return &Bridge{svc: svc, log: log.With("component", "mqtt"), entities: map[string]entity{}, published: map[string]bool{}}
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
	readOnly := b.readOnly
	b.mu.Unlock()
	ents := map[string]entity{}
	var keys []string
	for _, p := range snap.Panels {
		for _, it := range p.Items {
			if _, dup := ents[it.Key]; dup {
				continue
			}
			comp := it.HA.Component
			if readOnly && (comp == "select" || comp == "number") {
				comp = "sensor"
			}
			if comp == "" {
				continue
			}
			ents[it.Key] = entity{Item: it, component: comp}
			keys = append(keys, it.Key)
		}
	}
	slices.Sort(keys)
	sig := fmt.Sprintf("%d|%s|%v|%s", snap.DeviceID, snap.Firmware, readOnly, strings.Join(keys, ","))
	b.mu.Lock()
	if sig == b.sig {
		b.mu.Unlock()
		return
	}
	b.sig, b.entities = sig, ents
	old := b.published
	b.published = map[string]bool{}
	cfg := b.cfg
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
	disc := func(component, key string, payload map[string]any) {
		payload["unique_id"] = node + "_" + key
		payload["object_id"] = node + "_" + key
		payload["device"] = dev
		payload["availability_topic"] = b.availabilityTopic()
		topic := fmt.Sprintf("%s/%s/%s/%s/config", cfg.DiscoveryPrefix, component, node, key)
		b.publish(topic, true, payload)
		b.mu.Lock()
		b.published[topic] = true
		b.mu.Unlock()
	}
	for _, key := range keys {
		e := ents[key]
		p := map[string]any{"name": e.Label.EN, "state_topic": base + "/" + key + "/state"}
		if e.HA.Icon != "" {
			p["icon"] = e.HA.Icon
		}
		if e.HA.Category == "diagnostic" || (e.HA.Category == "config" && e.component != "sensor") {
			p["entity_category"] = e.HA.Category
		}
		switch e.component {
		case "binary_sensor":
			p["payload_on"], p["payload_off"] = "ON", "OFF"
			if e.HA.DeviceClass != "" {
				p["device_class"] = e.HA.DeviceClass
			}
		case "select":
			p["command_topic"] = base + "/" + key + "/set"
			p["options"] = optionLabels(e.Item, true)
		case "number":
			p["command_topic"] = base + "/" + key + "/set"
			p["min"], p["max"], p["step"], p["mode"] = e.Min, e.Max, e.Step, "box"
			if e.Unit != "" {
				p["unit_of_measurement"] = e.Unit
			}
			if e.HA.DeviceClass != "" {
				p["device_class"] = e.HA.DeviceClass
			}
		default: // sensor
			switch e.Display {
			case catalog.Enum:
				p["device_class"] = "enum"
				p["options"] = optionLabels(e.Item, false)
			default:
				if e.Unit != "" {
					p["unit_of_measurement"] = e.Unit
				}
				// offsets in K are not temperatures for Home Assistant
				if dc := e.HA.DeviceClass; dc != "" && dc != "enum" && !(dc == "temperature" && e.Unit != "°C") {
					p["device_class"] = dc
				}
				if e.HA.StateClass != "" {
					p["state_class"] = e.HA.StateClass
				}
				if e.Display == catalog.Ratio {
					p["suggested_display_precision"] = 2
				}
			}
		}
		disc(e.component, key, p)
	}
	// buttons
	disc("button", "refresh", map[string]any{"name": "Refresh", "command_topic": base + "/refresh/set", "icon": "mdi:refresh", "entity_category": "diagnostic"})
	if !readOnly {
		disc("button", "reset_fault", map[string]any{"name": "Reset fault", "command_topic": base + "/reset_fault/set", "icon": "mdi:alert-remove"})
		disc("button", "sync_clock", map[string]any{"name": "Sync clock", "command_topic": base + "/sync_clock/set", "icon": "mdi:clock-check", "entity_category": "config"})
	}
	disc("sensor", "firmware", map[string]any{"name": "Firmware", "state_topic": base + "/firmware/state", "entity_category": "diagnostic", "icon": "mdi:chip"})

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
	b.log.Info("announced Home Assistant entities", "count", len(keys)+2, "readOnly", readOnly)
}

func optionLabels(it catalog.Item, selectable bool) []string {
	var out []string
	for i, o := range it.Options {
		if selectable && slices.Contains(it.NotSelectable, i+it.Offset) {
			continue
		}
		out = append(out, o.EN)
	}
	return out
}

// value returns the current value of an item and whether it is known.
func value(snap service.Snapshot, it catalog.Item) (float64, bool) {
	if it.Display == catalog.Ratio {
		n, ok1 := snap.MP[it.Num]
		d, ok2 := snap.MP[it.Den]
		if !ok1 || !ok2 || d == 0 {
			return 0, false
		}
		return math.Round(n/d*100) / 100, true
	}
	src := snap.MP
	if it.Kind == protocol.SP {
		src = snap.SP
	}
	v, ok := src[it.ID]
	return v, ok
}

func (b *Bridge) publishStates(snap service.Snapshot) {
	b.mu.Lock()
	ents := b.entities
	b.mu.Unlock()
	base := b.baseTopic(snap)
	for key, e := range ents {
		v, ok := value(snap, e.Item)
		if !ok {
			continue
		}
		var s string
		switch {
		case e.Display == catalog.Enum:
			i := int(v) - e.Offset
			if i < 0 || i >= len(e.Options) {
				continue
			}
			s = e.Options[i].EN
		case e.component == "binary_sensor":
			s = map[bool]string{true: "ON", false: "OFF"}[v != 0]
		default:
			s = strconv.FormatFloat(v, 'f', -1, 64)
		}
		b.publish(base+"/"+key+"/state", true, s)
	}
	b.publish(base+"/firmware/state", true, snap.Firmware)
}

func (b *Bridge) onCommand(_ paho.Client, m paho.Message) {
	parts := strings.Split(m.Topic(), "/")
	if len(parts) < 3 {
		return
	}
	key, payload := parts[len(parts)-2], strings.TrimSpace(string(m.Payload()))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var err error
	switch key {
	case "refresh":
		err = b.svc.Refresh(ctx)
	case "reset_fault":
		err = b.svc.ResetFault(ctx)
	case "sync_clock":
		err = b.svc.SyncClock(ctx)
	default:
		b.mu.Lock()
		e, ok := b.entities[key]
		b.mu.Unlock()
		if !ok || (e.component != "select" && e.component != "number") {
			b.log.Warn("command for unknown or read-only entity", "key", key)
			return
		}
		var v float64
		if e.component == "select" {
			i := slices.IndexFunc(e.Options, func(o catalog.Text) bool { return o.EN == payload || o.DE == payload })
			if i < 0 {
				b.log.Warn("unknown option", "key", key, "payload", payload)
				return
			}
			v = float64(i + e.Offset)
		} else if v, err = strconv.ParseFloat(payload, 64); err != nil {
			b.log.Warn("bad number", "key", key, "payload", payload)
			return
		}
		_, err = b.svc.WriteSP(ctx, e.ID, v)
	}
	if err != nil {
		b.log.Warn("command failed", "key", key, "payload", payload, "err", err)
	}
	b.sync(b.svc.Snapshot()) // publish the real state (also reverts HA's optimistic UI)
}
