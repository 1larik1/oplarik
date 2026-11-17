package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSessionStats(t *testing.T) {
	var fps []float64
	for i := 0; i < 1000; i++ {
		fps = append(fps, 100)
	}
	fps[10], fps[20] = 30, 40
	st := sessionStats(fps)
	if st.Median != 100 || st.MinFPS != 30 || st.MaxFPS != 100 || st.Dips != 2 || st.Seconds != 1000 {
		t.Fatalf("stats %+v", st)
	}
	if st.Low01 != 30 || st.Low1 != 100 || st.Stability != 100 {
		t.Fatalf("lows %+v", st)
	}
	if len(downsample(fps, 120)) != 120 || len(downsample(fps[:50], 120)) != 50 {
		t.Fatal("downsample length")
	}
}

func TestReadSessionFPSAndPair(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "Sessions"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, "Sessions", "a.csv"), []byte("\xef\xbb\xbftime,fps,low\n1,100,1\n2,,1\n3,abc,1\n4,120.5,1\n"), 0o644)
	got := readSessionFPS(dir, "a.csv")
	if len(got) != 2 || got[1] != 120.5 {
		t.Fatalf("fps %v", got)
	}
	if readSessionFPS(dir, `..\x.csv`) != nil || readSessionFPS(dir, "../a.csv") != nil {
		t.Fatal("path escape accepted")
	}
	at := func(h int) string { return time.Date(2026, 10, 1, h, 0, 0, 0, time.UTC).Format(time.RFC3339) }
	list := []SessionSummary{ // newest first
		{ID: "4", Exe: "cs2.exe", Start: at(20), End: at(21)},
		{ID: "3", Exe: "dota2.exe", Start: at(18), End: at(19)},
		{ID: "2", Exe: "cs2.exe", Start: at(15), End: at(16)},
		{ID: "1", Exe: "cs2.exe", Start: at(10), End: at(11)},
	}
	if a, b := pickPair(list, nil); a != "2" || b != "4" {
		t.Fatalf("no history: %s %s", a, b)
	}
	hist := []HistItem{{At: at(12), ID: "game_mode", Action: "apply", OK: true}, {At: at(13), ID: "x", Action: "apply", OK: false}}
	if a, b := pickPair(list, hist); a != "1" || b != "4" {
		t.Fatalf("with history: %s %s", a, b)
	}
	if ch := changesBetween(hist, list[3], list[0]); len(ch) != 1 || ch[0].ID != "game_mode" {
		t.Fatalf("changes %v", ch)
	}
	if a, _ := pickPair(list[1:2], nil); a != "" {
		t.Fatal("single session paired")
	}
}

func TestAnalyzeDisk(t *testing.T) {
	ok := analyzeDisk(DiskHealth{Media: "SSD", Health: "Healthy", Temp: 40, Wear: 10})
	if ok.Level != "good" || len(ok.Issues) != 0 {
		t.Fatalf("%+v", ok)
	}
	w := analyzeDisk(DiskHealth{Media: "HDD", Health: "Healthy", Temp: 52, Wear: -1, ReadErr: 2})
	if w.Level != "warn" || strings.Join(w.Issues, ",") != "warm,errors" {
		t.Fatalf("%+v", w)
	}
	b := analyzeDisk(DiskHealth{Media: "SSD", Health: "Healthy", Temp: -1, Wear: 95, Predict: true})
	if b.Level != "bad" || strings.Join(b.Issues, ",") != "predict,wearBad" {
		t.Fatalf("%+v", b)
	}
}

func TestDriverVersions(t *testing.T) {
	cases := map[string]string{"31.0.15.3623": "536.23", "32.0.15.6094": "560.94", "32.0.15.7602": "576.02", "30.0.14.9649": "496.49", "bad": "", "1.2.3.x": ""}
	for in, want := range cases {
		if got := nvDriverVersion(in); got != want {
			t.Errorf("%s: %q want %q", in, got, want)
		}
	}
	if !verLess("536.23", "581.57") || verLess("581.57", "581.57") || !verLess("581.08", "581.57") || verLess("590.10", "581.99") {
		t.Fatal("verLess")
	}
	for _, n := range []string{"NVIDIA GeForce GTX 1060 6GB", "NVIDIA GeForce GTX 970", "NVIDIA TITAN Xp", "NVIDIA GeForce MX250", "NVIDIA GeForce GT 1030", "NVIDIA GeForce GTX 750 Ti", "NVIDIA GeForce 940MX", "NVIDIA GeForce GTX 980M", "NVIDIA GeForce 930M"} {
		if !nvLegacy(n) {
			t.Errorf("%s should be legacy", n)
		}
	}
	for _, n := range []string{"NVIDIA GeForce GTX 1650", "NVIDIA GeForce GTX 1660 SUPER", "NVIDIA GeForce RTX 3060", "NVIDIA GeForce MX450", "NVIDIA GeForce RTX 4090", "NVIDIA GeForce RTX 3080 Laptop GPU", "NVIDIA GeForce RTX 5070"} {
		if nvLegacy(n) {
			t.Errorf("%s should be current", n)
		}
	}
	l, err := parseNVLookup([]byte(`{"Success":"1","IDS":[{"downloadInfo":{"Version":"581.57","ReleaseDateTime":"Tue%20Oct%2014%2C%202025","DownloadURL":"https://us.download.nvidia.com/Windows/581.57/581.57-desktop.exe"}}]}`))
	if err != nil || l.Version != "581.57" || l.Date != "Tue Oct 14, 2025" || !strings.HasPrefix(l.URL, "https://us.download.nvidia.com/") {
		t.Fatalf("%+v %v", l, err)
	}
	l, err = parseNVLookup([]byte(`{"IDS":[{"downloadInfo":{"Version":"581.57","DownloadURL":"https://evil.example/x.exe"}}]}`))
	if err != nil || l.URL != "" {
		t.Fatal("foreign download host accepted")
	}
	if _, err := parseNVLookup([]byte(`{"IDS":[{"downloadInfo":{"Version":"<script>"}}]}`)); err == nil {
		t.Fatal("bad version accepted")
	}
}

func TestNetHelpers(t *testing.T) {
	p := pingStats("x", "1.1.1.1", []float64{10, 12, -1, 14})
	if p.Lost != 1 || p.Sent != 4 || p.Avg != 12 || p.Min != 10 || p.Max != 14 || p.Jitter != 2 {
		t.Fatalf("%+v", p)
	}
	if q := pingStats("x", "y", []float64{-1, -1}); q.Avg != -1 || q.Lost != 2 {
		t.Fatalf("%+v", q)
	}
	for extra, g := range map[float64]string{-1: "", 2: "A+", 20: "A", 45: "B", 100: "C", 300: "D", 900: "F"} {
		if bloatGrade(extra) != g {
			t.Errorf("%v -> %s", extra, bloatGrade(extra))
		}
	}
}

func TestNextWeekly(t *testing.T) {
	sat := time.Date(2026, 10, 10, 13, 0, 0, 0, time.UTC) // Saturday
	if n := nextWeekly(sat, 6, 21); !n.Equal(time.Date(2026, 10, 10, 21, 0, 0, 0, time.UTC)) {
		t.Fatal(n)
	}
	if n := nextWeekly(sat, 6, 12); !n.Equal(time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)) {
		t.Fatal(n)
	}
	if n := nextWeekly(sat, 0, 12); !n.Equal(time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)) {
		t.Fatal(n)
	}
}

func TestGameWatch(t *testing.T) {
	var g gameWatch
	alive := true
	isAlive := func(uint32) bool { return alive }
	now := time.Now()
	if g.Tick(now, 42, "cs2.exe", true, isAlive) != "" {
		t.Fatal("started at once")
	}
	if g.Tick(now.Add(3*time.Second), 42, "cs2.exe", false, isAlive) != "" || g.candPID != 0 {
		t.Fatal("not reset when not eligible")
	}
	g.Tick(now.Add(4*time.Second), 42, "cs2.exe", true, isAlive)
	if g.Tick(now.Add(13*time.Second), 42, "cs2.exe", true, isAlive) != "start" || g.Game() != "cs2.exe" {
		t.Fatal("no start")
	}
	if g.Tick(now.Add(20*time.Second), 7, "chrome.exe", false, isAlive) != "" {
		t.Fatal("alt-tab ended the game")
	}
	alive = false
	if g.Tick(now.Add(30*time.Second), 7, "explorer.exe", false, isAlive) != "end" || g.Game() != "" {
		t.Fatal("no end")
	}
	// ignored programs never start a session
	var h gameWatch
	h.Tick(now, 9, "chrome.exe", true, isAlive)
	if h.Tick(now.Add(time.Minute), 9, "chrome.exe", true, isAlive) != "" {
		t.Fatal("browser counted as game")
	}
}

func TestProfiles(t *testing.T) {
	c := defaultOverlay()
	c.Profiles = []OverlayProfile{
		{Exe: "CS2.exe", Metrics: []string{"fps", "bogus", "ping"}, Position: "bl", Layout: "column", Size: "x"},
		{Exe: "cs2.exe", Position: "tr"},     // duplicate
		{Exe: `..\evil.exe`, Position: "tr"}, // bad name
		{Exe: "dota2.exe", Name: "  ", Metrics: nil},
	}
	c.Normalize()
	if len(c.Profiles) != 2 {
		t.Fatalf("%+v", c.Profiles)
	}
	p := c.Profiles[0]
	if p.Exe != "cs2.exe" || strings.Join(p.Metrics, ",") != "fps,ping" || p.Position != "bl" || p.Layout != "column" || p.Size != "m" || p.Name != "Counter-Strike 2" {
		t.Fatalf("%+v", p)
	}
	if c.Profiles[1].Name != "Dota 2" || c.Profiles[1].Metrics == nil {
		t.Fatalf("%+v", c.Profiles[1])
	}
	g, ok := c.ForGame("CS2.EXE")
	if !ok || g.Position != "bl" || g.Opacity != c.Opacity || len(g.Metrics) != 2 {
		t.Fatalf("%+v", g)
	}
	if _, ok := c.ForGame("other.exe"); ok {
		t.Fatal("profile for unknown game")
	}
	if _, err := RenderOverlay(g, naData(), "en", 1); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateHelpers(t *testing.T) {
	for in, want := range map[string]string{"me/larik": "me/larik", "https://github.com/me/larik.git": "me/larik", "github.com/me/larik/": "me/larik",
		"me": "", "me/la rik": "", "../../x": "", "https://evil.com/a/b": ""} {
		if got := cleanRepo(in); got != want {
			t.Errorf("%q -> %q want %q", in, got, want)
		}
	}
	if !semverNewer("v1.0.8", "1.0.7") || semverNewer("1.0.7", "1.0.7") || !semverNewer("1.1.0", "1.0.9") || semverNewer("v1.0.6", "1.0.7") {
		t.Fatal("semver")
	}
	exe := []byte("MZ test program")
	orig := ghDownload
	defer func() { ghDownload = orig }()
	ghDownload = func(u string, max int64) ([]byte, error) {
		if strings.HasSuffix(u, ".sha256") {
			return []byte(sha256Hex(exe) + " *OpLarik.exe\n"), nil
		}
		return exe, nil
	}
	rel := &ghRelease{Assets: map[string]string{"OpLarik.exe": "https://github.com/a", "OpLarik.exe.sha256": "https://github.com/a.sha256"}}
	if b, err := fetchVerified(rel); err != nil || string(b) != string(exe) {
		t.Fatal(err)
	}
	exe = []byte("MZ tampered")
	ghDownload = func(u string, max int64) ([]byte, error) {
		if strings.HasSuffix(u, ".sha256") {
			return []byte(sha256Hex([]byte("MZ test program"))), nil
		}
		return exe, nil
	}
	if _, err := fetchVerified(rel); err == nil || err.Error() != "checksum_mismatch" {
		t.Fatal("tampered file accepted")
	}
}

type fakeNV struct{ v map[uint32]uint32 }

func (f *fakeNV) Available() bool { return true }
func (f *fakeNV) Get(id uint32) (uint32, bool, error) {
	v, ok := f.v[id]
	return v, ok, nil
}
func (f *fakeNV) Set(id, v uint32) error { f.v[id] = v; return nil }
func (f *fakeNV) Restore(id uint32) error {
	delete(f.v, id)
	return nil
}

func TestNVBackups(t *testing.T) {
	t.Setenv("LARIK_DATA", t.TempDir())
	d := &fakeNV{v: map[uint32]uint32{nvPrerenderID: 3}}
	if err := nvApply(d, "fpslimit", 5); err == nil {
		t.Fatal("fps 5 accepted")
	}
	if err := nvApply(d, "power", 7); err == nil {
		t.Fatal("unknown power value accepted")
	}
	if nvApply(d, "latency", 1) != nil || nvApply(d, "fpslimit", 60) != nil || nvApply(d, "fpslimit", 141) != nil {
		t.Fatal("apply failed")
	}
	if d.v[nvPrerenderID] != 1 || d.v[nvFrameLimitID] != 141 {
		t.Fatal("values not set")
	}
	_ = nvRevert(d, "latency")
	_ = nvRevert(d, "fpslimit")
	if d.v[nvPrerenderID] != 3 {
		t.Fatal("latency not restored to the old value")
	}
	if _, set := d.v[nvFrameLimitID]; set {
		t.Fatal("fps limit not restored to the driver default")
	}
	if _, err := os.Stat(nvBackupFile()); !os.IsNotExist(err) {
		t.Fatal("backup file left behind")
	}
}
