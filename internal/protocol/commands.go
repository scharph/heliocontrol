package protocol

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Commands understood by the web control. All are terminated with ';'.
const (
	CmdLogin           = "~LIN;"
	CmdLogout          = "~LOUT;"
	CmdReadID          = "~RID;"
	CmdCountMP         = "~RM;"  // reply: count, then one ~MP reply per point
	CmdCountSP         = "~RS;"  // reply: count, then one ~SP reply per point
	CmdClock           = "~CLK;" // reply contains DA=dd.MM.yy,TI=HH:mm:ss
	CmdTrendCount      = "~TRS;"
	CmdTrendDelete     = "~TRD;"
	CmdTrendStop       = "~TF,1;"
	CmdTrendRestart    = "~TF,0;"
	CmdTimeProgramList = "~PRL;" // reply: count, then one ~PRI reply per program

	// ModemConnect is sent raw (unframed) when the login gets no answer: the
	// controller may sit behind an analog/GSM modem that expects a CONNECT line.
	ModemConnect = "\r\nCONNECT 19200\r\n"

	// FaultResetSP is the setup point that acknowledges an active fault ("Entstören").
	FaultResetSP = 14
	// PlantTypeSP holds the installation bit mask ("WNA") that selects the UI tabs.
	PlantTypeSP = 9
)

func ReadMP(nr int) string { return "~MP,NR=" + strconv.Itoa(nr) + ";" }
func ReadSP(nr int) string { return "~SP,NR=" + strconv.Itoa(nr) + ";" }

func WriteSP(nr int, v float64) string {
	return fmt.Sprintf("~SP,NR=%d,VAL=%s;", nr, FormatValue(v))
}

// OverrideMP forces a measuring point to a manual value (ORF=1); AutoMP releases it.
func OverrideMP(nr int, v float64) string {
	return fmt.Sprintf("~MP,NR=%d,ORF=1,ORV=%s;", nr, FormatValue(v))
}
func AutoMP(nr int) string { return fmt.Sprintf("~MP,NR=%d,ORF=0;", nr) }

func joinInts(prefix string, ids []int) string {
	var b strings.Builder
	b.WriteString(prefix)
	for _, id := range ids {
		b.WriteByte(',')
		b.WriteString(strconv.Itoa(id))
	}
	b.WriteByte(';')
	return b.String()
}

// ReadValues asks for the current values of several measuring points at once;
// the device answers with one "~MA" reply per point.
func ReadValues(ids []int) string { return joinInts("~MR", ids) }

// ReadTrend requests trend buffer records; one reply per record.
func ReadTrend(ids []int) string { return joinInts("~TR", ids) }

// TrendConfig is the editable trend/alarm configuration of a measuring point.
type TrendConfig struct {
	Alarm      bool      `json:"alarm"`
	CallRemote bool      `json:"callRemote"`
	Enabled    bool      `json:"enabled"`
	Type       int       `json:"type"` // 0 = interval, 1 = hysteresis
	Hysteresis float64   `json:"hysteresis"`
	Interval   int       `json:"interval"` // seconds
	End        time.Time `json:"end"`
}

// WriteTrendConfig builds the ~MP command that stores a trend configuration.
// homeControl formatted the end time with a 12-hour clock ("hh"), which looks
// like a bug since the device reports 24-hour times; we send 24-hour time.
func WriteTrendConfig(nr int, c TrendConfig) string {
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	return fmt.Sprintf("~MP,NR=%d,ERF=%d,TRF=%d,TRT=%d,TRHV=%s,TRI=%d,TE=%s,RT1=%d;",
		nr, b(c.Alarm), b(c.Enabled), c.Type, FormatValue(c.Hysteresis), c.Interval, FormatDeviceTime(c.End), b(c.CallRemote))
}

// SetClock sets the controller clock. Weekday: Monday = 1 ... Sunday = 7.
func SetClock(t time.Time) string {
	wd := int(t.Weekday())
	if wd == 0 {
		wd = 7
	}
	return fmt.Sprintf("~CLK,DA=%s,TI=%s,WD=%d;", t.Format("02.01.06"), t.Format("15:04:05"), wd)
}

func ReadTimeProgram(id int) string     { return fmt.Sprintf("~PRI%d;", id) }
func ReadTimeProgramData(id int) string { return fmt.Sprintf("~PRD%d;", id) }

func ReadTimeProgramEvent(pr, day, ev int) string {
	return fmt.Sprintf("~PRE,PR=%d,DAY=%d,EV=%d;", pr, day, ev)
}

func WriteTimeProgramEvent(e TimeProgramEvent) string {
	return fmt.Sprintf("~PRE,PR=%d,DAY=%d,EV=%d,ST=%d,BEG=%s,END=%s;", e.Program, e.Day, e.Event, e.State, e.Begin, e.End)
}
