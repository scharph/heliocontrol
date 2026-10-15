// Package protocol builds the ASCII commands of the Heliotherm web control and
// parses its replies. Every message is "~CMD,KEY=VALUE,...;" wrapped in a
// frame (see package frame).
//
// The command set and reply grammar were recovered from homeControl 2.1.11
// (HeliothermDLL.Communication / GlobalData). Parsing follows that client:
// fields are matched individually, so their order does not matter and unknown
// fields are ignored.
package protocol

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Kind distinguishes measuring points (sensors, read-mostly) from setup points
// (parameters).
type Kind string

const (
	MP Kind = "mp"
	SP Kind = "sp"
)

// Point is a measuring point (MP) or setup point (SP) as reported by the device.
// Trend/alarm fields only exist on measuring points.
type Point struct {
	Kind Kind    `json:"kind"`
	ID   int     `json:"id"`
	NR   int     `json:"nr"`
	Name string  `json:"name"`
	Val  float64 `json:"value"`
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Len  int     `json:"len,omitempty"`
	Bit  int     `json:"bit,omitempty"`
	TP   bool    `json:"tp,omitempty"`
	ACS  int     `json:"acs"`                // access level
	US   bool    `json:"used"`               // point is used in this installation
	WR   bool    `json:"writable"`           // point may be written
	ORF  bool    `json:"override,omitempty"` // MP: manual override active
	ORV  float64 `json:"overrideValue,omitempty"`
	OFFV float64 `json:"offset,omitempty"`
	// Trend / alarm configuration (MP only)
	ERF  bool      `json:"alarm,omitempty"`      // alarm when outside MIN/MAX
	RT1  bool      `json:"callRemote,omitempty"` // call remote maintenance on alarm
	RT2  bool      `json:"rt2,omitempty"`
	RTL  bool      `json:"rtl,omitempty"`
	TRF  bool      `json:"trend,omitempty"`     // trend recording enabled
	TRT  int       `json:"trendType,omitempty"` // 0 = interval, 1 = hysteresis
	TRHV float64   `json:"trendHysteresis,omitempty"`
	TRI  int       `json:"trendInterval,omitempty"` // seconds
	TE   time.Time `json:"trendEnd,omitzero"`
}

var (
	reFirstInt = regexp.MustCompile(`\d+`)
	reNumber   = regexp.MustCompile(`-?\d+(\.\d+)?`)
	reTrendTS  = regexp.MustCompile(`\d{1,2}\.\d{1,2}\.\d{1,2}.\d{1,2}:\d{1,2}:\d{1,2}`)
	reClock    = regexp.MustCompile(`DA=(\d{1,2})\.(\d{1,2})\.(\d{1,2}),TI=(\d{1,2}):(\d{1,2}):(\d{1,2})`)
	rePRI      = regexp.MustCompile(`PRI(\d+)`)
	reHHMM     = regexp.MustCompile(`^(\d{1,2}):(\d{1,2})$`)
)

// Fields splits a reply such as "~MP,NR=3,VAL=21.5;" into its tag ("MP") and a
// key/value map. Tokens without '=' are returned in Args.
type Fields struct {
	Tag  string
	KV   map[string]string
	Args []string
}

func Split(reply string) Fields {
	s := strings.TrimSpace(reply)
	s = strings.TrimPrefix(s, "~")
	s = strings.TrimSuffix(s, ";")
	parts := strings.Split(s, ",")
	f := Fields{KV: map[string]string{}}
	for i, p := range parts {
		k, v, ok := strings.Cut(p, "=")
		switch {
		case i == 0 && !ok:
			f.Tag = p
		case ok:
			f.KV[strings.TrimSpace(k)] = v
		default:
			f.Args = append(f.Args, p)
		}
	}
	return f
}

func (f Fields) has(k string) bool { _, ok := f.KV[k]; return ok }

func (f Fields) int(k string) int {
	n, _ := strconv.Atoi(reFirstInt.FindString(f.KV[k]))
	return n
}

func (f Fields) float(k string) float64 {
	v, _ := strconv.ParseFloat(reNumber.FindString(f.KV[k]), 64)
	return v
}

func (f Fields) bool(k string) bool { return f.KV[k] == "1" }

// ParsePoint parses a "~MP,..." or "~SP,..." reply.
func ParsePoint(reply string) (Point, error) {
	f := Split(reply)
	var p Point
	switch f.Tag {
	case "MP":
		p.Kind = MP
	case "SP":
		p.Kind = SP
	default:
		return p, fmt.Errorf("protocol: not a point reply: %q", reply)
	}
	if !f.has("ID") && !f.has("NR") {
		return p, fmt.Errorf("protocol: point reply without ID/NR: %q", reply)
	}
	p.ID, p.NR = f.int("ID"), f.int("NR")
	if !f.has("ID") {
		p.ID = p.NR
	}
	p.Name = f.KV["NAME"]
	p.Val, p.Min, p.Max = f.float("VAL"), f.float("MIN"), f.float("MAX")
	p.Len, p.Bit, p.TP = f.int("LEN"), f.int("BIT"), f.bool("TP")
	p.ACS, p.US, p.WR = f.int("ACS"), f.bool("US"), f.bool("WR")
	p.ORF, p.ORV, p.OFFV = f.bool("ORF"), f.float("ORV"), f.float("OFFV")
	p.ERF, p.RT1, p.RT2, p.RTL = f.bool("ERF"), f.bool("RT1"), f.bool("RT2"), f.bool("RTL")
	p.TRF, p.TRT, p.TRHV, p.TRI = f.bool("TRF"), f.int("TRT"), f.float("TRHV"), f.int("TRI")
	if te, ok := f.KV["TE"]; ok {
		if t, err := ParseDeviceTime(te); err == nil {
			p.TE = t
		}
	}
	return p, nil
}

// ParseDeviceTime parses "dd.MM.yy-HH:mm:ss" (any single separator between date
// and time). Device times are local wall-clock times without a zone.
func ParseDeviceTime(s string) (time.Time, error) {
	m := regexp.MustCompile(`^(\d{1,2})\.(\d{1,2})\.(\d{1,2}).(\d{1,2}):(\d{1,2}):(\d{1,2})$`).FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return time.Time{}, fmt.Errorf("protocol: bad device time %q", s)
	}
	n := make([]int, 6)
	for i := range n {
		n[i], _ = strconv.Atoi(m[i+1])
	}
	return time.Date(2000+n[2], time.Month(n[1]), n[0], n[3], n[4], n[5], 0, time.Local), nil
}

// FormatDeviceTime renders t as "dd.MM.yy-HH:mm:ss".
func FormatDeviceTime(t time.Time) string { return t.Format("02.01.06-15:04:05") }

// FormatValue renders a number the way the device expects (dot decimal, no exponent).
func FormatValue(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// ParseCount extracts the first integer of a reply ("~SUM,123;" style answers
// to ~RM, ~RS, ~PRL, ~TRS).
func ParseCount(reply string) (int, error) {
	m := reFirstInt.FindString(reply)
	if m == "" {
		return 0, fmt.Errorf("protocol: no count in %q", reply)
	}
	return strconv.Atoi(m)
}

// Value is one entry of a "~MA" multi-value reply.
type Value struct {
	ID  int     `json:"id"`
	Val float64 `json:"value"`
}

// ParseValue parses "~MA..." replies: the first integer is the point ID, the next
// number its value.
func ParseValue(reply string) (Value, error) {
	if !strings.HasPrefix(reply, "~MA") {
		return Value{}, fmt.Errorf("protocol: not a value reply: %q", reply)
	}
	loc := reFirstInt.FindStringIndex(reply)
	if loc == nil {
		return Value{}, fmt.Errorf("protocol: value reply without id: %q", reply)
	}
	id, _ := strconv.Atoi(reply[loc[0]:loc[1]])
	num := reNumber.FindString(reply[loc[1]:])
	if num == "" {
		return Value{}, fmt.Errorf("protocol: value reply without value: %q", reply)
	}
	v, _ := strconv.ParseFloat(num, 64)
	return Value{ID: id, Val: v}, nil
}

// TrendRecord is one entry of the device trend buffer.
type TrendRecord struct {
	ID     int       `json:"id"`
	MPID   int       `json:"mpId"`
	Time   time.Time `json:"time"`
	Value  float64   `json:"value"`
	Reason int       `json:"reason"`
}

// ParseTrendRecord parses a trend buffer reply. The grammar is positional:
// record id, measuring point id, timestamp, value, reason.
func ParseTrendRecord(reply string) (TrendRecord, error) {
	var r TrendRecord
	bad := fmt.Errorf("protocol: bad trend record %q", reply)
	loc := reFirstInt.FindStringIndex(reply)
	if loc == nil {
		return r, bad
	}
	r.ID, _ = strconv.Atoi(reply[loc[0]:loc[1]])
	rest := reply[loc[1]:]
	if loc = reFirstInt.FindStringIndex(rest); loc == nil {
		return r, bad
	}
	r.MPID, _ = strconv.Atoi(rest[loc[0]:loc[1]])
	rest = rest[loc[1]:]
	if loc = reTrendTS.FindStringIndex(rest); loc == nil {
		return r, bad
	}
	t, err := ParseDeviceTime(rest[loc[0]:loc[1]])
	if err != nil {
		return r, err
	}
	r.Time = t
	rest = rest[loc[1]:]
	if loc = reNumber.FindStringIndex(rest); loc == nil {
		return r, bad
	}
	r.Value, _ = strconv.ParseFloat(rest[loc[0]:loc[1]], 64)
	rest = rest[loc[1]:]
	if loc = reFirstInt.FindStringIndex(rest); loc == nil {
		return r, bad
	}
	r.Reason, _ = strconv.Atoi(rest[loc[0]:loc[1]])
	return r, nil
}

// ParseClock parses "DA=dd.MM.yy,TI=HH:mm:ss" out of a ~CLK reply.
func ParseClock(reply string) (time.Time, error) {
	m := reClock.FindStringSubmatch(reply)
	if m == nil {
		return time.Time{}, fmt.Errorf("protocol: bad clock reply %q", reply)
	}
	n := make([]int, 6)
	for i := range n {
		n[i], _ = strconv.Atoi(m[i+1])
	}
	return time.Date(2000+n[2], time.Month(n[1]), n[0], n[3], n[4], n[5], 0, time.Local), nil
}

// TimeProgram describes one time program (heating, DHW, circulation, ...).
type TimeProgram struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	EAD  int    `json:"eventsPerDay"` // number of switching events per day
	NOS  int    `json:"states"`       // number of states (2 = off/on, 3 = normal/decrease/boost)
	STE  int    `json:"step"`         // time step in minutes
	NOD  int    `json:"days"`         // number of days (7)
	US   bool   `json:"used"`
}

// ParseTimeProgram parses a "~PRI<id>,NAME=..,EAD=..,NOS=..,STE=..,NOD=..,US=.." reply.
func ParseTimeProgram(reply string) (TimeProgram, error) {
	m := rePRI.FindStringSubmatch(reply)
	if m == nil {
		return TimeProgram{}, fmt.Errorf("protocol: bad time program %q", reply)
	}
	f := Split(reply)
	tp := TimeProgram{Name: f.KV["NAME"], EAD: f.int("EAD"), NOS: f.int("NOS"), STE: f.int("STE"), NOD: f.int("NOD"), US: f.bool("US")}
	tp.ID, _ = strconv.Atoi(m[1])
	if tp.NOD == 0 || tp.EAD == 0 {
		return tp, fmt.Errorf("protocol: time program without days/events %q", reply)
	}
	return tp, nil
}

// ClockTime is a time of day in minutes since midnight. 1440 means 24:00 (end of day).
type ClockTime int

func ParseClockTime(s string) (ClockTime, error) {
	m := reHHMM.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, fmt.Errorf("protocol: bad time %q", s)
	}
	h, _ := strconv.Atoi(m[1])
	mi, _ := strconv.Atoi(m[2])
	if h > 24 || mi > 59 || (h == 24 && mi != 0) {
		return 0, fmt.Errorf("protocol: bad time %q", s)
	}
	return ClockTime(h*60 + mi), nil
}

func (c ClockTime) String() string { return fmt.Sprintf("%02d:%02d", int(c)/60, int(c)%60) }

func (c ClockTime) MarshalText() ([]byte, error) { return []byte(c.String()), nil }

func (c *ClockTime) UnmarshalText(b []byte) error {
	v, err := ParseClockTime(string(b))
	*c = v
	return err
}

// TimeProgramEvent is one switching period of a time program day.
type TimeProgramEvent struct {
	Program int       `json:"program"`
	Day     int       `json:"day"` // 0 = Monday
	Event   int       `json:"event"`
	State   int       `json:"state"`
	Begin   ClockTime `json:"begin"`
	End     ClockTime `json:"end"`
}

// ParseTimeProgramEvent parses "~PRE,PR=..,DAY=..,EV=..,ST=..,BEG=HH:MM,END=HH:MM".
func ParseTimeProgramEvent(reply string) (TimeProgramEvent, error) {
	f := Split(reply)
	for _, k := range []string{"PR", "DAY", "EV", "ST", "BEG", "END"} {
		if !f.has(k) {
			return TimeProgramEvent{}, fmt.Errorf("protocol: time program event without %s: %q", k, reply)
		}
	}
	ev := TimeProgramEvent{Program: f.int("PR"), Day: f.int("DAY"), Event: f.int("EV"), State: f.int("ST")}
	var err error
	if ev.Begin, err = ParseClockTime(f.KV["BEG"]); err != nil {
		return ev, err
	}
	if ev.End, err = ParseClockTime(f.KV["END"]); err != nil {
		return ev, err
	}
	return ev, nil
}

// IsOK reports whether a reply is the generic acknowledgement "~OK;".
func IsOK(reply string) bool { return strings.HasPrefix(reply, "~OK") }
