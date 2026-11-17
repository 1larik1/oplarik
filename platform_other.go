//go:build !windows

package main

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"log"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

var demoSys *mockSys

// platformMain on non-Windows runs the simulated demo used for UI work and tests.
func platformMain(o Options) {
	cat, _ := LoadCatalog(catalogJSON)
	demoSys = newMockSys(cat)
	demoSys.slow = true
	o.Demo = true
	seedDemoSessions()
	s, ln := newServer(demoSys, true, o)
	fmt.Println("OpLarik demo running at", appURL(s.port, s.token))
	if err := s.Serve(ln); err != nil {
		log.Fatal(err)
	}
}

func newSys() Sys { return demoSys }

func dataDir() string {
	if d := os.Getenv("LARIK_DATA"); d != "" {
		return d
	}
	return filepath.Join(os.TempDir(), "larik-demo")
}

func systemLang() string { return "en" }

func removeInstanceFile(token string) {}

func platformDetectGames(sys Sys) []FoundGame {
	steam := `D:\SteamLibrary\steamapps\common\`
	return []FoundGame{
		{Name: "Among Us", Exe: "Among Us.exe", Path: steam + `Among Us\Among Us.exe`, Source: "steam", SteamID: "945360", Launch: "steam:945360"},
		{Name: "Counter-Strike 2", Exe: "cs2.exe", Path: steam + `Counter-Strike Global Offensive\game\bin\win64\cs2.exe`, Source: "steam", SteamID: "730", Launch: "steam:730"},
		{Name: "Dota 2", Exe: "dota2.exe", Path: steam + `dota 2 beta\game\bin\win64\dota2.exe`, Source: "steam", SteamID: "570", Launch: "steam:570"},
		{Name: "Death's Door", Exe: "DeathsDoor.exe", Path: steam + `DeathsDoor\DeathsDoor.exe`, Source: "steam", SteamID: "894020", Launch: "steam:894020"},
		{Name: "PUBG: BATTLEGROUNDS", Exe: "TslGame.exe", Path: steam + `PUBG\TslGame\Binaries\Win64\TslGame.exe`, Source: "steam", SteamID: "578080", Launch: "steam:578080"},
		{Name: "Wizard of Legend", Exe: "WizardOfLegend.exe", Path: steam + `Wizard of Legend\WizardOfLegend.exe`, Source: "steam", SteamID: "445980", Launch: "steam:445980"},
		{Name: "Worms W.M.D", Exe: "Worms W.M.D.exe", Path: steam + `WormsWMD\NoIcon\Worms W.M.D.exe`, Source: "steam", SteamID: "327030", Launch: "steam:327030"},
		{Name: "Fortnite", Exe: "FortniteClient-Win64-Shipping.exe", Path: `C:\Program Files\Epic Games\Fortnite\FortniteGame\Binaries\Win64\FortniteClient-Win64-Shipping.exe`, Source: "epic", Launch: "epic:fn:4fe75bbc5a674f4f9b356b5c90567da5:Fortnite"},
		{Name: "VALORANT", Exe: "VALORANT-Win64-Shipping.exe", Path: `C:\Riot Games\VALORANT\live\ShooterGame\Binaries\Win64\VALORANT-Win64-Shipping.exe`, Source: "riot", Launch: "riot:valorant"},
	}
}

func scanSystem(sys Sys) (*SysInfo, error) {
	time.Sleep(900 * time.Millisecond)
	const gb = 1 << 30
	i := &SysInfo{Computer: "DESKTOP-LARIK", User: "gamer", Printers: 0}
	i.OS.Name, i.OS.Version, i.OS.Build, i.OS.UBR, i.OS.Display, i.OS.Arch = "Microsoft Windows 10 Home", "10.0.19045", 19045, 7725, "22H2", "64-bit"
	i.OS.Install, i.OS.Uptime = "2023-03-14", 2*86400+5*3600
	i.CPU.Name, i.CPU.Vendor, i.CPU.Cores, i.CPU.Threads, i.CPU.MaxMHz = "Intel(R) Core(TM) i5-10400F CPU @ 2.90GHz", "GenuineIntel", 6, 12, 2904
	i.CPU.L2, i.CPU.L3, i.CPU.Socket, i.CPU.Virt = 1536, 12288, "LGA1200", true
	i.GPUs = []GPUInfo{{Name: "NVIDIA GeForce RTX 3060 Ti", Vendor: "NVIDIA", VRAM: 8 * gb, Driver: "32.0.16.1742", DriverDate: "2026-08-19", Res: "1920x1080", Hz: 144, ReBAR: "off"}}
	i.RAM.Total, i.RAM.Slots = 16*gb, 4
	i.RAM.Modules = []RAMModule{
		{Size: 8 * gb, Speed: 3200, Conf: 2666, Maker: "Kingston", Part: "KF432C16BB/8", Slot: "DIMM_A2", TypeCode: 26},
		{Size: 8 * gb, Speed: 3200, Conf: 2666, Maker: "Kingston", Part: "KF432C16BB/8", Slot: "DIMM_B2", TypeCode: 26},
	}
	i.Disks = []DiskInfo{
		{Model: "WDC WD3200AAKX-073CA1", Media: "Unspecified", Bus: "SATA", Size: 320 * 1000 * 1000 * 1000, Health: "Healthy", Num: 0},
		{Model: "KINGSTON SUV500240G", Media: "SSD", Bus: "SATA", Size: 240 * 1000 * 1000 * 1000, Health: "Healthy", System: true, Num: 1},
		{Model: "WDC WD1002FAEX-00Z3A0", Media: "Unspecified", Bus: "SATA", Size: 1000 * 1000 * 1000 * 1000, Health: "Healthy", Num: 2},
	}
	i.SysDrive.Letter, i.SysDrive.Total, i.SysDrive.Free, i.SysDrive.Media = "C:", 223*gb, 38*gb, "SSD"
	i.Board.Maker, i.Board.Product, i.Board.BIOS, i.Board.BIOSDate = "ASUSTeK COMPUTER INC.", "PRIME B460-PLUS", "1403", "2021-06-11"
	i.Displays = []DisplayInfo{{Name: "LG ULTRAGEAR", Res: "1920x1080", Hz: 144, MaxHz: 144}}
	i.Net = []NetInfo{{Name: "Ethernet", Desc: "Intel(R) Ethernet Connection (11) I219-V", Speed: "1 Gbps", Wifi: false}}
	i.Power.Plan, i.Power.Laptop = "Balanced", false
	i.Security.VBS, i.Security.HVCI, i.Security.SecureBoot, i.Security.AV = false, false, "off", "Windows Defender"
	i.Counts.Processes, i.Counts.Services, i.Counts.Startup = 188, 104, 9
	return i, nil
}

type demoLive struct {
	mu sync.Mutex
	t  float64
}

func newLiveSampler() LiveSampler { return &demoLive{} }

func (d *demoLive) Sample() LiveStats {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.t += 0.35
	const gb = 1 << 30
	return LiveStats{
		CPU:      math.Max(2, 22+14*math.Sin(d.t)+rand.Float64()*8),
		RAMUsed:  uint64((9.6 + 0.6*math.Sin(d.t/3)) * gb),
		RAMTotal: 16 * gb,
		GPU:      math.Max(0, 38+30*math.Sin(d.t/1.7)+rand.Float64()*6),
		VRAM:     uint64((2.1 + 0.4*math.Sin(d.t/2)) * gb),
		At:       time.Now().UnixMilli(),
	}
}

func listProcesses() []Proc {
	return []Proc{{"OneDrive.exe", 1, 96 << 20}, {"ms-teams.exe", 2, 210 << 20}, {"ms-teams.exe", 3, 120 << 20},
		{"Discord.exe", 4, 260 << 20}, {"Spotify.exe", 5, 180 << 20}, {"Widgets.exe", 6, 64 << 20},
		{"PhoneExperienceHost.exe", 7, 58 << 20}, {"EpicGamesLauncher.exe", 8, 140 << 20}}
}

var demoTemp int64 = 1_384_000_000

func cleanMemory() (uint64, uint64, error) {
	time.Sleep(1200 * time.Millisecond)
	return 5_900_000_000, 7_400_000_000, nil
}

func tempScan() (int64, int) { return demoTemp, int(demoTemp / 400_000) }

func tempClean() (int64, int) {
	time.Sleep(1500 * time.Millisecond)
	b := demoTemp
	demoTemp = 12_000_000
	return b - demoTemp, int((b - demoTemp) / 400_000)
}

func browseExe() (string, error) {
	return `C:\Games\Rocket League\Binaries\Win64\RocketLeague.exe`, nil
}

func rebootPC() error        { return nil }
func signOut() error         { return nil }
func restartExplorer() error { time.Sleep(800 * time.Millisecond); return nil }
func installApp() error      { time.Sleep(1200 * time.Millisecond); demoInstalled = true; return nil }
func openPath(p string)      {}
func openRestoreUI()         {}

var demoInstalled bool

func installStatus() map[string]any {
	return map[string]any{"installed": demoInstalled, "dir": `C:\Program Files\OpLarik`, "fromInstalled": demoInstalled, "canInstall": true}
}

func postTweak(sys Sys, id string) {}

// ---- overlay (simulated on non-Windows)

var demoOverlayVisible = true

func ensureOverlay(st Settings) {}

func syncAutostart(st Settings) error { return nil }

func writeOverlayVisible(v bool) error { demoOverlayVisible = v; return nil }

func overlayStatus() map[string]any {
	st := demoSys
	_ = st
	st0 := OpenStore(dataDir()).GetSettings()
	if !st0.Overlay.Enabled && !st0.AutoGame.Enabled {
		return map[string]any{"running": false, "platform": true}
	}
	return map[string]any{"running": true, "platform": true, "hotkey": "ok", "fps": "ok", "nvml": true, "cpuTemp": true,
		"visible": demoOverlayVisible, "admin": true, "ver": appVersion, "auto": st0.AutoGame.Enabled, "game": ""}
}

func sampleSensors() SensorSample {
	t := float64(time.Now().UnixMilli()%6000) / 6000
	return SensorSample{CPUTemp: 58 + 4*math.Sin(t*6.28), GPUTemp: 66 + 3*math.Sin(t*6.28), GPUClock: 1755, GPUPower: 54, GPUUtil: -1, VRAMUsed: -1, NVML: true, CPUTempOK: true}
}

// Demo builds keep the secret as-is (DPAPI exists only on Windows).
func protectSecret(plain []byte) ([]byte, error) { return append([]byte("demo:"), plain...), nil }
func unprotectSecret(enc []byte) ([]byte, error) {
	if len(enc) < 5 || string(enc[:5]) != "demo:" {
		return nil, fmt.Errorf("bad secret")
	}
	return enc[5:], nil
}
func secureDir(dir string) error { return nil }

// ---------------------------------------------------------------- 1.0.5 tools (demo)

var demoTools = struct {
	sync.Mutex
	shaderSeeded bool
	dns          []string
	dnsBackup    []string
	startup      map[string]bool
}{dns: []string{"192.168.0.1"}, startup: map[string]bool{}}

func shaderCacheDirs() []CacheDir {
	base := filepath.Join(os.TempDir(), "larik-demo-shader")
	demoTools.Lock()
	cleared := demoTools.shaderSeeded
	demoTools.Unlock()
	if !cleared {
		for _, d := range []string{"D3DSCache", "NVIDIA/DXCache", "NVIDIA/GLCache"} {
			p := filepath.Join(base, d)
			_ = os.MkdirAll(p, 0o755)
			for i := 0; i < 6; i++ {
				f := filepath.Join(p, fmt.Sprintf("cache%d.bin", i))
				if _, err := os.Stat(f); err != nil {
					_ = os.WriteFile(f, make([]byte, 180_000+i*90_000), 0o644)
				}
			}
		}
		demoTools.Lock()
		demoTools.shaderSeeded = true
		demoTools.Unlock()
	}
	return []CacheDir{
		{ID: "dx", Name: "DirectX", Path: filepath.Join(base, "D3DSCache")},
		{ID: "nvdx", Name: "NVIDIA DirectX", Path: filepath.Join(base, "NVIDIA", "DXCache")},
		{ID: "nvgl", Name: "NVIDIA OpenGL", Path: filepath.Join(base, "NVIDIA", "GLCache")},
	}
}

func init() {
	// The demo has no network: simulate resolver latencies.
	lat := map[string]float64{"192.168.0.1": 38, "1.1.1.1": 9, "1.0.0.1": 10, "8.8.8.8": 14, "8.8.4.4": 15,
		"9.9.9.9": 21, "149.112.112.112": 22, "208.67.222.222": 27, "208.67.220.220": 29}
	dnsProbe = func(server, domain string) (time.Duration, error) {
		ms, ok := lat[server]
		if !ok {
			return 0, fmt.Errorf("timeout")
		}
		time.Sleep(time.Duration(ms*float64(time.Millisecond)) + time.Duration(rand.Intn(4))*time.Millisecond)
		return time.Duration((ms + rand.Float64()*3) * float64(time.Millisecond)), nil
	}
}

func currentDNS() []string {
	demoTools.Lock()
	defer demoTools.Unlock()
	return append([]string{}, demoTools.dns...)
}
func dnsHasBackup() bool {
	demoTools.Lock()
	defer demoTools.Unlock()
	return demoTools.dnsBackup != nil
}
func applyDNS(p, s string) error {
	demoTools.Lock()
	defer demoTools.Unlock()
	if demoTools.dnsBackup == nil {
		demoTools.dnsBackup = demoTools.dns
	}
	demoTools.dns = []string{p, s}
	return nil
}
func restoreDNS() error {
	demoTools.Lock()
	defer demoTools.Unlock()
	if demoTools.dnsBackup == nil {
		return fmt.Errorf("nothing to restore")
	}
	demoTools.dns, demoTools.dnsBackup = demoTools.dnsBackup, nil
	return nil
}

var demoStartup = []StartupItem{
	{ID: "hkcu|Discord", Name: "Discord", Command: `C:\Users\gamer\AppData\Local\Discord\Update.exe --processStart Discord.exe`, Source: "hkcu"},
	{ID: "hkcu|Steam", Name: "Steam", Command: `"C:\Program Files (x86)\Steam\steam.exe" -silent`, Source: "hkcu"},
	{ID: "hkcu|EpicGamesLauncher", Name: "EpicGamesLauncher", Command: `"C:\Program Files (x86)\Epic Games\Launcher\Portal\Binaries\Win64\EpicGamesLauncher.exe" -silent`, Source: "hkcu"},
	{ID: "hkcu|OneDrive", Name: "OneDrive", Command: `"C:\Users\gamer\AppData\Local\Microsoft\OneDrive\OneDrive.exe" /background`, Source: "hkcu"},
	{ID: "hklm|RtkAudUService", Name: "RtkAudUService", Command: `"C:\Windows\System32\DriverStore\FileRepository\realtekservice.inf_amd64\RtkAudUService64.exe" -background`, Source: "hklm"},
	{ID: "hklm|SecurityHealth", Name: "SecurityHealth", Command: `%windir%\system32\SecurityHealthSystray.exe`, Source: "hklm"},
	{ID: "hklm32|Spotify", Name: "Spotify", Command: `"C:\Users\gamer\AppData\Roaming\Spotify\Spotify.exe" --autostart --minimized`, Source: "hklm32"},
	{ID: "ufolder|Telegram.lnk", Name: "Telegram", Command: `C:\Users\gamer\AppData\Roaming\Microsoft\Windows\Start Menu\Programs\Startup\Telegram.lnk`, Source: "ufolder"},
}

func startupItems() ([]StartupItem, error) {
	demoTools.Lock()
	defer demoTools.Unlock()
	out := make([]StartupItem, len(demoStartup))
	for i, it := range demoStartup {
		on, set := demoTools.startup[it.ID]
		it.On = !set || on
		out[i] = it
	}
	return out, nil
}

func setStartup(id string, on bool) error {
	demoTools.Lock()
	defer demoTools.Unlock()
	for _, it := range demoStartup {
		if it.ID == id {
			demoTools.startup[id] = on
			return nil
		}
	}
	return fmt.Errorf("unknown startup item")
}

func localDir() string { return filepath.Join(dataDir(), "local") }

// seedDemoSessions puts finished game sessions into the demo: two of
// Counter-Strike 2 (before and after optimizing) and one of Fortnite.
func seedDemoSessions() {
	dir := localDir()
	if len(loadSessions(dir)) > 0 {
		return
	}
	_ = os.MkdirAll(filepath.Join(dir, "Sessions"), 0o755)
	now := time.Now()
	rnd := rand.New(rand.NewSource(7))
	mk := func(game, exe string, ago, dur time.Duration, base, spread float64, dipEvery int, g, c, ping float64, warn int) SessionSummary {
		st := now.Add(-ago)
		id := st.Format("2006-01-02_15-04-05") + "_" + safeName(exe) + ".csv"
		p := filepath.Join(dir, "Sessions", id)
		var b strings.Builder
		b.WriteString("time,fps,low1_fps,frametime_ms\n")
		var fps []float64
		n := int(dur.Seconds())
		for i := 0; i < n; i++ {
			v := base + spread*math.Sin(float64(i)/40) + rnd.NormFloat64()*spread/3
			if dipEvery > 0 && i%dipEvery == dipEvery/2 {
				v *= 0.45 + rnd.Float64()*0.15
			}
			v = math.Max(20, v)
			fps = append(fps, v)
			fmt.Fprintf(&b, "%s,%.1f,%.1f,%.2f\n", st.Add(time.Duration(i)*time.Second).Format("15:04:05"), v, v*0.6, 1000/v)
		}
		_ = os.WriteFile(p, []byte(b.String()), 0o644)
		ss := sessionStats(fps)
		return SessionSummary{ID: id, Game: game, Exe: exe, Start: st.Format(time.RFC3339), End: st.Add(dur).Format(time.RFC3339), Duration: n,
			AvgFPS: ss.AvgFPS, Low1: ss.Low1, MinFPS: ss.MinFPS, MaxGPUTemp: g, MaxCPUTemp: c, AvgPing: ping, Warnings: warn, CSV: p}
	}
	list := []SessionSummary{
		mk("Counter-Strike 2", "cs2.exe", 3*time.Hour, 52*time.Minute, 186, 18, 140, 78, 71, 24, 0),
		mk("Fortnite", "fortniteclient-win64-shipping.exe", 27*time.Hour, 41*time.Minute, 98, 12, 90, 86, 74, 41, 37),
		mk("Counter-Strike 2", "cs2.exe", 50*time.Hour, 47*time.Minute, 158, 24, 45, 84, 79, 27, 12),
	}
	_ = writeAtomic(sessionsFile(dir), list)
	// The demo pretends optimizations were switched on between the two CS2 games.
	st := OpenStore(dataDir())
	at := now.Add(-30 * time.Hour).Format(time.RFC3339)
	st.Update(func(d *StoreData) {
		for _, id := range []string{"game_mode", "max_power_plan", "games_mmcss", "input_latency"} {
			d.History = append(d.History, HistItem{At: at, Action: "apply", ID: id, OK: true})
		}
		d.History = append(d.History, HistItem{At: at, Action: "apply", ID: "dns_tool", OK: true, Note: "Cloudflare"})
	})
}

func ensureTray(st Settings)              {}
func syncTrayAutostart(st Settings) error { return nil }

func lockBoostGlobal() func() { return func() {} }

// ---------------------------------------------------------------- 1.0.7 (demo)

type demoNV struct{}

var demoNVState = struct {
	sync.Mutex
	v map[uint32]uint32
}{v: map[uint32]uint32{}}

func nvidiaDriver() nvDriver { return demoNV{} }

func (demoNV) Available() bool { return true }
func (demoNV) Get(id uint32) (uint32, bool, error) {
	demoNVState.Lock()
	defer demoNVState.Unlock()
	v, ok := demoNVState.v[id]
	return v, ok, nil
}
func (demoNV) Set(id, v uint32) error {
	demoNVState.Lock()
	defer demoNVState.Unlock()
	demoNVState.v[id] = v
	return nil
}
func (demoNV) Restore(id uint32) error {
	demoNVState.Lock()
	defer demoNVState.Unlock()
	delete(demoNVState.v, id)
	return nil
}

func readDiskHealth() ([]DiskHealth, error) {
	time.Sleep(700 * time.Millisecond)
	return []DiskHealth{
		{Name: "WDC WD3200AAKX-073CA1", Media: fixMedia("", "SATA", "WDC WD3200AAKX-073CA1", -1), Bus: "SATA", Size: 320_072_933_376, Health: "Healthy", Temp: 38, TempMax: -1, Wear: -1, Hours: 41230, ReadErr: 0, WriteErr: 0},
		{Name: "KINGSTON SUV500240G", Media: "SSD", Bus: "SATA", Size: 240_057_409_536, Health: "Healthy", Temp: 36, TempMax: 70, Wear: 18, Hours: 15420, ReadErr: 0, WriteErr: 0},
		{Name: "WDC WD1002FAEX-00Z3A0", Media: fixMedia("", "SATA", "WDC WD1002FAEX-00Z3A0", -1), Bus: "SATA", Size: 1_000_204_886_016, Health: "Healthy", Temp: 41, TempMax: -1, Wear: -1, Hours: 38870, ReadErr: 0, WriteErr: 0},
	}, nil
}

func defaultGateway() string { return "192.168.0.1" }

func pingOnce(ip string, timeout time.Duration) float64 {
	base := map[string]float64{"192.168.0.1": 3, "1.1.1.1": 14, "8.8.8.8": 19}[ip]
	if base == 0 {
		return -1
	}
	if rand.Float64() < 0.02 {
		time.Sleep(60 * time.Millisecond)
		return -1
	}
	// Wi-Fi: the router hop jitters a bit; load adds bufferbloat.
	v := base + rand.Float64()*4 + float64(demoLoad.Load())*rand.Float64()
	time.Sleep(time.Duration(v) * time.Millisecond)
	return math.Round(v)
}

var demoLoad atomic.Int64

func demoTransfer(ctx context.Context, n *atomic.Int64, mbps float64, bloat int64) error {
	demoLoad.Store(bloat)
	defer demoLoad.Store(0)
	t := time.NewTicker(100 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-t.C:
			n.Add(int64(mbps * 1e6 / 8 / 10 * (0.85 + rand.Float64()*0.3)))
		}
	}
}

func init() {
	netDown = func(ctx context.Context, n *atomic.Int64) error { return demoTransfer(ctx, n, 96, 70) }
	netUp = func(ctx context.Context, n *atomic.Int64) error { return demoTransfer(ctx, n, 38, 110) }
	nvLatest = func() (*DriverLatest, error) {
		time.Sleep(600 * time.Millisecond)
		return &DriverLatest{Version: "581.57", Date: "Tue Oct 14, 2025", URL: "https://www.nvidia.com/Download/index.aspx", Checked: nowStr()}, nil
	}
	ghLatest = func(repo string) (*ghRelease, error) {
		time.Sleep(500 * time.Millisecond)
		v := verParts(appVersion)
		return &ghRelease{Tag: fmt.Sprintf("v%d.%d.%d", v[0], v[1], v[2]+1), Notes: "Demo release notes.", Published: time.Now().Add(-48 * time.Hour).Format(time.RFC3339),
			Assets: map[string]string{exeName: "https://github.com/demo/" + exeName, exeName + ".sha256": "https://github.com/demo/" + exeName + ".sha256"}}, nil
	}
	ghDownload = func(u string, max int64) ([]byte, error) {
		exe := []byte("MZ demo")
		if strings.HasSuffix(u, ".sha256") {
			sum := sha256Hex(exe)
			return []byte(sum + "  " + exeName + "\n"), nil
		}
		return exe, nil
	}
}

func syncCleanTask(c AutoCleanConfig) error { return nil }
func installUpdate(exe []byte) error        { time.Sleep(800 * time.Millisecond); return nil }
func restartAfterUpdate()                   {}
func cleanupOldExe()                        {}

// ---------------------------------------------------------------- 1.0.8 (demo): pictures

func coverExeExists(p string) bool { return !strings.Contains(p, "NoIcon") }
func driveLocal(vol string) bool   { return true }

// Demo: games are "started" only in the log.
func openURLAsUser(u string) error          { log.Print("demo launch: ", u); return nil }
func protocolRegistered(scheme string) bool { return true }
func startExeAsUser(path string, args []string, dir string, asAdmin bool) error {
	log.Print("demo start: ", path, args)
	return nil
}

func hsl(h, s, l float64) color.NRGBA {
	h = math.Mod(h, 360) / 360
	q := l * (1 + s)
	if l >= 0.5 {
		q = l + s - l*s
	}
	p := 2*l - q
	f := func(t float64) uint8 {
		t = math.Mod(t+1, 1)
		var v float64
		switch {
		case t < 1.0/6:
			v = p + (q-p)*6*t
		case t < 0.5:
			v = q
		case t < 2.0/3:
			v = p + (q-p)*(2.0/3-t)*6
		default:
			v = p
		}
		return uint8(math.Round(v * 255))
	}
	return color.NRGBA{f(h + 1.0/3), f(h), f(h - 1.0/3), 255}
}

// demo artwork: a simple poster per game, generated once.
func init() {
	root := filepath.Join(os.TempDir(), "larik-demo-steam")
	demoSteamRoot = root
	for k, id := range []string{"945360", "730", "570", "894020", "578080", "445980"} {
		dir := filepath.Join(root, "appcache", "librarycache", id)
		f := filepath.Join(dir, "library_600x900.jpg")
		if _, err := os.Stat(f); err == nil {
			continue
		}
		_ = os.MkdirAll(dir, 0o755)
		img := image.NewNRGBA(image.Rect(0, 0, 300, 450))
		hue := float64(k*57 + 200)
		for y := 0; y < 450; y++ {
			for x := 0; x < 300; x++ {
				t := float64(y) / 450
				c := hsl(hue+40*t, 0.55, 0.18+0.32*(1-t))
				dx, dy := float64(x-150), float64(y-170)
				if d := math.Hypot(dx, dy); d < 80 {
					c = hsl(hue+180, 0.7, 0.55+0.2*(1-d/80))
				}
				if (x+y/2)%60 < 6 && y > 300 {
					c = hsl(hue, 0.5, 0.35)
				}
				img.SetNRGBA(x, y, c)
			}
		}
		var b bytes.Buffer
		_ = jpeg.Encode(&b, img, &jpeg.Options{Quality: 85})
		_ = os.WriteFile(f, b.Bytes(), 0o644)
	}
}

func exeIcon(path string) ([]byte, error) {
	if !safeExePath(path) {
		return nil, fmt.Errorf("no icon")
	}
	h := float64(hue(path))
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			dx, dy := math.Max(0, math.Abs(float64(x)-31.5)-20), math.Max(0, math.Abs(float64(y)-31.5)-20)
			if math.Hypot(dx, dy) < 10 {
				c := hsl(h, 0.65, 0.45+float64(y)/64*0.15)
				if math.Hypot(float64(x)-31.5, float64(y)-31.5) < 12 {
					c = color.NRGBA{255, 255, 255, 255}
				}
				img.SetNRGBA(x, y, c)
			}
		}
	}
	var b bytes.Buffer
	err := png.Encode(&b, img)
	return b.Bytes(), err
}

func hue(s string) int {
	h := 0
	for _, c := range s {
		h = (h*31 + int(c)) % 360
	}
	return h
}
