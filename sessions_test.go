package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionRecorder(t *testing.T) {
	dir := t.TempDir()
	r := newSessionRecorder(dir)
	t0 := time.Date(2026, 10, 10, 20, 0, 0, 0, time.Local)
	gameFPS := func(fps float64) fpsLookup {
		return func(pid uint32) (float64, float64, float64, bool) {
			if pid == 42 && fps > 0 {
				return fps, fps * 0.8, 1000 / fps, true
			}
			return 0, 0, 0, false
		}
	}
	d := naData()
	d.GPUTemp, d.CPUTemp, d.Ping = 70, 60, 30
	// a browser never starts a session
	for i := 0; i < 30; i++ {
		r.Tick(t0.Add(time.Duration(i)*time.Second), 42, "chrome.exe", true, gameFPS(60), d)
	}
	if r.cur != nil {
		t.Fatal("browser recorded")
	}
	// game: 10 s warm-up, then 3 minutes
	var now time.Time
	for i := 0; i <= 190; i++ {
		now = t0.Add(time.Duration(100+i) * time.Second)
		fps := 140.0
		if i == 120 {
			fps = 35 // one bad second
			d.GPUTemp = 88
			d.Warn = []string{"gputemp"}
		} else {
			d.GPUTemp, d.Warn = 70, nil
		}
		if s := r.Tick(now, 42, "cs2.exe", true, gameFPS(fps), d); s != nil {
			t.Fatal("ended early")
		}
	}
	if r.cur == nil {
		t.Fatal("no session")
	}
	// game closed: summary after 20 s without frames
	var sum *SessionSummary
	for i := 1; i <= 25 && sum == nil; i++ {
		sum = r.Tick(now.Add(time.Duration(i)*time.Second), 1, "explorer.exe", false, gameFPS(0), naData())
	}
	if sum == nil {
		t.Fatal("no summary")
	}
	if sum.Game != "Counter-Strike 2" || sum.MinFPS != 35 || sum.MaxGPUTemp != 88 || sum.AvgPing != 30 || sum.Warnings != 1 || sum.Duration < 170 {
		t.Fatalf("summary %+v", sum)
	}
	if sum.AvgFPS < 139 || sum.AvgFPS > 140 {
		t.Fatal("avg", sum.AvgFPS)
	}
	b, err := os.ReadFile(sum.CSV)
	if err != nil || !strings.HasPrefix(string(b), "time,fps,") || strings.Count(string(b), "\n") < 170 {
		t.Fatal("csv", err)
	}
	if l := loadSessions(dir); len(l) != 1 || l[0].ID != sum.ID {
		t.Fatal("sessions.json")
	}
	// a short session is discarded with its file
	r2 := newSessionRecorder(dir)
	for i := 0; i <= 40; i++ {
		r2.Tick(t0.Add(time.Duration(1000+i)*time.Second), 42, "game.exe", true, gameFPS(90), d)
	}
	path := r2.cur.path
	r2.Close(t0)
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("short session kept")
	}
	if len(loadSessions(dir)) != 1 {
		t.Fatal("short session listed")
	}
}

func TestWarnState(t *testing.T) {
	var w warnState
	t0 := time.Now()
	hot := naSensors()
	hot.GPUTemp = 90
	for i := 0; i < 2; i++ {
		if len(w.update(t0.Add(time.Duration(i)*time.Second), hot, 50, 90)) != 0 {
			t.Fatal("warned too early")
		}
	}
	if got := w.update(t0.Add(2*time.Second), hot, 50, 90); len(got) != 1 || got[0] != "gputemp" {
		t.Fatal("expected gputemp", got)
	}
	cool := naSensors()
	cool.GPUTemp = 60
	if len(w.update(t0.Add(5*time.Second), cool, 50, 90)) != 1 {
		t.Fatal("warning should stay 10 s")
	}
	if len(w.update(t0.Add(13*time.Second), cool, 50, 90)) != 0 {
		t.Fatal("warning should clear")
	}
	idle := naSensors()
	idle.GPUThrottle = true
	for i := 0; i < 5; i++ {
		if len(w.update(t0.Add(time.Duration(20+i)*time.Second), idle, 5, 3)) != 0 {
			t.Fatal("idle GPU throttle flags must be ignored")
		}
	}
}

func TestSessionNotice(t *testing.T) {
	ti, body := sessionNotice("ru", SessionSummary{Game: "cs2", Duration: 4320, AvgFPS: 186.4, Low1: 97, MinFPS: 61, MaxGPUTemp: 78})
	if ti != "Итог игры: cs2" || body != "1 ч 12 мин · средний 186 FPS · 1% low 97 · мин 61 · ГП до 78 °C" {
		t.Fatalf("%q / %q", ti, body)
	}
}

func TestSessionPathsAreConfined(t *testing.T) {
	dir := t.TempDir()
	victim := filepath.Join(t.TempDir(), "important.txt")
	_ = os.WriteFile(victim, []byte("x"), 0o644)
	_ = writeAtomic(sessionsFile(dir), []SessionSummary{
		{ID: "../../important.txt", CSV: victim},
		{ID: "evil.csv", CSV: victim},
	})
	deleteSession(dir, "evil.csv")
	deleteSession(dir, "../../important.txt")
	if _, err := os.Stat(victim); err != nil {
		t.Fatal("deleted a file outside the Sessions folder")
	}
	for _, bad := range []string{"", "a/b.csv", `a\b.csv`, "x.txt", "C:evil.csv", ".."} {
		if _, ok := sessionCSV(dir, bad); ok {
			t.Fatal("accepted", bad)
		}
	}
}
