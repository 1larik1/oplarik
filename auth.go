package main

// Local Larik account: name, profile photo, password (PBKDF2-SHA256),
// optional two-step verification with an authenticator app (TOTP, RFC 6238)
// and a recovery key. Everything stays on this PC in an admin-only folder.
// While an account exists the engine starts locked: every API call except
// sign-in returns 423 until the password (and code) are entered.

import (
	"bytes"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"image/draw"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	pbkdfIter    = 600_000
	avatarSize   = 256
	minPassword  = 6
	maxNameRunes = 32
	ticketTTL    = 5 * time.Minute
	totpPeriod   = 30
	totpDigits   = 6
)

var (
	errBadPassword = errors.New("bad_password")
	errBadCode     = errors.New("bad_code")
	errBadKey      = errors.New("bad_key")
	errNoAccount   = errors.New("no_account")
	errExists      = errors.New("exists")
	errWeak        = errors.New("weak_password")
	errBadName     = errors.New("bad_name")
	errTooMany     = errors.New("too_many")
	errNoAccess    = errors.New("needs_admin")
	errBadImage    = errors.New("bad_image")
	errSecretLost  = errors.New("secret_lost")
	errTOTPOn      = errors.New("totp_on")
)

type Account struct {
	Name      string `json:"name"`
	Salt      string `json:"salt"`
	Hash      string `json:"hash"`
	Iter      int    `json:"iter"`
	TOTP      string `json:"totp,omitempty"` // DPAPI-protected base32 secret
	TOTPOn    bool   `json:"totpOn"`
	TOTPLast  int64  `json:"totpLast,omitempty"` // last accepted time step (no replay)
	RecSalt   string `json:"recSalt"`
	RecHash   string `json:"recHash"`
	Avatar    int    `json:"avatar"` // version; 0 = none
	Created   string `json:"created"`
	Fails     int    `json:"fails,omitempty"`
	LockUntil int64  `json:"lockUntil,omitempty"`
}

type Auth struct {
	mu       sync.Mutex
	dir      string
	acc      *Account
	readErr  bool // account folder exists but can't be read (engine without admin rights)
	damaged  bool // account.json exists but is unreadable garbage: stay locked
	admin    bool
	unlocked bool
	tickets  map[string]time.Time // password accepted, waiting for the code
	pending  string               // TOTP secret being set up (base32)
	onChange func()
}

func OpenAuth(dir string) *Auth {
	a := &Auth{dir: dir, tickets: map[string]time.Time{}}
	a.load()
	a.unlocked = a.acc == nil && !a.readErr && !a.damaged
	return a
}

func (a *Auth) file() string       { return filepath.Join(a.dir, "account.json") }
func (a *Auth) avatarFile() string { return filepath.Join(a.dir, "avatar.png") }

func (a *Auth) load() {
	b, err := os.ReadFile(a.file())
	if err != nil {
		if !os.IsNotExist(err) {
			a.readErr = true
		}
		a.acc = nil
		return
	}
	var acc Account
	if json.Unmarshal(b, &acc) != nil || acc.Hash == "" {
		// Never treat a damaged account as "no account": that would unlock Larik.
		a.acc, a.damaged = nil, true
		return
	}
	a.acc = &acc
}

func (a *Auth) save() error {
	if err := os.MkdirAll(a.dir, 0o700); err != nil {
		return err
	}
	if err := secureDir(a.dir); err != nil {
		return err
	}
	return writeAtomic(a.file(), a.acc)
}

func (a *Auth) changed() {
	if a.onChange != nil {
		go a.onChange()
	}
}

// ---- status

type AuthStatus struct {
	Exists    bool   `json:"exists"`
	Locked    bool   `json:"locked"`
	Damaged   bool   `json:"damaged,omitempty"`
	Name      string `json:"name,omitempty"`
	Avatar    int    `json:"avatar"`
	TOTP      bool   `json:"totp"`
	Created   string `json:"created,omitempty"`
	NoAccess  bool   `json:"noAccess,omitempty"`
	WaitUntil int64  `json:"waitUntil,omitempty"`
}

func (a *Auth) Status() AuthStatus {
	a.mu.Lock()
	defer a.mu.Unlock()
	st := AuthStatus{Locked: !a.unlocked, NoAccess: a.readErr, Damaged: a.damaged}
	if a.acc != nil {
		st.Exists, st.Name, st.Avatar, st.TOTP, st.Created = true, a.acc.Name, a.acc.Avatar, a.acc.TOTPOn, a.acc.Created
		if a.acc.LockUntil > time.Now().UnixMilli() {
			st.WaitUntil = a.acc.LockUntil
		}
	}
	if a.readErr || a.damaged {
		st.Exists = true
	}
	return st
}

func (a *Auth) Locked() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return !a.unlocked
}

func (a *Auth) Lock() {
	a.mu.Lock()
	changed := a.acc != nil && a.unlocked
	if a.acc != nil {
		a.unlocked = false
	}
	a.tickets = map[string]time.Time{}
	a.pending = ""
	a.mu.Unlock()
	if changed {
		a.changed()
	}
}

// ---- helpers

func randBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

func hashSecret(secret string, salt []byte, iter int) []byte {
	k, _ := pbkdf2.Key(sha256.New, secret, salt, iter, 32)
	return k
}

func cleanName(s string) (string, error) {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" || utf8.RuneCountInString(s) > maxNameRunes {
		return "", errBadName
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return "", errBadName
		}
	}
	return s, nil
}

func checkPassword(p string) error {
	if utf8.RuneCountInString(p) < minPassword || len(p) > 256 {
		return errWeak
	}
	return nil
}

func (a *Auth) setPassword(p string) {
	salt := randBytes(16)
	a.acc.Salt = base64.StdEncoding.EncodeToString(salt)
	a.acc.Hash = base64.StdEncoding.EncodeToString(hashSecret(p, salt, pbkdfIter))
	a.acc.Iter = pbkdfIter
}

func (a *Auth) passwordOK(p string) bool {
	salt, _ := base64.StdEncoding.DecodeString(a.acc.Salt)
	want, _ := base64.StdEncoding.DecodeString(a.acc.Hash)
	iter := a.acc.Iter
	if iter <= 0 {
		iter = pbkdfIter
	}
	return len(want) > 0 && subtle.ConstantTimeCompare(hashSecret(p, salt, iter), want) == 1
}

// Recovery keys use an alphabet without look-alike characters (no 0/O, 1/I).
const keyAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// newRecoveryKey returns a key formatted as XXXX-XXXX-XXXX-XXXX (80 bits) and stores its hash.
func (a *Auth) newRecoveryKey() string {
	raw := make([]byte, 16)
	for i, b := range randBytes(16) {
		raw[i] = keyAlphabet[b&31]
	}
	key := string(raw[0:4]) + "-" + string(raw[4:8]) + "-" + string(raw[8:12]) + "-" + string(raw[12:16])
	salt := randBytes(16)
	a.acc.RecSalt = base64.StdEncoding.EncodeToString(salt)
	a.acc.RecHash = base64.StdEncoding.EncodeToString(hashSecret(normKey(key), salt, 100_000))
	return key
}

func normKey(k string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(k) {
		if strings.ContainsRune(keyAlphabet, r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (a *Auth) recoveryOK(key string) bool {
	salt, _ := base64.StdEncoding.DecodeString(a.acc.RecSalt)
	want, _ := base64.StdEncoding.DecodeString(a.acc.RecHash)
	return len(want) > 0 && subtle.ConstantTimeCompare(hashSecret(normKey(key), salt, 100_000), want) == 1
}

// throttle: after 5 failures wait 30 s, doubling up to 15 min.
func (a *Auth) waiting() bool {
	return a.acc.LockUntil > time.Now().UnixMilli()
}

func (a *Auth) fail() {
	a.acc.Fails++
	if a.acc.Fails >= 5 {
		d := 30 * time.Second << min(a.acc.Fails-5, 5)
		if d > 15*time.Minute {
			d = 15 * time.Minute
		}
		a.acc.LockUntil = time.Now().Add(d).UnixMilli()
	}
	if err := a.save(); err != nil {
		log.Print("account: can't save failed attempt: ", err)
	}
}

func (a *Auth) succeed() {
	a.acc.Fails, a.acc.LockUntil = 0, 0
}

// ---- TOTP

func totpCode(secret []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step))
	h := hmac.New(sha1.New, secret)
	h.Write(msg[:])
	sum := h.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	v := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	s := itoa(int(v % 1_000_000))
	for len(s) < totpDigits {
		s = "0" + s
	}
	return s
}

// totpCheck returns the matching time step (±1 step for clock drift) or -1.
func totpCheck(secretB32, code string, now time.Time, after int64) int64 {
	secret, err := b32.DecodeString(strings.ToUpper(secretB32))
	if err != nil || len(secret) < 10 {
		return -1 // never accept codes for a missing or broken secret
	}
	code = strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, code)
	if len(code) != totpDigits {
		return -1
	}
	step := now.Unix() / totpPeriod
	for _, s := range []int64{step, step - 1, step + 1} {
		if s > after && subtle.ConstantTimeCompare([]byte(totpCode(secret, s)), []byte(code)) == 1 {
			return s
		}
	}
	return -1
}

func (a *Auth) totpSecret() string {
	raw, err := base64.StdEncoding.DecodeString(a.acc.TOTP)
	if err != nil {
		return ""
	}
	plain, err := unprotectSecret(raw)
	if err != nil {
		return ""
	}
	return string(plain)
}

// ---- operations

// Setup creates the account. Returns the recovery key (shown once).
func (a *Auth) Setup(name, password string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.acc != nil || a.readErr || a.damaged {
		return "", errExists
	}
	if !a.admin {
		return "", errNoAccess
	}
	n, err := cleanName(name)
	if err != nil {
		return "", err
	}
	if err := checkPassword(password); err != nil {
		return "", err
	}
	a.acc = &Account{Name: n, Created: nowStr()}
	a.setPassword(password)
	key := a.newRecoveryKey()
	if err := a.save(); err != nil {
		a.acc = nil
		return "", err
	}
	a.unlocked = true
	return key, nil
}

// Login checks the password. If two-step verification is on it returns a
// ticket for LoginCode instead of unlocking.
func (a *Auth) Login(password string) (ticket string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.readErr {
		return "", errNoAccess
	}
	if a.acc == nil {
		return "", errNoAccount
	}
	if a.waiting() {
		return "", errTooMany
	}
	if !a.passwordOK(password) {
		a.fail()
		return "", errBadPassword
	}
	if a.acc.TOTPOn {
		t := b32.EncodeToString(randBytes(15))
		a.tickets[t] = time.Now().Add(ticketTTL)
		return t, nil
	}
	a.succeed()
	if err := a.save(); err != nil {
		log.Print("account: ", err)
	}
	a.unlocked = true
	return "", nil
}

func (a *Auth) takeTicket(t string) bool {
	exp, ok := a.tickets[t]
	if !ok || time.Now().After(exp) {
		delete(a.tickets, t)
		return false
	}
	return true
}

func (a *Auth) LoginCode(ticket, code string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.acc == nil {
		return errNoAccount
	}
	if !a.takeTicket(ticket) {
		return errors.New("expired")
	}
	if a.waiting() {
		return errTooMany
	}
	secret := a.totpSecret()
	if secret == "" {
		// DPAPI can't decrypt it (e.g. Windows reinstalled): only the recovery key helps.
		return errSecretLost
	}
	step := totpCheck(secret, code, time.Now(), a.acc.TOTPLast)
	if step < 0 {
		a.fail()
		return errBadCode
	}
	delete(a.tickets, ticket)
	a.acc.TOTPLast = step
	a.succeed()
	if err := a.save(); err != nil {
		log.Print("account: ", err)
	}
	a.unlocked = true
	return nil
}

// Recover resets the password with the recovery key, turns two-step
// verification off (the phone may be lost) and issues a new key.
func (a *Auth) Recover(key, newPassword string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.acc == nil {
		return "", errNoAccount
	}
	if a.waiting() {
		return "", errTooMany
	}
	if err := checkPassword(newPassword); err != nil {
		return "", err
	}
	if !a.recoveryOK(key) {
		a.fail()
		return "", errBadKey
	}
	a.setPassword(newPassword)
	a.acc.TOTP, a.acc.TOTPOn, a.acc.TOTPLast = "", false, 0
	nk := a.newRecoveryKey()
	a.succeed()
	if err := a.save(); err != nil {
		return "", err
	}
	a.unlocked = true
	return nk, nil
}

func (a *Auth) requireUnlocked() error {
	if a.acc == nil {
		return errNoAccount
	}
	if !a.unlocked {
		return errors.New("locked")
	}
	return nil
}

func (a *Auth) Rename(name string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.requireUnlocked(); err != nil {
		return err
	}
	n, err := cleanName(name)
	if err != nil {
		return err
	}
	a.acc.Name = n
	return a.save()
}

// verify checks the password for sensitive changes, with the same throttling as sign-in.
func (a *Auth) verify(password string) error {
	if err := a.requireUnlocked(); err != nil {
		return err
	}
	if a.waiting() {
		return errTooMany
	}
	if !a.passwordOK(password) {
		a.fail()
		return errBadPassword
	}
	a.succeed()
	return nil
}

func (a *Auth) ChangePassword(old, nw string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := checkPassword(nw); err != nil {
		return err
	}
	if err := a.verify(old); err != nil {
		return err
	}
	a.setPassword(nw)
	return a.save()
}

func (a *Auth) NewRecoveryKey(password string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.verify(password); err != nil {
		return "", err
	}
	k := a.newRecoveryKey()
	return k, a.save()
}

// TOTPBegin creates a new secret and returns it with the otpauth:// URI.
func (a *Auth) TOTPBegin() (secret, uri string, err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.requireUnlocked(); err != nil {
		return "", "", err
	}
	if a.acc.TOTPOn {
		return "", "", errTOTPOn
	}
	a.pending = b32.EncodeToString(randBytes(20))
	label := url.PathEscape(appName + ":" + a.acc.Name)
	uri = "otpauth://totp/" + label + "?secret=" + a.pending + "&issuer=" + appName + "&algorithm=SHA1&digits=6&period=30"
	return a.pending, uri, nil
}

func (a *Auth) TOTPConfirm(password, code string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.acc != nil && a.acc.TOTPOn {
		return errTOTPOn
	}
	if a.pending == "" {
		return errors.New("expired")
	}
	if err := a.verify(password); err != nil {
		return err
	}
	step := totpCheck(a.pending, code, time.Now(), 0)
	if step < 0 {
		return errBadCode
	}
	prot, err := protectSecret([]byte(a.pending))
	if err != nil {
		return err
	}
	a.acc.TOTP = base64.StdEncoding.EncodeToString(prot)
	a.acc.TOTPOn, a.acc.TOTPLast = true, step
	a.pending = ""
	return a.save()
}

func (a *Auth) TOTPDisable(password string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.verify(password); err != nil {
		return err
	}
	a.acc.TOTP, a.acc.TOTPOn, a.acc.TOTPLast = "", false, 0
	return a.save()
}

// Remove deletes the account; Larik no longer asks for a password.
func (a *Auth) Remove(password string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.verify(password); err != nil {
		return err
	}
	_ = os.Remove(a.avatarFile())
	if err := os.Remove(a.file()); err != nil && !os.IsNotExist(err) {
		return err
	}
	a.acc = nil
	a.unlocked = true
	return nil
}

// ---- avatar

func (a *Auth) SetAvatar(data []byte) error {
	a.mu.Lock()
	err := a.requireUnlocked()
	a.mu.Unlock()
	if err != nil {
		return err
	}
	// Check the declared size before decoding: a tiny file can claim a huge image.
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || cfg.Width < 16 || cfg.Height < 16 || cfg.Width > 4096 || cfg.Height > 4096 {
		return errBadImage
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return errBadImage
	}
	out := squareResize(img, avatarSize)
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.requireUnlocked(); err != nil {
		return err
	}
	if err := os.MkdirAll(a.dir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(a.avatarFile(), buf.Bytes(), 0o644); err != nil {
		return err
	}
	a.acc.Avatar++
	return a.save()
}

func (a *Auth) ClearAvatar() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := a.requireUnlocked(); err != nil {
		return err
	}
	_ = os.Remove(a.avatarFile())
	a.acc.Avatar = 0
	return a.save()
}

func (a *Auth) AvatarPNG() []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.acc == nil || a.acc.Avatar == 0 {
		return nil
	}
	b, _ := os.ReadFile(a.avatarFile())
	return b
}

// squareResize center-crops to a square and scales to size×size (area average).
func squareResize(src image.Image, size int) *image.RGBA {
	b := src.Bounds()
	side := min(b.Dx(), b.Dy())
	x0, y0 := b.Min.X+(b.Dx()-side)/2, b.Min.Y+(b.Dy()-side)/2
	rgba := image.NewRGBA(image.Rect(0, 0, side, side))
	draw.Draw(rgba, rgba.Bounds(), src, image.Pt(x0, y0), draw.Src)
	if side == size {
		return rgba
	}
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	scale := float64(side) / float64(size)
	for y := 0; y < size; y++ {
		sy0, sy1 := int(float64(y)*scale), int(float64(y+1)*scale)
		if sy1 <= sy0 {
			sy1 = sy0 + 1
		}
		for x := 0; x < size; x++ {
			sx0, sx1 := int(float64(x)*scale), int(float64(x+1)*scale)
			if sx1 <= sx0 {
				sx1 = sx0 + 1
			}
			var r, g, bl, al, n uint32
			for sy := sy0; sy < sy1 && sy < side; sy++ {
				for sx := sx0; sx < sx1 && sx < side; sx++ {
					i := rgba.PixOffset(sx, sy)
					p := rgba.Pix[i : i+4]
					r += uint32(p[0])
					g += uint32(p[1])
					bl += uint32(p[2])
					al += uint32(p[3])
					n++
				}
			}
			if n == 0 {
				continue
			}
			i := dst.PixOffset(x, y)
			dst.Pix[i], dst.Pix[i+1], dst.Pix[i+2], dst.Pix[i+3] = byte(r/n), byte(g/n), byte(bl/n), byte(al/n)
		}
	}
	return dst
}
