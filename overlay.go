package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------- config

type OverlayConfig struct {
	Enabled   bool             `json:"enabled"`
	Mods      []string         `json:"mods"`
	Key       string           `json:"key"`
	VK        int              `json:"vk"`
	Position  string           `json:"position"`
	Layout    string           `json:"layout"`
	Size      string           `json:"size"`
	Opacity   int              `json:"opacity"`
	Interval  int              `json:"interval"`
	Metrics   []string         `json:"metrics"`
	Autostart bool             `json:"autostart"`
	WarnOff   bool             `json:"warnOff"`   // hide overheating / clock-drop warnings
	RecordOff bool             `json:"recordOff"` // don't record game sessions
	Profiles  []OverlayProfile `json:"profiles"`  // per-game layouts (1.0.7)
	Theme     string           `json:"-"`         // app theme, only colors the accent bar
}

// Metric keys in display order, grouped for the overlay.
var overlayMetrics = []string{"fps", "low1", "frametime", "ftgraph", "gpu", "gputemp", "gpuclock", "gpupower", "vram", "cpu", "cputemp", "ram", "ping", "clock"}

var metricGroup = map[string]string{
	"fps": "fps", "low1": "fps", "frametime": "fps", "ftgraph": "fps",
	"gpu": "gpu", "gputemp": "gpu", "gpuclock": "gpu", "gpupower": "gpu", "vram": "gpu",
	"cpu": "cpu", "cputemp": "cpu", "ram": "ram", "ping": "net", "clock": "clock",
}

func defaultOverlay() OverlayConfig {
	return OverlayConfig{
		Mods: []string{"alt"}, Key: "L", VK: 0x4C, Position: "tr", Layout: "row", Size: "m",
		Opacity: 85, Interval: 1000, Metrics: []string{"fps", "low1", "gpu", "gputemp", "cpu", "cputemp", "ram"},
	}
}

func (c *OverlayConfig) Normalize() {
	d := defaultOverlay()
	switch c.Position {
	case "tl", "tc", "tr", "bl", "bc", "br":
	default:
		c.Position = d.Position
	}
	if c.Layout != "row" && c.Layout != "column" {
		c.Layout = d.Layout
	}
	if c.Size != "s" && c.Size != "m" && c.Size != "l" {
		c.Size = d.Size
	}
	if c.Opacity < 30 || c.Opacity > 100 {
		c.Opacity = d.Opacity
	}
	if c.Interval != 500 && c.Interval != 1000 && c.Interval != 2000 {
		c.Interval = d.Interval
	}
	if c.VK <= 0 || c.VK > 254 {
		c.VK, c.Key, c.Mods = d.VK, d.Key, d.Mods
	}
	var mods []string
	seen := map[string]bool{}
	for _, m := range c.Mods {
		if (m == "ctrl" || m == "alt" || m == "shift" || m == "win") && !seen[m] {
			seen[m] = true
			mods = append(mods, m)
		}
	}
	c.Mods = mods
	if len(c.Key) > 16 {
		c.Key = c.Key[:16]
	}
	if c.Metrics == nil {
		c.Metrics = d.Metrics
	}
	var ms []string
	ok := map[string]bool{}
	for _, m := range overlayMetrics {
		ok[m] = true
	}
	for _, m := range c.Metrics {
		if ok[m] && !seen["m:"+m] {
			seen["m:"+m] = true
			ms = append(ms, m)
		}
	}
	c.Metrics = ms
	c.Profiles = normalizeProfiles(c.Profiles)
}

// OverlayProfile replaces the metrics and placement while one game is in front.
type OverlayProfile struct {
	Exe      string   `json:"exe"`
	Name     string   `json:"name"`
	Metrics  []string `json:"metrics"`
	Position string   `json:"position"`
	Layout   string   `json:"layout"`
	Size     string   `json:"size"`
}

const maxProfiles = 40

func normalizeProfiles(in []OverlayProfile) []OverlayProfile {
	out := []OverlayProfile{}
	seen := map[string]bool{}
	for _, p := range in {
		k := strings.ToLower(strings.TrimSpace(p.Exe))
		if !validExeName(k) || seen[k] || len(out) >= maxProfiles {
			continue
		}
		seen[k] = true
		c := OverlayConfig{Position: p.Position, Layout: p.Layout, Size: p.Size, Metrics: p.Metrics, Opacity: 85, Interval: 1000, VK: 1}
		if c.Metrics == nil {
			c.Metrics = []string{}
		}
		c.Normalize()
		name := strings.TrimSpace(p.Name)
		if r := []rune(name); len(r) > 60 {
			name = string(r[:60])
		}
		if name == "" {
			name = gameTitle(k)
		}
		out = append(out, OverlayProfile{Exe: k, Name: name, Metrics: orEmpty(c.Metrics), Position: c.Position, Layout: c.Layout, Size: c.Size})
	}
	return out
}

// ForGame returns the config to draw while exe is in the foreground.
func (c OverlayConfig) ForGame(exe string) (OverlayConfig, bool) {
	k := strings.ToLower(exe)
	for _, p := range c.Profiles {
		if p.Exe == k {
			c.Metrics, c.Position, c.Layout, c.Size = p.Metrics, p.Position, p.Layout, p.Size
			return c, true
		}
	}
	return c, false
}

// ---------------------------------------------------------------- data

// OverlayData holds one sample. Negative values mean "not available".
type OverlayData struct {
	FPS, Low1, FrameTime float64
	CPU, CPUTemp         float64
	GPU, GPUTemp         float64
	GPUClock, GPUPower   float64
	VRAM                 float64 // bytes
	RAMUsed, RAMTotal    float64 // bytes
	Clock                string
	FrameTimes           []float64 // recent frame times in ms, oldest first
	Ping                 float64   // ms to the game server, -1 = unknown
	Warn                 []string  // gputemp | cputemp | gputhr | cputhr
}

func naData() OverlayData {
	return OverlayData{FPS: -1, Low1: -1, FrameTime: -1, CPU: -1, CPUTemp: -1, GPU: -1, GPUTemp: -1, GPUClock: -1, GPUPower: -1, VRAM: -1, RAMUsed: -1, Ping: -1}
}

// ---------------------------------------------------------------- atlas

//go:embed overlay/atlas.png overlay/atlas.json
var atlasFS embed.FS

type glyph struct {
	X, Y, W, H, L, T int
	Adv              float64
}

type atlasFont struct {
	Size    int
	Ascent  int
	Descent int
	G       map[rune]glyph
}

type atlasScale struct {
	Scale        float64
	Label, Value *atlasFont
}

var (
	atlasOnce   sync.Once
	atlasPix    []byte
	atlasStride int
	atlasScales []atlasScale
	atlasErr    error
)

func loadAtlas() error {
	atlasOnce.Do(func() {
		pb, err := atlasFS.ReadFile("overlay/atlas.png")
		if err != nil {
			atlasErr = err
			return
		}
		img, err := png.Decode(bytes.NewReader(pb))
		if err != nil {
			atlasErr = err
			return
		}
		b := img.Bounds()
		atlasStride = b.Dx()
		atlasPix = make([]byte, b.Dx()*b.Dy())
		if g, ok := img.(*image.Gray); ok {
			for y := 0; y < b.Dy(); y++ {
				copy(atlasPix[y*atlasStride:(y+1)*atlasStride], g.Pix[y*g.Stride:y*g.Stride+b.Dx()])
			}
		} else {
			for y := 0; y < b.Dy(); y++ {
				for x := 0; x < b.Dx(); x++ {
					atlasPix[y*atlasStride+x] = color.GrayModel.Convert(img.At(b.Min.X+x, b.Min.Y+y)).(color.Gray).Y
				}
			}
		}
		jb, _ := atlasFS.ReadFile("overlay/atlas.json")
		var raw struct {
			Scales []float64 `json:"scales"`
			Fonts  map[string]map[string]struct {
				Size    int                   `json:"size"`
				Ascent  int                   `json:"ascent"`
				Descent int                   `json:"descent"`
				Glyphs  map[string][7]float64 `json:"glyphs"`
			} `json:"fonts"`
		}
		if err := json.Unmarshal(jb, &raw); err != nil {
			atlasErr = err
			return
		}
		for _, sc := range raw.Scales {
			fonts := raw.Fonts[fmt.Sprintf("%g", sc)]
			conv := func(name string) *atlasFont {
				f := fonts[name]
				af := &atlasFont{Size: f.Size, Ascent: f.Ascent, Descent: f.Descent, G: map[rune]glyph{}}
				for ch, v := range f.Glyphs {
					r := []rune(ch)[0]
					af.G[r] = glyph{X: int(v[0]), Y: int(v[1]), W: int(v[2]), H: int(v[3]), L: int(v[4]), T: int(v[5]), Adv: v[6]}
				}
				return af
			}
			atlasScales = append(atlasScales, atlasScale{Scale: sc, Label: conv("label"), Value: conv("value")})
		}
		sort.Slice(atlasScales, func(i, j int) bool { return atlasScales[i].Scale < atlasScales[j].Scale })
	})
	return atlasErr
}

func pickScale(want float64) atlasScale {
	best := atlasScales[0]
	for _, s := range atlasScales {
		if math.Abs(s.Scale-want) < math.Abs(best.Scale-want) {
			best = s
		}
	}
	return best
}

// ---------------------------------------------------------------- canvas

// Canvas is premultiplied ARGB (0xAARRGGBB), matching a 32-bit Windows DIB.
type Canvas struct {
	W, H int
	Pix  []uint32
}

type rgb struct{ R, G, B uint8 }

func (c *Canvas) blend(x, y int, col rgb, a uint32) {
	if x < 0 || y < 0 || x >= c.W || y >= c.H || a == 0 {
		return
	}
	if a > 255 {
		a = 255
	}
	i := y*c.W + x
	d := c.Pix[i]
	inv := 255 - a
	da, dr, dg, db := d>>24, (d>>16)&0xff, (d>>8)&0xff, d&0xff
	oa := a + da*inv/255
	or := uint32(col.R)*a/255 + dr*inv/255
	og := uint32(col.G)*a/255 + dg*inv/255
	ob := uint32(col.B)*a/255 + db*inv/255
	c.Pix[i] = oa<<24 | or<<16 | og<<8 | ob
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// roundRectSD returns the signed distance from (px,py) to a rounded rect.
func roundRectSD(px, py, x, y, w, h, r float64) float64 {
	cx, cy := x+w/2, y+h/2
	qx := math.Abs(px-cx) - (w/2 - r)
	qy := math.Abs(py-cy) - (h/2 - r)
	ox, oy := math.Max(qx, 0), math.Max(qy, 0)
	return math.Hypot(ox, oy) + math.Min(math.Max(qx, qy), 0) - r
}

func (c *Canvas) fillRoundRect(x, y, w, h, r float64, fill rgb, fillA float64, border rgb, borderA float64) {
	for py := int(y); py < int(math.Ceil(y+h)); py++ {
		for px := int(x); px < int(math.Ceil(x+w)); px++ {
			d := roundRectSD(float64(px)+0.5, float64(py)+0.5, x, y, w, h, r)
			outer := clamp01(0.5 - d)
			inner := clamp01(0.5 - (d + 1))
			if inner > 0 {
				c.blend(px, py, fill, uint32(inner*fillA*255))
			}
			if b := outer - inner; b > 0 && borderA > 0 {
				c.blend(px, py, border, uint32(b*borderA*255))
			}
		}
	}
}

func (c *Canvas) text(f *atlasFont, s string, x float64, baseline int, col rgb, alpha float64) float64 {
	for _, r := range s {
		g, ok := f.G[r]
		if !ok {
			g, ok = f.G['?']
			if !ok {
				continue
			}
		}
		ox := int(math.Round(x)) + g.L
		oy := baseline + g.T
		for yy := 0; yy < g.H; yy++ {
			row := (g.Y+yy)*atlasStride + g.X
			for xx := 0; xx < g.W; xx++ {
				if cov := atlasPix[row+xx]; cov > 0 {
					c.blend(ox+xx, oy+yy, col, uint32(float64(cov)*alpha))
				}
			}
		}
		x += g.Adv
	}
	return x
}

func textWidth(f *atlasFont, s string) float64 {
	w := 0.0
	for _, r := range s {
		if g, ok := f.G[r]; ok {
			w += g.Adv
		} else if g, ok := f.G['?']; ok {
			w += g.Adv
		}
	}
	return w
}

// ToImage converts to a straight-alpha image for PNG previews.
func (c *Canvas) ToImage() *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, c.W, c.H))
	for i, p := range c.Pix {
		a := p >> 24
		if a == 0 {
			continue
		}
		r, g, b := (p>>16)&0xff, (p>>8)&0xff, p&0xff
		img.Pix[i*4+0] = uint8(r * 255 / a)
		img.Pix[i*4+1] = uint8(g * 255 / a)
		img.Pix[i*4+2] = uint8(b * 255 / a)
		img.Pix[i*4+3] = uint8(a)
	}
	return img
}

// ---------------------------------------------------------------- rendering

// Themes the app offers; keep in sync with web/css/app.css [data-theme] and
// THEMES in web/js/core.js. Light themes come from tools/themes_light.py.
var themeNames = []string{"violet", "ocean", "emerald", "ruby", "amber", "sakura", "graphite",
	"light", "sky", "lagoon", "mint", "sand", "rose", "paper"}

func validTheme(t string) bool {
	_, ok := overlayPalettes[t]
	return ok
}

// ovPal: the overlay's colors for one app theme (1.1.2: the background and
// text follow the theme too, not only the accent bar).
type ovPal struct {
	bg, line, value, label, unit, fps, acc1, acc2, warn, bad rgb
	light                                                    bool
}

var overlayPalettes = map[string]ovPal{
	"violet":   {bg: rgb{0x12, 0x0C, 0x1D}, line: rgb{0x8B, 0x5C, 0xF6}, value: rgb{0xF3, 0xEF, 0xFB}, label: rgb{0xA9, 0x9B, 0xCB}, unit: rgb{0x8E, 0x82, 0xAD}, fps: rgb{0xC9, 0xB6, 0xFF}, acc1: rgb{0x8B, 0x5C, 0xF6}, acc2: rgb{0xD1, 0x6B, 0xFF}, warn: rgb{0xFF, 0xBF, 0x47}, bad: rgb{0xFF, 0x6B, 0x8B}, light: false},
	"ocean":    {bg: rgb{0x0B, 0x19, 0x31}, line: rgb{0x3B, 0x82, 0xF6}, value: rgb{0xEE, 0xF4, 0xFF}, label: rgb{0x9F, 0xB3, 0xD6}, unit: rgb{0x84, 0x96, 0xB8}, fps: rgb{0xA5, 0xD7, 0xFF}, acc1: rgb{0x3B, 0x82, 0xF6}, acc2: rgb{0x22, 0xD3, 0xEE}, warn: rgb{0xFF, 0xBF, 0x47}, bad: rgb{0xFF, 0x6B, 0x8B}, light: false},
	"emerald":  {bg: rgb{0x09, 0x1F, 0x17}, line: rgb{0x10, 0xB9, 0x81}, value: rgb{0xEC, 0xFB, 0xF4}, label: rgb{0x9C, 0xC4, 0xB3}, unit: rgb{0x7F, 0xA5, 0x95}, fps: rgb{0xA7, 0xF3, 0xD0}, acc1: rgb{0x10, 0xB9, 0x81}, acc2: rgb{0xA3, 0xE6, 0x35}, warn: rgb{0xFF, 0xBF, 0x47}, bad: rgb{0xFF, 0x6B, 0x8B}, light: false},
	"ruby":     {bg: rgb{0x22, 0x0B, 0x13}, line: rgb{0xE1, 0x1D, 0x48}, value: rgb{0xFD, 0xF0, 0xF3}, label: rgb{0xD0, 0xA6, 0xB2}, unit: rgb{0xB0, 0x87, 0x93}, fps: rgb{0xFE, 0xCD, 0xD3}, acc1: rgb{0xE1, 0x1D, 0x48}, acc2: rgb{0xFF, 0x80, 0xA0}, warn: rgb{0xFF, 0xBF, 0x47}, bad: rgb{0xFF, 0x6B, 0x8B}, light: false},
	"amber":    {bg: rgb{0x21, 0x17, 0x08}, line: rgb{0xF5, 0x9E, 0x0B}, value: rgb{0xFD, 0xF7, 0xEC}, label: rgb{0xCD, 0xBB, 0x9A}, unit: rgb{0xAE, 0x9C, 0x7D}, fps: rgb{0xFD, 0xE6, 0x8A}, acc1: rgb{0xF5, 0x8C, 0x1E}, acc2: rgb{0xFC, 0xD3, 0x4D}, warn: rgb{0xFF, 0xBF, 0x47}, bad: rgb{0xFF, 0x6B, 0x8B}, light: false},
	"sakura":   {bg: rgb{0x21, 0x0D, 0x1C}, line: rgb{0xEC, 0x48, 0x99}, value: rgb{0xFD, 0xF0, 0xFA}, label: rgb{0xCF, 0xA8, 0xC4}, unit: rgb{0xB0, 0x8A, 0xA6}, fps: rgb{0xFB, 0xCF, 0xE8}, acc1: rgb{0xEC, 0x48, 0x99}, acc2: rgb{0xC0, 0x84, 0xFC}, warn: rgb{0xFF, 0xBF, 0x47}, bad: rgb{0xFF, 0x6B, 0x8B}, light: false},
	"graphite": {bg: rgb{0x15, 0x16, 0x19}, line: rgb{0x9A, 0xA3, 0xB0}, value: rgb{0xF2, 0xF3, 0xF5}, label: rgb{0xAE, 0xB2, 0xB9}, unit: rgb{0x8E, 0x93, 0x9B}, fps: rgb{0xE6, 0xE9, 0xEE}, acc1: rgb{0x9A, 0xA3, 0xB0}, acc2: rgb{0xE6, 0xE9, 0xEE}, warn: rgb{0xFF, 0xBF, 0x47}, bad: rgb{0xFF, 0x6B, 0x8B}, light: false},
	"light":    {bg: rgb{0xF8, 0xF7, 0xFC}, line: rgb{0x8B, 0x5C, 0xF6}, value: rgb{0x1C, 0x15, 0x30}, label: rgb{0x3E, 0x2E, 0x64}, unit: rgb{0x6E, 0x62, 0x8B}, fps: rgb{0x6D, 0x28, 0xD9}, acc1: rgb{0x7C, 0x3A, 0xED}, acc2: rgb{0xC0, 0x26, 0xD3}, warn: rgb{0xB2, 0x6B, 0x00}, bad: rgb{0xE1, 0x1D, 0x48}, light: true},
	"sky":      {bg: rgb{0xF5, 0xF9, 0xFF}, line: rgb{0x25, 0x63, 0xEB}, value: rgb{0x0F, 0x1B, 0x33}, label: rgb{0x1E, 0x3F, 0x77}, unit: rgb{0x56, 0x6F, 0x99}, fps: rgb{0x1D, 0x4E, 0xD8}, acc1: rgb{0x1D, 0x4E, 0xD8}, acc2: rgb{0x08, 0x91, 0xB2}, warn: rgb{0xB2, 0x6B, 0x00}, bad: rgb{0xE1, 0x1D, 0x48}, light: true},
	"lagoon":   {bg: rgb{0xF3, 0xFB, 0xFB}, line: rgb{0x0D, 0x94, 0x88}, value: rgb{0x0B, 0x2A, 0x2C}, label: rgb{0x0E, 0x5C, 0x57}, unit: rgb{0x4A, 0x85, 0x81}, fps: rgb{0x0F, 0x76, 0x6E}, acc1: rgb{0x0F, 0x76, 0x6E}, acc2: rgb{0x06, 0xB6, 0xD4}, warn: rgb{0xB2, 0x6B, 0x00}, bad: rgb{0xE1, 0x1D, 0x48}, light: true},
	"mint":     {bg: rgb{0xF3, 0xFC, 0xF9}, line: rgb{0x05, 0x96, 0x69}, value: rgb{0x0F, 0x2A, 0x1F}, label: rgb{0x0F, 0x5C, 0x41}, unit: rgb{0x4B, 0x85, 0x70}, fps: rgb{0x04, 0x78, 0x57}, acc1: rgb{0x04, 0x78, 0x57}, acc2: rgb{0x65, 0xA3, 0x0D}, warn: rgb{0xB2, 0x6B, 0x00}, bad: rgb{0xE1, 0x1D, 0x48}, light: true},
	"sand":     {bg: rgb{0xFD, 0xFA, 0xF6}, line: rgb{0xD9, 0x77, 0x06}, value: rgb{0x2A, 0x1F, 0x0E}, label: rgb{0x67, 0x4E, 0x20}, unit: rgb{0x8D, 0x7A, 0x58}, fps: rgb{0xB4, 0x53, 0x09}, acc1: rgb{0xC2, 0x41, 0x0C}, acc2: rgb{0xCA, 0x8A, 0x04}, warn: rgb{0xB2, 0x6B, 0x00}, bad: rgb{0xE1, 0x1D, 0x48}, light: true},
	"rose":     {bg: rgb{0xFE, 0xF5, 0xF7}, line: rgb{0xE1, 0x1D, 0x48}, value: rgb{0x2E, 0x0F, 0x18}, label: rgb{0x73, 0x20, 0x30}, unit: rgb{0x96, 0x58, 0x64}, fps: rgb{0xBE, 0x12, 0x3C}, acc1: rgb{0xBE, 0x12, 0x3C}, acc2: rgb{0xDB, 0x27, 0x77}, warn: rgb{0xB2, 0x6B, 0x00}, bad: rgb{0xE1, 0x1D, 0x48}, light: true},
	"paper":    {bg: rgb{0xF8, 0xF8, 0xF8}, line: rgb{0x47, 0x55, 0x69}, value: rgb{0x18, 0x18, 0x1B}, label: rgb{0x37, 0x37, 0x3C}, unit: rgb{0x69, 0x69, 0x6D}, fps: rgb{0x33, 0x41, 0x55}, acc1: rgb{0x33, 0x41, 0x55}, acc2: rgb{0x94, 0xA3, 0xB8}, warn: rgb{0xB2, 0x6B, 0x00}, bad: rgb{0xE1, 0x1D, 0x48}, light: true},
}

func overlayPalette(t string) ovPal {
	if p, ok := overlayPalettes[t]; ok {
		return p
	}
	return overlayPalettes["violet"]
}

var overlayLabels = map[string]map[string]string{
	"en": {"fps": "FPS", "gpu": "GPU", "cpu": "CPU", "ram": "RAM", "low1": "1%", "ms": "ms", "mhz": "MHz", "w": "W", "gb": "GB", "net": "Ping",
		"w.gputemp": "GPU hot", "w.cputemp": "CPU hot", "w.gputhr": "GPU throttling", "w.cputhr": "CPU throttling"},
	"ru": {"fps": "FPS", "gpu": "ГП", "cpu": "ЦП", "ram": "ОЗУ", "low1": "1%", "ms": "мс", "mhz": "МГц", "w": "Вт", "gb": "ГБ", "net": "Пинг",
		"w.gputemp": "ГП перегрев", "w.cputemp": "ЦП перегрев", "w.gputhr": "ГП троттлинг", "w.cputhr": "ЦП троттлинг"},
	"uk": {"fps": "FPS", "gpu": "ГП", "cpu": "ЦП", "ram": "ОЗП", "low1": "1%", "ms": "мс", "mhz": "МГц", "w": "Вт", "gb": "ГБ", "net": "Пінг",
		"w.gputemp": "ГП перегрів", "w.cputemp": "ЦП перегрів", "w.gputhr": "ГП тротлінг", "w.cputhr": "ЦП тротлінг"},
	"pl": {"fps": "FPS", "gpu": "GPU", "cpu": "CPU", "ram": "RAM", "low1": "1%", "ms": "ms", "mhz": "MHz", "w": "W", "gb": "GB", "net": "Ping",
		"w.gputemp": "GPU gorące", "w.cputemp": "CPU gorące", "w.gputhr": "GPU dławi", "w.cputhr": "CPU dławi"},
}

func olabel(lang, k string) string {
	if m, ok := overlayLabels[lang]; ok {
		if s, ok := m[k]; ok {
			return s
		}
	}
	return overlayLabels["en"][k]
}

type ovItem struct {
	prefix, value, unit string
	col                 rgb
	na                  bool
	graph               []float64 // frame-time strip instead of text
}

type ovGroup struct {
	label string
	items []ovItem
	warn  bool // red label (warnings)
}

func fmtNum(v float64, dec int, lang string) string {
	s := fmt.Sprintf("%.*f", dec, v)
	if lang != "en" {
		s = strings.Replace(s, ".", ",", 1)
	}
	return s
}

func tempColor(p ovPal, t float64) rgb {
	switch {
	case t >= 85:
		return p.bad
	case t >= 72:
		return p.warn
	}
	return p.value
}

func buildGroups(cfg OverlayConfig, d OverlayData, lang string) []ovGroup {
	p := overlayPalette(cfg.Theme)
	on := map[string]bool{}
	for _, m := range cfg.Metrics {
		on[m] = true
	}
	na := ovItem{value: "—", col: p.unit, na: true}
	item := func(k string) ovItem {
		switch k {
		case "fps":
			if d.FPS < 0 {
				return na
			}
			c := p.fps
			if d.FPS < 30 {
				c = p.bad
			} else if d.FPS < 55 {
				c = p.warn
			}
			return ovItem{value: fmt.Sprintf("%.0f", d.FPS), col: c}
		case "low1":
			it := ovItem{prefix: olabel(lang, "low1"), value: "—", col: p.unit}
			if d.Low1 >= 0 {
				it.value, it.col = fmt.Sprintf("%.0f", d.Low1), p.value
			}
			return it
		case "frametime":
			if d.FrameTime < 0 {
				return na
			}
			return ovItem{value: fmtNum(d.FrameTime, 1, lang), unit: olabel(lang, "ms"), col: p.value}
		case "gpu", "cpu":
			v := d.GPU
			if k == "cpu" {
				v = d.CPU
			}
			if v < 0 {
				return na
			}
			c := p.value
			if v >= 97 {
				c = p.warn
			}
			return ovItem{value: fmt.Sprintf("%.0f", v), unit: "%", col: c}
		case "gputemp", "cputemp":
			v := d.GPUTemp
			if k == "cputemp" {
				v = d.CPUTemp
			}
			if v < 0 {
				return na
			}
			return ovItem{value: fmt.Sprintf("%.0f", v), unit: "°C", col: tempColor(p, v)}
		case "gpuclock":
			if d.GPUClock < 0 {
				return na
			}
			return ovItem{value: fmt.Sprintf("%.0f", d.GPUClock), unit: olabel(lang, "mhz"), col: p.value}
		case "gpupower":
			if d.GPUPower < 0 {
				return na
			}
			return ovItem{value: fmt.Sprintf("%.0f", d.GPUPower), unit: olabel(lang, "w"), col: p.value}
		case "vram":
			if d.VRAM < 0 {
				return na
			}
			return ovItem{prefix: "VRAM", value: fmtNum(d.VRAM/(1<<30), 1, lang), unit: olabel(lang, "gb"), col: p.value}
		case "ram":
			if d.RAMUsed < 0 {
				return na
			}
			return ovItem{value: fmtNum(d.RAMUsed/(1<<30), 1, lang), unit: olabel(lang, "gb"), col: p.value}
		case "clock":
			if d.Clock == "" {
				return na
			}
			return ovItem{value: d.Clock, col: p.value}
		case "ftgraph":
			if len(d.FrameTimes) < 4 {
				return na
			}
			return ovItem{graph: d.FrameTimes}
		case "ping":
			if d.Ping < 0 {
				return na
			}
			c := p.value
			if d.Ping >= 120 {
				c = p.bad
			} else if d.Ping >= 70 {
				c = p.warn
			}
			return ovItem{value: fmt.Sprintf("%.0f", d.Ping), unit: olabel(lang, "ms"), col: c}
		}
		return na
	}
	var groups []ovGroup
	for _, g := range []string{"fps", "gpu", "cpu", "ram", "net", "clock"} {
		var items []ovItem
		for _, m := range overlayMetrics {
			if on[m] && metricGroup[m] == g {
				items = append(items, item(m))
			}
		}
		if len(items) == 0 {
			continue
		}
		label := ""
		if g != "clock" {
			label = olabel(lang, g)
		}
		groups = append(groups, ovGroup{label: label, items: items})
	}
	if !cfg.WarnOff && len(d.Warn) > 0 {
		g := ovGroup{label: "!", warn: true}
		for _, w := range d.Warn {
			g.items = append(g.items, ovItem{value: olabel(lang, "w."+w), col: p.bad})
		}
		groups = append(groups, g)
	}
	return groups
}

func sizeFactor(s string) float64 {
	switch s {
	case "s":
		return 0.85
	case "l":
		return 1.25
	}
	return 1
}

// RenderOverlay draws the overlay bar. dpiScale is monitor DPI / 96.
func RenderOverlay(cfg OverlayConfig, d OverlayData, lang string, dpiScale float64) (*Canvas, error) {
	if err := loadAtlas(); err != nil {
		return nil, err
	}
	sc := pickScale(sizeFactor(cfg.Size) * dpiScale)
	s := sc.Scale
	lf, vf := sc.Label, sc.Value
	p := overlayPalette(cfg.Theme)
	groups := buildGroups(cfg, d, lang)
	if len(groups) == 0 {
		groups = []ovGroup{{label: appName, items: nil}}
	}
	alpha := float64(cfg.Opacity) / 100
	padX, padY := math.Round(11*s), math.Round(7*s)
	accentW, accentGap := math.Max(2, math.Round(3*s)), math.Round(8*s)
	itemGap, groupGap := math.Round(9*s), math.Round(11*s)
	labelGap, unitGap := math.Round(6*s), math.Round(2*s)
	lineH := float64(vf.Ascent+vf.Descent) * 0.86

	graphW := math.Round(84 * s)
	itemWidth := func(it ovItem) float64 {
		if it.graph != nil {
			return graphW
		}
		w := 0.0
		if it.prefix != "" {
			w += textWidth(lf, it.prefix) + unitGap*1.5
		}
		w += textWidth(vf, it.value)
		if it.unit != "" {
			w += unitGap + textWidth(lf, it.unit)
		}
		return w
	}
	groupValuesWidth := func(g ovGroup) float64 {
		w := 0.0
		for i, it := range g.items {
			if i > 0 {
				w += itemGap
			}
			w += itemWidth(it)
		}
		return w
	}

	contentX := padX*0.6 + accentW + accentGap
	var W, H float64
	labelCol := 0.0
	if cfg.Layout == "column" {
		for _, g := range groups {
			labelCol = math.Max(labelCol, textWidth(lf, g.label))
		}
		maxRow := 0.0
		for _, g := range groups {
			maxRow = math.Max(maxRow, groupValuesWidth(g))
		}
		W = contentX + labelCol + labelGap*1.4 + maxRow + padX
		H = padY*2 + lineH*float64(len(groups)) + math.Round(3*s)*float64(len(groups)-1)
	} else {
		w := 0.0
		for i, g := range groups {
			if i > 0 {
				w += groupGap*2 + 1
			}
			if g.label != "" {
				w += textWidth(lf, g.label) + labelGap
			}
			w += groupValuesWidth(g)
		}
		W = contentX + w + padX
		H = padY*2 + lineH
	}
	cw, ch := int(math.Ceil(W)), int(math.Ceil(H))
	c := &Canvas{W: cw, H: ch, Pix: make([]uint32, cw*ch)}
	radius := math.Round(9 * s)
	c.fillRoundRect(0, 0, float64(cw), float64(ch), radius, p.bg, alpha*0.92, p.line, alpha*0.42)

	// Accent pill in the app theme's colors (Larik violet → orchid by default).
	colAcc1, colAcc2 := p.acc1, p.acc2
	ax, ay, ah := padX*0.6, padY, float64(ch)-2*padY
	for py := int(ay); py < int(ay+ah); py++ {
		f := (float64(py) - ay) / math.Max(1, ah)
		col := rgb{uint8(float64(colAcc1.R) + (float64(colAcc2.R)-float64(colAcc1.R))*f), uint8(float64(colAcc1.G) + (float64(colAcc2.G)-float64(colAcc1.G))*f), uint8(float64(colAcc1.B) + (float64(colAcc2.B)-float64(colAcc1.B))*f)}
		for px := int(ax); px < int(math.Ceil(ax+accentW)); px++ {
			d := roundRectSD(float64(px)+0.5, float64(py)+0.5, ax, ay, accentW, ah, accentW/2)
			if cov := clamp01(0.5 - d); cov > 0 {
				c.blend(px, py, col, uint32(cov*alpha*255))
			}
		}
	}

	drawItems := func(g ovGroup, x float64, base int) float64 {
		for i, it := range g.items {
			if i > 0 {
				x += itemGap
			}
			if it.graph != nil {
				bottom := float64(base) + math.Round(1*s)
				top := bottom - math.Round(float64(vf.Ascent)*0.9)
				c.drawFrameGraph(p, it.graph, x, top, graphW, bottom-top, alpha)
				x += graphW
				continue
			}
			if it.prefix != "" {
				x = c.text(lf, it.prefix, x, base, p.unit, alpha)
				x += unitGap * 1.5
			}
			x = c.text(vf, it.value, x, base, it.col, alpha)
			if it.unit != "" {
				x += unitGap
				x = c.text(lf, it.unit, x, base, p.unit, alpha)
			}
		}
		return x
	}
	baseOff := (lineH-float64(vf.Ascent+vf.Descent))/2 + float64(vf.Ascent)
	if cfg.Layout == "column" {
		for i, g := range groups {
			top := padY + float64(i)*(lineH+math.Round(3*s))
			base := int(math.Round(top + baseOff))
			lc := p.label
			if g.warn {
				lc = p.bad
			}
			c.text(lf, g.label, contentX, base, lc, alpha)
			drawItems(g, contentX+labelCol+labelGap*1.4, base)
		}
	} else {
		base := int(math.Round(padY + baseOff))
		x := contentX
		for i, g := range groups {
			if i > 0 {
				x += groupGap
				sepTop, sepBot := int(padY+lineH*0.18), int(padY+lineH*0.86)
				for py := sepTop; py < sepBot; py++ {
					c.blend(int(x), py, p.line, uint32(alpha*0.55*255))
				}
				x += 1 + groupGap
			}
			if g.label != "" {
				lc := p.label
				if g.warn {
					lc = p.bad
				}
				x = c.text(lf, g.label, x, base, lc, alpha)
				x += labelGap
			}
			x = drawItems(g, x, base)
		}
	}
	return c, nil
}

// PreviewData gives plausible values for metrics the engine can't measure.
func PreviewData(l LiveStats, sens SensorSample) OverlayData {
	d := naData()
	d.FPS, d.Low1, d.FrameTime = 144, 118, 6.9
	d.Ping = 24
	for i := 0; i < 60; i++ {
		ft := 6.9 + 0.6*math.Sin(float64(i)*0.9)
		if i == 41 {
			ft = 21 // one stutter, so the strip shows what a freeze looks like
		}
		d.FrameTimes = append(d.FrameTimes, ft)
	}
	if l.RAMTotal > 0 {
		d.CPU = l.CPU
		d.RAMUsed, d.RAMTotal = float64(l.RAMUsed), float64(l.RAMTotal)
	}
	d.GPU = l.GPU
	if l.VRAM > 0 {
		d.VRAM = float64(l.VRAM)
	}
	d.CPUTemp, d.GPUTemp, d.GPUClock, d.GPUPower = sens.CPUTemp, sens.GPUTemp, sens.GPUClock, sens.GPUPower
	if sens.GPUUtil >= 0 {
		d.GPU = sens.GPUUtil
	}
	if sens.VRAMUsed > 0 {
		d.VRAM = sens.VRAMUsed
	}
	d.Clock = time.Now().Format("15:04")
	return d
}

// SensorSample is filled from NVML / thermal zones where available.
type SensorSample struct {
	CPUTemp, GPUTemp, GPUClock, GPUPower, GPUUtil, VRAMUsed float64
	CPUPerf                                                 float64 // % of base clock, -1 = unknown
	GPUThrottle                                             bool    // GPU slowing down for heat or power brake
	NVML, CPUTempOK                                         bool
}

func naSensors() SensorSample {
	return SensorSample{CPUTemp: -1, GPUTemp: -1, GPUClock: -1, GPUPower: -1, GPUUtil: -1, VRAMUsed: -1, CPUPerf: -1}
}

// Warning thresholds. The GTX 16-series and most current GPUs start to slow
// down around 87-90 °C; desktop Ryzen/Core CPUs around 95-100 °C.
const (
	warnGPUTemp = 85
	warnCPUTemp = 90
)

// warnState turns raw sensor samples into stable overlay warnings: a warning
// appears after the condition held for a few samples and stays visible for
// 10 seconds after it clears, so it doesn't flicker.
type warnState struct {
	streak map[string]int
	until  map[string]time.Time
}

func (w *warnState) update(now time.Time, s SensorSample, cpuLoad, gpuLoad float64) []string {
	if w.streak == nil {
		w.streak, w.until = map[string]int{}, map[string]time.Time{}
	}
	cond := map[string]bool{
		"gputemp": s.GPUTemp >= warnGPUTemp,
		"cputemp": s.CPUTemp >= warnCPUTemp,
		"gputhr":  s.GPUThrottle && gpuLoad >= 50,
		// under real load a CPU should run at or above its base clock
		"cputhr": s.CPUPerf > 0 && s.CPUPerf < 60 && cpuLoad >= 60,
	}
	need := map[string]int{"gputemp": 3, "cputemp": 3, "gputhr": 3, "cputhr": 8}
	var out []string
	for _, k := range []string{"gputemp", "gputhr", "cputemp", "cputhr"} {
		if cond[k] {
			w.streak[k]++
			if w.streak[k] >= need[k] {
				w.until[k] = now.Add(10 * time.Second)
			}
		} else {
			w.streak[k] = 0
		}
		if now.Before(w.until[k]) {
			out = append(out, k)
		}
	}
	return out
}

func encodePNG(c *Canvas) []byte {
	var buf bytes.Buffer
	_ = png.Encode(&buf, c.ToImage())
	return buf.Bytes()
}

// drawFrameGraph draws recent frame times as bars: taller = slower frame.
// Bars well above the typical frame time (stutters) are drawn in red.
func (c *Canvas) drawFrameGraph(p ovPal, ft []float64, x, y, w, h, alpha float64) {
	if len(ft) == 0 || w < 2 || h < 2 {
		return
	}
	sorted := append([]float64(nil), ft...)
	sort.Float64s(sorted)
	med := sorted[len(sorted)/2]
	ref := math.Max(med*2.5, 4)
	// faint background
	c.fillRoundRect(x, y, w, h, math.Min(3, h/4), p.line, alpha*0.12, p.line, 0)
	n := len(ft)
	for px := int(x); px < int(x+w); px++ {
		i := int((float64(px) - x) / w * float64(n))
		if i >= n {
			i = n - 1
		}
		v := ft[i]
		bh := math.Max(1, math.Min(1, v/ref)*h)
		col := p.fps
		a := alpha * 0.78
		if v > med*1.8 && v > med+4 {
			col, a = p.bad, alpha
		}
		top := y + h - bh
		for py := int(math.Floor(top)); py < int(y+h); py++ {
			cov := 1.0
			if float64(py) < top {
				cov = 1 - (top - float64(py))
			}
			if cov > 0 {
				c.blend(px, py, col, uint32(cov*a*255))
			}
		}
	}
}
