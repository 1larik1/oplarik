//go:build windows

package main

// Extra hardware facts the PowerShell scan can't give (1.0.8): whether a disk
// spins (seek penalty, the same check Windows' defragmenter uses), the
// highest refresh rate each monitor supports, and Resizable BAR on NVIDIA.

import (
	"fmt"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

var (
	procCreateFileW          = kernel32.NewProc("CreateFileW")
	procDeviceIoControl      = kernel32.NewProc("DeviceIoControl")
	procEnumDisplayDevicesW  = user32.NewProc("EnumDisplayDevicesW")
	procEnumDisplaySettingsW = user32.NewProc("EnumDisplaySettingsW")
)

// seekPenalty asks the drive whether it has a seek penalty: 1 = spinning
// disk, 0 = SSD, -1 = unknown (no answer, RAID, some USB bridges).
func seekPenalty(num int) int {
	if num < 0 || num > 64 {
		return -1
	}
	path := u16(`\\.\PhysicalDrive` + strconv.Itoa(num))
	// No access rights are needed for IOCTL_STORAGE_QUERY_PROPERTY.
	h, _, _ := procCreateFileW.Call(uintptr(unsafe.Pointer(path)), 0, 3 /* FILE_SHARE_READ|WRITE */, 0, 3 /* OPEN_EXISTING */, 0, 0)
	if h == 0 || h == ^uintptr(0) {
		return -1
	}
	defer syscall.CloseHandle(syscall.Handle(h))
	query := struct {
		PropertyID uint32 // StorageDeviceSeekPenaltyProperty
		QueryType  uint32 // PropertyStandardQuery
		Extra      [4]byte
	}{PropertyID: 7}
	var desc struct {
		Version, Size uint32
		Seek          byte
		_             [3]byte
	}
	var got uint32
	r, _, _ := procDeviceIoControl.Call(h, 0x002D1400 /* IOCTL_STORAGE_QUERY_PROPERTY */, uintptr(unsafe.Pointer(&query)), 12,
		uintptr(unsafe.Pointer(&desc)), unsafe.Sizeof(desc), uintptr(unsafe.Pointer(&got)), 0)
	if r == 0 || got < 9 {
		return -1
	}
	if desc.Seek != 0 {
		return 1
	}
	return 0
}

type displayDevice struct {
	Cb           uint32
	DeviceName   [32]uint16
	DeviceString [128]uint16
	StateFlags   uint32
	DeviceID     [128]uint16
	DeviceKey    [128]uint16
}

type devMode struct {
	DeviceName                                    [32]uint16
	SpecVersion, DriverVersion, Size, DriverExtra uint16
	Fields                                        uint32
	PosX, PosY                                    int32
	Orientation, FixedOutput                      uint32
	Color, Duplex, YResolution, TTOption, Collate int16
	FormName                                      [32]uint16
	LogPixels                                     uint16
	BitsPerPel, PelsWidth, PelsHeight             uint32
	DisplayFlags, DisplayFrequency                uint32
	ICMMethod, ICMIntent, MediaType, DitherType   uint32
	Reserved1, Reserved2                          uint32
	PanningWidth, PanningHeight                   uint32
}

type monitorMode struct {
	res       string
	hz, maxHz int
	primary   bool
}

// monitorModes lists the monitors on the desktop: current mode and the
// highest refresh rate Windows offers at the same resolution.
func monitorModes() []monitorMode {
	var out []monitorMode
	for i := uint32(0); i < 16; i++ {
		dd := displayDevice{Cb: uint32(unsafe.Sizeof(displayDevice{}))}
		if r, _, _ := procEnumDisplayDevicesW.Call(0, uintptr(i), uintptr(unsafe.Pointer(&dd)), 0); r == 0 {
			break
		}
		if dd.StateFlags&1 == 0 { // DISPLAY_DEVICE_ATTACHED_TO_DESKTOP
			continue
		}
		name := uintptr(unsafe.Pointer(&dd.DeviceName[0]))
		cur := devMode{Size: uint16(unsafe.Sizeof(devMode{}))}
		if r, _, _ := procEnumDisplaySettingsW.Call(name, ^uintptr(0) /* ENUM_CURRENT_SETTINGS */, uintptr(unsafe.Pointer(&cur))); r == 0 {
			continue
		}
		if cur.DisplayFrequency <= 1 { // 0/1 = "hardware default", not a real rate
			continue
		}
		m := monitorMode{res: fmt.Sprintf("%dx%d", cur.PelsWidth, cur.PelsHeight), hz: int(cur.DisplayFrequency), primary: dd.StateFlags&4 != 0}
		m.maxHz = m.hz
		for k := uint32(0); k < 4096; k++ {
			dm := devMode{Size: uint16(unsafe.Sizeof(devMode{}))}
			if r, _, _ := procEnumDisplaySettingsW.Call(name, uintptr(k), uintptr(unsafe.Pointer(&dm))); r == 0 {
				break
			}
			if dm.PelsWidth == cur.PelsWidth && dm.PelsHeight == cur.PelsHeight && int(dm.DisplayFrequency) > m.maxHz && dm.DisplayFrequency < 1000 {
				m.maxHz = int(dm.DisplayFrequency)
			}
		}
		if m.primary {
			out = append([]monitorMode{m}, out...)
		} else {
			out = append(out, m)
		}
	}
	return out
}

// refineScan adds what PowerShell can't see to a fresh scan.
func refineScan(info *SysInfo) {
	defer func() { _ = recover() }() // extra facts only; never fail the scan
	for i := range info.Disks {
		d := &info.Disks[i]
		if m := strings.ToUpper(d.Media); m != "SSD" && m != "HDD" {
			d.Media = fixMedia(d.Media, d.Bus, d.Model, seekPenalty(d.Num))
		}
		if d.System {
			info.SysDrive.Media = d.Media
		}
	}
	// One entry per monitor (primary first) instead of one per graphics card.
	if modes := monitorModes(); len(modes) > 0 {
		disp := make([]DisplayInfo, 0, len(modes))
		for k, m := range modes {
			d := DisplayInfo{Res: m.res, Hz: m.hz, MaxHz: m.maxHz}
			if k < len(info.Displays) {
				d.Name = info.Displays[k].Name
			}
			disp = append(disp, d)
		}
		info.Displays = disp
		for g := range info.GPUs {
			if info.GPUs[g].Res == modes[0].res {
				info.GPUs[g].Hz = modes[0].hz
				break
			}
		}
	}
	if bar1, ok := sensors.BAR1(); ok {
		for i := range info.GPUs {
			if strings.Contains(strings.ToUpper(info.GPUs[i].Name+info.GPUs[i].Vendor), "NVIDIA") {
				info.GPUs[i].ReBAR = rebarState(bar1, info.GPUs[i].VRAM)
				break
			}
		}
	}
}
