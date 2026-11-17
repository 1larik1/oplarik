package main

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"sync"
)

type TweakState struct {
	Active  bool     `json:"a"`
	Partial bool     `json:"p,omitempty"`
	NA      bool     `json:"na,omitempty"`
	Unknown bool     `json:"u,omitempty"`
	Applied bool     `json:"ap,omitempty"`
	Fit     bool     `json:"f"`
	Rec     bool     `json:"r"`
	Why     []string `json:"w,omitempty"`
}

type Engine struct {
	sys   Sys
	cat   *Catalog
	store *Store
	admin bool

	mu sync.Mutex // serializes system changes

	cmu     sync.Mutex
	psCache map[string]string
	deb     *debloatData
	info    *SysInfo
}

func NewEngine(sys Sys, cat *Catalog, st *Store, admin bool) *Engine {
	return &Engine{sys: sys, cat: cat, store: st, admin: admin, psCache: map[string]string{}}
}

func (e *Engine) Info() *SysInfo {
	e.cmu.Lock()
	defer e.cmu.Unlock()
	return e.info
}

func (e *Engine) SetInfo(i *SysInfo) {
	e.cmu.Lock()
	e.info = i
	e.cmu.Unlock()
}

func (e *Engine) Debloat() *debloatData {
	e.cmu.Lock()
	defer e.cmu.Unlock()
	return e.deb
}

// refreshPS runs PowerShell checks for ps-based tweaks that are not cached.
func (e *Engine) refreshPS(force bool) map[string]string {
	e.cmu.Lock()
	need := map[string]string{}
	for _, t := range e.cat.Tweaks {
		for _, op := range t.Ops {
			if op.T == "ps" && op.Check != "" {
				if _, ok := e.psCache[t.ID]; !ok || force {
					need[t.ID] = op.Check
				}
			}
		}
	}
	e.cmu.Unlock()
	if len(need) > 0 {
		res := e.sys.PSChecks(need)
		e.cmu.Lock()
		for id := range need {
			if r, ok := res[id]; ok {
				e.psCache[id] = r
			} else {
				delete(e.psCache, id)
			}
		}
		e.cmu.Unlock()
	}
	e.cmu.Lock()
	defer e.cmu.Unlock()
	cp := map[string]string{}
	for k, v := range e.psCache {
		cp[k] = v
	}
	return cp
}

func (e *Engine) ctx() *stateCtx {
	e.cmu.Lock()
	defer e.cmu.Unlock()
	cp := map[string]string{}
	for k, v := range e.psCache {
		cp[k] = v
	}
	return &stateCtx{ps: cp, deb: e.deb}
}

// States computes the state of every tweak.
func (e *Engine) States(forcePS bool) map[string]TweakState {
	ps := e.refreshPS(forcePS)
	ctx := &stateCtx{ps: ps, deb: e.Debloat()}
	info := e.Info()
	applied := map[string]bool{}
	e.store.Read(func(d *StoreData) {
		for id := range d.Applied {
			applied[id] = true
		}
	})
	out := make(map[string]TweakState, len(e.cat.Tweaks))
	for _, t := range e.cat.Tweaks {
		out[t.ID] = e.tweakState(t, ctx, info, applied[t.ID])
	}
	return out
}

func (e *Engine) tweakState(t *Tweak, ctx *stateCtx, info *SysInfo, applied bool) TweakState {
	st := TweakState{Applied: applied, Fit: true}
	for k, v := range t.Req {
		if v && !info.Cond(k) {
			st.NA = true
			st.Why = append(st.Why, k)
		}
	}
	for k, v := range t.When {
		if v && !info.Cond(k) {
			st.Fit = false
			st.Why = append(st.Why, k)
		}
	}
	if st.NA {
		return st
	}
	on, total, unknown := 0, 0, false
	for i := range t.Ops {
		switch e.opState(t, &t.Ops[i], ctx) {
		case stNA:
		case stOn:
			on++
			total++
		case stOff:
			total++
		case stUnknown:
			unknown = true
		}
	}
	if unknown {
		st.Unknown = true
		return st
	}
	if total == 0 {
		st.NA = true
		st.Why = append(st.Why, "missing")
		return st
	}
	st.Active = on == total
	st.Partial = on > 0 && on < total
	st.Rec = t.Rec && st.Fit
	return st
}

// orderForApply puts the power-plan switch first so per-plan power
// settings land on the plan that ends up active.
func (e *Engine) orderForApply(ids []string) []*Tweak {
	var ts []*Tweak
	seen := map[string]bool{}
	for _, id := range ids {
		if t := e.cat.Get(id); t != nil && !seen[id] {
			seen[id] = true
			ts = append(ts, t)
		}
	}
	sort.SliceStable(ts, func(i, j int) bool {
		return ts[i].HasOp("plan") && !ts[j].HasOp("plan")
	})
	return ts
}

type ItemResult struct {
	ID    string `json:"id"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

var errNeedAdmin = errors.New("administrator rights required")

// Apply applies tweaks, backing up originals first.
func (e *Engine) Apply(ids []string, progress func(done int, id string)) []ItemResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	ctx := e.ctx()
	var res []ItemResult
	touched := map[string]bool{}
	for i, t := range e.orderForApply(ids) {
		if progress != nil {
			progress(i, t.ID)
		}
		r := ItemResult{ID: t.ID, OK: true}
		if !e.admin {
			r.OK, r.Error = false, errNeedAdmin.Error()
			res = append(res, r)
			continue
		}
		var prev *Applied
		e.store.Read(func(d *StoreData) {
			if a := d.Applied[t.ID]; a != nil {
				cp := *a
				cp.B = map[string]json.RawMessage{}
				for k, v := range a.B {
					cp.B[k] = v
				}
				prev = &cp
			}
		})
		rec := prev
		if rec == nil {
			rec = &Applied{At: nowStr(), B: map[string]json.RawMessage{}}
		}
		var errs []string
		for oi := range t.Ops {
			op := &t.Ops[oi]
			touched[op.T] = true
			cur := e.opState(t, op, ctx)
			if cur == stNA {
				continue
			}
			if cur == stOn && op.T != "plan" {
				// Already in the wanted state: remember original state once.
				if _, ok := rec.B[strconv.Itoa(oi)]; !ok && op.T != "appx" {
					if bk, err := e.snapshot(t, op, ctx); err == nil && bk != nil {
						rec.B[strconv.Itoa(oi)] = bk
					}
				}
				continue
			}
			bk, err := e.opApply(t, op, ctx)
			if errors.Is(err, errNA) {
				continue
			}
			if bk != nil {
				if _, ok := rec.B[strconv.Itoa(oi)]; !ok {
					rec.B[strconv.Itoa(oi)] = bk
				}
			}
			if err != nil {
				errs = append(errs, err.Error())
			}
		}
		if len(errs) > 0 {
			r.OK = false
			r.Error = errs[0]
		}
		isApp := t.HasOp("appx")
		e.store.Update(func(d *StoreData) {
			if !isApp && len(rec.B) > 0 {
				d.Applied[t.ID] = rec
			}
			d.History = append(d.History, HistItem{At: nowStr(), Action: "apply", ID: t.ID, OK: r.OK, Err: r.Error})
		})
		if t.HasOp("ps") {
			e.cmu.Lock()
			delete(e.psCache, t.ID)
			e.cmu.Unlock()
		}
		postTweak(e.sys, t.ID)
		res = append(res, r)
	}
	e.afterDebloatChange(touched)
	return res
}

// snapshot captures the current value for an op without changing it, so a
// tweak that was already partly applied can still be reverted precisely.
func (e *Engine) snapshot(t *Tweak, op *Op, ctx *stateCtx) (json.RawMessage, error) {
	s := e.sys
	switch op.T {
	case "reg":
		v, err := s.RegGet(op.K, op.N)
		if err != nil {
			return nil, err
		}
		return mustJSON(v), nil
	case "dxkv":
		v, _ := s.RegGet(dxKey, dxValue)
		return mustJSON(v), nil
	case "svc":
		start, ok := s.SvcStart(op.N)
		if !ok {
			return nil, errNA
		}
		return mustJSON(svcBackup{Start: start}), nil
	case "task":
		exists, en := s.TaskState(op.Path)
		if !exists {
			return nil, errNA
		}
		return mustJSON(taskBackup{Enabled: en}), nil
	case "power":
		ac, dc, ok := s.PowerGet(op.Sub, op.Set)
		if !ok {
			return nil, errNA
		}
		return mustJSON(powerBackup{AC: ac, DC: dc}), nil
	case "exec":
		return mustJSON(wasBackup{Was: true}), nil
	case "feature":
		if ctx.deb != nil {
			return mustJSON(stateBackup{State: ctx.deb.Features[op.N]}), nil
		}
	case "cap":
		if ctx.deb != nil {
			return mustJSON(ctx.deb.Caps[op.N]), nil
		}
	}
	return nil, nil
}

// Revert restores originals (or Windows defaults when no backup exists).
func (e *Engine) Revert(ids []string, progress func(done int, id string)) []ItemResult {
	e.mu.Lock()
	defer e.mu.Unlock()
	ctx := e.ctx()
	var res []ItemResult
	touched := map[string]bool{}
	for i, id := range ids {
		t := e.cat.Get(id)
		if t == nil {
			continue
		}
		if progress != nil {
			progress(i, id)
		}
		r := ItemResult{ID: id, OK: true}
		if !e.admin {
			r.OK, r.Error = false, errNeedAdmin.Error()
			res = append(res, r)
			continue
		}
		var rec *Applied
		e.store.Read(func(d *StoreData) { rec = d.Applied[id] })
		var errs []string
		for oi := len(t.Ops) - 1; oi >= 0; oi-- {
			op := &t.Ops[oi]
			touched[op.T] = true
			var bk json.RawMessage
			if rec != nil {
				bk = rec.B[strconv.Itoa(oi)]
			}
			cur := e.opState(t, op, ctx)
			if cur == stNA {
				continue
			}
			if rec == nil && cur != stOn && op.T != "appx" {
				continue // nothing to undo for this op
			}
			if err := e.opRevert(t, op, bk, ctx); err != nil && !errors.Is(err, errNA) {
				errs = append(errs, err.Error())
			}
		}
		if len(errs) > 0 {
			r.OK, r.Error = false, errs[0]
		}
		e.store.Update(func(d *StoreData) {
			if r.OK {
				delete(d.Applied, id)
			}
			d.History = append(d.History, HistItem{At: nowStr(), Action: "revert", ID: id, OK: r.OK, Err: r.Error})
		})
		if t.HasOp("ps") {
			e.cmu.Lock()
			delete(e.psCache, id)
			e.cmu.Unlock()
		}
		postTweak(e.sys, id)
		res = append(res, r)
	}
	e.afterDebloatChange(touched)
	return res
}

// afterDebloatChange refreshes only the debloat data that changed.
func (e *Engine) afterDebloatChange(touched map[string]bool) {
	if e.Debloat() == nil {
		return
	}
	if touched["appx"] {
		if list, err := e.sys.AppxInstalled(); err == nil {
			e.cmu.Lock()
			e.deb.Appx = list
			e.cmu.Unlock()
		}
	}
	if touched["feature"] {
		if m, err := e.sys.Features(e.featureNames()); err == nil {
			e.cmu.Lock()
			e.deb.Features = m
			e.cmu.Unlock()
		}
	}
	if touched["cap"] {
		if m, err := e.sys.Caps(e.capNames()); err == nil {
			e.cmu.Lock()
			e.deb.Caps = m
			e.cmu.Unlock()
		}
	}
}

func (e *Engine) featureNames() []string {
	var out []string
	for _, t := range e.cat.Tweaks {
		for _, op := range t.Ops {
			if op.T == "feature" {
				out = append(out, op.N)
			}
		}
	}
	return out
}

func (e *Engine) capNames() []string {
	var out []string
	for _, t := range e.cat.Tweaks {
		for _, op := range t.Ops {
			if op.T == "cap" {
				out = append(out, op.N)
			}
		}
	}
	return out
}

// ScanDebloat loads installed apps, optional features and capabilities.
func (e *Engine) ScanDebloat() error {
	var wg sync.WaitGroup
	var appx []string
	var feats map[string]string
	var caps map[string]CapInfo
	var e1, e2, e3 error
	wg.Add(3)
	go func() { defer wg.Done(); appx, e1 = e.sys.AppxInstalled() }()
	go func() { defer wg.Done(); feats, e2 = e.sys.Features(e.featureNames()) }()
	go func() { defer wg.Done(); caps, e3 = e.sys.Caps(e.capNames()) }()
	wg.Wait()
	if e1 != nil {
		return e1
	}
	if feats == nil {
		feats = map[string]string{}
	}
	if caps == nil {
		caps = map[string]CapInfo{}
	}
	_ = e2
	_ = e3
	e.cmu.Lock()
	e.deb = &debloatData{Appx: appx, Features: feats, Caps: caps, At: nowStr()}
	e.cmu.Unlock()
	return nil
}

// PresetIDs resolves a preset into the tweaks it would apply on this PC.
func (e *Engine) PresetIDs(id string, states map[string]TweakState) []string {
	var p *Preset
	for i := range e.cat.Presets {
		if e.cat.Presets[i].ID == id {
			p = &e.cat.Presets[i]
		}
	}
	if p == nil {
		return nil
	}
	var out []string
	add := func(t *Tweak) {
		st := states[t.ID]
		if st.NA || !st.Fit {
			return
		}
		out = append(out, t.ID)
	}
	extra := map[string]bool{}
	for _, x := range p.Extra {
		extra[x] = true
	}
	for _, t := range e.cat.Tweaks {
		if !t.IsOptimization() {
			continue
		}
		switch p.Tweaks {
		case "auto:rec":
			if t.Rec && t.Risk == "safe" {
				add(t)
			}
		case "auto:competitive":
			if (t.Rec && t.Risk == "safe") || extra[t.ID] {
				add(t)
			}
		case "auto:privacy":
			if t.Cat == "privacy" && t.Risk != "advanced" {
				add(t)
			}
		}
	}
	return out
}

// Score = share of recommended optimizations that are active.
func (e *Engine) Score(states map[string]TweakState) (score float64, active, total int) {
	for _, t := range e.cat.Tweaks {
		if !t.IsOptimization() || !t.Rec {
			continue
		}
		st := states[t.ID]
		if st.NA || !st.Fit || st.Unknown {
			continue
		}
		total++
		if st.Active {
			active++
		}
	}
	if total == 0 {
		return 0, 0, 0
	}
	return float64(active) * 100 / float64(total), active, total
}

const restorePointScript = `
$k = 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion\SystemRestore'
$old = (Get-ItemProperty -Path $k -Name SystemRestorePointCreationFrequency -ErrorAction SilentlyContinue).SystemRestorePointCreationFrequency
New-ItemProperty -Path $k -Name SystemRestorePointCreationFrequency -Value 0 -PropertyType DWord -Force | Out-Null
try { Enable-ComputerRestore -Drive ($env:SystemDrive + '\') -ErrorAction Stop } catch {}
try {
  Checkpoint-Computer -Description 'Larik - before optimizations' -RestorePointType MODIFY_SETTINGS -ErrorAction Stop
  'ok'
} finally {
  if($null -eq $old){ Remove-ItemProperty -Path $k -Name SystemRestorePointCreationFrequency -ErrorAction SilentlyContinue }
  else { New-ItemProperty -Path $k -Name SystemRestorePointCreationFrequency -Value ([int]$old) -PropertyType DWord -Force | Out-Null }
}
`

func (e *Engine) CreateRestorePoint() error {
	if !e.admin {
		return errNeedAdmin
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	_, err := e.sys.PSRun("restore_point", "apply", restorePointScript, "")
	if err == nil {
		e.store.Update(func(d *StoreData) { d.RestorePoints = append(d.RestorePoints, nowStr()) })
	}
	return err
}
