package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestTOTPVectors(t *testing.T) {
	// RFC 6238 appendix B (SHA1, 8 digits) → last 6 digits.
	secret := []byte("12345678901234567890")
	for _, c := range []struct {
		t    int64
		want string
	}{{59, "287082"}, {1111111109, "081804"}, {1111111111, "050471"}, {1234567890, "005924"}, {2000000000, "279037"}} {
		if got := totpCode(secret, c.t/30); got != c.want {
			t.Fatalf("t=%d got %s want %s", c.t, got, c.want)
		}
	}
	b := b32.EncodeToString(secret)
	now := time.Unix(1111111111, 0)
	if s := totpCheck(b, "050 471", now, 0); s != 1111111111/30 {
		t.Fatal("check failed", s)
	}
	if s := totpCheck(b, "050471", now, 1111111111/30); s != -1 {
		t.Fatal("replay accepted")
	}
	if s := totpCheck(b, "000000", now, 0); s != -1 {
		t.Fatal("wrong code accepted")
	}
}

func newTestAuth(t *testing.T) *Auth {
	a := OpenAuth(filepath.Join(t.TempDir(), "account"))
	a.admin = true
	return a
}

func TestAuthFlow(t *testing.T) {
	a := newTestAuth(t)
	if a.Locked() || a.Status().Exists {
		t.Fatal("fresh install must be open")
	}
	if _, err := a.Setup("  Даня  ", "123"); err != errWeak {
		t.Fatal("weak password accepted")
	}
	key, err := a.Setup("  Даня   Л ", "secret1")
	if err != nil || len(key) != 19 {
		t.Fatal(err, key)
	}
	if a.Status().Name != "Даня Л" {
		t.Fatal("name", a.Status().Name)
	}
	// reopen: locked
	a2 := OpenAuth(a.dir)
	a2.admin = true
	if !a2.Locked() || !a2.Status().Exists {
		t.Fatal("must be locked after restart")
	}
	if _, err := a2.Login("wrong"); err != errBadPassword {
		t.Fatal(err)
	}
	if tk, err := a2.Login("secret1"); err != nil || tk != "" || a2.Locked() {
		t.Fatal("login", err)
	}
	// 2FA
	sec, uri, err := a2.TOTPBegin()
	if err != nil || len(uri) < 40 {
		t.Fatal(err)
	}
	raw, _ := b32.DecodeString(sec)
	now := time.Now().Unix() / 30
	if err := a2.TOTPConfirm("secret1", "000000"); err != errBadCode && totpCode(raw, now) != "000000" {
		t.Fatal("bad code accepted")
	}
	if err := a2.TOTPConfirm("wrong", totpCode(raw, now)); err != errBadPassword {
		t.Fatal("2FA enrolment without password", err)
	}
	if err := a2.TOTPConfirm("secret1", totpCode(raw, now)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a2.TOTPBegin(); err != errTOTPOn {
		t.Fatal("2FA secret replaceable while on")
	}
	a2.Lock()
	tk, err := a2.Login("secret1")
	if err != nil || tk == "" || !a2.Locked() {
		t.Fatal("2fa must need ticket", err)
	}
	// same code as used at confirm must be rejected (replay); next step works
	if err := a2.LoginCode(tk, totpCode(raw, now)); err == nil {
		t.Fatal("replayed code accepted")
	}
	if err := a2.LoginCode(tk, totpCode(raw, now+1)); err != nil || a2.Locked() {
		t.Fatal("code login", err)
	}
	// recovery resets password and 2FA
	a2.Lock()
	if _, err := a2.Recover("AAAA-BBBB-CCCC-DDDD", "newpass1"); err != errBadKey {
		t.Fatal(err)
	}
	nk, err := a2.Recover(" "+key+" ", "newpass1")
	if err != nil || nk == key || a2.Locked() || a2.Status().TOTP {
		t.Fatal("recover", err)
	}
	if err := a2.ChangePassword("secret1", "x123456"); err != errBadPassword {
		t.Fatal(err)
	}
	if err := a2.ChangePassword("newpass1", "x123456"); err != nil {
		t.Fatal(err)
	}
	// avatar
	img := image.NewRGBA(image.Rect(0, 0, 640, 480))
	for i := range img.Pix {
		img.Pix[i] = 200
	}
	img.Set(320, 240, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	if err := a2.SetAvatar(buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	out, err := png.Decode(bytes.NewReader(a2.AvatarPNG()))
	if err != nil || out.Bounds().Dx() != 256 || out.Bounds().Dy() != 256 {
		t.Fatal("avatar", err)
	}
	if err := a2.SetAvatar([]byte("not an image")); err != errBadImage {
		t.Fatal(err)
	}
	if err := a2.Remove("x123456"); err != nil || a2.Status().Exists || a2.Locked() {
		t.Fatal("remove", err)
	}
	if _, err := os.Stat(a2.avatarFile()); !os.IsNotExist(err) {
		t.Fatal("avatar left behind")
	}
}

func TestAuthThrottle(t *testing.T) {
	a := newTestAuth(t)
	_, _ = a.Setup("x", "secret1")
	a.Lock()
	for i := 0; i < 5; i++ {
		_, _ = a.Login("nope")
	}
	if _, err := a.Login("secret1"); err != errTooMany {
		t.Fatal("expected throttling, got", err)
	}
	if a.Status().WaitUntil == 0 {
		t.Fatal("waitUntil missing")
	}
	// persisted across restarts
	b := OpenAuth(a.dir)
	if _, err := b.Login("secret1"); err != errTooMany {
		t.Fatal("throttle not persisted")
	}
}

func TestAuthHardening(t *testing.T) {
	if totpCheck("", totpCode([]byte{}, time.Now().Unix()/30), time.Now(), 0) >= 0 {
		t.Fatal("empty secret accepted")
	}
	a := newTestAuth(t)
	_, _ = a.Setup("x", "secret1")
	// huge declared dimensions must be rejected before decoding
	var buf bytes.Buffer
	_ = png.Encode(&buf, image.NewGray(image.Rect(0, 0, 20, 20)))
	b := buf.Bytes()
	b[16], b[17], b[18], b[19] = 0, 0, 0xEA, 0x60 // width 60000
	b[20], b[21], b[22], b[23] = 0, 0, 0xEA, 0x60
	if err := a.SetAvatar(b); err != errBadImage {
		t.Fatal("huge avatar not rejected", err)
	}
	// lost secret: code login refused, not fail-open
	sec, _, _ := a.TOTPBegin()
	raw, _ := b32.DecodeString(sec)
	_ = a.TOTPConfirm("secret1", totpCode(raw, time.Now().Unix()/30))
	a.acc.TOTP = "garbage"
	a.Lock()
	tk, _ := a.Login("secret1")
	if err := a.LoginCode(tk, totpCode([]byte{}, time.Now().Unix()/30)); err != errSecretLost {
		t.Fatal("expected secret_lost", err)
	}
	// damaged account file stays locked
	_ = os.WriteFile(a.file(), []byte("{broken"), 0o644)
	d := OpenAuth(a.dir)
	d.admin = true
	if !d.Locked() || !d.Status().Damaged {
		t.Fatal("damaged account must stay locked")
	}
	if _, err := d.Setup("y", "secret2"); err == nil {
		t.Fatal("setup over damaged account")
	}
}

func TestAuthNeedsAdmin(t *testing.T) {
	a := OpenAuth(filepath.Join(t.TempDir(), "account"))
	if _, err := a.Setup("x", "secret1"); err != errNoAccess {
		t.Fatal("limited engine must not create accounts")
	}
}
