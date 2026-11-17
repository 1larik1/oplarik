package main

// Game session recording: while a full-screen game shows frames, the overlay
// process writes one CSV row per second to %LOCALAPPDATA%\Larik\Sessions and,
// when the game ends, a summary (average FPS, 1% low, minimum, top
// temperatures, ping) to sessions.json for the app and the tray.

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

type SessionSummary struct {
	ID         string  `json:"id"`
	Game       string  `json:"game"`
	Exe        string  `json:"exe,omitempty"` // 1.0.7+
	Start      string  `json:"start"`
	End        string  `json:"end"`
	Duration   int     `json:"duration"` // seconds
	AvgFPS     float64 `json:"avgFps"`
	Low1       float64 `json:"low1"` // 1st percentile of per-second FPS
	MinFPS     float64 `json:"minFps"`
	MaxGPUTemp float64 `json:"maxGpuTemp"` // -1 = unknown
	MaxCPUTemp float64 `json:"maxCpuTemp"`
	AvgPing    float64 `json:"avgPing"`
	Warnings   int     `json:"warnings"` // seconds with a heat / clock warning
	CSV        string  `json:"csv"`
}

const (
	sessionStartAfter = 10 * time.Second // game must be in front this long
	sessionEndAfter   = 20 * time.Second // no frames this long = game closed
	sessionMinLength  = 60 * time.Second // shorter sessions are discarded
	sessionsKept      = 50
)

// Programs that draw with DirectX but aren't games.
var sessionIgnore = map[string]bool{
	"chrome.exe": true, "msedge.exe": true, "firefox.exe": true, "opera.exe": true, "brave.exe": true, "yandex.exe": true,
	"discord.exe": true, "telegram.exe": true, "spotify.exe": true, "steamwebhelper.exe": true, "epicgameslauncher.exe": true,
	"explorer.exe": true, "applicationframehost.exe": true, "code.exe": true, "obs64.exe": true, "vlc.exe": true,
	"mpc-hc64.exe": true, "teams.exe": true, "ms-teams.exe": true, "zoom.exe": true, "larik.exe": true, "dwm.exe": true,
	"searchhost.exe": true, "startmenuexperiencehost.exe": true, "shellexperiencehost.exe": true, "textinputhost.exe": true,
}

type fpsLookup func(pid uint32) (fps, low1, ft float64, ok bool)

type activeSession struct {
	pid       uint32
	exe       string
	start     time.Time
	lastSeen  time.Time
	lastRow   time.Time
	path      string
	f         *os.File
	w         *csv.Writer
	fps       []float64
	maxG      float64
	maxC      float64
	pingSum   float64
	pingN     int
	warnSecs  int
	rows      int
	candidate bool
}

type sessionRecorder struct {
	mu       sync.Mutex
	dir      string // folder with sessions.json and Sessions\
	cur      *activeSession
	candPID  uint32
	candFrom time.Time
}

func newSessionRecorder(dir string) *sessionRecorder { return &sessionRecorder{dir: dir} }

func sessionsFile(dir string) string { return filepath.Join(dir, "sessions.json") }

func loadSessions(dir string) []SessionSummary {
	b, err := os.ReadFile(sessionsFile(dir))
	if err != nil {
		return nil
	}
	var out []SessionSummary
	_ = json.Unmarshal(b, &out)
	return out
}

func safeName(s string) string {
	s = strings.TrimSuffix(strings.ToLower(s), ".exe")
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "game"
	}
	return b.String()
}

// Tick is called on every overlay sample. fgPID/fgExe describe the
// foreground window; eligible says whether it looks like a game (full screen,
// not a browser or chat app). Returns a summary when a session just ended.
func (r *sessionRecorder) Tick(now time.Time, fgPID uint32, fgExe string, eligible bool, look fpsLookup, d OverlayData) *SessionSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cur != nil {
		fps, low1, ft, ok := look(r.cur.pid)
		if ok && fps > 0 {
			r.cur.lastSeen = now
			if now.Sub(r.cur.lastRow) >= time.Second {
				r.cur.lastRow = now
				r.row(now, fps, low1, ft, d)
			}
			return nil
		}
		if now.Sub(r.cur.lastSeen) < sessionEndAfter {
			return nil
		}
		return r.finish(r.cur.lastSeen)
	}
	// waiting for a game
	fps, _, _, ok := look(fgPID)
	if !eligible || !ok || fps <= 0 || sessionIgnore[strings.ToLower(fgExe)] {
		r.candPID = 0
		return nil
	}
	if r.candPID != fgPID {
		r.candPID, r.candFrom = fgPID, now
		return nil
	}
	if now.Sub(r.candFrom) < sessionStartAfter {
		return nil
	}
	r.begin(now, fgPID, fgExe)
	return nil
}

func (r *sessionRecorder) begin(now time.Time, pid uint32, exe string) {
	dir := filepath.Join(r.dir, "Sessions")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, now.Format("2006-01-02_15-04-05")+"_"+safeName(exe)+".csv")
	f, err := os.Create(path)
	if err != nil {
		return
	}
	s := &activeSession{pid: pid, exe: exe, start: now, lastSeen: now, path: path, f: f, w: csv.NewWriter(f), maxG: -1, maxC: -1}
	_ = s.w.Write([]string{"time", "fps", "low1_fps", "frametime_ms", "gpu_pct", "gpu_temp_c", "gpu_clock_mhz", "gpu_power_w",
		"cpu_pct", "cpu_temp_c", "ram_gb", "ping_ms", "warnings"})
	r.cur = s
	r.candPID = 0
}

func num(v float64, dec int) string {
	if v < 0 || math.IsNaN(v) {
		return ""
	}
	return fmt.Sprintf("%.*f", dec, v)
}

func (r *sessionRecorder) row(now time.Time, fps, low1, ft float64, d OverlayData) {
	s := r.cur
	s.fps = append(s.fps, fps)
	if d.GPUTemp > s.maxG {
		s.maxG = d.GPUTemp
	}
	if d.CPUTemp > s.maxC {
		s.maxC = d.CPUTemp
	}
	if d.Ping >= 0 {
		s.pingSum += d.Ping
		s.pingN++
	}
	if len(d.Warn) > 0 {
		s.warnSecs++
	}
	ram := -1.0
	if d.RAMUsed >= 0 {
		ram = d.RAMUsed / (1 << 30)
	}
	_ = s.w.Write([]string{now.Format("15:04:05"), num(fps, 1), num(low1, 1), num(ft, 2), num(d.GPU, 0), num(d.GPUTemp, 0),
		num(d.GPUClock, 0), num(d.GPUPower, 0), num(d.CPU, 0), num(d.CPUTemp, 0), num(ram, 1), num(d.Ping, 0), strings.Join(d.Warn, " ")})
	s.rows++
	if s.rows%10 == 0 {
		s.w.Flush()
	}
}

func (r *sessionRecorder) finish(end time.Time) *SessionSummary {
	s := r.cur
	r.cur = nil
	if s == nil {
		return nil
	}
	s.w.Flush()
	_ = s.f.Close()
	if end.Sub(s.start) < sessionMinLength || len(s.fps) < 30 {
		_ = os.Remove(s.path)
		return nil
	}
	sum := SessionSummary{ID: filepath.Base(s.path), Game: gameTitle(s.exe), Exe: strings.ToLower(s.exe),
		Start: s.start.Format(time.RFC3339), End: end.Format(time.RFC3339), Duration: int(end.Sub(s.start).Seconds()),
		MaxGPUTemp: s.maxG, MaxCPUTemp: s.maxC, AvgPing: -1, Warnings: s.warnSecs, CSV: s.path}
	total := 0.0
	for _, v := range s.fps {
		total += v
	}
	sorted := append([]float64(nil), s.fps...)
	sort.Float64s(sorted)
	sum.AvgFPS = math.Round(total/float64(len(s.fps))*10) / 10
	sum.MinFPS = math.Round(sorted[0]*10) / 10
	sum.Low1 = math.Round(sorted[int(float64(len(sorted)-1)*0.01)]*10) / 10
	if s.pingN > 0 {
		sum.AvgPing = math.Round(s.pingSum / float64(s.pingN))
	}
	list := append([]SessionSummary{sum}, loadSessions(r.dir)...)
	if len(list) > sessionsKept {
		for _, old := range list[sessionsKept:] {
			if p, ok := sessionCSV(r.dir, old.ID); ok {
				_ = os.Remove(p)
			}
		}
		list = list[:sessionsKept]
	}
	_ = os.MkdirAll(r.dir, 0o755)
	_ = writeAtomic(sessionsFile(r.dir), list)
	return &sum
}

// Close ends a running session (the overlay is shutting down).
func (r *sessionRecorder) Close(now time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cur != nil {
		r.finish(r.cur.lastSeen)
	}
}

// sessionCSV maps a session id to its file inside Larik's Sessions folder.
// sessions.json lives in a user-writable folder, so the stored "csv" path is
// never trusted for deleting or opening files.
func sessionCSV(dir, id string) (string, bool) {
	if id == "" || filepath.Base(id) != id || strings.ContainsAny(id, `/\:`) || !strings.HasSuffix(strings.ToLower(id), ".csv") {
		return "", false
	}
	return filepath.Join(dir, "Sessions", id), true
}

// deleteSession removes one summary and its CSV file.
func deleteSession(dir, id string) []SessionSummary {
	list := loadSessions(dir)
	out := list[:0]
	for _, ss := range list {
		if ss.ID == id {
			if p, ok := sessionCSV(dir, ss.ID); ok {
				_ = os.Remove(p)
			}
			continue
		}
		out = append(out, ss)
	}
	_ = writeAtomic(sessionsFile(dir), out)
	return out
}

func fmtDuration(lang string, sec int) string {
	h, m := sec/3600, (sec%3600)/60
	if h > 0 {
		return fmt.Sprintf(nt(lang, "durH"), h, m)
	}
	if m < 1 {
		m = 1
	}
	return fmt.Sprintf(nt(lang, "durM"), m)
}

// sessionNotice is the tray notification shown when a game session ends.
func sessionNotice(lang string, s SessionSummary) (string, string) {
	f := func(v float64) string { return fmtNum(v, 0, lang) }
	body := fmt.Sprintf(nt(lang, "sesBody"), fmtDuration(lang, s.Duration), f(s.AvgFPS), f(s.Low1), f(s.MinFPS))
	if s.MaxGPUTemp > 0 {
		body += fmt.Sprintf(nt(lang, "sesGPU"), s.MaxGPUTemp)
	}
	return fmt.Sprintf(nt(lang, "sesTitle"), s.Game), body
}

// gameTitle turns an exe name into the game's name when Larik knows it
// (FortniteClient-Win64-Shipping.exe → Fortnite).
func gameTitle(exe string) string {
	for _, g := range popularGames {
		if strings.EqualFold(g.Exe, exe) {
			return g.Name
		}
	}
	return strings.TrimSuffix(exe, filepath.Ext(exe))
}
