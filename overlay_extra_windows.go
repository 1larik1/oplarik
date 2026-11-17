//go:build windows

package main

// Overlay helpers added in 1.0.5: ICMP ping to the game server, process
// names and a full-screen check for the session recorder.

import (
	"encoding/binary"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	iphlpapi             = syscall.NewLazyDLL("iphlpapi.dll")
	procIcmpCreateFile   = iphlpapi.NewProc("IcmpCreateFile")
	procIcmpSendEcho     = iphlpapi.NewProc("IcmpSendEcho")
	procIcmpCloseHandle  = iphlpapi.NewProc("IcmpCloseHandle")
	procOpenProcess      = kernel32.NewProc("OpenProcess")
	procQueryFullImageW  = kernel32.NewProc("QueryFullProcessImageNameW")
	procGetWindowRect    = user32.NewProc("GetWindowRect")
	procIsIconic         = user32.NewProc("IsIconic")
	procGetShellWindow   = user32.NewProc("GetShellWindow")
	procGetDesktopWindow = user32.NewProc("GetDesktopWindow")
)

// icmpPing sends one echo request and returns the round trip, or -1.
func icmpPing(ip string, timeout time.Duration) float64 {
	a := net.ParseIP(ip).To4()
	if a == nil || procIcmpCreateFile.Find() != nil {
		return -1
	}
	h, _, _ := procIcmpCreateFile.Call()
	if h == 0 || h == ^uintptr(0) {
		return -1
	}
	defer procIcmpCloseHandle.Call(h)
	payload := []byte("LarikPing")
	reply := make([]byte, 256) // ICMP_ECHO_REPLY (40 bytes on x64) + data + room for ICMP errors
	n, _, _ := procIcmpSendEcho.Call(h, uintptr(binary.LittleEndian.Uint32(a)), uintptr(unsafe.Pointer(&payload[0])), uintptr(len(payload)),
		0, uintptr(unsafe.Pointer(&reply[0])), uintptr(len(reply)), uintptr(timeout.Milliseconds()))
	if n == 0 {
		return -1
	}
	status := binary.LittleEndian.Uint32(reply[4:])
	if status != 0 { // IP_SUCCESS
		return -1
	}
	return float64(binary.LittleEndian.Uint32(reply[8:]))
}

// pinger pings the current game server every 2 seconds in the background.
// Many servers ignore ping; after two misses the metric shows a dash.
type pinger struct {
	mu     sync.Mutex
	target string
	rtt    float64
	misses int
	once   sync.Once
}

func (p *pinger) Get(target string) float64 {
	p.once.Do(func() { go p.loop() })
	p.mu.Lock()
	defer p.mu.Unlock()
	if target != p.target {
		p.target, p.rtt, p.misses = target, -1, 0
	}
	if target == "" {
		return -1
	}
	return p.rtt
}

func (p *pinger) loop() {
	for {
		p.mu.Lock()
		t := p.target
		p.mu.Unlock()
		if t != "" {
			v := icmpPing(t, time.Second)
			p.mu.Lock()
			if p.target == t {
				if v >= 0 {
					p.rtt, p.misses = v, 0
				} else if p.misses++; p.misses >= 2 {
					p.rtt = -1
				}
			}
			p.mu.Unlock()
		}
		time.Sleep(2 * time.Second)
	}
}

type exeEntry struct {
	name string
	at   time.Time
}

var exeCache sync.Map // pid -> exeEntry; short-lived because Windows reuses PIDs

func processExe(pid uint32) string {
	if pid == 0 {
		return ""
	}
	if v, ok := exeCache.Load(pid); ok {
		if e := v.(exeEntry); time.Since(e.at) < 30*time.Second {
			return e.name
		}
	}
	h, _, _ := procOpenProcess.Call(0x1000 /* PROCESS_QUERY_LIMITED_INFORMATION */, 0, uintptr(pid))
	if h == 0 {
		exeCache.Delete(pid)
		return ""
	}
	defer syscall.CloseHandle(syscall.Handle(h))
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if r, _, _ := procQueryFullImageW.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r == 0 {
		return ""
	}
	name := filepath.Base(syscall.UTF16ToString(buf[:n]))
	exeCache.Store(pid, exeEntry{name: name, at: time.Now()})
	return name
}

// fullscreenWindow reports whether hwnd covers (almost) its whole monitor,
// like a full-screen or borderless game.
func fullscreenWindow(hwnd uintptr) bool {
	if hwnd == 0 {
		return false
	}
	if sh, _, _ := procGetShellWindow.Call(); sh == hwnd {
		return false
	}
	if dw, _, _ := procGetDesktopWindow.Call(); dw == hwnd {
		return false
	}
	if ic, _, _ := procIsIconic.Call(hwnd); ic != 0 {
		return false
	}
	var r rect
	if ok, _, _ := procGetWindowRect.Call(hwnd, uintptr(unsafe.Pointer(&r))); ok == 0 {
		return false
	}
	mon, _, _ := procMonitorFromWindow.Call(hwnd, 2 /* MONITOR_DEFAULTTONEAREST */)
	mi := monitorInfo{Size: uint32(unsafe.Sizeof(monitorInfo{}))}
	if ok, _, _ := procGetMonitorInfoW.Call(mon, uintptr(unsafe.Pointer(&mi))); ok == 0 {
		return false
	}
	mw, mh := mi.Monitor.R-mi.Monitor.L, mi.Monitor.B-mi.Monitor.T
	ww, wh := r.R-r.L, r.B-r.T
	return mw > 0 && mh > 0 && float64(ww) >= 0.95*float64(mw) && float64(wh) >= 0.95*float64(mh)
}

func isIgnoredExe(exe string) bool { return sessionIgnore[strings.ToLower(exe)] }

var procGetExitCodeProcess = kernel32.NewProc("GetExitCodeProcess")

// processAlive reports whether pid still runs (STILL_ACTIVE exit code).
func processAlive(pid uint32) bool {
	h, _, err := procOpenProcess.Call(0x1000 /* PROCESS_QUERY_LIMITED_INFORMATION */, 0, uintptr(pid))
	if h == 0 {
		// Access denied means the process exists (e.g. a protected game);
		// anything else means it's gone.
		return err == syscall.ERROR_ACCESS_DENIED
	}
	defer syscall.CloseHandle(syscall.Handle(h))
	var code uint32
	if r, _, _ := procGetExitCodeProcess.Call(h, uintptr(unsafe.Pointer(&code))); r == 0 {
		return true
	}
	return code == 259
}

// processPath returns the full image path of pid, or "".
func processPath(pid uint32) string {
	h, _, _ := procOpenProcess.Call(0x1000 /* PROCESS_QUERY_LIMITED_INFORMATION */, 0, uintptr(pid))
	if h == 0 {
		return ""
	}
	defer syscall.CloseHandle(syscall.Handle(h))
	buf := make([]uint16, 1024)
	n := uint32(len(buf))
	if r, _, _ := procQueryFullImageW.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n))); r == 0 {
		return ""
	}
	return syscall.UTF16ToString(buf[:n])
}

// isLarikProcess: the status files that name a PID live in folders other
// users can write to, so before killing a "stale overlay / tray" the engine
// checks that the process really is a copy of Larik (also an old one that an
// update renamed to Larik.exe.old-…).
func isLarikProcess(pid int) bool {
	if pid <= 0 {
		return false
	}
	img := strings.ToLower(filepath.Clean(processPath(uint32(pid))))
	if img == "" || img == "." {
		return false
	}
	legacy := filepath.Join(legacyInstallDir(), legacyExeName) // a pre-1.1.3 install still running
	for _, t := range append(updateTargets(), legacy) {
		if t != "" && strings.HasPrefix(img, strings.ToLower(filepath.Clean(t))) {
			return true
		}
	}
	return false
}

func exeRunning(exe string) bool {
	for _, p := range listProcesses() {
		if strings.EqualFold(p.Name, exe) {
			return true
		}
	}
	return false
}
