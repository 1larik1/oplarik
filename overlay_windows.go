//go:build windows

package main

import (
	"bufio"
	"encoding/json"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	gdi32                   = syscall.NewLazyDLL("gdi32.dll")
	shcore                  = syscall.NewLazyDLL("shcore.dll")
	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procSetTimer            = user32.NewProc("SetTimer")
	procRegisterHotKey      = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey    = user32.NewProc("UnregisterHotKey")
	procShowWindow          = user32.NewProc("ShowWindow")
	procSetWindowPos        = user32.NewProc("SetWindowPos")
	procUpdateLayeredWindow = user32.NewProc("UpdateLayeredWindow")
	procGetDC               = user32.NewProc("GetDC")
	procReleaseDC           = user32.NewProc("ReleaseDC")
	procGetForegroundWindow = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadPID  = user32.NewProc("GetWindowThreadProcessId")
	procMonitorFromWindow   = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW     = user32.NewProc("GetMonitorInfoW")
	procGetDpiForMonitor    = shcore.NewProc("GetDpiForMonitor")
	procCreateCompatibleDC  = gdi32.NewProc("CreateCompatibleDC")
	procCreateDIBSection    = gdi32.NewProc("CreateDIBSection")
	procSelectObject        = gdi32.NewProc("SelectObject")
	procDeleteObject        = gdi32.NewProc("DeleteObject")
	procDeleteDC            = gdi32.NewProc("DeleteDC")
	procCreateMutexW        = kernel32.NewProc("CreateMutexW")
	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
)

const (
	wsExLayered     = 0x00080000
	wsExTransparent = 0x00000020
	wsExTopmost     = 0x00000008
	wsExToolWindow  = 0x00000080
	wsExNoActivate  = 0x08000000
	wsPopup         = 0x80000000
	wmDestroy       = 0x0002
	wmTimer         = 0x0113
	wmHotkey        = 0x0312
	hotkeyID        = 0x4C41
)

type wndClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSm     uintptr
}

type winMsg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	PtX     int32
	PtY     int32
	Private uint32
}

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type point struct{ X, Y int32 }

// ---------------------------------------------------------------- shared files

func overlayStatePath() string  { return filepath.Join(dataDir(), "overlay-state.json") }
func overlayStatusPath() string { return filepath.Join(dataDir(), "overlay-status.json") }
func settingsPath() string      { return filepath.Join(dataDir(), "settings.json") }

type overlayStatusFile struct {
	PID     int    `json:"pid"`
	At      int64  `json:"at"`
	Hotkey  string `json:"hotkey"`
	FPS     string `json:"fps"`
	NVML    bool   `json:"nvml"`
	CPUTemp bool   `json:"cpuTemp"`
	Visible bool   `json:"visible"`
	Admin   bool   `json:"admin"`
	Ver     string `json:"ver"`
	Overlay bool   `json:"overlay"` // the overlay itself is on (not only automatic game mode)
	Auto    bool   `json:"auto"`    // automatic game mode is watching
	Game    string `json:"game"`    // game automatic mode detected
}

func readOverlayVisible() bool {
	b, err := os.ReadFile(overlayStatePath())
	if err != nil {
		return true
	}
	var s struct {
		Visible bool `json:"visible"`
	}
	if json.Unmarshal(b, &s) != nil {
		return true
	}
	return s.Visible
}

func writeOverlayVisible(v bool) error {
	return writeAtomic(overlayStatePath(), map[string]bool{"visible": v})
}

// ---------------------------------------------------------------- engine side

func overlayStatus() map[string]any {
	out := map[string]any{"running": false, "platform": true}
	b, err := os.ReadFile(overlayStatusPath())
	if err != nil {
		return out
	}
	var st overlayStatusFile
	if json.Unmarshal(b, &st) != nil {
		return out
	}
	running := time.Since(time.UnixMilli(st.At)) < 6*time.Second
	out["running"] = running
	if running {
		out["hotkey"], out["fps"], out["nvml"], out["cpuTemp"], out["visible"], out["admin"] = st.Hotkey, st.FPS, st.NVML, st.CPUTemp, st.Visible, st.Admin
		out["ver"], out["game"], out["auto"] = st.Ver, st.Game, st.Auto
	}
	return out
}

// overlayWanted: the overlay process also runs for automatic game mode.
func overlayWanted(st Settings) bool { return st.Overlay.Enabled || st.AutoGame.Enabled }

func ensureOverlay(st Settings) {
	if !overlayWanted(st) {
		return
	}
	cur := overlayStatus()
	if r, _ := cur["running"].(bool); r {
		// An overlay started without admin rights can't measure FPS, and one
		// left over from an older Larik version doesn't know the new settings:
		// replace either with a fresh one.
		admin, _ := cur["admin"].(bool)
		ver, _ := cur["ver"].(string)
		stale := ver != appVersion
		if (!stale && (admin || !isElevated())) || !replaceOverlay() {
			return
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if err := exec.Command(exe, "--overlay").Start(); err != nil {
		log.Print("overlay start: ", err)
	}
}

// replaceOverlay stops the running (non-elevated) overlay so a new one can start.
func replaceOverlay() bool {
	b, err := os.ReadFile(overlayStatusPath())
	if err != nil {
		return false
	}
	var st overlayStatusFile
	if json.Unmarshal(b, &st) != nil || st.PID <= 0 || st.PID == os.Getpid() || !isLarikProcess(st.PID) {
		return false
	}
	p, err := os.FindProcess(st.PID)
	if err != nil {
		return false
	}
	defer p.Release()
	if p.Kill() != nil {
		return false
	}
	done := make(chan struct{})
	go func() { _, _ = p.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		return false
	}
	_ = os.Remove(overlayStatusPath())
	log.Print("overlay: replaced non-elevated instance")
	return true
}

func overlayTaskExe() string {
	inst := filepath.Join(installDir(), exeName)
	if _, err := os.Stat(inst); err == nil {
		return inst
	}
	exe, _ := os.Executable()
	return exe
}

func xmlEsc(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// syncAutostart creates or removes the logon task that starts the overlay
// with administrator rights (needed for FPS tracing) without a UAC prompt.
func syncAutostart(st Settings) error {
	on := (st.Overlay.Enabled && st.Overlay.Autostart) || st.AutoGame.Enabled
	return syncLogonTask("Larik Overlay", "--overlay", "OpLarik in-game overlay and automatic game mode", on)
}

// syncLogonTask creates or removes a logon task that starts Larik with args,
// with the user's highest rights and without a UAC prompt.
func syncLogonTask(name, args, desc string, on bool) error {
	if !on {
		deleteTask(name)
		return nil
	}
	user := os.Getenv("USERDOMAIN") + `\` + os.Getenv("USERNAME")
	trig := `<LogonTrigger><Enabled>true</Enabled><UserId>` + xmlEsc(user) + `</UserId><Delay>PT15S</Delay></LogonTrigger>`
	return registerTask(name, desc, trig, args, false, "PT0S")
}

// ---------------------------------------------------------------- overlay process

type overlayProc struct {
	hwnd     uintptr
	memDC    uintptr
	bmp      uintptr
	origBmp  uintptr
	bits     unsafe.Pointer
	bw, bh   int
	cfg      OverlayConfig
	auto     AutoGameConfig
	lang     string
	visible  bool
	setMod   time.Time
	stateMod time.Time
	interval int
	hotkey   string
	live     *winLive
	lastStat time.Time

	fmu      sync.Mutex
	fps      map[string][3]float64
	graphs   map[string][]float64
	servers  map[string]string
	ping     pinger
	warn     warnState
	rec      *sessionRecorder
	lastSens SensorSample
	fpsState string
	fpsAt    time.Time
	child    *exec.Cmd
	restarts []time.Time
	quit     bool

	// 1.0.7: automatic game mode + per-game profiles
	watch     gameWatch
	autoShown bool // overlay shown only because a game started
	fgExe     string
	known     map[string]bool
	knownAt   time.Time
	hkOn      bool
	boostMu   sync.Mutex
}

func modTime(p string) time.Time {
	if st, err := os.Stat(p); err == nil {
		return st.ModTime()
	}
	return time.Time{}
}

func loadOverlaySettings() (Settings, string, bool) {
	b, err := os.ReadFile(settingsPath())
	if err != nil {
		return Settings{}, "", false
	}
	st := Settings{Overlay: defaultOverlay()}
	if json.Unmarshal(b, &st) != nil {
		return Settings{}, "", false
	}
	st.Overlay.Normalize()
	lang := st.Lang
	if lang == "" {
		lang = systemLang()
	}
	st.Overlay.Theme = st.Theme
	return st, lang, true
}

func (o *overlayProc) registerHotkey() {
	procUnregisterHotKey.Call(o.hwnd, hotkeyID)
	var mods uintptr = 0x4000 // MOD_NOREPEAT
	for _, m := range o.cfg.Mods {
		switch m {
		case "alt":
			mods |= 1
		case "ctrl":
			mods |= 2
		case "shift":
			mods |= 4
		case "win":
			mods |= 8
		}
	}
	r, _, _ := procRegisterHotKey.Call(o.hwnd, hotkeyID, mods, uintptr(o.cfg.VK))
	if r != 0 {
		o.hotkey = "ok"
	} else {
		o.hotkey = "busy"
	}
}

func (o *overlayProc) reload(force bool) bool {
	sm, vm := modTime(settingsPath()), modTime(overlayStatePath())
	if !force && sm.Equal(o.setMod) && vm.Equal(o.stateMod) {
		return true
	}
	st, lang, ok := loadOverlaySettings()
	if !ok {
		// The engine may be replacing settings.json right now: keep running and
		// retry on the next tick (setMod is left unchanged). Only a first load
		// that fails ends the process.
		return o.interval != 0
	}
	if !overlayWanted(st) {
		return false
	}
	cfg := st.Overlay
	hkChanged := force || strings.Join(cfg.Mods, "+") != strings.Join(o.cfg.Mods, "+") || cfg.VK != o.cfg.VK || cfg.Enabled != o.hkOn
	o.cfg, o.auto, o.lang, o.setMod, o.stateMod = cfg, st.AutoGame, lang, sm, vm
	if !st.AutoGame.Enabled || !st.AutoGame.Overlay {
		o.autoShown = false
	}
	o.visible = readOverlayVisible() || o.autoShown
	if hkChanged {
		if cfg.Enabled {
			o.registerHotkey()
		} else {
			procUnregisterHotKey.Call(o.hwnd, hotkeyID)
			o.hotkey = ""
		}
		o.hkOn = cfg.Enabled
	}
	if cfg.Interval != o.interval {
		o.interval = cfg.Interval
		procSetTimer.Call(o.hwnd, 1, uintptr(cfg.Interval), 0)
	}
	return true
}

func foregroundPID() (uint32, uintptr) {
	hw, _, _ := procGetForegroundWindow.Call()
	var pid uint32
	if hw != 0 {
		procGetWindowThreadPID.Call(hw, uintptr(unsafe.Pointer(&pid)))
	}
	return pid, hw
}

// ---- FPS child supervision

func (o *overlayProc) startFPSMon() {
	o.fmu.Lock()
	defer o.fmu.Unlock()
	if o.quit {
		return
	}
	now := time.Now()
	var recent []time.Time
	for _, t := range o.restarts {
		if now.Sub(t) < 2*time.Minute {
			recent = append(recent, t)
		}
	}
	o.restarts = recent
	if len(recent) >= 4 {
		o.fpsState = "unavailable"
		return
	}
	o.restarts = append(o.restarts, now)
	exe, _ := os.Executable()
	cmd := exec.Command(exe, "--fpsmon")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	out, err := cmd.StdoutPipe()
	if err != nil {
		o.fpsState = "unavailable"
		return
	}
	if err := cmd.Start(); err != nil {
		o.fpsState = "unavailable"
		return
	}
	o.child = cmd
	o.fpsState = "waiting"
	go func() {
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 64*1024), 1<<20)
		for sc.Scan() {
			var m struct {
				Ready bool                  `json:"ready"`
				Error string                `json:"error"`
				P     map[string][3]float64 `json:"p"`
				G     map[string][]float64  `json:"g"`
				N     map[string]string     `json:"n"`
			}
			if json.Unmarshal(sc.Bytes(), &m) != nil {
				continue
			}
			o.fmu.Lock()
			if m.Error != "" {
				log.Print("fpsmon: ", m.Error)
				o.fpsState = "unavailable"
			} else if m.Ready {
				o.fpsState = "ok"
			} else {
				o.fps, o.graphs, o.servers, o.fpsAt, o.fpsState = m.P, m.G, m.N, time.Now(), "ok"
			}
			o.fmu.Unlock()
		}
		_ = cmd.Wait()
		time.Sleep(3 * time.Second)
		o.startFPSMon()
	}()
}

func (o *overlayProc) stopFPSMon() {
	o.fmu.Lock()
	o.quit = true
	c := o.child
	o.fmu.Unlock()
	if c != nil && c.Process != nil {
		_ = c.Process.Kill()
	}
	stopETWSession()
}

func (o *overlayProc) sample() OverlayData {
	d := naData()
	l := o.live.Sample()
	d.CPU = l.CPU
	d.RAMUsed, d.RAMTotal = float64(l.RAMUsed), float64(l.RAMTotal)
	if l.GPU >= 0 {
		d.GPU = l.GPU
	}
	if l.VRAM > 0 {
		d.VRAM = float64(l.VRAM)
	}
	s := sensors.Sample()
	o.lastSens = s
	d.GPUTemp, d.GPUClock, d.GPUPower, d.CPUTemp = s.GPUTemp, s.GPUClock, s.GPUPower, s.CPUTemp
	if s.GPUUtil >= 0 {
		d.GPU = s.GPUUtil
	}
	if s.VRAMUsed > 0 {
		d.VRAM = s.VRAMUsed
	}
	pid, hwnd := foregroundPID()
	key := strconv.FormatUint(uint64(pid), 10)
	server := ""
	o.fmu.Lock()
	if time.Since(o.fpsAt) < 3*time.Second {
		if v, ok := o.fps[key]; ok {
			d.FPS, d.Low1, d.FrameTime = v[0], v[1], v[2]
			d.FrameTimes = append([]float64(nil), o.graphs[key]...)
			server = o.servers[key]
		}
	}
	o.fmu.Unlock()
	d.Ping = o.ping.Get(server)
	d.Warn = o.warn.update(time.Now(), s, d.CPU, d.GPU)
	d.Clock = time.Now().Format("15:04")
	exe := processExe(pid)
	o.fgExe = exe
	full := !isIgnoredExe(exe) && fullscreenWindow(hwnd)
	if o.rec != nil && !o.cfg.RecordOff {
		o.rec.Tick(time.Now(), pid, exe, full, o.lookupFPS, d)
	}
	if o.auto.Enabled {
		fps, _, _, hasFPS := o.lookupFPS(pid)
		eligible := full && ((hasFPS && fps > 0) || o.knownGame(exe))
		switch o.watch.Tick(time.Now(), pid, exe, eligible, processAlive) {
		case "start":
			o.onGameStart(o.watch.Game())
		case "end":
			o.onGameEnd()
		}
	} else if o.watch.pid != 0 {
		o.watch = gameWatch{}
		o.onGameEnd()
	}
	return d
}

// knownGame: games from Larik's Games page and the built-in popular list
// count as games even when FPS can't be measured (no admin rights).
func (o *overlayProc) knownGame(exe string) bool {
	if exe == "" {
		return false
	}
	if o.known == nil || time.Since(o.knownAt) > time.Minute {
		o.known, o.knownAt = map[string]bool{}, time.Now()
		for _, g := range popularGames {
			o.known[strings.ToLower(g.Exe)] = true
		}
		if b, err := os.ReadFile(filepath.Join(dataDir(), "state.json")); err == nil {
			var d struct {
				Games []struct {
					Exe string `json:"exe"`
				} `json:"games"`
			}
			if json.Unmarshal(b, &d) == nil {
				for _, g := range d.Games {
					o.known[strings.ToLower(g.Exe)] = true
				}
			}
		}
	}
	return o.known[strings.ToLower(exe)]
}

func (o *overlayProc) onGameStart(exe string) {
	log.Printf("auto game mode: %s started", exe)
	if o.auto.Overlay && o.cfg.Enabled && !o.visible {
		o.autoShown, o.visible = true, true
	}
	if o.auto.Boost {
		go func() {
			o.boostMu.Lock()
			defer o.boostMu.Unlock()
			if err := startAutoBoost(&winSys{}, exe); err != nil {
				log.Print("auto boost: ", err)
			}
		}()
	}
}

func (o *overlayProc) onGameEnd() {
	log.Print("auto game mode: game closed")
	if o.autoShown {
		o.autoShown = false
		o.visible = readOverlayVisible()
	}
	go func() {
		o.boostMu.Lock()
		defer o.boostMu.Unlock()
		if err := endAutoBoost(&winSys{}); err != nil {
			log.Print("auto boost off: ", err)
		}
	}()
}

// lookupFPS gives the recorder the FPS of any process (the game keeps its
// session while you alt-tab).
func (o *overlayProc) lookupFPS(pid uint32) (float64, float64, float64, bool) {
	o.fmu.Lock()
	defer o.fmu.Unlock()
	if time.Since(o.fpsAt) > 3*time.Second {
		return 0, 0, 0, false
	}
	v, ok := o.fps[strconv.FormatUint(uint64(pid), 10)]
	return v[0], v[1], v[2], ok
}

func (o *overlayProc) writeStatus(s SensorSample) {
	o.fmu.Lock()
	fs := o.fpsState
	o.fmu.Unlock()
	o.lastStat = time.Now()
	_ = writeAtomic(overlayStatusPath(), overlayStatusFile{PID: os.Getpid(), At: time.Now().UnixMilli(), Hotkey: o.hotkey,
		FPS: fs, NVML: s.NVML, CPUTemp: s.CPUTempOK, Visible: o.visible, Admin: isElevated(), Ver: appVersion,
		Overlay: o.cfg.Enabled, Auto: o.auto.Enabled, Game: o.watch.Game()})
}

func (o *overlayProc) render(d OverlayData) {
	if !o.visible || !o.cfg.Enabled {
		procShowWindow.Call(o.hwnd, 0) // SW_HIDE
		return
	}
	cfg, _ := o.cfg.ForGame(o.fgExe) // per-game profile, if any
	_, fg := foregroundPID()
	mon, _, _ := procMonitorFromWindow.Call(fg, 1 /* MONITOR_DEFAULTTOPRIMARY */)
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi)))
	scale := 1.0
	if procGetDpiForMonitor.Find() == nil {
		var dx, dy uint32
		if r, _, _ := procGetDpiForMonitor.Call(mon, 0, uintptr(unsafe.Pointer(&dx)), uintptr(unsafe.Pointer(&dy))); r == 0 && dx > 0 {
			scale = float64(dx) / 96
		}
	}
	c, err := RenderOverlay(cfg, d, o.lang, scale)
	if err != nil {
		return
	}
	if c.W != o.bw || c.H != o.bh || o.bmp == 0 {
		bi := struct {
			H    bitmapInfoHeader
			Rgbq uint32
		}{H: bitmapInfoHeader{Size: 40, Width: int32(c.W), Height: -int32(c.H), Planes: 1, BitCount: 32}}
		var bits unsafe.Pointer
		nb, _, _ := procCreateDIBSection.Call(o.memDC, uintptr(unsafe.Pointer(&bi)), 0, uintptr(unsafe.Pointer(&bits)), 0, 0)
		if nb == 0 || bits == nil {
			if nb != 0 {
				procDeleteObject.Call(nb)
			}
			return
		}
		// Select the new bitmap first: a bitmap still selected into a DC
		// can't be deleted, which would leak a GDI handle per resize.
		prev, _, _ := procSelectObject.Call(o.memDC, nb)
		if o.bmp != 0 {
			procDeleteObject.Call(o.bmp)
		} else {
			o.origBmp = prev
		}
		o.bmp, o.bits, o.bw, o.bh = nb, bits, c.W, c.H
	}
	copy(unsafe.Slice((*uint32)(o.bits), c.W*c.H), c.Pix)

	m := int32(16 * scale)
	r := mi.Monitor
	var x, y int32
	switch cfg.Position[1] {
	case 'l':
		x = r.L + m
	case 'c':
		x = r.L + (r.R-r.L-int32(c.W))/2
	default:
		x = r.R - m - int32(c.W)
	}
	if cfg.Position[0] == 't' {
		y = r.T + m
	} else {
		y = r.B - m - int32(c.H)
	}
	pt, src := point{x, y}, point{0, 0}
	size := struct{ CX, CY int32 }{int32(c.W), int32(c.H)}
	blend := uint32(0) | uint32(0)<<8 | uint32(255)<<16 | uint32(1)<<24 // AC_SRC_OVER, 255, AC_SRC_ALPHA
	screen, _, _ := procGetDC.Call(0)
	procUpdateLayeredWindow.Call(o.hwnd, screen, uintptr(unsafe.Pointer(&pt)), uintptr(unsafe.Pointer(&size)), o.memDC,
		uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), 2 /* ULW_ALPHA */)
	procReleaseDC.Call(0, screen)
	// Re-assert topmost: games sometimes push themselves above other windows.
	procSetWindowPos.Call(o.hwnd, ^uintptr(0) /* HWND_TOPMOST */, 0, 0, 0, 0, 0x0001|0x0002|0x0010|0x0040)
}

var theOverlay *overlayProc

func overlayWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	o := theOverlay
	switch msg {
	case wmTimer:
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Print("overlay tick: ", r)
				}
			}()
			if !o.reload(false) {
				procDestroyWindow.Call(hwnd)
				return
			}
			o.render(o.sample())
			if time.Since(o.lastStat) > 1500*time.Millisecond {
				o.writeStatus(o.lastSens)
			}
		}()
		return 0
	case wmHotkey:
		if wparam == hotkeyID {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Print("overlay hotkey: ", r)
					}
				}()
				if !o.cfg.Enabled {
					return
				}
				o.visible = !o.visible
				o.autoShown = false
				_ = writeOverlayVisible(o.visible)
				o.stateMod = modTime(overlayStatePath())
				o.render(o.sample())
				o.writeStatus(o.lastSens)
			}()
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
}

// runOverlay is the --overlay process.
func runOverlay() {
	runtime.LockOSThread()
	h, _, e := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(u16(`Local\LarikOverlay`))))
	if h == 0 || e == syscall.Errno(183) { // already running (or owned by an elevated instance)
		return
	}
	if _, _, ok := loadOverlaySettings(); !ok {
		return
	}
	o := &overlayProc{live: &winLive{}, fps: map[string][3]float64{}, fpsState: "waiting", rec: newSessionRecorder(localDir())}
	theOverlay = o
	inst, _, _ := procGetModuleHandleW.Call(0)
	cls := u16("LarikOverlay")
	wc := wndClassEx{WndProc: syscall.NewCallback(overlayWndProc), Instance: inst, ClassName: cls}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		log.Print("overlay class: ", err)
		return
	}
	o.hwnd, _, _ = procCreateWindowExW.Call(wsExLayered|wsExTransparent|wsExTopmost|wsExToolWindow|wsExNoActivate,
		uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(u16("OpLarik Overlay"))), wsPopup, 0, 0, 1, 1, 0, 0, inst, 0)
	if o.hwnd == 0 {
		log.Print("overlay window failed")
		return
	}
	screen, _, _ := procGetDC.Call(0)
	o.memDC, _, _ = procCreateCompatibleDC.Call(screen)
	procReleaseDC.Call(0, screen)
	if !o.reload(true) {
		return
	}
	// Boost left on by automatic mode (e.g. the PC was shut down mid-game).
	if b := loadBoost(); b != nil && b.Auto != "" && !exeRunning(b.Auto) {
		go func() { _ = endAutoBoost(&winSys{}) }()
	}
	o.startFPSMon()
	o.render(o.sample())
	o.writeStatus(o.lastSens)
	log.Printf("overlay started (hotkey %s)", o.hotkey)

	var m winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	procUnregisterHotKey.Call(o.hwnd, hotkeyID)
	o.rec.Close(time.Now())
	// Overlay and automatic mode were switched off: end a Boost that
	// automatic mode started (Boost started by hand stays on).
	o.boostMu.Lock()
	_ = endAutoBoost(&winSys{})
	o.boostMu.Unlock()
	o.stopFPSMon()
	if o.bmp != 0 {
		procSelectObject.Call(o.memDC, o.origBmp)
		procDeleteObject.Call(o.bmp)
	}
	procDeleteDC.Call(o.memDC)
	_ = os.Remove(overlayStatusPath())
	log.Print("overlay stopped")
}
