package main

import (
	"net/http"
	"sort"
	"strings"
)

type Proc struct {
	Name string `json:"name"`
	PID  uint32 `json:"pid"`
	Mem  uint64 `json:"mem"`
}

type BgApp struct {
	Exe     string `json:"exe"`
	Name    string `json:"name"`
	Mem     uint64 `json:"mem"`
	Count   int    `json:"count"`
	Suggest bool   `json:"suggest"`
}

// Known background apps that are safe to close before playing.
var knownBg = []struct {
	Exe, Name string
	Suggest   bool
}{
	{"OneDrive.exe", "OneDrive", true},
	{"ms-teams.exe", "Microsoft Teams", true},
	{"Teams.exe", "Microsoft Teams (classic)", true},
	{"Skype.exe", "Skype", true},
	{"PhoneExperienceHost.exe", "Phone Link", true},
	{"Widgets.exe", "Widgets", true},
	{"WidgetService.exe", "Widgets service", true},
	{"Copilot.exe", "Copilot", true},
	{"CCXProcess.exe", "Adobe Creative Cloud", true},
	{"Creative Cloud.exe", "Adobe Creative Cloud", true},
	{"AdobeCollabSync.exe", "Adobe Acrobat sync", true},
	{"GoogleDriveFS.exe", "Google Drive", true},
	{"Dropbox.exe", "Dropbox", true},
	{"iCloudServices.exe", "iCloud", true},
	{"EADesktop.exe", "EA app", false},
	{"upc.exe", "Ubisoft Connect", false},
	{"EpicGamesLauncher.exe", "Epic Games Launcher", false},
	{"Battle.net.exe", "Battle.net", false},
	{"GalaxyClient.exe", "GOG Galaxy", false},
	{"Spotify.exe", "Spotify", false},
	{"Telegram.exe", "Telegram", false},
	{"Viber.exe", "Viber", false},
	{"WhatsApp.exe", "WhatsApp", false},
	{"Zoom.exe", "Zoom", false},
	{"slack.exe", "Slack", false},
	{"Discord.exe", "Discord", false},
	{"chrome.exe", "Google Chrome", false},
	{"firefox.exe", "Firefox", false},
	{"opera.exe", "Opera", false},
	{"brave.exe", "Brave", false},
}

// Services paused during a Game Boost session.
var boostServices = []string{"WSearch", "SysMain", "DiagTrack", "wuauserv", "BITS", "DoSvc"}

func backgroundApps() []BgApp {
	procs := listProcesses()
	byExe := map[string]*BgApp{}
	for _, k := range knownBg {
		for _, p := range procs {
			if strings.EqualFold(p.Name, k.Exe) {
				b := byExe[k.Exe]
				if b == nil {
					b = &BgApp{Exe: k.Exe, Name: k.Name, Suggest: k.Suggest}
					byExe[k.Exe] = b
				}
				b.Mem += p.Mem
				b.Count++
			}
		}
	}
	var out []BgApp
	for _, b := range byExe {
		out = append(out, *b)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Mem > out[j].Mem })
	return out
}

func isKnownBg(exe string) bool {
	for _, k := range knownBg {
		if strings.EqualFold(k.Exe, exe) {
			return true
		}
	}
	return false
}

func (s *Server) boostRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/boost", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"session": loadBoost(), "bg": backgroundApps()})
	})

	mux.HandleFunc("POST /api/boost/ram", func(w http.ResponseWriter, r *http.Request) {
		before, after, err := cleanMemory()
		if err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"before": before, "after": after})
	})

	mux.HandleFunc("GET /api/boost/temp", func(w http.ResponseWriter, r *http.Request) {
		b, n := tempScan()
		writeJSON(w, map[string]any{"bytes": b, "files": n})
	})

	mux.HandleFunc("POST /api/boost/temp", func(w http.ResponseWriter, r *http.Request) {
		b, n := tempClean()
		writeJSON(w, map[string]any{"bytes": b, "files": n})
	})

	mux.HandleFunc("POST /api/boost/dns", func(w http.ResponseWriter, r *http.Request) {
		if _, err := s.e.sys.Exec("ipconfig", "/flushdns"); err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})

	mux.HandleFunc("POST /api/boost/kill", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Exes []string `json:"exes"`
		}
		if err := readJSON(r, &req); err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		n := 0
		for _, exe := range req.Exes {
			if isKnownBg(exe) {
				if _, err := s.e.sys.Exec("taskkill", "/IM", exe, "/F", "/T"); err == nil {
					n++
				}
			}
		}
		writeJSON(w, map[string]any{"closed": n, "bg": backgroundApps()})
	})

	mux.HandleFunc("POST /api/boost/session", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			On bool `json:"on"`
		}
		if err := readJSON(r, &req); err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		if !s.e.admin {
			errJSON(w, 403, errNeedAdmin.Error())
			return
		}
		s.e.mu.Lock()
		defer s.e.mu.Unlock()
		sess, err := setBoost(s.e.sys, req.On)
		if err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"session": sess})
	})
}
