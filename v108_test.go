package main

import (
	"strings"
	"testing"
)

func TestMediaGuess(t *testing.T) {
	cases := map[string]string{
		"WDC WD3200AAKX-073CA1": "HDD", "WDC WD1002FAEX-00Z3A0": "HDD", "ST1000DM010-2EP102": "HDD", "TOSHIBA DT01ACA100": "HDD",
		"KINGSTON SUV500240G": "SSD", "KINGSTON SA400S37480G": "SSD", "Samsung SSD 860 EVO 500GB": "SSD", "WDC WDS500G2B0A-00SM50": "SSD",
		"CT500MX500SSD1": "SSD", "Generic USB Device": "", "": "",
	}
	for m, want := range cases {
		if got := guessMediaByModel(m); got != want {
			t.Errorf("%q: %q want %q", m, got, want)
		}
	}
	if fixMedia("Unspecified", "SATA", "WDC WD3200AAKX", -1) != "HDD" || fixMedia("Unspecified", "NVMe", "x", -1) != "SSD" ||
		fixMedia("", "SATA", "Mystery", 1) != "HDD" || fixMedia("", "SATA", "WDC WD3200AAKX", 0) != "SSD" || fixMedia("SSD", "", "WDC WD3200", 1) != "SSD" {
		t.Fatal("fixMedia")
	}
}

func TestMemLimit(t *testing.T) {
	type c struct {
		cpu, board string
		locked     bool
		limit      int
	}
	for _, x := range []c{
		{"Intel(R) Core(TM) i5-10400F CPU @ 2.90GHz", "PRIME B460-PLUS", true, 2666},
		{"Intel(R) Core(TM) i7-10700 CPU @ 2.90GHz", "B460M DS3H", true, 2933},
		{"Intel(R) Core(TM) i5-10400F CPU @ 2.90GHz", "ROG STRIX Z490-E GAMING", false, 0},
		{"Intel(R) Core(TM) i5-11400F @ 2.60GHz", "PRO B560M-A", false, 0},
		{"Intel(R) Core(TM) i5-10400F CPU @ 2.90GHz", "PRO B560M-A", true, 2666},
		{"Intel(R) Core(TM) i5-11400F @ 2.60GHz", "H510M-K", true, 3200},
		{"Intel(R) Core(TM) i5-9400F CPU @ 2.90GHz", "B365M PRO-VH", true, 2666},
		{"Intel(R) Core(TM) i3-8100 CPU @ 3.60GHz", "H310M S2H", true, 2400},
		{"Intel(R) Core(TM) i5-12400F", "PRO B660M-A DDR4", false, 0},
		{"AMD Ryzen 5 3600 6-Core Processor", "B450M Pro4", false, 0},
	} {
		_, locked, limit := memLimit(x.cpu, x.board)
		if locked != x.locked || limit != x.limit {
			t.Errorf("%s / %s: %v %d", x.cpu, x.board, locked, limit)
		}
	}
}

func analyzeTips(i *SysInfo) string {
	i.ScanAt = "x"
	i.Analyze()
	var codes []string
	for _, t := range i.Tips {
		codes = append(codes, t.Code)
	}
	return strings.Join(codes, ",")
}

func TestHardwareTips(t *testing.T) {
	i := &SysInfo{}
	i.CPU.Name, i.Board.Product = "Intel(R) Core(TM) i5-10400F CPU @ 2.90GHz", "PRIME B460-PLUS"
	i.RAM.Modules = []RAMModule{{Speed: 3200, Conf: 2666}, {Speed: 3200, Conf: 2666}}
	i.GPUs = []GPUInfo{{Name: "NVIDIA GeForce RTX 3060 Ti", VRAM: 8 << 30, ReBAR: "off"}}
	i.Displays = []DisplayInfo{{Res: "1920x1080", Hz: 144, MaxHz: 144}}
	i.Disks = []DiskInfo{{Model: "KINGSTON SUV500240G", Media: "SSD", System: true}, {Model: "WDC WD3200AAKX-073CA1", Media: "Unspecified"}}
	tips := analyzeTips(i)
	if !strings.Contains(tips, "ram_locked") || strings.Contains(tips, "ram_xmp") || !strings.Contains(tips, "rebar_off") || strings.Contains(tips, "hz") {
		t.Fatal(tips)
	}
	if i.Disks[1].Media != "HDD" || i.SysDrive.Media != "SSD" {
		t.Fatalf("%+v %v", i.Disks, i.SysDrive.Media)
	}
	// Locked board but memory below what it allows: XMP advice up to the limit.
	j := &SysInfo{}
	j.CPU.Name, j.Board.Product = "Intel(R) Core(TM) i5-10400F CPU @ 2.90GHz", "PRIME B460-PLUS"
	j.RAM.Modules = []RAMModule{{Speed: 3200, Conf: 2133}}
	j.Displays = []DisplayInfo{{Res: "1920x1080", Hz: 60, MaxHz: 75}}
	tips = analyzeTips(j)
	if !strings.Contains(tips, "ram_xmp") || j.Tips[1].P["rated"] != "2666" || !strings.Contains(tips, "hz_low") || strings.Contains(tips, "hz60") {
		t.Fatalf("%s %+v", tips, j.Tips)
	}
	// System on an "Unspecified" hard drive.
	k := &SysInfo{}
	k.Disks = []DiskInfo{{Model: "ST1000DM010-2EP102", Media: "Unspecified", System: true}}
	if tips = analyzeTips(k); !strings.Contains(tips, "hdd_system") || k.Cond("ssd") {
		t.Fatal(tips)
	}
	if rebarState(256<<20, 8<<30) != "off" || rebarState(8<<30, 8<<30) != "on" || rebarState(0, 1) != "" {
		t.Fatal("rebarState")
	}
	if !rebarCapable("NVIDIA GeForce RTX 3060 Ti") || rebarCapable("NVIDIA GeForce GTX 1660 SUPER") || rebarCapable("NVIDIA GeForce RTX 2070") {
		t.Fatal("rebarCapable")
	}
}

func TestExeFromCommand(t *testing.T) {
	env := func(k string) string {
		if strings.EqualFold(k, "windir") {
			return `C:\Windows`
		}
		return ""
	}
	for in, want := range map[string]string{
		`"C:\Program Files (x86)\Steam\steam.exe" -silent`:                       `C:\Program Files (x86)\Steam\steam.exe`,
		`%windir%\system32\SecurityHealthSystray.exe`:                            `C:\Windows\system32\SecurityHealthSystray.exe`,
		`C:\Users\a\AppData\Local\Discord\Update.exe --processStart Discord.exe`: `C:\Users\a\AppData\Local\Discord\Update.exe`,
		`C:\x\Telegram.lnk`:          "",
		`%UNKNOWN%\a.exe`:            "",
		`rundll32.exe shell32.dll,x`: "rundll32.exe",
	} {
		if got := exeFromCommand(in, env); got != want {
			t.Errorf("%q -> %q want %q", in, got, want)
		}
	}
	if safeExePath(`\\server\share\game.exe`) || safeExePath(`C:\x\a.txt`) {
		t.Fatal("unsafe path accepted")
	}
	if steamCoverFile("", "730") != "" || steamCoverFile("/tmp", "../x") != "" || digitsOnly("12a") != "" {
		t.Fatal("steam cover path checks")
	}
}

func TestReviewFixes108(t *testing.T) {
	for _, p := range []string{`\/srv\share\a.exe`, `/\srv\share\a.exe`, `\??\UNC\srv\share\a.exe`, `OneDrive.exe`, `C:foo.exe`, `C:/x/a.exe`} {
		if safeExePath(p) {
			t.Errorf("%q accepted", p)
		}
	}
	if !safeExePath(`C:\Games\x.exe`) {
		t.Error("local path refused")
	}
	if _, locked, limit := memLimit("Intel(R) Core(TM) i3-8100 CPU @ 3.60GHz", "H310CM-HDV"); !locked || limit != 2400 {
		t.Error("H310CM")
	}
	if _, locked, _ := memLimit("Intel(R) Core(TM) i5-11500", "Q570M"); !locked {
		t.Error("Q570")
	}
	for _, n := range []string{"Quadro RTX 4000", "NVIDIA RTX 5000", "NVIDIA GeForce RTX 2080"} {
		if rebarCapable(n) {
			t.Errorf("%s", n)
		}
	}
	for _, n := range []string{"NVIDIA GeForce RTX 3050 Ti Laptop GPU", "NVIDIA GeForce RTX 4050 Laptop GPU", "NVIDIA GeForce RTX 5090"} {
		if !rebarCapable(n) {
			t.Errorf("%s", n)
		}
	}
	i := &SysInfo{}
	i.CPU.Name, i.Board.Product = "Intel(R) Core(TM) i5-10400F CPU @ 2.90GHz", "PRIME B460-PLUS"
	i.RAM.Modules = []RAMModule{{Speed: 2400, Conf: 2133}, {Speed: 2400, Conf: 2133}}
	if tips := analyzeTips(i); !strings.Contains(tips, "ram_xmp") || i.Tips[0].P["rated"] != "2400" {
		t.Fatalf("%s %+v", tips, i.Tips)
	}
}
