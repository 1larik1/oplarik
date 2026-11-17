package main

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
)

// authOpenPath lists what the UI may call while the account is locked:
// the sign-in screen itself, nothing that reveals or changes the PC.
func authOpenPath(method, path string) bool {
	switch path {
	case "/api/auth", "/api/auth/avatar.png", "/api/events", "/api/health":
		return method == http.MethodGet
	case "/api/auth/login", "/api/auth/code", "/api/auth/recover", "/api/auth/setup", "/api/auth/lock", "/api/auth/lang", "/api/bye", "/api/elevate":
		return method == http.MethodPost
	}
	return false
}

func authErr(w http.ResponseWriter, err error) {
	code := 400
	switch {
	case errors.Is(err, errTooMany):
		code = 429
	case errors.Is(err, errBadPassword), errors.Is(err, errBadCode), errors.Is(err, errBadKey):
		code = 403
	case err.Error() == "locked":
		code = http.StatusLocked
	}
	errJSON(w, code, err.Error())
}

func (s *Server) authChanged() { s.hub.Send("auth", s.auth.Status()) }

func (s *Server) authRoutes(mux *http.ServeMux) {
	a := s.auth
	type pw struct {
		Name     string `json:"name"`
		Password string `json:"password"`
		Old      string `json:"old"`
		Ticket   string `json:"ticket"`
		Code     string `json:"code"`
		Key      string `json:"key"`
		Lang     string `json:"lang"`
	}
	read := func(w http.ResponseWriter, r *http.Request) (pw, bool) {
		var p pw
		if err := readJSON(r, &p); err != nil {
			errJSON(w, 400, "bad request")
			return p, false
		}
		return p, true
	}

	mux.HandleFunc("GET /api/auth", func(w http.ResponseWriter, r *http.Request) {
		st := s.e.store.GetSettings()
		writeJSON(w, map[string]any{"auth": a.Status(), "lang": st.Lang, "sysLang": systemLang(), "version": appVersion,
			"admin": s.e.admin, "demo": s.demo, "asked": st.AcctAsked, "motion": st.Motion, "theme": st.Theme})
	})
	mux.HandleFunc("POST /api/auth/lang", func(w http.ResponseWriter, r *http.Request) {
		p, ok := read(w, r)
		if !ok {
			return
		}
		switch p.Lang {
		case "en", "ru", "uk", "pl", "es", "de", "fr", "ja", "cs":
		default:
			errJSON(w, 400, "bad lang")
			return
		}
		st := s.e.store.GetSettings()
		st.Lang = p.Lang
		_ = s.e.store.SetSettings(st)
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/auth/setup", func(w http.ResponseWriter, r *http.Request) {
		p, ok := read(w, r)
		if !ok {
			return
		}
		key, err := a.Setup(p.Name, p.Password)
		if err != nil {
			authErr(w, err)
			return
		}
		s.markAsked()
		s.authChanged()
		writeJSON(w, map[string]any{"key": key})
	})
	mux.HandleFunc("POST /api/auth/skip", func(w http.ResponseWriter, r *http.Request) {
		s.markAsked()
		writeJSON(w, map[string]bool{"ok": true})
	})
	mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
		p, ok := read(w, r)
		if !ok {
			return
		}
		t, err := a.Login(p.Password)
		if err != nil {
			authErr(w, err)
			return
		}
		if t != "" {
			writeJSON(w, map[string]any{"ticket": t})
			return
		}
		s.authChanged()
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /api/auth/code", func(w http.ResponseWriter, r *http.Request) {
		p, ok := read(w, r)
		if !ok {
			return
		}
		if err := a.LoginCode(p.Ticket, p.Code); err != nil {
			authErr(w, err)
			return
		}
		s.authChanged()
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /api/auth/recover", func(w http.ResponseWriter, r *http.Request) {
		p, ok := read(w, r)
		if !ok {
			return
		}
		key, err := a.Recover(p.Key, p.Password)
		if err != nil {
			authErr(w, err)
			return
		}
		s.authChanged()
		writeJSON(w, map[string]any{"key": key})
	})
	mux.HandleFunc("POST /api/auth/lock", func(w http.ResponseWriter, r *http.Request) {
		a.Lock()
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /api/auth/name", func(w http.ResponseWriter, r *http.Request) {
		p, ok := read(w, r)
		if !ok {
			return
		}
		if err := a.Rename(p.Name); err != nil {
			authErr(w, err)
			return
		}
		s.authChanged()
		writeJSON(w, a.Status())
	})
	mux.HandleFunc("POST /api/auth/password", func(w http.ResponseWriter, r *http.Request) {
		p, ok := read(w, r)
		if !ok {
			return
		}
		if err := a.ChangePassword(p.Old, p.Password); err != nil {
			authErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("POST /api/auth/recovery-key", func(w http.ResponseWriter, r *http.Request) {
		p, ok := read(w, r)
		if !ok {
			return
		}
		key, err := a.NewRecoveryKey(p.Password)
		if err != nil {
			authErr(w, err)
			return
		}
		writeJSON(w, map[string]any{"key": key})
	})
	mux.HandleFunc("POST /api/auth/totp/begin", func(w http.ResponseWriter, r *http.Request) {
		secret, uri, err := a.TOTPBegin()
		if err != nil {
			authErr(w, err)
			return
		}
		png, err := QRPNG(uri, 6)
		if err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		// Grouped for typing by hand: ABCD EFGH ...
		var g []string
		for i := 0; i < len(secret); i += 4 {
			g = append(g, secret[i:min(i+4, len(secret))])
		}
		writeJSON(w, map[string]any{"secret": strings.Join(g, " "), "qr": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)})
	})
	mux.HandleFunc("POST /api/auth/totp/confirm", func(w http.ResponseWriter, r *http.Request) {
		p, ok := read(w, r)
		if !ok {
			return
		}
		if err := a.TOTPConfirm(p.Password, p.Code); err != nil {
			authErr(w, err)
			return
		}
		s.authChanged()
		writeJSON(w, a.Status())
	})
	mux.HandleFunc("POST /api/auth/totp/disable", func(w http.ResponseWriter, r *http.Request) {
		p, ok := read(w, r)
		if !ok {
			return
		}
		if err := a.TOTPDisable(p.Password); err != nil {
			authErr(w, err)
			return
		}
		s.authChanged()
		writeJSON(w, a.Status())
	})
	mux.HandleFunc("POST /api/auth/remove", func(w http.ResponseWriter, r *http.Request) {
		p, ok := read(w, r)
		if !ok {
			return
		}
		if err := a.Remove(p.Password); err != nil {
			authErr(w, err)
			return
		}
		s.authChanged()
		writeJSON(w, a.Status())
	})
	mux.HandleFunc("POST /api/auth/avatar", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Data string `json:"data"`
		}
		defer r.Body.Close()
		if err := jsonDecodeLimit(r, &req, 12<<20); err != nil {
			errJSON(w, 400, "bad request")
			return
		}
		raw := req.Data
		if i := strings.Index(raw, ","); strings.HasPrefix(raw, "data:") && i > 0 {
			raw = raw[i+1:]
		}
		b, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			authErr(w, errBadImage)
			return
		}
		if err := a.SetAvatar(b); err != nil {
			authErr(w, err)
			return
		}
		s.authChanged()
		writeJSON(w, a.Status())
	})
	mux.HandleFunc("POST /api/auth/avatar/clear", func(w http.ResponseWriter, r *http.Request) {
		if err := a.ClearAvatar(); err != nil {
			authErr(w, err)
			return
		}
		s.authChanged()
		writeJSON(w, a.Status())
	})
	mux.HandleFunc("GET /api/auth/avatar.png", func(w http.ResponseWriter, r *http.Request) {
		b := a.AvatarPNG()
		if b == nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Header().Set("Cache-Control", "private, max-age=31536000") // URL carries the version
		_, _ = w.Write(b)
	})
}

func (s *Server) markAsked() {
	st := s.e.store.GetSettings()
	if !st.AcctAsked {
		st.AcctAsked = true
		_ = s.e.store.SetSettings(st)
	}
}
