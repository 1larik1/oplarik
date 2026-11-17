//go:build windows

package main

import (
	"encoding/json"
	"errors"
	"net"
	"strings"
	"time"
)

const psDiskHealth = `function N($v){ if($null -eq $v){ return '' }; return ([string]$v).Trim() }
function F($v){ if($null -eq $v){ return $null }; try { return [double]$v } catch { return $null } }
# SMART failure prediction is keyed by the disk's PnP device path; Win32_DiskDrive
# links that path to the disk number that Get-PhysicalDisk reports as DeviceId.
$pred = @(); $drives = @()
try { $pred = @(Get-CimInstance -Namespace root\wmi -ClassName MSStorageDriver_FailurePredictStatus -ErrorAction Stop) } catch {}
try { $drives = @(Get-CimInstance Win32_DiskDrive -ErrorAction Stop) } catch {}
$disks = @()
foreach($d in @(Get-PhysicalDisk)){
  $r = $null
  try { $r = $d | Get-StorageReliabilityCounter -ErrorAction Stop } catch {}
  $media = N $d.MediaType; $bus = N $d.BusType
  if($media -eq '' -or $media -eq 'Unspecified'){ if($bus -eq 'NVMe'){ $media = 'SSD' } else { $media = '' } }
  $fail = $false
  foreach($dd in $drives){
    if(([string]$dd.Index) -ne ([string]$d.DeviceId) -or -not $dd.PNPDeviceID){ continue }
    foreach($p in $pred){ if(([string]$p.InstanceName).StartsWith([string]$dd.PNPDeviceID, [StringComparison]::OrdinalIgnoreCase) -and $p.PredictFailure){ $fail = $true } }
  }
  $o = [ordered]@{ name = N $d.FriendlyName; media = $media; bus = $bus; size = [uint64]$d.Size; health = N $d.HealthStatus; fail = $fail; num = -1;
    temp = $null; tempMax = $null; wear = $null; hours = $null; readErr = $null; writeErr = $null }
  try { $o.num = [int]$d.DeviceId } catch {}
  if($r){
    $o.temp = F $r.Temperature; $o.tempMax = F $r.TemperatureMax; $o.wear = F $r.Wear; $o.hours = F $r.PowerOnHours
    $o.readErr = F $r.ReadErrorsUncorrected; $o.writeErr = F $r.WriteErrorsUncorrected
  }
  $disks += ,$o
}
[ordered]@{ disks = $disks } | ConvertTo-Json -Depth 4 -Compress`

func val(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}

func readDiskHealth() ([]DiskHealth, error) {
	out, err := runPS(psDiskHealth, false, 60*time.Second)
	if err != nil {
		return nil, err
	}
	js := strings.TrimSpace(out)
	if i := strings.Index(js, "{"); i >= 0 {
		js = js[i:]
	}
	var r struct {
		Disks []struct {
			Name     string   `json:"name"`
			Media    string   `json:"media"`
			Bus      string   `json:"bus"`
			Size     uint64   `json:"size"`
			Health   string   `json:"health"`
			Fail     bool     `json:"fail"`
			Num      int      `json:"num"`
			Temp     *float64 `json:"temp"`
			TempMax  *float64 `json:"tempMax"`
			Wear     *float64 `json:"wear"`
			Hours    *float64 `json:"hours"`
			ReadErr  *float64 `json:"readErr"`
			WriteErr *float64 `json:"writeErr"`
		} `json:"disks"`
	}
	if err := json.Unmarshal([]byte(js), &r); err != nil {
		return nil, errors.New("could not read disk health")
	}
	var list []DiskHealth
	for _, d := range r.Disks {
		h := DiskHealth{Name: d.Name, Media: fixMedia(d.Media, d.Bus, d.Name, seekPenalty(d.Num)), Bus: d.Bus, Size: d.Size, Health: d.Health, Predict: d.Fail,
			Temp: val(d.Temp), TempMax: val(d.TempMax), Wear: val(d.Wear), Hours: val(d.Hours), ReadErr: val(d.ReadErr), WriteErr: val(d.WriteErr)}
		// Drives report 0 °C / 0 h when they don't support the counter.
		if h.Temp == 0 {
			h.Temp = -1
		}
		if h.TempMax == 0 {
			h.TempMax = -1
		}
		if h.Hours == 0 {
			h.Hours = -1
		}
		list = append(list, h)
	}
	return list, nil
}

// defaultGateway returns the IPv4 router of the active connection.
func defaultGateway() string {
	out, err := runPS(`$r = Get-NetRoute -DestinationPrefix '0.0.0.0/0' -ErrorAction SilentlyContinue | Sort-Object { $_.RouteMetric + $_.InterfaceMetric } | Select-Object -First 1
if($r){ [string]$r.NextHop }`, false, 20*time.Second)
	if err != nil {
		return ""
	}
	ip := net.ParseIP(strings.TrimSpace(out))
	if ip == nil || ip.To4() == nil || ip.IsUnspecified() {
		return ""
	}
	return ip.String()
}

func pingOnce(ip string, timeout time.Duration) float64 { return icmpPing(ip, timeout) }
