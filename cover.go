package main

// Game and program pictures (1.0.8), all from this PC, nothing downloaded:
// Steam keeps every game's library artwork in Steam\appcache\librarycache;
// for other games and startup programs Larik shows the icon inside the .exe.
// The UI only gets opaque /api/cover/<id> links; the server keeps the
// id → file mapping, so the page can never ask for an arbitrary file.

import (
	"crypto/sha1"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

type coverSrc struct {
	steam string // Steam app id
	exe   string // program path
}

var coverReg = struct {
	sync.Mutex
	src   map[string]coverSrc
	icons map[string][]byte // extracted .exe icons (PNG)
	idx   map[string]string // lower(Steam install dir) -> app id
	idxAt time.Time
}{src: map[string]coverSrc{}, icons: map[string][]byte{}}

func digitsOnly(s string) string {
	for _, c := range s {
		if c < '0' || c > '9' {
			return ""
		}
	}
	if len(s) > 12 {
		return ""
	}
	return s
}

// steamCoverFile finds the library artwork of a Steam game: the tall
// "capsule" first, the wide header as a fallback. Steam has used both a flat
// layout (<id>_library_600x900.jpg) and per-game folders (<id>/…).
func steamCoverFile(root, id string) string {
	if root == "" || digitsOnly(id) == "" || id == "" {
		return ""
	}
	dir := filepath.Join(root, "appcache", "librarycache")
	for _, name := range []string{"library_600x900_2x.jpg", "library_600x900.jpg", "header.jpg", "library_header.jpg"} {
		cands := []string{filepath.Join(dir, id+"_"+name), filepath.Join(dir, id, name)}
		if m, _ := filepath.Glob(filepath.Join(dir, id, "*", name)); len(m) > 0 {
			cands = append(cands, m...)
		}
		for _, c := range cands {
			if st, err := os.Stat(c); err == nil && st.Mode().IsRegular() && st.Size() > 0 && st.Size() < 8<<20 {
				return c
			}
		}
	}
	return ""
}

var reSteamCommon = regexp.MustCompile(`(?i)[\\/]steamapps[\\/]common[\\/]([^\\/]+)`)

// steamIDForPath finds the Steam app id of a game added before 1.0.8 (or by
// hand) from its folder under steamapps\common.
func (s *Server) steamIDForPath(path string) string {
	m := reSteamCommon.FindStringSubmatch(path)
	if m == nil {
		return ""
	}
	coverReg.Lock()
	idx, fresh := coverReg.idx, coverReg.idx != nil && time.Since(coverReg.idxAt) < time.Minute
	coverReg.Unlock()
	if !fresh {
		// Read the manifests without holding the lock (slow on hard drives).
		idx = map[string]string{}
		root := steamRoot(s.e.sys)
		libs := []string{root}
		if b, err := os.ReadFile(filepath.Join(root, "steamapps", "libraryfolders.vdf")); err == nil && root != "" {
			for _, lm := range reVdfPath.FindAllStringSubmatch(string(b), -1) {
				libs = append(libs, winPath(strings.ReplaceAll(lm[1], `\\`, `\`)))
			}
		}
		for _, lib := range libs {
			if lib == "" {
				continue
			}
			files, _ := filepath.Glob(filepath.Join(lib, "steamapps", "appmanifest_*.acf"))
			for _, f := range files {
				if b, err := os.ReadFile(f); err == nil {
					if dir, id := first(reAcfDir, string(b)), first(reAcfID, string(b)); dir != "" && id != "" {
						idx[strings.ToLower(dir)] = id
					}
				}
			}
		}
		coverReg.Lock()
		coverReg.idx, coverReg.idxAt = idx, time.Now()
		coverReg.Unlock()
	}
	return idx[strings.ToLower(m[1])]
}

var reLocalPath = regexp.MustCompile(`^[A-Za-z]:\\[^/:*?"<>|]+$`)

// safeExePath: absolute paths on a local drive only. Network shares (\\srv,
// \/srv, \??\UNC, mapped network drives) are refused before anything
// touches the file: opening one would make Windows send the user's
// credentials to that server. Relative paths are refused too.
func safeExePath(p string) bool {
	if len(p) >= 400 || !strings.HasSuffix(strings.ToLower(p), ".exe") || !reLocalPath.MatchString(p) {
		return false
	}
	return driveLocal(p[:2]) && coverExeExists(p)
}

// coverURL registers a picture for a game or program and returns its link,
// or "" when there is nothing better than the letter tile.
func (s *Server) coverURL(steamID, path string) string {
	var id string
	var src coverSrc
	if steamID = digitsOnly(steamID); steamID != "" && steamCoverFile(steamRoot(s.e.sys), steamID) != "" {
		id, src = "s"+steamID, coverSrc{steam: steamID}
	} else if safeExePath(path) {
		h := sha1.Sum([]byte(strings.ToLower(filepath.Clean(path))))
		id, src = "x"+hex.EncodeToString(h[:8]), coverSrc{exe: path}
	} else {
		return ""
	}
	coverReg.Lock()
	if len(coverReg.src) > 2000 {
		coverReg.src = map[string]coverSrc{}
	}
	coverReg.src[id] = src
	coverReg.Unlock()
	return "/api/cover/" + id
}

func (s *Server) coverRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/cover/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		coverReg.Lock()
		src, ok := coverReg.src[id]
		icon, tried := coverReg.icons[id]
		coverReg.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "private, max-age=86400")
		if src.steam != "" {
			f := steamCoverFile(steamRoot(s.e.sys), src.steam)
			fh, err := os.Open(f)
			if f == "" || err != nil {
				http.NotFound(w, r)
				return
			}
			defer fh.Close()
			st, err := fh.Stat()
			if err != nil {
				http.NotFound(w, r)
				return
			}
			ct := "image/jpeg"
			if strings.HasSuffix(strings.ToLower(f), ".png") {
				ct = "image/png"
			}
			w.Header().Set("Content-Type", ct)
			http.ServeContent(w, r, "", st.ModTime(), fh)
			return
		}
		if !tried {
			b, err := exeIcon(src.exe)
			if err != nil {
				b = []byte{} // remember "no icon" so it isn't extracted again
			}
			icon = b
			coverReg.Lock()
			if len(coverReg.icons) > 500 {
				coverReg.icons = map[string][]byte{}
			}
			coverReg.icons[id] = icon
			coverReg.Unlock()
		}
		if len(icon) == 0 {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(icon)
	})
}
