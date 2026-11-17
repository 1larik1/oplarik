package main

import (
	"crypto/rand"
	"embed"
	"encoding/hex"
	"flag"
	"io/fs"
	"log"
	"net"
	"path/filepath"
	"strconv"
)

//go:embed web
var webFS embed.FS

//go:embed catalog.json
var catalogJSON []byte

const appVersion = "1.1.3"

// 1.1.3: the program is called OpLarik. Data folders, task names and other
// internal identifiers keep the old "Larik" so settings and undo backups of
// earlier versions stay valid.
const (
	appName           = "OpLarik"
	exeName           = "OpLarik.exe"
	legacyExeName     = "Larik.exe"
	defaultUpdateRepo = "1larik1/oplarik" // public GitHub repository with the releases
)

type Options struct {
	Engine    bool
	Port      int
	Token     string
	Uninstall bool
	Demo      bool
	Stay      bool
	NoWindow  bool
	Overlay   bool
	FPSMon    bool
	Tray      bool
	Clean     bool
}

func main() {
	var o Options
	flag.BoolVar(&o.Engine, "engine", false, "run the elevated engine (internal)")
	flag.IntVar(&o.Port, "port", 0, "engine port (internal)")
	flag.StringVar(&o.Token, "token", "", "session token (internal)")
	flag.BoolVar(&o.Uninstall, "uninstall", false, "uninstall OpLarik")
	flag.BoolVar(&o.Demo, "demo", false, "simulated system (testing)")
	flag.BoolVar(&o.Stay, "stay", false, "keep running when the window closes")
	flag.BoolVar(&o.NoWindow, "no-window", false, "do not open a window")
	flag.BoolVar(&o.Overlay, "overlay", false, "run the in-game overlay (internal)")
	flag.BoolVar(&o.FPSMon, "fpsmon", false, "run the FPS tracer (internal)")
	flag.BoolVar(&o.Tray, "tray", false, "run the tray icon (internal)")
	flag.BoolVar(&o.Clean, "clean", false, "remove temporary files (scheduled cleanup)")
	flag.Parse()
	platformMain(o)
}

func randToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func appURL(port int, token string) string {
	return "http://127.0.0.1:" + strconv.Itoa(port) + "/#t=" + token
}

// newServer builds the engine + HTTP server and binds the port.
func newServer(sys Sys, admin bool, o Options) (*Server, net.Listener) {
	cat, err := LoadCatalog(catalogJSON)
	if err != nil {
		log.Fatal("catalog: ", err)
	}
	st := OpenStore(dataDir())
	migrateBoost(st)
	e := NewEngine(sys, cat, st, admin)
	sub, _ := fs.Sub(webFS, "web")
	if o.Token == "" {
		o.Token = randToken()
	}
	s := &Server{e: e, token: o.Token, port: o.Port, demo: o.Demo, stay: o.Stay, web: sub,
		hub: NewHub(), live: newLiveSampler(), quit: make(chan struct{})}
	s.auth = OpenAuth(filepath.Join(dataDir(), "account"))
	s.auth.admin = admin
	s.auth.onChange = func() { s.hub.Send("auth", s.auth.Status()) }
	ln, err := s.Listen()
	if err != nil {
		log.Fatal("listen: ", err)
	}
	s.port = ln.Addr().(*net.TCPAddr).Port
	go s.scan()
	go ensureOverlay(st.GetSettings())
	go ensureTray(st.GetSettings())
	return s, ln
}
