package main

// PC health (1.0.7): disk SMART status with warnings and a check for a newer
// NVIDIA driver. Disk data comes from Windows Storage (Get-PhysicalDisk,
// Get-StorageReliabilityCounter) and the SMART failure prediction flag; the
// demo build fakes it in platform_other.go.

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type DiskHealth struct {
	Name     string   `json:"name"`
	Media    string   `json:"media"` // SSD | HDD | ""
	Bus      string   `json:"bus"`
	Size     uint64   `json:"size"`
	Health   string   `json:"health"` // Healthy | Warning | Unhealthy | Unknown
	Temp     float64  `json:"temp"`   // °C, -1 = not reported
	TempMax  float64  `json:"tempMax"`
	Wear     float64  `json:"wear"`  // % of rated life used, -1 = not reported
	Hours    float64  `json:"hours"` // power-on hours, -1
	ReadErr  float64  `json:"readErr"`
	WriteErr float64  `json:"writeErr"`
	Predict  bool     `json:"predict"` // SMART predicts a failure
	Issues   []string `json:"issues"`
	Level    string   `json:"level"` // good | warn | bad
}

// analyzeDisk turns raw values into warnings. Thresholds are conservative:
// "bad" means back up your files now.
func analyzeDisk(d DiskHealth) DiskHealth {
	d.Issues = []string{}
	level := 0
	add := func(issue string, l int) {
		d.Issues = append(d.Issues, issue)
		if l > level {
			level = l
		}
	}
	switch strings.ToLower(d.Health) {
	case "unhealthy":
		add("unhealthy", 2)
	case "warning":
		add("healthWarn", 1)
	}
	if d.Predict {
		add("predict", 2)
	}
	if d.Wear >= 90 {
		add("wearBad", 2)
	} else if d.Wear >= 70 {
		add("wear", 1)
	}
	hot, warm := 70.0, 60.0
	if strings.EqualFold(d.Media, "HDD") {
		hot, warm = 55, 50
	}
	if d.Temp >= hot {
		add("hot", 2)
	} else if d.Temp >= warm {
		add("warm", 1)
	}
	if d.ReadErr > 0 || d.WriteErr > 0 {
		add("errors", 1)
	}
	d.Level = []string{"good", "warn", "bad"}[level]
	return d
}

var diskCache struct {
	sync.Mutex
	at    time.Time
	disks []DiskHealth
	err   error
}

func cachedDisks(force bool) ([]DiskHealth, error) {
	diskCache.Lock()
	defer diskCache.Unlock()
	if force || diskCache.disks == nil || time.Since(diskCache.at) > 10*time.Minute {
		raw, err := readDiskHealth()
		var out []DiskHealth
		for _, d := range raw {
			out = append(out, analyzeDisk(d))
		}
		diskCache.disks, diskCache.err, diskCache.at = orEmpty(out), err, time.Now()
	}
	return diskCache.disks, diskCache.err
}

// ---------------------------------------------------------------- GPU driver

// nvDriverVersion converts a Windows driver version (31.0.15.3623) to the
// NVIDIA version people know (536.23): the last five digits of the last two parts.
func nvDriverVersion(win string) string {
	p := strings.Split(strings.TrimSpace(win), ".")
	if len(p) != 4 {
		return ""
	}
	last := p[3]
	for len(last) < 4 {
		last = "0" + last
	}
	digits := p[2] + last
	if len(digits) < 5 {
		return ""
	}
	for _, c := range digits {
		if c < '0' || c > '9' {
			return ""
		}
	}
	d := digits[len(digits)-5:]
	major := strings.TrimLeft(d[:3], "0")
	if major == "" {
		return ""
	}
	return major + "." + d[3:]
}

func verLess(a, b string) bool {
	pa, pb := strings.SplitN(a, ".", 2), strings.SplitN(b, ".", 2)
	if len(pa) != 2 || len(pb) != 2 {
		return false
	}
	am, _ := strconv.Atoi(pa[0])
	bm, _ := strconv.Atoi(pb[0])
	if am != bm {
		return am < bm
	}
	an, _ := strconv.Atoi(pa[1])
	bn, _ := strconv.Atoi(pb[1])
	return an < bn
}

// GeForce 900/1000 series and other Maxwell/Pascal/Volta cards stay on
// NVIDIA's 580 driver branch; newer Game Ready drivers don't support them.
var nvLegacyRe = regexp.MustCompile(`(?i)\b(GTX\s?(7[45]0|9[0-9]{2}|10[0-9]{2})|TITAN\s?(X|Xp|V)\b|MX\s?(1[0-9]{2}|2[0-9]{2}|3[0-9]{2})\b|GT\s?10[0-9]{2}|[89][0-9]{2}MX?\b)`)

func nvLegacy(name string) bool { return nvLegacyRe.MatchString(name) }

type DriverLatest struct {
	Version string `json:"version"`
	Date    string `json:"date"`
	URL     string `json:"-"`
	Checked string `json:"checked"`
}

var nvVersionRe = regexp.MustCompile(`^\d{3}\.\d{2}$`)

// The same request NVIDIA's own driver search makes. The product is fixed to
// a current GeForce card (RTX 3080, Windows 10/11 64-bit, DCH, WHQL): all
// GeForce 16 / RTX cards share one Game Ready driver.
const nvLookupURL = "https://gfwsl.geforce.com/services_toolkit/services/com/nvidia/services/AjaxDriverService.php" +
	"?func=DriverManualLookup&psid=120&pfid=929&osID=57&languageCode=1033&beta=0&isWHQL=1&dltype=-1&dch=1&upCRD=0&sort1=0&numberOfResults=1"

// nvLatest asks NVIDIA for the newest Game Ready driver. Replaced in demo mode.
var nvLatest = func() (*DriverLatest, error) {
	c := http.Client{Timeout: 20 * time.Second}
	req, _ := http.NewRequest("GET", nvLookupURL, nil)
	req.Header.Set("User-Agent", appName+"/"+appVersion)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New("NVIDIA: " + resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	return parseNVLookup(b)
}

func parseNVLookup(b []byte) (*DriverLatest, error) {
	var r struct {
		IDS []struct {
			DownloadInfo struct {
				Version         string `json:"Version"`
				ReleaseDateTime string `json:"ReleaseDateTime"`
				DownloadURL     string `json:"DownloadURL"`
			} `json:"downloadInfo"`
		} `json:"IDS"`
	}
	if err := json.Unmarshal(b, &r); err != nil || len(r.IDS) == 0 {
		return nil, errors.New("unexpected answer from NVIDIA")
	}
	di := r.IDS[0].DownloadInfo
	v := strings.TrimSpace(di.Version)
	if !nvVersionRe.MatchString(v) {
		return nil, errors.New("unexpected answer from NVIDIA")
	}
	out := &DriverLatest{Version: v, Checked: nowStr()}
	if d, err := url.QueryUnescape(di.ReleaseDateTime); err == nil {
		out.Date = strings.TrimSpace(d)
	}
	if len(out.Date) > 40 {
		out.Date = ""
	}
	if u, err := url.Parse(di.DownloadURL); err == nil && u.Scheme == "https" && (u.Hostname() == "nvidia.com" || strings.HasSuffix(u.Hostname(), ".nvidia.com")) {
		out.URL = u.String()
	}
	return out, nil
}

var driverCache struct {
	sync.Mutex
	latest *DriverLatest
}

type GPUDriver struct {
	Name     string `json:"name"`
	Vendor   string `json:"vendor"`
	Driver   string `json:"driver"`   // Windows driver version
	Version  string `json:"version"`  // NVIDIA version, if NVIDIA
	Date     string `json:"date"`     // driver date
	Legacy   bool   `json:"legacy"`   // on NVIDIA's long-term branch
	Outdated bool   `json:"outdated"` // older than the latest Game Ready driver
}

func (s *Server) gpuDrivers() []GPUDriver {
	out := []GPUDriver{}
	info := s.e.Info()
	if info == nil {
		return out
	}
	driverCache.Lock()
	latest := driverCache.latest
	driverCache.Unlock()
	for _, g := range info.GPUs {
		d := GPUDriver{Name: g.Name, Vendor: g.Vendor, Driver: g.Driver, Date: g.DriverDate}
		if strings.Contains(strings.ToLower(g.Vendor+" "+g.Name), "nvidia") {
			d.Vendor = "NVIDIA"
			d.Version = nvDriverVersion(g.Driver)
			d.Legacy = nvLegacy(g.Name)
			if latest != nil && d.Version != "" && !d.Legacy {
				d.Outdated = verLess(d.Version, latest.Version)
			}
		}
		out = append(out, d)
	}
	return out
}

func (s *Server) healthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/health/disks", func(w http.ResponseWriter, r *http.Request) {
		disks, err := cachedDisks(r.URL.Query().Get("force") == "1")
		out := map[string]any{"disks": disks}
		if err != nil {
			out["error"] = err.Error()
		}
		writeJSON(w, out)
	})
	mux.HandleFunc("GET /api/health/driver", func(w http.ResponseWriter, r *http.Request) {
		driverCache.Lock()
		latest := driverCache.latest
		driverCache.Unlock()
		writeJSON(w, map[string]any{"gpus": s.gpuDrivers(), "latest": latest})
	})
	mux.HandleFunc("POST /api/health/driver/check", func(w http.ResponseWriter, r *http.Request) {
		l, err := nvLatest()
		if err != nil {
			errJSON(w, 502, err.Error())
			return
		}
		driverCache.Lock()
		driverCache.latest = l
		driverCache.Unlock()
		writeJSON(w, map[string]any{"gpus": s.gpuDrivers(), "latest": l})
	})
	mux.HandleFunc("POST /api/health/driver/open", func(w http.ResponseWriter, r *http.Request) {
		driverCache.Lock()
		latest := driverCache.latest
		driverCache.Unlock()
		u := "https://www.nvidia.com/Download/index.aspx"
		if latest != nil && latest.URL != "" {
			u = latest.URL
		}
		s.e.sys.OpenURL(u)
		writeJSON(w, map[string]bool{"ok": true})
	})
}
