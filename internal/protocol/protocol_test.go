package protocol

import (
	"testing"
	"time"
)

func TestParsePoint(t *testing.T) {
	p, err := ParsePoint("~MP,NR=3,ID=3,NAME=Vorlauf,VAL=34.5,MIN=-20,MAX=80.5,US=1,WR=0,TRF=1,TRT=1,TRHV=0.5,TRI=60,TE=31.12.99-23:59:00,ERF=1,RT1=0,ACS=2;")
	if err != nil {
		t.Fatal(err)
	}
	want := Point{Kind: MP, ID: 3, NR: 3, Name: "Vorlauf", Val: 34.5, Min: -20, Max: 80.5, US: true, TRF: true, TRT: 1, TRHV: 0.5, TRI: 60, ERF: true, ACS: 2,
		TE: time.Date(2099, 12, 31, 23, 59, 0, 0, time.Local)}
	if p != want {
		t.Fatalf("got %+v\nwant %+v", p, want)
	}
	sp, err := ParsePoint("~SP,NR=13,VAL=1;")
	if err != nil || sp.Kind != SP || sp.ID != 13 || sp.Val != 1 {
		t.Fatalf("%+v %v", sp, err)
	}
	if _, err := ParsePoint("~OK;"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseValue(t *testing.T) {
	for in, want := range map[string]Value{
		"~MA,3,-4.5;":          {3, -4.5},
		"~MA,NR=12,VAL=21.25;": {12, 21.25},
	} {
		got, err := ParseValue(in)
		if err != nil || got != want {
			t.Fatalf("%q: %+v %v", in, got, err)
		}
	}
}

func TestParseTrendRecord(t *testing.T) {
	r, err := ParseTrendRecord("~TA,17,3,05.01.26-07:30:15,-3.5,2;")
	if err != nil {
		t.Fatal(err)
	}
	want := TrendRecord{ID: 17, MPID: 3, Time: time.Date(2026, 1, 5, 7, 30, 15, 0, time.Local), Value: -3.5, Reason: 2}
	if r != want {
		t.Fatalf("%+v", r)
	}
}

func TestClock(t *testing.T) {
	tm := time.Date(2026, 10, 18, 9, 5, 7, 0, time.Local) // a Sunday
	cmd := SetClock(tm)
	if cmd != "~CLK,DA=18.10.26,TI=09:05:07,WD=7;" {
		t.Fatal(cmd)
	}
	got, err := ParseClock(cmd)
	if err != nil || !got.Equal(tm) {
		t.Fatalf("%v %v", got, err)
	}
}

func TestTimePrograms(t *testing.T) {
	tp, err := ParseTimeProgram("~PRI2,NAME=Warmwasser,EAD=6,NOS=2,STE=15,NOD=7,ACS=0,US=1;")
	if err != nil || tp != (TimeProgram{ID: 2, Name: "Warmwasser", EAD: 6, NOS: 2, STE: 15, NOD: 7, US: true}) {
		t.Fatalf("%+v %v", tp, err)
	}
	ev, err := ParseTimeProgramEvent("~PRE,PR=2,DAY=6,EV=5,ST=1,BEG=22:15,END=24:00;")
	if err != nil {
		t.Fatal(err)
	}
	if ev.Begin != 22*60+15 || ev.End != 1440 {
		t.Fatalf("%+v", ev)
	}
	if got := WriteTimeProgramEvent(ev); got != "~PRE,PR=2,DAY=6,EV=5,ST=1,BEG=22:15,END=24:00;" {
		t.Fatal(got)
	}
}

func TestCommands(t *testing.T) {
	cases := map[string]string{
		WriteSP(69, 21.5):        "~SP,NR=69,VAL=21.5;",
		WriteSP(13, 1):           "~SP,NR=13,VAL=1;",
		ReadValues([]int{0, 3}):  "~MR,0,3;",
		ReadTrend([]int{10, 11}): "~TR,10,11;",
		OverrideMP(5, 2):         "~MP,NR=5,ORF=1,ORV=2;",
		WriteTrendConfig(3, TrendConfig{Enabled: true, Type: 0, Interval: 300, Hysteresis: 0.5, End: time.Date(2027, 3, 1, 14, 0, 0, 0, time.Local)}): "~MP,NR=3,ERF=0,TRF=1,TRT=0,TRHV=0.5,TRI=300,TE=01.03.27-14:00:00,RT1=0;",
	}
	for got, want := range cases {
		if got != want {
			t.Errorf("got %q want %q", got, want)
		}
	}
}
