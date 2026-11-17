package main

// Larik 1.0.5 tools: shader cache cleanup, fastest-DNS finder and the
// startup apps manager. Platform parts live in tools_windows.go and the demo
// mocks in platform_other.go.

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------------------------------------------------------------- file cleaning

type CacheDir struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
	Files int    `json:"files"`
}

// walkClean counts (or deletes) regular files under root. It never follows
// symlinks or junctions, skips files in use, and removes emptied subfolders
// but never root itself.
func walkClean(root string, del bool) (int64, int) {
	var total int64
	var n int
	var dirs []string
	st, err := os.Lstat(root)
	if err != nil || !st.IsDir() {
		return 0, 0
	}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			if p != root {
				dirs = append(dirs, p)
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if del && os.Remove(p) != nil {
			return nil // in use (a game is running) — skip
		}
		total += info.Size()
		n++
		return nil
	})
	if del {
		for i := len(dirs) - 1; i >= 0; i-- {
			_ = os.Remove(dirs[i]) // succeeds only when empty
		}
	}
	return total, n
}

func shaderScan(del bool) ([]CacheDir, int64, int) {
	var out []CacheDir
	var tb int64
	var tn int
	for _, d := range shaderCacheDirs() {
		b, n := walkClean(d.Path, del)
		if b == 0 && n == 0 {
			if _, err := os.Stat(d.Path); err != nil {
				continue
			}
		}
		d.Bytes, d.Files = b, n
		out = append(out, d)
		tb += b
		tn += n
	}
	return out, tb, tn
}

// ---------------------------------------------------------------- DNS

type DNSServer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Primary   string `json:"primary"`
	Secondary string `json:"secondary"`
}

// Public resolvers without content filtering.
var dnsServers = []DNSServer{
	{"cloudflare", "Cloudflare", "1.1.1.1", "1.0.0.1"},
	{"google", "Google", "8.8.8.8", "8.8.4.4"},
	{"quad9", "Quad9", "9.9.9.9", "149.112.112.112"},
	{"opendns", "OpenDNS", "208.67.222.222", "208.67.220.220"},
}

// Domains games and launchers really look up; resolvers have them cached,
// so the time measured is what you feel when a launcher connects.
var dnsDomains = []string{"steamcommunity.com", "api.steampowered.com", "riotgames.com", "epicgames.com", "discord.com",
	"battle.net", "ea.com", "xboxlive.com", "faceit.com", "google.com"}

type DNSResult struct {
	DNSServer
	Ms      float64 `json:"ms"` // median, -1 = no answer
	Fails   int     `json:"fails"`
	Current bool    `json:"current"`
}

func dnsQueryPacket(domain string) ([]byte, uint16) {
	var idb [2]byte
	_, _ = rand.Read(idb[:])
	id := binary.BigEndian.Uint16(idb[:])
	b := make([]byte, 12, 64)
	binary.BigEndian.PutUint16(b[0:], id)
	binary.BigEndian.PutUint16(b[2:], 0x0100) // recursion desired
	binary.BigEndian.PutUint16(b[4:], 1)      // one question
	for _, l := range strings.Split(strings.TrimSuffix(domain, "."), ".") {
		b = append(b, byte(len(l)))
		b = append(b, l...)
	}
	b = append(b, 0, 0, 1, 0, 1) // root, type A, class IN
	return b, id
}

// dnsProbe sends one query and returns the round-trip time. Replaced in demo mode.
var dnsProbe = func(server, domain string) (time.Duration, error) {
	conn, err := net.DialTimeout("udp", net.JoinHostPort(server, "53"), 1500*time.Millisecond)
	if err != nil {
		return 0, err
	}
	defer conn.Close()
	q, id := dnsQueryPacket(domain)
	start := time.Now()
	_ = conn.SetDeadline(start.Add(1500 * time.Millisecond))
	if _, err := conn.Write(q); err != nil {
		return 0, err
	}
	buf := make([]byte, 1500)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			return 0, err
		}
		if n >= 12 && binary.BigEndian.Uint16(buf[0:]) == id && buf[2]&0x80 != 0 {
			if rc := buf[3] & 0x0f; rc != 0 && rc != 3 {
				return 0, errors.New("dns error")
			}
			return time.Since(start), nil
		}
	}
}

func benchServer(srv DNSServer) DNSResult {
	r := DNSResult{DNSServer: srv, Ms: -1}
	var times []float64
	// warm-up query (first packet may include ARP / route setup)
	_, _ = dnsProbe(srv.Primary, dnsDomains[len(dnsDomains)-1])
	for i, d := range dnsDomains[:8] {
		addr := srv.Primary
		if i%4 == 3 && srv.Secondary != "" {
			addr = srv.Secondary
		}
		t, err := dnsProbe(addr, d)
		if err != nil {
			r.Fails++
			continue
		}
		times = append(times, float64(t.Microseconds())/1000)
	}
	if len(times) > 0 {
		sort.Float64s(times)
		r.Ms = times[len(times)/2]
	}
	return r
}

// benchDNS measures the public resolvers plus the ones the PC uses now.
func benchDNS(current []string) []DNSResult {
	list := append([]DNSServer{}, dnsServers...)
	if len(current) > 0 {
		cur := DNSServer{ID: "current", Name: "current", Primary: current[0]}
		if len(current) > 1 {
			cur.Secondary = current[1]
		}
		list = append([]DNSServer{cur}, list...)
	}
	out := make([]DNSResult, len(list))
	var wg sync.WaitGroup
	for i, s := range list {
		wg.Add(1)
		go func(i int, s DNSServer) {
			defer wg.Done()
			out[i] = benchServer(s)
		}(i, s)
	}
	wg.Wait()
	for i := range out {
		for _, c := range current {
			if out[i].Primary == c {
				out[i].Current = true
			}
		}
	}
	return out
}

// bestDNS picks the fastest server that answered every query.
func bestDNS(res []DNSResult) string {
	best, bestMs := "", 0.0
	for _, r := range res {
		if r.ID == "current" || r.Ms < 0 || r.Fails > 0 {
			continue
		}
		if best == "" || r.Ms < bestMs {
			best, bestMs = r.ID, r.Ms
		}
	}
	return best
}

func dnsByID(id string) (DNSServer, bool) {
	for _, s := range dnsServers {
		if s.ID == id {
			return s, true
		}
	}
	return DNSServer{}, false
}

// ---------------------------------------------------------------- startup apps

type StartupItem struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Command string `json:"command"`
	Source  string `json:"source"` // hkcu | hklm | hklm32 | ufolder | cfolder
	On      bool   `json:"on"`
	Cover   string `json:"cover,omitempty"` // program icon (1.0.8)
}

func (s *Server) withIcons(items []StartupItem) []StartupItem {
	for i := range items {
		items[i].Cover = s.coverURL("", exeFromCommand(items[i].Command, os.Getenv))
	}
	return orEmpty(items)
}

// approvedOn reads a StartupApproved value the way Task Manager does: an even
// first byte (or no value at all) means enabled.
func approvedOn(v RegVal) bool {
	if v.Absent || len(v.Data) == 0 {
		return true
	}
	return v.Data[0]&1 == 0
}

// approvedValue builds the 12-byte StartupApproved value Task Manager writes.
func approvedValue(on bool) RegVal {
	b := make([]byte, 12)
	if on {
		b[0] = 2
	} else {
		b[0] = 3
		// FILETIME of when it was disabled (shown by Task Manager)
		ft := uint64(time.Now().UnixNano()/100) + 116444736000000000
		binary.LittleEndian.PutUint64(b[4:], ft)
	}
	return RegVal{Type: REG_BINARY, Data: b}
}

// ---------------------------------------------------------------- HTTP

func (s *Server) toolRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/tools/shader", func(w http.ResponseWriter, r *http.Request) {
		dirs, b, n := shaderScan(false)
		writeJSON(w, map[string]any{"dirs": orEmpty(dirs), "bytes": b, "files": n})
	})
	mux.HandleFunc("POST /api/tools/shader", func(w http.ResponseWriter, r *http.Request) {
		_, b, n := shaderScan(true)
		dirs, left, _ := shaderScan(false)
		writeJSON(w, map[string]any{"bytes": b, "files": n, "left": left, "dirs": orEmpty(dirs)})
	})

	mux.HandleFunc("GET /api/tools/dns", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"current": orEmpty(currentDNS()), "backup": dnsHasBackup(), "servers": dnsServers})
	})
	mux.HandleFunc("POST /api/tools/dns/bench", func(w http.ResponseWriter, r *http.Request) {
		cur := currentDNS()
		res := benchDNS(cur)
		writeJSON(w, map[string]any{"results": res, "best": bestDNS(res), "current": orEmpty(cur), "backup": dnsHasBackup()})
	})
	mux.HandleFunc("POST /api/tools/dns/apply", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"id"`
		}
		if err := readJSON(r, &req); err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		if !s.e.admin {
			errJSON(w, 403, errNeedAdmin.Error())
			return
		}
		srv, ok := dnsByID(req.ID)
		if !ok {
			errJSON(w, 400, "unknown server")
			return
		}
		var tweakOn bool
		s.e.store.Read(func(d *StoreData) { _, tweakOn = d.Applied["dns_cloudflare"] })
		if tweakOn {
			errJSON(w, 409, "dns_tweak_on")
			return
		}
		if err := applyDNS(srv.Primary, srv.Secondary); err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		s.e.logHist("dns_tool", "apply", srv.Name)
		writeJSON(w, map[string]any{"current": orEmpty(currentDNS()), "backup": dnsHasBackup()})
	})
	mux.HandleFunc("POST /api/tools/dns/restore", func(w http.ResponseWriter, r *http.Request) {
		if !s.e.admin {
			errJSON(w, 403, errNeedAdmin.Error())
			return
		}
		if err := restoreDNS(); err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		s.e.logHist("dns_tool", "revert", "")
		writeJSON(w, map[string]any{"current": orEmpty(currentDNS()), "backup": dnsHasBackup()})
	})

	mux.HandleFunc("GET /api/sessions", func(w http.ResponseWriter, r *http.Request) {
		ov := s.e.store.GetSettings().Overlay
		writeJSON(w, map[string]any{"sessions": orEmpty(loadSessions(localDir())), "recording": ov.Enabled && !ov.RecordOff})
	})
	mux.HandleFunc("POST /api/sessions/delete", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"id"`
		}
		if err := readJSON(r, &req); err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		writeJSON(w, map[string]any{"sessions": orEmpty(deleteSession(localDir(), req.ID))})
	})

	mux.HandleFunc("GET /api/startup", func(w http.ResponseWriter, r *http.Request) {
		items, err := startupItems()
		if err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"items": s.withIcons(items)})
	})
	mux.HandleFunc("POST /api/startup", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"id"`
			On bool   `json:"on"`
		}
		if err := readJSON(r, &req); err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		if !s.e.admin {
			errJSON(w, 403, errNeedAdmin.Error())
			return
		}
		if err := setStartup(req.ID, req.On); err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		items, _ := startupItems()
		writeJSON(w, map[string]any{"items": s.withIcons(items)})
	})
}

func orEmpty[T any](v []T) []T {
	if v == nil {
		return []T{}
	}
	return v
}
