// Package sim is a simulated Heliotherm web control. It speaks the framed
// ASCII protocol on a TCP listener so the server and UI can be developed and
// tested without a heat pump.
//
// Command handling follows what homeControl sends; the exact reply layouts of a
// real controller were not observable, so replies are shaped after what
// homeControl's parser accepts (see package protocol).
package sim

import (
	"fmt"
	"log/slog"
	"math"
	"net"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"heliocontrol/internal/frame"
	"heliocontrol/internal/protocol"
)

type point struct {
	name     string
	val      float64
	min, max float64
	used, wr bool
	// trend configuration (MP)
	erf, rt1, trf bool
	trt, tri      int
	trhv          float64
	te            time.Time
	lastTrend     time.Time
	lastTrendVal  float64
}

type trendRec struct {
	mp     int
	t      time.Time
	val    float64
	reason int
}

// Device is the simulated controller state.
type Device struct {
	mu          sync.Mutex
	ID          int
	mp, sp      map[int]*point
	nMP, nSP    int
	trend       []trendRec
	trendPaused bool
	clockOffset time.Duration
	programs    []simProgram
	start       time.Time
	lastStep    time.Time
	Latency     time.Duration
	log         *slog.Logger
	commands    map[string]int // received commands by tag (for tests)
}

// CommandCount returns how many commands with this tag (e.g. "SP", "MR") were received.
func (d *Device) CommandCount(tag string) int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.commands[tag]
}

type simProgram struct {
	protocol.TimeProgram
	events []protocol.TimeProgramEvent
}

// Options configure the simulated installation.
type Options struct {
	ID        int
	PlantType int  // SP 9 bit mask, see catalog.PlantType
	Mixer2    bool // SP 222
	Energy    bool // SP 263
	Fault     bool // start with an active fault (MP 31)
}

func DefaultOptions() Options {
	return Options{ID: 123456, PlantType: 0x2000 | 0x01 | 0x20 | 0x100, Mixer2: true, Energy: true}
}

func New(o Options, log *slog.Logger) *Device {
	if log == nil {
		log = slog.Default()
	}
	d := &Device{ID: o.ID, mp: map[int]*point{}, sp: map[int]*point{}, nMP: 100, nSP: 280, start: time.Now(), Latency: 15 * time.Millisecond, log: log}
	for i := 0; i < d.nMP; i++ {
		d.mp[i] = &point{name: fmt.Sprintf("MP %d", i), min: -50, max: 150}
	}
	for i := 0; i < d.nSP; i++ {
		d.sp[i] = &point{name: fmt.Sprintf("SP %d", i), min: 0, max: 100}
	}
	mp := func(id int, name string, v, min, max float64) {
		d.mp[id] = &point{name: name, val: v, min: min, max: max, used: true, wr: true, te: time.Date(2099, 12, 31, 23, 59, 0, 0, time.Local)}
	}
	sp := func(id int, name string, v, min, max float64) {
		d.sp[id] = &point{name: name, val: v, min: min, max: max, used: true, wr: true}
	}
	for id, n := range map[int]string{0: "Aussentemperatur", 2: "Warmwasser", 3: "Vorlauf", 4: "Ruecklauf", 5: "Puffer", 6: "EQ Eintritt", 7: "EQ Austritt",
		9: "Sauggas", 11: "Frischwasser", 12: "Verdampfung", 13: "Kondensation", 57: "Ruecklauf Soll", 63: "MKR1 Vorlauf", 64: "MKR1 Ruecklauf",
		66: "MKR1 Soll", 69: "MKR2 Vorlauf", 70: "MKR2 Ruecklauf", 72: "MKR2 Soll", 86: "WMZ Temp ein", 87: "WMZ Temp aus", 88: "WMZ Temp diff"} {
		mp(id, n, 20, -30, 90)
	}
	mp(20, "Niederdruck", 4, 0, 30)
	mp(21, "Hochdruck", 15, 0, 40)
	for id, n := range map[int]string{22: "HKR Pumpe", 23: "Pufferpumpe", 24: "EQ Pumpe", 25: "WW Pumpe", 30: "Verdichter", 32: "Umschaltventil", 50: "FWS Pumpe"} {
		mp(id, n, 0, 0, 1)
	}
	mp(31, "Stoerung", 0, 0, 1)
	if o.Fault {
		d.mp[31].val = 1
	}
	mp(56, "Anforderung", 0, 0, 6)
	for id, n := range map[int]string{52: "Energie Heizen ab", 53: "Energie Heizen auf", 54: "Energie WW ab", 55: "Energie WW auf", 75: "Energie gesamt auf",
		83: "WMZ P ein", 84: "Energie gesamt ab", 89: "WMZ P aus"} {
		mp(id, n, 0, 0, 1e7)
	}
	for id, n := range map[int]string{76: "Spannung L1", 77: "Spannung L2", 78: "Spannung L3"} {
		mp(id, n, 230, 0, 260)
	}
	for id, n := range map[int]string{79: "Strom L1", 80: "Strom L2", 81: "Strom L3"} {
		mp(id, n, 0, 0, 32)
	}
	d.mp[52].val, d.mp[53].val, d.mp[54].val, d.mp[55].val = 18432, 4310, 5210, 1690
	d.mp[75].val, d.mp[84].val = 6000, 23642
	mp(82, "Frequenz", 50, 45, 55)
	mp(85, "WMZ Durchfluss", 0, 0, 5000)
	mp(92, "WMZ COP", 0, 0, 10)
	for _, id := range []int{0, 3, 4, 2, 5} {
		d.mp[id].trf, d.mp[id].tri = true, 300
	}

	b := func(v bool) float64 {
		if v {
			return 1
		}
		return 0
	}
	sp(protocol.PlantTypeSP, "2.07D ", float64(o.PlantType), 0, 0) // real controllers report the firmware version as name
	sp(10, "Verdichterstatus", 0, 0, 10)
	sp(11, "Laeuft seit", 0, 0, 1e7)
	sp(13, "Betriebsart", 1, 0, 10)
	sp(protocol.FaultResetSP, "Entstoeren", 0, 0, 1)
	sp(69, "Raum Soll", 20, 10, 25)
	sp(71, "Aufheiztemperatur", 3, 1, 10)
	sp(72, "Absenktemperatur", -3, -10, -1)
	sp(76, "Heizgrenze", 18, 10, 24)
	sp(80, "RL Soll Heizgrenze", 22, 15, 40)
	sp(81, "RL Soll 0 Grad", 30, 20, 50)
	sp(82, "RL Soll -15 Grad", 38, 25, 60)
	sp(83, "WW Normaltemperatur", 48, 10, 75)
	sp(85, "WW Minimaltemperatur", 42, 5, 45)
	sp(171, "Betriebsstunden WW", 1834, 0, 1e6)
	sp(172, "Betriebsstunden Heizen", 9721, 0, 1e6)
	sp(173, "Betriebsstunden gesamt", 11555, 0, 1e6)
	for i, n := range []string{"MKR1 Raum Soll", "", "MKR1 Aufheiztemp", "MKR1 Absenktemp", "", "MKR1 Heizgrenze", "", "", "", "MKR1 RL Soll HG", "MKR1 RL Soll 0", "MKR1 RL Soll -15"} {
		if n != "" {
			sp(200+i, n, []float64{20, 0, 3, -3, 0, 18, 0, 0, 0, 25, 32, 40}[i], -10, 60)
		}
	}
	sp(221, "MKR1 Betriebsart", 1, 0, 10)
	sp(222, "MKR2 vorhanden", b(o.Mixer2), 0, 1)
	for i, n := range []string{"MKR2 Raum Soll", "", "MKR2 Aufheiztemp", "MKR2 Absenktemp", "", "MKR2 Heizgrenze", "", "", "", "MKR2 RL Soll HG", "MKR2 RL Soll 0", "MKR2 RL Soll -15"} {
		if n != "" {
			sp(223+i, n, []float64{21, 0, 2, -2, 0, 17, 0, 0, 0, 24, 30, 36}[i], -10, 60)
		}
	}
	sp(244, "MKR2 Betriebsart", 1, 0, 10)
	sp(263, "Energiezaehlung", b(o.Energy), 0, 1)

	for _, p := range []struct {
		name string
		nos  int
	}{{"Heizung", 3}, {"Warmwasser", 2}, {"Zirkulation", 2}} {
		pr := simProgram{TimeProgram: protocol.TimeProgram{ID: len(d.programs), Name: p.name, EAD: 6, NOS: p.nos, STE: 15, NOD: 7, US: true}}
		for day := 0; day < 7; day++ {
			bounds := []protocol.ClockTime{0, 6 * 60, 8 * 60, 16 * 60, 22 * 60, 23 * 60, 1440}
			for ev := 0; ev < 6; ev++ {
				st := 0
				if (p.nos == 2 && ev%2 == 1) || (p.nos == 3 && (ev == 0 || ev == 4 || ev == 5)) {
					st = 1
				}
				pr.events = append(pr.events, protocol.TimeProgramEvent{Program: pr.ID, Day: day, Event: ev, State: st, Begin: bounds[ev], End: bounds[ev+1]})
			}
		}
		d.programs = append(d.programs, pr)
	}
	d.programs = append(d.programs, simProgram{TimeProgram: protocol.TimeProgram{ID: 3, Name: "Reserve", EAD: 6, NOS: 2, STE: 15, NOD: 7}})

	// seed a day of trend history
	now := time.Now()
	for t := now.Add(-24 * time.Hour); t.Before(now); t = t.Add(5 * time.Minute) {
		d.step(t)
		for _, id := range []int{0, 3, 4, 2, 5} {
			d.trend = append(d.trend, trendRec{mp: id, t: t, val: d.mp[id].val, reason: 1})
		}
	}
	return d
}

// step advances the physical model to time t. Callers hold mu.
func (d *Device) step(t time.Time) {
	h := float64(t.Unix()%86400) / 3600
	outdoor := 4 + 6*math.Sin((h-9)/24*2*math.Pi)
	d.mp[0].val = round1(outdoor)
	curve := func(at, hg, rlHG, rl0, rl15 float64) float64 {
		switch {
		case at >= hg:
			return rlHG
		case at >= 0:
			return rl0 + (rlHG-rl0)*at/hg
		default:
			return rl0 + (rl15-rl0)*(-at)/15
		}
	}
	target := curve(outdoor, d.sp[76].val, d.sp[80].val, d.sp[81].val, d.sp[82].val)
	mode := int(d.sp[13].val)
	running := mode != 0 && math.Mod(float64(t.Unix())/60, 30) < 18 && d.mp[31].val == 0
	d.mp[57].val = round1(target)
	ret := target - 1.5 + math.Sin(float64(t.Unix())/400)
	if !running {
		ret -= 3
	}
	d.mp[4].val = round1(ret)
	d.mp[3].val = round1(ret + map[bool]float64{true: 5.2, false: 0.4}[running])
	d.mp[2].val = round1(d.sp[83].val - 2 + 2*math.Sin(float64(t.Unix())/900))
	d.mp[5].val = round1(ret + 1)
	d.mp[6].val = round1(9 + 0.1*outdoor)
	d.mp[7].val = round1(d.mp[6].val - map[bool]float64{true: 3, false: 0}[running])
	d.mp[9].val = round1(d.mp[7].val - 1)
	d.mp[11].val = round1(d.mp[2].val - 3)
	d.mp[12].val = round1(d.mp[7].val - 5)
	d.mp[13].val = round1(d.mp[3].val + 3)
	d.mp[20].val = round1(4 + 0.1*d.mp[12].val)
	d.mp[21].val = round1(map[bool]float64{true: 14 + 0.25*d.mp[3].val, false: 8}[running])
	for _, id := range []int{22, 24, 30} {
		d.mp[id].val = boolf(running)
	}
	d.mp[22].val = boolf(mode != 0)
	d.mp[23].val = boolf(running)
	d.mp[25].val = boolf(running && h > 5 && h < 7)
	d.mp[56].val = float64(map[bool]int{true: 2, false: 0}[running])
	d.sp[10].val = float64(map[bool]int{true: 8, false: 0}[running])
	for _, base := range []int{63, 69} { // mixer circuit 1 and 2: flow, return, target
		d.mp[base].val = round1(target - 2)
		d.mp[base+1].val = round1(target - 6)
		d.mp[base+3].val = round1(target - 1)
	}
	hours := 0.0
	if !d.lastStep.IsZero() && t.After(d.lastStep) {
		hours = min(t.Sub(d.lastStep).Hours(), 1)
	}
	d.lastStep = t
	if running {
		in := 2.1 * hours // kWh electrical input
		d.mp[53].val += in
		d.mp[52].val += in * 4.3
		d.mp[75].val += in * 1.05
		d.mp[84].val += in * 4.4
		d.mp[79].val, d.mp[80].val, d.mp[81].val = 4.1, 4.0, 4.2
		d.mp[85].val = 1450
	} else {
		d.mp[79].val, d.mp[80].val, d.mp[81].val = 0.2, 0.2, 0.2
		d.mp[85].val = 0
	}
	d.mp[86].val, d.mp[87].val = d.mp[3].val, d.mp[4].val
	d.mp[88].val = round1(d.mp[3].val - d.mp[4].val)
	d.mp[92].val = round1(map[bool]float64{true: 4.2 + 0.05*outdoor, false: 0}[running])
	d.sp[11].val = float64(int(t.Sub(d.start).Seconds()) % 3600)
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }
func boolf(b bool) float64 {
	if b {
		return 1
	}
	return 0
}

func fmtPoint(tag string, id int, p *point) string {
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	s := fmt.Sprintf("~%s,NR=%d,ID=%d,NAME=%s,VAL=%s,MIN=%s,MAX=%s,US=%d,WR=%d,ACS=0", tag, id, id, p.name,
		protocol.FormatValue(round1(p.val)), protocol.FormatValue(p.min), protocol.FormatValue(p.max), b(p.used), b(p.wr))
	if tag == "MP" {
		s += fmt.Sprintf(",ERF=%d,RT1=%d,TRF=%d,TRT=%d,TRHV=%s,TRI=%d,TE=%s", b(p.erf), b(p.rt1), b(p.trf), p.trt, protocol.FormatValue(p.trhv), p.tri, protocol.FormatDeviceTime(p.te))
	}
	return s + ";"
}

// Handle processes one command and returns the replies. Exported for tests.
func (d *Device) Handle(cmd string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.step(time.Now())
	f := protocol.Split(cmd)
	if d.commands == nil {
		d.commands = map[string]int{}
	}
	d.commands[f.Tag]++
	ok := []string{"~OK;"}
	switch {
	case f.Tag == "LIN" || f.Tag == "LOUT":
		return ok
	case f.Tag == "RID":
		return []string{fmt.Sprintf("~RID,%d;", d.ID)}
	case f.Tag == "RM" || f.Tag == "RS":
		tag, pts, n := "MP", d.mp, d.nMP
		if f.Tag == "RS" {
			tag, pts, n = "SP", d.sp, d.nSP
		}
		out := []string{fmt.Sprintf("~SUM=%d;", n)}
		for i := 0; i < n; i++ {
			out = append(out, fmtPoint(tag, i, pts[i]))
		}
		return out
	case f.Tag == "MP" || f.Tag == "SP":
		pts := d.mp
		if f.Tag == "SP" {
			pts = d.sp
		}
		nr, _ := strconv.Atoi(f.KV["NR"])
		p, exists := pts[nr]
		if !exists {
			return []string{"~ERR;"}
		}
		if v, has := f.KV["VAL"]; has && f.Tag == "SP" {
			val, err := strconv.ParseFloat(v, 64)
			if err != nil || !p.wr {
				return []string{"~ERR;"}
			}
			if nr == protocol.FaultResetSP && val == 1 {
				d.mp[31].val = 0
				val = 0
			}
			p.val = math.Max(p.min, math.Min(p.max, val))
		}
		if f.Tag == "MP" {
			if _, has := f.KV["TRF"]; has {
				p.erf, p.trf, p.rt1 = f.KV["ERF"] == "1", f.KV["TRF"] == "1", f.KV["RT1"] == "1"
				p.trt, _ = strconv.Atoi(f.KV["TRT"])
				p.tri, _ = strconv.Atoi(f.KV["TRI"])
				p.trhv, _ = strconv.ParseFloat(f.KV["TRHV"], 64)
				if te, err := protocol.ParseDeviceTime(f.KV["TE"]); err == nil {
					p.te = te
				}
			}
			if f.KV["ORF"] == "1" {
				p.val, _ = strconv.ParseFloat(f.KV["ORV"], 64)
			}
		}
		return []string{fmtPoint(f.Tag, nr, p)}
	case f.Tag == "MR":
		var out []string
		for _, a := range f.Args {
			id, _ := strconv.Atoi(a)
			if p, ok := d.mp[id]; ok {
				// real controllers append a third field and a line break
				out = append(out, fmt.Sprintf("~MA,%d,%s,20;\n", id, protocol.FormatValue(round1(p.val))))
			}
		}
		return out
	case f.Tag == "CLK":
		if da, has := f.KV["DA"]; has {
			t, err := protocol.ParseClock("DA=" + da + ",TI=" + f.KV["TI"])
			if err != nil {
				return []string{"~ERR;"}
			}
			d.clockOffset = time.Until(t)
			return []string{strings.TrimSuffix(cmd, ";") + ";"}
		}
		t := time.Now().Add(d.clockOffset)
		return []string{protocol.SetClock(t)}
	case f.Tag == "TRS":
		return []string{fmt.Sprintf("~SUM=%d;", len(d.trend))}
	case f.Tag == "TRD":
		d.trend = nil
		return ok
	case f.Tag == "TF":
		d.trendPaused = len(f.Args) > 0 && f.Args[0] == "1"
		return []string{strings.TrimSuffix(cmd, ";") + ";"}
	case f.Tag == "TR":
		var out []string
		for _, a := range f.Args {
			id, _ := strconv.Atoi(a)
			if id >= 0 && id < len(d.trend) {
				r := d.trend[id]
				out = append(out, fmt.Sprintf("~TA,%d,%d,%s,%s,%d;", id, r.mp, protocol.FormatDeviceTime(r.t), protocol.FormatValue(r.val), r.reason))
			}
		}
		return out
	case f.Tag == "PRL":
		out := []string{fmt.Sprintf("~SUM=%d;", len(d.programs))}
		for _, p := range d.programs {
			out = append(out, fmtProgram(p))
		}
		return out
	case strings.HasPrefix(f.Tag, "PRI"):
		id, _ := strconv.Atoi(strings.TrimPrefix(f.Tag, "PRI"))
		if id < 0 || id >= len(d.programs) {
			return []string{"~ERR;"}
		}
		return []string{fmtProgram(d.programs[id])}
	case strings.HasPrefix(f.Tag, "PRD"):
		id, _ := strconv.Atoi(strings.TrimPrefix(f.Tag, "PRD"))
		if id < 0 || id >= len(d.programs) {
			return []string{"~ERR;"}
		}
		out := []string{fmtProgram(d.programs[id])}
		for _, ev := range d.programs[id].events {
			out = append(out, protocol.WriteTimeProgramEvent(ev))
		}
		return out
	case f.Tag == "PRE":
		pr, _ := strconv.Atoi(f.KV["PR"])
		day, _ := strconv.Atoi(f.KV["DAY"])
		evn, _ := strconv.Atoi(f.KV["EV"])
		if pr < 0 || pr >= len(d.programs) {
			return []string{"~ERR;"}
		}
		p := &d.programs[pr]
		for i := range p.events {
			e := &p.events[i]
			if e.Day != day || e.Event != evn {
				continue
			}
			if _, has := f.KV["ST"]; has {
				ne, err := protocol.ParseTimeProgramEvent(cmd)
				if err != nil {
					return []string{"~ERR;"}
				}
				*e = ne
			}
			return []string{protocol.WriteTimeProgramEvent(*e)}
		}
		return []string{"~ERR;"}
	}
	return []string{"~ERR;"}
}

func fmtProgram(p simProgram) string {
	us := 0
	if p.US {
		us = 1
	}
	return fmt.Sprintf("~PRI%d,NAME=%s,EAD=%d,NOS=%d,STE=%d,NOD=%d,ACS=0,US=%d;", p.ID, p.Name, p.EAD, p.NOS, p.STE, p.NOD, us)
}

// recordTrends appends trend records for points with interval recording.
func (d *Device) recordTrends(now time.Time) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.trendPaused {
		return
	}
	d.step(now)
	ids := make([]int, 0)
	for id, p := range d.mp {
		if p.trf {
			ids = append(ids, id)
		}
	}
	sort.Ints(ids)
	for _, id := range ids {
		p := d.mp[id]
		if p.te.Before(now) {
			continue
		}
		due := (p.trt == 0 && p.tri > 0 && now.Sub(p.lastTrend) >= time.Duration(p.tri)*time.Second) ||
			(p.trt == 1 && math.Abs(p.val-p.lastTrendVal) >= p.trhv && p.trhv > 0)
		if due {
			d.trend = append(d.trend, trendRec{mp: id, t: now, val: p.val, reason: 1 + p.trt})
			p.lastTrend, p.lastTrendVal = now, p.val
		}
	}
	if len(d.trend) > 20000 {
		d.trend = d.trend[len(d.trend)-20000:]
	}
}

// Serve accepts connections until the listener is closed.
func (d *Device) Serve(l net.Listener) error {
	go func() {
		for range time.Tick(10 * time.Second) {
			d.recordTrends(time.Now())
		}
	}()
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}
		go d.serveConn(conn)
	}
}

func (d *Device) serveConn(conn net.Conn) {
	defer conn.Close()
	d.log.Info("sim: client connected", "remote", conn.RemoteAddr())
	var dec frame.Decoder
	buf := make([]byte, 4096)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			d.log.Info("sim: client disconnected", "remote", conn.RemoteAddr())
			return
		}
		dec.Write(buf[:n])
		for {
			f, ok := dec.Next()
			if !ok {
				break
			}
			cmd := string(f.Payload)
			replies := d.Handle(cmd)
			d.log.Debug("sim", "cmd", cmd, "replies", len(replies))
			for _, r := range replies {
				time.Sleep(d.Latency)
				if _, err := conn.Write(frame.Encode([]byte(r), frame.AddrClient, frame.AddrHeatPump)); err != nil {
					return
				}
			}
		}
	}
}
