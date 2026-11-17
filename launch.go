package main

// Starting games from Larik (1.0.9). Store games start through their own
// launcher (steam://, the Epic Games launcher, the Riot Client) so sign-in,
// cloud saves, anti-cheat and updates work as usual; other games start from
// their .exe in their own folder. Larik itself runs as administrator, so the
// game is started with the user's normal rights (see launch_windows.go).

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var reLaunch = regexp.MustCompile(`^(steam:\d{1,12}|epic:[A-Za-z0-9_.\-]{1,100}(:[A-Za-z0-9_.\-]{1,100}){0,2}|riot:(valorant|league_of_legends))$`)

func cleanLaunch(s string) string {
	s = strings.TrimSpace(s)
	if reLaunch.MatchString(s) {
		return s
	}
	return ""
}

type launchPlan struct {
	Key  string // launch string to remember for the game
	URL  string // open through the shell (store launchers)
	Exe  string // or start this program
	Args []string
	Dir  string
}

var riotProduct = map[string]string{"valorant": "valorant", "league of legends": "league_of_legends"}

// riotClient finds RiotClientServices.exe from the Riot Client's own record.
func riotClient() string {
	pd := os.Getenv("ProgramData")
	if pd == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(pd, "Riot Games", "RiotClientInstalls.json"))
	if err != nil {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return ""
	}
	for _, k := range []string{"rc_default", "rc_live"} {
		if p, _ := m[k].(string); p != "" && safeExePath(winPath(p)) {
			return winPath(p)
		}
	}
	return ""
}

// launchFor decides how to start a game from Larik's list.
func (s *Server) launchFor(g *Game) (launchPlan, error) {
	l := cleanLaunch(g.Launch)
	if l == "" { // games added before 1.0.9
		if id := digitsOnly(g.SteamID); id != "" {
			l = "steam:" + id
		} else if id := s.steamIDForPath(g.Path); id != "" {
			l = "steam:" + id
		} else if g.Source == "epic" && g.Path != "" {
			for _, f := range epicGames() {
				if strings.EqualFold(f.Path, g.Path) {
					l = cleanLaunch(f.Launch)
				}
			}
		} else if g.Source == "riot" {
			low := strings.ToLower(g.Path)
			for dir, p := range riotProduct {
				if strings.Contains(low, `\riot games\`+dir+`\`) {
					l = "riot:" + p
				}
			}
		}
	}
	kind, val, _ := strings.Cut(l, ":")
	switch kind {
	case "steam":
		if !protocolRegistered("steam") {
			return launchPlan{}, errors.New("launcher_missing")
		}
		return launchPlan{Key: l, URL: "steam://rungameid/" + val}, nil
	case "epic":
		if !protocolRegistered("com.epicgames.launcher") {
			return launchPlan{}, errors.New("launcher_missing")
		}
		// namespace%3Aitem%3Aapp, as Epic's own desktop shortcuts write it
		parts := strings.Split(val, ":")
		for i := range parts {
			parts[i] = url.PathEscape(parts[i])
		}
		return launchPlan{Key: l, URL: "com.epicgames.launcher://apps/" + strings.Join(parts, "%3A") + "?action=launch&silent=true"}, nil
	case "riot":
		if rc := riotClient(); rc != "" {
			return launchPlan{Key: l, Exe: rc, Args: []string{"--launch-product=" + val, "--launch-patchline=live"}, Dir: winDir(rc)}, nil
		}
		return launchPlan{}, errors.New("launcher_missing") // the game exe alone exits without the Riot Client
	}
	if g.Source == "epic" || g.Source == "riot" {
		return launchPlan{}, errors.New("launcher_missing")
	}
	p := strings.TrimSpace(g.Path) // stored as a Windows path
	if p == "" {
		return launchPlan{}, errors.New("no_path")
	}
	if !safeExePath(p) {
		return launchPlan{}, errors.New("not_found")
	}
	return launchPlan{Exe: p, Dir: winDir(p)}, nil
}

// winDir is filepath.Dir for a Windows path on any OS (the demo runs on Linux).
func winDir(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i > 0 {
		if i == 2 && p[1] == ':' {
			return p[:3] // "C:\"
		}
		return p[:i]
	}
	return p
}

func (s *Server) launchRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/games/launch", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID    string `json:"id"`
			Admin bool   `json:"admin"` // the user agreed to start it as administrator
		}
		if err := readJSON(r, &req); err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		var g *Game
		s.e.store.Read(func(d *StoreData) {
			for _, x := range d.Games {
				if x.ID == req.ID {
					cp := *x
					g = &cp
				}
			}
		})
		if g == nil {
			errJSON(w, 404, "unknown game")
			return
		}
		plan, err := s.launchFor(g)
		if err == nil {
			if plan.URL != "" {
				err = openURLAsUser(plan.URL)
			} else {
				err = startExeAsUser(plan.Exe, plan.Args, plan.Dir, req.Admin)
			}
		}
		if err != nil {
			code := 500
			switch err.Error() {
			case "no_path", "not_found", "launcher_missing", "user_start_failed":
				code = 400
			}
			errJSON(w, code, err.Error())
			return
		}
		s.e.store.Update(func(d *StoreData) {
			for _, x := range d.Games {
				if x.ID == req.ID {
					x.LastLaunch = nowStr()
					if plan.Key != "" {
						x.Launch = plan.Key // remember what was worked out for older entries
					}
				}
			}
		})
		via := "exe"
		if plan.URL != "" {
			via, _, _ = strings.Cut(plan.URL, ":")
		} else if len(plan.Args) > 0 {
			via = "riot"
		}
		writeJSON(w, map[string]any{"ok": true, "via": via, "games": s.gameList()})
	})
}
