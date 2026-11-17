package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Text is a localized string: lang -> text.
type Text map[string]string

// Op is one reversible system change. Field meaning depends on T.
type Op struct {
	T      string          `json:"t"`
	K      string          `json:"k,omitempty"`
	N      string          `json:"n,omitempty"`
	VT     string          `json:"vt,omitempty"`
	V      json.RawMessage `json:"v,omitempty"`
	D      json.RawMessage `json:"d,omitempty"`
	DV     *string         `json:"dv,omitempty"`
	Req    []string        `json:"req,omitempty"`
	Key    string          `json:"key,omitempty"`
	Val    string          `json:"val,omitempty"`
	S      string          `json:"s,omitempty"`
	Path   string          `json:"path,omitempty"`
	Sub    string          `json:"sub,omitempty"`
	Set    string          `json:"set,omitempty"`
	DC     bool            `json:"dc,omitempty"`
	Run    []string        `json:"run,omitempty"`
	Undo   []string        `json:"undo,omitempty"`
	Detect *Detect         `json:"detect,omitempty"`
	Apply  string          `json:"apply,omitempty"`
	Revert string          `json:"revert,omitempty"`
	Check  string          `json:"check,omitempty"`
	P      []string        `json:"p,omitempty"`
}

type Detect struct {
	K string `json:"k"`
	N string `json:"n"`
	V uint64 `json:"v"`
}

type Tweak struct {
	ID      string          `json:"id"`
	Cat     string          `json:"cat"`
	Risk    string          `json:"risk"`
	Rec     bool            `json:"rec"`
	Imp     int             `json:"imp"`
	Restart *string         `json:"restart"`
	Tags    []string        `json:"tags"`
	Req     map[string]bool `json:"req"`
	When    map[string]bool `json:"when"`
	Name    Text            `json:"name"`
	Desc    Text            `json:"desc"`
	Warn    Text            `json:"warn,omitempty"`
	Store   string          `json:"store,omitempty"`
	Ops     []Op            `json:"ops"`
}

type Preset struct {
	ID     string   `json:"id"`
	Tweaks string   `json:"tweaks"`
	Extra  []string `json:"extra,omitempty"`
	Name   Text     `json:"name"`
	Desc   Text     `json:"desc"`
}

type Catalog struct {
	Version int      `json:"version"`
	Tweaks  []*Tweak `json:"tweaks"`
	Presets []Preset `json:"presets"`
	byID    map[string]*Tweak
}

func LoadCatalog(data []byte) (*Catalog, error) {
	var c Catalog
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	c.byID = map[string]*Tweak{}
	for _, t := range c.Tweaks {
		c.byID[t.ID] = t
	}
	return &c, nil
}

func (c *Catalog) Get(id string) *Tweak { return c.byID[id] }

// Optimization categories shown on the Optimize page; the rest live on Debloat.
var optimizeCats = map[string]bool{"fps": true, "power": true, "gpu": true, "input": true, "network": true, "privacy": true, "qol": true}

func (t *Tweak) IsOptimization() bool { return optimizeCats[t.Cat] }

func (t *Tweak) HasOp(kind string) bool {
	for _, o := range t.Ops {
		if o.T == kind {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------- registry values

const (
	REG_NONE      = 0
	REG_SZ        = 1
	REG_EXPAND_SZ = 2
	REG_BINARY    = 3
	REG_DWORD     = 4
	REG_MULTI_SZ  = 7
	REG_QWORD     = 11
)

// RegVal is an exact registry value (type + raw bytes) so backups restore
// precisely what was there, or Absent when the value did not exist.
type RegVal struct {
	Absent bool   `json:"a,omitempty"`
	Type   uint32 `json:"t,omitempty"`
	Data   []byte `json:"d,omitempty"`
}

func DwordVal(n uint32) RegVal {
	return RegVal{Type: REG_DWORD, Data: []byte{byte(n), byte(n >> 8), byte(n >> 16), byte(n >> 24)}}
}

func QwordVal(n uint64) RegVal {
	b := make([]byte, 8)
	for i := 0; i < 8; i++ {
		b[i] = byte(n >> (8 * i))
	}
	return RegVal{Type: REG_QWORD, Data: b}
}

func StrVal(s string) RegVal { return RegVal{Type: REG_SZ, Data: utf16Bytes(s)} }

func utf16Bytes(s string) []byte {
	var out []byte
	for _, r := range s {
		if r >= 0x10000 {
			r -= 0x10000
			hi, lo := 0xD800+(r>>10), 0xDC00+(r&0x3FF)
			out = append(out, byte(hi), byte(hi>>8), byte(lo), byte(lo>>8))
		} else {
			out = append(out, byte(r), byte(r>>8))
		}
	}
	return append(out, 0, 0)
}

func decodeUTF16(b []byte) []string {
	var parts []string
	var cur []uint16
	for i := 0; i+1 < len(b); i += 2 {
		u := uint16(b[i]) | uint16(b[i+1])<<8
		if u == 0 {
			parts = append(parts, utf16ToString(cur))
			cur = cur[:0]
			continue
		}
		cur = append(cur, u)
	}
	if len(cur) > 0 {
		parts = append(parts, utf16ToString(cur))
	}
	return parts
}

func utf16ToString(u []uint16) string {
	var sb strings.Builder
	for i := 0; i < len(u); i++ {
		c := rune(u[i])
		if c >= 0xD800 && c < 0xDC00 && i+1 < len(u) {
			c = 0x10000 + (c-0xD800)<<10 + (rune(u[i+1]) - 0xDC00)
			i++
		}
		sb.WriteRune(c)
	}
	return sb.String()
}

// Num returns the numeric value of a DWORD/QWORD.
func (v RegVal) Num() (uint64, bool) {
	if v.Absent {
		return 0, false
	}
	switch v.Type {
	case REG_DWORD:
		if len(v.Data) < 4 {
			return 0, false
		}
		return uint64(v.Data[0]) | uint64(v.Data[1])<<8 | uint64(v.Data[2])<<16 | uint64(v.Data[3])<<24, true
	case REG_QWORD:
		if len(v.Data) < 8 {
			return 0, false
		}
		var n uint64
		for i := 0; i < 8; i++ {
			n |= uint64(v.Data[i]) << (8 * i)
		}
		return n, true
	}
	return 0, false
}

// Str returns string content (multi-sz joined by spaces).
func (v RegVal) Str() (string, bool) {
	if v.Absent {
		return "", false
	}
	switch v.Type {
	case REG_SZ, REG_EXPAND_SZ:
		p := decodeUTF16(v.Data)
		if len(p) == 0 {
			return "", true
		}
		return p[0], true
	case REG_MULTI_SZ:
		var nz []string
		for _, s := range decodeUTF16(v.Data) {
			if s != "" {
				nz = append(nz, s)
			}
		}
		return strings.Join(nz, " "), true
	}
	return "", false
}

func (v RegVal) Display() string {
	if v.Absent {
		return "(absent)"
	}
	if n, ok := v.Num(); ok {
		return fmt.Sprintf("%d", n)
	}
	if s, ok := v.Str(); ok {
		return strconv.Quote(s)
	}
	return fmt.Sprintf("(%d bytes)", len(v.Data))
}

// targetVal converts an op's JSON value (v or d) into a RegVal for type vt.
// Returns absent=true when raw is JSON null.
func targetVal(vt string, raw json.RawMessage) (RegVal, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return RegVal{Absent: true}, nil
	}
	switch vt {
	case "dword":
		n, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
		if err != nil {
			return RegVal{}, err
		}
		return DwordVal(uint32(n)), nil
	case "qword":
		n, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 10, 64)
		if err != nil {
			return RegVal{}, err
		}
		return QwordVal(n), nil
	case "sz":
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return RegVal{}, err
		}
		return StrVal(s), nil
	}
	return RegVal{}, fmt.Errorf("unknown value type %q", vt)
}

// sameValue compares a current value against the target semantically.
func sameValue(cur, want RegVal) bool {
	if cur.Absent || want.Absent {
		return cur.Absent == want.Absent
	}
	if wn, ok := want.Num(); ok {
		cn, ok2 := cur.Num()
		return ok2 && cn == wn
	}
	if ws, ok := want.Str(); ok {
		cs, ok2 := cur.Str()
		return ok2 && cs == ws
	}
	return string(cur.Data) == string(want.Data)
}

func rawNum(raw json.RawMessage) (int, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(raw)))
	return n, err == nil
}

func rawStr(raw json.RawMessage) string {
	var s string
	_ = json.Unmarshal(raw, &s)
	return s
}

// ---------------------------------------------------------------- dx key/value strings

// parseKV parses "A=1;B=2;" into ordered keys.
func parseKV(s string) ([]string, map[string]string) {
	m := map[string]string{}
	var keys []string
	for _, part := range strings.Split(s, ";") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		if _, seen := m[k]; !seen {
			keys = append(keys, k)
		}
		m[k] = v
	}
	return keys, m
}

func buildKV(keys []string, m map[string]string) string {
	var sb strings.Builder
	for _, k := range keys {
		if v, ok := m[k]; ok {
			sb.WriteString(k + "=" + v + ";")
		}
	}
	return sb.String()
}
