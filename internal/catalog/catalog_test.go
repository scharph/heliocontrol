package catalog

import (
	"slices"
	"testing"
)

func ids(ps []Panel) []string {
	var out []string
	for _, p := range ps {
		out = append(out, p.ID)
	}
	return out
}

func TestResolveRealInstallation(t *testing.T) {
	// SP 9 = 532 on the real controller this was tested with:
	// 0x004 ground source, 0x010 (no buffer), 0x200 hot water with priority valve.
	got := Resolve(Installation{PlantType: 532})
	want := []string{"main", "source_ground", "plant"}
	if !slices.Equal(ids(got), want) {
		t.Fatalf("panels %v, want %v", ids(got), want)
	}
	if v := PlantVariant(532); v != 5 {
		t.Fatalf("variant %d", v)
	}
	mps, sps := Points(got)
	for _, id := range []int{0, 2, 3, 4, 25, 31} {
		if !slices.Contains(mps, id) {
			t.Errorf("MP %d missing", id)
		}
	}
	for _, id := range []int{9, 13, 69, 83, 85, 199, 222, 263} {
		if !slices.Contains(sps, id) {
			t.Errorf("SP %d missing", id)
		}
	}
}

func TestResolveFull(t *testing.T) {
	got := Resolve(Installation{PlantType: 0x2000 | 0x01 | 0x20 | 0x100, Mixer2: true, Energy: true})
	want := []string{"main", "mixer1", "mixer2", "source_brine", "plant", "energy"}
	if !slices.Equal(ids(got), want) {
		t.Fatalf("panels %v, want %v", ids(got), want)
	}
	if got[0].Items[0].HA.DeviceClass != "temperature" {
		t.Fatal("HA hints missing")
	}
	if len(Resolve(Installation{})) != 1 {
		t.Fatal("unknown installation should only show the main page")
	}
}

func TestHeatCurve(t *testing.T) {
	// limit 18 °C -> 24, 0 °C -> 28, -15 °C -> 33 (values of the real controller)
	for _, c := range []struct{ at, want float64 }{{20, 24}, {18, 24}, {9, 26}, {0, 28}, {-7.5, 30.5}, {-15, 33}, {-20, 33}} {
		if got := HeatCurveTarget(c.at, 18, 24, 28, 33, 0); got != c.want {
			t.Errorf("at %v: %v, want %v", c.at, got, c.want)
		}
	}
	if got := HeatCurveTarget(0, 18, 24, 28, 33, -2); got != 26 {
		t.Errorf("reduction shift: %v", got)
	}
}
