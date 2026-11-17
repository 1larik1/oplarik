//go:build windows

package main

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

var (
	advapi32             = syscall.NewLazyDLL("advapi32.dll")
	procRegCreateKeyExW  = advapi32.NewProc("RegCreateKeyExW")
	procRegSetValueExW   = advapi32.NewProc("RegSetValueExW")
	procRegDeleteValueW  = advapi32.NewProc("RegDeleteValueW")
	procRegDeleteTreeW   = advapi32.NewProc("RegDeleteTreeW")
	procOpenSCManagerW   = advapi32.NewProc("OpenSCManagerW")
	procOpenServiceW     = advapi32.NewProc("OpenServiceW")
	procQueryServiceStat = advapi32.NewProc("QueryServiceStatus")
	procCloseServiceH    = advapi32.NewProc("CloseServiceHandle")
	procStartServiceW    = advapi32.NewProc("StartServiceW")
	procControlService   = advapi32.NewProc("ControlService")
)

const (
	createNoWindow = 0x08000000
	keyWow64_64    = 0x0100
)

type winSys struct{}

func newSys() Sys { return &winSys{} }

func splitKey(path string) (syscall.Handle, string, error) {
	root, sub, _ := strings.Cut(path, `\`)
	switch strings.ToUpper(root) {
	case "HKLM", "HKEY_LOCAL_MACHINE":
		return syscall.HKEY_LOCAL_MACHINE, sub, nil
	case "HKCU", "HKEY_CURRENT_USER":
		return syscall.HKEY_CURRENT_USER, sub, nil
	case "HKU", "HKEY_USERS":
		return syscall.HKEY_USERS, sub, nil
	case "HKCR", "HKEY_CLASSES_ROOT":
		return syscall.HKEY_CLASSES_ROOT, sub, nil
	}
	return 0, "", fmt.Errorf("unknown registry hive in %q", path)
}

func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func openKey(path string, access uint32) (syscall.Handle, error) {
	root, sub, err := splitKey(path)
	if err != nil {
		return 0, err
	}
	var h syscall.Handle
	if err := syscall.RegOpenKeyEx(root, u16(sub), 0, access|keyWow64_64, &h); err != nil {
		return 0, err
	}
	return h, nil
}

func (w *winSys) RegGet(key, name string) (RegVal, error) {
	h, err := openKey(key, syscall.KEY_QUERY_VALUE)
	if err != nil {
		if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) || errors.Is(err, syscall.ERROR_PATH_NOT_FOUND) {
			return RegVal{Absent: true}, nil
		}
		return RegVal{}, err
	}
	defer syscall.RegCloseKey(h)
	var typ, n uint32
	err = syscall.RegQueryValueEx(h, u16(name), nil, &typ, nil, &n)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return RegVal{Absent: true}, nil
	}
	if err != nil && !errors.Is(err, syscall.ERROR_MORE_DATA) {
		return RegVal{}, err
	}
	buf := make([]byte, n+2)
	n = uint32(len(buf))
	var p *byte
	if len(buf) > 0 {
		p = &buf[0]
	}
	if err := syscall.RegQueryValueEx(h, u16(name), nil, &typ, p, &n); err != nil {
		return RegVal{}, err
	}
	return RegVal{Type: typ, Data: buf[:n]}, nil
}

func createKey(path string, access uint32) (syscall.Handle, error) {
	root, sub, err := splitKey(path)
	if err != nil {
		return 0, err
	}
	var h syscall.Handle
	var disp uint32
	r, _, _ := procRegCreateKeyExW.Call(uintptr(root), uintptr(unsafe.Pointer(u16(sub))), 0, 0, 0,
		uintptr(access|keyWow64_64), 0, uintptr(unsafe.Pointer(&h)), uintptr(unsafe.Pointer(&disp)))
	if r != 0 {
		return 0, syscall.Errno(r)
	}
	return h, nil
}

func (w *winSys) RegSet(key, name string, v RegVal) error {
	if v.Absent {
		return w.RegDelete(key, name)
	}
	h, err := createKey(key, syscall.KEY_SET_VALUE)
	if err != nil {
		return fmt.Errorf("%s: %w", key, err)
	}
	defer syscall.RegCloseKey(h)
	data := v.Data
	var p uintptr
	if len(data) > 0 {
		p = uintptr(unsafe.Pointer(&data[0]))
	}
	r, _, _ := procRegSetValueExW.Call(uintptr(h), uintptr(unsafe.Pointer(u16(name))), 0, uintptr(v.Type), p, uintptr(len(data)))
	if r != 0 {
		return fmt.Errorf(`%s\%s: %w`, key, name, syscall.Errno(r))
	}
	return nil
}

func (w *winSys) RegDelete(key, name string) error {
	h, err := openKey(key, syscall.KEY_SET_VALUE)
	if err != nil {
		if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) || errors.Is(err, syscall.ERROR_PATH_NOT_FOUND) {
			return nil
		}
		return err
	}
	defer syscall.RegCloseKey(h)
	r, _, _ := procRegDeleteValueW.Call(uintptr(h), uintptr(unsafe.Pointer(u16(name))))
	if r != 0 && syscall.Errno(r) != syscall.ERROR_FILE_NOT_FOUND {
		return syscall.Errno(r)
	}
	return nil
}

func (w *winSys) RegKeyExists(key string) bool {
	h, err := openKey(key, syscall.KEY_QUERY_VALUE)
	if err != nil {
		return false
	}
	syscall.RegCloseKey(h)
	return true
}

func (w *winSys) RegCreateKey(key string) error {
	h, err := createKey(key, syscall.KEY_QUERY_VALUE)
	if err != nil {
		return err
	}
	syscall.RegCloseKey(h)
	return nil
}

func (w *winSys) RegDeleteTree(key string) error {
	if !w.RegKeyExists(key) {
		return nil
	}
	root, sub, err := splitKey(key)
	if err != nil {
		return err
	}
	if sub == "" || !strings.Contains(sub, `\`) {
		return fmt.Errorf("refusing to delete top-level key %q", key)
	}
	r, _, _ := procRegDeleteTreeW.Call(uintptr(root), uintptr(unsafe.Pointer(u16(sub))))
	if r != 0 && syscall.Errno(r) != syscall.ERROR_FILE_NOT_FOUND {
		return syscall.Errno(r)
	}
	return nil
}

func (w *winSys) RegSubkeys(key string) ([]string, error) {
	h, err := openKey(key, syscall.KEY_ENUMERATE_SUB_KEYS|syscall.KEY_QUERY_VALUE)
	if err != nil {
		return nil, err
	}
	defer syscall.RegCloseKey(h)
	var out []string
	for i := uint32(0); ; i++ {
		buf := make([]uint16, 512)
		n := uint32(len(buf))
		err := syscall.RegEnumKeyEx(h, i, &buf[0], &n, nil, nil, nil, nil)
		if errors.Is(err, syscall.Errno(259)) {
			break
		}
		if err != nil {
			return out, err
		}
		out = append(out, syscall.UTF16ToString(buf[:n]))
	}
	return out, nil
}

// ---------------------------------------------------------------- services

const svcBase = `HKLM\SYSTEM\CurrentControlSet\Services\`

func (w *winSys) SvcStart(name string) (string, bool) {
	v, err := w.RegGet(svcBase+name, "Start")
	if err != nil {
		return "", false
	}
	n, ok := v.Num()
	if !ok {
		return "", false
	}
	switch n {
	case 0:
		return "boot", true
	case 1:
		return "system", true
	case 2:
		d, _ := w.RegGet(svcBase+name, "DelayedAutostart")
		if dn, ok := d.Num(); ok && dn == 1 {
			return "delayed-auto", true
		}
		return "auto", true
	case 3:
		return "demand", true
	case 4:
		return "disabled", true
	}
	return "", true
}

func (w *winSys) SvcConfig(name, start string) error {
	switch start {
	case "boot", "system", "auto", "demand", "disabled", "delayed-auto":
	default:
		return fmt.Errorf("bad start type %q", start)
	}
	_, err := w.Exec("sc", "config", name, "start=", start)
	return err
}

func scHandle(name string, access uint32) (uintptr, uintptr, error) {
	m, _, e := procOpenSCManagerW.Call(0, 0, 1) // SC_MANAGER_CONNECT
	if m == 0 {
		return 0, 0, e
	}
	s, _, e := procOpenServiceW.Call(m, uintptr(unsafe.Pointer(u16(name))), uintptr(access))
	if s == 0 {
		procCloseServiceH.Call(m)
		return 0, 0, e
	}
	return m, s, nil
}

type serviceStatus struct {
	ServiceType, CurrentState, ControlsAccepted, Win32ExitCode, ServiceSpecificExitCode, CheckPoint, WaitHint uint32
}

func serviceRunning(name string) bool {
	m, s, err := scHandle(name, 0x0004) // SERVICE_QUERY_STATUS
	if err != nil {
		return false
	}
	defer procCloseServiceH.Call(m)
	defer procCloseServiceH.Call(s)
	var st serviceStatus
	r, _, _ := procQueryServiceStat.Call(s, uintptr(unsafe.Pointer(&st)))
	return r != 0 && (st.CurrentState == 4 || st.CurrentState == 2) // RUNNING / START_PENDING
}

func (w *winSys) SvcStop(name string) {
	m, s, err := scHandle(name, 0x0020|0x0004) // SERVICE_STOP | QUERY_STATUS
	if err != nil {
		return
	}
	defer procCloseServiceH.Call(m)
	defer procCloseServiceH.Call(s)
	var st serviceStatus
	procControlService.Call(s, 1, uintptr(unsafe.Pointer(&st))) // SERVICE_CONTROL_STOP
}

func (w *winSys) SvcRun(name string) {
	m, s, err := scHandle(name, 0x0010) // SERVICE_START
	if err != nil {
		return
	}
	defer procCloseServiceH.Call(m)
	defer procCloseServiceH.Call(s)
	procStartServiceW.Call(s, 0, 0)
}

// ---------------------------------------------------------------- scheduled tasks

var reSettings = regexp.MustCompile(`(?s)<Settings>(.*?)</Settings>`)
var reEnabled = regexp.MustCompile(`<Enabled>(true|false)</Enabled>`)

func decodeText(b []byte) string {
	if len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		u := make([]uint16, (len(b)-2)/2)
		for i := range u {
			u[i] = uint16(b[2+2*i]) | uint16(b[3+2*i])<<8
		}
		return string(utf16.Decode(u))
	}
	return string(bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF}))
}

func taskEnabledFromXML(x string) bool {
	m := reSettings.FindStringSubmatch(x)
	if m == nil {
		return true
	}
	// Only the direct <Enabled> of <Settings>; nested blocks don't use it.
	inner := regexp.MustCompile(`(?s)<(IdleSettings|NetworkSettings|RestartOnFailure|MaintenanceSettings)>.*?</(IdleSettings|NetworkSettings|RestartOnFailure|MaintenanceSettings)>`).ReplaceAllString(m[1], "")
	e := reEnabled.FindStringSubmatch(inner)
	return e == nil || e[1] == "true"
}

func (w *winSys) TaskState(path string) (bool, bool) {
	file := filepath.Join(os.Getenv("SystemRoot"), "System32", "Tasks", strings.TrimPrefix(path, `\`))
	if b, err := os.ReadFile(file); err == nil {
		return true, taskEnabledFromXML(decodeText(b))
	} else if os.IsNotExist(err) {
		return false, false
	}
	out, err := w.Exec("schtasks", "/Query", "/TN", path, "/XML", "ONE")
	if err != nil {
		return false, false
	}
	return true, taskEnabledFromXML(out)
}

func (w *winSys) TaskSet(path string, enabled bool) error {
	flag := "/Disable"
	if enabled {
		flag = "/Enable"
	}
	_, err := w.Exec("schtasks", "/Change", "/TN", path, flag)
	return err
}

// ---------------------------------------------------------------- power

var reHex = regexp.MustCompile(`0x[0-9a-fA-F]{8}`)
var reGUID = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

func (w *winSys) PowerGet(sub, set string) (int, int, bool) {
	out, err := w.Exec("powercfg", "/qh", "scheme_current", sub, set)
	if err != nil {
		return 0, 0, false
	}
	hx := reHex.FindAllString(out, -1)
	if len(hx) < 2 {
		return 0, 0, false
	}
	ac, _ := strconv.ParseInt(hx[len(hx)-2][2:], 16, 64)
	dc, _ := strconv.ParseInt(hx[len(hx)-1][2:], 16, 64)
	return int(ac), int(dc), true
}

func (w *winSys) PowerSet(sub, set string, ac, dc int, setDC bool) error {
	if _, err := w.Exec("powercfg", "/setacvalueindex", "scheme_current", sub, set, strconv.Itoa(ac)); err != nil {
		return err
	}
	if setDC {
		if _, err := w.Exec("powercfg", "/setdcvalueindex", "scheme_current", sub, set, strconv.Itoa(dc)); err != nil {
			return err
		}
	}
	_, err := w.Exec("powercfg", "/setactive", "scheme_current")
	return err
}

func (w *winSys) ActivePlan() string {
	if v, err := w.RegGet(`HKLM\SYSTEM\CurrentControlSet\Control\Power\User\PowerSchemes`, "ActivePowerScheme"); err == nil {
		if s, ok := v.Str(); ok && s != "" {
			return strings.ToLower(s)
		}
	}
	out, _ := w.Exec("powercfg", "/getactivescheme")
	return strings.ToLower(reGUID.FindString(out))
}

func (w *winSys) PlanExists(guid string) bool {
	out, err := w.Exec("powercfg", "/list")
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(out), strings.ToLower(guid))
}

// ---------------------------------------------------------------- exec / PowerShell

func sys32(name string) string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	return filepath.Join(root, "System32", name+".exe")
}

func hidden(cmd *exec.Cmd) *exec.Cmd {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}

func asciiOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r < 128 && (r >= 32 || r == '\n') {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

func (w *winSys) Exec(args ...string) (string, error) {
	if len(args) == 0 {
		return "", errors.New("empty command")
	}
	bin := args[0]
	if !strings.ContainsAny(bin, `\/`) {
		bin = sys32(bin)
	}
	cmd := hidden(exec.Command(bin, args[1:]...))
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		return "", err
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		s := out.String()
		if err != nil {
			msg := asciiOnly(s)
			if len(msg) > 200 {
				msg = msg[:200]
			}
			return s, fmt.Errorf("%s failed (%v) %s", args[0], err, msg)
		}
		return s, nil
	case <-time.After(90 * time.Second):
		_ = cmd.Process.Kill()
		return out.String(), fmt.Errorf("%s timed out", args[0])
	}
}

const psPreamble = "$ProgressPreference='SilentlyContinue'\n$ErrorActionPreference='Stop'\ntry { [Console]::OutputEncoding = [Text.Encoding]::UTF8 } catch {}\n"

func psPath() string {
	return filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}

// runPS executes a PowerShell 5.1 script hidden and returns stdout.
func runPS(script string, sta bool, timeout time.Duration) (string, error) {
	return runPSx(script, sta, false, timeout)
}

// runPSx with gui=true skips the SW_HIDE startup flag, which would otherwise
// hide the first window the script shows (e.g. a file picker).
func runPSx(script string, sta, gui bool, timeout time.Duration) (string, error) {
	full := psPreamble + "try {\n" + script + "\n} catch { Write-Output ('@@ERR@@' + $_.Exception.Message); exit 1 }\n"
	args := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass"}
	if sta {
		args = append(args, "-STA")
	}
	u := utf16.Encode([]rune(full))
	b := make([]byte, len(u)*2)
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	enc := base64.StdEncoding.EncodeToString(b)
	var tmp string
	if len(enc) > 24000 {
		f, err := os.CreateTemp("", "larik-*.ps1")
		if err != nil {
			return "", err
		}
		_, _ = f.Write([]byte{0xEF, 0xBB, 0xBF})
		_, _ = f.WriteString(full)
		f.Close()
		tmp = f.Name()
		defer os.Remove(tmp)
		args = append(args, "-File", tmp)
	} else {
		args = append(args, "-EncodedCommand", enc)
	}
	cmd := exec.Command(psPath(), args...)
	if gui {
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	} else {
		hidden(cmd)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		return "", err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	var err error
	select {
	case err = <-done:
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		return out.String(), errors.New("PowerShell timed out")
	}
	s := strings.ReplaceAll(out.String(), "\r", "")
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "@@ERR@@") {
			return s, errors.New(strings.TrimSpace(strings.TrimPrefix(line, "@@ERR@@")))
		}
	}
	if err != nil {
		msg := strings.TrimSpace(errb.String())
		if len(msg) > 300 {
			msg = msg[:300]
		}
		return s, fmt.Errorf("PowerShell failed: %v %s", err, msg)
	}
	return s, nil
}

func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func (w *winSys) PSRun(id, mode, script, backup string) (string, error) {
	pre := "$LarikBackup = " + psQuote(backup) + "\n"
	return runPS(pre+script, false, 10*time.Minute)
}

func (w *winSys) PSChecks(checks map[string]string) map[string]string {
	var sb strings.Builder
	for id, chk := range checks {
		fmt.Fprintf(&sb, "try { $r = & {\n%s\n}; $v = [string](@($r) | Select-Object -Last 1) } catch { $v = 'err' }\nWrite-Output ('@@C@@%s|' + $v)\n", chk, id)
	}
	out, _ := runPS(sb.String(), false, 2*time.Minute)
	res := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if rest, ok := strings.CutPrefix(line, "@@C@@"); ok {
			id, v, _ := strings.Cut(rest, "|")
			if v == "err" || v == "" {
				v = "na" // the check itself failed: treat as not available here
			}
			res[id] = v
		}
	}
	for id := range checks {
		if _, ok := res[id]; !ok {
			res[id] = "na"
		}
	}
	return res
}

// ---------------------------------------------------------------- debloat

func psArray(items []string) string {
	q := make([]string, len(items))
	for i, s := range items {
		q[i] = psQuote(s)
	}
	return "@(" + strings.Join(q, ",") + ")"
}

func (w *winSys) AppxInstalled() ([]string, error) {
	out, err := runPS(`
$n = @(Get-AppxPackage -AllUsers -ErrorAction SilentlyContinue | Select-Object -ExpandProperty Name)
$p = @(Get-AppxProvisionedPackage -Online -ErrorAction SilentlyContinue | Select-Object -ExpandProperty DisplayName)
foreach($x in ($n + $p | Sort-Object -Unique)){ Write-Output ('@@L@@' + $x) }
`, false, 3*time.Minute)
	if err != nil {
		return nil, err
	}
	var list []string
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "@@L@@"); ok {
			list = append(list, rest)
		}
	}
	return list, nil
}

func (w *winSys) AppxRemove(patterns []string) error {
	_, err := runPS(`
$errs = @()
foreach($p in `+psArray(patterns)+`){
  foreach($pkg in @(Get-AppxPackage -AllUsers -Name $p -ErrorAction SilentlyContinue)){
    try { Remove-AppxPackage -Package $pkg.PackageFullName -AllUsers -ErrorAction Stop }
    catch { try { Remove-AppxPackage -Package $pkg.PackageFullName -ErrorAction Stop } catch { $errs += $_.Exception.Message } }
  }
  foreach($pp in @(Get-AppxProvisionedPackage -Online -ErrorAction SilentlyContinue | Where-Object { $_.DisplayName -like $p })){
    try { Remove-AppxProvisionedPackage -Online -PackageName $pp.PackageName -ErrorAction Stop | Out-Null } catch {}
  }
}
if($errs.Count -gt 0){ throw ($errs | Select-Object -First 1) }
`, false, 5*time.Minute)
	return err
}

func (w *winSys) Features(names []string) (map[string]string, error) {
	out, err := runPS(`
$names = `+psArray(names)+`
foreach($f in @(Get-WindowsOptionalFeature -Online -ErrorAction Stop)){ if($names -contains $f.FeatureName){ Write-Output ('@@F@@' + $f.FeatureName + '|' + [string]$f.State) } }
`, false, 3*time.Minute)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "@@F@@"); ok {
			n, st, _ := strings.Cut(rest, "|")
			m[n] = st
		}
	}
	return m, nil
}

func (w *winSys) FeatureSet(name string, enable bool) error {
	cmd := "Disable-WindowsOptionalFeature -Online -FeatureName " + psQuote(name) + " -NoRestart -ErrorAction Stop | Out-Null"
	if enable {
		cmd = "Enable-WindowsOptionalFeature -Online -FeatureName " + psQuote(name) + " -All -NoRestart -ErrorAction Stop | Out-Null"
	}
	_, err := runPS(cmd, false, 10*time.Minute)
	return err
}

func (w *winSys) Caps(prefixes []string) (map[string]CapInfo, error) {
	out, err := runPS(`
$pre = `+psArray(prefixes)+`
foreach($c in @(Get-WindowsCapability -Online -ErrorAction Stop)){
  foreach($p in $pre){ if($c.Name -like ($p + '~*')){ Write-Output ('@@K@@' + $p + '|' + $c.Name + '|' + [string]$c.State) } }
}
`, false, 4*time.Minute)
	if err != nil {
		return nil, err
	}
	m := map[string]CapInfo{}
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "@@K@@"); ok {
			parts := strings.SplitN(rest, "|", 3)
			if len(parts) != 3 {
				continue
			}
			if cur, ok := m[parts[0]]; ok && cur.State == "Installed" {
				continue
			}
			m[parts[0]] = CapInfo{Name: parts[1], State: parts[2]}
		}
	}
	return m, nil
}

func (w *winSys) CapRemove(name string) error {
	_, err := runPS("Remove-WindowsCapability -Online -Name "+psQuote(name)+" -ErrorAction Stop | Out-Null", false, 10*time.Minute)
	return err
}

func (w *winSys) CapAdd(name string) error {
	_, err := runPS("Add-WindowsCapability -Online -Name "+psQuote(name)+" -ErrorAction Stop | Out-Null", false, 15*time.Minute)
	return err
}

// OpenURL hands the URL to the already-running (non-elevated) Explorer so
// browsers and the Store never start with administrator rights.
func (w *winSys) OpenURL(url string) {
	_ = hidden(exec.Command(filepath.Join(os.Getenv("SystemRoot"), "explorer.exe"), url)).Start()
}
