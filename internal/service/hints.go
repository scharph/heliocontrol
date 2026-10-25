package service

import (
	"fmt"

	"heliocontrol/internal/catalog"
)

// Community experience from other Heliotherm owners. These are forum reports,
// not manufacturer advice, and the UI labels them that way. Parameters behind
// the service menu should be changed by the installer.
const (
	srcSNTM1   = "https://www.energiesparhaus.at/forum-erfahrungen-mit-heliotherm-sntm-s-3-10-wp/81145_1"
	srcSNTM3   = "https://www.energiesparhaus.at/forum-erfahrungen-mit-heliotherm-sntm-s-3-10-wp/81145_3"
	srcHP08E   = "https://www.energiesparhaus.at/forum/39774"
	srcModular = "https://www.energiesparhaus.at/forum-modulare-waermepumpe-heliotherm-einstellungen-heizkurve/81672"
)

var (
	tipHysteresis = Tip{Source: srcSNTM1, Text: catalog.Text{
		DE: "Hysterese von 8 K auf 4 K verkleinert, den Pufferfühler weiter unten im Speicher gesetzt und die Stützheizung ab 10 °C abgeschaltet: danach längere Laufzeiten mit Modulation.",
		EN: "Reduced the hysteresis from 8 K to 4 K, moved the buffer sensor lower in the tank and disabled the backup heater above 10 °C: longer, modulating runs afterwards."}}
	tipCompressorCurve = Tip{Source: srcSNTM3, Text: catalog.Text{
		DE: "Der Installateur hat die Verdichterdrehzahl über die Außentemperatur gesenkt (15 % bei 18 °C, 30 % bei 0 °C, 50 % bei −15 °C, Servicemenü): statt 8–10 Starts pro Tag deutlich weniger, Arbeitszahl ≈ 6,5.",
		EN: "The installer lowered the compressor speed over outdoor temperature (15 % at 18 °C, 30 % at 0 °C, 50 % at −15 °C, service menu): far fewer than the previous 8–10 starts per day, COP ≈ 6.5."}}
	tipBufferSize = Tip{Source: srcSNTM3, Text: catalog.Text{
		DE: "Ist die kleinste Verdichterleistung größer als der Bedarf, lädt die Wärmepumpe den Puffer sofort voll und schaltet ab. Das lässt sich am Regler kaum beheben, nur hydraulisch (Puffer, Durchfluss).",
		EN: "If the smallest compressor output exceeds the demand, the heat pump fills the buffer at once and stops. Controller settings barely help; only the hydraulics do (buffer, flow)."}}
	tipReturnShortCircuit = Tip{Source: srcSNTM3, Text: catalog.Text{
		DE: "Die Regelung startet, wenn der Rücklauf 3 K unter Soll fällt. Ein Kurzschluss über Weiche oder Puffer verfälscht den Rücklauf, den die Wärmepumpe sieht.",
		EN: "The controller starts when the return falls 3 K below target. A short circuit through a separator or buffer distorts the return the heat pump sees."}}
	tipDHWSeparate = Tip{Source: srcHP08E, Text: catalog.Text{
		DE: "Warmwasservorrang unterbricht das Heizen, danach erreichte das Haus die Solltemperatur nicht. Mit Warmwasser außerhalb der Heizzeiten stellte sich die Raumtemperatur problemlos ein.",
		EN: "Hot water priority interrupted heating and the house then missed its target. With hot water outside the heating times, the room temperature settled without trouble."}}
	tipPVWindow = Tip{Source: srcModular, Text: catalog.Text{
		DE: "Heizen tagsüber (z. B. 05–17 Uhr) mit +3 K Überhöhung für PV-Strom, nachts 6 K Absenkung, damit die Wärmepumpe nachts nicht startet. Konstanter Sollwert führte dagegen zu häufigem Takten.",
		EN: "Heating during the day (e.g. 05:00–17:00) with +3 K boost on PV power and a 6 K night setback so the heat pump does not start at night. A constant setpoint caused frequent cycling instead."}}
)

// hintSettings are current controller settings the hints refer to; nil if unknown.
type hintSettings struct {
	HeatingLimit *float64 // SP 76
	CurveAtLimit *float64 // SP 80
}

// week aggregates the days with enough data.
type week struct {
	n, starts        int
	runtime, outdoor float64
	flow, spread     float64
	nFlow, nSpread   int
	nReq, interrupts int
	aboveLimitDays   int
	heatOut, heatIn  float64
	nEnergy          int
	outdoorEnergy    float64
}

func aggregate(days []DayInsight, set hintSettings) week {
	var w week
	for _, d := range days {
		if d.Coverage < 0.5 || d.Starts == nil || d.RuntimeHours == nil {
			continue
		}
		w.n++
		w.starts += *d.Starts
		w.runtime += *d.RuntimeHours
		if d.Outdoor != nil {
			w.outdoor += *d.Outdoor
		}
		if d.FlowRunning != nil {
			w.flow += *d.FlowRunning
			w.nFlow++
		}
		if d.Spread != nil {
			w.spread += *d.Spread
			w.nSpread++
		}
		if d.DHWInterrupts != nil {
			w.nReq++
			w.interrupts += *d.DHWInterrupts
		}
		if set.HeatingLimit != nil && d.Outdoor != nil && d.HeatingHours != nil &&
			*d.Outdoor > *set.HeatingLimit && *d.HeatingHours >= 0.5 {
			w.aboveLimitDays++
		}
		if d.HeatOutKWh != nil && d.HeatInKWh != nil {
			w.nEnergy++
			w.heatOut += *d.HeatOutKWh
			w.heatIn += *d.HeatInKWh
			if d.Outdoor != nil {
				w.outdoorEnergy += *d.Outdoor
			}
		}
	}
	return w
}

// cop is the week's heating SPF, if enough energy was metered.
func (w week) cop() (float64, bool) {
	if w.heatIn < 5 {
		return 0, false
	}
	return w.heatOut / w.heatIn, true
}

// hints applies simple rules to the last 7 days with enough data, and
// compares the heating SPF with the 7 days before.
func hints(days []DayInsight, set hintSettings) []Hint {
	last := days[max(0, len(days)-7):]
	w := aggregate(last, set)
	out := []Hint{}
	if w.n == 0 {
		return out
	}
	perDay := float64(w.starts) / float64(w.n)
	avgOutdoor := w.outdoor / float64(w.n)
	avgFlow, haveFlow := 0.0, w.nFlow > 0
	if haveFlow {
		avgFlow = w.flow / float64(w.nFlow)
	}
	settings := catalog.Text{}
	if set.HeatingLimit != nil && set.CurveAtLimit != nil {
		settings = catalog.Text{
			DE: fmt.Sprintf(" Aktuell: Heizgrenze %.0f °C, Rücklauf Soll bei Heizgrenze %.1f °C.", *set.HeatingLimit, *set.CurveAtLimit),
			EN: fmt.Sprintf(" Current: heating limit %.0f °C, return target at heating limit %.1f °C.", *set.HeatingLimit, *set.CurveAtLimit)}
	}

	if w.starts > 0 {
		cycle := w.runtime * 60 / float64(w.starts)
		if perDay >= 12 && cycle < 15 {
			out = append(out, Hint{Level: "warn",
				Title: catalog.Text{DE: "Verdichter taktet häufig", EN: "Compressor cycles often"},
				Detail: catalog.Text{
					DE: fmt.Sprintf("Ø %.0f Starts pro Tag mit Ø %.0f min Laufzeit. Kurze Zyklen kosten Effizienz und verschleißen den Verdichter. Häufige Ursachen: Heizkurve in der Übergangszeit zu hoch, zu kleine Hysterese, zu wenig Volumenstrom oder fehlender Puffer.", perDay, cycle) + settings.DE,
					EN: fmt.Sprintf("Avg. %.0f starts per day with avg. %.0f min runtime. Short cycles cost efficiency and wear the compressor. Common causes: heating curve too high in mild weather, small hysteresis, low flow rate or no buffer tank.", perDay, cycle) + settings.EN},
				Tips: []Tip{tipHysteresis, tipCompressorCurve, tipPVWindow}})
		}
		// many short cycles although the curve is already low: the heat pump
		// delivers more than the house takes, which settings cannot fix
		if perDay >= 8 && cycle < 20 && avgOutdoor >= 7 && haveFlow && avgFlow < 35 {
			out = append(out, Hint{Level: "info",
				Title: catalog.Text{DE: "Leistung über dem Bedarf", EN: "Output above demand"},
				Detail: catalog.Text{
					DE: fmt.Sprintf("Bei Ø %.0f °C außen und niedrigem Vorlauf (Ø %.0f °C) startet der Verdichter Ø %.0f-mal pro Tag für Ø %.0f min. Die kleinste Leistung der Wärmepumpe liegt offenbar über dem Bedarf. Das ist eher eine Frage der Hydraulik (Puffer, Durchfluss, offene Heizkreise) als der Heizkurve; mit dem Fachbetrieb klären.", avgOutdoor, avgFlow, perDay, cycle),
					EN: fmt.Sprintf("At avg. %.0f °C outdoors and a low flow temperature (avg. %.0f °C) the compressor starts %.0f times a day for avg. %.0f min. The heat pump's minimum output apparently exceeds the demand. That is a matter of hydraulics (buffer, flow, open heating circuits) more than of the heating curve; check with the installer.", avgOutdoor, avgFlow, perDay, cycle)},
				Tips: []Tip{tipBufferSize, tipCompressorCurve}})
		}
	}
	if haveFlow && avgOutdoor >= 5 && avgFlow >= 40 {
		out = append(out, Hint{Level: "info",
			Title: catalog.Text{DE: "Vorlauf hoch für die Außentemperatur", EN: "Flow temperature high for the outdoor temperature"},
			Detail: catalog.Text{
				DE: fmt.Sprintf("Bei Ø %.0f °C außen läuft der Vorlauf mit Ø %.0f °C. Jedes Grad weniger bringt rund 2,5 %% Effizienz. Die Heizkurve schrittweise (1–2 K, über mehrere Tage) senken und die Räume beobachten.", avgOutdoor, avgFlow) + settings.DE,
				EN: fmt.Sprintf("At avg. %.0f °C outdoors the flow runs at avg. %.0f °C. Each degree less gains about 2.5 %% efficiency. Lower the heating curve step by step (1–2 K over several days) while watching room temperatures.", avgOutdoor, avgFlow) + settings.EN}})
	}
	if w.nSpread > 0 {
		sp := w.spread / float64(w.nSpread)
		switch {
		case sp > 8:
			out = append(out, Hint{Level: "info",
				Title: catalog.Text{DE: "Große Spreizung", EN: "Large temperature spread"},
				Detail: catalog.Text{
					DE: fmt.Sprintf("Vorlauf und Rücklauf liegen im Betrieb Ø %.1f K auseinander (üblich: 3–7 K). Das deutet auf zu wenig Volumenstrom hin (Pumpenstufe, Ventile, hydraulischer Abgleich).", sp),
					EN: fmt.Sprintf("Flow and return differ by avg. %.1f K while running (typical: 3–7 K). This points to a low flow rate (pump setting, valves, hydraulic balancing).", sp)}})
		case sp < 2:
			out = append(out, Hint{Level: "info",
				Title: catalog.Text{DE: "Sehr kleine Spreizung", EN: "Very small temperature spread"},
				Detail: catalog.Text{
					DE: fmt.Sprintf("Vorlauf und Rücklauf liegen im Betrieb nur Ø %.1f K auseinander (üblich: 3–7 K). Das Heizwasser kommt fast so warm zurück, wie es hinausgeht: Hinweis auf einen hydraulischen Kurzschluss (Weiche, Puffer, Überströmventil) oder einen zu hohen Volumenstrom.", sp),
					EN: fmt.Sprintf("Flow and return differ by only avg. %.1f K while running (typical: 3–7 K). The water comes back almost as warm as it left: a sign of a hydraulic short circuit (separator, buffer, bypass valve) or too high a flow rate.", sp)},
				Tips: []Tip{tipReturnShortCircuit}})
		}
	}
	if w.nReq > 0 {
		if per := float64(w.interrupts) / float64(w.nReq); per >= 1 {
			out = append(out, Hint{Level: "info",
				Title: catalog.Text{DE: "Warmwasser unterbricht das Heizen", EN: "Hot water interrupts heating"},
				Detail: catalog.Text{
					DE: fmt.Sprintf("Ø %.1f-mal pro Tag wurde eine laufende Heizanforderung für Warmwasser unterbrochen. Jede Unterbrechung ist ein zusätzlicher Zyklus mit höherer Temperatur. Das Warmwasser-Zeitprogramm auf ein Fenster außerhalb der Heizzeiten legen, am besten mittags (wärmere Luft, PV-Strom).", per),
					EN: fmt.Sprintf("A running heating request was interrupted for hot water avg. %.1f times a day. Each interruption is an extra cycle at a higher temperature. Move the hot water time program to a window outside the heating times, ideally midday (warmer air, PV power).", per)},
				Tips: []Tip{tipDHWSeparate, tipPVWindow}})
		}
	}
	if w.aboveLimitDays >= 2 {
		out = append(out, Hint{Level: "info",
			Title: catalog.Text{DE: "Heizbetrieb über der Heizgrenze", EN: "Heating above the heating limit"},
			Detail: catalog.Text{
				DE: fmt.Sprintf("An %d Tagen lag die Außentemperatur im Mittel über der Heizgrenze (%.0f °C), trotzdem lief der Verdichter mindestens 30 min zum Heizen. Die Heizgrenze gilt für den Momentanwert; an Tagen mit kalter Nacht ist das normal. Wiederholt es sich, die Heizgrenze um 1 K senken.", w.aboveLimitDays, *set.HeatingLimit),
				EN: fmt.Sprintf("On %d days the mean outdoor temperature was above the heating limit (%.0f °C), yet the compressor ran at least 30 min for heating. The limit applies to the current value, so cold nights explain some of it. If it keeps happening, lower the heating limit by 1 K.", w.aboveLimitDays, *set.HeatingLimit)}})
	}
	if len(days) >= 14 {
		prev := aggregate(days[len(days)-14:len(days)-7], set)
		c1, ok1 := prev.cop()
		c2, ok2 := w.cop()
		if ok1 && ok2 && prev.nEnergy > 0 && w.nEnergy > 0 {
			if change := (c2 - c1) / c1; change >= 0.1 || change <= -0.1 {
				o1, o2 := prev.outdoorEnergy/float64(prev.nEnergy), w.outdoorEnergy/float64(w.nEnergy)
				up := change > 0
				title := map[bool]catalog.Text{
					true:  {DE: "Arbeitszahl gestiegen", EN: "SPF improved"},
					false: {DE: "Arbeitszahl gesunken", EN: "SPF dropped"},
				}[up]
				out = append(out, Hint{Level: "info", Title: title,
					Detail: catalog.Text{
						DE: fmt.Sprintf("Heizen: Arbeitszahl %.2f in den letzten 7 Tagen gegenüber %.2f in den 7 Tagen davor (%+.0f %%). Ø außen %.1f °C gegenüber %.1f °C; wärmeres Wetter allein hebt die Arbeitszahl. Ein Vergleich hilft vor allem nach einer Änderung an Heizkurve oder Hysterese.", c2, c1, change*100, o2, o1),
						EN: fmt.Sprintf("Heating: SPF %.2f in the last 7 days against %.2f in the 7 days before (%+.0f %%). Avg. outdoor %.1f °C against %.1f °C; warmer weather alone raises the SPF. The comparison helps most after a change to the heating curve or hysteresis.", c2, c1, change*100, o2, o1)}})
			}
		}
	}
	return out
}
