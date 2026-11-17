//go:build windows

package main

// Tray icon (--tray): a small background process with quick actions —
// open Larik, Game Boost on/off, show/hide the overlay — and a notification
// with the summary when a recorded game session ends. It only reads Larik's
// settings; Boost state is shared through boost.json and overlay visibility
// through the overlay state file, so it never overwrites the app's data.

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	procShellNotifyIconW     = shell32.NewProc("Shell_NotifyIconW")
	procLoadImageW           = user32.NewProc("LoadImageW")
	procDestroyIcon          = user32.NewProc("DestroyIcon")
	procCreatePopupMenu      = user32.NewProc("CreatePopupMenu")
	procAppendMenuW          = user32.NewProc("AppendMenuW")
	procTrackPopupMenu       = user32.NewProc("TrackPopupMenu")
	procDestroyMenu          = user32.NewProc("DestroyMenu")
	procSetForegroundWindow  = user32.NewProc("SetForegroundWindow")
	procPostMessageW         = user32.NewProc("PostMessageW")
	procRegisterWindowMsgW   = user32.NewProc("RegisterWindowMessageW")
	procGetCursorPos         = user32.NewProc("GetCursorPos")
	procChangeWndMsgFilterEx = user32.NewProc("ChangeWindowMessageFilterEx")
)

const (
	nimAdd        = 0
	nimModify     = 1
	nimDelete     = 2
	nifMessage    = 0x1
	nifIcon       = 0x2
	nifTip        = 0x4
	nifInfo       = 0x10
	niifInfo      = 0x1
	wmTrayIcon    = 0x8001 // WM_APP + 1
	wmTrayRefresh = 0x8002
	wmCommand     = 0x0111
	wmNull        = 0x0000
	wmLButtonUp   = 0x0202
	wmRButtonUp   = 0x0205
	wmContextMnu  = 0x007B
	mfString      = 0x0
	mfGrayed      = 0x1
	mfChecked     = 0x8
	mfSeparator   = 0x800
	tpmRightBtn   = 0x2
	tpmReturnCmd  = 0x100

	cmdOpen    = 1
	cmdBoost   = 2
	cmdOverlay = 3
	cmdHide    = 4
)

// notifyIconData mirrors NOTIFYICONDATAW (x64, 976 bytes).
type notifyIconData struct {
	Size            uint32
	_               uint32
	Wnd             uintptr
	ID              uint32
	Flags           uint32
	CallbackMessage uint32
	_               uint32
	Icon            uintptr
	Tip             [128]uint16
	State           uint32
	StateMask       uint32
	Info            [256]uint16
	Version         uint32
	InfoTitle       [64]uint16
	InfoFlags       uint32
	GUIDItem        [16]byte
	BalloonIcon     uintptr
}

func setUTF16(dst []uint16, s string) {
	u, _ := syscall.UTF16FromString(s)
	if len(u) > len(dst) {
		u = u[:len(dst)]
		u[len(u)-1] = 0
	}
	copy(dst, u)
}

type trayProc struct {
	hwnd       uintptr
	icon       uintptr
	nid        notifyIconData
	taskbarMsg uintptr
	lang       string
	setMod     time.Time
	sesMod     time.Time
	lastEnd    string // End of the newest session already seen (RFC 3339)
	admin      bool
	added      bool // NIM_ADD succeeded
	busy       sync.WaitGroup
}

var theTray *trayProc

func trayStatusPath() string { return filepath.Join(localDir(), "tray.json") }

type trayStatusFile struct {
	PID   int    `json:"pid"`
	At    int64  `json:"at"`
	Admin bool   `json:"admin"`
	Ver   string `json:"ver"`
}

func trayStatus() (trayStatusFile, bool) {
	var st trayStatusFile
	b, err := os.ReadFile(trayStatusPath())
	if err != nil || json.Unmarshal(b, &st) != nil {
		return st, false
	}
	return st, time.Since(time.UnixMilli(st.At)) < 15*time.Second
}

// ensureTray starts the tray process when it's enabled and not running. A
// tray started without admin rights is replaced once the engine has them,
// so Game Boost works from it.
func ensureTray(st Settings) {
	if !st.Tray {
		return
	}
	if cur, ok := trayStatus(); ok {
		// Replace a tray started without admin rights or by an older version.
		if (cur.Ver == appVersion && (cur.Admin || !isElevated())) || cur.PID <= 0 || cur.PID == os.Getpid() || !isLarikProcess(cur.PID) {
			return
		}
		p, err := os.FindProcess(cur.PID)
		if err != nil {
			return
		}
		_ = p.Kill()
		done := make(chan struct{})
		go func() { _, _ = p.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			p.Release()
			return
		}
		p.Release()
		_ = os.Remove(trayStatusPath())
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	if err := exec.Command(exe, "--tray").Start(); err != nil {
		log.Print("tray start: ", err)
	}
}

func syncTrayAutostart(st Settings) error {
	return syncLogonTask("Larik Tray", "--tray", "OpLarik tray icon", st.Tray && st.TrayAuto)
}

func (t *trayProc) loadSettings() (Settings, bool) {
	b, err := os.ReadFile(settingsPath())
	if err != nil {
		return Settings{}, false
	}
	st := Settings{Overlay: defaultOverlay()}
	if json.Unmarshal(b, &st) != nil {
		return Settings{}, false
	}
	return st, true
}

func (t *trayProc) addIcon() {
	t.nid.Size = uint32(unsafe.Sizeof(t.nid))
	t.nid.Wnd = t.hwnd
	t.nid.ID = 1
	t.nid.Flags = nifMessage | nifIcon | nifTip
	t.nid.CallbackMessage = wmTrayIcon
	t.nid.Icon = t.icon
	t.updateTip()
	r, _, _ := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&t.nid)))
	t.added = r != 0 // fails at logon while Explorer is still starting; tick() retries
}

func (t *trayProc) updateTip() {
	tip := appName
	if loadBoost() != nil {
		tip = nt(t.lang, "trayBoostOn")
	}
	t.nid.Tip = [128]uint16{}
	setUTF16(t.nid.Tip[:], tip)
}

func (t *trayProc) notify(title, body string) {
	n := t.nid
	n.Flags = nifInfo
	n.InfoFlags = niifInfo
	setUTF16(n.InfoTitle[:], title)
	setUTF16(n.Info[:], body)
	procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&n)))
}

func (t *trayProc) menu() {
	st, _ := t.loadSettings()
	m, _, _ := procCreatePopupMenu.Call()
	if m == 0 {
		return
	}
	defer procDestroyMenu.Call(m)
	add := func(flags uintptr, id uintptr, text string) {
		procAppendMenuW.Call(m, flags, id, uintptr(unsafe.Pointer(u16(text))))
	}
	add(mfString, cmdOpen, nt(t.lang, "trayOpen"))
	add(mfSeparator, 0, "")
	if t.admin {
		f := uintptr(mfString)
		if loadBoost() != nil {
			f |= mfChecked
		}
		add(f, cmdBoost, nt(t.lang, "trayBoost"))
	} else {
		add(mfString|mfGrayed, cmdBoost, nt(t.lang, "trayNeedAdmin"))
	}
	if st.Overlay.Enabled && t.admin {
		f := uintptr(mfString)
		if readOverlayVisible() {
			f |= mfChecked
		}
		add(f, cmdOverlay, nt(t.lang, "trayOverlay"))
	} else if !st.Overlay.Enabled {
		add(mfString|mfGrayed, cmdOverlay, nt(t.lang, "trayOverlayOff"))
	} else {
		add(mfString|mfGrayed, cmdOverlay, nt(t.lang, "trayOverlay"))
	}
	if list := loadSessions(localDir()); len(list) > 0 {
		add(mfSeparator, 0, "")
		add(mfString|mfGrayed, 0, fmt.Sprintf(nt(t.lang, "trayLast"), list[0].Game, fmtNum(list[0].AvgFPS, 0, t.lang)))
	}
	add(mfSeparator, 0, "")
	add(mfString, cmdHide, nt(t.lang, "trayHide"))

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(t.hwnd) // so the menu closes when you click elsewhere
	cmd, _, _ := procTrackPopupMenu.Call(m, tpmRightBtn|tpmReturnCmd, uintptr(pt.X), uintptr(pt.Y), 0, t.hwnd, 0)
	procPostMessageW.Call(t.hwnd, wmNull, 0, 0)
	t.run(cmd)
}

func (t *trayProc) run(cmd uintptr) {
	switch cmd {
	case cmdOpen:
		if exe, err := os.Executable(); err == nil {
			_ = exec.Command(exe).Start()
		}
	case cmdBoost:
		t.busy.Add(1)
		go func() {
			defer t.busy.Done()
			on := loadBoost() == nil
			if _, err := setBoost(newSys(), on); err != nil {
				log.Print("tray boost: ", err)
			}
			// update the tooltip on the window's own thread
			procPostMessageW.Call(t.hwnd, wmTrayRefresh, 0, 0)
		}()
	case cmdOverlay:
		_ = writeOverlayVisible(!readOverlayVisible())
	case cmdHide:
		procDestroyWindow.Call(t.hwnd)
	}
}

// tick: exit when the tray is switched off, refresh language, notify about
// finished game sessions, and keep the heartbeat file fresh.
func (t *trayProc) tick() bool {
	if m := modTime(settingsPath()); !m.Equal(t.setMod) {
		st, ok := t.loadSettings()
		if ok {
			t.setMod = m
			if !st.Tray {
				return false
			}
			t.lang = st.Lang
			if t.lang == "" {
				t.lang = systemLang()
			}
		}
	}
	if !t.added {
		t.addIcon()
	}
	if m := modTime(sessionsFile(localDir())); !m.Equal(t.sesMod) {
		t.sesMod = m
		// Only a session that ended after the newest one we've seen is new;
		// deleting sessions in the app never triggers a notification.
		if list := loadSessions(localDir()); len(list) > 0 && list[0].End > t.lastEnd {
			title, body := sessionNotice(t.lang, list[0])
			t.notify(title, body)
			t.lastEnd = list[0].End
		}
	}
	_ = writeAtomic(trayStatusPath(), trayStatusFile{PID: os.Getpid(), At: time.Now().UnixMilli(), Admin: t.admin, Ver: appVersion})
	return true
}

func trayWndProc(hwnd, msg, wparam, lparam uintptr) uintptr {
	t := theTray
	switch {
	case msg == wmTrayIcon:
		switch lparam & 0xffff {
		case wmRButtonUp, wmContextMnu:
			t.menu()
		case wmLButtonUp:
			t.run(cmdOpen)
		}
		return 0
	case msg == wmTrayRefresh:
		t.updateTip()
		n := t.nid
		n.Flags = nifTip
		procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&n)))
		return 0
	case msg == wmTimer:
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Print("tray tick: ", r)
				}
			}()
			if !t.tick() {
				procDestroyWindow.Call(hwnd)
			}
		}()
		return 0
	case t != nil && t.taskbarMsg != 0 && msg == t.taskbarMsg:
		t.addIcon() // Explorer restarted: put the icon back
		return 0
	case msg == wmDestroy:
		procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&t.nid)))
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, msg, wparam, lparam)
	return r
}

func runTray() {
	runtime.LockOSThread()
	h, _, e := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(u16(`Local\LarikTray`))))
	if h == 0 || e == syscall.Errno(183) {
		return
	}
	t := &trayProc{admin: isElevated(), lang: uiLang()}
	st, ok := t.loadSettings()
	if !ok || !st.Tray {
		return
	}
	t.setMod = modTime(settingsPath())
	for _, ss := range loadSessions(localDir()) { // sessions from before the tray started
		if ss.End > t.lastEnd {
			t.lastEnd = ss.End
		}
	}
	t.sesMod = modTime(sessionsFile(localDir()))
	theTray = t
	inst, _, _ := procGetModuleHandleW.Call(0)
	cls := u16("LarikTray")
	wc := wndClassEx{WndProc: syscall.NewCallback(trayWndProc), Instance: inst, ClassName: cls}
	wc.Size = uint32(unsafe.Sizeof(wc))
	if r, _, err := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		log.Print("tray class: ", err)
		return
	}
	t.hwnd, _, _ = procCreateWindowExW.Call(wsExToolWindow, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(u16("OpLarik Tray"))), wsPopup, 0, 0, 0, 0, 0, 0, inst, 0)
	if t.hwnd == 0 {
		log.Print("tray window failed")
		return
	}
	cx, _, _ := procGetSysMetrics.Call(49) // SM_CXSMICON
	cy, _, _ := procGetSysMetrics.Call(50)
	t.icon, _, _ = procLoadImageW.Call(inst, 1 /* icon resource id */, 1 /* IMAGE_ICON */, cx, cy, 0)
	t.taskbarMsg, _, _ = procRegisterWindowMsgW.Call(uintptr(unsafe.Pointer(u16("TaskbarCreated"))))
	// Explorer isn't elevated, so let its "taskbar re-created" message through
	// to this elevated window (otherwise the icon is lost after an Explorer restart).
	if t.taskbarMsg != 0 && procChangeWndMsgFilterEx.Find() == nil {
		procChangeWndMsgFilterEx.Call(t.hwnd, t.taskbarMsg, 1 /* MSGFLT_ALLOW */, 0)
	}
	t.addIcon()
	t.tick()
	procSetTimer.Call(t.hwnd, 1, 3000, 0)
	log.Print("tray started")

	var m winMsg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if r == 0 || int32(r) == -1 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	t.busy.Wait() // let a running Boost switch finish writing its state
	if t.icon != 0 {
		procDestroyIcon.Call(t.icon)
	}
	_ = os.Remove(trayStatusPath())
	log.Print("tray stopped")
}
