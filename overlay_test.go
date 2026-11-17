package main

import (
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func sampleData() OverlayData {
	return OverlayData{FPS: 143, Low1: 96, FrameTime: 6.99, CPU: 23, CPUTemp: 61, GPU: 97, GPUTemp: 74, GPUClock: 1830,
		GPUPower: 61, VRAM: 3.1 * (1 << 30), RAMUsed: 9.4 * (1 << 30), RAMTotal: 16 * (1 << 30), Clock: "21:47"}
}

func TestOverlayRender(t *testing.T) {
	cfg := defaultOverlay()
	cfg.Normalize()
	for _, lang := range []string{"en", "ru", "uk", "pl"} {
		for _, layout := range []string{"row", "column"} {
			for _, size := range []string{"s", "m", "l"} {
				cfg.Layout, cfg.Size = layout, size
				c, err := RenderOverlay(cfg, sampleData(), lang, 1)
				if err != nil {
					t.Fatal(err)
				}
				if c.W < 50 || c.H < 15 || c.W > 2000 {
					t.Fatalf("bad size %dx%d", c.W, c.H)
				}
			}
		}
	}
	// All metrics, all n/a: must not panic.
	cfg.Metrics = overlayMetrics
	if _, err := RenderOverlay(cfg, naData(), "ru", 2.2); err != nil {
		t.Fatal(err)
	}
	cfg.Metrics = nil
	if _, err := RenderOverlay(cfg, naData(), "en", 1); err != nil {
		t.Fatal(err)
	}
}

// LARIK_SHOTS=dir go test -run TestOverlayShots writes preview PNGs.
func TestOverlayShots(t *testing.T) {
	dir := os.Getenv("LARIK_SHOTS")
	if dir == "" {
		t.Skip()
	}
	bg := image.NewRGBA(image.Rect(0, 0, 900, 520))
	for y := 0; y < 520; y++ {
		for x := 0; x < 900; x++ {
			bg.Set(x, y, color.RGBA{uint8(40 + x*60/900), uint8(70 + y*80/520), uint8(90 + (x+y)%40), 255})
		}
	}
	y := 12
	full := defaultOverlay()
	full.Metrics = overlayMetrics
	cases := []struct {
		cfg  OverlayConfig
		lang string
		d    OverlayData
	}{
		{defaultOverlay(), "en", sampleData()},
		{defaultOverlay(), "ru", sampleData()},
		{full, "uk", sampleData()},
		{full, "pl", naData()},
	}
	for _, cs := range cases {
		cs.cfg.Normalize()
		c, err := RenderOverlay(cs.cfg, cs.d, cs.lang, 1)
		if err != nil {
			t.Fatal(err)
		}
		draw.Draw(bg, image.Rect(12, y, 12+c.W, y+c.H), c.ToImage(), image.Point{}, draw.Over)
		y += c.H + 12
	}
	col := defaultOverlay()
	col.Layout, col.Size, col.Metrics = "column", "l", overlayMetrics
	c, _ := RenderOverlay(col, sampleData(), "ru", 1)
	draw.Draw(bg, image.Rect(12, y, 12+c.W, y+c.H), c.ToImage(), image.Point{}, draw.Over)
	col.Size, col.Opacity = "s", 60
	c2, _ := RenderOverlay(col, sampleData(), "en", 1)
	draw.Draw(bg, image.Rect(40+c.W, y, 40+c.W+c2.W, y+c2.H), c2.ToImage(), image.Point{}, draw.Over)
	f, _ := os.Create(filepath.Join(dir, "overlay-preview.png"))
	defer f.Close()
	_ = png.Encode(f, bg)
}
