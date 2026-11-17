//go:build windows

package main

import (
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	procGetSystemTimes = kernel32.NewProc("GetSystemTimes")
	pdh                = syscall.NewLazyDLL("pdh.dll")
	procPdhOpenQuery   = pdh.NewProc("PdhOpenQueryW")
	procPdhAddEnglish  = pdh.NewProc("PdhAddEnglishCounterW")
	procPdhCollect     = pdh.NewProc("PdhCollectQueryData")
	procPdhGetArray    = pdh.NewProc("PdhGetFormattedCounterArrayW")
)

const (
	pdhFmtDouble   = 0x00000200
	pdhFmtNoCap100 = 0x00008000
	pdhMoreData    = 0x800007D2
)

type winLive struct {
	mu        sync.Mutex
	lastIdle  uint64
	lastTotal uint64
	pdhOK     bool
	pdhTried  bool
	query     uintptr
	gpuCtr    uintptr
	vramCtr   uintptr
}

func newLiveSampler() LiveSampler { return &winLive{} }

func ft(f syscall.Filetime) uint64 { return uint64(f.HighDateTime)<<32 | uint64(f.LowDateTime) }

func (l *winLive) cpu() float64 {
	var idle, kern, user syscall.Filetime
	r, _, _ := procGetSystemTimes.Call(uintptr(unsafe.Pointer(&idle)), uintptr(unsafe.Pointer(&kern)), uintptr(unsafe.Pointer(&user)))
	if r == 0 {
		return 0
	}
	i, t := ft(idle), ft(kern)+ft(user)
	di, dt := i-l.lastIdle, t-l.lastTotal
	l.lastIdle, l.lastTotal = i, t
	if dt == 0 || l.lastTotal == t && di > dt {
		return 0
	}
	v := (1 - float64(di)/float64(dt)) * 100
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return v
}

func (l *winLive) initPDH() {
	l.pdhTried = true
	defer func() {
		if recover() != nil {
			l.pdhOK = false
		}
	}()
	if procPdhOpenQuery.Find() != nil || procPdhAddEnglish.Find() != nil {
		return
	}
	if r, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&l.query))); r != 0 {
		return
	}
	r1, _, _ := procPdhAddEnglish.Call(l.query, uintptr(unsafe.Pointer(u16(`\GPU Engine(*engtype_3D)\Utilization Percentage`))), 0, uintptr(unsafe.Pointer(&l.gpuCtr)))
	_, _, _ = procPdhAddEnglish.Call(l.query, uintptr(unsafe.Pointer(u16(`\GPU Adapter Memory(*)\Dedicated Usage`))), 0, uintptr(unsafe.Pointer(&l.vramCtr)))
	if r1 != 0 {
		return
	}
	procPdhCollect.Call(l.query)
	l.pdhOK = true
}

type pdhItem struct {
	Name    *uint16
	CStatus uint32
	_       uint32
	Value   float64
}

func (l *winLive) counterArray(ctr uintptr) map[string]float64 {
	out := map[string]float64{}
	if ctr == 0 {
		return out
	}
	var size, count uint32
	r, _, _ := procPdhGetArray.Call(ctr, pdhFmtDouble|pdhFmtNoCap100, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), 0)
	if uint32(r) != pdhMoreData || size == 0 {
		return out
	}
	buf := make([]byte, size)
	r, _, _ = procPdhGetArray.Call(ctr, pdhFmtDouble|pdhFmtNoCap100, uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&buf[0])))
	if r != 0 {
		return out
	}
	itemSize := unsafe.Sizeof(pdhItem{})
	for i := uint32(0); i < count; i++ {
		it := (*pdhItem)(unsafe.Pointer(&buf[uintptr(i)*itemSize]))
		if it.CStatus > 1 || it.Name == nil {
			continue
		}
		out[utf16PtrToString(it.Name)] += it.Value
	}
	return out
}

func utf16PtrToString(p *uint16) string {
	if p == nil {
		return ""
	}
	var s []uint16
	for ptr := unsafe.Pointer(p); ; ptr = unsafe.Add(ptr, 2) {
		c := *(*uint16)(ptr)
		if c == 0 {
			break
		}
		s = append(s, c)
		if len(s) > 1024 {
			break
		}
	}
	return syscall.UTF16ToString(s)
}

func (l *winLive) gpu() (float64, uint64) {
	if !l.pdhTried {
		l.initPDH()
	}
	if !l.pdhOK {
		return -1, 0
	}
	if r, _, _ := procPdhCollect.Call(l.query); r != 0 {
		return -1, 0
	}
	// Sum per physical engine (luid+phys+eng), then take the busiest engine.
	engines := map[string]float64{}
	for name, v := range l.counterArray(l.gpuCtr) {
		i := strings.Index(name, "luid_")
		j := strings.Index(name, "_engtype")
		if i < 0 || j < i {
			continue
		}
		engines[name[i:j]] += v
	}
	var max float64
	for _, v := range engines {
		if v > max {
			max = v
		}
	}
	if max > 100 {
		max = 100
	}
	var vram float64
	for _, v := range l.counterArray(l.vramCtr) {
		if v > vram {
			vram = v
		}
	}
	return max, uint64(vram)
}

func (l *winLive) Sample() LiveStats {
	l.mu.Lock()
	defer l.mu.Unlock()
	m := memStatus()
	g, vram := l.gpu()
	return LiveStats{CPU: l.cpu(), RAMUsed: m.TotalPhys - m.AvailPhys, RAMTotal: m.TotalPhys, GPU: g, VRAM: vram,
		GPUTemp: -1, CPUTemp: -1, At: time.Now().UnixMilli()}
}
