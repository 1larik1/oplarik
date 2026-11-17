//go:build windows

package main

import (
	"errors"
	"sync"
	"syscall"
	"unsafe"
)

var (
	crypt32                = syscall.NewLazyDLL("crypt32.dll")
	procCryptProtectData   = crypt32.NewProc("CryptProtectData")
	procCryptUnprotectData = crypt32.NewProc("CryptUnprotectData")
	procLocalFree          = kernel32.NewProc("LocalFree")
)

type dataBlob struct {
	Size uint32
	Data *byte
}

const (
	cryptprotectUIForbidden  = 0x1
	cryptprotectLocalMachine = 0x4
)

var dpapiEntropy = []byte("Larik.Account.v1")

func blobOf(b []byte) *dataBlob {
	if len(b) == 0 {
		return &dataBlob{}
	}
	return &dataBlob{Size: uint32(len(b)), Data: &b[0]}
}

func (b *dataBlob) bytes() []byte {
	out := make([]byte, b.Size)
	copy(out, unsafe.Slice(b.Data, b.Size))
	return out
}

// protectSecret encrypts with DPAPI (machine scope: the account is shared by
// every Windows user of this PC; the folder itself is admin-only).
func protectSecret(plain []byte) ([]byte, error) {
	var out dataBlob
	r, _, err := procCryptProtectData.Call(uintptr(unsafe.Pointer(blobOf(plain))), 0, uintptr(unsafe.Pointer(blobOf(dpapiEntropy))),
		0, 0, cryptprotectUIForbidden|cryptprotectLocalMachine, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, err
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.Data)))
	return out.bytes(), nil
}

func unprotectSecret(enc []byte) ([]byte, error) {
	if len(enc) == 0 {
		return nil, errors.New("empty")
	}
	var out dataBlob
	r, _, err := procCryptUnprotectData.Call(uintptr(unsafe.Pointer(blobOf(enc))), 0, uintptr(unsafe.Pointer(blobOf(dpapiEntropy))),
		0, 0, cryptprotectUIForbidden, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, err
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.Data)))
	return out.bytes(), nil
}

var securedDirs sync.Map

// secureDir limits the account folder to Administrators and SYSTEM, so
// standard users can't read the password hash.
func secureDir(dir string) error {
	if _, done := securedDirs.Load(dir); done {
		return nil
	}
	if _, err := (&winSys{}).Exec("icacls", dir, "/inheritance:r", "/grant:r", "*S-1-5-32-544:(OI)(CI)F", "*S-1-5-18:(OI)(CI)F"); err != nil {
		return err
	}
	securedDirs.Store(dir, true)
	return nil
}
