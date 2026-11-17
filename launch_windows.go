//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var (
	shell32           = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW = shell32.NewProc("ShellExecuteW")
	user32            = syscall.NewLazyDLL("user32.dll")
	procMessageBoxW   = user32.NewProc("MessageBoxW")
	procGetSysMetrics = user32.NewProc("GetSystemMetrics")
	procGetDpiForSys  = user32.NewProc("GetDpiForSystem")
	procSysParamsInfo = user32.NewProc("SystemParametersInfoW")
	kernel32          = syscall.NewLazyDLL("kernel32.dll")
	procGetUILang     = kernel32.NewProc("GetUserDefaultUILanguage")
)

func dataDir() string {
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	return filepath.Join(pd, "Larik")
}

func localDir() string {
	la := os.Getenv("LOCALAPPDATA")
	if la == "" {
		la = os.TempDir()
	}
	return filepath.Join(la, "Larik")
}

func isElevated() bool {
	var tok syscall.Token
	if err := syscall.OpenProcessToken(syscall.Handle(^uintptr(0)), syscall.TOKEN_QUERY, &tok); err != nil {
		p, _ := syscall.GetCurrentProcess()
		if err := syscall.OpenProcessToken(p, syscall.TOKEN_QUERY, &tok); err != nil {
			return false
		}
	}
	defer tok.Close()
	var elevated uint32
	var n uint32
	if err := syscall.GetTokenInformation(tok, 20 /* TokenElevation */, (*byte)(unsafe.Pointer(&elevated)), 4, &n); err != nil {
		return false
	}
	return elevated != 0
}

func shellExecute(verb, file, params string, show int) bool {
	var dir *uint16
	if wd := filepath.Dir(file); wd != "." {
		dir = u16(wd)
	}
	r, _, _ := procShellExecuteW.Call(0, uintptr(unsafe.Pointer(u16(verb))), uintptr(unsafe.Pointer(u16(file))),
		uintptr(unsafe.Pointer(u16(params))), uintptr(unsafe.Pointer(dir)), uintptr(show))
	return r > 32
}

const (
	mbOK          = 0x0
	mbYesNo       = 0x4
	mbIconInfo    = 0x40
	mbIconWarn    = 0x30
	mbIconQuest   = 0x20
	mbTopMost     = 0x40000
	mbSetForegrnd = 0x10000
	idYes         = 6
)

func msgBox(text string, flags uintptr) int {
	r, _, _ := procMessageBoxW.Call(0, uintptr(unsafe.Pointer(u16(text))), uintptr(unsafe.Pointer(u16(appName))), flags|mbTopMost|mbSetForegrnd)
	return int(r)
}

func systemLang() string {
	r, _, _ := procGetUILang.Call()
	switch r & 0x3FF {
	case 0x19:
		return "ru"
	case 0x22:
		return "uk"
	case 0x15:
		return "pl"
	case 0x0A:
		return "es"
	case 0x07:
		return "de"
	case 0x0C:
		return "fr"
	case 0x11:
		return "ja"
	case 0x05:
		return "cs"
	}
	return "en"
}

func uiLang() string {
	st := OpenStore(dataDir()).GetSettings()
	if st.Lang != "" {
		return st.Lang
	}
	return systemLang()
}

// ---------------------------------------------------------------- instance file

type instanceInfo struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
	Admin bool   `json:"admin"`
}

func instancePath() string { return filepath.Join(localDir(), "instance.json") }

func writeInstance(port int, token string, admin bool) {
	_ = os.MkdirAll(localDir(), 0o755)
	b, _ := json.Marshal(instanceInfo{Port: port, Token: token, PID: os.Getpid(), Admin: admin})
	_ = os.WriteFile(instancePath(), b, 0o600)
}

func readInstance() *instanceInfo {
	b, err := os.ReadFile(instancePath())
	if err != nil {
		return nil
	}
	var i instanceInfo
	if json.Unmarshal(b, &i) != nil || i.Port == 0 {
		return nil
	}
	return &i
}

func removeInstanceFile(token string) {
	if i := readInstance(); i != nil && i.Token == token {
		_ = os.Remove(instancePath())
	}
}

func healthy(port int, token string) bool {
	c := http.Client{Timeout: 1500 * time.Millisecond}
	// /api/auth answers even while the account is locked, and still checks the token.
	req, _ := http.NewRequest("GET", "http://127.0.0.1:"+strconv.Itoa(port)+"/api/auth", nil)
	req.Header.Set("X-Larik-Token", token)
	resp, err := c.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == 200
}

func waitHealthy(port int, token string, d time.Duration) bool {
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if healthy(port, token) {
			return true
		}
		time.Sleep(250 * time.Millisecond)
	}
	return false
}

func freePort() int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

// ---------------------------------------------------------------- window

func regDefault(key string) string {
	v, err := (&winSys{}).RegGet(key, "")
	if err != nil {
		return ""
	}
	s, _ := v.Str()
	return strings.Trim(s, `"`)
}

func findBrowser() string {
	for _, exe := range []string{"msedge.exe", "chrome.exe", "brave.exe"} {
		for _, hive := range []string{"HKLM", "HKCU"} {
			if p := regDefault(hive + `\SOFTWARE\Microsoft\Windows\CurrentVersion\App Paths\` + exe); p != "" {
				if _, err := os.Stat(p); err == nil {
					return p
				}
			}
		}
	}
	for _, p := range []string{
		filepath.Join(os.Getenv("ProgramFiles(x86)"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("ProgramFiles"), `Microsoft\Edge\Application\msedge.exe`),
		filepath.Join(os.Getenv("ProgramFiles"), `Google\Chrome\Application\chrome.exe`),
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

type rect struct{ L, T, R, B int32 }

func windowGeometry() (w, h, x, y int) {
	dpi := uintptr(96)
	if procGetDpiForSys.Find() == nil {
		if d, _, _ := procGetDpiForSys.Call(); d > 0 {
			dpi = d
		}
	}
	scale := float64(dpi) / 96
	var wa rect
	r, _, _ := procSysParamsInfo.Call(0x0030 /* SPI_GETWORKAREA */, 0, uintptr(unsafe.Pointer(&wa)), 0)
	var sw, sh float64
	var ox, oy float64
	if r != 0 && wa.R > wa.L {
		sw, sh = float64(wa.R-wa.L)/scale, float64(wa.B-wa.T)/scale
		ox, oy = float64(wa.L)/scale, float64(wa.T)/scale
	} else {
		cx, _, _ := procGetSysMetrics.Call(0)
		cy, _, _ := procGetSysMetrics.Call(1)
		sw, sh = float64(cx)/scale, float64(cy)/scale
	}
	w, h = 1360, 880
	if float64(w) > sw*0.94 {
		w = int(sw * 0.94)
	}
	if float64(h) > sh*0.94 {
		h = int(sh * 0.94)
	}
	x = int(ox + (sw-float64(w))/2)
	y = int(oy + (sh-float64(h))/2)
	return
}

func openWindow(url string) {
	b := findBrowser()
	if b == "" {
		msgBox(nt(uiLang(), "edgeMissing"), mbOK|mbIconInfo)
		shellExecute("open", url, "", 1)
		return
	}
	w, h, x, y := windowGeometry()
	args := []string{
		"--app=" + url,
		"--user-data-dir=" + filepath.Join(localDir(), "WebView"),
		fmt.Sprintf("--window-size=%d,%d", w, h),
		fmt.Sprintf("--window-position=%d,%d", x, y),
		"--no-first-run", "--no-default-browser-check", "--disable-extensions", "--disable-sync",
		"--disable-background-timer-throttling", "--disable-renderer-backgrounding",
		"--disable-backgrounding-occluded-windows", "--hide-crash-restore-bubble",
		"--disable-features=Translate,msEdgeSidebarV2,msHubApps,msEdgeShopping,msUndersideButton,EdgeCollections,msImplicitSignin",
	}
	cmd := exec.Command(b, args...)
	if err := cmd.Start(); err != nil {
		shellExecute("open", url, "", 1)
	}
}

// ---------------------------------------------------------------- entry

func platformMain(o Options) {
	if o.FPSMon {
		runFPSMon()
		return
	}
	logFile()
	if o.Overlay {
		runOverlay()
		return
	}
	if o.Tray {
		runTray()
		return
	}
	if o.Clean {
		r := runCleanup(true)
		log.Printf("scheduled cleanup: %d files, %d bytes", r.Files, r.Bytes)
		return
	}
	cleanupOldExe()
	if o.Uninstall {
		runUninstall()
		return
	}
	if o.Engine {
		runServerMode(o, isElevated(), false)
		return
	}
	if inst := readInstance(); inst != nil && healthy(inst.Port, inst.Token) {
		openWindow(appURL(inst.Port, inst.Token))
		return
	}
	if isElevated() {
		runServerMode(o, true, true)
		return
	}
	exe, _ := os.Executable()
	port, token := freePort(), randToken()
	if port != 0 && shellExecute("runas", exe, fmt.Sprintf("--engine --port %d --token %s", port, token), 0) {
		if waitHealthy(port, token, 45*time.Second) {
			openWindow(appURL(port, token))
			return
		}
		msgBox(nt(uiLang(), "engineFail"), mbOK|mbIconWarn)
		return
	}
	// UAC declined: run with limited rights so the user can still look around.
	runServerMode(o, false, true)
}

func runServerMode(o Options, admin, openUI bool) {
	s, ln := newServer(newSys(), admin, o)
	writeInstance(s.port, s.token, admin)
	if !admin {
		s.onElevate = func() (string, error) {
			exe, _ := os.Executable()
			port, token := freePort(), randToken()
			if port == 0 || !shellExecute("runas", exe, fmt.Sprintf("--engine --port %d --token %s", port, token), 0) {
				return "", fmt.Errorf("elevation was cancelled")
			}
			if !waitHealthy(port, token, 45*time.Second) {
				return "", fmt.Errorf("engine did not start")
			}
			return appURL(port, token), nil
		}
	}
	if openUI && !o.NoWindow {
		go func() {
			time.Sleep(150 * time.Millisecond)
			openWindow(appURL(s.port, s.token))
		}()
	}
	if err := s.Serve(ln); err != nil {
		log.Print(err)
	}
}

func logFile() {
	_ = os.MkdirAll(localDir(), 0o755)
	f, err := os.OpenFile(filepath.Join(localDir(), "larik.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	if st, _ := f.Stat(); st != nil && st.Size() > 2<<20 {
		_ = f.Truncate(0)
	}
	log.SetOutput(f)
	log.SetFlags(log.LstdFlags)
	log.Printf("OpLarik %s start, args=%v", appVersion, os.Args[1:])
}

func sampleSensors() SensorSample { return sensors.Sample() }
