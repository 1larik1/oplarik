package main

import (
	"encoding/json"
	"os"
	"testing"
)

var langs111 = []string{"en", "ru", "uk", "pl", "es", "de", "fr", "ja", "cs"}

func TestNativeTextAllLanguages(t *testing.T) {
	for _, l := range langs111 {
		m, ok := nativeText[l]
		if !ok {
			t.Fatalf("no native texts for %s", l)
		}
		for k := range nativeText["en"] {
			if m[k] == "" {
				t.Errorf("%s: missing %s", l, k)
			}
		}
	}
}

func TestOverlayLabelsDrawable(t *testing.T) {
	if err := loadAtlas(); err != nil {
		t.Fatal(err)
	}
	for _, l := range langs111 {
		for k := range overlayLabels["en"] {
			s := olabel(l, k)
			for _, sc := range atlasScales {
				for _, f := range []*atlasFont{sc.Label, sc.Value} {
					for _, r := range s {
						if _, ok := f.G[r]; !ok {
							t.Fatalf("%s %s: glyph %q not in atlas", l, k, r)
						}
					}
				}
			}
		}
	}
}

func TestCatalogAllLanguages(t *testing.T) {
	b, err := os.ReadFile("catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		Tweaks []struct {
			ID         string
			Name, Desc map[string]string
		}
	}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	for _, tw := range c.Tweaks {
		for _, l := range langs111 {
			if tw.Name[l] == "" || tw.Desc[l] == "" {
				t.Errorf("%s: no %s text", tw.ID, l)
			}
		}
	}
}

func TestSettingsLangAndMotion(t *testing.T) {
	for _, l := range langs111 {
		st := Settings{Lang: l}
		st = normalizeSettings(st)
		if st.Lang != l || !st.Motion {
			t.Errorf("%s: got lang %q motion %v", l, st.Lang, st.Motion)
		}
	}
	st := Settings{Lang: "xx"}
	st = normalizeSettings(st)
	if st.Lang != "en" {
		t.Errorf("bad lang kept: %q", st.Lang)
	}
}

func TestThemesHavePalettes(t *testing.T) {
	if len(themeNames) != 14 {
		t.Fatalf("themes: %d", len(themeNames))
	}
	light := 0
	for _, n := range themeNames {
		p, ok := overlayPalettes[n]
		if !ok {
			t.Fatalf("no overlay palette for %s", n)
		}
		if p.light {
			light++
		}
		if !validTheme(n) {
			t.Errorf("%s not valid", n)
		}
	}
	if light != 7 {
		t.Errorf("light themes: %d", light)
	}
	if validTheme("nope") || overlayPalette("nope") != overlayPalettes["violet"] {
		t.Error("unknown theme")
	}
}

func TestDefaultUpdateRepo113(t *testing.T) {
	st := OpenStore(t.TempDir())
	if st.Settings.UpdateRepo != defaultUpdateRepo || !st.Settings.UpdateAuto {
		t.Fatalf("defaults: %q %v", st.Settings.UpdateRepo, st.Settings.UpdateAuto)
	}
	if n := normalizeSettings(Settings{UpdateRepo: "not a repo"}); n.UpdateRepo != defaultUpdateRepo {
		t.Fatalf("normalize: %q", n.UpdateRepo)
	}
	if n := normalizeSettings(Settings{UpdateRepo: "me/fork"}); n.UpdateRepo != "me/fork" {
		t.Fatalf("custom repo lost: %q", n.UpdateRepo)
	}
}
