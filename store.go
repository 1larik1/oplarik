package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Applied records the original values Larik replaced for one tweak.
type Applied struct {
	At string                     `json:"at"`
	B  map[string]json.RawMessage `json:"b"`
}

type HistItem struct {
	At     string `json:"at"`
	Action string `json:"action"`
	ID     string `json:"id"`
	OK     bool   `json:"ok"`
	Err    string `json:"err,omitempty"`
	Note   string `json:"note,omitempty"` // tool changes (DNS, NVIDIA): what was set
}

type Game struct {
	ID     string                     `json:"id"`
	Name   string                     `json:"name"`
	Exe    string                     `json:"exe"`
	Path   string                     `json:"path,omitempty"`
	Source string                     `json:"source"`
	Prio   bool                       `json:"prio"`
	GPU    bool                       `json:"gpu"`
	FSO    bool                       `json:"fso"`
	B      map[string]json.RawMessage `json:"b,omitempty"`
	Added  string                     `json:"added"`
	// 1.0.8
	SteamID string `json:"steamId,omitempty"`
	Cover   string `json:"cover,omitempty"` // filled when listing, not stored
	// 1.0.9
	Launch     string `json:"launch,omitempty"`
	LastLaunch string `json:"lastLaunch,omitempty"`
}

type BoostSession struct {
	Active   bool     `json:"active"`
	Since    string   `json:"since"`
	PrevPlan string   `json:"prevPlan"`
	Stopped  []string `json:"stopped"`
	Auto     string   `json:"auto,omitempty"` // game exe when automatic game mode started it
}

type StoreData struct {
	Applied       map[string]*Applied `json:"applied"`
	Games         []*Game             `json:"games"`
	History       []HistItem          `json:"history"`
	Boost         *BoostSession       `json:"boost,omitempty"`
	RestorePoints []string            `json:"restorePoints"`
	Saved         []string            `json:"saved"`
}

type Settings struct {
	Lang        string        `json:"lang"`
	AutoRestore bool          `json:"autoRestore"`
	Onboarded   bool          `json:"onboarded"`
	Sounds      bool          `json:"sounds"`
	Motion      bool          `json:"motion"`
	Overlay     OverlayConfig `json:"overlay"`
	Theme       string        `json:"theme"`
	Tray        bool          `json:"tray"`          // tray icon with quick actions
	TrayAuto    bool          `json:"trayAutostart"` // start the tray icon with Windows
	AutoLock    int           `json:"autoLock"`      // minutes of inactivity before locking; 0 = never
	AcctAsked   bool          `json:"acctAsked"`
	InputMode   []string      `json:"inputMode"`   // tweaks the low-input-lag switch turned on (it reverts only these)
	SeenVersion string        `json:"seenVersion"` // last version whose "What's new" was shown
	// 1.0.7
	AutoGame   AutoGameConfig  `json:"autoGame"`
	AutoClean  AutoCleanConfig `json:"autoClean"`
	UpdateRepo string          `json:"updateRepo"` // GitHub "owner/repo" with Larik releases
	UpdateAuto bool            `json:"updateAuto"` // look for a new version when Larik starts
	UpdateSeen string          `json:"updateSeen"` // last date the automatic check ran
}

// AutoGameConfig: when a game starts, Larik turns on Game Boost and shows the
// overlay, and puts both back when the game closes.
type AutoGameConfig struct {
	Enabled bool `json:"enabled"`
	Boost   bool `json:"boost"`
	Overlay bool `json:"overlay"`
}

// AutoCleanConfig: weekly removal of temporary files by a scheduled task.
type AutoCleanConfig struct {
	Enabled bool `json:"enabled"`
	Day     int  `json:"day"`  // 0 = Sunday … 6 = Saturday
	Hour    int  `json:"hour"` // 0…23
}

func (c *AutoCleanConfig) Normalize() {
	if c.Day < 0 || c.Day > 6 {
		c.Day = 0
	}
	if c.Hour < 0 || c.Hour > 23 {
		c.Hour = 12
	}
}

type Store struct {
	mu       sync.Mutex
	dir      string
	Data     StoreData
	Settings Settings
}

func OpenStore(dir string) *Store {
	_ = os.MkdirAll(dir, 0o755)
	s := &Store{dir: dir}
	s.Data.Applied = map[string]*Applied{}
	s.Settings = Settings{AutoRestore: true, Motion: true, Overlay: defaultOverlay(), Theme: "violet",
		AutoGame: AutoGameConfig{Boost: true, Overlay: true}, AutoClean: AutoCleanConfig{Hour: 12}}
	if b, err := os.ReadFile(filepath.Join(dir, "state.json")); err == nil {
		_ = json.Unmarshal(b, &s.Data)
	}
	if s.Data.Applied == nil {
		s.Data.Applied = map[string]*Applied{}
	}
	if b, err := os.ReadFile(filepath.Join(dir, "settings.json")); err == nil {
		_ = json.Unmarshal(b, &s.Settings)
	}
	s.Settings.Overlay.Normalize()
	s.Settings.AutoClean.Normalize()
	if s.Settings.UpdateRepo == "" { // 1.1.3: updates come from the official repository by default
		s.Settings.UpdateRepo = defaultUpdateRepo
		s.Settings.UpdateAuto = true
	}
	return s
}

func writeAtomic(path string, v any) error {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		return err
	}
	// Unique temp name: the engine and the overlay process may write at once.
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_ = f.Chmod(0o644)
	_, werr := f.Write(b)
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		_ = os.Remove(tmp)
		return werr
	}
	// On Windows the rename fails while another process has the target open
	// (e.g. the overlay reading settings.json), so retry briefly.
	for i := 0; ; i++ {
		if err = os.Rename(tmp, path); err == nil || i == 5 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		_ = os.Remove(tmp)
	}
	return err
}

// Save persists state; caller must hold s.mu.
func (s *Store) saveLocked() error {
	if len(s.Data.History) > 300 {
		s.Data.History = s.Data.History[len(s.Data.History)-300:]
	}
	return writeAtomic(filepath.Join(s.dir, "state.json"), &s.Data)
}

func (s *Store) Update(fn func(d *StoreData)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.Data)
	_ = s.saveLocked()
}

func (s *Store) Read(fn func(d *StoreData)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.Data)
}

func (s *Store) GetSettings() Settings {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Settings
}

func (s *Store) SetSettings(st Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Settings = st
	return writeAtomic(filepath.Join(s.dir, "settings.json"), &s.Settings)
}

func nowStr() string { return time.Now().Format(time.RFC3339) }

// logHist records a change made by one of Larik's tools in the history list.
func (e *Engine) logHist(id, action, note string) {
	e.store.Update(func(d *StoreData) {
		d.History = append(d.History, HistItem{At: nowStr(), Action: action, ID: id, OK: true, Note: note})
	})
}
