package main

// NVIDIA driver settings (the same global settings as NVIDIA Control Panel ›
// Manage 3D settings), written through NVAPI's driver settings (DRS) API.
// Only documented setting IDs from NVIDIA's NvApiDriverSettings.h are used.
// Before Larik changes a setting it records the previous value, and "Restore"
// puts that value back (or the driver default if it was never set).

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
)

const (
	nvPrefPStateID    = 0x1057EB71 // Power management mode
	nvPStateMax       = 1          // PREFERRED_PSTATE_PREFER_MAX
	nvPrerenderID     = 0x007BA09E // Maximum pre-rendered frames ("Low latency mode: On" = 1)
	nvFrameLimitID    = 0x10835002 // Frame Rate Limiter (fps, 0 = off, max 1023)
	nvShaderCacheMax  = 0x00AC8497 // Shader disk cache maximum size
	nvShaderUnlimited = 0xFFFFFFFF
	nvTexQualityID    = 0x00CE2691 // Texture filtering - Quality
	nvTexHighPerf     = 0x00000014 // QUALITY_ENHANCEMENTS_HIGHPERFORMANCE
)

type NVSetting struct {
	Key   string `json:"key"` // power | latency | fpslimit | shadercache | texfilter
	Value uint32 `json:"value"`
	Set   bool   `json:"set"`  // set in the global profile (not the driver default)
	Ours  bool   `json:"ours"` // Larik changed it and has a backup
}

type nvBackup struct {
	Set   bool   `json:"set"`
	Value uint32 `json:"value"`
}

var nvKeys = map[string]uint32{"power": nvPrefPStateID, "latency": nvPrerenderID, "fpslimit": nvFrameLimitID,
	"shadercache": nvShaderCacheMax, "texfilter": nvTexQualityID}

var nvOrder = []string{"power", "latency", "fpslimit", "shadercache", "texfilter"}

// nvTarget is the value "on" means for each switch (the FPS limit is chosen by the user).
func nvTarget(key string, v uint32) uint32 {
	switch key {
	case "power":
		return nvPStateMax
	case "latency":
		return 1
	case "shadercache":
		return nvShaderUnlimited
	case "texfilter":
		return nvTexHighPerf
	}
	return v
}

// nvDriver is the platform layer (NVAPI on Windows, in-memory in the demo).
type nvDriver interface {
	Available() bool
	Get(id uint32) (value uint32, set bool, err error)
	Set(id, value uint32) error
	Restore(id uint32) error // back to the driver default
}

var nvMu sync.Mutex

func nvBackupFile() string { return filepath.Join(dataDir(), "nvidia.json") }

func loadNVBackups() map[string]nvBackup {
	m := map[string]nvBackup{}
	if b, err := os.ReadFile(nvBackupFile()); err == nil {
		_ = json.Unmarshal(b, &m)
	}
	return m
}

func nvState(d nvDriver) []NVSetting {
	bk := loadNVBackups()
	var out []NVSetting
	for _, k := range nvOrder {
		v, set, err := d.Get(nvKeys[k])
		if err != nil {
			continue
		}
		_, ours := bk[k]
		out = append(out, NVSetting{Key: k, Value: v, Set: set, Ours: ours})
	}
	return out
}

func validNV(key string, v uint32) bool {
	switch key {
	case "power":
		return v == nvPStateMax
	case "latency":
		return v == 1
	case "fpslimit":
		return v >= 20 && v <= 1023
	case "shadercache":
		return v == nvShaderUnlimited
	case "texfilter":
		return v == nvTexHighPerf
	}
	return false
}

// nvApply sets one setting, keeping the first original value as the backup.
func nvApply(d nvDriver, key string, v uint32) error {
	nvMu.Lock()
	defer nvMu.Unlock()
	id, ok := nvKeys[key]
	if !ok || !validNV(key, v) {
		return errors.New("bad setting")
	}
	bk := loadNVBackups()
	if _, have := bk[key]; !have {
		old, set, err := d.Get(id)
		if err != nil {
			return err
		}
		bk[key] = nvBackup{Set: set, Value: old}
		if err := writeAtomic(nvBackupFile(), bk); err != nil {
			return err
		}
	}
	return d.Set(id, v)
}

// nvRevert puts the value from before Larik's change back.
func nvRevert(d nvDriver, key string) error {
	nvMu.Lock()
	defer nvMu.Unlock()
	id, ok := nvKeys[key]
	if !ok {
		return errors.New("bad setting")
	}
	bk := loadNVBackups()
	b, have := bk[key]
	var err error
	switch {
	case !have || !b.Set:
		err = d.Restore(id)
	default:
		err = d.Set(id, b.Value)
	}
	if err != nil {
		return err
	}
	delete(bk, key)
	if len(bk) == 0 {
		_ = os.Remove(nvBackupFile())
		return nil
	}
	return writeAtomic(nvBackupFile(), bk)
}

// nvRevertAll puts back every setting Larik changed (used by the uninstaller).
func nvRevertAll(d nvDriver) {
	if !d.Available() {
		return
	}
	for k := range loadNVBackups() {
		_ = nvRevert(d, k)
	}
}

func (s *Server) nvidiaRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nvidia", func(w http.ResponseWriter, r *http.Request) {
		d := nvidiaDriver()
		if !d.Available() {
			writeJSON(w, map[string]any{"available": false})
			return
		}
		writeJSON(w, map[string]any{"available": true, "settings": orEmpty(nvState(d))})
	})
	mux.HandleFunc("POST /api/nvidia", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Key   string `json:"key"`
			On    bool   `json:"on"`
			Value uint32 `json:"value"`
		}
		if err := readJSON(r, &req); err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		if !s.e.admin {
			errJSON(w, 403, errNeedAdmin.Error())
			return
		}
		d := nvidiaDriver()
		if !d.Available() {
			errJSON(w, 400, "no_nvidia")
			return
		}
		var err error
		if req.On {
			v := nvTarget(req.Key, req.Value)
			if err = nvApply(d, req.Key, v); err == nil {
				note := ""
				if req.Key == "fpslimit" {
					note = strconv.FormatUint(uint64(v), 10)
				}
				s.e.logHist("nv_"+req.Key, "apply", note)
			}
		} else if err = nvRevert(d, req.Key); err == nil {
			s.e.logHist("nv_"+req.Key, "revert", "")
		}
		if err != nil {
			code := 500
			if err.Error() == "bad setting" {
				code = 400
			}
			errJSON(w, code, err.Error())
			return
		}
		writeJSON(w, map[string]any{"available": true, "settings": orEmpty(nvState(d))})
	})
}
