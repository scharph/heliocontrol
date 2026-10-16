package haentity

import (
	"testing"

	"heliocontrol/internal/catalog"
)

func find(t *testing.T, es []Entity, key string) Entity {
	t.Helper()
	for _, e := range es {
		if e.Key == key {
			return e
		}
	}
	t.Fatalf("entity %q missing", key)
	return Entity{}
}

func TestBuild(t *testing.T) {
	panels := catalog.Resolve(catalog.Installation{PlantType: 0x2000 | 0x01 | 0x20 | 0x100, Energy: true})
	es := Build(panels, false)

	mode := find(t, es, "mode")
	if mode.Component != "select" || len(mode.Selectable) != len(mode.Options)-1 {
		t.Fatalf("mode: %+v", mode)
	}
	if n := find(t, es, "mixer1_mode").Name.DE; n != "Mischerkreis 1 Betriebsart" {
		t.Fatalf("mixer name %q", n)
	}
	if e := find(t, es, "boost"); e.DeviceClass != "" || e.Unit != "K" || e.Category != "config" {
		t.Fatalf("K offsets are not temperatures: %+v", e)
	}
	if e := find(t, es, "outdoor"); e.DeviceClass != "temperature" || e.StateClass != "measurement" {
		t.Fatalf("outdoor: %+v", e)
	}
	find(t, es, "reset_fault")
	seen := map[string]bool{}
	for _, e := range es {
		if seen[e.Key] {
			t.Fatalf("duplicate key %s", e.Key)
		}
		seen[e.Key] = true
	}

	ro := Build(panels, true)
	if e := find(t, ro, "mode"); e.Component != "sensor" || e.DeviceClass != "enum" {
		t.Fatalf("read-only mode: %+v", e)
	}
	if e := find(t, ro, "room_target"); e.Component != "sensor" || e.Category != "" {
		t.Fatalf("read-only number: %+v", e)
	}
	for _, e := range ro {
		if e.Action == "reset_fault" || e.Action == "sync_clock" {
			t.Fatal("write buttons in read-only mode")
		}
	}
}

func TestState(t *testing.T) {
	es := Build(catalog.Resolve(catalog.Installation{PlantType: 0x01, Energy: true}), false)
	mp := map[int]float64{0: 7.5, 52: 430, 53: 100, 30: 1}
	sp := map[int]float64{13: 3}
	if st, ok := State(find(t, es, "outdoor"), mp, sp, "", "en"); !ok || st != 7.5 {
		t.Fatal(st)
	}
	if st, _ := State(find(t, es, "mode"), mp, sp, "", "de"); st != "Sommer" {
		t.Fatal(st)
	}
	if st, _ := State(find(t, es, "cop_heating"), mp, sp, "", "en"); st != 4.3 {
		t.Fatal(st)
	}
	if st, _ := State(find(t, es, "compressor"), mp, sp, "", "en"); st != true || FormatState(st) != "ON" {
		t.Fatal(st)
	}
	if _, ok := State(find(t, es, "flow"), mp, sp, "", "en"); ok {
		t.Fatal("unknown value reported")
	}
	if v, err := OptionValue(find(t, es, "mode"), "Party"); err != nil || v != 7 {
		t.Fatal(v, err)
	}
	if _, err := OptionValue(find(t, es, "mode"), "Hauptschalter"); err == nil {
		t.Fatal("main switch must not be selectable")
	}
}
