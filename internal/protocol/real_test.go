package protocol

import (
	"testing"
	"time"
)

// Replies captured from a real web control (firmware 2.07D) with
// `heliocli probe`; the controller ID is anonymized.
func TestRealDeviceReplies(t *testing.T) {
	mp, err := ParsePoint("~MP,NR=0,ID=0,NAME=Temp. Aussen,LEN=4,TP=1,BIT=1,VAL=15.3,MAX=40.0,MIN=-20.0,ERF=0,ORV=5.0,ORF=0,TRF=0,TRT=1,TRHV=1.0,TRI=100,TE=04.11.13-11:59:00,OFFV=0.0,RT1=0,RTL=0,WR=1,US=1;")
	if err != nil {
		t.Fatal(err)
	}
	if mp.Name != "Temp. Aussen" || mp.Val != 15.3 || mp.Min != -20 || mp.Max != 40 || !mp.US || !mp.WR || mp.TRT != 1 || mp.TRI != 100 || mp.ORV != 5 ||
		!mp.TE.Equal(time.Date(2013, 11, 4, 11, 59, 0, 0, time.Local)) {
		t.Fatalf("%+v", mp)
	}
	sp, err := ParsePoint("~SP,NR=9,ID=9,NAME=2.07D ,LEN=4,TP=0,BIT=0,VAL=532,MAX=0,MIN=0,WR=0,US=1;")
	if err != nil || sp.Val != 532 || sp.Name != "2.07D " || sp.WR {
		t.Fatalf("%+v %v", sp, err)
	}
	if n, err := ParseCount("~SUM=1256;"); err != nil || n != 1256 {
		t.Fatal(n, err)
	}
	if n, err := ParseCount("~RID,123456;"); err != nil || n != 123456 {
		t.Fatal(n, err)
	}
	if v, err := ParseValue("~MA,57,24.7,20;\n"); err != nil || v != (Value{57, 24.7}) {
		t.Fatal(v, err)
	}
	tr, err := ParseTrendRecord("~TA,1,58,14.10.26-23:48:22,50.0,1;\n")
	if err != nil || tr != (TrendRecord{ID: 1, MPID: 58, Time: time.Date(2026, 10, 14, 23, 48, 22, 0, time.Local), Value: 50, Reason: 1}) {
		t.Fatal(tr, err)
	}
	clk, err := ParseClock("~CLK,DA=15.10.26,TI=12:55:12,WD=4;")
	if err != nil || !clk.Equal(time.Date(2026, 10, 15, 12, 55, 12, 0, time.Local)) {
		t.Fatal(clk, err)
	}
	tp, err := ParseTimeProgram("~PRI3,NAME=Mischer 1,EAD=7,NOS=3,STE=15,NOD=7,ACS=0,US=0;")
	if err != nil || tp != (TimeProgram{ID: 3, Name: "Mischer 1", EAD: 7, NOS: 3, STE: 15, NOD: 7}) {
		t.Fatal(tp, err)
	}
	ev, err := ParseTimeProgramEvent("~PRE,PR=0,DAY=0,EV=6,ST=0,BEG=22:00,END=24:00;\n")
	if err != nil || ev != (TimeProgramEvent{Program: 0, Day: 0, Event: 6, State: 0, Begin: 22 * 60, End: 1440}) {
		t.Fatal(ev, err)
	}
}
