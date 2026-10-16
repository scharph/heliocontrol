// Package config loads and saves the server configuration (a YAML file that is
// also editable from the web UI).
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"

	"heliocontrol/internal/device"
)

type Device struct {
	device.TransportConfig `yaml:",inline"`
	// ExpectedID, if non-zero, must match the controller ID (~RID) or the
	// connection is refused - homeControl's "serial number" safety check.
	ExpectedID  int  `yaml:"expectedId" json:"expectedId"`
	AutoConnect bool `yaml:"autoConnect" json:"autoConnect"`
}

type MQTT struct {
	Enabled         bool   `yaml:"enabled" json:"enabled"`
	Broker          string `yaml:"broker" json:"broker"` // e.g. tcp://homeassistant.local:1883
	Username        string `yaml:"username" json:"username"`
	Password        string `yaml:"password" json:"password,omitempty"`
	ClientID        string `yaml:"clientId" json:"clientId"`
	DiscoveryPrefix string `yaml:"discoveryPrefix" json:"discoveryPrefix"`
	TopicPrefix     string `yaml:"topicPrefix" json:"topicPrefix"`
	// Language of entity names and option labels in Home Assistant ("de" or "en").
	Language string `yaml:"language" json:"language"`
}

type Config struct {
	Listen  string `yaml:"listen" json:"listen"`
	DataDir string `yaml:"dataDir" json:"dataDir"`
	Device  Device `yaml:"device" json:"device"`
	// PollSeconds is the refresh interval of the live values (homeControl's
	// auto refresh, minimum 10 s).
	PollSeconds int `yaml:"pollSeconds" json:"pollSeconds"`
	// TrendPauseDuringDownload stops trend recording while the buffer is read so
	// records arriving meanwhile are not lost (homeControl "all trend data").
	TrendPauseDuringDownload bool `yaml:"trendPauseDuringDownload" json:"trendPauseDuringDownload"`
	// ReadOnly (the default) refuses every command that would change the heat
	// pump. It is enforced in the API and again in the device client.
	ReadOnly *bool `yaml:"readOnly" json:"readOnly"`
	// WriteBudgetPerDay limits writes per setup point and day (protects the
	// controller's EEPROM from runaway automations). 0 = default (48), -1 = unlimited.
	WriteBudgetPerDay int `yaml:"writeBudgetPerDay" json:"writeBudgetPerDay"`
	// HistoryDays is how long the continuous history is kept (default 400).
	HistoryDays int `yaml:"historyDays" json:"historyDays"`
	// ExpertMode allows writing any writable setup point from the "all points" page.
	ExpertMode   bool   `yaml:"expertMode" json:"expertMode"`
	PasswordHash string `yaml:"passwordHash" json:"-"`
	MQTT         MQTT   `yaml:"mqtt" json:"mqtt"`
}

func (c *Config) applyDefaults() {
	if c.Listen == "" {
		c.Listen = ":8080"
	}
	if c.DataDir == "" {
		c.DataDir = "data"
	}
	c.Device.ApplyDefaults()
	if c.PollSeconds < 10 {
		c.PollSeconds = 15
	}
	if c.HistoryDays <= 0 {
		c.HistoryDays = 400
	}
	if c.ReadOnly == nil {
		on := true
		c.ReadOnly = &on
	}
	if c.MQTT.ClientID == "" {
		c.MQTT.ClientID = "heliocontrol"
	}
	if c.MQTT.DiscoveryPrefix == "" {
		c.MQTT.DiscoveryPrefix = "homeassistant"
	}
	if c.MQTT.TopicPrefix == "" {
		c.MQTT.TopicPrefix = "heliocontrol"
	}
	if c.MQTT.Language != "de" {
		c.MQTT.Language = "en"
	}
}

// WriteBudget returns the effective daily write limit per setup point (-1 = unlimited).
func (c Config) WriteBudget() int {
	switch {
	case c.WriteBudgetPerDay < 0:
		return -1
	case c.WriteBudgetPerDay == 0:
		return 48
	}
	return c.WriteBudgetPerDay
}

// IsReadOnly reports whether writes to the heat pump are disabled.
func (c Config) IsReadOnly() bool { return c.ReadOnly == nil || *c.ReadOnly }

func (c Config) Validate() error {
	switch c.Device.Type {
	case "tcp":
		if c.Device.Port <= 0 || c.Device.Port > 65535 {
			return fmt.Errorf("invalid port %d", c.Device.Port)
		}
	case "serial":
	default:
		return fmt.Errorf("unknown device type %q", c.Device.Type)
	}
	if c.MQTT.Enabled && c.MQTT.Broker == "" {
		return errors.New("MQTT is enabled but no broker is set")
	}
	return nil
}

// Store holds the current configuration and persists changes.
type Store struct {
	mu   sync.RWMutex
	path string
	cfg  Config
}

// Load reads path; a missing file yields the defaults (and is created on Save).
func Load(path string) (*Store, error) {
	s := &Store{path: path}
	b, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := yaml.Unmarshal(b, &s.cfg); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	s.cfg.applyDefaults()
	return s, s.cfg.Validate()
}

func (s *Store) Get() Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cfg
}

// Update applies fn to a copy, validates and saves it.
func (s *Store) Update(fn func(*Config)) (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg
	fn(&c)
	c.applyDefaults()
	if err := c.Validate(); err != nil {
		return s.cfg, err
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return s.cfg, err
	}
	if dir := filepath.Dir(s.path); dir != "" {
		_ = os.MkdirAll(dir, 0o755)
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return s.cfg, err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return s.cfg, err
	}
	s.cfg = c
	return c, nil
}
