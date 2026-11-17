package main

// Network test (1.0.7): ping, jitter and packet loss to your router and two
// public servers, then download / upload speed with the ping measured at the
// same time. The extra delay under load is "bufferbloat", the usual reason a
// game lags while someone else streams or downloads. Speed traffic goes to
// Cloudflare's public speed test (speed.cloudflare.com).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

type PingStats struct {
	ID     string  `json:"id"` // router | cloudflare | google
	Target string  `json:"target"`
	Sent   int     `json:"sent"`
	Lost   int     `json:"lost"`
	Avg    float64 `json:"avg"` // ms, -1 when every ping was lost
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Jitter float64 `json:"jitter"`
}

type NetTest struct {
	Running  bool        `json:"running"`
	Stage    string      `json:"stage"` // ping | download | upload | done
	Progress float64     `json:"progress"`
	At       string      `json:"at"`
	Error    string      `json:"error,omitempty"`
	Pings    []PingStats `json:"pings"`
	Idle     float64     `json:"idle"` // ms to 1.1.1.1 before the load
	Down     float64     `json:"down"` // Mbit/s, -1 = failed
	Up       float64     `json:"up"`
	LoadDown float64     `json:"loadDown"` // ms during download
	LoadUp   float64     `json:"loadUp"`
	Bloat    float64     `json:"bloat"` // extra ms under load
	Grade    string      `json:"grade"` // A+ … F, "" = unknown
}

const (
	pingCount   = 30
	pingGap     = 300 * time.Millisecond
	loadTarget  = "1.1.1.1"
	downTime    = 8 * time.Second
	upTime      = 6 * time.Second
	speedStream = 4
)

// Platform hooks (replaced by the demo).
var (
	netPing    = func(ip string, timeout time.Duration) float64 { return pingOnce(ip, timeout) }
	netGateway = func() string { return defaultGateway() }
	netDown    = httpDownload
	netUp      = httpUpload
)

func pingStats(id, target string, rtts []float64) PingStats {
	p := PingStats{ID: id, Target: target, Sent: len(rtts), Avg: -1, Min: -1, Max: -1, Jitter: -1}
	var ok []float64
	for _, v := range rtts {
		if v < 0 {
			p.Lost++
			continue
		}
		ok = append(ok, v)
	}
	if len(ok) == 0 {
		return p
	}
	sum, mn, mx := 0.0, ok[0], ok[0]
	for _, v := range ok {
		sum += v
		mn, mx = math.Min(mn, v), math.Max(mx, v)
	}
	p.Avg, p.Min, p.Max = round1(sum/float64(len(ok))), mn, mx
	if len(ok) > 1 {
		j := 0.0
		for i := 1; i < len(ok); i++ {
			j += math.Abs(ok[i] - ok[i-1])
		}
		p.Jitter = round1(j / float64(len(ok)-1))
	} else {
		p.Jitter = 0
	}
	return p
}

func median(v []float64) float64 {
	var ok []float64
	for _, x := range v {
		if x >= 0 {
			ok = append(ok, x)
		}
	}
	if len(ok) == 0 {
		return -1
	}
	sort.Float64s(ok)
	return ok[len(ok)/2]
}

// bloatGrade uses the common bufferbloat scale (extra latency under load).
func bloatGrade(extra float64) string {
	switch {
	case extra < 0:
		return ""
	case extra < 5:
		return "A+"
	case extra < 30:
		return "A"
	case extra < 60:
		return "B"
	case extra < 200:
		return "C"
	case extra < 400:
		return "D"
	}
	return "F"
}

type countWriter struct{ n *atomic.Int64 }

func (c countWriter) Write(p []byte) (int, error) { c.n.Add(int64(len(p))); return len(p), nil }

var speedClient = &http.Client{Transport: &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConnsPerHost: speedStream, ForceAttemptHTTP2: false}}

// httpDownload pulls test data until ctx ends and counts the bytes.
func httpDownload(ctx context.Context, n *atomic.Int64) error {
	var wg sync.WaitGroup
	var firstErr firstError
	for i := 0; i < speedStream; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				req, _ := http.NewRequestWithContext(ctx, "GET", "https://speed.cloudflare.com/__down?bytes=25000000", nil)
				req.Header.Set("User-Agent", appName+"/"+appVersion)
				resp, err := speedClient.Do(req)
				if err != nil {
					if ctx.Err() == nil {
						firstErr.CompareAndSwap(nil, err)
					}
					return
				}
				if resp.StatusCode != http.StatusOK {
					resp.Body.Close()
					firstErr.CompareAndSwap(nil, fmt.Errorf("%w: %s", errSpeedHTTP, resp.Status))
					return
				}
				_, _ = io.Copy(countWriter{n}, resp.Body)
				resp.Body.Close()
			}
		}()
	}
	wg.Wait()
	return speedErr(&firstErr, n)
}

var errSpeedHTTP = errors.New("speed test refused")

// firstError keeps the first error any stream reported.
type firstError struct {
	mu  sync.Mutex
	err error
}

func (f *firstError) CompareAndSwap(_ error, err error) {
	f.mu.Lock()
	if f.err == nil {
		f.err = err
	}
	f.mu.Unlock()
}

// speedErr: a refused request (non-200) always fails the test; network
// errors only when nothing was transferred at all.
func speedErr(first *firstError, n *atomic.Int64) error {
	first.mu.Lock()
	e := first.err
	first.mu.Unlock()
	if e != nil && (errors.Is(e, errSpeedHTTP) || n.Load() == 0) {
		return e
	}
	return nil
}

type zeroReader struct {
	left int64
	n    *atomic.Int64
}

func (z *zeroReader) Read(p []byte) (int, error) {
	if z.left <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > z.left {
		p = p[:z.left]
	}
	for i := range p {
		p[i] = 0
	}
	z.left -= int64(len(p))
	z.n.Add(int64(len(p)))
	return len(p), nil
}

func httpUpload(ctx context.Context, n *atomic.Int64) error {
	var wg sync.WaitGroup
	var firstErr firstError
	for i := 0; i < speedStream; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for ctx.Err() == nil {
				const size = 10_000_000
				req, _ := http.NewRequestWithContext(ctx, "POST", "https://speed.cloudflare.com/__up", &zeroReader{left: size, n: n})
				req.ContentLength = size
				req.Header.Set("Content-Type", "application/octet-stream")
				req.Header.Set("User-Agent", appName+"/"+appVersion)
				resp, err := speedClient.Do(req)
				if err != nil {
					if ctx.Err() == nil {
						firstErr.CompareAndSwap(nil, err)
					}
					return
				}
				_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
				resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					firstErr.CompareAndSwap(nil, fmt.Errorf("%w: %s", errSpeedHTTP, resp.Status))
					return
				}
			}
		}()
	}
	wg.Wait()
	return speedErr(&firstErr, n)
}

type netTester struct {
	mu  sync.Mutex
	cur NetTest
}

func (t *netTester) get() NetTest {
	t.mu.Lock()
	defer t.mu.Unlock()
	c := t.cur
	c.Pings = append([]PingStats{}, t.cur.Pings...)
	return c
}

func (t *netTester) set(fn func(n *NetTest)) {
	t.mu.Lock()
	fn(&t.cur)
	t.mu.Unlock()
}

// loaded measures speed with run() for d while pinging loadTarget.
func (t *netTester) loaded(stage string, from, to float64, d time.Duration, run func(context.Context, *atomic.Int64) error) (mbps, lat float64, err error) {
	t.set(func(n *NetTest) { n.Stage, n.Progress = stage, from })
	ctx, cancel := context.WithTimeout(context.Background(), d)
	defer cancel()
	var bytes atomic.Int64
	var rtts []float64
	var rmu sync.Mutex
	done := make(chan struct{})
	go func() {
		defer close(done)
		start := time.Now()
		for ctx.Err() == nil {
			v := netPing(loadTarget, time.Second)
			rmu.Lock()
			rtts = append(rtts, v)
			rmu.Unlock()
			el := time.Since(start).Seconds() / d.Seconds()
			t.set(func(n *NetTest) { n.Progress = from + (to-from)*math.Min(1, el) })
			select {
			case <-ctx.Done():
			case <-time.After(200 * time.Millisecond):
			}
		}
	}()
	start := time.Now()
	err = run(ctx, &bytes)
	el := time.Since(start).Seconds()
	cancel()
	<-done
	rmu.Lock()
	defer rmu.Unlock()
	// skip the first second: TCP is still ramping up
	if len(rtts) > 5 {
		rtts = rtts[5:]
	}
	lat = median(rtts)
	if err != nil || bytes.Load() == 0 || el <= 0 {
		return -1, lat, err
	}
	return round1(float64(bytes.Load()) * 8 / el / 1e6), lat, nil
}

func (t *netTester) run() {
	t.set(func(n *NetTest) {
		*n = NetTest{Running: true, Stage: "ping", At: nowStr(), Pings: []PingStats{}, Idle: -1, Down: -1, Up: -1, LoadDown: -1, LoadUp: -1, Bloat: -1}
	})
	defer t.set(func(n *NetTest) { n.Running, n.Stage, n.Progress = false, "done", 1 })

	targets := [][2]string{{"cloudflare", "1.1.1.1"}, {"google", "8.8.8.8"}}
	if gw := netGateway(); gw != "" {
		targets = append([][2]string{{"router", gw}}, targets...)
	}
	res := make([][]float64, len(targets))
	var wg sync.WaitGroup
	var sent atomic.Int64
	for i, tg := range targets {
		wg.Add(1)
		go func(i int, ip string) {
			defer wg.Done()
			for k := 0; k < pingCount; k++ {
				start := time.Now()
				res[i] = append(res[i], netPing(ip, time.Second))
				done := sent.Add(1)
				t.set(func(n *NetTest) { n.Progress = 0.3 * float64(done) / float64(pingCount*len(targets)) })
				if w := pingGap - time.Since(start); w > 0 {
					time.Sleep(w)
				}
			}
		}(i, tg[1])
	}
	wg.Wait()
	var pings []PingStats
	for i, tg := range targets {
		pings = append(pings, pingStats(tg[0], tg[1], res[i]))
	}
	idle := -1.0
	for i, tg := range targets {
		if tg[0] == "cloudflare" {
			idle = median(res[i])
		}
	}
	t.set(func(n *NetTest) { n.Pings, n.Idle = pings, idle })

	down, ld, err := t.loaded("download", 0.3, 0.65, downTime, netDown)
	t.set(func(n *NetTest) { n.Down, n.LoadDown = down, ld })
	if err != nil {
		t.set(func(n *NetTest) { n.Error = "speed_failed" })
		return
	}
	up, lu, err := t.loaded("upload", 0.65, 1, upTime, netUp)
	t.set(func(n *NetTest) { n.Up, n.LoadUp = up, lu })
	if err != nil {
		t.set(func(n *NetTest) { n.Error = "upload_failed" })
	}
	if idle >= 0 {
		worst := math.Max(ld, lu)
		if worst >= 0 {
			extra := math.Max(0, worst-idle)
			t.set(func(n *NetTest) { n.Bloat, n.Grade = math.Round(extra), bloatGrade(extra) })
		}
	}
}

var theNetTest netTester

func (s *Server) netRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/nettest", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, theNetTest.get())
	})
	mux.HandleFunc("POST /api/nettest", func(w http.ResponseWriter, r *http.Request) {
		theNetTest.mu.Lock()
		busy := theNetTest.cur.Running
		if !busy {
			theNetTest.cur.Running = true
		}
		theNetTest.mu.Unlock()
		if busy {
			errJSON(w, 409, "busy")
			return
		}
		s.running.Add(1)
		go func() {
			defer s.running.Add(-1)
			defer func() {
				if r := recover(); r != nil {
					theNetTest.set(func(n *NetTest) { n.Running, n.Error = false, "internal" })
				}
			}()
			theNetTest.run()
		}()
		writeJSON(w, theNetTest.get())
	})
}
