package main

import (
	"strings"
	"time"
)

type GPUInfo struct {
	Name       string `json:"name"`
	Vendor     string `json:"vendor"`
	VRAM       uint64 `json:"vram"`
	Driver     string `json:"driver"`
	DriverDate string `json:"driverDate"`
	Res        string `json:"res"`
	Hz         int    `json:"hz"`
	ReBAR      string `json:"rebar,omitempty"` // on | off | "" (unknown / not supported)
}

type RAMModule struct {
	Size     uint64 `json:"size"`
	Speed    int    `json:"speed"`
	Conf     int    `json:"conf"`
	Maker    string `json:"maker"`
	Part     string `json:"part"`
	Slot     string `json:"slot"`
	TypeCode int    `json:"typeCode"`
}

type DiskInfo struct {
	Model  string `json:"model"`
	Media  string `json:"media"`
	Bus    string `json:"bus"`
	Size   uint64 `json:"size"`
	Health string `json:"health"`
	System bool   `json:"system"`
	Num    int    `json:"num"` // Windows disk number (PhysicalDriveN)
}

type NetInfo struct {
	Name  string `json:"name"`
	Desc  string `json:"desc"`
	Speed string `json:"speed"`
	Wifi  bool   `json:"wifi"`
}

type DisplayInfo struct {
	Name  string `json:"name"`
	Res   string `json:"res"`
	Hz    int    `json:"hz"`
	MaxHz int    `json:"maxHz,omitempty"` // highest refresh rate at this resolution
}

type SysInfo struct {
	Computer string `json:"computer"`
	User     string `json:"user"`
	OS       struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Build   int    `json:"build"`
		UBR     int    `json:"ubr"`
		Display string `json:"display"`
		Arch    string `json:"arch"`
		Install string `json:"install"`
		Uptime  int64  `json:"uptime"`
	} `json:"os"`
	CPU struct {
		Name    string `json:"name"`
		Vendor  string `json:"vendor"`
		Cores   int    `json:"cores"`
		Threads int    `json:"threads"`
		MaxMHz  int    `json:"maxMHz"`
		L2      int    `json:"l2"`
		L3      int    `json:"l3"`
		Socket  string `json:"socket"`
		Virt    bool   `json:"virt"`
	} `json:"cpu"`
	GPUs []GPUInfo `json:"gpus"`
	RAM  struct {
		Total   uint64      `json:"total"`
		Slots   int         `json:"slots"`
		Modules []RAMModule `json:"modules"`
	} `json:"ram"`
	Disks    []DiskInfo `json:"disks"`
	SysDrive struct {
		Letter string `json:"letter"`
		Total  uint64 `json:"total"`
		Free   uint64 `json:"free"`
		Media  string `json:"media"`
	} `json:"sysDrive"`
	Board struct {
		Maker    string `json:"maker"`
		Product  string `json:"product"`
		BIOS     string `json:"bios"`
		BIOSDate string `json:"biosDate"`
	} `json:"board"`
	Displays []DisplayInfo `json:"displays"`
	Net      []NetInfo     `json:"net"`
	Power    struct {
		Plan    string `json:"plan"`
		Laptop  bool   `json:"laptop"`
		Battery bool   `json:"battery"`
	} `json:"power"`
	Security struct {
		VBS        bool   `json:"vbs"`
		HVCI       bool   `json:"hvci"`
		SecureBoot string `json:"secureBoot"`
		AV         string `json:"av"`
	} `json:"security"`
	Counts struct {
		Processes int `json:"processes"`
		Services  int `json:"services"`
		Startup   int `json:"startup"`
	} `json:"counts"`
	Printers int `json:"printers"`

	// Derived by Analyze().
	Tier    string `json:"tier"`
	Tips    []Tip  `json:"tips"`
	Win11   bool   `json:"win11"`
	ScanAt  string `json:"scanAt"`
	ScanErr string `json:"scanErr,omitempty"`
}

type Tip struct {
	Code  string            `json:"code"`
	Level string            `json:"level"` // info | warn | good
	P     map[string]string `json:"p,omitempty"`
	Tweak string            `json:"tweak,omitempty"`
}

func (s *SysInfo) MaxVRAM() uint64 {
	var m uint64
	for _, g := range s.GPUs {
		if g.VRAM > m {
			m = g.VRAM
		}
	}
	return m
}

func (s *SysInfo) HasNvidia() bool {
	for _, g := range s.GPUs {
		if strings.Contains(strings.ToUpper(g.Name), "NVIDIA") || strings.Contains(strings.ToUpper(g.Vendor), "NVIDIA") {
			return true
		}
	}
	return false
}

// Analyze computes tier, tips and flags.
func (s *SysInfo) Analyze() {
	const gb = 1 << 30
	s.Win11 = s.OS.Build >= 22000
	// Windows reports "Unspecified" for many older drives: decide from the model.
	for i := range s.Disks {
		d := &s.Disks[i]
		d.Media = fixMedia(d.Media, d.Bus, d.Model, -1)
		if d.System && (s.SysDrive.Media == "" || strings.EqualFold(s.SysDrive.Media, "Unspecified")) {
			s.SysDrive.Media = d.Media
		}
	}
	ram := float64(s.RAM.Total) / gb
	vram := float64(s.MaxVRAM()) / gb
	score := 0
	switch {
	case ram >= 31:
		score += 2
	case ram >= 15:
		score++
	}
	switch {
	case vram >= 10:
		score += 2
	case vram >= 6:
		score++
	}
	switch {
	case s.CPU.Threads >= 16:
		score += 2
	case s.CPU.Threads >= 8:
		score++
	}
	switch {
	case score <= 2:
		s.Tier = "entry"
	case score <= 4:
		s.Tier = "mid"
	default:
		s.Tier = "high"
	}

	s.Tips = nil
	add := func(code, level string, p map[string]string, tweak string) {
		s.Tips = append(s.Tips, Tip{Code: code, Level: level, P: p, Tweak: tweak})
	}
	if n := len(s.RAM.Modules); n == 1 {
		add("ram_single", "warn", nil, "")
	}
	rated, conf := 0, 0
	for _, m := range s.RAM.Modules {
		if m.Speed > rated {
			rated = m.Speed
		}
		if conf == 0 || (m.Conf > 0 && m.Conf < conf) {
			conf = m.Conf
		}
	}
	if rated > 0 && conf > 0 && rated-conf >= 200 && !s.Power.Laptop { // laptops rarely offer XMP
		chip, locked, limit := memLimit(s.CPU.Name, s.Board.Product)
		switch {
		case !locked:
			add("ram_xmp", "warn", map[string]string{"cur": itoa(conf), "rated": itoa(rated)}, "")
		case limit > 0 && conf <= min(rated, limit)-200:
			// Locked board, but memory still runs below what both it and the
			// memory allow.
			add("ram_xmp", "warn", map[string]string{"cur": itoa(conf), "rated": itoa(min(rated, limit))}, "")
		case limit > 0 && rated <= limit:
			// nothing to raise
		default:
			p := map[string]string{"cur": itoa(conf), "rated": itoa(rated), "chip": chip}
			add("ram_locked", "info", p, "")
		}
	}
	if ram > 0 && ram < 12 {
		add("ram_low", "info", map[string]string{"gb": itoa(int(ram + 0.5))}, "")
	}
	if s.SysDrive.Media == "HDD" {
		add("hdd_system", "warn", nil, "")
	}
	if s.SysDrive.Total > 0 {
		free := float64(s.SysDrive.Free) / gb
		if free < 20 || float64(s.SysDrive.Free)/float64(s.SysDrive.Total) < 0.1 {
			add("disk_low", "warn", map[string]string{"gb": itoa(int(free))}, "")
		}
	}
	wifi, eth := false, false
	for _, n := range s.Net {
		if n.Wifi {
			wifi = true
		} else {
			eth = true
		}
	}
	if wifi && !eth {
		add("wifi", "info", nil, "")
	}
	for _, d := range s.Displays {
		if d.Hz > 0 && d.MaxHz >= d.Hz+10 {
			add("hz_low", "warn", map[string]string{"hz": itoa(d.Hz), "max": itoa(d.MaxHz), "name": d.Name}, "")
			break
		}
		if d.Hz > 0 && d.Hz <= 60 {
			add("hz60", "info", map[string]string{"hz": itoa(d.Hz)}, "")
			break
		}
	}
	for _, g := range s.GPUs {
		if g.ReBAR == "off" && rebarCapable(g.Name) {
			p := map[string]string{"gpu": g.Name}
			if s.Security.SecureBoot == "unsupported" {
				p["legacy"] = "1" // Windows boots in legacy (CSM) mode
			}
			add("rebar_off", "info", p, "")
			break
		}
	}
	if s.Security.VBS || s.Security.HVCI {
		add("vbs", "info", nil, "vbs_off")
	}
	for _, g := range s.GPUs {
		if t, err := time.Parse("2006-01-02", g.DriverDate); err == nil && time.Since(t) > 270*24*time.Hour && g.Vendor != "Microsoft" {
			add("driver_old", "warn", map[string]string{"date": g.DriverDate, "gpu": g.Name}, "")
			break
		}
	}
	if s.OS.Uptime > 7*24*3600 {
		add("uptime", "info", map[string]string{"days": itoa(int(s.OS.Uptime / 86400))}, "")
	}
	if s.Counts.Startup > 8 {
		add("startup", "info", map[string]string{"n": itoa(s.Counts.Startup)}, "")
	}
	if s.Power.Laptop {
		add("laptop", "info", nil, "")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// Cond evaluates a catalog condition against this PC. Unknown info = true.
func (s *SysInfo) Cond(name string) bool {
	if s == nil || s.ScanAt == "" {
		return true
	}
	switch name {
	case "desktop":
		return !s.Power.Laptop
	case "ssd":
		return s.SysDrive.Media == "SSD" // unknown drives get no SSD-only tweaks
	case "ram16":
		return s.RAM.Total >= 15*(1<<30)
	case "nvidia":
		return s.HasNvidia()
	case "noprinter":
		return s.Printers == 0
	case "win11":
		return s.Win11
	}
	return true
}
