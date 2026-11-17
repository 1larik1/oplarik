package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const ifeoBase = `HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Image File Execution Options`
const gpuPrefKey = `HKCU\Software\Microsoft\DirectX\UserGpuPreferences`
const layersKey = `HKCU\Software\Microsoft\Windows NT\CurrentVersion\AppCompatFlags\Layers`
const fsoFlag = "DISABLEDXMAXIMIZEDWINDOWEDMODE"

type PopularGame struct {
	Name string `json:"name"`
	Exe  string `json:"exe"`
}

var popularGames = []PopularGame{
	{"Counter-Strike 2", "cs2.exe"},
	{"VALORANT", "VALORANT-Win64-Shipping.exe"},
	{"Fortnite", "FortniteClient-Win64-Shipping.exe"},
	{"Apex Legends", "r5apex_dx12.exe"},
	{"Dota 2", "dota2.exe"},
	{"League of Legends", "League of Legends.exe"},
	{"Grand Theft Auto V", "GTA5.exe"},
	{"Rocket League", "RocketLeague.exe"},
	{"Overwatch 2", "Overwatch.exe"},
	{"Rainbow Six Siege", "RainbowSix.exe"},
	{"PUBG: Battlegrounds", "TslGame.exe"},
	{"Call of Duty", "cod.exe"},
	{"Roblox", "RobloxPlayerBeta.exe"},
	{"Rust", "RustClient.exe"},
	{"Deadlock", "project8.exe"},
	{"Marvel Rivals", "Marvel-Win64-Shipping.exe"},
	{"Escape from Tarkov", "EscapeFromTarkov.exe"},
	{"World of Tanks", "WorldOfTanks.exe"},
	{"Genshin Impact", "GenshinImpact.exe"},
	{"Minecraft (Bedrock)", "Minecraft.Windows.exe"},
	{"The Finals", "Discovery.exe"},
	{"Battlefield 2042", "BF2042.exe"},
	{"Warframe", "Warframe.x64.exe"},
	{"Path of Exile 2", "PathOfExile.exe"},
}

type FoundGame struct {
	Name    string `json:"name"`
	Exe     string `json:"exe"`
	Path    string `json:"path"`
	Source  string `json:"source"`
	SteamID string `json:"steamId,omitempty"`
	Cover   string `json:"cover,omitempty"`  // picture URL (1.0.8)
	Launch  string `json:"launch,omitempty"` // how to start it: steam:<id> | epic:<ns>:<item>:<app> | riot:<product> (1.0.9)
}

func validExeName(s string) bool {
	if s == "" || len(s) > 120 || !strings.HasSuffix(strings.ToLower(s), ".exe") {
		return false
	}
	return !strings.ContainsAny(s, `\/:*?"<>|`)
}

func (s *Server) gameList() []*Game {
	out := []*Game{}
	s.e.store.Read(func(d *StoreData) {
		for _, g := range d.Games {
			cp := *g
			cp.B = nil
			out = append(out, &cp)
		}
	})
	for _, g := range out {
		id := g.SteamID
		if id == "" {
			id = s.steamIDForPath(g.Path)
		}
		g.Cover = s.coverURL(id, g.Path)
	}
	return out
}

func (s *Server) gameRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/games", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"games": s.gameList(), "popular": popularGames})
	})

	mux.HandleFunc("POST /api/games/detect", func(w http.ResponseWriter, r *http.Request) {
		found := platformDetectGames(s.e.sys)
		for i := range found {
			found[i].Cover = s.coverURL(found[i].SteamID, found[i].Path)
		}
		writeJSON(w, map[string]any{"found": orEmpty(found)})
	})

	mux.HandleFunc("POST /api/games/browse", func(w http.ResponseWriter, r *http.Request) {
		p, err := browseExe()
		if err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]string{"path": p, "exe": filepath.Base(winPath(p)), "name": strings.TrimSuffix(filepath.Base(winPath(p)), filepath.Ext(p))})
	})

	mux.HandleFunc("POST /api/games/add", func(w http.ResponseWriter, r *http.Request) {
		var req FoundGame
		if err := readJSON(r, &req); err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		if req.Exe == "" && req.Path != "" {
			req.Exe = filepath.Base(winPath(req.Path))
		}
		if !validExeName(req.Exe) {
			errJSON(w, 400, "invalid exe name")
			return
		}
		if req.Path != "" && (!strings.HasSuffix(strings.ToLower(req.Path), ".exe") || len(req.Path) > 400) {
			errJSON(w, 400, "invalid path")
			return
		}
		if req.Name == "" {
			req.Name = strings.TrimSuffix(req.Exe, filepath.Ext(req.Exe))
		}
		dup := false
		s.e.store.Update(func(d *StoreData) {
			for _, g := range d.Games {
				if strings.EqualFold(g.Exe, req.Exe) && (req.Path == "" || strings.EqualFold(g.Path, req.Path) || g.Path == "") {
					if g.Path == "" && req.Path != "" {
						g.Path = req.Path
					}
					if l := cleanLaunch(req.Launch); l != "" {
						g.Launch = l
					}
					if id := digitsOnly(req.SteamID); id != "" {
						g.SteamID = id
					}
					dup = true
					return
				}
			}
			d.Games = append(d.Games, &Game{
				ID: "g" + strconv.FormatInt(time.Now().UnixNano(), 36), Name: req.Name, Exe: req.Exe,
				Path: req.Path, Source: nonEmpty(req.Source, "manual"), Added: nowStr(), SteamID: digitsOnly(req.SteamID),
				Launch: cleanLaunch(req.Launch),
			})
		})
		_ = dup
		writeJSON(w, map[string]any{"games": s.gameList()})
	})

	mux.HandleFunc("POST /api/games/update", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID   string `json:"id"`
			Prio *bool  `json:"prio"`
			GPU  *bool  `json:"gpu"`
			FSO  *bool  `json:"fso"`
		}
		if err := readJSON(r, &req); err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		if !s.e.admin {
			errJSON(w, 403, errNeedAdmin.Error())
			return
		}
		var err error
		s.e.mu.Lock()
		s.e.store.Update(func(d *StoreData) {
			for _, g := range d.Games {
				if g.ID != req.ID {
					continue
				}
				if g.B == nil {
					g.B = map[string]json.RawMessage{}
				}
				if req.Prio != nil && *req.Prio != g.Prio {
					err = setGamePrio(s.e.sys, g, *req.Prio)
				}
				if err == nil && req.GPU != nil && *req.GPU != g.GPU {
					err = setGamePathFlag(s.e.sys, g, "gpu", *req.GPU)
				}
				if err == nil && req.FSO != nil && *req.FSO != g.FSO {
					err = setGamePathFlag(s.e.sys, g, "fso", *req.FSO)
				}
			}
		})
		s.e.mu.Unlock()
		if err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"games": s.gameList()})
	})

	mux.HandleFunc("POST /api/games/remove", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"id"`
		}
		if err := readJSON(r, &req); err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		var err error
		s.e.mu.Lock()
		s.e.store.Update(func(d *StoreData) {
			var keep []*Game
			for _, g := range d.Games {
				if g.ID != req.ID {
					keep = append(keep, g)
					continue
				}
				if g.Prio {
					err = setGamePrio(s.e.sys, g, false)
				}
				if g.GPU {
					err = errors.Join(err, setGamePathFlag(s.e.sys, g, "gpu", false))
				}
				if g.FSO {
					err = errors.Join(err, setGamePathFlag(s.e.sys, g, "fso", false))
				}
			}
			d.Games = keep
		})
		s.e.mu.Unlock()
		if err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"games": s.gameList()})
	})
}

type prioBackup struct {
	KeyExisted  bool   `json:"keyExisted"`
	PerfExisted bool   `json:"perfExisted"`
	Val         RegVal `json:"val"`
}

func setGamePrio(sys Sys, g *Game, on bool) error {
	if g.B == nil {
		g.B = map[string]json.RawMessage{}
	}
	exeKey := ifeoBase + `\` + g.Exe
	perf := exeKey + `\PerfOptions`
	if on {
		b := prioBackup{KeyExisted: sys.RegKeyExists(exeKey), PerfExisted: sys.RegKeyExists(perf)}
		b.Val, _ = sys.RegGet(perf, "CpuPriorityClass")
		if err := sys.RegCreateKey(perf); err != nil {
			return err
		}
		if err := sys.RegSet(perf, "CpuPriorityClass", DwordVal(3)); err != nil {
			return err
		}
		g.B["prio"] = mustJSON(b)
		g.Prio = true
		return nil
	}
	var b prioBackup
	if raw, ok := g.B["prio"]; ok {
		_ = json.Unmarshal(raw, &b)
	} else {
		b = prioBackup{KeyExisted: true, PerfExisted: true, Val: RegVal{Absent: true}}
	}
	var err error
	if !b.PerfExisted {
		err = sys.RegDeleteTree(perf)
		if !b.KeyExisted {
			if subs, e2 := sys.RegSubkeys(exeKey); e2 == nil && len(subs) == 0 {
				_ = sys.RegDeleteTree(exeKey)
			}
		}
	} else if b.Val.Absent {
		err = sys.RegDelete(perf, "CpuPriorityClass")
	} else {
		err = sys.RegSet(perf, "CpuPriorityClass", b.Val)
	}
	if err != nil {
		return err
	}
	delete(g.B, "prio")
	g.Prio = false
	return nil
}

func setGamePathFlag(sys Sys, g *Game, flag string, on bool) error {
	if g.B == nil {
		g.B = map[string]json.RawMessage{}
	}
	if g.Path == "" {
		return errors.New("full game path required: use Browse to pick the .exe")
	}
	key := gpuPrefKey
	if flag == "fso" {
		key = layersKey
	}
	if on {
		cur, _ := sys.RegGet(key, g.Path)
		var val string
		if flag == "gpu" {
			str, _ := cur.Str()
			keys, m := parseKV(str)
			if _, ok := m["GpuPreference"]; !ok {
				keys = append(keys, "GpuPreference")
			}
			m["GpuPreference"] = "2"
			val = buildKV(keys, m)
		} else {
			str, _ := cur.Str()
			fields := strings.Fields(str)
			has := false
			var rest []string
			for _, f := range fields {
				if f == "~" {
					continue
				}
				if strings.EqualFold(f, fsoFlag) {
					has = true
				}
				rest = append(rest, f)
			}
			if !has {
				rest = append(rest, fsoFlag)
			}
			val = "~ " + strings.Join(rest, " ")
		}
		if err := sys.RegCreateKey(key); err != nil {
			return err
		}
		if err := sys.RegSet(key, g.Path, StrVal(val)); err != nil {
			return err
		}
		if _, ok := g.B[flag]; !ok {
			g.B[flag] = mustJSON(cur)
		}
		if flag == "gpu" {
			g.GPU = true
		} else {
			g.FSO = true
		}
		return nil
	}
	v := RegVal{Absent: true}
	if raw, ok := g.B[flag]; ok {
		_ = json.Unmarshal(raw, &v)
	}
	var err error
	if v.Absent {
		err = sys.RegDelete(key, g.Path)
	} else {
		err = sys.RegSet(key, g.Path, v)
	}
	if err != nil {
		return err
	}
	delete(g.B, flag)
	if flag == "gpu" {
		g.GPU = false
	} else {
		g.FSO = false
	}
	return nil
}

// ---------------------------------------------------------------- detection

var (
	reVdfPath = regexp.MustCompile(`"path"\s+"([^"]+)"`)
	reAcfName = regexp.MustCompile(`"name"\s+"([^"]+)"`)
	reAcfDir  = regexp.MustCompile(`"installdir"\s+"([^"]+)"`)
	reAcfID   = regexp.MustCompile(`"appid"\s+"(\d+)"`)
)

// winPath normalizes Windows paths so filepath works in tests on Linux too.
func winPath(p string) string {
	if filepath.Separator == '/' {
		return strings.ReplaceAll(p, `\`, "/")
	}
	return strings.ReplaceAll(p, "/", `\`)
}

func detectGames(sys Sys) []FoundGame {
	var out []FoundGame
	seen := map[string]bool{}
	add := func(f FoundGame) {
		k := strings.ToLower(f.Path)
		if f.Path == "" || seen[k] {
			return
		}
		seen[k] = true
		out = append(out, f)
	}
	for _, f := range steamGames(sys) {
		add(f)
	}
	for _, f := range epicGames() {
		add(f)
	}
	for _, f := range knownPathGames() {
		add(f)
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// steamRoot is the Steam folder from the registry ("" = not installed).
func steamRoot(sys Sys) string {
	if demoSteamRoot != "" {
		return demoSteamRoot
	}
	var root string
	if v, err := sys.RegGet(`HKCU\Software\Valve\Steam`, "SteamPath"); err == nil {
		root, _ = v.Str()
	}
	if root == "" {
		if v, err := sys.RegGet(`HKLM\SOFTWARE\WOW6432Node\Valve\Steam`, "InstallPath"); err == nil {
			root, _ = v.Str()
		}
	}
	if root == "" {
		return ""
	}
	return winPath(root)
}

var demoSteamRoot string

func steamGames(sys Sys) []FoundGame {
	root := steamRoot(sys)
	if root == "" {
		return nil
	}
	libs := []string{root}
	if b, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf")); err == nil {
		for _, m := range reVdfPath.FindAllStringSubmatch(string(b), -1) {
			p := winPath(strings.ReplaceAll(m[1], `\\`, `\`))
			if !strings.EqualFold(filepath.Clean(p), filepath.Clean(root)) {
				libs = append(libs, p)
			}
		}
	}
	var out []FoundGame
	for _, lib := range libs {
		files, _ := filepath.Glob(filepath.Join(lib, "steamapps", "appmanifest_*.acf"))
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			txt := string(b)
			name, dir := first(reAcfName, txt), first(reAcfDir, txt)
			id := first(reAcfID, txt)
			if name == "" || dir == "" || id == "228980" || strings.Contains(name, "Proton") ||
				strings.Contains(name, "Steam Linux Runtime") || strings.Contains(name, "Steamworks") || strings.Contains(name, "Redistributable") {
				continue
			}
			gameDir := filepath.Join(lib, "steamapps", "common", dir)
			if exe := guessExe(gameDir); exe != "" {
				out = append(out, FoundGame{Name: name, Exe: filepath.Base(exe), Path: toWin(exe), Source: "steam", SteamID: id, Launch: "steam:" + id})
			}
		}
	}
	return out
}

func first(re *regexp.Regexp, s string) string {
	if m := re.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

func toWin(p string) string {
	if filepath.Separator == '/' {
		return p
	}
	return strings.ReplaceAll(p, "/", `\`)
}

func epicGames() []FoundGame {
	pd := os.Getenv("ProgramData")
	if pd == "" {
		return nil
	}
	files, _ := filepath.Glob(filepath.Join(pd, "Epic", "EpicGamesLauncher", "Data", "Manifests", "*.item"))
	var out []FoundGame
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var m struct {
			DisplayName      string   `json:"DisplayName"`
			InstallLocation  string   `json:"InstallLocation"`
			LaunchExecutable string   `json:"LaunchExecutable"`
			AppCategories    []string `json:"AppCategories"`
			CatalogNamespace string   `json:"CatalogNamespace"`
			CatalogItemID    string   `json:"CatalogItemId"`
			AppName          string   `json:"AppName"`
			MainGameAppName  string   `json:"MainGameAppName"`
		}
		if json.Unmarshal(b, &m) != nil || m.LaunchExecutable == "" || m.InstallLocation == "" {
			continue
		}
		isGame := len(m.AppCategories) == 0
		for _, c := range m.AppCategories {
			if c == "games" {
				isGame = true
			}
		}
		if !isGame {
			continue
		}
		exe := filepath.Join(winPath(m.InstallLocation), winPath(m.LaunchExecutable))
		// Unreal titles launch a small bootstrapper; prefer the shipping binary.
		if real := guessExe(filepath.Dir(exe)); real != "" && strings.Contains(strings.ToLower(real), "-shipping.exe") {
			exe = real
		}
		if _, err := os.Stat(exe); err != nil {
			continue
		}
		app := m.MainGameAppName
		if app == "" {
			app = m.AppName
		}
		launch := ""
		if app != "" {
			launch = "epic:" + app
			if m.CatalogNamespace != "" && m.CatalogItemID != "" {
				launch = "epic:" + m.CatalogNamespace + ":" + m.CatalogItemID + ":" + app
			}
		}
		out = append(out, FoundGame{Name: m.DisplayName, Exe: filepath.Base(exe), Path: toWin(exe), Source: "epic", Launch: launch})
	}
	return out
}

func knownPathGames() []FoundGame {
	var out []FoundGame
	cands := []FoundGame{
		{Name: "VALORANT", Path: `C:\Riot Games\VALORANT\live\ShooterGame\Binaries\Win64\VALORANT-Win64-Shipping.exe`, Source: "riot", Launch: "riot:valorant"},
		{Name: "League of Legends", Path: `C:\Riot Games\League of Legends\Game\League of Legends.exe`, Source: "riot", Launch: "riot:league_of_legends"},
	}
	for _, c := range cands {
		if _, err := os.Stat(winPath(c.Path)); err == nil {
			c.Exe = filepath.Base(winPath(c.Path))
			out = append(out, c)
		}
	}
	return out
}

var exeSkip = []string{"unins", "crash", "report", "redist", "setup", "install", "vc_", "dxsetup", "easyanticheat",
	"beservice", "battleye", "updater", "helper", "unitycrashhandler", "cefprocess", "webhelper", "dotnet", "python",
	"7z", "touchup", "cleanup", "vcredist", "oalinst", "physx", "uploader", "error", "_be.exe", "_eac", "prereq",
	"launcherpatcher", "notification", "overlay", "benchmark", "editor", "server", "dedicated", "mono", "openssl"}

// guessExe picks the most likely game binary inside a folder.
func guessExe(dir string) string {
	type cand struct {
		path  string
		size  int64
		score int
	}
	var cs []cand
	base := strings.Count(filepath.Clean(dir), string(filepath.Separator))
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		depth := strings.Count(filepath.Clean(p), string(filepath.Separator)) - base
		if d.IsDir() {
			if depth > 5 {
				return filepath.SkipDir
			}
			return nil
		}
		low := strings.ToLower(d.Name())
		if !strings.HasSuffix(low, ".exe") {
			return nil
		}
		for _, sk := range exeSkip {
			if strings.Contains(low, sk) {
				return nil
			}
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		sc := 0
		if strings.HasSuffix(low, "-shipping.exe") {
			sc += 100
		}
		if strings.Contains(strings.ToLower(p), "win64") || strings.Contains(strings.ToLower(p), "x64") {
			sc += 10
		}
		if strings.Contains(low, "launcher") {
			sc -= 30
		}
		cs = append(cs, cand{p, info.Size(), sc})
		return nil
	})
	if len(cs) == 0 {
		return ""
	}
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].score != cs[j].score {
			return cs[i].score > cs[j].score
		}
		return cs[i].size > cs[j].size
	})
	return cs[0].path
}
