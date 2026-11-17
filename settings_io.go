package main

// Export / import (1.0.7): a small JSON file with Larik's settings, the list
// of optimizations that are on and Larik's NVIDIA settings, to move them to
// another PC or keep them after reinstalling Windows. Account data (name,
// password, photo, 2FA) is never exported.

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
)

type portableSettings struct {
	Lang        string          `json:"lang"`
	Theme       string          `json:"theme"`
	Sounds      bool            `json:"sounds"`
	Motion      bool            `json:"motion"`
	AutoRestore bool            `json:"autoRestore"`
	Overlay     OverlayConfig   `json:"overlay"`
	Tray        bool            `json:"tray"`
	TrayAuto    bool            `json:"trayAutostart"`
	AutoLock    int             `json:"autoLock"`
	AutoGame    AutoGameConfig  `json:"autoGame"`
	AutoClean   AutoCleanConfig `json:"autoClean"`
	// The update source is deliberately not exported or imported: a shared
	// settings file must never decide where Larik downloads new versions from.
}

func toPortable(st Settings) portableSettings {
	return portableSettings{Lang: st.Lang, Theme: st.Theme, Sounds: st.Sounds, Motion: st.Motion, AutoRestore: st.AutoRestore,
		Overlay: st.Overlay, Tray: st.Tray, TrayAuto: st.TrayAuto, AutoLock: st.AutoLock, AutoGame: st.AutoGame,
		AutoClean: st.AutoClean}
}

func (p portableSettings) apply(st Settings) Settings {
	st.Lang, st.Theme, st.Sounds, st.Motion, st.AutoRestore = p.Lang, p.Theme, p.Sounds, p.Motion, p.AutoRestore
	st.Overlay, st.Tray, st.TrayAuto, st.AutoLock = p.Overlay, p.Tray, p.TrayAuto, p.AutoLock
	st.AutoGame, st.AutoClean = p.AutoGame, p.AutoClean
	return st
}

type ExportFile struct {
	App      string            `json:"app"`
	Format   int               `json:"format"`
	Version  string            `json:"version"`
	Exported string            `json:"exported"`
	Settings json.RawMessage   `json:"settings"`
	Tweaks   []string          `json:"tweaks"`
	NVIDIA   map[string]uint32 `json:"nvidia,omitempty"`
}

func (s *Server) exportData() ExportFile {
	st := s.e.store.GetSettings()
	raw, _ := json.Marshal(toPortable(st))
	out := ExportFile{App: appName, Format: 1, Version: appVersion, Exported: nowStr(), Settings: raw, Tweaks: []string{}}
	s.e.store.Read(func(d *StoreData) {
		for id := range d.Applied {
			out.Tweaks = append(out.Tweaks, id)
		}
	})
	sort.Strings(out.Tweaks)
	if d := nvidiaDriver(); d.Available() {
		for _, n := range nvState(d) {
			if n.Ours {
				if out.NVIDIA == nil {
					out.NVIDIA = map[string]uint32{}
				}
				out.NVIDIA[n.Key] = n.Value
			}
		}
	}
	return out
}

// importData applies the settings from an export file and returns which
// optimizations and NVIDIA settings it lists (the UI asks before applying them).
func (s *Server) importData(f ExportFile) (Settings, []string, map[string]uint32, string, error) {
	if (f.App != appName && f.App != "Larik") || f.Format != 1 || len(f.Settings) == 0 {
		return Settings{}, nil, nil, "", errors.New("not_larik_file")
	}
	cur := s.e.store.GetSettings()
	p := toPortable(cur)
	if err := json.Unmarshal(f.Settings, &p); err != nil {
		return Settings{}, nil, nil, "", errors.New("not_larik_file")
	}
	st, warn, err := s.saveSettings(p.apply(cur))
	if err != nil {
		return Settings{}, nil, nil, "", err
	}
	var tweaks []string
	seen := map[string]bool{}
	for _, id := range f.Tweaks {
		if s.e.cat.Get(id) != nil && !seen[id] && len(tweaks) < 500 {
			seen[id] = true
			tweaks = append(tweaks, id)
		}
	}
	nv := map[string]uint32{}
	for k, v := range f.NVIDIA {
		if _, ok := nvKeys[k]; ok && validNV(k, v) {
			nv[k] = v
		}
	}
	return st, orEmpty(tweaks), nv, warn, nil
}

func (s *Server) ioRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/export", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.exportData())
	})
	mux.HandleFunc("POST /api/import", func(w http.ResponseWriter, r *http.Request) {
		var f ExportFile
		if err := jsonDecodeLimit(r, &f, 1<<20); err != nil {
			errJSON(w, 400, "not_larik_file")
			return
		}
		st, tweaks, nv, warn, err := s.importData(f)
		if err != nil {
			code := 500
			if err.Error() == "not_larik_file" {
				code = 400
			}
			errJSON(w, code, err.Error())
			return
		}
		writeJSON(w, map[string]any{"settings": st, "tweaks": tweaks, "nvidia": nv, "warn": warn, "from": f.Version})
	})
}
