//go:build !windows

package main

import (
	"errors"
	"strings"
	"sync"
	"time"
)

// mockSys simulates a fresh Windows 11 PC in memory. It powers demo mode on
// non-Windows machines and the engine tests.
type mockSys struct {
	mu       sync.Mutex
	keys     map[string]bool
	vals     map[string]map[string]RegVal
	svcs     map[string]string
	running  map[string]bool
	tasks    map[string]bool
	power    map[string][2]int
	plans    map[string]bool
	active   string
	ps       map[string]string
	appx     []string
	features map[string]string
	caps     map[string]CapInfo
	opened   []string
	slow     bool
}

func lk(k string) string { return strings.ToLower(k) }

func newMockSys(cat *Catalog) *mockSys {
	m := &mockSys{keys: map[string]bool{}, vals: map[string]map[string]RegVal{}, svcs: map[string]string{},
		running: map[string]bool{}, tasks: map[string]bool{}, power: map[string][2]int{},
		plans: map[string]bool{planBalanced: true, planHighPerf: true}, active: planBalanced,
		ps: map[string]string{}, features: map[string]string{}, caps: map[string]CapInfo{}}
	for _, t := range cat.Tweaks {
		for _, op := range t.Ops {
			switch op.T {
			case "reg":
				if d, err := targetVal(op.VT, op.D); err == nil && !d.Absent {
					_ = m.RegSet(op.K, op.N, d)
				}
			case "svc":
				if op.N == "NvTelemetryContainer" || op.N == "RetailDemo" {
					continue // not installed on this PC
				}
				m.svcs[lk(op.N)] = rawStr(op.D)
				m.running[lk(op.N)] = rawStr(op.D) == "auto"
			case "task":
				if !strings.Contains(op.Path, "ProgramDataUpdater") {
					m.tasks[lk(op.Path)] = true
				}
			case "power":
				d, _ := rawNum(op.D)
				m.power[lk(op.Sub+"|"+op.Set)] = [2]int{d, d}
			case "ps":
				m.ps[t.ID] = "0"
			case "feature":
				m.features[op.N] = "Enabled"
			case "cap":
				m.caps[op.N] = CapInfo{Name: op.N + "~~~~0.0.1.0", State: "Installed"}
			}
		}
	}
	for _, s := range boostServices {
		if _, ok := m.svcs[lk(s)]; !ok {
			m.svcs[lk(s)] = "auto"
		}
		m.running[lk(s)] = true
	}
	m.ps["interrupt_moderation_off"] = "na"
	m.features["Internet-Explorer-Optional-amd64"] = ""
	delete(m.features, "Internet-Explorer-Optional-amd64")
	m.features["WindowsMediaPlayer"] = "Disabled"
	delete(m.caps, "Microsoft.Windows.WordPad")
	// Realistic defaults that differ from catalog defaults.
	_ = m.RegSet(`HKLM\SYSTEM\CurrentControlSet\Control\Power`, "HibernateEnabled", DwordVal(1))
	_ = m.RegSet(`HKLM\SYSTEM\CurrentControlSet\Control\Power\User\PowerSchemes`, "ActivePowerScheme", StrVal(planBalanced))
	_ = m.RegSet(`HKLM\SYSTEM\CurrentControlSet\Control\FileSystem`, "DisableDeleteNotification", DwordVal(0))
	tcp := `HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces`
	_ = m.RegSet(tcp+`\{6B1F2C3D-0000-4E5F-9A1B-2C3D4E5F6071}`, "DhcpIPAddress", StrVal("192.168.1.34"))
	_ = m.RegSet(tcp+`\{1A2B3C4D-1111-4E5F-9A1B-2C3D4E5F6072}`, "EnableDHCP", DwordVal(1))
	m.appx = []string{
		"Microsoft.BingNews", "Microsoft.BingWeather", "Microsoft.GetHelp", "Microsoft.Getstarted",
		"Microsoft.MicrosoftOfficeHub", "Microsoft.MicrosoftSolitaireCollection", "Microsoft.People",
		"Microsoft.PowerAutomateDesktop", "Microsoft.Todos", "Microsoft.WindowsFeedbackHub", "Microsoft.WindowsMaps",
		"Microsoft.ZuneVideo", "Clipchamp.Clipchamp", "MSTeams", "Microsoft.YourPhone", "Microsoft.OutlookForWindows",
		"Microsoft.Windows.DevHome", "Microsoft.BingSearch", "Microsoft.Copilot", "MicrosoftCorporationII.MicrosoftFamily",
		"Microsoft.WindowsSoundRecorder", "Microsoft.WindowsAlarms", "Microsoft.XboxGamingOverlay", "Microsoft.GamingApp",
		"Microsoft.XboxSpeechToTextOverlay", "king.com.CandyCrushSaga", "BytedancePte.Ltd.TikTok", "Disney.37853FC22B2CE",
		"AmazonVideo.PrimeVideo", "SpotifyAB.SpotifyMusic", "Facebook.InstagramBeta", "7EE7776C.LinkedInforWindows",
		"Microsoft.WindowsStore", "Microsoft.WindowsCalculator", "Microsoft.Windows.Photos", "Microsoft.WindowsNotepad",
	}
	return m
}

func (m *mockSys) delay() {
	if m.slow {
		time.Sleep(120 * time.Millisecond)
	}
}

func (m *mockSys) RegGet(key, name string) (RegVal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v, ok := m.vals[lk(key)][lk(name)]; ok {
		return v, nil
	}
	return RegVal{Absent: true}, nil
}

func (m *mockSys) RegSet(key, name string, v RegVal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := lk(key)
	// create parent chain
	parts := strings.Split(k, `\`)
	for i := 1; i <= len(parts); i++ {
		m.keys[strings.Join(parts[:i], `\`)] = true
	}
	if v.Absent {
		delete(m.vals[k], lk(name))
		return nil
	}
	if m.vals[k] == nil {
		m.vals[k] = map[string]RegVal{}
	}
	m.vals[k][lk(name)] = v
	return nil
}

func (m *mockSys) RegDelete(key, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.vals[lk(key)], lk(name))
	return nil
}

func (m *mockSys) RegKeyExists(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.keys[lk(key)]
}

func (m *mockSys) RegCreateKey(key string) error {
	return m.RegSet(key, "__larik_probe__", RegVal{Absent: true})
}

func (m *mockSys) RegDeleteTree(key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := lk(key)
	for kk := range m.keys {
		if kk == k || strings.HasPrefix(kk, k+`\`) {
			delete(m.keys, kk)
			delete(m.vals, kk)
		}
	}
	return nil
}

func (m *mockSys) RegSubkeys(key string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	k := lk(key) + `\`
	seen := map[string]bool{}
	var out []string
	for kk := range m.keys {
		if strings.HasPrefix(kk, k) {
			rest := kk[len(k):]
			child, _, _ := strings.Cut(rest, `\`)
			if child != "" && !seen[child] {
				seen[child] = true
				out = append(out, key+`\`+child)
			}
		}
	}
	for i, o := range out {
		out[i] = o[strings.LastIndex(o, `\`)+1:]
	}
	if len(out) == 0 && !m.keys[lk(key)] {
		return nil, errors.New("not found")
	}
	return out, nil
}

func (m *mockSys) SvcStart(name string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.svcs[lk(name)]
	return s, ok
}

func (m *mockSys) SvcConfig(name, start string) error {
	m.delay()
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.svcs[lk(name)]; !ok {
		return errors.New("service not found")
	}
	m.svcs[lk(name)] = start
	return nil
}

func (m *mockSys) SvcStop(name string) {
	m.mu.Lock()
	m.running[lk(name)] = false
	m.mu.Unlock()
}

func (m *mockSys) SvcRun(name string) {
	m.mu.Lock()
	m.running[lk(name)] = true
	m.mu.Unlock()
}

func (m *mockSys) TaskState(path string) (bool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	en, ok := m.tasks[lk(path)]
	return ok, en
}

func (m *mockSys) TaskSet(path string, enabled bool) error {
	m.delay()
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.tasks[lk(path)]; !ok {
		return errors.New("task not found")
	}
	m.tasks[lk(path)] = enabled
	return nil
}

func (m *mockSys) PowerGet(sub, set string) (int, int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.power[lk(sub+"|"+set)]
	return v[0], v[1], ok
}

func (m *mockSys) PowerSet(sub, set string, ac, dc int, setDC bool) error {
	m.delay()
	m.mu.Lock()
	defer m.mu.Unlock()
	v := m.power[lk(sub+"|"+set)]
	v[0] = ac
	if setDC {
		v[1] = dc
	}
	m.power[lk(sub+"|"+set)] = v
	return nil
}

func (m *mockSys) ActivePlan() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.active
}

func (m *mockSys) PlanExists(guid string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.plans[strings.ToLower(guid)]
}

func (m *mockSys) Exec(args ...string) (string, error) {
	m.delay()
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(args) >= 3 && args[0] == "powercfg" {
		switch args[1] {
		case "-duplicatescheme":
			if len(args) >= 4 {
				m.plans[strings.ToLower(args[3])] = true
			}
		case "-setactive":
			g := strings.ToLower(args[2])
			if !m.plans[g] {
				return "", errors.New("plan not found")
			}
			m.active = g
			m.vals[lk(`HKLM\SYSTEM\CurrentControlSet\Control\Power\User\PowerSchemes`)]["activepowerscheme"] = StrVal(g)
		case "-delete":
			delete(m.plans, strings.ToLower(args[2]))
		case "/hibernate":
			v := uint32(0)
			if args[2] == "on" {
				v = 1
			}
			m.vals[lk(`HKLM\SYSTEM\CurrentControlSet\Control\Power`)]["hibernateenabled"] = DwordVal(v)
		}
	}
	return "", nil
}

func (m *mockSys) PSRun(id, mode, script, backup string) (string, error) {
	time.Sleep(200 * time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	switch mode {
	case "apply":
		if id == "restore_point" {
			time.Sleep(1500 * time.Millisecond)
			return "ok", nil
		}
		m.ps[id] = "1"
		return "@@BACKUP@@{\"demo\":0}", nil
	case "revert":
		m.ps[id] = "0"
	}
	return "", nil
}

func (m *mockSys) PSChecks(checks map[string]string) map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]string{}
	for id := range checks {
		out[id] = m.ps[id]
	}
	return out
}

func (m *mockSys) AppxInstalled() ([]string, error) {
	time.Sleep(600 * time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.appx...), nil
}

func (m *mockSys) AppxRemove(patterns []string) error {
	time.Sleep(350 * time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	var keep []string
	for _, a := range m.appx {
		if !appxPresent([]string{a}, patterns) {
			keep = append(keep, a)
		}
	}
	m.appx = keep
	return nil
}

func (m *mockSys) Features(names []string) (map[string]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]string{}
	for k, v := range m.features {
		out[k] = v
	}
	return out, nil
}

func (m *mockSys) FeatureSet(name string, enable bool) error {
	time.Sleep(300 * time.Millisecond)
	m.mu.Lock()
	defer m.mu.Unlock()
	if enable {
		m.features[name] = "Enabled"
	} else {
		m.features[name] = "Disabled"
	}
	return nil
}

func (m *mockSys) Caps(prefixes []string) (map[string]CapInfo, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]CapInfo{}
	for k, v := range m.caps {
		out[k] = v
	}
	return out, nil
}

func (m *mockSys) CapRemove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, c := range m.caps {
		if c.Name == name {
			c.State = "NotPresent"
			m.caps[k] = c
		}
	}
	return nil
}

func (m *mockSys) CapAdd(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, c := range m.caps {
		if c.Name == name {
			c.State = "Installed"
			m.caps[k] = c
		}
	}
	return nil
}

func (m *mockSys) OpenURL(url string) {
	m.mu.Lock()
	m.opened = append(m.opened, url)
	m.mu.Unlock()
}

func serviceRunning(name string) bool { return demoSys != nil && demoSys.running[lk(name)] }
