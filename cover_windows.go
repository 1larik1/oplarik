//go:build windows

package main

import (
	"bytes"
	"errors"
	"image"
	"image/png"
	"os"
	"runtime"
	"unsafe"
)

var (
	procPrivateExtractIconsW = user32.NewProc("PrivateExtractIconsW")
	procGetIconInfo          = user32.NewProc("GetIconInfo")
	procGetDIBits            = gdi32.NewProc("GetDIBits")
)

var procGetDriveTypeW = kernel32.NewProc("GetDriveTypeW")

// driveLocal: "C:" is a fixed or removable disk, not a mapped network drive.
func driveLocal(vol string) bool {
	t, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(u16(vol + `\`))))
	return t == 2 || t == 3 // DRIVE_REMOVABLE, DRIVE_FIXED
}

func coverExeExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.Mode().IsRegular()
}

type iconInfo struct {
	FIcon    int32
	XHotspot uint32
	YHotspot uint32
	HbmMask  uintptr
	HbmColor uintptr
}

const iconSize = 64

// dibPixels reads a bitmap as 32-bit top-down BGRA.
func dibPixels(dc, bmp uintptr, w, h int) ([]byte, bool) {
	bi := bitmapInfoHeader{Size: 40, Width: int32(w), Height: -int32(h), Planes: 1, BitCount: 32}
	buf := make([]byte, w*h*4)
	// BITMAPINFO = header + room for color masks / palette.
	info := struct {
		H   bitmapInfoHeader
		Pal [256]uint32
	}{H: bi}
	r, _, _ := procGetDIBits.Call(dc, bmp, 0, uintptr(h), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&info)), 0 /* DIB_RGB_COLORS */)
	return buf, r != 0
}

// exeIcon extracts the program's main icon as a 64×64 PNG. The file is read
// as a resource container only (nothing in it runs).
func exeIcon(path string) ([]byte, error) {
	if !safeExePath(path) {
		return nil, errors.New("not a local program")
	}
	// GetDC / ReleaseDC must run on the same OS thread.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	var hicon uintptr
	var id uint32
	n, _, _ := procPrivateExtractIconsW.Call(uintptr(unsafe.Pointer(u16(path))), 0, iconSize, iconSize,
		uintptr(unsafe.Pointer(&hicon)), uintptr(unsafe.Pointer(&id)), 1, 0)
	if n == 0 || n == 0xFFFFFFFF || hicon == 0 {
		return nil, errors.New("no icon")
	}
	defer procDestroyIcon.Call(hicon)
	var ii iconInfo
	if r, _, _ := procGetIconInfo.Call(hicon, uintptr(unsafe.Pointer(&ii))); r == 0 {
		return nil, errors.New("no icon")
	}
	if ii.HbmMask != 0 {
		defer procDeleteObject.Call(ii.HbmMask)
	}
	if ii.HbmColor == 0 {
		return nil, errors.New("monochrome icon")
	}
	defer procDeleteObject.Call(ii.HbmColor)
	dc, _, _ := procGetDC.Call(0)
	defer procReleaseDC.Call(0, dc)
	col, ok := dibPixels(dc, ii.HbmColor, iconSize, iconSize)
	if !ok {
		return nil, errors.New("no icon")
	}
	hasAlpha := false
	for i := 3; i < len(col); i += 4 {
		if col[i] != 0 {
			hasAlpha = true
			break
		}
	}
	var mask []byte
	if !hasAlpha && ii.HbmMask != 0 {
		mask, _ = dibPixels(dc, ii.HbmMask, iconSize, iconSize)
	}
	img := image.NewNRGBA(image.Rect(0, 0, iconSize, iconSize))
	for i := 0; i < iconSize*iconSize; i++ {
		b, g, r, a := col[i*4], col[i*4+1], col[i*4+2], col[i*4+3]
		if !hasAlpha {
			a = 255
			if mask != nil && mask[i*4] != 0 { // white in the AND mask = transparent
				a = 0
			}
		}
		img.Pix[i*4], img.Pix[i*4+1], img.Pix[i*4+2], img.Pix[i*4+3] = r, g, b, a
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
