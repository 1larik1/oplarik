//go:build windows

package main

// NVAPI driver settings (DRS). nvapi64.dll exports a single function,
// nvapi_QueryInterface, that returns the other entry points by ID. All calls
// return an NvAPI_Status (0 = OK). Every operation opens its own DRS session,
// loads the current settings, works on the global profile (the one NVIDIA
// Control Panel shows under "Global Settings") and saves when it changed it.

import (
	"errors"
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

const (
	nvOK              = 0
	nvSettingNotFound = -160 // NVAPI_SETTING_NOT_FOUND

	nvIDInitialize           = 0x0150E828
	nvIDCreateSession        = 0x0694D52E
	nvIDDestroySession       = 0xDAD9CFF8
	nvIDLoadSettings         = 0x375DBD6B
	nvIDSaveSettings         = 0xFCBC7E14
	nvIDGetCurrentGlobal     = 0x617BFF9F
	nvIDGetBaseProfile       = 0xDA8466A0
	nvIDGetSetting           = 0x73BF8338
	nvIDSetSetting           = 0x577DD202
	nvIDRestoreDefault       = 0x53F0381E
	nvIDDeleteProfileSetting = 0xE4A26362

	// NVDRS_SETTING_V1: version, NvAPI_UnicodeString name (2048 × u16),
	// settingId, settingType, settingLocation, isCurrentPredefined,
	// isPredefinedValid, then two 4100-byte unions (predefined, current).
	nvSettingSize     = 4 + 4096 + 4*5 + 4100 + 4100 // 12320
	nvSettingVer      = nvSettingSize | 1<<16
	nvOffID           = 4100
	nvOffType         = 4104
	nvOffLocation     = 4108
	nvOffCurrentValue = 8220
)

type nvapi struct {
	once  sync.Once
	ok    bool
	fns   map[uint32]uintptr
	query *syscall.LazyProc
}

var theNV nvapi

func (n *nvapi) init() bool {
	n.once.Do(func() {
		dll := syscall.NewLazyDLL("nvapi64.dll")
		n.query = dll.NewProc("nvapi_QueryInterface")
		if n.query.Find() != nil {
			return
		}
		n.fns = map[uint32]uintptr{}
		for _, id := range []uint32{nvIDInitialize, nvIDCreateSession, nvIDDestroySession, nvIDLoadSettings, nvIDSaveSettings,
			nvIDGetCurrentGlobal, nvIDGetBaseProfile, nvIDGetSetting, nvIDSetSetting, nvIDRestoreDefault, nvIDDeleteProfileSetting} {
			p, _, _ := n.query.Call(uintptr(id))
			n.fns[id] = p
		}
		for _, id := range []uint32{nvIDInitialize, nvIDCreateSession, nvIDDestroySession, nvIDLoadSettings, nvIDSaveSettings, nvIDGetSetting, nvIDSetSetting} {
			if n.fns[id] == 0 {
				return
			}
		}
		n.ok = nvst(syscall.SyscallN(n.fns[nvIDInitialize])) == nvOK
	})
	return n.ok
}

// Calls go straight to syscall.SyscallN (not through a helper taking
// ...uintptr) so the compiler keeps pointer arguments alive and in place.
func nvst(r, _ uintptr, _ syscall.Errno) int32 { return int32(r) }

func (n *nvapi) has(id uint32) bool { return n.fns[id] != 0 }

func nvErr(what string, st int32) error { return fmt.Errorf("NVIDIA %s failed (%d)", what, st) }

// session runs fn with a loaded DRS session and the global profile.
func (n *nvapi) session(save bool, fn func(sess, prof uintptr) error) error {
	if !n.init() {
		return errors.New("no_nvidia")
	}
	var sess uintptr
	if st := nvst(syscall.SyscallN(n.fns[nvIDCreateSession], uintptr(unsafe.Pointer(&sess)))); st != nvOK {
		return nvErr("session", st)
	}
	defer func() { syscall.SyscallN(n.fns[nvIDDestroySession], sess) }()
	if st := nvst(syscall.SyscallN(n.fns[nvIDLoadSettings], sess)); st != nvOK {
		return nvErr("load", st)
	}
	var prof uintptr
	st := int32(-3)
	if n.has(nvIDGetCurrentGlobal) {
		st = nvst(syscall.SyscallN(n.fns[nvIDGetCurrentGlobal], sess, uintptr(unsafe.Pointer(&prof))))
	}
	if st != nvOK || prof == 0 {
		if !n.has(nvIDGetBaseProfile) {
			return nvErr("profile", st)
		}
		if st = nvst(syscall.SyscallN(n.fns[nvIDGetBaseProfile], sess, uintptr(unsafe.Pointer(&prof)))); st != nvOK || prof == 0 {
			return nvErr("profile", st)
		}
	}
	if err := fn(sess, prof); err != nil {
		return err
	}
	if save {
		if st := nvst(syscall.SyscallN(n.fns[nvIDSaveSettings], sess)); st != nvOK {
			return nvErr("save", st)
		}
	}
	return nil
}

type winNV struct{}

func nvidiaDriver() nvDriver { return winNV{} }

func (winNV) Available() bool {
	if !theNV.init() {
		return false
	}
	return theNV.session(false, func(_, _ uintptr) error { return nil }) == nil
}

func (winNV) Get(id uint32) (uint32, bool, error) {
	var val uint32
	var set bool
	err := theNV.session(false, func(sess, prof uintptr) error {
		buf := make([]byte, nvSettingSize)
		*(*uint32)(unsafe.Pointer(&buf[0])) = nvSettingVer
		st := nvst(syscall.SyscallN(theNV.fns[nvIDGetSetting], sess, prof, uintptr(id), uintptr(unsafe.Pointer(&buf[0]))))
		if st == nvSettingNotFound {
			return nil // not set: driver default
		}
		if st != nvOK {
			return nvErr("read", st)
		}
		if typ := *(*uint32)(unsafe.Pointer(&buf[nvOffType])); typ != 0 { // NVDRS_DWORD_TYPE
			return errors.New("unexpected setting type")
		}
		val = *(*uint32)(unsafe.Pointer(&buf[nvOffCurrentValue]))
		// 0 = NVDRS_CURRENT_PROFILE_LOCATION: stored in this profile
		set = *(*uint32)(unsafe.Pointer(&buf[nvOffLocation])) == 0
		return nil
	})
	return val, set, err
}

func (winNV) Set(id, value uint32) error {
	return theNV.session(true, func(sess, prof uintptr) error {
		buf := make([]byte, nvSettingSize)
		*(*uint32)(unsafe.Pointer(&buf[0])) = nvSettingVer
		*(*uint32)(unsafe.Pointer(&buf[nvOffID])) = id
		*(*uint32)(unsafe.Pointer(&buf[nvOffType])) = 0 // NVDRS_DWORD_TYPE
		*(*uint32)(unsafe.Pointer(&buf[nvOffCurrentValue])) = value
		if st := nvst(syscall.SyscallN(theNV.fns[nvIDSetSetting], sess, prof, uintptr(unsafe.Pointer(&buf[0])))); st != nvOK {
			return nvErr("write", st)
		}
		return nil
	})
}

func (winNV) Restore(id uint32) error {
	return theNV.session(true, func(sess, prof uintptr) error {
		st := int32(-3)
		if theNV.has(nvIDRestoreDefault) {
			st = nvst(syscall.SyscallN(theNV.fns[nvIDRestoreDefault], sess, prof, uintptr(id)))
		}
		if st == nvOK || st == nvSettingNotFound {
			return nil
		}
		// Settings without a predefined value can't be "restored", only removed.
		if !theNV.has(nvIDDeleteProfileSetting) {
			return nvErr("restore", st)
		}
		st = nvst(syscall.SyscallN(theNV.fns[nvIDDeleteProfileSetting], sess, prof, uintptr(id)))
		if st == nvOK || st == nvSettingNotFound {
			return nil
		}
		return nvErr("restore", st)
	})
}
