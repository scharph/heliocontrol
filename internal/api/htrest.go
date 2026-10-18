package api

import (
	"maps"
	"net/http"
	"slices"

	"heliocontrol/internal/protocol"
	"heliocontrol/internal/service"
)

// HtREST compatibility (github.com/dstrigl/HtREST): the /api/v1/param and
// /api/v1/fastquery endpoints, so Loxone configurations built for HtREST work
// unchanged. Values come from the poll cache and are always numbers (Loxone
// cannot parse booleans). See docs/adr/0001-htrest-parameter-names.md.

type htParam struct {
	kind protocol.Kind
	id   int
}

// htParams is htheatpump's default htparams.csv; the names must stay exactly
// as they are, Loxone configurations match on them.
var htParams = map[string]htParam{
	"Temp. Aussen":                   {protocol.MP, 0},
	"Temp. Aussen verzoegert":        {protocol.MP, 1},
	"Temp. Brauchwasser":             {protocol.MP, 2},
	"Temp. Vorlauf":                  {protocol.MP, 3},
	"Temp. Ruecklauf":                {protocol.MP, 4},
	"Temp. EQ_Eintritt":              {protocol.MP, 6},
	"Temp. EQ_Austritt":              {protocol.MP, 7},
	"Temp. Sauggas":                  {protocol.MP, 9},
	"Temp. Frischwasser_Istwert":     {protocol.MP, 11},
	"Temp. Verdampfung":              {protocol.MP, 12},
	"Temp. Kondensation":             {protocol.MP, 13},
	"Temp. Heissgas":                 {protocol.MP, 15},
	"Niederdruck (bar)":              {protocol.MP, 20},
	"Hochdruck (bar)":                {protocol.MP, 21},
	"Heizkreispumpe":                 {protocol.MP, 22},
	"EQ Pumpe (Ventilator)":          {protocol.MP, 24},
	"Warmwasservorrang":              {protocol.MP, 25},
	"Zirkulationspumpe WW":           {protocol.MP, 29},
	"Verdichter":                     {protocol.MP, 30},
	"Stoerung":                       {protocol.MP, 31},
	"Hauptschalter":                  {protocol.MP, 36},
	"FWS Stroemungsschalter":         {protocol.MP, 38},
	"BSZ Verdichter Schaltungen":     {protocol.MP, 41},
	"Frischwasserpumpe":              {protocol.MP, 50},
	"Verdichteranforderung":          {protocol.MP, 56},
	"HKR_Sollwert":                   {protocol.MP, 57},
	"Liegenschaft":                   {protocol.SP, 3},
	"WP_System":                      {protocol.SP, 4},
	"WW Type":                        {protocol.SP, 5},
	"FWS Type":                       {protocol.SP, 6},
	"Puffer Type":                    {protocol.SP, 7},
	"Verdichter_Status":              {protocol.SP, 10},
	"Verdichter laeuft seit":         {protocol.SP, 11},
	"Verdichter Einschaltverz.(sec)": {protocol.SP, 12},
	"Betriebsart":                    {protocol.SP, 13},
	"HKR Soll_Raum":                  {protocol.SP, 69},
	"HKR Aufheiztemp. (K)":           {protocol.SP, 71},
	"HKR Absenktemp. (K)":            {protocol.SP, 72},
	"HKR Heizgrenze":                 {protocol.SP, 76},
	"HKR RLT Soll_oHG (Heizkurve)":   {protocol.SP, 80},
	"HKR RLT Soll_0 (Heizkurve)":     {protocol.SP, 81},
	"HKR RLT Soll_uHG (Heizkurve)":   {protocol.SP, 82},
	"WW Normaltemp.":                 {protocol.SP, 83},
	"WW Hysterese Normaltemp.":       {protocol.SP, 84},
	"WW Minimaltemp.":                {protocol.SP, 85},
	"WW Hysterese Minimaltemp.":      {protocol.SP, 86},
	"BSZ HKP Betriebsstunden":        {protocol.SP, 153},
	"BSZ HKP Schaltung":              {protocol.SP, 155},
	"BSZ EQ Betriebsstunden":         {protocol.SP, 162},
	"BSZ EQ Schaltungen":             {protocol.SP, 164},
	"BSZ WWV Betriebsstunden":        {protocol.SP, 165},
	"BSZ WWV Schaltungen":            {protocol.SP, 167},
	"BSZ ZIPWW Betriebsstunden":      {protocol.SP, 168},
	"BSZ ZIPWW Schaltungen":          {protocol.SP, 170},
	"BSZ Verdichter Betriebsst. WW":  {protocol.SP, 171},
	"BSZ Verdichter Betriebsst. HKR": {protocol.SP, 172},
	"BSZ Verdichter Betriebsst. ges": {protocol.SP, 173},
	"BSZ Verdichter akt. Laufzeit":   {protocol.SP, 174},
	"MKR2 Aktiviert":                 {protocol.SP, 222},
	"Energiezaehler":                 {protocol.SP, 263},
	"BSZ Verdichter Schaltung WW":    {protocol.SP, 375},
}

func (s *Server) htrestRoutes(h func(string, func(http.ResponseWriter, *http.Request) error)) {
	for _, p := range []string{"/api/v1/param", "/api/v1/param/{$}"} {
		h("GET "+p, s.htList(""))
		h("PUT "+p, s.htPutMany)
	}
	for _, p := range []string{"/api/v1/fastquery", "/api/v1/fastquery/{$}"} {
		h("GET "+p, s.htList(protocol.MP))
	}
	h("GET /api/v1/param/{name}", s.htGet(""))
	h("GET /api/v1/fastquery/{name}", s.htGet(protocol.MP))
	h("PUT /api/v1/param/{name}", s.htPut)
}

// htValues returns the cached values of all known parameters (of one kind, if
// given). Parameters that are not polled are missing.
func (s *Server) htValues(kind protocol.Kind) (map[string]float64, error) {
	snap := s.svc.Snapshot()
	if snap.Status != service.Online {
		return nil, service.ErrOffline
	}
	out := map[string]float64{}
	for name, p := range htParams {
		if kind != "" && p.kind != kind {
			continue
		}
		src := snap.SP
		if p.kind == protocol.MP {
			src = snap.MP
		}
		if v, ok := src[p.id]; ok {
			out[name] = v
		}
	}
	return out, nil
}

func (s *Server) htList(kind protocol.Kind) func(http.ResponseWriter, *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		vals, err := s.htValues(kind)
		if err != nil {
			return err
		}
		// HtREST subset syntax: ?Name1&Name2
		if q := r.URL.Query(); len(q) > 0 {
			sub := map[string]float64{}
			for name := range q {
				v, ok := vals[name]
				if !ok {
					return httpError{http.StatusNotFound, "unknown or unavailable parameter: " + name}
				}
				sub[name] = v
			}
			vals = sub
		}
		return writeJSON(w, vals)
	}
}

func (s *Server) htGet(kind protocol.Kind) func(http.ResponseWriter, *http.Request) error {
	return func(w http.ResponseWriter, r *http.Request) error {
		vals, err := s.htValues(kind)
		if err != nil {
			return err
		}
		v, ok := vals[r.PathValue("name")]
		if !ok {
			return httpError{http.StatusNotFound, "unknown or unavailable parameter: " + r.PathValue("name")}
		}
		return writeJSON(w, map[string]float64{"value": v})
	}
}

// htSetupPoint resolves a parameter name for writing.
func htSetupPoint(name string) (int, error) {
	p, ok := htParams[name]
	if !ok {
		return 0, httpError{http.StatusNotFound, "unknown parameter: " + name}
	}
	if p.kind != protocol.SP {
		return 0, badRequest("%s is a measuring point and cannot be written", name)
	}
	return p.id, nil
}

func (s *Server) htPut(w http.ResponseWriter, r *http.Request) error {
	id, err := htSetupPoint(r.PathValue("name"))
	if err != nil {
		return err
	}
	var body struct {
		Value *float64 `json:"value"`
	}
	if err := decode(r, &body); err != nil {
		return err
	}
	if body.Value == nil {
		return badRequest("value is required")
	}
	got, err := s.svc.WriteSP(r.Context(), id, *body.Value)
	if err != nil {
		return err
	}
	return writeJSON(w, map[string]float64{"value": got})
}

// htPutMany writes several parameters in name order and stops at the first
// failure; the controller has no transactions, so earlier writes stay.
func (s *Server) htPutMany(w http.ResponseWriter, r *http.Request) error {
	var body map[string]float64
	if err := decode(r, &body); err != nil {
		return err
	}
	names := slices.Sorted(maps.Keys(body))
	for _, n := range names {
		if _, err := htSetupPoint(n); err != nil {
			return err
		}
	}
	written := map[string]float64{}
	for _, n := range names {
		got, err := s.svc.WriteSP(r.Context(), htParams[n].id, body[n])
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(errorStatus(err))
			return writeJSON(w, map[string]any{"error": n + ": " + err.Error(), "written": written})
		}
		written[n] = got
	}
	return writeJSON(w, written)
}
