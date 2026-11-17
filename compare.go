package main

// "Before / after": compares two recorded sessions of the same game, e.g. one
// from before you turned optimizations on and one from after. The numbers come
// from the per-second CSV rows the overlay wrote while you played.

import (
	"encoding/csv"
	"io"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

type SessionStats struct {
	AvgFPS    float64 `json:"avgFps"`
	Low1      float64 `json:"low1"`  // 1st percentile of per-second FPS
	Low01     float64 `json:"low01"` // 0.1th percentile
	Median    float64 `json:"median"`
	MinFPS    float64 `json:"minFps"`
	MaxFPS    float64 `json:"maxFps"`
	Stability float64 `json:"stability"` // % of seconds within ±15% of the median
	Dips      int     `json:"dips"`      // seconds below 60% of the median
	Seconds   int     `json:"seconds"`
}

type CompareSide struct {
	SessionSummary
	Stats  SessionStats `json:"stats"`
	Series []float64    `json:"series"` // FPS over the session, at most seriesPoints values
}

type CompareChange struct {
	At     string `json:"at"`
	ID     string `json:"id"`
	Action string `json:"action"`
	Note   string `json:"note,omitempty"`
}

const seriesPoints = 120

// readSessionFPS reads the fps column of a session CSV (at most 24 h of rows).
func readSessionFPS(dir, id string) []float64 {
	p, ok := sessionCSV(dir, id)
	if !ok {
		return nil
	}
	f, err := os.Open(p)
	if err != nil {
		return nil
	}
	defer f.Close()
	r := csv.NewReader(io.LimitReader(f, 32<<20))
	r.FieldsPerRecord = -1
	head, err := r.Read()
	if err != nil {
		return nil
	}
	col := -1
	for i, h := range head {
		if strings.TrimSpace(strings.TrimPrefix(h, "\xef\xbb\xbf")) == "fps" {
			col = i
		}
	}
	if col < 0 {
		return nil
	}
	var out []float64
	for len(out) < 86400 {
		rec, err := r.Read()
		if err != nil {
			break
		}
		if col >= len(rec) {
			continue
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(rec[col]), 64)
		if err != nil || v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) {
			continue
		}
		out = append(out, v)
	}
	return out
}

func pct(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	return sorted[int(float64(len(sorted)-1)*p)]
}

func round1(v float64) float64 { return math.Round(v*10) / 10 }

func sessionStats(fps []float64) SessionStats {
	st := SessionStats{Seconds: len(fps)}
	if len(fps) == 0 {
		return st
	}
	sorted := append([]float64(nil), fps...)
	sort.Float64s(sorted)
	sum := 0.0
	for _, v := range fps {
		sum += v
	}
	med := pct(sorted, 0.5)
	in, dips := 0, 0
	for _, v := range fps {
		if math.Abs(v-med) <= 0.15*med {
			in++
		}
		if v < 0.6*med {
			dips++
		}
	}
	st.AvgFPS = round1(sum / float64(len(fps)))
	st.Low1, st.Low01, st.Median = round1(pct(sorted, 0.01)), round1(pct(sorted, 0.001)), round1(med)
	st.MinFPS, st.MaxFPS = round1(sorted[0]), round1(sorted[len(sorted)-1])
	st.Stability = math.Round(float64(in) / float64(len(fps)) * 100)
	st.Dips = dips
	return st
}

// downsample averages v into at most n buckets.
func downsample(v []float64, n int) []float64 {
	if len(v) <= n {
		return append([]float64{}, v...)
	}
	out := make([]float64, n)
	for i := 0; i < n; i++ {
		a, b := i*len(v)/n, (i+1)*len(v)/n
		s := 0.0
		for _, x := range v[a:b] {
			s += x
		}
		out[i] = round1(s / float64(b-a))
	}
	return out
}

func sameGame(a, b SessionSummary) bool {
	if a.Exe != "" && b.Exe != "" {
		return strings.EqualFold(a.Exe, b.Exe)
	}
	return strings.EqualFold(a.Game, b.Game)
}

func parseT(s string) time.Time {
	t, _ := time.Parse(time.RFC3339, s)
	return t
}

// changesBetween lists the optimizations changed after session a ended and
// before session b started.
func changesBetween(hist []HistItem, a, b SessionSummary) []CompareChange {
	from, to := parseT(a.End), parseT(b.Start)
	if to.Before(from) {
		from, to = parseT(b.End), parseT(a.Start)
	}
	out := []CompareChange{}
	for _, h := range hist {
		t := parseT(h.At)
		if !h.OK || t.Before(from) || t.After(to) {
			continue
		}
		out = append(out, CompareChange{At: h.At, ID: h.ID, Action: h.Action, Note: h.Note})
	}
	return out
}

// pickPair suggests what to compare: the newest session ("after") and an
// older session of the same game, preferring the newest one recorded before
// you changed something in Larik. list is newest first.
func pickPair(list []SessionSummary, hist []HistItem) (string, string) {
	if len(list) == 0 {
		return "", ""
	}
	b := list[0]
	var fallback string
	for _, c := range list[1:] {
		if !sameGame(c, b) {
			continue
		}
		if fallback == "" {
			fallback = c.ID
		}
		if len(changesBetween(hist, c, b)) > 0 {
			return c.ID, b.ID
		}
	}
	if fallback == "" {
		return "", b.ID
	}
	return fallback, b.ID
}

func findSession(list []SessionSummary, id string) (SessionSummary, bool) {
	for _, s := range list {
		if s.ID == id {
			return s, true
		}
	}
	return SessionSummary{}, false
}

func compareSide(dir string, s SessionSummary) CompareSide {
	fps := readSessionFPS(dir, s.ID)
	side := CompareSide{SessionSummary: s, Series: []float64{}}
	side.CSV = ""
	if len(fps) >= 10 {
		side.Stats = sessionStats(fps)
		side.Series = downsample(fps, seriesPoints)
	} else {
		// CSV deleted or empty: fall back to the summary.
		side.Stats = SessionStats{AvgFPS: s.AvgFPS, Low1: s.Low1, MinFPS: s.MinFPS, Seconds: s.Duration, Low01: -1, Median: -1, MaxFPS: -1, Stability: -1, Dips: -1}
	}
	return side
}

func (s *Server) compareRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/sessions/compare", func(w http.ResponseWriter, r *http.Request) {
		dir := localDir()
		list := loadSessions(dir)
		var hist []HistItem
		s.e.store.Read(func(d *StoreData) { hist = append(hist, d.History...) })
		a, b := r.URL.Query().Get("a"), r.URL.Query().Get("b")
		auto := a == "" && b == ""
		if auto {
			a, b = pickPair(list, hist)
		}
		sb, okB := findSession(list, b)
		sa, okA := findSession(list, a)
		out := map[string]any{"auto": auto, "changes": []CompareChange{}}
		if okB {
			out["b"] = compareSide(dir, sb)
		}
		if okA {
			out["a"] = compareSide(dir, sa)
		}
		if okA && okB {
			out["changes"] = changesBetween(hist, sa, sb)
		}
		writeJSON(w, out)
	})
}
