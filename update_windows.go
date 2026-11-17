//go:build windows

package main

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var selfPath = sync.OnceValue(func() string {
	p, err := os.Executable()
	if err != nil {
		return ""
	}
	if r, err := filepath.EvalSymlinks(p); err == nil {
		p = r
	}
	return p
})

// updateTargets: the running copy and, if Larik is installed elsewhere, the
// installed copy (the Start menu shortcut and scheduled tasks point there).
func updateTargets() []string {
	self := selfPath()
	out := []string{self}
	inst := filepath.Join(installDir(), exeName)
	if _, err := os.Stat(inst); err == nil && !strings.EqualFold(filepath.Clean(inst), filepath.Clean(self)) {
		out = append(out, inst)
	}
	return out
}

// swapExe replaces path with exe. Windows lets a running program be renamed
// (not overwritten), so the current file moves to Larik.exe.old-<time> and is
// deleted on a later start.
func swapExe(path string, exe []byte) error {
	stamp := strconv.FormatInt(time.Now().UnixNano(), 36)
	newp, old := path+".new-"+stamp, path+".old-"+stamp
	if err := os.WriteFile(newp, exe, 0o755); err != nil {
		return err
	}
	if err := os.Rename(path, old); err != nil {
		_ = os.Remove(newp)
		return err
	}
	if err := os.Rename(newp, path); err != nil {
		_ = os.Rename(old, path)
		return err
	}
	_ = os.Remove(old) // works when nothing runs it; otherwise cleanupOldExe later
	return nil
}

func installUpdate(exe []byte) error {
	targets := updateTargets()
	if targets[0] == "" {
		return errors.New("can't find " + exeName)
	}
	if err := swapExe(targets[0], exe); err != nil {
		return err
	}
	for _, t := range targets[1:] {
		if err := swapExe(t, exe); err != nil {
			log.Print("update installed copy: ", err)
		}
	}
	return nil
}

// restartAfterUpdate starts the new version (with the same rights).
func restartAfterUpdate() {
	if err := exec.Command(selfPath()).Start(); err != nil {
		log.Print("update restart: ", err)
	}
}

// cleanupOldExe removes copies left by earlier updates (once nothing runs them).
func cleanupOldExe() {
	for _, t := range updateTargets() {
		if t == "" {
			continue
		}
		for _, pat := range []string{t + ".old*", t + ".new-*"} {
			m, _ := filepath.Glob(pat)
			for _, f := range m {
				_ = os.Remove(f)
			}
		}
	}
}
