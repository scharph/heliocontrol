// Package catalog describes what the Heliotherm controller's points mean: which
// point appears on which page, its unit, how it is shown and whether it can be
// changed. It is the layout of homeControl 2.1 (MainWnd tab pages, whose
// controls carry the point number in their Tag) turned into data.
package catalog

import (
	"fmt"
	"slices"

	"heliocontrol/internal/protocol"
)

// Text is a translated label.
type Text struct {
	DE string `json:"de"`
	EN string `json:"en"`
}

func t(de, en string) Text { return Text{de, en} }

// Display tells the UI how to render a value.
type Display string

const (
	Value    Display = "value"    // number with unit
	Bool     Display = "bool"     // pump/compressor running
	Enum     Display = "enum"     // index into Options (minus Offset)
	Duration Display = "duration" // seconds
	Fault    Display = "fault"    // non-zero = fault active
	Ratio    Display = "ratio"    // computed: value of Num / value of Den
)

// HA carries Home Assistant discovery hints.
type HA struct {
	Component   string `json:"component,omitempty"` // sensor, binary_sensor, select, number
	DeviceClass string `json:"deviceClass,omitempty"`
	StateClass  string `json:"stateClass,omitempty"`
	Category    string `json:"category,omitempty"` // "", "config", "diagnostic"
	Icon        string `json:"icon,omitempty"`
}

// Item is one value on a page.
type Item struct {
	Key      string        `json:"key"` // stable identifier (also used for HA unique ids)
	Kind     protocol.Kind `json:"kind"`
	ID       int           `json:"id"`
	Label    Text          `json:"label"`
	Unit     string        `json:"unit,omitempty"`
	Display  Display       `json:"display"`
	Options  []Text        `json:"options,omitempty"`
	Offset   int           `json:"offset,omitempty"` // option index = value - Offset
	Writable bool          `json:"writable,omitempty"`
	Min      float64       `json:"min,omitempty"`
	Max      float64       `json:"max,omitempty"`
	Step     float64       `json:"step,omitempty"`
	// NotSelectable lists option values the user may not choose (shown only).
	NotSelectable []int `json:"notSelectable,omitempty"`
	// Ratio items: Num / Den measuring points.
	Num int `json:"num,omitempty"`
	Den int `json:"den,omitempty"`
	HA  HA  `json:"ha"`
}

// HeatCurve references the setup points of one heating circuit's curve.
type HeatCurve struct {
	Boost     int `json:"boost"`     // "Aufheiztemperatur": offset during boost periods (K)
	Reduction int `json:"reduction"` // "Absenktemperatur": offset during reduced periods (K, negative)
	Limit     int `json:"limit"`     // "Heizgrenze": outdoor temperature above which heating stops
	AtLimit   int `json:"atLimit"`   // return target at the heating limit
	At0       int `json:"at0"`       // return target at 0 °C outdoor
	AtMinus15 int `json:"atMinus15"` // return target at -15 °C outdoor
}

// Panel is one page (homeControl tab).
type Panel struct {
	ID        string     `json:"id"`
	Title     Text       `json:"title"`
	Subtitle  Text       `json:"subtitle,omitzero"`
	Items     []Item     `json:"items"`
	HeatCurve *HeatCurve `json:"heatCurve,omitempty"`
}

// ---------------------------------------------------------------------------

var (
	modes = []Text{t("Aus", "Off"), t("Automatik", "Automatic"), t("Kühlen", "Cooling"), t("Sommer", "Summer"),
		t("Dauerbetrieb", "Continuous operation"), t("Absenkung", "Reduced"), t("Urlaub", "Holiday"), t("Party", "Party"),
		t("Ausheizen", "Concrete dry-out"), t("EVU-Sperre", "Utility block"), t("Hauptschalter", "Main switch")}
	// "Main switch" is only reported while the main switch is off; homeControl
	// does not let the user select it.
	modesNotSelectable = []int{10}

	compressorStates = []Text{t("Stillstandszeit", "Off time"), t("2. Stufe", "2nd step"), t("Nachlaufzeit HKP", "Pump overrun (CP)"),
		t("Verdichter vorwärmen", "Compressor heat-up"), t("Nachlaufzeit EQP", "Pump overrun (source)"), t("Einspritzzeit", "Injection time"),
		t("Vorlaufzeit", "Pre-run time"), t("Startvorgang", "Starting"), t("Verdichter läuft", "Compressor running"),
		t("Absaugen", "Suction"), t("Abschaltung", "Shut-off")}

	requests = []Text{t("Keine Anforderung", "No request"), t("Kühlen", "Cooling"), t("Heizen", "Heating"),
		t("Warmwasser", "Hot water"), t("Externe Anforderung", "External request"), t("Ausheizen", "Concrete dry-out"), t("Absaugen", "Suction")}
)

func temp(key string, id int, label Text) Item {
	return Item{Key: key, Kind: protocol.MP, ID: id, Label: label, Unit: "°C", Display: Value,
		HA: HA{Component: "sensor", DeviceClass: "temperature", StateClass: "measurement"}}
}

func running(key string, id int, label Text) Item {
	return Item{Key: key, Kind: protocol.MP, ID: id, Label: label, Display: Bool,
		HA: HA{Component: "binary_sensor", DeviceClass: "running"}}
}

func pressure(key string, id int, label Text) Item {
	return Item{Key: key, Kind: protocol.MP, ID: id, Label: label, Unit: "bar", Display: Value,
		HA: HA{Component: "sensor", DeviceClass: "pressure", StateClass: "measurement", Category: "diagnostic"}}
}

func setpoint(key string, id int, label Text, unit string, min, max, step float64) Item {
	return Item{Key: key, Kind: protocol.SP, ID: id, Label: label, Unit: unit, Display: Value, Writable: true, Min: min, Max: max, Step: step,
		HA: HA{Component: "number", DeviceClass: map[bool]string{true: "temperature"}[unit == "°C"], Category: "config"}}
}

func mode(key string, id int, label Text) Item {
	return Item{Key: key, Kind: protocol.SP, ID: id, Label: label, Display: Enum, Options: modes, Writable: true, NotSelectable: modesNotSelectable,
		HA: HA{Component: "select", Icon: "mdi:heat-pump"}}
}

func energy(key string, id int, label Text) Item {
	return Item{Key: key, Kind: protocol.MP, ID: id, Label: label, Unit: "kWh", Display: Value,
		HA: HA{Component: "sensor", DeviceClass: "energy", StateClass: "total_increasing"}}
}

func ratio(key string, num, den int, label Text) Item {
	return Item{Key: key, Label: label, Display: Ratio, Num: num, Den: den,
		HA: HA{Component: "sensor", StateClass: "measurement", Icon: "mdi:gauge"}}
}

func hours(key string, id int, label Text) Item {
	return Item{Key: key, Kind: protocol.SP, ID: id, Label: label, Unit: "h", Display: Value,
		HA: HA{Component: "sensor", DeviceClass: "duration", StateClass: "total_increasing", Category: "diagnostic"}}
}

func mpValue(key string, id int, label Text, unit, deviceClass string) Item {
	return Item{Key: key, Kind: protocol.MP, ID: id, Label: label, Unit: unit, Display: Value,
		HA: HA{Component: "sensor", DeviceClass: deviceClass, StateClass: "measurement"}}
}

var (
	lOutdoor  = t("Außentemperatur", "Outdoor temperature")
	lFlow     = t("Vorlauf", "Flow temperature")
	lReturn   = t("Rücklauf", "Return temperature")
	lHeatPump = t("Heizkreispumpe", "Heating pump")
	lComp     = t("Verdichter", "Compressor")
	lSrcIn    = t("Energiequelle Eintritt", "Source inlet")
	lSrcOut   = t("Energiequelle Austritt", "Source outlet")
	lSuction  = t("Sauggas", "Suction gas")
	lEvap     = t("Verdampfung", "Evaporation")
	lCond     = t("Kondensation", "Condensation")
	lLowP     = t("Niederdruck", "Low pressure")
	lHighP    = t("Hochdruck", "High pressure")
	lSrcPump  = t("Energiequellenpumpe", "Source pump")
	lDHW      = t("Warmwasser", "Hot water")
	lBuffer   = t("Puffer", "Buffer")
	lBufPump  = t("Pufferladepumpe", "Buffer pump")
	lDHWPump  = t("Warmwasservorrang", "Hot water priority")
	lFWS      = t("Frischwasserstation", "Fresh water station")
	lFWSPump  = t("Frischwasserpumpe", "Fresh water pump")
)

func mainPanel() Panel {
	return Panel{ID: "main", Title: t("Grundeinstellung", "Main settings"),
		Items: []Item{
			temp("outdoor", 0, lOutdoor),
			temp("flow", 3, lFlow),
			temp("return", 4, lReturn),
			temp("return_target", 57, t("Rücklauf Soll", "Return target")),
			{Key: "request", Kind: protocol.MP, ID: 56, Label: t("Anforderung", "Request"), Display: Enum, Options: requests,
				HA: HA{Component: "sensor", DeviceClass: "enum"}},
			{Key: "compressor_state", Kind: protocol.SP, ID: 10, Label: t("Verdichterstatus", "Compressor state"), Display: Enum, Options: compressorStates,
				HA: HA{Component: "sensor", DeviceClass: "enum"}},
			{Key: "running_since", Kind: protocol.SP, ID: 11, Label: t("Läuft seit", "Running since"), Unit: "s", Display: Duration,
				HA: HA{Component: "sensor", DeviceClass: "duration", Category: "diagnostic"}},
			mode("mode", 13, t("Betriebsart", "Operating mode")),
			{Key: "fault", Kind: protocol.MP, ID: 31, Label: t("Störung", "Fault"), Display: Fault,
				HA: HA{Component: "binary_sensor", DeviceClass: "problem"}},
			setpoint("room_target", 69, t("Raum Soll", "Room target"), "°C", 10, 25, 0.5),
			setpoint("boost", 71, t("Aufheiztemperatur", "Boost offset"), "K", 1, 10, 1),
			setpoint("reduction", 72, t("Absenktemperatur", "Reduction offset"), "K", -10, -1, 1),
			setpoint("heating_limit", 76, t("Heizgrenze", "Heating limit"), "°C", 10, 24, 1),
			setpoint("curve_at_limit", 80, t("RL Soll bei Heizgrenze", "Return target at heating limit"), "°C", 15, 40, 0.1),
			setpoint("curve_at_0", 81, t("RL Soll bei 0 °C", "Return target at 0 °C"), "°C", 20, 50, 0.1),
			setpoint("curve_at_minus15", 82, t("RL Soll bei -15 °C", "Return target at -15 °C"), "°C", 25, 60, 0.1),
			hours("hours_dhw", 171, t("Betriebsstunden Warmwasser", "Operating hours hot water")),
			hours("hours_heating", 172, t("Betriebsstunden Heizen", "Operating hours heating")),
			hours("hours_total", 173, t("Betriebsstunden gesamt", "Operating hours total")),
		},
		HeatCurve: &HeatCurve{Boost: 71, Reduction: 72, Limit: 76, AtLimit: 80, At0: 81, AtMinus15: 82},
	}
}

func mixerPanel(n int) Panel {
	mp, sp := 63, 200
	if n == 2 {
		mp, sp = 69, 223
	}
	id := fmt.Sprintf("mixer%d", n)
	k := func(s string) string { return id + "_" + s }
	return Panel{ID: id, Title: t(fmt.Sprintf("Mischerkreis %d", n), fmt.Sprintf("Mixer circuit %d", n)),
		Items: []Item{
			temp(k("flow"), mp, lFlow),
			temp(k("return"), mp+1, lReturn),
			temp(k("target"), mp+3, t("Soll", "Target")),
			mode(k("mode"), sp+21, t("Betriebsart", "Operating mode")),
			setpoint(k("room_target"), sp, t("Raum Soll", "Room target"), "°C", 10, 25, 0.5),
			setpoint(k("boost"), sp+2, t("Aufheiztemperatur", "Boost offset"), "K", 1, 10, 1),
			setpoint(k("reduction"), sp+3, t("Absenktemperatur", "Reduction offset"), "K", -10, -1, 1),
			setpoint(k("heating_limit"), sp+5, t("Heizgrenze", "Heating limit"), "°C", 10, 23, 1),
			setpoint(k("curve_at_limit"), sp+9, t("RL Soll bei Heizgrenze", "Target at heating limit"), "°C", 15, 40, 0.1),
			setpoint(k("curve_at_0"), sp+10, t("RL Soll bei 0 °C", "Target at 0 °C"), "°C", 20, 50, 0.1),
			setpoint(k("curve_at_minus15"), sp+11, t("RL Soll bei -15 °C", "Target at -15 °C"), "°C", 25, 60, 0.1),
		},
		HeatCurve: &HeatCurve{Boost: sp + 2, Reduction: sp + 3, Limit: sp + 5, AtLimit: sp + 9, At0: sp + 10, AtMinus15: sp + 11},
	}
}

// sourcePanel: homeControl had separate tabs per energy source with the same
// title. Its brine and water tabs bound all three pump/compressor indicators to
// MP 22, evidently a copy-paste slip; we use the IDs of the other tabs (22
// heating pump, 24 source pump, 30 compressor).
func sourcePanel(kind string) Panel {
	common := []Item{temp("outdoor", 0, lOutdoor), temp("flow", 3, lFlow), temp("return", 4, lReturn)}
	var sub Text
	var items []Item
	switch kind {
	case "brine", "water":
		sub = map[string]Text{"brine": t("Sole", "Brine"), "water": t("Wasser", "Water")}[kind]
		items = []Item{temp("source_in", 6, lSrcIn), temp("source_out", 7, lSrcOut), temp("suction_gas", 9, lSuction),
			temp("evaporation", 12, lEvap), temp("condensation", 13, lCond), pressure("low_pressure", 20, lLowP), pressure("high_pressure", 21, lHighP),
			running("heating_pump", 22, lHeatPump), running("source_pump", 24, lSrcPump), running("compressor", 30, lComp)}
	case "ground":
		sub = t("Erdreich (Direktverdampfung)", "Ground (direct expansion)")
		items = []Item{temp("evaporation", 12, lEvap), temp("condensation", 13, lCond), pressure("low_pressure", 20, lLowP),
			pressure("high_pressure", 21, lHighP), running("heating_pump", 22, lHeatPump), running("compressor", 30, lComp)}
	case "air":
		sub = t("Luft", "Air")
		items = []Item{temp("source_in", 6, lSrcIn), temp("evaporation", 12, lEvap), temp("condensation", 13, lCond),
			pressure("low_pressure", 20, lLowP), pressure("high_pressure", 21, lHighP), running("heating_pump", 22, lHeatPump),
			running("fan", 24, t("Ventilator", "Fan")), running("compressor", 30, lComp), running("switch_valve", 32, t("Umschaltventil", "Switch valve"))}
	}
	return Panel{ID: "source_" + kind, Title: t("Energiequelle", "Energy source"), Subtitle: sub, Items: append(common, items...)}
}

// plantPanel is homeControl's "Anlagenschema" tab; the variant (1..10) follows
// from the installation bit mask, see Resolve.
func plantPanel(variant int) Panel {
	has := func(vs ...int) bool { return slices.Contains(vs, variant) }
	items := []Item{temp("flow", 3, lFlow), temp("return", 4, lReturn), running("heating_pump", 22, lHeatPump)}
	if variant >= 3 {
		items = append(items, temp("dhw", 2, lDHW))
	}
	if has(2, 4, 6, 8, 10) {
		items = append(items, temp("buffer", 5, lBuffer), running("buffer_pump", 23, lBufPump))
	}
	if has(5, 6, 9, 10) {
		items = append(items, running("dhw_priority", 25, lDHWPump))
	}
	if has(7, 8, 9, 10) {
		items = append(items, temp("fws", 11, lFWS), running("fws_pump", 50, lFWSPump))
	}
	if variant >= 3 {
		items = append(items,
			setpoint("dhw_target", 83, t("Warmwasser Soll", "Hot water target"), "°C", 10, 75, 1),
			setpoint("dhw_min", 85, t("Warmwasser Minimum", "Hot water minimum"), "°C", 5, 45, 1))
	}
	return Panel{ID: "plant", Title: t("Anlagenschema", "Installation"), Subtitle: plantVariantNames[variant], Items: items}
}

var plantVariantNames = map[int]Text{
	1: t("Heizen", "Heating"), 2: t("Heizen mit Puffer", "Heating with buffer"),
	3: t("Heizen + Warmwasser", "Heating + hot water"), 4: t("Puffer + Warmwasser", "Buffer + hot water"),
	5: t("Warmwasser mit Ladepumpe", "Hot water with charge pump"), 6: t("Puffer + Warmwasser mit Ladepumpe", "Buffer + hot water with charge pump"),
	7: t("Frischwasserstation", "Fresh water station"), 8: t("Puffer + Frischwasserstation", "Buffer + fresh water station"),
	9: t("Frischwasserstation mit Pumpe", "Fresh water station with pump"), 10: t("Puffer + Frischwasserstation mit Pumpe", "Buffer + fresh water station with pump"),
}

func energyPanel() Panel {
	return Panel{ID: "energy", Title: t("Energiezählung", "Energy metering"),
		Items: []Item{
			energy("energy_heating_out", 52, t("Heizen abgegeben", "Heating delivered")),
			energy("energy_heating_in", 53, t("Heizen aufgenommen", "Heating consumed")),
			ratio("cop_heating", 52, 53, t("Arbeitszahl Heizen", "SPF heating")),
			energy("energy_dhw_out", 54, t("Warmwasser abgegeben", "Hot water delivered")),
			energy("energy_dhw_in", 55, t("Warmwasser aufgenommen", "Hot water consumed")),
			ratio("cop_dhw", 54, 55, t("Arbeitszahl Warmwasser", "SPF hot water")),
			energy("energy_total_out", 84, t("Gesamt abgegeben", "Total delivered")),
			energy("energy_total_in", 75, t("Gesamt aufgenommen", "Total consumed")),
			ratio("cop_total", 84, 75, t("Arbeitszahl gesamt", "SPF total")),
			mpValue("voltage_l1", 76, t("Spannung L1", "Voltage L1"), "V", "voltage"),
			mpValue("voltage_l2", 77, t("Spannung L2", "Voltage L2"), "V", "voltage"),
			mpValue("voltage_l3", 78, t("Spannung L3", "Voltage L3"), "V", "voltage"),
			mpValue("current_l1", 79, t("Strom L1", "Current L1"), "A", "current"),
			mpValue("current_l2", 80, t("Strom L2", "Current L2"), "A", "current"),
			mpValue("current_l3", 81, t("Strom L3", "Current L3"), "A", "current"),
			mpValue("frequency", 82, t("Frequenz", "Frequency"), "Hz", "frequency"),
			mpValue("power_in", 83, t("Leistungsaufnahme", "Electrical power"), "W", "power"),
			mpValue("power_out", 89, t("Wärmeleistung", "Heat output"), "kW", "power"),
			mpValue("hm_flow", 85, t("WMZ Durchfluss", "Heat meter flow"), "l/h", ""),
			temp("hm_temp_in", 86, t("WMZ Temperatur ein", "Heat meter temperature in")),
			temp("hm_temp_out", 87, t("WMZ Temperatur aus", "Heat meter temperature out")),
			mpValue("hm_cop", 92, t("WMZ COP", "Heat meter COP"), "", ""),
		},
	}
}

// Installation describes the configuration bits read from the controller.
type Installation struct {
	PlantType int  `json:"plantType"` // SP 9 ("WNA")
	Mixer1    bool `json:"mixer1"`    // SP 199 > 0 ("MKR1 Aktiviert")
	Mixer2    bool `json:"mixer2"`    // SP 222 > 0
	Energy    bool `json:"energy"`    // SP 263 > 0
}

// ConfigPoints are the setup points that decide which pages exist.
var ConfigPoints = []int{protocol.PlantTypeSP, 199, 222, 263}

// PlantVariant maps the installation bits to homeControl's diagram 1..10.
func PlantVariant(wna int) int {
	n := 1
	if wna&0x20 != 0 {
		n = 2
	}
	if wna&0x100 != 0 {
		n += 2
	}
	if wna&0x400 != 0 {
		n += 2
	}
	if wna&0x200 != 0 {
		n += 4
	}
	if wna&0x800 != 0 {
		n += 4
	}
	return n
}

// Resolve returns the pages present in an installation, in homeControl's order.
func Resolve(in Installation) []Panel {
	panels := []Panel{mainPanel()}
	if in.PlantType == 0 {
		return panels
	}
	if in.PlantType&0x2000 != 0 || in.Mixer1 {
		panels = append(panels, mixerPanel(1))
	}
	if in.Mixer2 {
		panels = append(panels, mixerPanel(2))
	}
	for _, s := range []struct {
		bit  int
		kind string
	}{{0x1, "brine"}, {0x2, "water"}, {0x4, "ground"}, {0x8, "air"}} {
		if in.PlantType&s.bit != 0 {
			panels = append(panels, sourcePanel(s.kind))
		}
	}
	if v := PlantVariant(in.PlantType); v >= 1 && v <= 10 {
		panels = append(panels, plantPanel(v))
	}
	if in.Energy {
		panels = append(panels, energyPanel())
	}
	return panels
}

// Points returns the measuring and setup points needed to fill the panels.
func Points(panels []Panel) (mps, sps []int) {
	add := func(list []int, id int) []int {
		if !slices.Contains(list, id) {
			list = append(list, id)
		}
		return list
	}
	for _, p := range panels {
		for _, it := range p.Items {
			switch {
			case it.Display == Ratio:
				mps = add(add(mps, it.Num), it.Den)
			case it.Kind == protocol.MP:
				mps = add(mps, it.ID)
			default:
				sps = add(sps, it.ID)
			}
		}
	}
	for _, id := range ConfigPoints {
		sps = add(sps, id)
	}
	slices.Sort(mps)
	slices.Sort(sps)
	return mps, sps
}

// FindWritable returns the catalog item for a writable setup point, if any.
func FindWritable(panels []Panel, id int) (Item, bool) {
	for _, p := range panels {
		for _, it := range p.Items {
			if it.Kind == protocol.SP && it.ID == id && it.Writable {
				return it, true
			}
		}
	}
	return Item{}, false
}

// TrendPoints are the measuring points homeControl offers for trend recording.
var TrendPoints = []int{0, 1, 2, 3, 4, 5, 6, 7, 10, 12, 13, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 28, 29, 30, 31, 32, 33, 34, 35, 36,
	37, 38, 41, 49, 50, 51, 52, 53, 54, 55, 56, 57, 63, 64, 65, 69, 70, 71, 75, 76, 77, 78, 79, 80, 81, 82, 83, 84, 85, 86, 87, 88, 89, 90, 92, 94, 95, 96}

// HeatCurveTarget evaluates a heating curve: the return target for an outdoor
// temperature. The curve is two straight segments through (limit, atLimit),
// (0, at0) and (-15, atMinus15), flat outside; shift adds the boost or
// reduction offset.
func HeatCurveTarget(outdoor, limit, atLimit, at0, atMinus15, shift float64) float64 {
	var v float64
	switch {
	case outdoor >= limit:
		v = atLimit
	case outdoor >= 0:
		v = at0 + (atLimit-at0)*outdoor/limit
	case outdoor >= -15:
		v = at0 + (atMinus15-at0)*(-outdoor)/15
	default:
		v = atMinus15
	}
	return v + shift
}
