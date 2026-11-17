package main

// Hardware knowledge used by the scan (1.0.8): telling HDDs from SSDs when
// Windows reports "Unspecified", memory speed limits of Intel boards that
// don't allow memory overclocking (XMP), and which GPUs support Resizable BAR.

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	reSSDModel = regexp.MustCompile(`(?i)(\bSSD\b|NVME|\bM\.2\b|^KINGSTON\s+S(A|UV|KC|NV|V|Q|EDC|HS)\d|^CT\d+(MX|BX|P\d|T\d)|^SAMSUNG\s+MZ|^ADATA\s+(SU|SX|LEGEND)|^WDC?\s*WDS\d|^WD\s+(BLUE|GREEN|BLACK)\s+SN|^INTEL\s+SSD|^SANDISK\s+SD|^TEAM\s|^PATRIOT|^GOODRAM|^APACER|^HP\s+SSD|^SPCC|^NETAC|^KIOXIA|^PNY\s+CS)`)
	reHDDModel = regexp.MustCompile(`(?i)(^WDC\s+WD\d|^WD\d{3,5}[A-Z]{2}|^ST\d{3,5}(DM|DX|LM|VX|NM|AS|NE|LX|VN)|^TOSHIBA\s+(DT|MQ|HDW|MG|MD|P300)|^HGST|^HITACHI\s+H|^SAMSUNG\s+(HD|HM|SP)\d|^MAXTOR|^ST\d{4,}A?$)`)
)

// guessMediaByModel recognises common drive model names: "SSD", "HDD" or "".
func guessMediaByModel(model string) string {
	m := strings.TrimSpace(model)
	switch {
	case m == "":
		return ""
	case reSSDModel.MatchString(m):
		return "SSD"
	case reHDDModel.MatchString(m):
		return "HDD"
	}
	return ""
}

// fixMedia decides SSD or HDD from everything known about a disk. Windows
// reports MediaType "Unspecified" for many older drives (and some RAID / USB
// setups); seekPenalty is what the drive itself says (-1 unknown, 0 no seek
// penalty = SSD, 1 = spinning disk).
func fixMedia(media, bus, model string, seekPenalty int) string {
	switch strings.ToUpper(strings.TrimSpace(media)) {
	case "SSD":
		return "SSD"
	case "HDD":
		return "HDD"
	}
	if strings.EqualFold(bus, "NVMe") {
		return "SSD"
	}
	switch seekPenalty {
	case 0:
		return "SSD"
	case 1:
		return "HDD"
	}
	return guessMediaByModel(model)
}

var (
	reIntelCore = regexp.MustCompile(`(?i)\bi([3579])-(\d{4,5})([A-Z]*)\b`)
	reChipset   = regexp.MustCompile(`(?i)\b([BHQZ])(\d{3})[A-Z]{0,2}\b`)
)

// memLimit tells whether this Intel board + CPU combination locks memory to
// the processor's official speed, and to what. Intel only allows memory
// overclocking (XMP) on Z chipsets up to the 400 series; on 500-series B/H
// boards only with 11th-gen CPUs (not H510). AMD boards allow it.
// It returns the chipset name, whether memory is locked, and the speed limit
// in MT/s (0 = locked but the exact limit is unknown).
func memLimit(cpuName, board string) (chipset string, locked bool, limit int) {
	cm := reChipset.FindStringSubmatch(board)
	if cm == nil || !strings.Contains(strings.ToLower(cpuName), "intel") {
		return "", false, 0
	}
	letter := strings.ToUpper(cm[1])
	series, _ := strconv.Atoi(cm[2])
	chipset = letter + cm[2]
	if letter == "Z" {
		return chipset, false, 0
	}
	gen, tier := 0, ""
	if m := reIntelCore.FindStringSubmatch(cpuName); m != nil {
		tier = m[1]
		num := m[2]
		if len(num) == 5 {
			gen, _ = strconv.Atoi(num[:2])
		} else {
			gen, _ = strconv.Atoi(num[:1])
		}
	}
	switch {
	case series >= 100 && series < 500:
		locked = true
	case series >= 500 && series < 600:
		locked = letter == "Q" || chipset == "H510" || gen == 10 || gen == 0
	case series == 610 || series == 810:
		locked = letter == "H"
	default:
		return chipset, false, 0
	}
	if !locked {
		return chipset, false, 0
	}
	switch {
	case gen == 6:
		limit = 2133
	case gen == 7:
		limit = 2400
	case gen == 8 || gen == 9:
		limit = 2666
		if tier == "3" {
			limit = 2400
		}
	case gen == 10:
		limit = 2666
		if tier == "7" || tier == "9" {
			limit = 2933
		}
	case gen == 11:
		limit = 3200
	}
	return chipset, true, limit
}

// GeForce RTX 3050–3090, 4050–4090, 5050–5090 (not Quadro RTX 4000/5000).
var reReBAR = regexp.MustCompile(`(?i)\bRTX\s?(30[5-9]0|40[5-9]0|50[5-9]0)\b`)

// rebarCapable: GPUs that gain from Resizable BAR and whose state Larik can
// read (NVIDIA through NVML).
func rebarCapable(name string) bool { return reReBAR.MatchString(name) }

// rebarState turns NVML's BAR1 size into "on"/"off". Without Resizable BAR
// the CPU sees the GPU memory through a 256 MB window.
func rebarState(bar1, vram uint64) string {
	if bar1 == 0 {
		return ""
	}
	if bar1 > 512<<20 || (vram > 0 && bar1 >= vram/2) {
		return "on"
	}
	return "off"
}

// exeFromCommand pulls the program path out of a startup command line
// ("C:\x\a.exe" -silent, %windir%\b.exe /x, C:\Program Files\c.exe --y).
func exeFromCommand(cmd string, getenv func(string) string) string {
	c := strings.TrimSpace(cmd)
	if strings.HasPrefix(c, `"`) {
		if i := strings.Index(c[1:], `"`); i >= 0 {
			c = c[1 : i+1]
		}
	} else if i := strings.Index(strings.ToLower(c), ".exe"); i >= 0 {
		c = c[:i+4]
	}
	c = regexp.MustCompile(`%([A-Za-z0-9_()]+)%`).ReplaceAllStringFunc(c, func(v string) string {
		if e := getenv(strings.Trim(v, "%")); e != "" {
			return e
		}
		return v
	})
	if !strings.HasSuffix(strings.ToLower(c), ".exe") || strings.Contains(c, "%") {
		return ""
	}
	return c
}
