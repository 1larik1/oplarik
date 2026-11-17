package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestQRShots writes sample codes for external decoding when LARIK_QR is set.
func TestQRShots(t *testing.T) {
	dir := os.Getenv("LARIK_QR")
	samples := []string{"A", "hello", strings.Repeat("x", 40),
		"otpauth://totp/Larik:%D0%94%D0%B0%D0%BD%D0%B8%D0%BB?secret=JBSWY3DPEHPK3PXPJBSWY3DPEHPK3PXP&issuer=Larik",
		strings.Repeat("0123456789", 15), strings.Repeat("Z", 213)}
	for i, s := range samples {
		b, err := QRPNG(s, 4)
		if err != nil {
			t.Fatal(i, err)
		}
		if dir != "" {
			_ = os.WriteFile(filepath.Join(dir, "qr"+itoa(i)+".png"), b, 0o644)
			_ = os.WriteFile(filepath.Join(dir, "qr"+itoa(i)+".txt"), []byte(s), 0o644)
		}
	}
	if _, err := QRPNG(strings.Repeat("x", 214), 4); err == nil {
		t.Fatal("expected too-long error")
	}
}
