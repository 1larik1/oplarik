//go:build windows

package main

// Larik's engine runs as administrator. A game started from it directly would
// inherit those rights (Steam then warns, and drag-and-drop or overlays of
// non-admin apps stop working). So games are started with the rights of the
// signed-in user, using a copy of the desktop shell's (Explorer's) token.
// If that isn't possible, Larik never silently falls back to administrator
// rights: it reports "user_start_failed" and the UI asks first.

import (
	"errors"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	procOpenProcessToken        = advapi32.NewProc("OpenProcessToken")
	procDuplicateTokenEx        = advapi32.NewProc("DuplicateTokenEx")
	procCreateProcessWithTokenW = advapi32.NewProc("CreateProcessWithTokenW")
)

var errUserStart = errors.New("user_start_failed")

func explorerExe() string { return filepath.Join(os.Getenv("SystemRoot"), "explorer.exe") }

// protocolRegistered: is a launcher (steam, com.epicgames.launcher) installed?
func protocolRegistered(scheme string) bool {
	return (&winSys{}).RegKeyExists(`HKCR\` + scheme + `\shell\open\command`)
}

// shellToken returns a primary token copied from the desktop shell (Explorer).
func shellToken() (syscall.Token, error) {
	hw, _, _ := procGetShellWindow.Call()
	if hw == 0 {
		return 0, errors.New("no desktop shell")
	}
	var pid uint32
	procGetWindowThreadPID.Call(hw, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return 0, errors.New("no desktop shell")
	}
	h, _, err := procOpenProcess.Call(0x1000 /* PROCESS_QUERY_LIMITED_INFORMATION */, 0, uintptr(pid))
	if h == 0 {
		return 0, err
	}
	defer syscall.CloseHandle(syscall.Handle(h))
	var tok syscall.Token
	if r, _, err := procOpenProcessToken.Call(h, 0x0002 /* TOKEN_DUPLICATE */, uintptr(unsafe.Pointer(&tok))); r == 0 {
		return 0, err
	}
	defer tok.Close()
	var dup syscall.Token
	// TOKEN_QUERY | TOKEN_DUPLICATE | TOKEN_ASSIGN_PRIMARY | TOKEN_ADJUST_DEFAULT | TOKEN_ADJUST_SESSIONID
	r, _, err := procDuplicateTokenEx.Call(uintptr(tok), 0x0008|0x0002|0x0001|0x0080|0x0100, 0,
		2 /* SecurityImpersonation */, 1 /* TokenPrimary */, uintptr(unsafe.Pointer(&dup)))
	if r == 0 {
		return 0, err
	}
	return dup, nil
}

// runAsShellUser starts cmdline with the desktop user's token. Needs the
// Secondary Logon service (seclogon).
func runAsShellUser(cmdline, dir string) error {
	tok, err := shellToken()
	if err != nil {
		return err
	}
	defer tok.Close()
	cl, err := syscall.UTF16FromString(cmdline)
	if err != nil {
		return err
	}
	var d *uint16
	if dir != "" {
		d, _ = syscall.UTF16PtrFromString(dir)
	}
	si := syscall.StartupInfo{Cb: uint32(unsafe.Sizeof(syscall.StartupInfo{}))}
	var pi syscall.ProcessInformation
	r, _, err := procCreateProcessWithTokenW.Call(uintptr(tok), 0, 0, uintptr(unsafe.Pointer(&cl[0])), 0, 0,
		uintptr(unsafe.Pointer(d)), uintptr(unsafe.Pointer(&si)), uintptr(unsafe.Pointer(&pi)))
	if r == 0 {
		return err
	}
	syscall.CloseHandle(pi.Thread)
	syscall.CloseHandle(pi.Process)
	return nil
}

func startDetached(name string, args []string, dir string) error {
	c := exec.Command(name, args...)
	c.Dir = dir
	if err := c.Start(); err != nil {
		return err
	}
	return c.Process.Release()
}

// openURLAsUser hands a launcher link (steam://…) to Explorer running as the
// desktop user, which opens it with the launcher.
func openURLAsUser(u string) error {
	if !strings.HasPrefix(u, "steam://") && !strings.HasPrefix(u, "com.epicgames.launcher://") {
		return errors.New("unsupported link")
	}
	if !isElevated() {
		return startDetached(explorerExe(), []string{u}, "")
	}
	err := runAsShellUser(syscall.EscapeArg(explorerExe())+" "+syscall.EscapeArg(u), "")
	if err == nil {
		return nil
	}
	log.Print("start as user: ", err)
	// An elevated Explorer forwards the link to the running (non-elevated)
	// shell when it's the same account.
	return startDetached(explorerExe(), []string{u}, "")
}

// startExeAsUser starts a program in its own folder with the user's normal
// rights. asAdmin=true (the user agreed) allows the elevated fallback.
func startExeAsUser(path string, args []string, dir string, asAdmin bool) error {
	if !safeExePath(path) {
		return errors.New("not_found")
	}
	if !isElevated() || asAdmin {
		return startDetached(path, args, dir)
	}
	cmd := syscall.EscapeArg(path)
	for _, a := range args {
		cmd += " " + syscall.EscapeArg(a)
	}
	err := runAsShellUser(cmd, dir)
	if err == nil {
		return nil
	}
	log.Print("start as user: ", err)
	if len(args) == 0 {
		// Explorer starts a program as the desktop user too (without arguments).
		if startDetached(explorerExe(), []string{path}, "") == nil {
			return nil
		}
	}
	return errUserStart
}
