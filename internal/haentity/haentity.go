// Package haentity turns the catalog pages into Home Assistant entity
// descriptions. It is the single source for both Home Assistant paths: the MQTT
// discovery bridge renders it, and the custom integration fetches it from
// GET /api/ha/entities.
package haentity

import (
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"heliocontrol/internal/catalog"
	"heliocontrol/internal/protocol"
)

// Entity is one Home Assistant entity.
type Entity struct {
	Key         string          `json:"key"`
	Component   string          `json:"component"` // sensor, binary_sensor, select, number, button
	Name        catalog.Text    `json:"name"`
	Kind        protocol.Kind   `json:"kind,omitempty"`
	ID          int             `json:"id"`                // point number (0 is a valid point)
	Display     catalog.Display `json:"display,omitempty"` // value, bool, enum, duration, fault, ratio, firmware
	Unit        string          `json:"unit,omitempty"`
	DeviceClass string          `json:"deviceClass,omitempty"`
	StateClass  string          `json:"stateClass,omitempty"`
	Category    string          `json:"category,omitempty"` // diagnostic, config
	Icon        string          `json:"icon,omitempty"`
	Options     []catalog.Text  `json:"options,omitempty"`
	Offset      int             `json:"offset,omitempty"`
	// Selectable lists the option values a select may be set to.
	Selectable []int   `json:"selectable,omitempty"`
	Min        float64 `json:"min,omitempty"`
	Max        float64 `json:"max,omitempty"`
	Step       float64 `json:"step,omitempty"`
	Num        int     `json:"num,omitempty"`
	Den        int     `json:"den,omitempty"`
	Precision  int     `json:"precision,omitempty"`
	// Action of a button: refresh, reset_fault, sync_clock.
	Action string `json:"action,omitempty"`
}

// Firmware is the display of the firmware version sensor.
const Firmware catalog.Display = "firmware"

// Build describes the entities of an installation. In read-only mode controls
// (select, number) become sensors and buttons that write are left out, so Home
// Assistant never offers something that would be refused.
func Build(panels []catalog.Panel, readOnly bool) []Entity {
	var out []Entity
	seen := map[string]bool{}
	for _, p := range panels {
		mixer := strings.HasPrefix(p.ID, "mixer")
		for _, it := range p.Items {
			if seen[it.Key] || it.HA.Component == "" {
				continue
			}
			seen[it.Key] = true
			out = append(out, entity(it, mixer, p.Title, readOnly))
		}
	}
	out = append(out, Entity{Key: "firmware", Component: "sensor", Display: Firmware, Name: catalog.Text{DE: "Firmware", EN: "Firmware"},
		Category: "diagnostic", Icon: "mdi:chip"})
	buttons := []Entity{{Key: "refresh", Action: "refresh", Name: catalog.Text{DE: "Aktualisieren", EN: "Refresh"}, Icon: "mdi:refresh", Category: "diagnostic"}}
	if !readOnly {
		buttons = append(buttons,
			Entity{Key: "reset_fault", Action: "reset_fault", Name: catalog.Text{DE: "Entstören", EN: "Reset fault"}, Icon: "mdi:alert-remove"},
			Entity{Key: "sync_clock", Action: "sync_clock", Name: catalog.Text{DE: "Zeit synchronisieren", EN: "Sync clock"}, Icon: "mdi:clock-check", Category: "config"})
	}
	for _, b := range buttons {
		b.Component = "button"
		out = append(out, b)
	}
	return out
}

func entity(it catalog.Item, mixer bool, title catalog.Text, readOnly bool) Entity {
	e := Entity{Key: it.Key, Component: it.HA.Component, Name: it.Label, Kind: it.Kind, ID: it.ID, Display: it.Display, Unit: it.Unit,
		DeviceClass: it.HA.DeviceClass, StateClass: it.HA.StateClass, Icon: it.HA.Icon, Num: it.Num, Den: it.Den}
	if mixer { // e.g. "Mischerkreis 1 Betriebsart": the circuits must be told apart
		e.Name = catalog.Text{DE: title.DE + " " + it.Label.DE, EN: title.EN + " " + it.Label.EN}
	}
	if readOnly && (e.Component == "select" || e.Component == "number") {
		e.Component = "sensor"
	}
	switch it.HA.Category {
	case "diagnostic":
		e.Category = "diagnostic"
	case "config":
		if e.Component != "sensor" { // HA does not allow config sensors
			e.Category = "config"
		}
	}
	if it.Display == catalog.Enum {
		e.Options, e.Offset = it.Options, it.Offset
		e.DeviceClass, e.StateClass = "", ""
		if e.Component == "sensor" {
			e.DeviceClass = "enum"
		}
		for i := range it.Options {
			if v := i + it.Offset; !slices.Contains(it.NotSelectable, v) {
				e.Selectable = append(e.Selectable, v)
			}
		}
	}
	// offsets in K are temperature differences, which HA does not treat as temperatures
	if e.DeviceClass == "temperature" && e.Unit != "°C" {
		e.DeviceClass = ""
	}
	if e.Component == "number" {
		e.Min, e.Max, e.Step = it.Min, it.Max, it.Step
		if e.Step == 0 {
			e.Step = 1
		}
		e.StateClass = ""
	}
	if it.Display == catalog.Ratio {
		e.Precision = 2
	}
	return e
}

// State is the current Home Assistant state of an entity: a number, an option
// label (in the given language), a bool, or the firmware string. ok is false
// while the value is unknown.
func State(e Entity, mp, sp map[int]float64, firmware, lang string) (state any, ok bool) {
	if e.Display == Firmware {
		return firmware, firmware != ""
	}
	var v float64
	switch {
	case e.Display == catalog.Ratio:
		n, ok1 := mp[e.Num]
		d, ok2 := mp[e.Den]
		if !ok1 || !ok2 || d == 0 {
			return nil, false
		}
		v = math.Round(n/d*100) / 100
	case e.Kind == protocol.MP:
		v, ok = mp[e.ID]
		if !ok {
			return nil, false
		}
	case e.Kind == protocol.SP:
		v, ok = sp[e.ID]
		if !ok {
			return nil, false
		}
	default:
		return nil, false
	}
	switch {
	case e.Display == catalog.Enum:
		i := int(v) - e.Offset
		if i < 0 || i >= len(e.Options) {
			return nil, false
		}
		return Label(e.Options[i], lang), true
	case e.Component == "binary_sensor":
		return v != 0, true
	}
	return v, true
}

// Label picks a language.
func Label(t catalog.Text, lang string) string {
	if strings.HasPrefix(lang, "de") && t.DE != "" {
		return t.DE
	}
	return t.EN
}

// OptionValue maps an option label (either language) back to its value.
func OptionValue(e Entity, label string) (int, error) {
	for i, o := range e.Options {
		if o.EN == label || o.DE == label {
			v := i + e.Offset
			if !slices.Contains(e.Selectable, v) {
				return 0, fmt.Errorf("%q cannot be selected", label)
			}
			return v, nil
		}
	}
	return 0, fmt.Errorf("unknown option %q", label)
}

// FormatState renders a state as an MQTT payload.
func FormatState(state any) string {
	switch v := state.(type) {
	case bool:
		if v {
			return "ON"
		}
		return "OFF"
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case string:
		return v
	}
	return fmt.Sprint(state)
}
