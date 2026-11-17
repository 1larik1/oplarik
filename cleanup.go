package main

// Scheduled cleanup (1.0.7): a weekly Windows task runs "Larik.exe --clean",
// which removes the same temporary files as the "Clean temp files" tool
// (files older than an hour; never shader caches or your own files).

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type CleanRun struct {
	At    string `json:"at"`
	Bytes int64  `json:"bytes"`
	Files int    `json:"files"`
	Auto  bool   `json:"auto"` // run by the schedule
}

func cleanLogFile() string { return filepath.Join(localDir(), "cleanup.json") }

func loadCleanLog() []CleanRun {
	var out []CleanRun
	if b, err := os.ReadFile(cleanLogFile()); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return orEmpty(out)
}

func runCleanup(auto bool) CleanRun {
	b, n := tempClean()
	r := CleanRun{At: nowStr(), Bytes: b, Files: n, Auto: auto}
	list := append([]CleanRun{r}, loadCleanLog()...)
	if len(list) > 12 {
		list = list[:12]
	}
	_ = os.MkdirAll(localDir(), 0o755)
	_ = writeAtomic(cleanLogFile(), list)
	return r
}

// nextWeekly is the next time the weekly task fires after now.
func nextWeekly(now time.Time, day, hour int) time.Time {
	t := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	add := (day - int(now.Weekday()) + 7) % 7
	t = t.AddDate(0, 0, add)
	if !t.After(now) {
		t = t.AddDate(0, 0, 7)
	}
	return t
}

func (s *Server) cleanupRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/cleanup", func(w http.ResponseWriter, r *http.Request) {
		c := s.e.store.GetSettings().AutoClean
		out := map[string]any{"runs": loadCleanLog(), "config": c}
		if c.Enabled {
			out["next"] = nextWeekly(time.Now(), c.Day, c.Hour).Format(time.RFC3339)
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("POST /api/cleanup/run", func(w http.ResponseWriter, r *http.Request) {
		run := runCleanup(false)
		writeJSON(w, map[string]any{"run": run, "runs": loadCleanLog()})
	})
}
