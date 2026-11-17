//go:build windows

package main

import (
	"strconv"
	"strings"
	"unsafe"
)

var (
	procSPI          = user32.NewProc("SystemParametersInfoW")
	procSendMsgTmout = user32.NewProc("SendMessageTimeoutW")
)

// postTweak applies settings that Windows only reads at sign-in, so the
// change is visible right away.
func postTweak(sys Sys, id string) {
	defer func() { recover() }()
	switch id {
	case "mouse_accel_off":
		get := func(n string, def int) int32 {
			v, _ := sys.RegGet(`HKCU\Control Panel\Mouse`, n)
			s, ok := v.Str()
			if !ok {
				return int32(def)
			}
			i, err := strconv.Atoi(strings.TrimSpace(s))
			if err != nil {
				return int32(def)
			}
			return int32(i)
		}
		p := [3]int32{get("MouseThreshold1", 6), get("MouseThreshold2", 10), get("MouseSpeed", 1)}
		procSPI.Call(0x0004 /* SPI_SETMOUSE */, 0, uintptr(unsafe.Pointer(&p[0])), 3 /* UPDATEINIFILE|SENDCHANGE */)
	case "transparency_off":
		var res uintptr
		procSendMsgTmout.Call(0xFFFF /* HWND_BROADCAST */, 0x001A /* WM_SETTINGCHANGE */, 0,
			uintptr(unsafe.Pointer(u16("ImmersiveColorSet"))), 0x0002 /* SMTO_ABORTIFHUNG */, 2000, uintptr(unsafe.Pointer(&res)))
	}
}
