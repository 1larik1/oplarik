//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

var procRegEnumValueW = advapi32.NewProc("RegEnumValueW")

// ---------------------------------------------------------------- shader caches

func shaderCacheDirs() []CacheDir {
	la := os.Getenv("LOCALAPPDATA")
	low := filepath.Join(os.Getenv("USERPROFILE"), "AppData", "LocalLow")
	pd := os.Getenv("ProgramData")
	var out []CacheDir
	add := func(id, name, p string) {
		if filepath.IsAbs(p) {
			out = append(out, CacheDir{ID: id, Name: name, Path: p})
		}
	}
	if la != "" {
		add("dx", "DirectX", filepath.Join(la, "D3DSCache"))
		add("nvdx", "NVIDIA DirectX", filepath.Join(la, "NVIDIA", "DXCache"))
		add("nvgl", "NVIDIA OpenGL", filepath.Join(la, "NVIDIA", "GLCache"))
		add("nvtmp", "NVIDIA", filepath.Join(la, "Temp", "NVIDIA Corporation", "NV_Cache"))
		add("amddx", "AMD DirectX", filepath.Join(la, "AMD", "DxCache"))
		add("amddxc", "AMD DirectX 12", filepath.Join(la, "AMD", "DxcCache"))
		add("amdgl", "AMD OpenGL", filepath.Join(la, "AMD", "GLCache"))
		add("amdvk", "AMD Vulkan", filepath.Join(la, "AMD", "VkCache"))
	}
	if pd != "" {
		add("nvpd", "NVIDIA", filepath.Join(pd, "NVIDIA Corporation", "NV_Cache"))
	}
	if os.Getenv("USERPROFILE") != "" {
		add("intel", "Intel", filepath.Join(low, "Intel", "ShaderCache"))
	}
	return out
}

// ---------------------------------------------------------------- DNS

const psPhys = `$ads = @(Get-NetAdapter -Physical -ErrorAction SilentlyContinue | Where-Object Status -eq 'Up')`

func dnsBackupFile() string { return filepath.Join(dataDir(), "dns_backup.json") }

func dnsHasBackup() bool {
	_, err := os.Stat(dnsBackupFile())
	return err == nil
}

func currentDNS() []string {
	out, err := runPS(psPhys+`
$r = @()
foreach($a in $ads){ $r += @((Get-DnsClientServerAddress -InterfaceIndex $a.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue).ServerAddresses) }
($r | Select-Object -Unique) -join ','`, false, 30*time.Second)
	if err != nil {
		return nil
	}
	var ips []string
	for _, f := range strings.FieldsFunc(strings.TrimSpace(out), func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\r' }) {
		if ip := net.ParseIP(f); ip != nil && ip.To4() != nil {
			ips = append(ips, ip.String())
		}
	}
	return ips
}

// applyDNS remembers what each adapter used before and sets the new servers.
// The backup is keyed by adapter GUID (stable across driver reinstalls) and
// grows with adapters that weren't backed up yet, so "Restore" always returns
// every adapter Larik touched to its original setup.
func applyDNS(primary, secondary string) error {
	if net.ParseIP(primary) == nil || (secondary != "" && net.ParseIP(secondary) == nil) {
		return errors.New("bad address")
	}
	out, err := runPS(psPhys+`
if($ads.Count -eq 0){ throw 'no active network adapter' }
$bk = @{}
foreach($a in $ads){
  $g = [string]$a.InterfaceGuid
  $ns = (Get-ItemProperty -Path ("HKLM:\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\" + $g) -Name NameServer -ErrorAction SilentlyContinue).NameServer
  $bk[$g] = [string]$ns
}
$bk | ConvertTo-Json -Compress`, false, 60*time.Second)
	if err != nil {
		return err
	}
	var now map[string]string
	js := strings.TrimSpace(out)
	if i := strings.Index(js, "{"); i >= 0 {
		js = js[i:]
	}
	if json.Unmarshal([]byte(js), &now) != nil || len(now) == 0 {
		return errors.New("could not read the current DNS settings")
	}
	bk := map[string]string{}
	if b, err := os.ReadFile(dnsBackupFile()); err == nil {
		_ = json.Unmarshal(b, &bk)
	}
	for g, v := range now {
		if _, ok := bk[g]; !ok { // keep the oldest (original) value
			bk[g] = v
		}
	}
	if err := writeAtomic(dnsBackupFile(), bk); err != nil {
		return err
	}
	list := psQuote(primary)
	if secondary != "" {
		list += "," + psQuote(secondary)
	}
	_, err = runPS(psPhys+`
if($ads.Count -eq 0){ throw 'no active network adapter' }
foreach($a in $ads){ Set-DnsClientServerAddress -InterfaceIndex $a.ifIndex -ServerAddresses @(`+list+`) -ErrorAction Stop }
Clear-DnsClientCache`, false, 60*time.Second)
	return err
}

// restoreDNS puts every backed-up adapter back. Entries that fail stay in the
// backup so the user can try again; the file is removed only when all succeed.
func restoreDNS() error {
	b, err := os.ReadFile(dnsBackupFile())
	if err != nil {
		return errors.New("nothing to restore")
	}
	var bk map[string]string
	if json.Unmarshal(b, &bk) != nil {
		return errors.New("backup is damaged")
	}
	js, _ := json.Marshal(bk)
	out, err := runPS(`$bk = `+psQuote(string(js))+` | ConvertFrom-Json
$all = @(Get-NetAdapter -IncludeHidden -ErrorAction SilentlyContinue)
$failed = @()
foreach($prop in $bk.PSObject.Properties){
  $a = $all | Where-Object { [string]$_.InterfaceGuid -eq $prop.Name } | Select-Object -First 1
  if(-not $a){ continue } # adapter removed: nothing left to restore
  $v = [string]$prop.Value
  try {
    if([string]::IsNullOrWhiteSpace($v)){ Set-DnsClientServerAddress -InterfaceIndex $a.ifIndex -ResetServerAddresses -ErrorAction Stop }
    else { Set-DnsClientServerAddress -InterfaceIndex $a.ifIndex -ServerAddresses @($v -split '[,\s]+' | Where-Object { $_ }) -ErrorAction Stop }
  } catch { $failed += $prop.Name }
}
Clear-DnsClientCache
'@@FAILED@@' + ($failed -join ',')`, false, 60*time.Second)
	if err != nil {
		return err
	}
	failed := map[string]bool{}
	if i := strings.Index(out, "@@FAILED@@"); i >= 0 {
		for _, g := range strings.Split(strings.TrimSpace(out[i+len("@@FAILED@@"):]), ",") {
			if g = strings.TrimSpace(g); g != "" {
				failed[g] = true
			}
		}
	} else {
		return errors.New("restore did not finish")
	}
	if len(failed) == 0 {
		return os.Remove(dnsBackupFile())
	}
	left := map[string]string{}
	for g, v := range bk {
		if failed[g] {
			left[g] = v
		}
	}
	_ = writeAtomic(dnsBackupFile(), left)
	return errors.New("some network adapters could not be restored")
}

// ---------------------------------------------------------------- startup apps

const approvedBase = `\Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\`

type startupSource struct {
	id       string
	runKey   string // registry Run key, or "" for a folder
	folder   string
	approved string
}

func startupSources() []startupSource {
	return []startupSource{
		{id: "hkcu", runKey: `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, approved: `HKCU` + approvedBase + `Run`},
		{id: "hklm", runKey: `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Run`, approved: `HKLM` + approvedBase + `Run`},
		{id: "hklm32", runKey: `HKLM\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Run`, approved: `HKLM` + approvedBase + `Run32`},
		{id: "ufolder", folder: filepath.Join(os.Getenv("APPDATA"), `Microsoft\Windows\Start Menu\Programs\Startup`), approved: `HKCU` + approvedBase + `StartupFolder`},
		{id: "cfolder", folder: filepath.Join(os.Getenv("ProgramData"), `Microsoft\Windows\Start Menu\Programs\StartUp`), approved: `HKLM` + approvedBase + `StartupFolder`},
	}
}

func regValueNames(key string) ([]string, error) {
	h, err := openKey(key, syscall.KEY_READ)
	if err != nil {
		return nil, err
	}
	defer syscall.RegCloseKey(h)
	var names []string
	buf := make([]uint16, 16384)
	for i := uint32(0); i < 4096; i++ {
		n := uint32(len(buf))
		r, _, _ := procRegEnumValueW.Call(uintptr(h), uintptr(i), uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&n)), 0, 0, 0, 0)
		if r == 259 { // ERROR_NO_MORE_ITEMS
			break
		}
		if r != 0 {
			continue
		}
		if name := syscall.UTF16ToString(buf[:n]); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func startupItems() ([]StartupItem, error) {
	ws := &winSys{}
	var out []StartupItem
	for _, src := range startupSources() {
		if src.runKey != "" {
			names, err := regValueNames(src.runKey)
			if err != nil {
				continue
			}
			for _, n := range names {
				v, err := ws.RegGet(src.runKey, n)
				if err != nil || v.Absent {
					continue
				}
				cmd, _ := v.Str()
				ap, _ := ws.RegGet(src.approved, n)
				out = append(out, StartupItem{ID: src.id + "|" + n, Name: n, Command: cmd, Source: src.id, On: approvedOn(ap)})
			}
			continue
		}
		ents, err := os.ReadDir(src.folder)
		if err != nil {
			continue
		}
		for _, e := range ents {
			if e.IsDir() || strings.EqualFold(e.Name(), "desktop.ini") {
				continue
			}
			ap, _ := ws.RegGet(src.approved, e.Name())
			out = append(out, StartupItem{ID: src.id + "|" + e.Name(), Name: strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())),
				Command: filepath.Join(src.folder, e.Name()), Source: src.id, On: approvedOn(ap)})
		}
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out, nil
}

// setStartup only flips the StartupApproved flag, like Task Manager: the
// program's own entry is never touched, so it can always be switched back on.
func setStartup(id string, on bool) error {
	items, _ := startupItems()
	found := false
	for _, it := range items {
		if it.ID == id {
			found = true
			break
		}
	}
	if !found {
		return errors.New("unknown startup item")
	}
	srcID, name, _ := strings.Cut(id, "|")
	for _, src := range startupSources() {
		if src.id == srcID {
			return (&winSys{}).RegSet(src.approved, name, approvedValue(on))
		}
	}
	return errors.New("unknown startup item")
}
