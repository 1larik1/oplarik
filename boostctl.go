package main

// Game Boost state lives in its own small file (boost.json) instead of the
// engine's state.json, so both the engine and the tray icon process can switch
// Boost on and off without overwriting each other's data.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var boostMu sync.Mutex

func boostFile() string { return filepath.Join(dataDir(), "boost.json") }

func loadBoost() *BoostSession {
	b, err := os.ReadFile(boostFile())
	if err != nil {
		return nil
	}
	var s BoostSession
	if json.Unmarshal(b, &s) != nil || !s.Active {
		return nil
	}
	return &s
}

func saveBoost(s *BoostSession) error {
	if s == nil {
		err := os.Remove(boostFile())
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	_ = os.MkdirAll(dataDir(), 0o755)
	return writeAtomic(boostFile(), s)
}

var errNoMaxPlan = errors.New("this PC only offers the Balanced plan (Modern Standby); high-performance plans are unavailable")

// activateMaxPlan switches to Larik's Ultimate Performance copy (created on
// first use), falling back to High performance.
func activateMaxPlan(s Sys) error {
	if !s.PlanExists(planOurs) {
		_, _ = s.Exec("powercfg", "-duplicatescheme", planUltimate, planOurs)
		if !s.PlanExists(planOurs) {
			_, _ = s.Exec("powercfg", "-duplicatescheme", planHighPerf, planOurs)
		}
		if s.PlanExists(planOurs) {
			_, _ = s.Exec("powercfg", "-changename", planOurs, "OpLarik Ultimate Performance", "Maximum performance plan created by OpLarik")
		}
	}
	if s.PlanExists(planOurs) {
		_, err := s.Exec("powercfg", "-setactive", planOurs)
		return err
	}
	if s.PlanExists(planHighPerf) {
		_, err := s.Exec("powercfg", "-setactive", planHighPerf)
		return err
	}
	return errNoMaxPlan
}

// setBoost turns the Game Boost session on or off. Turning it off restarts
// exactly the services it paused and restores the previous power plan.
// A machine-wide lock keeps the app and the tray from running it at the same
// time, and the restore record is saved before each change, so it survives
// even if the process is closed halfway.
func setBoost(sys Sys, on bool) (*BoostSession, error) { return setBoostFor(sys, on, "") }

// setBoostFor is setBoost for automatic game mode: auto names the game. An
// automatic "off" only ends a session that automatic mode started for a game.
func setBoostFor(sys Sys, on bool, auto string) (*BoostSession, error) {
	boostMu.Lock()
	defer boostMu.Unlock()
	unlock := lockBoostGlobal()
	defer unlock()
	sess := loadBoost()
	if !on && auto != "" && (sess == nil || sess.Auto == "") {
		return sess, nil // switched on by hand meanwhile: keep it
	}
	if on {
		if sess != nil {
			return sess, nil
		}
		ns := &BoostSession{Active: true, Since: nowStr(), PrevPlan: strings.ToLower(sys.ActivePlan()), Auto: auto}
		if err := saveBoost(ns); err != nil {
			return nil, err
		}
		_ = activateMaxPlan(sys)
		for _, name := range boostServices {
			if serviceRunning(name) {
				ns.Stopped = append(ns.Stopped, name)
				_ = saveBoost(ns) // record first, then stop
				sys.SvcStop(name)
			}
		}
		_, _, _ = cleanMemory()
		return ns, saveBoost(ns)
	}
	if sess != nil {
		for _, name := range sess.Stopped {
			sys.SvcRun(name)
		}
		if sess.PrevPlan != "" && !strings.EqualFold(sess.PrevPlan, sys.ActivePlan()) {
			_, _ = sys.Exec("powercfg", "-setactive", sess.PrevPlan)
		}
	}
	return nil, saveBoost(nil)
}

// migrateBoost moves a session saved by Larik 1.0.4 or older out of state.json.
func migrateBoost(st *Store) {
	var old *BoostSession
	st.Read(func(d *StoreData) { old = d.Boost })
	if old == nil {
		return
	}
	if loadBoost() == nil && old.Active {
		_ = saveBoost(old)
	}
	st.Update(func(d *StoreData) { d.Boost = nil })
}
