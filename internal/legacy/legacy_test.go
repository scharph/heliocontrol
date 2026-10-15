package legacy

import (
	"strings"
	"testing"
	"time"
)

// Shaped like DataSet.WriteXml output of homeControl's DS_TrendData.
const htd = `<?xml version="1.0" standalone="yes"?>
<DS_TrendData xmlns="http://tempuri.org/DS_TrendData.xsd">
  <TrendDataLog>
    <ID>0</ID>
    <MP_ID>15</MP_ID>
    <Timestamp>2026-10-14T23:48:18+02:00</Timestamp>
    <Value>54.5</Value>
    <Reason>1</Reason>
  </TrendDataLog>
  <TrendDataLog>
    <ID>1</ID>
    <MP_ID>58</MP_ID>
    <Timestamp>2026-10-14T23:48:22.0000000+02:00</Timestamp>
    <Value>50</Value>
    <Reason>1</Reason>
  </TrendDataLog>
</DS_TrendData>`

const hwo = `<?xml version="1.0" standalone="yes"?>
<DS_Heliograph xmlns="http://tempuri.org/DS_Heliograph.xsd">
  <RecordingPointsTemp><PointID>3</PointID><Name>Temp. Vorlauf</Name><Min>0</Min><Max>70</Max><CurrentVal>24.6</CurrentVal><Color>-65536</Color><Visible>true</Visible></RecordingPointsTemp>
  <RecordedValuesTemp><PointID>3</PointID><TimeInSec>0</TimeInSec><Value>24.6</Value></RecordedValuesTemp>
  <RecordedValuesTemp><PointID>3</PointID><TimeInSec>5</TimeInSec><Value>24.8</Value></RecordedValuesTemp>
  <RecorderProperties><Title>Test</Title><Start>2026-10-15T12:00:00+02:00</Start></RecorderProperties>
</DS_Heliograph>`

func TestReadTrend(t *testing.T) {
	recs, err := ReadTrend(strings.NewReader(htd))
	if err != nil || len(recs) != 2 {
		t.Fatal(recs, err)
	}
	if recs[1].MPID != 58 || recs[1].Value != 50 || recs[1].Time.Unix() != time.Date(2026, 10, 14, 21, 48, 22, 0, time.UTC).Unix() {
		t.Fatalf("%+v", recs[1])
	}
}

func TestReadRecording(t *testing.T) {
	rec, samples, err := ReadRecording(strings.NewReader(hwo), "file.hwo")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Name != "Test" || len(rec.Points) != 1 || rec.Points[0].Color != "#ff0000" || len(samples) != 2 {
		t.Fatalf("%+v %+v", rec, samples)
	}
	if samples[1].Time.Sub(rec.Started) != 5*time.Second || rec.Ended.Sub(rec.Started) != 5*time.Second {
		t.Fatal("relative times")
	}
}
