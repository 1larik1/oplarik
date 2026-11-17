//go:build !windows

package main

import (
	"strings"
	"testing"
)

func TestLaunchPlans(t *testing.T) {
	cat, _ := LoadCatalog(catalogJSON)
	s := &Server{e: &Engine{sys: newMockSys(cat)}}
	p, err := s.launchFor(&Game{Launch: "steam:730", Path: `D:\x\cs2.exe`})
	if err != nil || p.URL != "steam://rungameid/730" {
		t.Fatalf("%+v %v", p, err)
	}
	p, _ = s.launchFor(&Game{Launch: "epic:fn:4fe75bbc:Fortnite"})
	if p.URL != "com.epicgames.launcher://apps/fn%3A4fe75bbc%3AFortnite?action=launch&silent=true" {
		t.Fatal(p.URL)
	}
	// Riot without the Riot Client: the game exe alone would just exit.
	if _, err = s.launchFor(&Game{Launch: "riot:valorant", Source: "riot", Path: `C:\Riot Games\VALORANT\live\VALORANT.exe`}); err == nil || err.Error() != "launcher_missing" {
		t.Fatalf("%v", err)
	}
	if _, err = s.launchFor(&Game{Source: "epic", Path: `C:\Games\Fortnite.exe`}); err == nil || err.Error() != "launcher_missing" {
		t.Fatalf("%v", err)
	}
	p, err = s.launchFor(&Game{Source: "manual", Path: `C:\Games\Old\game.exe`})
	if err != nil || p.Exe != `C:\Games\Old\game.exe` || p.Dir != `C:\Games\Old` {
		t.Fatalf("%+v %v", p, err)
	}
	if _, err := s.launchFor(&Game{Exe: "game.exe"}); err == nil || err.Error() != "no_path" {
		t.Fatal("no path accepted")
	}
	if _, err := s.launchFor(&Game{Path: `\\srv\share\game.exe`}); err == nil {
		t.Fatal("network path accepted")
	}
	// A bad launch string is ignored, the exe is used instead.
	p, _ = s.launchFor(&Game{Launch: "steam:730&evil", Path: `C:\Games\a.exe`})
	if p.URL != "" || p.Exe != `C:\Games\a.exe` {
		t.Fatalf("%+v", p)
	}
	for _, bad := range []string{"steam:12a", "epic:a b", "riot:other", "file:c:/x", "steam://x"} {
		if cleanLaunch(bad) != "" {
			t.Errorf("%q accepted", bad)
		}
	}
	if !strings.HasPrefix(cleanLaunch("epic:ns:item:App"), "epic:") {
		t.Fatal("epic triple refused")
	}
}
