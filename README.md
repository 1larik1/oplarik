# OpLarik — FPS optimizer for Windows

OpLarik tunes Windows 10 and 11 for gaming on entry-level and mid-range PCs: it scans your hardware, applies reversible
performance tweaks, removes bloatware and gives each game its own priority and GPU settings.

Languages: English, Українська, Русский, Polski, Español, Deutsch, Français, 日本語, Čeština (switch any time in Settings → Language).

## Install

1. Download `OpLarik.exe` (or unzip `Larik-1.1.2-win-x64.zip`).
2. Double-click it. Windows asks for administrator rights — OpLarik needs them to change system settings.
3. First run only: if SmartScreen says "Windows protected your PC", click **More info → Run anyway**.
   OpLarik isn't code-signed, so Windows doesn't recognise it yet.
4. Optional: open **Settings → Install OpLarik** to copy it to Program Files and add Start menu and desktop shortcuts.
   It then appears in **Settings › Apps › Installed apps**, where you can uninstall it normally.

OpLarik also runs as a portable app — no installation needed.

Requirements: Windows 10 or 11 (64-bit) and Microsoft Edge, which is built into Windows. OpLarik's window is a
Microsoft Edge app window, so no extra runtime has to be installed.

## What's inside

| Area | What it does |
|---|---|
| Home | Optimization score, live CPU / GPU / RAM / VRAM usage, hardware tips (single-channel RAM, XMP off, HDD, Wi-Fi, 60 Hz display, old GPU driver…) |
| Optimize | 83 optimizations in 7 categories: FPS & latency, power, graphics, input, network & ping, privacy & security, quality of life. Presets: Recommended, Competitive, Privacy |
| Games | Finds Steam, Epic Games and Riot games and starts them from OpLarik. Per game: high CPU priority, high-performance GPU, true fullscreen. Recorded game sessions and the **Before / after** comparison |
| Boost | Game Boost session (max power plan + pauses Windows Update, indexing and telemetry), automatic game mode, low input lag mode, NVIDIA driver settings, RAM cleaner, temp file cleaner, shader cache cleaner, DNS flush, fastest-DNS finder, Explorer restart, close background apps |
| Startup | Programs that start with Windows, with on/off switches (same mechanism as Task Manager) |
| Debloat | Remove 44 preinstalled and sponsored apps, switch off 12 Windows components and 14 background services |
| My PC | Full system scan: CPU, GPU, memory modules, disks, motherboard, display, network, Windows, security |
| Health | Disk health (SMART) with warnings, graphics driver update check, network test (ping, jitter, packet loss, speed, bufferbloat), weekly scheduled cleanup |
| Backups | System restore points, every change OpLarik made with one-click undo, activity log |
| Themes | Fourteen color themes: seven dark and seven light; the overlay uses the same colors. Settings › Appearance |
| Account | Optional password sign-in with profile photo, two-step verification (authenticator app) and a recovery key. Settings › Account |
| Overlay | In-game performance overlay: FPS, 1% low, frame time, GPU / CPU load and temperature, GPU clock and power, VRAM, RAM, clock. Configured in Settings › Overlay |

Recommendations are tailored to your hardware: for example SysMain stays on for hard drives, memory compression stays on
below 16 GB of RAM, and power-hungry tweaks are flagged on laptops.

## New in 1.1.3

- Larik is now called **OpLarik**. Settings, game profiles and undo backups carry over.
- Updates come from GitHub automatically (github.com/1larik1/oplarik).
- Installing OpLarik replaces an older Larik installation and its shortcuts.

## New in 1.1.2

- The overlay takes the colors of the app theme (background, text and accent), not only the accent bar.
- Six new light themes (seven in total): Lavender, Sky, Lagoon, Mint, Sand, Rose, Paper.
- The Overlay settings category has no sub-sections any more.

## New in 1.1.1

- Language is its own category in Settings; the top-bar language switch is gone. Appearance shows only the themes.
- Five new languages: Español, Deutsch, Français, 日本語, Čeština.
- Animations are always on.

## New in 1.1.0

- Settings open in their own window with categories, sub-sections and search (Esc closes it).
- The Play buttons are on the Games page.

## New in 1.0.9

- Start games from OpLarik: **Play** buttons on the Games page. Steam, Epic Games and Riot games start
  through their own launcher; games start with your normal user rights, not as administrator.

## New in 1.0.8

- Game pictures: Steam artwork and program icons, taken from your PC (nothing is downloaded).
- Hard drives reported as "Unspecified" by Windows are recognized; SSD-only optimizations are offered only for SSDs.
- Smarter hardware tips: memory speed locked by the motherboard (Intel B460, H410, H470…), Resizable BAR switched off,
  monitor running below its maximum refresh rate.

## New in 1.0.7

- **Before / after** (Games): pick two sessions of the same game and compare average FPS, 1% and 0.1% low, stability,
  dips, temperatures and ping, with an FPS chart and the list of optimizations changed in between. Play the game, turn
  optimizations on, play again — OpLarik pairs the sessions automatically. Compare similar matches: maps and player
  counts change FPS a lot.
- **NVIDIA driver settings** (Boost): power management "prefer maximum performance", low latency mode On, a frame rate
  limiter, unlimited shader cache and high-performance texture filtering. These are the same global settings as in
  NVIDIA Control Panel › Manage 3D settings. OpLarik saves the previous values and **Restore all** puts them back.
  Per-game profiles in NVIDIA Control Panel still win over the global settings.
- **Automatic game mode** (Boost): when a full-screen game has been running for a few seconds, Game Boost turns on and
  the overlay appears; when the game closes, both go back. It runs in the background with the overlay process and
  starts with Windows (scheduled task *Larik Overlay*). Boost you start yourself is never switched off automatically.
- **Health** page: disk temperature, wear, power-on time and errors from SMART with warnings (budget SSDs such as the
  Kingston A400 may not report wear); the newest NVIDIA Game Ready driver compared with yours (checked only when you
  press the button); a network test that tells whether lag comes from Wi-Fi, packet loss or bufferbloat (uses
  Cloudflare's public speed test, about 100–300 MB of data); weekly cleanup of temporary files via Task Scheduler
  (*Larik Cleanup*).
- **Overlay profiles** (Settings › Overlay): different metrics and position per game.
- **Settings backup** (Settings): export settings, enabled optimizations and NVIDIA settings to a JSON file and import
  them on another PC. The account and the update source are never exported.
- **Updates** (Settings): OpLarik can update itself from GitHub Releases — see below.

## Updates

OpLarik updates itself from the public repository **https://github.com/1larik1/oplarik** (Releases). On start it
checks for a new version (Settings › Updates — the check can be switched off there) and installs it in one click.

OpLarik downloads only from GitHub over HTTPS, checks the SHA-256 checksum before replacing anything, updates both the
running copy and the installed copy in Program Files, and restarts. The checksum protects against damaged downloads; it is
not a signature — whoever controls the GitHub repository controls the updates.

For maintainers: every release is tagged `vX.Y.Z` and must carry `OpLarik.exe` and `OpLarik.exe.sha256`
(both made by `tools/package.sh`).

## Code signing

Windows SmartScreen and some antivirus engines warn about unsigned programs. Removing the warning needs a code signing
certificate bought from a certificate authority (around 100–300 USD per year, or Microsoft's Azure Trusted Signing
service); it is tied to a verified person or company, so it can't be included in the download. With a certificate:
`signtool sign /fd SHA256 /tr http://timestamp.digicert.com /td SHA256 /a OpLarik.exe` (sign before computing
`OpLarik.exe.sha256`).

## Tray icon

Settings › Tray icon. Right-click the icon next to the clock to open OpLarik, switch Game Boost on or off, or show and
hide the overlay. When a recorded game session ends, the icon shows its summary. Quick actions from the tray work
without signing in to the OpLarik account (like the overlay hotkey); opening OpLarik itself still asks for the password.

## Account and sign-in

On first start OpLarik offers to create an account (you can skip it and create one later in **Settings › Account**).

- The account exists only on this PC — there is no online registration and nothing is sent anywhere.
- OpLarik asks for the password every time it opens and locks again when you close its window. Optional auto-lock after
  5, 15 or 30 minutes without activity.
- **Two-step verification:** turn it on in Settings › Account and scan the QR code with Google Authenticator or
  Microsoft Authenticator. After the password OpLarik then asks for the 6-digit code from your phone. Codes work offline.
- **Recovery key:** shown once when you create the account. It resets a forgotten password (and turns two-step
  verification off). Keep it somewhere safe.
- After 5 wrong attempts OpLarik makes you wait (30 seconds, growing up to 15 minutes).
- What it protects: nobody can open OpLarik and change your PC's settings without the password. It is not a Windows
  password — someone with administrator rights on this PC can still delete OpLarik's data folder.
- Lost both the password and the recovery key: delete `C:\ProgramData\Larik\account` as administrator. Your
  optimizations stay; only the account is removed.

## In-game overlay

Turn it on in **Settings › Overlay**, then press **Alt+L** (or your own shortcut) to show or hide it in a game.

- New in 1.0.5: frame-time graph (stutters in red), ping to the game server, overheating and clock-drop warnings,
  and game session recording with a summary after each game (Games page; CSV files in `%LOCALAPPDATA%\Larik\Sessions`).
- Choose what it shows, where it sits (six positions), row or column layout, size, background opacity and refresh rate.
  The preview in Settings shows exactly what you'll see.
- FPS, 1% low and frame time are measured the same way as Intel PresentMon — from Windows' own DirectX event
  tracing. Nothing is injected into games, so it is safe with anti-cheat. Works with DirectX 9, 10, 11 and 12 games.
- GPU temperature, clock and power come from the NVIDIA driver (NVIDIA cards only; other cards show load and VRAM).
  CPU temperature appears only if your motherboard reports it to Windows.
- The overlay is a transparent window on top of the game. Like other non-injecting overlays it isn't visible in
  exclusive fullscreen — for games where you want it, keep **True fullscreen** (Games) and the
  **Disable fullscreen optimizations** tweak off, or play borderless/windowed.
- "Start with Windows" adds a scheduled task named *Larik Overlay*, removed again when you switch it off or uninstall.

## Safety

- Before changing anything, OpLarik saves the original value (registry value, service start type, scheduled task state,
  power setting). Turning an optimization off restores it exactly. Backups live in `C:\ProgramData\Larik`.
- OpLarik offers to create a Windows restore point before the first change of each session.
- Risky tweaks are labelled **Moderate** or **Expert** and ask for confirmation. The info panel of every tweak lists
  the exact registry keys, services, tasks or PowerShell it uses.
- OpLarik never removes Microsoft Store, Calculator, Photos, Notepad, Terminal, Edge or the Xbox sign-in components
  games rely on.
- The engine listens only on `127.0.0.1` and requires a random per-session token, so other websites can't talk to it.
  It only runs OpLarik's built-in actions — there is no way to send it arbitrary commands.

## Uninstall

Settings › Apps › Installed apps › OpLarik › Uninstall (if you used "Install OpLarik"), or just delete `OpLarik.exe` if you ran
it portable. The uninstaller removes the scheduled tasks (*Larik Overlay*, *Larik Cleanup*), ends a running Game Boost
and asks whether to restore all Windows settings OpLarik changed (including the DNS servers and NVIDIA settings).

## Troubleshooting

- Log file: `%LOCALAPPDATA%\Larik\larik.log`
- If you clicked **No** on the administrator prompt, OpLarik opens in limited mode (read-only) with a
  "Restart as administrator" button.
- Antivirus warnings: OpLarik edits system settings and isn't code-signed, which some antivirus engines flag
  heuristically. The complete source code is in `OpLarik-1.1.3-source.zip` so you can review or rebuild it.

## Credits

Icons: [Lucide](https://lucide.dev) (ISC license). Typeface: Inter by Rasmus Andersson (SIL Open Font License 1.1).
