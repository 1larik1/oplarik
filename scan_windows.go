//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const scanScript = `
$ErrorActionPreference = 'SilentlyContinue'
function N($v){ if($null -eq $v){ return '' }; return ([string]$v).Trim() }
$r = [ordered]@{}
$cs = Get-CimInstance Win32_ComputerSystem
$os = Get-CimInstance Win32_OperatingSystem
$cv = Get-ItemProperty 'HKLM:\SOFTWARE\Microsoft\Windows NT\CurrentVersion'
$r.computer = N $env:COMPUTERNAME
$r.user = N $env:USERNAME
$up = 0; if($os.LastBootUpTime){ $up = [int64]((Get-Date) - $os.LastBootUpTime).TotalSeconds }
$inst = ''; if($os.InstallDate){ $inst = $os.InstallDate.ToString('yyyy-MM-dd') }
$r.os = [ordered]@{ name = N $os.Caption; version = N $os.Version; build = [int]$os.BuildNumber; ubr = [int]$cv.UBR; display = N $cv.DisplayVersion; arch = N $os.OSArchitecture; install = $inst; uptime = $up }

$cpu = @(Get-CimInstance Win32_Processor)[0]
$r.cpu = [ordered]@{ name = N $cpu.Name; vendor = N $cpu.Manufacturer; cores = [int]$cpu.NumberOfCores; threads = [int]$cpu.NumberOfLogicalProcessors; maxMHz = [int]$cpu.MaxClockSpeed; l2 = [int]$cpu.L2CacheSize; l3 = [int]$cpu.L3CacheSize; socket = N $cpu.SocketDesignation; virt = [bool]$cpu.VirtualizationFirmwareEnabled }

$cls = @(Get-ChildItem 'HKLM:\SYSTEM\CurrentControlSet\Control\Class\{4d36e968-e325-11ce-bfc1-08002be10318}' | Where-Object { $_.PSChildName -match '^\d{4}$' } | ForEach-Object { Get-ItemProperty -Path $_.PSPath })
$vcs = @(Get-CimInstance Win32_VideoController)
$phys = @($vcs | Where-Object { $_.PNPDeviceID -like 'PCI\*' })
if($phys.Count -gt 0){ $vcs = $phys }
$gpus = @()
foreach($v in $vcs){
  $vram = [uint64]0
  if($v.AdapterRAM){ $vram = [uint64]$v.AdapterRAM }
  $m = $cls | Where-Object { $_.DriverDesc -eq $v.Name } | Select-Object -First 1
  if($m){
    $q = $m.'HardwareInformation.qwMemorySize'
    if($q){ $vram = [uint64]$q }
    elseif($m.'HardwareInformation.MemorySize'){
      $ms = $m.'HardwareInformation.MemorySize'
      if($ms -is [byte[]]){ $vram = [uint64][BitConverter]::ToUInt32($ms, 0) } else { $vram = [uint64]$ms }
    }
  }
  $dd = ''; if($v.DriverDate){ $dd = $v.DriverDate.ToString('yyyy-MM-dd') }
  $res = ''; if($v.CurrentHorizontalResolution){ $res = [string]$v.CurrentHorizontalResolution + 'x' + [string]$v.CurrentVerticalResolution }
  $gpus += ,([ordered]@{ name = N $v.Name; vendor = N $v.AdapterCompatibility; vram = $vram; driver = N $v.DriverVersion; driverDate = $dd; res = $res; hz = [int]$v.CurrentRefreshRate })
}
$r.gpus = $gpus

$mods = @()
foreach($m in @(Get-CimInstance Win32_PhysicalMemory)){
  $mods += ,([ordered]@{ size = [uint64]$m.Capacity; speed = [int]$m.Speed; conf = [int]$m.ConfiguredClockSpeed; maker = N $m.Manufacturer; part = N $m.PartNumber; slot = N $m.DeviceLocator; typeCode = [int]$m.SMBIOSMemoryType })
}
$slots = 0; foreach($a in @(Get-CimInstance Win32_PhysicalMemoryArray)){ $slots += [int]$a.MemoryDevices }
$r.ram = [ordered]@{ total = [uint64]$cs.TotalPhysicalMemory; slots = $slots; modules = $mods }

$sysNum = $null
try { $sysNum = (Get-Partition -DriveLetter $env:SystemDrive.Substring(0,1) -ErrorAction Stop | Get-Disk -ErrorAction Stop).Number } catch {}
$disks = @()
foreach($d in @(Get-PhysicalDisk)){
  $media = N $d.MediaType; $bus = N $d.BusType
  if($media -eq '' -or $media -eq 'Unspecified'){ if($bus -eq 'NVMe'){ $media = 'SSD' } }
  $isSys = ($null -ne $sysNum -and ([string]$d.DeviceId) -eq ([string]$sysNum))
  $num = -1; try { $num = [int]$d.DeviceId } catch {}
  $disks += ,([ordered]@{ model = N $d.FriendlyName; media = $media; bus = $bus; size = [uint64]$d.Size; health = N $d.HealthStatus; system = [bool]$isSys; num = $num })
}
$r.disks = $disks
$ld = Get-CimInstance Win32_LogicalDisk -Filter ("DeviceID='" + $env:SystemDrive + "'")
$sysMedia = ''; foreach($d in $disks){ if($d.system){ $sysMedia = $d.media } }
$r.sysDrive = [ordered]@{ letter = N $env:SystemDrive; total = [uint64]$ld.Size; free = [uint64]$ld.FreeSpace; media = $sysMedia }

$bb = Get-CimInstance Win32_BaseBoard; $bios = Get-CimInstance Win32_BIOS
$bd = ''; if($bios.ReleaseDate){ $bd = $bios.ReleaseDate.ToString('yyyy-MM-dd') }
$r.board = [ordered]@{ maker = N $bb.Manufacturer; product = N $bb.Product; bios = N $bios.SMBIOSBIOSVersion; biosDate = $bd }

$names = @()
foreach($mon in @(Get-CimInstance -Namespace root\wmi -ClassName WmiMonitorID)){
  $n = ''; if($mon.UserFriendlyName){ $n = -join ($mon.UserFriendlyName | Where-Object { $_ -ne 0 } | ForEach-Object { [char]$_ }) }
  $names += $n
}
$disp = @(); $i = 0
foreach($g in $gpus){ if($g.res -ne ''){ $nm = ''; if($i -lt $names.Count){ $nm = $names[$i] }; $disp += ,([ordered]@{ name = $nm; res = $g.res; hz = $g.hz }); $i++ } }
$r.displays = $disp

$net = @()
foreach($a in @(Get-NetAdapter -Physical | Where-Object Status -eq 'Up')){
  $wifi = ([string]$a.MediaType -match '802\.11') -or ([string]$a.PhysicalMediaType -match '802\.11') -or ([string]$a.InterfaceDescription -match 'Wi-?Fi|Wireless|WLAN|802\.11')
  $net += ,([ordered]@{ name = N $a.Name; desc = N $a.InterfaceDescription; speed = N $a.LinkSpeed; wifi = [bool]$wifi })
}
$r.net = $net

$plan = ''
try { $plan = N (@(Get-CimInstance -Namespace root\cimv2\power -ClassName Win32_PowerPlan -ErrorAction Stop | Where-Object { $_.IsActive })[0].ElementName) } catch {}
$bat = @(Get-CimInstance Win32_Battery).Count -gt 0
$ch = @(@(Get-CimInstance Win32_SystemEnclosure)[0].ChassisTypes)
$lap = $bat -or (@($ch | Where-Object { @(8,9,10,11,12,14,18,21,30,31,32) -contains [int]$_ }).Count -gt 0)
$r.power = [ordered]@{ plan = $plan; laptop = [bool]$lap; battery = [bool]$bat }

$vbs = $false; $hvci = $false
try { $dg = Get-CimInstance -Namespace root\Microsoft\Windows\DeviceGuard -ClassName Win32_DeviceGuard -ErrorAction Stop; $vbs = ([int]$dg.VirtualizationBasedSecurityStatus -eq 2); $hvci = (@($dg.SecurityServicesRunning) -contains 2) } catch {}
$sb = 'unsupported'; try { if(Confirm-SecureBootUEFI -ErrorAction Stop){ $sb = 'on' } else { $sb = 'off' } } catch {}
$av = ''; try { $av = N ((@(Get-CimInstance -Namespace root\SecurityCenter2 -ClassName AntiVirusProduct -ErrorAction Stop) | ForEach-Object { $_.displayName }) -join ', ') } catch {}
$r.security = [ordered]@{ vbs = [bool]$vbs; hvci = [bool]$hvci; secureBoot = $sb; av = $av }

$r.counts = [ordered]@{ processes = @(Get-Process).Count; services = @(Get-Service | Where-Object { $_.Status -eq 'Running' }).Count; startup = @(Get-CimInstance Win32_StartupCommand).Count }
$r.printers = @(Get-CimInstance Win32_Printer | Where-Object { $_.Name -notmatch 'PDF|XPS|OneNote|Fax|Send To|AnyDesk' }).Count
Write-Output ('@@JSON@@' + ($r | ConvertTo-Json -Depth 6 -Compress))
`

func scanSystem(sys Sys) (*SysInfo, error) {
	out, err := runPS(scanScript, false, 2*time.Minute)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(out, "\n") {
		if js, ok := strings.CutPrefix(strings.TrimSpace(line), "@@JSON@@"); ok {
			var info SysInfo
			if err := json.Unmarshal([]byte(js), &info); err != nil {
				return nil, err
			}
			refineScan(&info)
			return &info, nil
		}
	}
	return nil, errors.New("system scan returned no data")
}
