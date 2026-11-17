//go:build windows

package main

import (
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	uninstallKey       = `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\OpLarik`
	legacyUninstallKey = `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Larik` // before 1.1.3
)

func programFiles() string {
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	return pf
}

func installDir() string { return filepath.Join(programFiles(), appName) }

// legacyInstallDir: where versions before 1.1.3 (still called Larik) installed.
func legacyInstallDir() string { return filepath.Join(programFiles(), "Larik") }

// Start menu and desktop shortcuts, current and pre-1.1.3 names.
const psShortcutPaths = `@(([Environment]::GetFolderPath('CommonPrograms') + '\OpLarik.lnk'), ([Environment]::GetFolderPath('CommonDesktopDirectory') + '\OpLarik.lnk'))`
const psLegacyShortcutPaths = `@(([Environment]::GetFolderPath('CommonPrograms') + '\Larik.lnk'), ([Environment]::GetFolderPath('CommonDesktopDirectory') + '\Larik.lnk'))`

// removeLegacyInstall cleans up an installation made before the rename
// (Program Files\Larik\Larik.exe): its shortcuts, its Apps & features entry
// and the old program file. Settings and undo backups are shared and kept.
func removeLegacyInstall() {
	_, _ = runPS(`foreach($p in `+psLegacyShortcutPaths+`){ Remove-Item -LiteralPath $p -Force -ErrorAction SilentlyContinue }`, false, time.Minute)
	ws := &winSys{}
	if ws.RegKeyExists(legacyUninstallKey) {
		_ = ws.RegDeleteTree(legacyUninstallKey)
	}
	dir := legacyInstallDir()
	exe, _ := os.Executable()
	if strings.EqualFold(filepath.Dir(exe), dir) {
		return // running from there; the new copy cleans it up next time
	}
	old := filepath.Join(dir, legacyExeName)
	if _, err := os.Stat(old); err != nil {
		return
	}
	if err := os.Remove(old); err != nil {
		log.Print("legacy install: ", err) // e.g. still running; harmless
		return
	}
	if ents, err := os.ReadDir(dir); err == nil {
		for _, e := range ents { // leftovers of the old updater: Larik.exe.old-… / .new-…
			n := strings.ToLower(e.Name())
			if strings.HasPrefix(n, "larik.exe.old-") || strings.HasPrefix(n, "larik.exe.new-") {
				_ = os.Remove(filepath.Join(dir, e.Name()))
			}
		}
	}
	_ = os.Remove(dir) // only if empty
}

func installStatus() map[string]any {
	ws := &winSys{}
	installed := ws.RegKeyExists(uninstallKey)
	exe, _ := os.Executable()
	from := strings.EqualFold(filepath.Dir(exe), installDir())
	return map[string]any{"installed": installed, "dir": installDir(), "fromInstalled": from, "canInstall": isElevated()}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + ".new"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	_ = os.Remove(dst)
	return os.Rename(tmp, dst)
}

// installApp copies Larik into Program Files, adds Start menu and desktop
// shortcuts and registers it in Apps & features.
func installApp() error {
	if !isElevated() {
		return errNeedAdmin
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir := installDir()
	target := filepath.Join(dir, exeName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if !strings.EqualFold(exe, target) {
		if err := copyFile(exe, target); err != nil {
			return fmt.Errorf("copy failed: %w", err)
		}
	}
	_, err = runPS(`
$s = New-Object -ComObject WScript.Shell
$t = `+psQuote(target)+`
foreach($p in `+psShortcutPaths+`){
  $l = $s.CreateShortcut($p); $l.TargetPath = $t; $l.WorkingDirectory = `+psQuote(dir)+`; $l.IconLocation = $t + ',0'; $l.Description = 'OpLarik - FPS optimizer'; $l.Save()
}
`, false, time.Minute)
	if err != nil {
		return fmt.Errorf("shortcuts: %w", err)
	}
	ws := &winSys{}
	size := uint32(0)
	if st, err := os.Stat(target); err == nil {
		size = uint32(st.Size() / 1024)
	}
	q := `"` + target + `"`
	sets := []struct {
		n string
		v RegVal
	}{
		{"DisplayName", StrVal(appName)},
		{"DisplayVersion", StrVal(appVersion)},
		{"Publisher", StrVal(appName)},
		{"DisplayIcon", StrVal(target + ",0")},
		{"InstallLocation", StrVal(dir)},
		{"UninstallString", StrVal(q + " --uninstall")},
		{"InstallDate", StrVal(time.Now().Format("20060102"))},
		{"EstimatedSize", DwordVal(size)},
		{"NoModify", DwordVal(1)},
		{"NoRepair", DwordVal(1)},
	}
	for _, s := range sets {
		if err := ws.RegSet(uninstallKey, s.n, s.v); err != nil {
			return err
		}
	}
	removeLegacyInstall()
	return nil
}

func runUninstall() {
	exe, _ := os.Executable()
	if !isElevated() {
		shellExecute("runas", exe, "--uninstall", 1)
		return
	}
	lang := uiLang()
	if msgBox(nt(lang, "uninstallQ"), mbYesNo|mbIconQuest) != idYes {
		return
	}
	cat, _ := LoadCatalog(catalogJSON)
	st := OpenStore(dataDir())
	// Stop the overlay (it exits when disabled) and remove its logon task.
	ovs := st.GetSettings()
	ovs.Overlay.Enabled = false
	ovs.AutoGame.Enabled = false
	ovs.AutoClean.Enabled = false
	ovs.Tray, ovs.TrayAuto = false, false // the tray exits when it sees this
	_ = st.SetSettings(ovs)
	_ = syncAutostart(ovs)
	_ = syncTrayAutostart(ovs)
	_ = syncCleanTask(ovs.AutoClean)
	_, _ = setBoost(&winSys{}, false) // a running Boost session always ends
	var applied []string
	var games []*Game
	st.Read(func(d *StoreData) {
		for id := range d.Applied {
			applied = append(applied, id)
		}
		games = append(games, d.Games...)
	})
	keepData := false
	extra := len(loadNVBackups()) // NVIDIA settings and DNS servers Larik changed
	if dnsHasBackup() {
		extra++
	}
	if len(applied) > 0 || len(games) > 0 || extra > 0 {
		if msgBox(fmt.Sprintf(nt(lang, "revertQ"), len(applied)+len(games)+extra), mbYesNo|mbIconQuest) == idYes {
			if dnsHasBackup() {
				_ = restoreDNS()
			}
			nvRevertAll(nvidiaDriver())
			sys := newSys()
			e := NewEngine(sys, cat, st, true)
			e.Revert(applied, nil)
			for _, g := range games {
				if g.Prio {
					_ = setGamePrio(sys, g, false)
				}
				if g.GPU {
					_ = setGamePathFlag(sys, g, "gpu", false)
				}
				if g.FSO {
					_ = setGamePathFlag(sys, g, "fso", false)
				}
			}
		} else {
			keepData = true
		}
	}
	_, _ = runPS(`
foreach($p in `+psShortcutPaths+` + `+psLegacyShortcutPaths+`){ Remove-Item -LiteralPath $p -Force -ErrorAction SilentlyContinue }
`, false, time.Minute)
	_ = (&winSys{}).RegDeleteTree(uninstallKey)
	_ = (&winSys{}).RegDeleteTree(legacyUninstallKey)
	var targets []string
	if !keepData {
		targets = append(targets, dataDir())
	}
	targets = append(targets, filepath.Join(localDir(), "WebView"))
	if strings.EqualFold(filepath.Dir(exe), installDir()) {
		targets = append(targets, installDir())
	}
	var sb strings.Builder
	sb.WriteString("ping 127.0.0.1 -n 4 >nul")
	for _, t := range targets {
		sb.WriteString(` & rmdir /s /q "` + t + `"`)
	}
	msgBox(nt(lang, "uninstallOK"), mbOK|mbIconInfo)
	_ = os.Remove(overlayStatusPath())
	cmd := exec.Command(filepath.Join(os.Getenv("SystemRoot"), "System32", "cmd.exe"))
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow, CmdLine: `cmd.exe /c ` + sb.String()}
	_ = cmd.Start()
}

// ---------------------------------------------------------------- shell helpers

func openPath(p string) {
	_ = hidden(exec.Command(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), p)).Start()
}

func openRestoreUI() {
	_ = exec.Command(sys32("rstrui")).Start()
}

func rebootPC() error {
	_, err := (&winSys{}).Exec("shutdown", "/r", "/t", "5", "/c", "OpLarik: restarting to apply optimizations")
	return err
}

func signOut() error {
	_, err := (&winSys{}).Exec("shutdown", "/l")
	return err
}

func explorerRunning() bool {
	for _, p := range listProcesses() {
		if strings.EqualFold(p.Name, "explorer.exe") {
			return true
		}
	}
	return false
}

// restartExplorer restarts the shell without leaving it elevated: Windows
// relaunches the shell itself; if it doesn't, a limited scheduled task does.
func restartExplorer() error {
	ws := &winSys{}
	_, _ = ws.Exec("taskkill", "/F", "/IM", "explorer.exe")
	for i := 0; i < 12; i++ {
		time.Sleep(500 * time.Millisecond)
		if explorerRunning() {
			return nil
		}
	}
	exe := filepath.Join(os.Getenv("SystemRoot"), "explorer.exe")
	if _, err := ws.Exec("schtasks", "/Create", "/TN", "LarikRestartExplorer", "/TR", exe, "/SC", "ONCE", "/ST", "00:00", "/RL", "LIMITED", "/F"); err == nil {
		_, _ = ws.Exec("schtasks", "/Run", "/TN", "LarikRestartExplorer")
		time.Sleep(1500 * time.Millisecond)
		_, _ = ws.Exec("schtasks", "/Delete", "/TN", "LarikRestartExplorer", "/F")
	}
	if !explorerRunning() {
		return exec.Command(exe).Start()
	}
	return nil
}

func browseExe() (string, error) {
	out, err := runPSx(`
Add-Type -AssemblyName System.Windows.Forms
$owner = New-Object System.Windows.Forms.Form -Property @{ TopMost = $true; ShowInTaskbar = $false }
$d = New-Object System.Windows.Forms.OpenFileDialog
$d.Filter = 'Game (*.exe)|*.exe'
$d.Title = 'Larik'
if($d.ShowDialog($owner) -eq [System.Windows.Forms.DialogResult]::OK){ Write-Output ('@@P@@' + $d.FileName) }
$owner.Dispose()
`, true, true, 10*time.Minute)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		if p, ok := strings.CutPrefix(strings.TrimSpace(line), "@@P@@"); ok {
			return p, nil
		}
	}
	return "", errors.New("cancelled")
}

func platformDetectGames(sys Sys) []FoundGame { return detectGames(sys) }

// ---------------------------------------------------------------- processes & memory

var (
	procK32GetProcMemInfo = kernel32.NewProc("K32GetProcessMemoryInfo")
	procK32EmptyWS        = kernel32.NewProc("K32EmptyWorkingSet")
	procGlobalMemStatus   = kernel32.NewProc("GlobalMemoryStatusEx")
	ntdll                 = syscall.NewLazyDLL("ntdll.dll")
	procNtSetSysInfo      = ntdll.NewProc("NtSetSystemInformation")
	procLookupPrivValue   = advapi32.NewProc("LookupPrivilegeValueW")
	procAdjustTokenPriv   = advapi32.NewProc("AdjustTokenPrivileges")
)

type memCounters struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

type memStatusEx struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func memStatus() memStatusEx {
	var m memStatusEx
	m.Length = uint32(unsafe.Sizeof(m))
	procGlobalMemStatus.Call(uintptr(unsafe.Pointer(&m)))
	return m
}

func listProcesses() []Proc {
	snap, err := syscall.CreateToolhelp32Snapshot(syscall.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer syscall.CloseHandle(snap)
	var pe syscall.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	var out []Proc
	for err = syscall.Process32First(snap, &pe); err == nil; err = syscall.Process32Next(snap, &pe) {
		p := Proc{Name: syscall.UTF16ToString(pe.ExeFile[:]), PID: pe.ProcessID}
		if h, e := syscall.OpenProcess(0x1000 /* QUERY_LIMITED_INFORMATION */, false, pe.ProcessID); e == nil {
			var mc memCounters
			mc.CB = uint32(unsafe.Sizeof(mc))
			if r, _, _ := procK32GetProcMemInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&mc)), uintptr(mc.CB)); r != 0 {
				p.Mem = uint64(mc.WorkingSetSize)
			}
			syscall.CloseHandle(h)
		}
		out = append(out, p)
	}
	return out
}

type luid struct {
	Low  uint32
	High int32
}
type tokenPrivileges struct {
	Count uint32
	Luid  luid
	Attr  uint32
}

func enablePrivilege(name string) error {
	p, _ := syscall.GetCurrentProcess()
	var tok syscall.Token
	if err := syscall.OpenProcessToken(p, 0x0020|0x0008 /* ADJUST_PRIVILEGES|QUERY */, &tok); err != nil {
		return err
	}
	defer tok.Close()
	var l luid
	if r, _, e := procLookupPrivValue.Call(0, uintptr(unsafe.Pointer(u16(name))), uintptr(unsafe.Pointer(&l))); r == 0 {
		return e
	}
	tp := tokenPrivileges{Count: 1, Luid: l, Attr: 2 /* SE_PRIVILEGE_ENABLED */}
	if r, _, e := procAdjustTokenPriv.Call(uintptr(tok), 0, uintptr(unsafe.Pointer(&tp)), 0, 0, 0); r == 0 {
		return e
	}
	return nil
}

// cleanMemory trims every process working set and purges the standby list.
func cleanMemory() (before, after uint64, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("memory clean failed: %v", r)
		}
	}()
	before = memStatus().AvailPhys
	_ = enablePrivilege("SeDebugPrivilege")
	for _, p := range listProcesses() {
		if p.PID <= 4 {
			continue
		}
		h, e := syscall.OpenProcess(0x1000|0x0100 /* QUERY_LIMITED|SET_QUOTA */, false, p.PID)
		if e != nil {
			continue
		}
		procK32EmptyWS.Call(uintptr(h))
		syscall.CloseHandle(h)
	}
	if enablePrivilege("SeProfileSingleProcessPrivilege") == nil {
		cmd := int32(4) // MemoryPurgeStandbyList
		procNtSetSysInfo.Call(80 /* SystemMemoryListInformation */, uintptr(unsafe.Pointer(&cmd)), 4)
	}
	time.Sleep(400 * time.Millisecond)
	after = memStatus().AvailPhys
	return before, after, nil
}

// ---------------------------------------------------------------- temp files

func tempDirs() []string {
	win := os.Getenv("SystemRoot")
	la := os.Getenv("LOCALAPPDATA")
	pd := os.Getenv("ProgramData")
	dirs := []string{os.TempDir(), filepath.Join(win, "Temp")}
	if la != "" {
		dirs = append(dirs, filepath.Join(la, "CrashDumps"), filepath.Join(la, "Temp"))
	}
	if pd != "" {
		dirs = append(dirs, filepath.Join(pd, `Microsoft\Windows\WER\ReportArchive`), filepath.Join(pd, `Microsoft\Windows\WER\ReportQueue`))
	}
	seen := map[string]bool{}
	var out []string
	for _, d := range dirs {
		k := strings.ToLower(filepath.Clean(d))
		if d == "" || seen[k] || len(k) < 8 {
			continue
		}
		seen[k] = true
		out = append(out, d)
	}
	return out
}

func walkTemp(del bool) (int64, int) {
	cutoff := time.Now().Add(-time.Hour)
	var total int64
	var n int
	for _, root := range tempDirs() {
		var dirs []string
		_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.Type()&os.ModeSymlink != 0 {
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
			info, err := d.Info()
			if err != nil || info.ModTime().After(cutoff) {
				return nil
			}
			if del {
				if os.Remove(p) != nil {
					return nil
				}
			}
			total += info.Size()
			n++
			return nil
		})
		if del {
			for i := len(dirs) - 1; i >= 0; i-- {
				_ = os.Remove(dirs[i]) // only succeeds when empty
			}
		}
	}
	return total, n
}

func tempScan() (int64, int)  { return walkTemp(false) }
func tempClean() (int64, int) { return walkTemp(true) }

func init() {
	_ = strconv.Itoa
}
