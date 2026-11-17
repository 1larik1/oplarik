package main

// Automatic game mode (1.0.7): the overlay process watches for a game in the
// foreground. Once a full-screen game has been in front for a few seconds,
// Larik turns on Game Boost and shows the overlay; when the game process
// exits, it switches Boost off again and hides the overlay. Boost started by
// hand is never switched off automatically.

import (
	"strings"
	"time"
)

const autoGameStartAfter = 8 * time.Second

type gameWatch struct {
	candPID  uint32
	candFrom time.Time
	pid      uint32
	exe      string
	since    time.Time
}

// Tick returns "start" when a game was detected, "end" when it closed, or "".
// eligible says whether the foreground window looks like a game right now;
// alive reports whether a process still runs.
func (g *gameWatch) Tick(now time.Time, fgPID uint32, fgExe string, eligible bool, alive func(uint32) bool) string {
	if g.pid != 0 {
		if !alive(g.pid) {
			g.pid, g.exe, g.candPID = 0, "", 0
			return "end"
		}
		return ""
	}
	if !eligible || fgPID == 0 || isIgnoredName(fgExe) {
		g.candPID = 0
		return ""
	}
	if g.candPID != fgPID {
		g.candPID, g.candFrom = fgPID, now
		return ""
	}
	if now.Sub(g.candFrom) < autoGameStartAfter {
		return ""
	}
	g.pid, g.exe, g.since, g.candPID = fgPID, fgExe, now, 0
	return "start"
}

func (g *gameWatch) Game() string { return g.exe }

func isIgnoredName(exe string) bool { return exe == "" || sessionIgnore[strings.ToLower(exe)] }

// startAutoBoost turns Game Boost on for a game unless it is already on.
func startAutoBoost(sys Sys, exe string) error {
	if loadBoost() != nil {
		return nil // already on (by hand or for another game): leave it alone
	}
	_, err := setBoostFor(sys, true, strings.ToLower(exe))
	return err
}

// endAutoBoost switches Boost off only if automatic game mode turned it on.
func endAutoBoost(sys Sys) error {
	b := loadBoost()
	if b == nil || b.Auto == "" {
		return nil
	}
	_, err := setBoostFor(sys, false, b.Auto)
	return err
}
