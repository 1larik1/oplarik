//go:build windows

package main

// FPS measurement via Event Tracing for Windows, the same technique as
// Intel PresentMon: every DXGI (D3D10/11/12) and D3D9 Present call emits an
// ETW event tagged with the presenting process. No game files are touched and
// nothing is injected. Runs as an isolated child process (--fpsmon) so a
// failure can never take the overlay or engine down with it.

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"errors"
	"log"
	"math"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

var (
	procStartTraceW    = advapi32.NewProc("StartTraceW")
	procControlTraceW  = advapi32.NewProc("ControlTraceW")
	procEnableTraceEx2 = advapi32.NewProc("EnableTraceEx2")
	procOpenTraceW     = advapi32.NewProc("OpenTraceW")
	procProcessTrace   = advapi32.NewProc("ProcessTrace")
	procCloseTrace     = advapi32.NewProc("CloseTrace")
	procQPC            = kernel32.NewProc("QueryPerformanceCounter")
	procQPF            = kernel32.NewProc("QueryPerformanceFrequency")
)

const etwSession = "Larik-FPS"

type etwGUID [16]byte

func parseGUID(s string) etwGUID {
	var g etwGUID
	p := strings.Split(strings.Trim(s, "{}"), "-")
	d1, _ := strconv.ParseUint(p[0], 16, 32)
	d2, _ := strconv.ParseUint(p[1], 16, 16)
	d3, _ := strconv.ParseUint(p[2], 16, 16)
	binary.LittleEndian.PutUint32(g[0:], uint32(d1))
	binary.LittleEndian.PutUint16(g[4:], uint16(d2))
	binary.LittleEndian.PutUint16(g[6:], uint16(d3))
	rest := p[3] + p[4]
	for i := 0; i < 8; i++ {
		b, _ := strconv.ParseUint(rest[i*2:i*2+2], 16, 8)
		g[8+i] = byte(b)
	}
	return g
}

var (
	guidDXGI = parseGUID("CA11C036-0102-4A2D-A6AD-F03CFED5D3C9") // Microsoft-Windows-DXGI
	guidD3D9 = parseGUID("783ACA0A-790E-4D7F-8451-AA850511C6B9") // Microsoft-Windows-D3D9
	guidNet  = parseGUID("7DD42A49-5329-4832-8DFD-43D979153A88") // Microsoft-Windows-Kernel-Network
)

// ---- game server detection (for the ping metric)
// Kernel-Network send events (TCPv4 = 10, UDPv4 = 42) start with
// PID, size, daddr, saddr; daddr of a packet we send is always the remote host.
var (
	netMu     sync.Mutex
	netCounts = map[uint32]map[[4]byte]float64{}
)

func publicIPv4(a [4]byte) bool {
	switch {
	case a[0] == 0, a[0] == 10, a[0] == 127, a[0] >= 224:
		return false
	case a[0] == 172 && a[1]&0xf0 == 16, a[0] == 192 && a[1] == 168, a[0] == 169 && a[1] == 254:
		return false
	case a[0] == 100 && a[1]&0xc0 == 64: // carrier-grade NAT
		return false
	}
	return true
}

func onNetEvent(r *eventRecord) {
	if r.UserData == nil || r.UserDataLength < 16 {
		return
	}
	pid := *(*uint32)(r.UserData)
	var a [4]byte
	copy(a[:], unsafe.Slice((*byte)(unsafe.Add(r.UserData, 8)), 4))
	if pid == 0 || !publicIPv4(a) {
		return
	}
	netMu.Lock()
	m := netCounts[pid]
	if m == nil {
		m = map[[4]byte]float64{}
		netCounts[pid] = m
	}
	if _, ok := m[a]; ok || len(m) < 64 {
		m[a]++
	}
	netMu.Unlock()
}

// topServers returns, per process, the public address it sends the most
// packets to (with decay, so it follows server changes within seconds).
func topServers() map[string]string {
	out := map[string]string{}
	netMu.Lock()
	defer netMu.Unlock()
	for pid, m := range netCounts {
		var best [4]byte
		bestN := 0.0
		for a, n := range m {
			if n > bestN {
				best, bestN = a, n
			}
			m[a] = n * 0.8
			if m[a] < 0.5 {
				delete(m, a)
			}
		}
		if len(m) == 0 {
			delete(netCounts, pid)
		}
		if bestN >= 8 {
			out[strconv.FormatUint(uint64(pid), 10)] = strconv.Itoa(int(best[0])) + "." + strconv.Itoa(int(best[1])) + "." + strconv.Itoa(int(best[2])) + "." + strconv.Itoa(int(best[3]))
		}
	}
	return out
}

// eventRecord mirrors EVENT_RECORD (x64, 112 bytes).
type eventRecord struct {
	Size, HeaderType, Flags, EventProperty uint16
	ThreadID, ProcessID                    uint32
	TimeStamp                              int64
	ProviderID                             etwGUID
	ID                                     uint16
	Version, Channel, Level, Opcode        uint8
	Task                                   uint16
	Keyword                                uint64
	ProcessorTime                          uint64
	ActivityID                             etwGUID
	BufferContext                          uint32
	ExtDataCount, UserDataLength           uint16
	ExtendedData                           uintptr
	UserData                               unsafe.Pointer
	UserContext                            uintptr
}

func qpc() int64 {
	var v int64
	procQPC.Call(uintptr(unsafe.Pointer(&v)))
	return v
}

// newTraceProps builds EVENT_TRACE_PROPERTIES (120 bytes) + logger name space.
func newTraceProps() []byte {
	size := 120 + (len(etwSession)+1)*2 + 64
	b := make([]byte, size)
	binary.LittleEndian.PutUint32(b[0:], uint32(size)) // Wnode.BufferSize
	g := parseGUID("4C61726B-4650-5300-8000-0000000000F5")
	copy(b[24:40], g[:])                              // Wnode.Guid (session id)
	binary.LittleEndian.PutUint32(b[40:], 1)          // Wnode.ClientContext: QPC timestamps
	binary.LittleEndian.PutUint32(b[44:], 0x00020000) // WNODE_FLAG_TRACED_GUID
	binary.LittleEndian.PutUint32(b[48:], 64)         // BufferSize (KB)
	binary.LittleEndian.PutUint32(b[52:], 4)          // MinimumBuffers
	binary.LittleEndian.PutUint32(b[56:], 128)        // MaximumBuffers (room for network events)
	binary.LittleEndian.PutUint32(b[64:], 0x00000100) // EVENT_TRACE_REAL_TIME_MODE
	binary.LittleEndian.PutUint32(b[68:], 1)          // FlushTimer (s)
	binary.LittleEndian.PutUint32(b[116:], 120)       // LoggerNameOffset
	return b
}

// EVENT_FILTER_DESCRIPTOR and ENABLE_TRACE_PARAMETERS (x64 layouts).
type eventFilterDescriptor struct {
	Ptr  uint64
	Size uint32
	Type uint32
}

type enableTraceParams struct {
	Version         uint32
	EnableProperty  uint32
	ControlFlags    uint32
	SourceID        etwGUID
	FilterDesc      *eventFilterDescriptor
	FilterDescCount uint32
}

// enableNetEvents turns on only TCPv4-send (10) and UDPv4-send (42) events of
// the kernel network provider, filtered by Windows itself, so downloads can't
// flood the session and cost Present events. Network events only feed the
// ping metric: if the filter isn't supported, they are simply left off.
func enableNetEvents(session uint64) {
	filter := []byte{1, 0, 2, 0, 10, 0, 42, 0} // EVENT_FILTER_EVENT_ID: FilterIn=1, Count=2, IDs
	desc := eventFilterDescriptor{Ptr: uint64(uintptr(unsafe.Pointer(&filter[0]))), Size: uint32(len(filter)), Type: 0x80000200}
	params := enableTraceParams{Version: 2, FilterDesc: &desc, FilterDescCount: 1}
	g := guidNet
	r, _, _ := procEnableTraceEx2.Call(uintptr(session), uintptr(unsafe.Pointer(&g)), 1 /* ENABLE */, 5, /* VERBOSE */
		0x10 /* KERNEL_NETWORK_KEYWORD_IPV4 */, 0, 0, uintptr(unsafe.Pointer(&params)))
	runtime.KeepAlive(filter)
	runtime.KeepAlive(&desc)
	if r != 0 {
		log.Print("fpsmon: network events unavailable (", r, "); ping will show a dash")
	}
}

func stopETWSession() {
	p := newTraceProps()
	procControlTraceW.Call(0, uintptr(unsafe.Pointer(u16(etwSession))), uintptr(unsafe.Pointer(&p[0])), 1 /* STOP */)
}

var (
	fpsMu    sync.Mutex
	presents = map[uint32][]int64{}
)

func onETWEvent(r *eventRecord) uintptr {
	if r == nil {
		return 0
	}
	if r.ProviderID == guidNet {
		if r.ID == 10 || r.ID == 42 {
			onNetEvent(r)
		}
		return 0
	}
	ok := (r.ProviderID == guidDXGI && (r.ID == 42 || r.ID == 55)) || (r.ProviderID == guidD3D9 && r.ID == 1)
	if !ok {
		return 0
	}
	// DXGI Present_Start payload: pIDXGISwapChain (pointer), Flags, SyncInterval.
	// Presents with DXGI_PRESENT_TEST only check occlusion and show no frame.
	if r.ProviderID == guidDXGI && r.ID == 42 && r.UserData != nil {
		off := uintptr(8)
		if r.Flags&0x20 != 0 { // EVENT_HEADER_FLAG_32_BIT_HEADER: 32-bit game
			off = 4
		}
		if uintptr(r.UserDataLength) >= off+4 && *(*uint32)(unsafe.Add(r.UserData, off))&1 != 0 {
			return 0
		}
	}
	fpsMu.Lock()
	// Real-time ETW uses per-CPU buffers, so events can arrive slightly out
	// of order: keep each list sorted.
	ts := append(presents[r.ProcessID], r.TimeStamp)
	for i := len(ts) - 1; i > 0 && ts[i] < ts[i-1]; i-- {
		ts[i], ts[i-1] = ts[i-1], ts[i]
	}
	presents[r.ProcessID] = ts
	fpsMu.Unlock()
	return 0
}

func computeFPS(freq int64) (map[string][3]float64, map[string][]float64) {
	now := qpc()
	out := map[string][3]float64{}
	graphs := map[string][]float64{}
	fpsMu.Lock()
	defer fpsMu.Unlock()
	for pid, ts := range presents {
		// keep the last 5 seconds
		cut := now - 5*freq
		i := 0
		for i < len(ts) && ts[i] < cut {
			i++
		}
		ts = ts[i:]
		if len(ts) == 0 {
			delete(presents, pid)
			continue
		}
		presents[pid] = ts
		last := ts[len(ts)-1]
		if now-last > freq*3/2 || len(ts) < 3 {
			continue
		}
		// FPS over the last second of frames that arrived.
		n := 0
		for j := len(ts) - 1; j >= 0 && ts[j] > last-freq; j-- {
			n++
		}
		span := float64(last-ts[len(ts)-n]) / float64(freq)
		fps := float64(n)
		if n > 1 && span > 0.2 {
			fps = float64(n-1) / span
		}
		var ft []float64
		for j := 1; j < len(ts); j++ {
			d := float64(ts[j]-ts[j-1]) / float64(freq) * 1000
			if d > 0 && d < 1000 {
				ft = append(ft, d)
			}
		}
		low := fps
		if len(ft) >= 20 {
			sort.Float64s(ft)
			p99 := ft[int(math.Min(float64(len(ft)-1), math.Ceil(float64(len(ft))*0.99)-1))]
			if p99 > 0 {
				low = 1000 / p99
			}
		}
		avgFT := 0.0
		if fps > 0 {
			avgFT = 1000 / fps
		}
		key := strconv.FormatUint(uint64(pid), 10)
		out[key] = [3]float64{math.Round(fps*10) / 10, math.Round(low*10) / 10, math.Round(avgFT*100) / 100}
		// the last 60 frame times, for the frame-time strip
		from := len(ts) - 61
		if from < 0 {
			from = 0
		}
		g := make([]float64, 0, 60)
		for j := from + 1; j < len(ts); j++ {
			g = append(g, math.Round(float64(ts[j]-ts[j-1])/float64(freq)*10000)/10)
		}
		graphs[key] = g
	}
	return out, graphs
}

// runFPSMon is the --fpsmon child: prints {"p":{pid:[fps,low1,frametime]}} lines.
func runFPSMon() {
	w := bufio.NewWriter(os.Stdout)
	fail := func(msg string) {
		_, _ = w.WriteString(`{"error":` + strconv.Quote(msg) + "}\n")
		_ = w.Flush()
		stopETWSession() // os.Exit skips deferred calls
		os.Exit(2)
	}
	var freq int64
	procQPF.Call(uintptr(unsafe.Pointer(&freq)))
	if freq == 0 {
		fail("no QPC")
	}
	stopETWSession()
	props := newTraceProps()
	var session uint64
	r, _, _ := procStartTraceW.Call(uintptr(unsafe.Pointer(&session)), uintptr(unsafe.Pointer(u16(etwSession))), uintptr(unsafe.Pointer(&props[0])))
	if r != 0 {
		fail("StartTrace " + strconv.Itoa(int(r)))
	}
	defer stopETWSession()
	enabled := 0
	enableNetEvents(session)
	for _, g := range []etwGUID{guidDXGI, guidD3D9} {
		g := g
		if rc, _, _ := procEnableTraceEx2.Call(uintptr(session), uintptr(unsafe.Pointer(&g)), 1 /* ENABLE */, 5, /* VERBOSE */
			^uintptr(0) /* MatchAnyKeyword */, 0, 0, 0); rc == 0 {
			enabled++
		}
	}
	if enabled == 0 {
		fail("EnableTraceEx2 failed")
	}

	// EVENT_TRACE_LOGFILEW is 448 bytes on x64.
	lf := make([]byte, 448)
	name := u16(etwSession)
	*(*uintptr)(unsafe.Pointer(&lf[8])) = uintptr(unsafe.Pointer(name))      // LoggerName
	binary.LittleEndian.PutUint32(lf[28:], 0x00000100|0x10000000|0x00001000) // REAL_TIME|EVENT_RECORD|RAW_TIMESTAMP
	*(*uintptr)(unsafe.Pointer(&lf[424])) = syscall.NewCallback(onETWEvent)  // EventRecordCallback
	h, _, _ := procOpenTraceW.Call(uintptr(unsafe.Pointer(&lf[0])))
	runtime.KeepAlive(name)
	if h == ^uintptr(0) || h == 0 {
		fail("OpenTrace failed")
	}
	done := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		hh := uint64(h)
		rc, _, _ := procProcessTrace.Call(uintptr(unsafe.Pointer(&hh)), 1, 0, 0)
		if rc != 0 {
			done <- errors.New("ProcessTrace " + strconv.Itoa(int(rc)))
			return
		}
		done <- nil
	}()
	_, _ = w.WriteString("{\"ready\":true}\n")
	_ = w.Flush()
	t := time.NewTicker(500 * time.Millisecond)
	defer t.Stop()
	for {
		select {
		case err := <-done:
			if err != nil {
				fail(err.Error())
			}
			return
		case <-t.C:
			p, g := computeFPS(freq)
			b, _ := json.Marshal(map[string]any{"p": p, "g": g, "n": topServers()})
			if _, err := w.Write(append(b, '\n')); err != nil {
				procCloseTrace.Call(h)
				return
			}
			if err := w.Flush(); err != nil { // parent is gone
				procCloseTrace.Call(h)
				return
			}
		}
	}
}
