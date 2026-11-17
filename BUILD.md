# Building OpLarik

Version: see `appVersion` in main.go and rsrc/larik.rc (bumped with every release, see CHANGELOG.md).

OpLarik is a single Go program (no cgo) with an embedded HTML/JS interface.

```
larik/
  main.go, server.go, engine.go, ops.go   engine, HTTP + SSE API, apply/revert with backups
  sys_windows.go                          registry, services, tasks, power plans, PowerShell runner
  scan_windows.go, live_windows.go        system scan, live CPU / GPU (PDH) / RAM
  overlay.go, overlay_windows.go          in-game overlay: renderer (portable), layered window, hotkey, autostart
  etw_windows.go, sensors_windows.go      FPS via ETW (DXGI/D3D9 Present events), NVML + ACPI temperatures
  auth.go, server_auth.go, auth_windows.go local account: PBKDF2 passwords, TOTP, recovery key, DPAPI, lock
  qr.go                                   small QR encoder for the authenticator setup code
  overlay/atlas.png, atlas.json           pre-rendered Inter glyphs for the overlay (tools/atlas.py)
  launch_windows.go, misc_windows.go      UAC elevation, Edge app window, install/uninstall, boost tools
  compare.go                              before / after comparison of recorded game sessions
  nvidia.go, nvapi_windows.go             NVIDIA driver settings through NVAPI DRS, with backups (nvidia.json)
  autogame.go                             automatic game mode (game detection runs in the overlay process)
  health.go, health_windows.go            disk SMART health, NVIDIA driver version check
  nettest.go                              ping / jitter / loss, speed and bufferbloat test
  cleanup.go, tasks_windows.go            weekly cleanup task (OpLarik.exe --clean), Task Scheduler helper
  update.go, update_windows.go            updates from GitHub Releases (SHA-256 checked)
  settings_io.go                          settings export / import
  cover.go, cover_windows.go              game / program pictures (Steam library artwork, .exe icons)
  hwrules.go, hwscan_windows.go           disk type, Intel memory limits, Resizable BAR, monitor refresh rates
  dllsafe_windows.go                      load DLLs from System32 only
  sys_mock.go, platform_other.go          simulated Windows PC for demo mode and tests
  tools/catalog.py                        tweak catalog source (all 4 languages) -> catalog.json
  web/                                    UI: index.html, css/app.css, js/*.js, fonts, img
  rsrc/                                   icon, manifest, version info -> rsrc_windows_amd64.syso
```

Requirements: Go 1.22+, Python 3 (catalog and overlay glyph atlas only; atlas needs Pillow). Optional: LLVM (`llvm-rc`, `llvm-cvtres`) to rebuild resources.

```sh
python3 tools/catalog.py                     # regenerate catalog.json after editing tweaks
python3 tools/atlas.py                       # regenerate overlay/atlas.* after changing overlay fonts/labels
go test ./...                                # engine round-trip tests on the simulated PC
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
  go build -buildvcs=false -trimpath -ldflags "-H windowsgui -s -w" -o OpLarik.exe .
```

Rebuild resources (icon/manifest/version):

```sh
cd rsrc && llvm-rc /FO larik.res larik.rc && llvm-cvtres /MACHINE:X64 /OUT:../rsrc_windows_amd64.syso larik.res
```

Release: `tools/package.sh` builds `dist/Larik.exe`, `dist/Larik.exe.sha256` and the zips. For in-app updates,
publish a GitHub release (repository 1larik1/oplarik) tagged `vX.Y.Z` with `OpLarik.exe` and `OpLarik.exe.sha256` as assets.

UI tests: `tools/demo.sh start && python3 tools/flow107.py tools/shots ru` and `tools/flow108.py` (Playwright, CSP enforced; restart the demo between runs).

Demo mode (any OS except Windows): `go run . --port 8765 --token test --stay`, then open
`http://127.0.0.1:8765/#t=test`. All system changes are simulated in memory.

To sign the release (removes the SmartScreen warning once reputation builds):
`signtool sign /fd SHA256 /tr http://timestamp.digicert.com /td SHA256 /a OpLarik.exe`
