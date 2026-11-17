//go:build windows

package main

// Load DLLs only from System32. Without this, Windows also searches the folder
// Larik.exe is in, so a DLL named like one of ours (nvapi64.dll, pdh.dll…)
// dropped next to the exe in Downloads would run with administrator rights.

import "syscall"

func init() {
	k := syscall.NewLazyDLL("kernel32.dll") // KnownDLL: always the system copy
	if p := k.NewProc("SetDefaultDllDirectories"); p.Find() == nil {
		p.Call(0x00000800) // LOAD_LIBRARY_SEARCH_SYSTEM32
	}
}
