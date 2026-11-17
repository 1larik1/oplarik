//go:build windows

package main

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

// sensorReader reads NVIDIA GPU sensors through NVML (ships with the NVIDIA
// driver) and CPU temperature through Windows ACPI thermal zones.
type sensorReader struct {
	mu sync.Mutex

	nvmlTried, nvmlOK bool
	dev               uintptr
	pTemp, pUtil      *syscall.Proc
	pClock, pPower    *syscall.Proc
	pMem              *syscall.Proc
	pThrottle         *syscall.Proc
	pBar1             *syscall.Proc

	perfTried, perfOK bool
	pq, pctr          uintptr

	thermTried, thermOK bool
	tq, tctr            uintptr
	tFirst              time.Time
	tMin, tMax          float64
	tFake               bool
}

var sensors = &sensorReader{}

func (s *sensorReader) initNVML() {
	s.nvmlTried = true
	defer func() {
		if recover() != nil {
			s.nvmlOK = false
		}
	}()
	var dll *syscall.DLL
	for _, p := range []string{
		filepath.Join(os.Getenv("SystemRoot"), "System32", "nvml.dll"),
		filepath.Join(os.Getenv("ProgramFiles"), "NVIDIA Corporation", "NVSMI", "nvml.dll"),
	} {
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if d, err := syscall.LoadDLL(p); err == nil {
			dll = d
			break
		}
	}
	if dll == nil {
		return
	}
	find := func(n string) *syscall.Proc {
		p, err := dll.FindProc(n)
		if err != nil {
			return nil
		}
		return p
	}
	initp, handle := find("nvmlInit_v2"), find("nvmlDeviceGetHandleByIndex_v2")
	s.pTemp, s.pUtil = find("nvmlDeviceGetTemperature"), find("nvmlDeviceGetUtilizationRates")
	s.pClock, s.pPower, s.pMem = find("nvmlDeviceGetClockInfo"), find("nvmlDeviceGetPowerUsage"), find("nvmlDeviceGetMemoryInfo")
	// Renamed "ClocksEventReasons" in newer drivers; the old name still exists.
	if s.pThrottle = find("nvmlDeviceGetCurrentClocksEventReasons"); s.pThrottle == nil {
		s.pThrottle = find("nvmlDeviceGetCurrentClocksThrottleReasons")
	}
	s.pBar1 = find("nvmlDeviceGetBAR1MemoryInfo")
	if initp == nil || handle == nil {
		return
	}
	if r, _, _ := initp.Call(); r != 0 {
		return
	}
	if r, _, _ := handle.Call(0, uintptr(unsafe.Pointer(&s.dev))); r != 0 || s.dev == 0 {
		return
	}
	s.nvmlOK = true
}

func (s *sensorReader) initThermal() {
	s.thermTried = true
	if procPdhOpenQuery.Find() != nil {
		return
	}
	if r, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&s.tq))); r != 0 {
		return
	}
	if r, _, _ := procPdhAddEnglish.Call(s.tq, uintptr(unsafe.Pointer(u16(`\Thermal Zone Information(*)\Temperature`))), 0, uintptr(unsafe.Pointer(&s.tctr))); r != 0 {
		return
	}
	s.thermOK = true
}

// initPerf opens "% Processor Performance": current CPU speed relative to
// its base clock (over 100 = boosting). Used to spot clock drops under load.
func (s *sensorReader) initPerf() {
	s.perfTried = true
	if procPdhOpenQuery.Find() != nil {
		return
	}
	if r, _, _ := procPdhOpenQuery.Call(0, 0, uintptr(unsafe.Pointer(&s.pq))); r != 0 {
		return
	}
	if r, _, _ := procPdhAddEnglish.Call(s.pq, uintptr(unsafe.Pointer(u16(`\Processor Information(_Total)\% Processor Performance`))), 0, uintptr(unsafe.Pointer(&s.pctr))); r != 0 {
		return
	}
	procPdhCollect.Call(s.pq)
	s.perfOK = true
}

// GPU clock-event reasons that mean the card slowed itself down.
const (
	nvSwThermal  = 0x20
	nvHwThermal  = 0x40
	nvHwSlowdown = 0x08
	nvPowerBrake = 0x80
)

// BAR1 returns the size of the window the CPU uses to reach GPU memory
// (equal to the VRAM size with Resizable BAR on, 256 MB without).
func (s *sensorReader) BAR1() (total uint64, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	defer func() {
		if recover() != nil {
			total, ok = 0, false
		}
	}()
	if !s.nvmlTried {
		s.initNVML()
	}
	if !s.nvmlOK || s.pBar1 == nil {
		return 0, false
	}
	var m struct{ Total, Free, Used uint64 } // nvmlBAR1Memory_t
	if r, _, _ := s.pBar1.Call(s.dev, uintptr(unsafe.Pointer(&m))); r != 0 || m.Total == 0 {
		return 0, false
	}
	return m.Total, true
}

func (s *sensorReader) Sample() SensorSample {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := naSensors()
	func() {
		defer func() { recover() }()
		if !s.nvmlTried {
			s.initNVML()
		}
		if !s.nvmlOK {
			return
		}
		out.NVML = true
		var v uint32
		if s.pTemp != nil {
			if r, _, _ := s.pTemp.Call(s.dev, 0, uintptr(unsafe.Pointer(&v))); r == 0 {
				out.GPUTemp = float64(v)
			}
		}
		if s.pClock != nil {
			if r, _, _ := s.pClock.Call(s.dev, 0, uintptr(unsafe.Pointer(&v))); r == 0 {
				out.GPUClock = float64(v)
			}
		}
		if s.pPower != nil {
			if r, _, _ := s.pPower.Call(s.dev, uintptr(unsafe.Pointer(&v))); r == 0 {
				out.GPUPower = float64(v) / 1000
			}
		}
		if s.pUtil != nil {
			var u [2]uint32
			if r, _, _ := s.pUtil.Call(s.dev, uintptr(unsafe.Pointer(&u[0]))); r == 0 {
				out.GPUUtil = float64(u[0])
			}
		}
		if s.pMem != nil {
			var m [3]uint64
			if r, _, _ := s.pMem.Call(s.dev, uintptr(unsafe.Pointer(&m[0]))); r == 0 {
				out.VRAMUsed = float64(m[2])
			}
		}
		if s.pThrottle != nil {
			var reasons uint64
			if r, _, _ := s.pThrottle.Call(s.dev, uintptr(unsafe.Pointer(&reasons))); r == 0 {
				out.GPUThrottle = reasons&(nvSwThermal|nvHwThermal|nvHwSlowdown|nvPowerBrake) != 0
			}
		}
	}()
	func() {
		defer func() { recover() }()
		if !s.perfTried {
			s.initPerf()
		}
		if !s.perfOK {
			return
		}
		if r, _, _ := procPdhCollect.Call(s.pq); r != 0 {
			return
		}
		for _, v := range (&winLive{}).counterArray(s.pctr) {
			if v > 0 && v < 400 {
				out.CPUPerf = v
			}
		}
	}()
	func() {
		defer func() { recover() }()
		if !s.thermTried {
			s.initThermal()
		}
		if !s.thermOK || s.tFake {
			return
		}
		if r, _, _ := procPdhCollect.Call(s.tq); r != 0 {
			return
		}
		max := -1.0
		for _, k := range (&winLive{}).counterArray(s.tctr) {
			c := k - 273.15
			if c > 5 && c < 115 && c > max {
				max = c
			}
		}
		if max < 0 {
			return
		}
		// Many desktop boards expose a fixed fake zone (e.g. 27.8 °C). If the
		// value never moves for two minutes and is low, stop showing it.
		now := time.Now()
		if s.tFirst.IsZero() {
			s.tFirst, s.tMin, s.tMax = now, max, max
		}
		if max < s.tMin {
			s.tMin = max
		}
		if max > s.tMax {
			s.tMax = max
		}
		if now.Sub(s.tFirst) > 2*time.Minute && s.tMax-s.tMin < 0.5 && max < 40 {
			s.tFake = true
			return
		}
		out.CPUTemp, out.CPUTempOK = max, true
	}()
	return out
}
