package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOverlayNewShots(t *testing.T) {
	dir := os.Getenv("LARIK_SHOTS")
	if dir == "" {
		t.Skip("set LARIK_SHOTS")
	}
	d := PreviewData(LiveStats{CPU: 34, GPU: 91, RAMUsed: 9 << 30, RAMTotal: 16 << 30}, SensorSample{CPUTemp: 71, GPUTemp: 87, GPUClock: 1830, GPUPower: 72, GPUUtil: 91, VRAMUsed: 3.1 * (1 << 30)})
	d.Warn = []string{"gputemp", "gputhr"}
	for _, lay := range []string{"row", "column"} {
		cfg := defaultOverlay()
		cfg.Layout = lay
		cfg.Metrics = overlayMetrics
		cfg.Theme = "violet"
		c, err := RenderOverlay(cfg, d, "ru", 1.25)
		if err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(dir, "ov-new-"+lay+".png"), encodePNG(c), 0o644)
	}
}

// One overlay per theme (1.1.2), stacked into a single picture.
func TestOverlayThemeShots(t *testing.T) {
	dir := os.Getenv("LARIK_SHOTS")
	if dir == "" {
		t.Skip("set LARIK_SHOTS")
	}
	d := PreviewData(LiveStats{CPU: 7, GPU: 4, RAMUsed: 9 << 30, RAMTotal: 16 << 30}, SensorSample{CPUTemp: 44, GPUTemp: 44, GPUClock: 1710, GPUPower: 51, GPUUtil: 4, VRAMUsed: 1.9 * (1 << 30)})
	for _, th := range themeNames {
		cfg := defaultOverlay()
		cfg.Theme = th
		c, err := RenderOverlay(cfg, d, "ru", 1.25)
		if err != nil {
			t.Fatal(err)
		}
		_ = os.WriteFile(filepath.Join(dir, "ov-theme-"+th+".png"), encodePNG(c), 0o644)
	}
}
