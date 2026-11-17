package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
)

const (
	stNA      = -1
	stOff     = 0
	stOn      = 1
	stUnknown = 2
)

// stateCtx carries data fetched once per state refresh.
type stateCtx struct {
	ps  map[string]string // tweak id -> "1"/"0"/"na"
	deb *debloatData      // nil when the debloat scan has not run yet
}

type debloatData struct {
	Appx     []string           `json:"appx"`
	Features map[string]string  `json:"features"`
	Caps     map[string]CapInfo `json:"caps"`
	At       string             `json:"at"`
}

// ---------------------------------------------------------------- state

func (e *Engine) opState(t *Tweak, op *Op, ctx *stateCtx) int {
	s := e.sys
	switch op.T {
	case "reg":
		want, err := targetVal(op.VT, op.V)
		if err != nil {
			return stNA
		}
		cur, err := s.RegGet(op.K, op.N)
		if err != nil {
			return stOff
		}
		if sameValue(cur, want) {
			return stOn
		}
		return stOff
	case "regkey":
		if !s.RegKeyExists(op.K) {
			return stOff
		}
		cur, _ := s.RegGet(op.K, "")
		cs, ok := cur.Str()
		if ok && op.DV != nil && cs == *op.DV {
			return stOn
		}
		return stOff
	case "regeach":
		subs := e.eligibleSubkeys(op)
		if len(subs) == 0 {
			return stNA
		}
		want, err := targetVal(op.VT, op.V)
		if err != nil {
			return stNA
		}
		for _, sk := range subs {
			cur, _ := s.RegGet(sk, op.N)
			if !sameValue(cur, want) {
				return stOff
			}
		}
		return stOn
	case "dxkv":
		cur, _ := s.RegGet(dxKey, dxValue)
		str, _ := cur.Str()
		_, m := parseKV(str)
		if m[op.Key] == op.Val {
			return stOn
		}
		return stOff
	case "svc":
		start, ok := s.SvcStart(op.N)
		if !ok {
			return stNA
		}
		if start == op.S {
			return stOn
		}
		return stOff
	case "task":
		exists, enabled := s.TaskState(op.Path)
		if !exists {
			return stNA
		}
		if !enabled {
			return stOn
		}
		return stOff
	case "power":
		ac, dc, ok := s.PowerGet(op.Sub, op.Set)
		if !ok {
			return stNA
		}
		want, _ := rawNum(op.V)
		if ac == want && (!op.DC || dc == want) {
			return stOn
		}
		return stOff
	case "plan":
		switch strings.ToLower(s.ActivePlan()) {
		case planOurs, planUltimate, planHighPerf:
			return stOn
		}
		return stOff
	case "exec":
		if op.Detect == nil {
			return stUnknown
		}
		cur, _ := s.RegGet(op.Detect.K, op.Detect.N)
		if n, ok := cur.Num(); ok && n == op.Detect.V {
			return stOn
		}
		return stOff
	case "ps":
		r, ok := ctx.ps[t.ID]
		if !ok {
			return stUnknown
		}
		switch strings.TrimSpace(r) {
		case "1":
			return stOn
		case "0":
			return stOff
		case "na":
			return stNA
		}
		return stUnknown
	case "appx":
		if ctx.deb == nil {
			return stUnknown
		}
		if appxPresent(ctx.deb.Appx, op.P) {
			return stOff
		}
		return stOn
	case "feature":
		if ctx.deb == nil {
			return stUnknown
		}
		st, ok := ctx.deb.Features[op.N]
		if !ok {
			return stNA
		}
		if st == "Enabled" || st == "EnablePending" {
			return stOff
		}
		return stOn
	case "cap":
		if ctx.deb == nil {
			return stUnknown
		}
		c, ok := ctx.deb.Caps[op.N]
		if !ok {
			return stNA
		}
		if c.State == "Installed" || c.State == "InstallPending" {
			return stOff
		}
		return stOn
	}
	return stNA
}

func appxPresent(installed []string, patterns []string) bool {
	for _, name := range installed {
		for _, p := range patterns {
			if ok, _ := path.Match(strings.ToLower(p), strings.ToLower(name)); ok {
				return true
			}
		}
	}
	return false
}

func (e *Engine) eligibleSubkeys(op *Op) []string {
	subs, err := e.sys.RegSubkeys(op.K)
	if err != nil {
		return nil
	}
	var out []string
	for _, name := range subs {
		full := op.K + `\` + name
		for _, rv := range op.Req {
			v, err := e.sys.RegGet(full, rv)
			if err != nil {
				continue
			}
			if s, ok := v.Str(); ok {
				s = strings.TrimSpace(s)
				if s != "" && s != "0.0.0.0" {
					out = append(out, full)
					break
				}
			}
		}
	}
	return out
}

const dxKey = `HKCU\Software\Microsoft\DirectX\UserGpuPreferences`
const dxValue = "DirectXUserGlobalSettings"

// ---------------------------------------------------------------- apply

type svcBackup struct {
	Start string `json:"start"`
}
type taskBackup struct {
	Enabled bool `json:"enabled"`
}
type powerBackup struct {
	AC int `json:"ac"`
	DC int `json:"dc"`
}
type planBackup struct {
	GUID string `json:"guid"`
}
type wasBackup struct {
	Was bool `json:"was"`
}
type psBackup struct {
	Data string `json:"data"`
}
type keyBackup struct {
	Existed bool   `json:"existed"`
	Default RegVal `json:"default"`
}
type stateBackup struct {
	State string `json:"state"`
}

var errNA = errors.New("not applicable")

func mustJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

// opApply performs an op and returns a backup of what it changed.
func (e *Engine) opApply(t *Tweak, op *Op, ctx *stateCtx) (json.RawMessage, error) {
	s := e.sys
	switch op.T {
	case "reg":
		want, err := targetVal(op.VT, op.V)
		if err != nil {
			return nil, err
		}
		cur, err := s.RegGet(op.K, op.N)
		if err != nil {
			return nil, err
		}
		if err := s.RegSet(op.K, op.N, want); err != nil {
			return nil, err
		}
		return mustJSON(cur), nil
	case "regkey":
		existed := s.RegKeyExists(op.K)
		var def RegVal
		if existed {
			def, _ = s.RegGet(op.K, "")
		} else {
			def = RegVal{Absent: true}
		}
		if err := s.RegCreateKey(op.K); err != nil {
			return nil, err
		}
		dv := ""
		if op.DV != nil {
			dv = *op.DV
		}
		if err := s.RegSet(op.K, "", StrVal(dv)); err != nil {
			return nil, err
		}
		return mustJSON(keyBackup{Existed: existed, Default: def}), nil
	case "regeach":
		want, err := targetVal(op.VT, op.V)
		if err != nil {
			return nil, err
		}
		subs := e.eligibleSubkeys(op)
		if len(subs) == 0 {
			return nil, errNA
		}
		bk := map[string]RegVal{}
		for _, sk := range subs {
			cur, _ := s.RegGet(sk, op.N)
			bk[sk] = cur
			if err := s.RegSet(sk, op.N, want); err != nil {
				return mustJSON(bk), err
			}
		}
		return mustJSON(bk), nil
	case "dxkv":
		cur, _ := s.RegGet(dxKey, dxValue)
		str, _ := cur.Str()
		keys, m := parseKV(str)
		if _, ok := m[op.Key]; !ok {
			keys = append(keys, op.Key)
		}
		m[op.Key] = op.Val
		if err := s.RegSet(dxKey, dxValue, StrVal(buildKV(keys, m))); err != nil {
			return nil, err
		}
		return mustJSON(cur), nil
	case "svc":
		start, ok := s.SvcStart(op.N)
		if !ok {
			return nil, errNA
		}
		if err := s.SvcConfig(op.N, op.S); err != nil {
			return nil, err
		}
		if op.S == "disabled" {
			s.SvcStop(op.N)
		}
		return mustJSON(svcBackup{Start: start}), nil
	case "task":
		exists, enabled := s.TaskState(op.Path)
		if !exists {
			return nil, errNA
		}
		if err := s.TaskSet(op.Path, false); err != nil {
			return nil, err
		}
		return mustJSON(taskBackup{Enabled: enabled}), nil
	case "power":
		ac, dc, ok := s.PowerGet(op.Sub, op.Set)
		if !ok {
			return nil, errNA
		}
		want, _ := rawNum(op.V)
		if err := s.PowerSet(op.Sub, op.Set, want, want, op.DC); err != nil {
			return nil, err
		}
		return mustJSON(powerBackup{AC: ac, DC: dc}), nil
	case "plan":
		cur := strings.ToLower(s.ActivePlan())
		if err := e.activateMaxPlan(); err != nil {
			return nil, err
		}
		return mustJSON(planBackup{GUID: cur}), nil
	case "exec":
		was := e.opState(t, op, ctx) == stOn
		if !was {
			if _, err := s.Exec(op.Run...); err != nil {
				return nil, err
			}
		}
		return mustJSON(wasBackup{Was: was}), nil
	case "ps":
		out, err := s.PSRun(t.ID, "apply", op.Apply, "")
		if err != nil {
			return nil, err
		}
		data := ""
		for _, line := range strings.Split(out, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "@@BACKUP@@") {
				data = strings.TrimPrefix(line, "@@BACKUP@@")
			}
		}
		return mustJSON(psBackup{Data: data}), nil
	case "appx":
		return nil, s.AppxRemove(op.P)
	case "feature":
		st := ""
		if ctx.deb != nil {
			st = ctx.deb.Features[op.N]
		}
		if st == "" {
			return nil, errNA
		}
		if err := s.FeatureSet(op.N, false); err != nil {
			return nil, err
		}
		return mustJSON(stateBackup{State: st}), nil
	case "cap":
		var c CapInfo
		if ctx.deb != nil {
			c = ctx.deb.Caps[op.N]
		}
		if c.Name == "" {
			return nil, errNA
		}
		if c.State != "Installed" {
			return mustJSON(c), nil
		}
		if err := s.CapRemove(c.Name); err != nil {
			return nil, err
		}
		return mustJSON(c), nil
	}
	return nil, fmt.Errorf("unknown op %q", op.T)
}

func (e *Engine) activateMaxPlan() error { return activateMaxPlan(e.sys) }

// ---------------------------------------------------------------- revert

func (e *Engine) opRevert(t *Tweak, op *Op, bk json.RawMessage, ctx *stateCtx) error {
	s := e.sys
	has := len(bk) > 0 && string(bk) != "null"
	switch op.T {
	case "reg":
		var v RegVal
		if has {
			if err := json.Unmarshal(bk, &v); err != nil {
				return err
			}
		} else {
			d, err := targetVal(op.VT, op.D)
			if err != nil {
				return err
			}
			v = d
		}
		if v.Absent {
			return s.RegDelete(op.K, op.N)
		}
		return s.RegSet(op.K, op.N, v)
	case "regkey":
		var b keyBackup
		if has {
			_ = json.Unmarshal(bk, &b)
		}
		if !b.Existed {
			return s.RegDeleteTree(op.K)
		}
		if b.Default.Absent {
			return s.RegDelete(op.K, "")
		}
		return s.RegSet(op.K, "", b.Default)
	case "regeach":
		m := map[string]RegVal{}
		if has {
			_ = json.Unmarshal(bk, &m)
		} else {
			d, _ := targetVal(op.VT, op.D)
			for _, sk := range e.eligibleSubkeys(op) {
				m[sk] = d
			}
		}
		var firstErr error
		for sk, v := range m {
			var err error
			if v.Absent {
				err = s.RegDelete(sk, op.N)
			} else {
				err = s.RegSet(sk, op.N, v)
			}
			if err != nil && firstErr == nil {
				firstErr = err
			}
		}
		return firstErr
	case "dxkv":
		if has {
			var v RegVal
			if err := json.Unmarshal(bk, &v); err != nil {
				return err
			}
			if v.Absent {
				return s.RegDelete(dxKey, dxValue)
			}
			return s.RegSet(dxKey, dxValue, v)
		}
		cur, _ := s.RegGet(dxKey, dxValue)
		str, _ := cur.Str()
		keys, m := parseKV(str)
		delete(m, op.Key)
		out := buildKV(keys, m)
		if out == "" {
			return s.RegDelete(dxKey, dxValue)
		}
		return s.RegSet(dxKey, dxValue, StrVal(out))
	case "svc":
		start := rawStr(op.D)
		if has {
			var b svcBackup
			_ = json.Unmarshal(bk, &b)
			if b.Start != "" {
				start = b.Start
			}
		}
		if start == "" {
			start = "demand"
		}
		if err := s.SvcConfig(op.N, start); err != nil {
			return err
		}
		if start == "auto" || start == "delayed-auto" {
			s.SvcRun(op.N)
		}
		return nil
	case "task":
		enabled := true
		if has {
			var b taskBackup
			_ = json.Unmarshal(bk, &b)
			enabled = b.Enabled
		}
		return s.TaskSet(op.Path, enabled)
	case "power":
		if has {
			var b powerBackup
			_ = json.Unmarshal(bk, &b)
			return s.PowerSet(op.Sub, op.Set, b.AC, b.DC, op.DC)
		}
		d, ok := rawNum(op.D)
		if !ok {
			return nil
		}
		return s.PowerSet(op.Sub, op.Set, d, d, op.DC)
	case "plan":
		guid := planBalanced
		if has {
			var b planBackup
			_ = json.Unmarshal(bk, &b)
			switch b.GUID {
			case "", planOurs, planUltimate, planHighPerf:
			default:
				guid = b.GUID
			}
		}
		if _, err := s.Exec("powercfg", "-setactive", guid); err != nil {
			if _, err2 := s.Exec("powercfg", "-setactive", planBalanced); err2 != nil {
				return err
			}
		}
		if s.PlanExists(planOurs) {
			_, _ = s.Exec("powercfg", "-delete", planOurs)
		}
		return nil
	case "exec":
		if has {
			var b wasBackup
			_ = json.Unmarshal(bk, &b)
			if b.Was {
				return nil
			}
		}
		_, err := s.Exec(op.Undo...)
		return err
	case "ps":
		data := ""
		if has {
			var b psBackup
			_ = json.Unmarshal(bk, &b)
			data = b.Data
		}
		_, err := s.PSRun(t.ID, "revert", op.Revert, data)
		return err
	case "appx":
		q := t.Store
		if q == "" {
			q = t.Name["en"]
		}
		s.OpenURL("ms-windows-store://search/?query=" + urlQueryEscape(q))
		return nil
	case "feature":
		if has {
			var b stateBackup
			_ = json.Unmarshal(bk, &b)
			if b.State != "" && b.State != "Enabled" {
				return nil
			}
		}
		return s.FeatureSet(op.N, true)
	case "cap":
		var c CapInfo
		if has {
			_ = json.Unmarshal(bk, &c)
		}
		if c.Name == "" && ctx.deb != nil {
			c = ctx.deb.Caps[op.N]
		}
		if c.Name == "" {
			return errNA
		}
		if has && c.State != "" && c.State != "Installed" {
			return nil
		}
		return s.CapAdd(c.Name)
	}
	return fmt.Errorf("unknown op %q", op.T)
}

func urlQueryEscape(s string) string {
	var sb strings.Builder
	for _, b := range []byte(s) {
		if (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '-' || b == '_' || b == '.' {
			sb.WriteByte(b)
		} else {
			fmt.Fprintf(&sb, "%%%02X", b)
		}
	}
	return sb.String()
}

// describeOp renders a human-readable line for the info drawer.
func describeOp(op *Op) map[string]any {
	m := map[string]any{"t": op.T}
	switch op.T {
	case "reg":
		v, _ := targetVal(op.VT, op.V)
		m["text"] = fmt.Sprintf(`%s\%s = %s (%s)`, op.K, nonEmpty(op.N, "(Default)"), v.Display(), strings.ToUpper(op.VT))
	case "regkey":
		m["text"] = fmt.Sprintf(`create key %s`, op.K)
	case "regeach":
		v, _ := targetVal(op.VT, op.V)
		m["text"] = fmt.Sprintf(`%s\{each active interface}\%s = %s`, op.K, op.N, v.Display())
	case "dxkv":
		m["text"] = fmt.Sprintf(`%s\%s: %s=%s`, dxKey, dxValue, op.Key, op.Val)
	case "svc":
		m["text"] = fmt.Sprintf("service %s → %s", op.N, op.S)
	case "task":
		m["text"] = fmt.Sprintf("scheduled task %s → disabled", op.Path)
	case "power":
		v, _ := rawNum(op.V)
		mode := "AC"
		if op.DC {
			mode = "AC+DC"
		}
		m["text"] = fmt.Sprintf("powercfg %s %s = %d (%s, active plan)", op.Sub, op.Set, v, mode)
	case "plan":
		m["text"] = "powercfg -duplicatescheme " + planUltimate + " → set active"
	case "exec":
		m["text"] = strings.Join(op.Run, " ")
	case "ps":
		m["text"] = "PowerShell"
		m["code"] = op.Apply
	case "appx":
		m["text"] = "Remove-AppxPackage " + strings.Join(op.P, ", ")
	case "feature":
		m["text"] = "Disable-WindowsOptionalFeature " + op.N
	case "cap":
		m["text"] = "Remove-WindowsCapability " + op.N + "~~~~"
	}
	return m
}

func nonEmpty(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
