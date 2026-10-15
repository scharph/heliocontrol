package protocol

import (
	"testing"
	"time"
)

func TestIsReadCommand(t *testing.T) {
	reads := []string{CmdLogin, CmdLogout, CmdReadID, CmdCountMP, CmdCountSP, CmdClock, "~CLK", CmdTrendCount, CmdTimeProgramList,
		ReadMP(3), ReadSP(13), ReadValues([]int{0, 3, 4}), ReadTrend([]int{1, 2}), ReadTimeProgram(2), ReadTimeProgramData(1),
		ReadTimeProgramEvent(1, 2, 3)}
	for _, c := range reads {
		if !IsReadCommand(c) {
			t.Errorf("%q should be a read", c)
		}
	}
	writes := []string{WriteSP(13, 1), WriteSP(69, 21.5), OverrideMP(3, 1), AutoMP(3), SetClock(time.Now()),
		CmdTrendDelete, CmdTrendStop, CmdTrendRestart, WriteTrendConfig(3, TrendConfig{}),
		WriteTimeProgramEvent(TimeProgramEvent{Program: 1, Begin: 0, End: 60}), "~SID,5;", "~STP;", "~ALA;", "~ALD;", "~CSB;",
		"~MP,NR=3;~SP,NR=13,VAL=1;", "~SP,NR=13,VAL=1", "~btld;", "~erase;", ModemConnect, ""}
	for _, c := range writes {
		if IsReadCommand(c) {
			t.Errorf("%q must not be treated as a read", c)
		}
	}
}
