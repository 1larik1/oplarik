package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Server struct {
	e         *Engine
	token     string
	port      int
	demo      bool
	stay      bool
	web       fs.FS
	hub       *Hub
	live      LiveSampler
	jobs      sync.Map
	jobSeq    atomic.Int64
	running   atomic.Int32
	scanning  sync.Mutex
	quit      chan struct{}
	onElevate func() (string, error)
	auth      *Auth
}

// ---------------------------------------------------------------- SSE hub

type Hub struct {
	mu      sync.Mutex
	clients map[chan string]bool
	last    time.Time
	ever    bool
}

func NewHub() *Hub { return &Hub{clients: map[chan string]bool{}, last: time.Now()} }

func (h *Hub) Add() chan string {
	h.mu.Lock()
	defer h.mu.Unlock()
	c := make(chan string, 64)
	h.clients[c] = true
	h.ever = true
	return c
}

func (h *Hub) Remove(c chan string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, c)
	h.last = time.Now()
}

func (h *Hub) Count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

func (h *Hub) Send(event string, data any) {
	b, _ := json.Marshal(data)
	msg := "event: " + event + "\ndata: " + string(b) + "\n\n"
	h.mu.Lock()
	defer h.mu.Unlock()
	for c := range h.clients {
		select {
		case c <- msg:
		default:
		}
	}
}

// ---------------------------------------------------------------- jobs

type Job struct {
	mu sync.Mutex
	JobView
}

type JobView struct {
	ID       string       `json:"id"`
	Kind     string       `json:"kind"`
	Total    int          `json:"total"`
	Done     int          `json:"done"`
	Current  string       `json:"current,omitempty"`
	Finished bool         `json:"finished"`
	Results  []ItemResult `json:"results,omitempty"`
	Error    string       `json:"error,omitempty"`
	Data     any          `json:"data,omitempty"`
}

func (s *Server) startJob(kind string, total int, fn func(j *Job)) *Job {
	j := &Job{JobView: JobView{ID: "j" + strconv.FormatInt(s.jobSeq.Add(1), 10), Kind: kind, Total: total}}
	s.jobs.Store(j.ID, j)
	s.running.Add(1)
	go func() {
		defer s.running.Add(-1)
		defer func() {
			if r := recover(); r != nil {
				j.mu.Lock()
				j.Error = fmt.Sprint("internal error: ", r)
				j.mu.Unlock()
			}
			j.mu.Lock()
			j.Finished = true
			j.Done = j.Total
			snap := j.JobView
			j.mu.Unlock()
			s.hub.Send("job", &snap)
			s.pushStates(false)
		}()
		j.mu.Lock()
		first := j.JobView
		j.mu.Unlock()
		s.hub.Send("job", &first)
		fn(j)
	}()
	return j
}

func (j *Job) progress(s *Server, done int, cur string) {
	j.mu.Lock()
	j.Done, j.Current = done, cur
	snap := j.JobView
	j.mu.Unlock()
	s.hub.Send("job", &snap)
}

func (s *Server) pushStates(force bool) {
	states := s.e.States(force)
	score, a, t := s.e.Score(states)
	s.hub.Send("states", map[string]any{"states": states, "score": score, "active": a, "total": t})
}

// ---------------------------------------------------------------- http

func (s *Server) Listen() (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(s.port))
}

func (s *Server) Serve(ln net.Listener) error {
	s.port = ln.Addr().(*net.TCPAddr).Port
	mux := http.NewServeMux()
	s.routes(mux)
	s.authRoutes(mux)
	srv := &http.Server{Handler: s.guard(mux), ReadHeaderTimeout: 10 * time.Second}
	go s.liveLoop()
	go s.lifecycle(srv)
	err := srv.Serve(ln)
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}

// guard enforces loopback Host (anti DNS-rebinding) and the session token.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		okHost := host == "127.0.0.1:"+strconv.Itoa(s.port) || host == "localhost:"+strconv.Itoa(s.port)
		if !okHost {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/health" {
			tok := r.Header.Get("X-Larik-Token")
			if tok == "" {
				tok = r.URL.Query().Get("t")
			}
			if subtle.ConstantTimeCompare([]byte(tok), []byte(s.token)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if r.Method == http.MethodPost {
				if o := r.Header.Get("Origin"); o != "" && o != "http://"+host {
					http.Error(w, "bad origin", http.StatusForbidden)
					return
				}
			}
			if s.auth != nil && s.auth.Locked() && !authOpenPath(r.Method, r.URL.Path) {
				w.WriteHeader(http.StatusLocked)
				writeJSON(w, map[string]string{"error": "locked"})
				return
			}
		} else {
			w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'")
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20)).Decode(v)
}

func jsonDecodeLimit(r *http.Request, v any, limit int64) error {
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, limit)).Decode(v)
}

func errJSON(w http.ResponseWriter, code int, msg string) {
	w.WriteHeader(code)
	writeJSON(w, map[string]string{"error": msg})
}

func (s *Server) routes(mux *http.ServeMux) {
	static := http.FileServer(http.FS(s.web))
	// Explicit types: Go's mime package reads the Windows registry, where some
	// PCs map .js to text/plain, which would break ES module loading.
	types := map[string]string{".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8",
		".html": "text/html; charset=utf-8", ".svg": "image/svg+xml", ".png": "image/png", ".woff": "font/woff",
		".txt": "text/plain; charset=utf-8"}
	mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if strings.HasSuffix(p, ".html") || p == "/" {
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Type", types[".html"])
		} else if i := strings.LastIndex(p, "."); i >= 0 {
			if ct, ok := types[strings.ToLower(p[i:])]; ok {
				w.Header().Set("Content-Type", ct)
			}
		}
		static.ServeHTTP(w, r)
	}))

	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "app": "larik"})
	})

	mux.HandleFunc("GET /api/bootstrap", func(w http.ResponseWriter, r *http.Request) {
		type tw struct {
			*Tweak
			Ops []map[string]any `json:"ops"`
		}
		var list []tw
		for _, t := range s.e.cat.Tweaks {
			var ops []map[string]any
			for i := range t.Ops {
				ops = append(ops, describeOp(&t.Ops[i]))
			}
			list = append(list, tw{Tweak: t, Ops: ops})
		}
		var saved []string
		s.e.store.Read(func(d *StoreData) { saved = append(saved, d.Saved...) })
		writeJSON(w, map[string]any{
			"version":  appVersion,
			"admin":    s.e.admin,
			"demo":     s.demo,
			"settings": s.e.store.GetSettings(),
			"tweaks":   list,
			"presets":  s.e.cat.Presets,
			"saved":    saved,
			"install":  installStatus(),
			"sysLang":  systemLang(),
			"dataDir":  s.e.store.dir,
		})
	})

	mux.HandleFunc("GET /api/states", func(w http.ResponseWriter, r *http.Request) {
		states := s.e.States(r.URL.Query().Get("force") == "1")
		score, a, t := s.e.Score(states)
		writeJSON(w, map[string]any{"states": states, "score": score, "active": a, "total": t})
	})

	mux.HandleFunc("GET /api/system", func(w http.ResponseWriter, r *http.Request) {
		info := s.e.Info()
		if info == nil || r.URL.Query().Get("rescan") == "1" {
			info = s.scan()
		}
		writeJSON(w, info)
	})

	mux.HandleFunc("POST /api/apply", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IDs []string `json:"ids"`
		}
		if err := readJSON(r, &req); err != nil || len(req.IDs) == 0 {
			errJSON(w, 400, "ids required")
			return
		}
		j := s.startJob("apply", len(req.IDs), func(j *Job) {
			res := s.e.Apply(req.IDs, func(d int, id string) { j.progress(s, d, id) })
			j.mu.Lock()
			j.Results = res
			j.mu.Unlock()
		})
		writeJSON(w, map[string]string{"job": j.ID})
	})

	mux.HandleFunc("POST /api/revert", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			IDs []string `json:"ids"`
			All bool     `json:"all"`
		}
		if err := readJSON(r, &req); err != nil {
			errJSON(w, 400, "bad request")
			return
		}
		ids := req.IDs
		if req.All {
			ids = nil
			s.e.store.Read(func(d *StoreData) {
				for id := range d.Applied {
					ids = append(ids, id)
				}
			})
		}
		if len(ids) == 0 {
			errJSON(w, 400, "nothing to revert")
			return
		}
		j := s.startJob("revert", len(ids), func(j *Job) {
			res := s.e.Revert(ids, func(d int, id string) { j.progress(s, d, id) })
			j.mu.Lock()
			j.Results = res
			j.mu.Unlock()
		})
		writeJSON(w, map[string]string{"job": j.ID})
	})

	mux.HandleFunc("GET /api/preset", func(w http.ResponseWriter, r *http.Request) {
		states := s.e.States(false)
		writeJSON(w, map[string]any{"ids": s.e.PresetIDs(r.URL.Query().Get("id"), states)})
	})

	mux.HandleFunc("POST /api/debloat/scan", func(w http.ResponseWriter, r *http.Request) {
		j := s.startJob("debloat-scan", 1, func(j *Job) {
			if err := s.e.ScanDebloat(); err != nil {
				j.mu.Lock()
				j.Error = err.Error()
				j.mu.Unlock()
			}
		})
		writeJSON(w, map[string]string{"job": j.ID})
	})

	mux.HandleFunc("GET /api/debloat", func(w http.ResponseWriter, r *http.Request) {
		d := s.e.Debloat()
		if d == nil {
			writeJSON(w, map[string]any{"scanned": false})
			return
		}
		writeJSON(w, map[string]any{"scanned": true, "at": d.At, "appx": d.Appx})
	})

	mux.HandleFunc("POST /api/restore-point", func(w http.ResponseWriter, r *http.Request) {
		j := s.startJob("restore-point", 1, func(j *Job) {
			if err := s.e.CreateRestorePoint(); err != nil {
				j.mu.Lock()
				j.Error = err.Error()
				j.mu.Unlock()
			}
		})
		writeJSON(w, map[string]string{"job": j.ID})
	})

	mux.HandleFunc("GET /api/backups", func(w http.ResponseWriter, r *http.Request) {
		var out struct {
			Applied       map[string]string `json:"applied"`
			History       []HistItem        `json:"history"`
			RestorePoints []string          `json:"restorePoints"`
		}
		out.Applied = map[string]string{}
		out.History = []HistItem{}
		out.RestorePoints = []string{}
		s.e.store.Read(func(d *StoreData) {
			for id, a := range d.Applied {
				out.Applied[id] = a.At
			}
			out.History = append(out.History, d.History...)
			out.RestorePoints = append(out.RestorePoints, d.RestorePoints...)
		})
		writeJSON(w, out)
	})

	mux.HandleFunc("GET /api/settings", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.e.store.GetSettings())
	})
	mux.HandleFunc("POST /api/settings", func(w http.ResponseWriter, r *http.Request) {
		st := s.e.store.GetSettings()
		if err := readJSON(r, &st); err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		st, warn, err := s.saveSettings(st)
		if err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"settings": st, "warn": warn})
	})

	mux.HandleFunc("POST /api/saved", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID string `json:"id"`
			On bool   `json:"on"`
		}
		if err := readJSON(r, &req); err != nil || s.e.cat.Get(req.ID) == nil {
			errJSON(w, 400, "bad id")
			return
		}
		var saved []string
		s.e.store.Update(func(d *StoreData) {
			var nl []string
			for _, x := range d.Saved {
				if x != req.ID {
					nl = append(nl, x)
				}
			}
			if req.On {
				nl = append(nl, req.ID)
			}
			d.Saved = nl
			saved = append(saved, nl...)
		})
		writeJSON(w, map[string]any{"saved": saved})
	})

	s.gameRoutes(mux)
	s.boostRoutes(mux)
	s.toolRoutes(mux)
	s.compareRoutes(mux)
	s.nvidiaRoutes(mux)
	s.healthRoutes(mux)
	s.netRoutes(mux)
	s.cleanupRoutes(mux)
	s.updateRoutes(mux)
	s.ioRoutes(mux)
	s.coverRoutes(mux)
	s.launchRoutes(mux)

	mux.HandleFunc("POST /api/power", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Action string `json:"action"`
		}
		_ = readJSON(r, &req)
		var err error
		switch req.Action {
		case "restart":
			err = rebootPC()
		case "signout":
			err = signOut()
		case "explorer":
			err = restartExplorer()
		default:
			err = fmt.Errorf("unknown action")
		}
		if err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})

	mux.HandleFunc("POST /api/install", func(w http.ResponseWriter, r *http.Request) {
		if err := installApp(); err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		cur := s.e.store.GetSettings()
		if (cur.Overlay.Enabled && cur.Overlay.Autostart) || cur.AutoGame.Enabled {
			_ = syncAutostart(cur) // point the logon task at the installed copy
		}
		if cur.AutoClean.Enabled {
			_ = syncCleanTask(cur.AutoClean)
		}
		_ = syncTrayAutostart(cur) // 1.1.3: tasks of an old "Larik" install pointed at the old folder
		writeJSON(w, installStatus())
	})

	mux.HandleFunc("POST /api/open", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			What string `json:"what"`
			ID   string `json:"id"`
		}
		_ = readJSON(r, &req)
		switch req.What {
		case "data":
			openPath(s.e.store.dir)
		case "sessions":
			dir := filepath.Join(localDir(), "Sessions")
			_ = os.MkdirAll(dir, 0o755)
			openPath(dir)
		case "session":
			// only files Larik recorded itself, looked up by id
			for _, ss := range loadSessions(localDir()) {
				if ss.ID == req.ID {
					if p, ok := sessionCSV(localDir(), ss.ID); ok {
						if _, err := os.Stat(p); err == nil {
							openPath(p)
						}
					}
				}
			}
		case "restore":
			openRestoreUI()
		case "store":
			s.e.sys.OpenURL("ms-windows-store://home")
		case "drivers":
			s.e.sys.OpenURL("https://www.nvidia.com/Download/index.aspx")
		case "amd":
			s.e.sys.OpenURL("https://www.amd.com/en/support/download/drivers.html")
		case "intel":
			s.e.sys.OpenURL("https://www.intel.com/content/www/us/en/download-center/home.html")
		case "display":
			s.e.sys.OpenURL("ms-settings:display-advanced")
		}
		writeJSON(w, map[string]bool{"ok": true})
	})

	mux.HandleFunc("POST /api/elevate", func(w http.ResponseWriter, r *http.Request) {
		if s.onElevate == nil {
			errJSON(w, 400, "not available")
			return
		}
		url, err := s.onElevate()
		if err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]string{"url": url})
		go func() {
			time.Sleep(1500 * time.Millisecond)
			s.shutdown()
		}()
	})

	mux.HandleFunc("GET /api/overlay", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, overlayStatus())
	})
	mux.HandleFunc("POST /api/overlay/visible", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Visible bool `json:"visible"`
		}
		_ = readJSON(r, &req)
		if err := writeOverlayVisible(req.Visible); err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("GET /api/overlay/preview.png", func(w http.ResponseWriter, r *http.Request) {
		st := s.e.store.GetSettings()
		cfg := st.Overlay
		cfg.Theme = st.Theme
		if len(cfg.Metrics) == 0 {
			cfg.Metrics = nil
			cfg.Metrics = []string{}
		}
		lang := st.Lang
		if lang == "" {
			lang = systemLang()
		}
		c, err := RenderOverlay(cfg, PreviewData(s.live.Sample(), sampleSensors()), lang, 1)
		if err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(encodePNG(c))
	})

	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("POST /api/bye", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]bool{"ok": true})
	})
}

func (s *Server) scan() *SysInfo {
	s.scanning.Lock()
	defer s.scanning.Unlock()
	info, err := scanSystem(s.e.sys)
	if info == nil {
		info = &SysInfo{}
	}
	if err != nil {
		info.ScanErr = err.Error()
	}
	info.ScanAt = nowStr()
	info.Analyze()
	s.e.SetInfo(info)
	return info
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "no streaming", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Connection", "keep-alive")
	c := s.hub.Add()
	defer s.hub.Remove(c)
	fmt.Fprint(w, "retry: 1000\n\n")
	fl.Flush()
	ping := time.NewTicker(15 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-s.quit:
			return
		case msg := <-c:
			if s.auth != nil && s.auth.Locked() && !strings.HasPrefix(msg, "event: auth\n") {
				continue
			}
			if _, err := fmt.Fprint(w, msg); err != nil {
				return
			}
			fl.Flush()
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil {
				return
			}
			fl.Flush()
		}
	}
}

func (s *Server) liveLoop() {
	t := time.NewTicker(1500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case <-s.quit:
			return
		case <-t.C:
			if s.hub.Count() == 0 {
				continue
			}
			func() {
				defer func() { recover() }()
				l := s.live.Sample()
				sens := sampleSensors()
				l.GPUTemp, l.CPUTemp = sens.GPUTemp, sens.CPUTemp
				if sens.GPUUtil >= 0 {
					l.GPU = sens.GPUUtil
				}
				s.hub.Send("live", l)
			}()
		}
	}
}

// lifecycle exits the engine once the UI window is gone.
func (s *Server) lifecycle(srv *http.Server) {
	start := time.Now()
	idleSince, noUI := time.Time{}, time.Time{}
	for {
		select {
		case <-s.quit:
			_ = srv.Close()
			return
		case <-time.After(time.Second):
		}
		// Lock the account once every Larik window is closed.
		if s.hub.Count() == 0 {
			if noUI.IsZero() {
				noUI = time.Now()
			} else if time.Since(noUI) > 4*time.Second && s.auth != nil && !s.auth.Locked() {
				s.auth.Lock()
			}
		} else {
			noUI = time.Time{}
		}
		if s.stay {
			continue
		}
		s.hub.mu.Lock()
		ever := s.hub.ever
		s.hub.mu.Unlock()
		n := s.hub.Count()
		if !ever {
			if time.Since(start) > 3*time.Minute {
				log.Print("no UI connected; exiting")
				s.shutdown()
			}
			continue
		}
		if n > 0 || s.running.Load() > 0 {
			idleSince = time.Time{}
			continue
		}
		if idleSince.IsZero() {
			idleSince = time.Now()
		}
		if time.Since(idleSince) > 12*time.Second {
			log.Print("UI closed; exiting")
			s.shutdown()
		}
	}
}

var quitOnce sync.Once

func (s *Server) shutdown() {
	quitOnce.Do(func() {
		close(s.quit)
		removeInstanceFile(s.token)
		go func() {
			time.Sleep(800 * time.Millisecond)
			os.Exit(0)
		}()
	})
}

// normalizeSettings clamps every field to a valid value.
func normalizeSettings(st Settings) Settings {
	switch st.Lang {
	case "en", "ru", "uk", "pl", "es", "de", "fr", "ja", "cs", "":
	default:
		st.Lang = "en"
	}
	st.Motion = true // animations can no longer be switched off (1.1.1)
	st.Overlay.Normalize()
	if !validTheme(st.Theme) {
		st.Theme = "violet"
	}
	switch st.AutoLock {
	case 0, 5, 15, 30, 60:
	default:
		st.AutoLock = 0
	}
	st.AutoClean.Normalize()
	st.UpdateRepo = cleanRepo(st.UpdateRepo)
	if st.UpdateRepo == "" {
		st.UpdateRepo = defaultUpdateRepo
	}
	if len(st.InputMode) > 20 {
		st.InputMode = st.InputMode[:20]
	}
	return st
}

// saveSettings stores new settings and applies what changed: logon tasks for
// the overlay / automatic game mode and the tray, and the cleanup schedule.
func (s *Server) saveSettings(st Settings) (Settings, string, error) {
	st = normalizeSettings(st)
	prev := s.e.store.GetSettings()
	if err := s.e.store.SetSettings(st); err != nil {
		return prev, "", err
	}
	var warn string
	note := func(err error) {
		if err != nil && warn == "" {
			warn = err.Error()
		}
	}
	old := prev.Overlay
	if st.Overlay.Enabled != old.Enabled || st.Overlay.Autostart != old.Autostart || st.AutoGame.Enabled != prev.AutoGame.Enabled {
		note(syncAutostart(st))
	}
	if st.Overlay.Enabled && !old.Enabled {
		_ = writeOverlayVisible(true)
	}
	go ensureOverlay(st)
	if prev.Tray != st.Tray || prev.TrayAuto != st.TrayAuto {
		note(syncTrayAutostart(st))
	}
	go ensureTray(st)
	if prev.AutoGame.Enabled && !st.AutoGame.Enabled {
		go func() { _ = endAutoBoost(s.e.sys) }() // automatic mode off: end its Boost
	}
	if st.AutoClean != prev.AutoClean {
		note(syncCleanTask(st.AutoClean))
	}
	return st, warn, nil
}
