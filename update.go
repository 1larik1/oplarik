package main

// Updates from GitHub Releases: the release must contain OpLarik.exe and
// OpLarik.exe.sha256 (tools/package.sh creates both). Larik downloads the exe,
// checks the SHA-256, swaps it in next to the running copy and restarts.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var repoRe = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,100}/[A-Za-z0-9_.-]{1,100}$`)

// cleanRepo accepts "owner/repo" or a github.com URL; anything else is dropped.
func cleanRepo(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "github.com/")
	s = strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
	if !repoRe.MatchString(s) {
		return ""
	}
	return s
}

// semverNewer reports whether version a (like "v1.0.8") is newer than b.
func semverNewer(a, b string) bool {
	pa, pb := verParts(a), verParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func verParts(v string) [3]int {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+ "); i >= 0 {
		v = v[:i]
	}
	for i, p := range strings.SplitN(v, ".", 3) {
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

type ghRelease struct {
	Tag       string            `json:"tag"`
	Notes     string            `json:"notes"`
	Published string            `json:"published"`
	Assets    map[string]string `json:"-"` // name -> download URL
}

// ghLatest is replaced in demo mode.
var ghLatest = func(repo string) (*ghRelease, error) {
	c := http.Client{Timeout: 20 * time.Second}
	req, _ := http.NewRequest("GET", "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", appName+"/"+appVersion)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 404 {
		return nil, errors.New("no_release")
	}
	if resp.StatusCode != 200 {
		return nil, errors.New("GitHub: " + resp.Status)
	}
	var r struct {
		Tag       string `json:"tag_name"`
		Body      string `json:"body"`
		Published string `json:"published_at"`
		Assets    []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&r); err != nil {
		return nil, err
	}
	out := &ghRelease{Tag: r.Tag, Notes: r.Body, Published: r.Published, Assets: map[string]string{}}
	for _, a := range r.Assets {
		out.Assets[a.Name] = a.URL
	}
	return out, nil
}

func githubHost(u *url.URL) bool {
	h := strings.ToLower(u.Hostname())
	return u.Scheme == "https" && (h == "github.com" || strings.HasSuffix(h, ".githubusercontent.com"))
}

// ghDownload is replaced in demo mode. Only GitHub hosts over HTTPS are
// accepted, also for the redirect to GitHub's file storage.
var ghDownload = func(raw string, max int64) ([]byte, error) {
	u, err := url.Parse(raw)
	if err != nil || !githubHost(u) {
		return nil, errors.New("unexpected download host")
	}
	c := http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) > 5 || !githubHost(req.URL) {
			return errors.New("unexpected download host")
		}
		return nil
	}}
	url := u.String()
	req, _ := http.NewRequest("GET", url, nil)
	req.Header.Set("User-Agent", appName+"/"+appVersion)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, errors.New("download: " + resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > max {
		return nil, errors.New("file too large")
	}
	return b, nil
}

type UpdateInfo struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	Notes     string `json:"notes"`
	Published string `json:"published"`
	Available bool   `json:"available"`
	CanApply  bool   `json:"canApply"` // release has OpLarik.exe and OpLarik.exe.sha256
}

func checkUpdate(repo string) (*UpdateInfo, *ghRelease, error) {
	if repo == "" {
		return nil, nil, errors.New("no_repo")
	}
	rel, err := ghLatest(repo)
	if err != nil {
		return nil, nil, err
	}
	notes := rel.Notes
	if len(notes) > 4000 {
		notes = notes[:4000]
	}
	info := &UpdateInfo{Current: appVersion, Latest: strings.TrimPrefix(rel.Tag, "v"), Notes: notes, Published: rel.Published}
	info.Available = semverNewer(rel.Tag, appVersion)
	_, a := rel.Assets[exeName]
	_, b := rel.Assets[exeName+".sha256"]
	info.CanApply = a && b
	return info, rel, nil
}

// fetchVerified downloads OpLarik.exe and checks it against OpLarik.exe.sha256.
func fetchVerified(rel *ghRelease) ([]byte, error) {
	sumFile, err := ghDownload(rel.Assets[exeName+".sha256"], 4096)
	if err != nil {
		return nil, err
	}
	want := strings.ToLower(strings.Fields(string(sumFile) + " x")[0])
	if len(want) != 64 {
		return nil, errors.New("bad checksum file")
	}
	exe, err := ghDownload(rel.Assets[exeName], 64<<20)
	if err != nil {
		return nil, err
	}
	if sha256Hex(exe) != want {
		return nil, errors.New("checksum_mismatch")
	}
	if len(exe) < 2 || exe[0] != 'M' || exe[1] != 'Z' {
		return nil, errors.New("not a Windows program")
	}
	return exe, nil
}

func (s *Server) updateRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/update/check", func(w http.ResponseWriter, r *http.Request) {
		info, _, err := checkUpdate(s.e.store.GetSettings().UpdateRepo)
		if err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		writeJSON(w, info)
	})
	mux.HandleFunc("POST /api/update/install", func(w http.ResponseWriter, r *http.Request) {
		if !s.e.admin {
			errJSON(w, 403, errNeedAdmin.Error())
			return
		}
		info, rel, err := checkUpdate(s.e.store.GetSettings().UpdateRepo)
		if err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		if !info.Available || !info.CanApply {
			errJSON(w, 400, "nothing_to_install")
			return
		}
		exe, err := fetchVerified(rel)
		if err != nil {
			errJSON(w, 400, err.Error())
			return
		}
		if err := installUpdate(exe); err != nil {
			errJSON(w, 500, err.Error())
			return
		}
		writeJSON(w, map[string]any{"ok": true, "version": info.Latest})
		go func() {
			time.Sleep(1200 * time.Millisecond)
			if s.demo {
				return
			}
			removeInstanceFile(s.token) // the new copy must not reuse this engine
			restartAfterUpdate()
			s.shutdown()
		}()
	})
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
