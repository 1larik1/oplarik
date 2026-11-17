//go:build !windows

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

func snapshotMock(m *mockSys) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	vals := map[string]map[string]RegVal{}
	for k, v := range m.vals {
		if len(v) > 0 {
			vals[k] = v
		}
	}
	b, _ := json.Marshal(struct {
		V  map[string]map[string]RegVal
		S  map[string]string
		T  map[string]bool
		P  map[string][2]int
		A  string
		PS map[string]string
		F  map[string]string
		C  map[string]CapInfo
	}{vals, m.svcs, m.tasks, m.power, m.active, m.ps, m.features, m.caps})
	return string(b)
}

func newTestEngine(t *testing.T) (*Engine, *mockSys) {
	cat, err := LoadCatalog(catalogJSON)
	if err != nil {
		t.Fatal(err)
	}
	m := newMockSys(cat)
	demoSys = m
	dir := t.TempDir()
	e := NewEngine(m, cat, OpenStore(dir), true)
	info, _ := scanSystem(m)
	info.ScanAt = nowStr()
	info.Analyze()
	e.SetInfo(info)
	if err := e.ScanDebloat(); err != nil {
		t.Fatal(err)
	}
	return e, m
}

func TestCatalogSize(t *testing.T) {
	cat, _ := LoadCatalog(catalogJSON)
	opt := 0
	for _, tw := range cat.Tweaks {
		if tw.IsOptimization() {
			opt++
		}
	}
	if opt < 50 {
		t.Fatalf("only %d optimizations", opt)
	}
	t.Logf("%d tweaks, %d optimizations", len(cat.Tweaks), opt)
}

func TestApplyRevertRoundTrip(t *testing.T) {
	e, m := newTestEngine(t)
	var ids []string
	for _, tw := range e.cat.Tweaks {
		if !tw.HasOp("appx") {
			ids = append(ids, tw.ID)
		}
	}
	before := snapshotMock(m)
	st0 := e.States(true)

	res := e.Apply(ids, nil)
	for _, r := range res {
		if !r.OK {
			t.Errorf("apply %s: %s", r.ID, r.Error)
		}
	}
	st := e.States(true)
	for _, id := range ids {
		s := st[id]
		if s.NA {
			continue
		}
		if !s.Active {
			t.Errorf("%s not active after apply: %+v", id, s)
		}
	}

	res = e.Revert(ids, nil)
	for _, r := range res {
		if !r.OK {
			t.Errorf("revert %s: %s", r.ID, r.Error)
		}
	}
	after := snapshotMock(m)
	st1 := e.States(true)
	for _, id := range ids {
		if st0[id].Active != st1[id].Active {
			t.Errorf("%s state changed after round trip: %v -> %v", id, st0[id].Active, st1[id].Active)
		}
	}
	if before != after {
		// find a diff to report
		var a, b map[string]any
		_ = json.Unmarshal([]byte(before), &a)
		_ = json.Unmarshal([]byte(after), &b)
		for k := range a {
			if !reflect.DeepEqual(a[k], b[k]) {
				dumpDiff(t, k, a[k], b[k])
			}
		}
	}
	var applied int
	e.store.Read(func(d *StoreData) { applied = len(d.Applied) })
	if applied != 0 {
		t.Errorf("%d tweaks still recorded as applied", applied)
	}
}

func dumpDiff(t *testing.T, k string, a, b any) {
	am, ok1 := a.(map[string]any)
	bm, ok2 := b.(map[string]any)
	if !ok1 || !ok2 {
		t.Errorf("%s differs: %v vs %v", k, a, b)
		return
	}
	for kk, v := range am {
		if !reflect.DeepEqual(v, bm[kk]) {
			t.Errorf("%s/%s differs:\n before %v\n after  %v", k, kk, v, bm[kk])
		}
	}
	for kk, v := range bm {
		if _, ok := am[kk]; !ok {
			t.Errorf("%s/%s added: %v", k, kk, v)
		}
	}
}

func TestPresetsAndScore(t *testing.T) {
	e, _ := newTestEngine(t)
	st := e.States(true)
	score, a, total := e.Score(st)
	rec := e.PresetIDs("recommended", st)
	comp := e.PresetIDs("competitive", st)
	priv := e.PresetIDs("privacy", st)
	t.Logf("score %.1f%% (%d/%d) rec=%d comp=%d priv=%d", score, a, total, len(rec), len(comp), len(priv))
	if len(rec) < 25 || len(comp) <= len(rec) {
		t.Fatal("presets too small")
	}
	e.Apply(rec, nil)
	st = e.States(true)
	score2, _, _ := e.Score(st)
	if score2 < 99 {
		for _, id := range rec {
			if !st[id].Active {
				t.Logf("not active: %s %+v", id, st[id])
			}
		}
		t.Fatalf("score after recommended = %.1f", score2)
	}
}

func TestDebloatApps(t *testing.T) {
	e, m := newTestEngine(t)
	st := e.States(false)
	if st["app_news"].Active || st["app_news"].Unknown {
		t.Fatalf("news should be installed: %+v", st["app_news"])
	}
	e.Apply([]string{"app_news", "sp_candy"}, nil)
	st = e.States(false)
	if !st["app_news"].Active || !st["sp_candy"].Active {
		t.Fatal("apps not removed")
	}
	for _, a := range m.appx {
		if a == "Microsoft.WindowsStore" {
			return
		}
	}
	t.Fatal("store was removed")
}

func TestGameFlags(t *testing.T) {
	_, m := newTestEngine(t)
	g := &Game{Exe: "cs2.exe", Path: `C:\Games\cs2.exe`}
	before := snapshotMock(m)
	for _, f := range []func(bool) error{
		func(on bool) error { return setGamePrio(m, g, on) },
		func(on bool) error { return setGamePathFlag(m, g, "gpu", on) },
		func(on bool) error { return setGamePathFlag(m, g, "fso", on) },
	} {
		if err := f(true); err != nil {
			t.Fatal(err)
		}
	}
	v, _ := m.RegGet(ifeoBase+`\cs2.exe\PerfOptions`, "CpuPriorityClass")
	if n, _ := v.Num(); n != 3 {
		t.Fatal("priority not set")
	}
	v, _ = m.RegGet(layersKey, g.Path)
	if s, _ := v.Str(); s != "~ "+fsoFlag {
		t.Fatalf("fso flag = %q", s)
	}
	setGamePrio(m, g, false)
	setGamePathFlag(m, g, "gpu", false)
	setGamePathFlag(m, g, "fso", false)
	if snapshotMock(m) != before {
		t.Fatal("game flags not fully reverted")
	}
	if m.RegKeyExists(ifeoBase + `\cs2.exe`) {
		t.Fatal("IFEO key left behind")
	}
}

func TestMain(m *testing.M) {
	os.Exit(m.Run())
}

var _ = fmt.Sprintf
