//go:build windows

package main

import (
	"runtime"
	"syscall"
	"unsafe"
)

var (
	procWaitForSingleObject = kernel32.NewProc("WaitForSingleObject")
	procReleaseMutex        = kernel32.NewProc("ReleaseMutex")
)

// lockBoostGlobal serializes Game Boost between the engine and the tray
// process (both elevated) with a named mutex. Waits up to 60 s.
func lockBoostGlobal() func() {
	h, _, _ := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(u16(`Global\LarikBoost`))))
	if h == 0 {
		return func() {}
	}
	// A Win32 mutex belongs to the thread that took it: keep this goroutine
	// on one OS thread until it is released.
	runtime.LockOSThread()
	r, _, _ := procWaitForSingleObject.Call(h, 60000)
	if r != 0 && r != 0x80 { // WAIT_OBJECT_0 or WAIT_ABANDONED
		runtime.UnlockOSThread()
		syscall.CloseHandle(syscall.Handle(h))
		return func() {}
	}
	return func() {
		procReleaseMutex.Call(h)
		runtime.UnlockOSThread()
		syscall.CloseHandle(syscall.Handle(h))
	}
}
