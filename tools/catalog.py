#!/usr/bin/env python3
"""Larik tweak catalog generator.

Produces catalog.json (embedded into Larik.exe). Every tweak is a list of
reversible operations plus EN/RU/UK/PL text. The Go engine executes ops,
stores the original values before changing anything, and restores them on
revert.
"""
import json
import os
import sys

OUT = os.path.join(os.path.dirname(__file__), "..", "catalog.json")


def L(en, ru, uk, pl):
    return {"en": en, "ru": ru, "uk": uk, "pl": pl}


# ---------------------------------------------------------------- op helpers
def reg(k, n, vt, v, d=None):
    """Set registry value. d = Windows default used for revert when no backup
    exists (None = delete the value)."""
    return {"t": "reg", "k": k, "n": n, "vt": vt, "v": v, "d": d}


def regkey(k, dv=""):
    return {"t": "regkey", "k": k, "dv": dv}


def regeach(k, req, n, vt, v, d=None):
    return {"t": "regeach", "k": k, "req": req, "n": n, "vt": vt, "v": v, "d": d}


def dxkv(key, val):
    return {"t": "dxkv", "key": key, "val": val}


def svc(name, s, d):
    return {"t": "svc", "n": name, "s": s, "d": d}


def task(path):
    return {"t": "task", "path": path}


def power(sub, setting, v, d, dc=False):
    return {"t": "power", "sub": sub, "set": setting, "v": v, "d": d, "dc": dc}


def plan():
    return {"t": "plan"}


def execop(apply, revert, detect):
    return {"t": "exec", "run": apply, "undo": revert, "detect": detect}


def ps(apply, revert, check):
    return {"t": "ps", "apply": apply.strip(), "revert": revert.strip(), "check": check.strip()}


def appx(*patterns):
    return {"t": "appx", "p": list(patterns)}


def feature(name):
    return {"t": "feature", "n": name}


def cap(prefix):
    return {"t": "cap", "n": prefix}


TWEAKS = []


def T(id, cat, risk, name, desc, ops, rec=False, imp=2, restart=None, tags=None,
      req=None, when=None, warn=None, store=None):
    assert risk in ("safe", "moderate", "advanced"), id
    assert restart in (None, "pc", "signout", "explorer"), id
    t = {"id": id, "cat": cat, "risk": risk, "rec": rec, "imp": imp,
         "restart": restart, "tags": tags or [], "req": req or {},
         "when": when or {}, "name": name, "desc": desc, "ops": ops}
    if warn:
        t["warn"] = warn
    if store:
        t["store"] = store
    TWEAKS.append(t)


# ---------------------------------------------------------------- constants
GC = r"HKCU\System\GameConfigStore"
MM = r"HKLM\SOFTWARE\Microsoft\Windows NT\CurrentVersion\Multimedia\SystemProfile"
ADV = r"HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Advanced"
CDM = r"HKCU\Software\Microsoft\Windows\CurrentVersion\ContentDeliveryManager"
POL = r"HKLM\SOFTWARE\Policies\Microsoft\Windows"
UPOL = r"HKCU\Software\Policies\Microsoft\Windows"
FS = r"HKLM\SYSTEM\CurrentControlSet\Control\FileSystem"
DESK = r"HKCU\Control Panel\Desktop"

SUB_PROCESSOR = "54533251-82be-4824-96c1-47b60b740d00"
CPMINCORES = "0cc5b647-c1df-4637-891a-dec35c318583"
PROCTHROTTLEMIN = "893dee8e-2bef-41e0-89c6-b55d0929964c"
SUB_USB = "2a737441-1930-4402-8d77-b2bebba308a3"
USBSUSPEND = "48e6b7a6-50f5-4782-a5d4-53bb8f07e226"
SUB_PCIE = "501a4d13-42af-4429-9fd1-a8218c268e20"
ASPM = "ee12f906-d277-404b-b6da-e5fa1a576df5"
SUB_DISK = "0012ee47-9041-4b5d-9b77-535fba8b1442"
DISKIDLE = "6738e2c4-e8a5-4a42-b16a-e040e769756e"

W11 = {"win11": True}

# ============================================================ FPS & LATENCY
T("game_mode", "fps", "safe",
  L("Enable Game Mode", "Включить игровой режим", "Увімкнути ігровий режим", "Włącz tryb gry"),
  L("Lets Windows give your game priority for CPU and GPU time and pauses driver installs and update notifications while you play.",
    "Windows отдаёт игре приоритет на процессор и видеокарту и откладывает установку драйверов и уведомления об обновлениях во время игры.",
    "Windows надає грі пріоритет на процесор і відеокарту та відкладає встановлення драйверів і сповіщення про оновлення під час гри.",
    "Windows daje grze priorytet do procesora i karty graficznej oraz wstrzymuje instalację sterowników i powiadomienia o aktualizacjach podczas gry."),
  [reg(r"HKCU\Software\Microsoft\GameBar", "AutoGameModeEnabled", "dword", 1),
   reg(r"HKCU\Software\Microsoft\GameBar", "AllowAutoGameMode", "dword", 1)],
  rec=True, imp=2)

T("game_dvr_off", "fps", "safe",
  L("Disable background game recording", "Отключить фоновую запись игр", "Вимкнути фонову запис ігор", "Wyłącz nagrywanie gier w tle"),
  L("Turns off Game DVR, which silently records the last minutes of gameplay and costs FPS on weaker GPUs. The Game Bar overlay keeps working.",
    "Отключает Game DVR, который незаметно записывает последние минуты игры и отнимает FPS на слабых видеокартах. Оверлей Game Bar продолжит работать.",
    "Вимикає Game DVR, який непомітно записує останні хвилини гри та забирає FPS на слабких відеокартах. Оверлей Game Bar і далі працюватиме.",
    "Wyłącza Game DVR, który po cichu nagrywa ostatnie minuty rozgrywki i obniża FPS na słabszych kartach. Nakładka Game Bar nadal działa."),
  [reg(GC, "GameDVR_Enabled", "dword", 0, 1),
   reg(r"HKCU\Software\Microsoft\Windows\CurrentVersion\GameDVR", "AppCaptureEnabled", "dword", 0),
   reg(POL + r"\GameDVR", "AllowGameDVR", "dword", 0)],
  rec=True, imp=3)

T("fso_off", "fps", "moderate",
  L("Disable fullscreen optimizations", "Отключить оптимизацию во весь экран", "Вимкнути оптимізацію на весь екран", "Wyłącz optymalizacje pełnoekranowe"),
  L("Forces true exclusive fullscreen in older DirectX 9–11 games. Can lower input lag, but may break fast Alt+Tab and overlays in some titles.",
    "Включает настоящий эксклюзивный полноэкранный режим в старых играх на DirectX 9–11. Может снизить задержку ввода, но в некоторых играх ломает быстрый Alt+Tab и оверлеи.",
    "Вмикає справжній ексклюзивний повноекранний режим у старих іграх на DirectX 9–11. Може зменшити затримку введення, але в деяких іграх ламає швидкий Alt+Tab і оверлеї.",
    "Wymusza prawdziwy tryb wyłącznego pełnego ekranu w starszych grach DirectX 9–11. Może zmniejszyć opóźnienie, ale w niektórych grach psuje szybki Alt+Tab i nakładki."),
  [reg(GC, "GameDVR_FSEBehaviorMode", "dword", 2),
   reg(GC, "GameDVR_FSEBehavior", "dword", 2),
   reg(GC, "GameDVR_HonorUserFSEBehaviorMode", "dword", 1),
   reg(GC, "GameDVR_DXGIHonorFSEWindowsCompatible", "dword", 1)],
  imp=2, restart="signout")

T("sys_responsiveness", "fps", "safe",
  L("Reserve less CPU for background tasks", "Меньше процессора для фоновых задач", "Менше процесора для фонових задач", "Mniej procesora dla zadań w tle"),
  L("Windows keeps 20% of CPU time for background multimedia tasks. This lowers the reserve to 10% so games get more of the processor.",
    "Windows держит 20% процессорного времени для фоновых мультимедиа-задач. Твик снижает резерв до 10%, чтобы игре доставалось больше.",
    "Windows тримає 20% процесорного часу для фонових мультимедійних задач. Твік зменшує резерв до 10%, щоб грі діставалося більше.",
    "Windows rezerwuje 20% czasu procesora dla multimediów w tle. Ta zmiana obniża rezerwę do 10%, więc gra dostaje więcej mocy."),
  [reg(MM, "SystemResponsiveness", "dword", 10, 20)],
  rec=True, imp=2)

T("games_mmcss", "fps", "safe",
  L("Raise game scheduling priority", "Повысить приоритет игр в планировщике", "Підвищити пріоритет ігор у планувальнику", "Podnieś priorytet gier w harmonogramie"),
  L("Tells the multimedia scheduler (MMCSS) to treat games as high priority for CPU, GPU and disk access.",
    "Мультимедийный планировщик (MMCSS) начинает считать игры задачами высокого приоритета для процессора, видеокарты и диска.",
    "Мультимедійний планувальник (MMCSS) починає вважати ігри задачами високого пріоритету для процесора, відеокарти та диска.",
    "Harmonogram multimediów (MMCSS) traktuje gry jako zadania o wysokim priorytecie dla procesora, GPU i dysku."),
  [reg(MM + r"\Tasks\Games", "GPU Priority", "dword", 8, 8),
   reg(MM + r"\Tasks\Games", "Priority", "dword", 6, 2),
   reg(MM + r"\Tasks\Games", "Scheduling Category", "sz", "High", "Medium"),
   reg(MM + r"\Tasks\Games", "SFIO Priority", "sz", "High", "Normal")],
  rec=True, imp=2)

T("win32_priority", "fps", "safe",
  L("Prioritize the active window", "Приоритет активному окну", "Пріоритет активному вікну", "Priorytet dla aktywnego okna"),
  L("Sets processor scheduling to short, variable time slices with the maximum boost for the foreground app — your game.",
    "Настраивает планировщик на короткие переменные кванты с максимальным ускорением для программы на переднем плане — вашей игры.",
    "Налаштовує планувальник на короткі змінні кванти з максимальним прискоренням для програми на передньому плані — вашої гри.",
    "Ustawia krótkie, zmienne kwanty czasu procesora z maksymalnym wzmocnieniem dla aplikacji na pierwszym planie — twojej gry."),
  [reg(r"HKLM\SYSTEM\CurrentControlSet\Control\PriorityControl", "Win32PrioritySeparation", "dword", 38, 2)],
  rec=True, imp=1)

T("power_throttling_off", "fps", "safe",
  L("Disable power throttling", "Отключить Power Throttling", "Вимкнути Power Throttling", "Wyłącz ograniczanie mocy"),
  L("Stops Windows from moving apps it thinks are in the background onto slow, power-saving CPU states. Uses a bit more power.",
    "Windows перестаёт переводить «фоновые», по её мнению, программы в медленные энергосберегающие режимы процессора. Немного повышает энергопотребление.",
    "Windows перестає переводити «фонові», на її думку, програми в повільні енергоощадні режими процесора. Трохи збільшує енергоспоживання.",
    "Windows przestaje przenosić aplikacje uznane za działające w tle na wolne, oszczędne stany procesora. Zużywa nieco więcej energii."),
  [reg(r"HKLM\SYSTEM\CurrentControlSet\Control\Power\PowerThrottling", "PowerThrottlingOff", "dword", 1)],
  rec=True, imp=2, tags=["power"], when={"desktop": True})

T("timer_res_global", "fps", "moderate",
  L("Global timer resolution", "Глобальное разрешение таймера", "Глобальна роздільність таймера", "Globalna rozdzielczość zegara"),
  L("Restores the classic behaviour where a high-precision timer request from any app applies system-wide. Smoother frame pacing in some games, slightly higher power use.",
    "Возвращает классическое поведение, когда запрос точного таймера от любой программы действует на всю систему. Ровнее кадры в некоторых играх, чуть выше энергопотребление.",
    "Повертає класичну поведінку, коли запит точного таймера від будь-якої програми діє на всю систему. Рівніші кадри в деяких іграх, трохи вище енергоспоживання.",
    "Przywraca klasyczne działanie, w którym żądanie precyzyjnego zegara przez dowolną aplikację działa w całym systemie. Równiejsze klatki w części gier, nieco większy pobór mocy."),
  [reg(r"HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\kernel", "GlobalTimerResolutionRequests", "dword", 1)],
  imp=2, restart="pc", tags=["power", "win11"], req=W11)

T("background_apps_off", "fps", "moderate",
  L("Stop Store apps running in background", "Запретить фоновую работу приложений", "Заборонити фонову роботу застосунків", "Zablokuj aplikacje w tle"),
  L("Microsoft Store apps can no longer run when closed. Frees RAM and CPU; some apps (Phone Link, Mail) stop sending notifications until opened.",
    "Приложения из Microsoft Store больше не работают после закрытия. Освобождает память и процессор; некоторые приложения (Связь с телефоном, Почта) не присылают уведомления, пока их не открыть.",
    "Застосунки з Microsoft Store більше не працюють після закриття. Звільняє пам'ять і процесор; деякі застосунки (Зв'язок із телефоном, Пошта) не надсилають сповіщень, доки їх не відкрити.",
    "Aplikacje ze Sklepu Microsoft nie działają po zamknięciu. Zwalnia RAM i procesor; niektóre aplikacje (Łącze z telefonem, Poczta) nie wysyłają powiadomień, dopóki ich nie otworzysz."),
  [reg(POL + r"\AppPrivacy", "LetAppsRunInBackground", "dword", 2),
   reg(r"HKCU\Software\Microsoft\Windows\CurrentVersion\BackgroundAccessApplications", "GlobalUserDisabled", "dword", 1)],
  imp=2)

T("visual_effects", "fps", "safe",
  L("Reduce animations and visual effects", "Уменьшить анимации и визуальные эффекты", "Зменшити анімації та візуальні ефекти", "Ogranicz animacje i efekty wizualne"),
  L("Turns off window, taskbar and list animations and Aero Peek. The desktop feels snappier and the GPU does less work on entry-level PCs.",
    "Отключает анимации окон, панели задач и списков, а также Aero Peek. Интерфейс отзывчивее, а видеокарта меньше нагружена на бюджетных ПК.",
    "Вимикає анімації вікон, панелі завдань і списків, а також Aero Peek. Інтерфейс швидший, а відеокарта менше навантажена на бюджетних ПК.",
    "Wyłącza animacje okien, paska zadań i list oraz Aero Peek. Pulpit działa żwawiej, a GPU ma mniej pracy na słabszych komputerach."),
  [reg(DESK + r"\WindowMetrics", "MinAnimate", "sz", "0", "1"),
   reg(ADV, "TaskbarAnimations", "dword", 0),
   reg(ADV, "ListviewAlphaSelect", "dword", 0),
   reg(ADV, "ListviewShadow", "dword", 0),
   reg(r"HKCU\Software\Microsoft\Windows\DWM", "EnableAeroPeek", "dword", 0),
   reg(r"HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\VisualEffects", "VisualFXSetting", "dword", 3)],
  rec=True, imp=1, restart="signout")

T("transparency_off", "fps", "safe",
  L("Disable transparency effects", "Отключить прозрачность", "Вимкнути прозорість", "Wyłącz efekty przezroczystości"),
  L("Removes the blur and transparency from the taskbar, Start and windows, saving GPU work on integrated and entry-level graphics.",
    "Убирает размытие и прозрачность панели задач, меню «Пуск» и окон — меньше нагрузки на встроенную и бюджетную графику.",
    "Прибирає розмиття та прозорість панелі завдань, меню «Пуск» і вікон — менше навантаження на вбудовану й бюджетну графіку.",
    "Usuwa rozmycie i przezroczystość paska zadań, menu Start i okien, odciążając zintegrowaną i budżetową grafikę."),
  [reg(r"HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize", "EnableTransparency", "dword", 0, 1)],
  rec=True, imp=1)

T("edge_background_off", "fps", "safe",
  L("Stop Edge running in background", "Запретить Edge работать в фоне", "Заборонити Edge працювати у фоні", "Zablokuj Edge w tle"),
  L("Disables Edge Startup Boost and background mode, which keep Edge processes in memory even when it's closed. Edge will show 'managed by your organization'.",
    "Отключает «Ускорение запуска» и фоновый режим Edge, из-за которых его процессы висят в памяти даже после закрытия. В Edge появится надпись «управляется организацией».",
    "Вимикає «Прискорення запуску» та фоновий режим Edge, через які його процеси залишаються в пам'яті навіть після закриття. В Edge з'явиться напис «керується організацією».",
    "Wyłącza przyspieszenie uruchamiania i tryb tła Edge, przez które jego procesy zostają w pamięci po zamknięciu. Edge pokaże komunikat „zarządzane przez organizację”."),
  [reg(r"HKLM\SOFTWARE\Policies\Microsoft\Edge", "StartupBoostEnabled", "dword", 0),
   reg(r"HKLM\SOFTWARE\Policies\Microsoft\Edge", "BackgroundModeEnabled", "dword", 0)],
  rec=True, imp=1)

T("widgets_off", "fps", "safe",
  L("Disable Widgets and News", "Отключить виджеты и новости", "Вимкнути віджети та новини", "Wyłącz widżety i wiadomości"),
  L("Turns off the Widgets board (Windows 11) and News and Interests (Windows 10), which run web processes in the background all the time.",
    "Отключает панель виджетов (Windows 11) и «Новости и интересы» (Windows 10), которые постоянно держат в фоне веб-процессы.",
    "Вимикає панель віджетів (Windows 11) і «Новини та інтереси» (Windows 10), які постійно тримають у фоні вебпроцеси.",
    "Wyłącza tablicę widżetów (Windows 11) oraz Wiadomości i zainteresowania (Windows 10), które stale utrzymują procesy w tle."),
  [reg(r"HKLM\SOFTWARE\Policies\Microsoft\Dsh", "AllowNewsAndInterests", "dword", 0),
   reg(POL + r"\Windows Feeds", "EnableFeeds", "dword", 0)],
  rec=True, imp=1, restart="explorer")

T("sysmain_off", "fps", "moderate",
  L("Disable SysMain (Superfetch)", "Отключить SysMain (Superfetch)", "Вимкнути SysMain (Superfetch)", "Wyłącz SysMain (Superfetch)"),
  L("SysMain preloads apps into RAM and constantly reads the disk. On an SSD it brings little benefit and can cause stutter on PCs with 8 GB of RAM. Keep it on HDD systems.",
    "SysMain заранее загружает программы в память и постоянно читает диск. На SSD пользы почти нет, а на ПК с 8 ГБ памяти он может вызывать фризы. На HDD лучше оставить.",
    "SysMain заздалегідь завантажує програми в пам'ять і постійно читає диск. На SSD користі майже немає, а на ПК з 8 ГБ пам'яті він може спричиняти фризи. На HDD краще залишити.",
    "SysMain wstępnie ładuje aplikacje do RAM i stale czyta dysk. Na SSD daje niewiele, a przy 8 GB RAM może powodować przycięcia. Na dyskach HDD lepiej go zostawić."),
  [svc("SysMain", "disabled", "auto")],
  imp=2, when={"ssd": True})

T("search_index_off", "fps", "moderate",
  L("Disable search indexing", "Отключить индексирование поиска", "Вимкнути індексування пошуку", "Wyłącz indeksowanie wyszukiwania"),
  L("Stops the Windows Search service from scanning your files in the background. Start still finds apps, but searching file contents gets slower.",
    "Служба поиска Windows перестаёт сканировать файлы в фоне. «Пуск» по-прежнему находит программы, но поиск по содержимому файлов станет медленнее.",
    "Служба пошуку Windows перестає сканувати файли у фоні. «Пуск» і далі знаходить програми, але пошук за вмістом файлів стане повільнішим.",
    "Usługa Windows Search przestaje skanować pliki w tle. Menu Start nadal znajduje aplikacje, ale wyszukiwanie w treści plików będzie wolniejsze."),
  [svc("WSearch", "disabled", "delayed-auto")],
  imp=2)

T("memory_compression_off", "fps", "moderate",
  L("Disable memory compression", "Отключить сжатие памяти", "Вимкнути стиснення пам'яті", "Wyłącz kompresję pamięci"),
  L("Saves the CPU work of compressing RAM pages. Only worth it with 16 GB of RAM or more — with 8 GB, compression helps you.",
    "Экономит работу процессора на сжатие страниц памяти. Имеет смысл только при 16 ГБ памяти и больше — при 8 ГБ сжатие вам помогает.",
    "Економить роботу процесора на стиснення сторінок пам'яті. Має сенс лише за 16 ГБ пам'яті й більше — за 8 ГБ стиснення вам допомагає.",
    "Oszczędza pracę procesora na kompresji stron pamięci. Opłaca się tylko przy 16 GB RAM lub więcej — przy 8 GB kompresja pomaga."),
  [ps("Disable-MMAgent -MemoryCompression",
      "Enable-MMAgent -MemoryCompression",
      "if((Get-MMAgent).MemoryCompression){'0'}else{'1'}")],
  imp=1, restart="pc", when={"ram16": True})

T("ntfs_last_access", "fps", "safe",
  L("Stop last-access timestamps", "Отключить метки последнего доступа", "Вимкнути мітки останнього доступу", "Wyłącz znaczniki ostatniego dostępu"),
  L("NTFS writes a timestamp every time a file is read. Turning it off removes thousands of small disk writes when games load assets.",
    "NTFS записывает метку времени при каждом чтении файла. Отключение убирает тысячи мелких записей на диск, когда игра загружает ресурсы.",
    "NTFS записує мітку часу під час кожного читання файлу. Вимкнення прибирає тисячі дрібних записів на диск, коли гра завантажує ресурси.",
    "NTFS zapisuje znacznik czasu przy każdym odczycie pliku. Wyłączenie usuwa tysiące drobnych zapisów, gdy gra wczytuje zasoby."),
  [reg(FS, "NtfsDisableLastAccessUpdate", "dword", 0x80000001, 0x80000002)],
  rec=True, imp=1)

T("ntfs_8dot3_off", "fps", "safe",
  L("Disable short 8.3 file names", "Отключить короткие имена 8.3", "Вимкнути короткі імена 8.3", "Wyłącz krótkie nazwy 8.3"),
  L("Stops NTFS from creating legacy MS-DOS names for every new file, making file creation in big game folders faster.",
    "NTFS перестаёт создавать устаревшие имена MS-DOS для каждого нового файла — создание файлов в больших папках игр ускоряется.",
    "NTFS перестає створювати застарілі імена MS-DOS для кожного нового файлу — створення файлів у великих теках ігор пришвидшується.",
    "NTFS przestaje tworzyć przestarzałe nazwy MS-DOS dla każdego nowego pliku, co przyspiesza tworzenie plików w dużych folderach gier."),
  [reg(FS, "NtfsDisable8dot3NameCreation", "dword", 1, 2)],
  imp=1)

T("trim_on", "fps", "safe",
  L("Make sure SSD TRIM is on", "Включить TRIM для SSD", "Увімкнути TRIM для SSD", "Upewnij się, że TRIM jest włączony"),
  L("TRIM lets your SSD clean up deleted blocks so write speed doesn't drop over time. This makes sure it isn't switched off.",
    "TRIM позволяет SSD очищать удалённые блоки, чтобы скорость записи не падала со временем. Твик гарантирует, что он не отключён.",
    "TRIM дозволяє SSD очищати видалені блоки, щоб швидкість запису не падала з часом. Твік гарантує, що його не вимкнено.",
    "TRIM pozwala SSD porządkować usunięte bloki, by prędkość zapisu nie spadała z czasem. Ta opcja pilnuje, by był włączony."),
  [reg(FS, "DisableDeleteNotification", "dword", 0, 0)],
  rec=True, imp=1, when={"ssd": True})

# ============================================================ POWER
T("max_power_plan", "power", "safe",
  L("Ultimate Performance power plan", "Схема «Максимальная производительность»", "Схема «Максимальна продуктивність»", "Plan zasilania Najwyższa wydajność"),
  L("Unlocks and activates the hidden Ultimate Performance plan: no CPU downclocking delays, no device sleep. Falls back to High Performance if unavailable.",
    "Открывает и включает скрытую схему «Максимальная производительность»: без задержек снижения частоты и усыпления устройств. Если она недоступна — «Высокая производительность».",
    "Відкриває та вмикає приховану схему «Максимальна продуктивність»: без затримок зниження частоти та присипляння пристроїв. Якщо вона недоступна — «Висока продуктивність».",
    "Odblokowuje i włącza ukryty plan Najwyższa wydajność: bez opóźnień obniżania taktowania i usypiania urządzeń. Gdy jest niedostępny, używa planu Wysoka wydajność."),
  [plan()],
  rec=True, imp=3, tags=["power"], when={"desktop": True})

T("core_parking_off", "power", "safe",
  L("Disable CPU core parking", "Отключить парковку ядер", "Вимкнути паркування ядер", "Wyłącz parkowanie rdzeni"),
  L("Keeps every CPU core awake on the active power plan, so the game never waits for a parked core to wake up. Helps 1% low FPS.",
    "Все ядра процессора остаются активными в текущей схеме питания, и игре не приходится ждать пробуждения «запаркованного» ядра. Улучшает 1% low FPS.",
    "Усі ядра процесора залишаються активними в поточній схемі живлення, і грі не доводиться чекати пробудження «запаркованого» ядра. Покращує 1% low FPS.",
    "Wszystkie rdzenie pozostają aktywne w bieżącym planie zasilania, więc gra nie czeka na wybudzenie zaparkowanego rdzenia. Poprawia 1% low FPS."),
  [power(SUB_PROCESSOR, CPMINCORES, 100, 10)],
  rec=True, imp=2, tags=["power"], when={"desktop": True})

T("cpu_min_100", "power", "moderate",
  L("Keep CPU at full clock", "Держать процессор на полной частоте", "Тримати процесор на повній частоті", "Utrzymuj pełne taktowanie CPU"),
  L("Sets the minimum processor state to 100% while plugged in. Removes clock ramp-up delay, but the CPU runs warmer at idle.",
    "Минимальное состояние процессора 100% при питании от сети. Убирает задержку разгона частоты, но в простое процессор горячее.",
    "Мінімальний стан процесора 100% під час живлення від мережі. Прибирає затримку розгону частоти, але в простої процесор гарячіший.",
    "Ustawia minimalny stan procesora na 100% przy zasilaniu sieciowym. Usuwa opóźnienie podbijania taktowania, ale CPU jest cieplejszy w spoczynku."),
  [power(SUB_PROCESSOR, PROCTHROTTLEMIN, 100, 5)],
  imp=1, tags=["power"], when={"desktop": True})

T("usb_suspend_off", "power", "safe",
  L("Disable USB selective suspend", "Отключить временное отключение USB", "Вимкнути тимчасове вимкнення USB", "Wyłącz selektywne wstrzymywanie USB"),
  L("Prevents Windows from putting USB ports to sleep, which can cause mouse and headset dropouts or a delay on the first input.",
    "Windows перестаёт усыплять USB-порты — это может вызывать пропадание мыши и гарнитуры или задержку первого нажатия.",
    "Windows перестає присипляти USB-порти — це може спричиняти зникнення миші й гарнітури або затримку першого натискання.",
    "Windows przestaje usypiać porty USB, co może powodować zanikanie myszy i słuchawek lub opóźnienie pierwszego kliknięcia."),
  [power(SUB_USB, USBSUSPEND, 0, 1, dc=True)],
  rec=True, imp=1)

T("pcie_aspm_off", "power", "safe",
  L("Disable PCIe link power saving", "Отключить энергосбережение PCIe", "Вимкнути енергозбереження PCIe", "Wyłącz oszczędzanie energii PCIe"),
  L("Keeps the PCI Express link to your graphics card and NVMe drive at full power, avoiding wake-up latency.",
    "Канал PCI Express к видеокарте и NVMe-диску работает на полной мощности без задержек на пробуждение.",
    "Канал PCI Express до відеокарти та NVMe-диска працює на повній потужності без затримок на пробудження.",
    "Łącze PCI Express do karty graficznej i dysku NVMe działa z pełną mocą, bez opóźnień wybudzania."),
  [power(SUB_PCIE, ASPM, 0, 1)],
  rec=True, imp=1, tags=["power"], when={"desktop": True})

T("disk_sleep_off", "power", "safe",
  L("Never turn off hard drives", "Не отключать жёсткие диски", "Не вимикати жорсткі диски", "Nigdy nie wyłączaj dysków"),
  L("Stops drives from spinning down while plugged in. Prevents the multi-second freeze when a game reads from a sleeping HDD.",
    "Диски не останавливаются при питании от сети. Убирает зависание на несколько секунд, когда игра читает с уснувшего HDD.",
    "Диски не зупиняються під час живлення від мережі. Прибирає зависання на кілька секунд, коли гра читає із заснулого HDD.",
    "Dyski nie zatrzymują się przy zasilaniu sieciowym. Usuwa kilkusekundowe zawieszenie, gdy gra czyta z uśpionego HDD."),
  [power(SUB_DISK, DISKIDLE, 0, 1200)],
  rec=True, imp=1)

T("hibernate_off", "power", "safe",
  L("Disable hibernation", "Отключить гибернацию", "Вимкнути гібернацію", "Wyłącz hibernację"),
  L("Deletes hiberfil.sys (several GB on the system drive) and disables Fast Startup with it. Sleep still works.",
    "Удаляет hiberfil.sys (несколько ГБ на системном диске) и вместе с этим отключает быстрый запуск. Спящий режим продолжает работать.",
    "Видаляє hiberfil.sys (кілька ГБ на системному диску) і разом із цим вимикає швидкий запуск. Режим сну й далі працює.",
    "Usuwa hiberfil.sys (kilka GB na dysku systemowym) i wyłącza przy tym szybkie uruchamianie. Uśpienie nadal działa."),
  [execop(["powercfg", "/hibernate", "off"], ["powercfg", "/hibernate", "on"],
          {"k": r"HKLM\SYSTEM\CurrentControlSet\Control\Power", "n": "HibernateEnabled", "v": 0})],
  rec=True, imp=1, when={"desktop": True})

T("fast_startup_off", "power", "safe",
  L("Disable Fast Startup", "Отключить быстрый запуск", "Вимкнути швидкий запуск", "Wyłącz szybkie uruchamianie"),
  L("Shutdown becomes a real shutdown, so drivers and memory start clean every boot. Fixes many 'only works after restart' problems.",
    "Завершение работы становится настоящим выключением — драйверы и память стартуют начисто. Решает многие проблемы вида «работает только после перезагрузки».",
    "Завершення роботи стає справжнім вимкненням — драйвери й пам'ять стартують начисто. Вирішує багато проблем на кшталт «працює лише після перезавантаження».",
    "Zamykanie systemu staje się prawdziwym wyłączeniem, więc sterowniki i pamięć startują od zera. Rozwiązuje wiele problemów typu „działa tylko po restarcie”."),
  [reg(r"HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Power", "HiberbootEnabled", "dword", 0, 1)],
  rec=True, imp=1)

# ============================================================ GRAPHICS
T("hags_on", "gpu", "safe",
  L("Hardware-accelerated GPU scheduling", "Аппаратное планирование GPU", "Апаратне планування GPU", "Sprzętowe planowanie GPU"),
  L("Lets the graphics card manage its own memory queue, cutting CPU overhead and latency. Needs GTX 1000 / RX 5000 or newer; results vary, so test your games.",
    "Видеокарта сама управляет очередью памяти, снижая нагрузку на процессор и задержку. Нужна GTX 1000 / RX 5000 или новее; эффект зависит от игры — проверьте.",
    "Відеокарта сама керує чергою пам'яті, зменшуючи навантаження на процесор і затримку. Потрібна GTX 1000 / RX 5000 або новіша; ефект залежить від гри — перевірте.",
    "Karta graficzna sama zarządza kolejką pamięci, zmniejszając obciążenie CPU i opóźnienie. Wymaga GTX 1000 / RX 5000 lub nowszej; efekt zależy od gry."),
  [reg(r"HKLM\SYSTEM\CurrentControlSet\Control\GraphicsDrivers", "HwSchMode", "dword", 2)],
  imp=2, restart="pc")

T("windowed_opt", "gpu", "safe",
  L("Optimizations for windowed games", "Оптимизация игр в окне", "Оптимізація ігор у вікні", "Optymalizacje gier w oknie"),
  L("Uses the modern flip presentation model for DirectX 10/11 games in borderless and windowed mode — lower latency and support for VRR.",
    "Современная модель вывода flip для игр на DirectX 10/11 в оконном и безрамочном режиме — ниже задержка и поддержка VRR.",
    "Сучасна модель виведення flip для ігор на DirectX 10/11 у віконному та безрамковому режимі — нижча затримка й підтримка VRR.",
    "Nowoczesny model prezentacji flip dla gier DirectX 10/11 w oknie i bez ramki — mniejsze opóźnienie i obsługa VRR."),
  [dxkv("SwapEffectUpgradeEnable", "1")],
  rec=True, imp=2, req=W11, tags=["win11"])

T("mpo_off", "gpu", "moderate",
  L("Disable Multiplane Overlay", "Отключить Multiplane Overlay", "Вимкнути Multiplane Overlay", "Wyłącz Multiplane Overlay"),
  L("A fix recommended by NVIDIA for flickering, black screens and stutter in borderless games and when a video plays on a second monitor.",
    "Исправление, рекомендованное NVIDIA, против мерцания, чёрных экранов и фризов в безрамочных играх и при видео на втором мониторе.",
    "Виправлення, рекомендоване NVIDIA, проти мерехтіння, чорних екранів і фризів у безрамкових іграх та під час відео на другому моніторі.",
    "Poprawka zalecana przez NVIDIA na migotanie, czarne ekrany i przycięcia w grach bez ramki i przy wideo na drugim monitorze."),
  [reg(r"HKLM\SOFTWARE\Microsoft\Windows\Dwm", "OverlayTestMode", "dword", 5)],
  imp=1, restart="pc")

MSI_KEY = r"Device Parameters\Interrupt Management\MessageSignaledInterruptProperties"
T("msi_gpu", "gpu", "moderate",
  L("Message Signaled Interrupts for GPU", "MSI-прерывания для видеокарты", "MSI-переривання для відеокарти", "Przerwania MSI dla karty graficznej"),
  L("Switches the graphics card to MSI mode so interrupts go straight to the CPU instead of through shared lines — lower DPC latency.",
    "Переводит видеокарту в режим MSI: прерывания идут напрямую в процессор, а не через общие линии — ниже DPC-задержка.",
    "Переводить відеокарту в режим MSI: переривання йдуть напряму в процесор, а не через спільні лінії — нижча DPC-затримка.",
    "Przełącza kartę graficzną w tryb MSI, więc przerwania trafiają prosto do CPU zamiast przez wspólne linie — mniejsze opóźnienie DPC."),
  [ps(r"""
$bk = @{}
foreach($d in (Get-CimInstance Win32_VideoController | Where-Object { $_.PNPDeviceID -like 'PCI\*' })){
  $p = "HKLM:\SYSTEM\CurrentControlSet\Enum\$($d.PNPDeviceID)\%s"
  $v = (Get-ItemProperty -Path $p -Name MSISupported -ErrorAction SilentlyContinue).MSISupported
  if($null -eq $v){ $bk[$d.PNPDeviceID] = -1 } else { $bk[$d.PNPDeviceID] = [int]$v }
  if(-not (Test-Path $p)){ New-Item -Path $p -Force | Out-Null }
  New-ItemProperty -Path $p -Name MSISupported -Value 1 -PropertyType DWord -Force | Out-Null
}
'@@BACKUP@@' + ($bk | ConvertTo-Json -Compress)
""" % MSI_KEY, r"""
if([string]::IsNullOrEmpty($LarikBackup)){ return }
$bk = $LarikBackup | ConvertFrom-Json
foreach($prop in $bk.PSObject.Properties){
  $p = "HKLM:\SYSTEM\CurrentControlSet\Enum\$($prop.Name)\%s"
  if([int]$prop.Value -eq -1){ Remove-ItemProperty -Path $p -Name MSISupported -ErrorAction SilentlyContinue }
  else { New-ItemProperty -Path $p -Name MSISupported -Value ([int]$prop.Value) -PropertyType DWord -Force | Out-Null }
}
""" % MSI_KEY, r"""
$devs = @(Get-CimInstance Win32_VideoController | Where-Object { $_.PNPDeviceID -like 'PCI\*' })
if($devs.Count -eq 0){ 'na'; return }
$all = $true
foreach($d in $devs){
  $p = "HKLM:\SYSTEM\CurrentControlSet\Enum\$($d.PNPDeviceID)\%s"
  $v = (Get-ItemProperty -Path $p -Name MSISupported -ErrorAction SilentlyContinue).MSISupported
  if($v -ne 1){ $all = $false }
}
if($all){'1'}else{'0'}
""" % MSI_KEY)],
  imp=2, restart="pc")

T("nvidia_telemetry_off", "gpu", "safe",
  L("Disable NVIDIA telemetry", "Отключить телеметрию NVIDIA", "Вимкнути телеметрію NVIDIA", "Wyłącz telemetrię NVIDIA"),
  L("Stops the NVIDIA Telemetry Container service that older drivers install to send usage data.",
    "Отключает службу NVIDIA Telemetry Container, которую ставят старые драйверы для отправки статистики.",
    "Вимикає службу NVIDIA Telemetry Container, яку встановлюють старі драйвери для надсилання статистики.",
    "Wyłącza usługę NVIDIA Telemetry Container, którą starsze sterowniki instalują do wysyłania statystyk."),
  [svc("NvTelemetryContainer", "disabled", "auto")],
  rec=True, imp=1, when={"nvidia": True})

# ============================================================ INPUT
T("mouse_accel_off", "input", "safe",
  L("Disable mouse acceleration", "Отключить ускорение мыши", "Вимкнути прискорення миші", "Wyłącz przyspieszenie myszy"),
  L("Turns off 'Enhance pointer precision' so the crosshair moves the same distance no matter how fast you flick — essential for consistent aim.",
    "Отключает «Повышенную точность указателя»: прицел смещается на одно и то же расстояние при любой скорости движения — основа стабильного аима.",
    "Вимикає «Підвищену точність вказівника»: приціл зміщується на ту саму відстань за будь-якої швидкості руху — основа стабільного аіму.",
    "Wyłącza „Popraw precyzję wskaźnika”, więc celownik przesuwa się o tyle samo niezależnie od szybkości ruchu — podstawa powtarzalnego celowania."),
  [reg(r"HKCU\Control Panel\Mouse", "MouseSpeed", "sz", "0", "1"),
   reg(r"HKCU\Control Panel\Mouse", "MouseThreshold1", "sz", "0", "6"),
   reg(r"HKCU\Control Panel\Mouse", "MouseThreshold2", "sz", "0", "10")],
  rec=True, imp=2)

T("keyboard_fast", "input", "safe",
  L("Fastest keyboard repeat", "Максимальная скорость повтора клавиш", "Максимальна швидкість повтору клавіш", "Najszybsze powtarzanie klawiszy"),
  L("Sets the shortest delay before a held key starts repeating and the fastest repeat rate.",
    "Минимальная задержка перед повтором удерживаемой клавиши и максимальная скорость повтора.",
    "Мінімальна затримка перед повтором утримуваної клавіші та максимальна швидкість повтору.",
    "Najkrótsze opóźnienie przed powtarzaniem przytrzymanego klawisza i najszybsze tempo powtarzania."),
  [reg(r"HKCU\Control Panel\Keyboard", "KeyboardDelay", "sz", "0", "1"),
   reg(r"HKCU\Control Panel\Keyboard", "KeyboardSpeed", "sz", "31", "31")],
  imp=1, restart="signout")

T("sticky_keys_off", "input", "safe",
  L("Disable Sticky Keys pop-ups", "Отключить окна залипания клавиш", "Вимкнути вікна залипання клавіш", "Wyłącz okna klawiszy trwałych"),
  L("Pressing Shift five times or holding it no longer throws you out of the game with a Sticky / Filter / Toggle Keys dialog.",
    "Пятикратное нажатие или удержание Shift больше не выкидывает из игры окном залипания, фильтрации или озвучивания клавиш.",
    "П'ятикратне натискання або утримання Shift більше не викидає з гри вікном залипання, фільтрації чи озвучення клавіш.",
    "Pięciokrotne naciśnięcie lub przytrzymanie Shift nie wyrzuca już z gry oknem klawiszy trwałych, filtrowania ani przełączających."),
  [reg(r"HKCU\Control Panel\Accessibility\StickyKeys", "Flags", "sz", "506", "510"),
   reg(r"HKCU\Control Panel\Accessibility\Keyboard Response", "Flags", "sz", "122", "126"),
   reg(r"HKCU\Control Panel\Accessibility\ToggleKeys", "Flags", "sz", "58", "62")],
  rec=True, imp=1, restart="signout")

# ============================================================ NETWORK
TCPIF = r"HKLM\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces"
T("nagle_off", "network", "safe",
  L("Disable Nagle's algorithm", "Отключить алгоритм Нейгла", "Вимкнути алгоритм Нейгла", "Wyłącz algorytm Nagle'a"),
  L("Sends small TCP packets immediately instead of batching them. Lowers ping in games that use TCP (many MMOs and MOBAs).",
    "Мелкие TCP-пакеты отправляются сразу, а не копятся. Снижает пинг в играх, работающих по TCP (многие MMO и MOBA).",
    "Дрібні TCP-пакети надсилаються одразу, а не накопичуються. Знижує пінг в іграх, що працюють через TCP (багато MMO і MOBA).",
    "Małe pakiety TCP są wysyłane od razu zamiast grupowania. Obniża ping w grach korzystających z TCP (wiele MMO i MOBA)."),
  [regeach(TCPIF, ["DhcpIPAddress", "IPAddress"], "TcpAckFrequency", "dword", 1),
   regeach(TCPIF, ["DhcpIPAddress", "IPAddress"], "TCPNoDelay", "dword", 1)],
  rec=True, imp=2, restart="pc")

T("net_throttling_off", "network", "safe",
  L("Disable network throttling", "Отключить ограничение сети", "Вимкнути обмеження мережі", "Wyłącz dławienie sieci"),
  L("Removes the limit Windows puts on network packet processing while multimedia is playing.",
    "Снимает ограничение, которое Windows накладывает на обработку сетевых пакетов во время воспроизведения мультимедиа.",
    "Знімає обмеження, яке Windows накладає на обробку мережевих пакетів під час відтворення мультимедіа.",
    "Usuwa limit, który Windows nakłada na przetwarzanie pakietów sieciowych podczas odtwarzania multimediów."),
  [reg(MM, "NetworkThrottlingIndex", "dword", 0xFFFFFFFF, 10)],
  rec=True, imp=2)

T("delivery_opt_off", "network", "safe",
  L("Stop sharing updates with other PCs", "Не раздавать обновления другим ПК", "Не роздавати оновлення іншим ПК", "Nie udostępniaj aktualizacji innym PC"),
  L("Turns off Delivery Optimization peer-to-peer, so Windows doesn't use your upload bandwidth to send updates to strangers mid-match.",
    "Отключает P2P-доставку обновлений: Windows не будет тратить ваш исходящий канал на раздачу обновлений чужим ПК прямо во время матча.",
    "Вимикає P2P-доставку оновлень: Windows не витрачатиме ваш вихідний канал на роздачу оновлень чужим ПК просто під час матчу.",
    "Wyłącza P2P Optymalizacji dostarczania, więc Windows nie zużywa twojego wysyłania na przesyłanie aktualizacji obcym w trakcie meczu."),
  [reg(POL + r"\DeliveryOptimization", "DODownloadMode", "dword", 0)],
  rec=True, imp=2)

T("qos_limit_off", "network", "safe",
  L("Remove QoS bandwidth reservation", "Убрать резерв канала QoS", "Прибрати резерв каналу QoS", "Usuń rezerwację pasma QoS"),
  L("Sets the QoS Packet Scheduler reservable bandwidth limit to 0%, so prioritized system traffic can't hold back part of your connection.",
    "Устанавливает резервируемую планировщиком пакетов QoS полосу в 0%, чтобы системный трафик не мог удерживать часть канала.",
    "Встановлює смугу, що резервується планувальником пакетів QoS, у 0%, щоб системний трафік не міг утримувати частину каналу.",
    "Ustawia rezerwowane pasmo harmonogramu pakietów QoS na 0%, aby ruch systemowy nie blokował części łącza."),
  [reg(POL + r"\Psched", "NonBestEffortLimit", "dword", 0)],
  imp=1)

NIC_CLASS = r"HKLM:\SYSTEM\CurrentControlSet\Control\Class\{4D36E972-E325-11CE-BFC1-08002BE10318}"
NIC_KEYS = r"""
$ids = @(Get-NetAdapter -Physical -ErrorAction SilentlyContinue | ForEach-Object { [string]$_.InterfaceGuid })
$keys = @(Get-ChildItem '%s' -ErrorAction SilentlyContinue | Where-Object { $_.PSChildName -match '^\d{4}$' } | Where-Object { $ids -contains [string](Get-ItemProperty -Path $_.PSPath -Name NetCfgInstanceId -ErrorAction SilentlyContinue).NetCfgInstanceId })
""" % NIC_CLASS
T("nic_power_off", "network", "safe",
  L("Keep network adapter awake", "Не отключать сетевую карту", "Не вимикати мережеву карту", "Nie usypiaj karty sieciowej"),
  L("Clears 'Allow the computer to turn off this device to save power' on your network adapters — no more random disconnects and ping spikes after idle.",
    "Снимает флажок «Разрешить отключение этого устройства для экономии энергии» у сетевых карт — без случайных обрывов и скачков пинга после простоя.",
    "Знімає прапорець «Дозволити вимкнення цього пристрою для економії енергії» у мережевих карт — без випадкових обривів і стрибків пінгу після простою.",
    "Odznacza „Zezwalaj komputerowi na wyłączanie tego urządzenia” dla kart sieciowych — koniec z rozłączeniami i skokami pingu po bezczynności."),
  [ps(NIC_KEYS + r"""
$bk = @{}
foreach($k in $keys){
  $v = (Get-ItemProperty -Path $k.PSPath -Name PnPCapabilities -ErrorAction SilentlyContinue).PnPCapabilities
  if($null -eq $v){ $bk[$k.PSChildName] = -1 } else { $bk[$k.PSChildName] = [int]$v }
  New-ItemProperty -Path $k.PSPath -Name PnPCapabilities -Value 24 -PropertyType DWord -Force | Out-Null
}
'@@BACKUP@@' + ($bk | ConvertTo-Json -Compress)
""", r"""
if([string]::IsNullOrEmpty($LarikBackup)){ return }
$bk = $LarikBackup | ConvertFrom-Json
foreach($prop in $bk.PSObject.Properties){
  $p = '%s\' + $prop.Name
  if([int]$prop.Value -eq -1){ Remove-ItemProperty -Path $p -Name PnPCapabilities -ErrorAction SilentlyContinue }
  else { New-ItemProperty -Path $p -Name PnPCapabilities -Value ([int]$prop.Value) -PropertyType DWord -Force | Out-Null }
}
""" % NIC_CLASS, NIC_KEYS + r"""
if($keys.Count -eq 0){ 'na'; return }
$ok = $true
foreach($k in $keys){ if((Get-ItemProperty -Path $k.PSPath -Name PnPCapabilities -ErrorAction SilentlyContinue).PnPCapabilities -ne 24){ $ok = $false } }
if($ok){'1'}else{'0'}
""")],
  rec=True, imp=1, restart="pc", when={"desktop": True})

ETH = r"$ads = @(Get-NetAdapter -Physical -ErrorAction SilentlyContinue | Where-Object { $_.Status -eq 'Up' -and $_.MediaType -eq '802.3' })"
T("interrupt_moderation_off", "network", "advanced",
  L("Disable interrupt moderation", "Отключить модерацию прерываний", "Вимкнути модерацію переривань", "Wyłącz moderację przerwań"),
  L("Your Ethernet adapter handles each packet immediately instead of grouping them. Shaves latency at the cost of some extra CPU load.",
    "Сетевая карта Ethernet обрабатывает каждый пакет сразу, а не группами. Снижает задержку ценой небольшой дополнительной нагрузки на процессор.",
    "Мережева карта Ethernet обробляє кожен пакет одразу, а не групами. Знижує затримку ціною невеликого додаткового навантаження на процесор.",
    "Karta Ethernet obsługuje każdy pakiet od razu zamiast grupować. Zmniejsza opóźnienie kosztem nieco większego obciążenia CPU."),
  [ps(ETH + r"""
$bk = @{}
foreach($a in $ads){
  $p = Get-NetAdapterAdvancedProperty -Name $a.Name -RegistryKeyword '*InterruptModeration' -ErrorAction SilentlyContinue
  if($p){ $bk[$a.Name] = [string]($p.RegistryValue | Select-Object -First 1); Set-NetAdapterAdvancedProperty -Name $a.Name -RegistryKeyword '*InterruptModeration' -RegistryValue 0 -NoRestart }
}
'@@BACKUP@@' + ($bk | ConvertTo-Json -Compress)
""", r"""
$bk = $null
if(-not [string]::IsNullOrEmpty($LarikBackup)){ $bk = $LarikBackup | ConvertFrom-Json }
""" + ETH + r"""
foreach($a in $ads){
  $v = '1'
  if($bk -and $bk.PSObject.Properties[$a.Name]){ $v = [string]$bk.PSObject.Properties[$a.Name].Value }
  Set-NetAdapterAdvancedProperty -Name $a.Name -RegistryKeyword '*InterruptModeration' -RegistryValue $v -NoRestart -ErrorAction SilentlyContinue
}
""", ETH + r"""
$props = @(foreach($a in $ads){ Get-NetAdapterAdvancedProperty -Name $a.Name -RegistryKeyword '*InterruptModeration' -ErrorAction SilentlyContinue })
if($props.Count -eq 0){ 'na'; return }
$bad = @($props | Where-Object { [string]($_.RegistryValue | Select-Object -First 1) -ne '0' })
if($bad.Count -eq 0){'1'}else{'0'}
""")],
  imp=1, restart="pc")

PHYS = r"$ads = @(Get-NetAdapter -Physical -ErrorAction SilentlyContinue | Where-Object Status -eq 'Up')"
T("dns_cloudflare", "network", "safe",
  L("Fast DNS (Cloudflare 1.1.1.1)", "Быстрый DNS (Cloudflare 1.1.1.1)", "Швидкий DNS (Cloudflare 1.1.1.1)", "Szybki DNS (Cloudflare 1.1.1.1)"),
  L("Switches active adapters to Cloudflare DNS, usually faster and more private than your provider's. Speeds up matchmaking and launcher connections, not in-game ping.",
    "Переключает активные адаптеры на DNS Cloudflare — обычно быстрее и приватнее, чем у провайдера. Ускоряет подключение лаунчеров и поиск матча, но не пинг в игре.",
    "Перемикає активні адаптери на DNS Cloudflare — зазвичай швидший і приватніший, ніж у провайдера. Пришвидшує підключення лаунчерів і пошук матчу, але не пінг у грі.",
    "Przełącza aktywne karty na DNS Cloudflare — zwykle szybszy i bardziej prywatny niż u dostawcy. Przyspiesza łączenie launcherów i matchmaking, nie ping w grze."),
  [ps(PHYS + r"""
$bk = @{}
foreach($a in $ads){
  $ns = (Get-ItemProperty -Path ("HKLM:\SYSTEM\CurrentControlSet\Services\Tcpip\Parameters\Interfaces\" + [string]$a.InterfaceGuid) -Name NameServer -ErrorAction SilentlyContinue).NameServer
  $bk[[string]$a.ifIndex] = [string]$ns
  Set-DnsClientServerAddress -InterfaceIndex $a.ifIndex -ServerAddresses ('1.1.1.1','1.0.0.1')
}
Clear-DnsClientCache
'@@BACKUP@@' + ($bk | ConvertTo-Json -Compress)
""", r"""
if([string]::IsNullOrEmpty($LarikBackup)){
""" + PHYS + r"""
  foreach($a in $ads){ Set-DnsClientServerAddress -InterfaceIndex $a.ifIndex -ResetServerAddresses -ErrorAction SilentlyContinue }
} else {
  $bk = $LarikBackup | ConvertFrom-Json
  foreach($prop in $bk.PSObject.Properties){
    $v = [string]$prop.Value
    if([string]::IsNullOrWhiteSpace($v)){ Set-DnsClientServerAddress -InterfaceIndex ([int]$prop.Name) -ResetServerAddresses -ErrorAction SilentlyContinue }
    else { Set-DnsClientServerAddress -InterfaceIndex ([int]$prop.Name) -ServerAddresses @($v -split '[,\s]+' | Where-Object { $_ }) -ErrorAction SilentlyContinue }
  }
}
Clear-DnsClientCache
""", PHYS + r"""
if($ads.Count -eq 0){ 'na'; return }
$ok = $true
foreach($a in $ads){ $s = @((Get-DnsClientServerAddress -InterfaceIndex $a.ifIndex -AddressFamily IPv4 -ErrorAction SilentlyContinue).ServerAddresses); if(-not ($s -contains '1.1.1.1')){ $ok = $false } }
if($ok){'1'}else{'0'}
""")],
  imp=1)

# ============================================================ PRIVACY & SECURITY
T("telemetry_off", "privacy", "safe",
  L("Disable Windows telemetry", "Отключить телеметрию Windows", "Вимкнути телеметрію Windows", "Wyłącz telemetrię Windows"),
  L("Sets diagnostic data to the minimum and stops the Connected User Experiences (DiagTrack) service that uploads it in the background.",
    "Снижает сбор диагностических данных до минимума и отключает службу DiagTrack, которая отправляет их в фоне.",
    "Знижує збір діагностичних даних до мінімуму та вимикає службу DiagTrack, яка надсилає їх у фоні.",
    "Ogranicza dane diagnostyczne do minimum i wyłącza usługę DiagTrack, która wysyła je w tle."),
  [reg(POL + r"\DataCollection", "AllowTelemetry", "dword", 0),
   svc("DiagTrack", "disabled", "auto"),
   svc("dmwappushservice", "disabled", "demand")],
  rec=True, imp=2)

T("telemetry_tasks_off", "privacy", "safe",
  L("Disable telemetry scheduled tasks", "Отключить задачи телеметрии", "Вимкнути завдання телеметрії", "Wyłącz zadania telemetrii"),
  L("Turns off the Compatibility Appraiser and CEIP tasks — they wake up at random and can max out the CPU and disk for minutes.",
    "Отключает задачи Compatibility Appraiser и CEIP — они запускаются в случайное время и могут на минуты загрузить процессор и диск.",
    "Вимикає завдання Compatibility Appraiser і CEIP — вони запускаються у випадковий час і можуть на хвилини завантажити процесор і диск.",
    "Wyłącza zadania Compatibility Appraiser i CEIP — uruchamiają się losowo i potrafią na minuty obciążyć CPU i dysk."),
  [task(r"\Microsoft\Windows\Application Experience\Microsoft Compatibility Appraiser"),
   task(r"\Microsoft\Windows\Application Experience\ProgramDataUpdater"),
   task(r"\Microsoft\Windows\Application Experience\PcaPatchDbTask"),
   task(r"\Microsoft\Windows\Customer Experience Improvement Program\Consolidator"),
   task(r"\Microsoft\Windows\Customer Experience Improvement Program\UsbCeip"),
   task(r"\Microsoft\Windows\DiskDiagnostic\Microsoft-Windows-DiskDiagnosticDataCollector"),
   task(r"\Microsoft\Windows\Autochk\Proxy"),
   task(r"\Microsoft\Windows\Feedback\Siuf\DmClient"),
   task(r"\Microsoft\Windows\Feedback\Siuf\DmClientOnScenarioDownload")],
  rec=True, imp=2)

T("ad_id_off", "privacy", "safe",
  L("Disable advertising ID", "Отключить рекламный идентификатор", "Вимкнути рекламний ідентифікатор", "Wyłącz identyfikator reklamowy"),
  L("Apps can no longer use your advertising ID to show personalized ads.",
    "Приложения больше не смогут использовать рекламный идентификатор для персональной рекламы.",
    "Застосунки більше не зможуть використовувати рекламний ідентифікатор для персоналізованої реклами.",
    "Aplikacje nie mogą już używać identyfikatora reklamowego do spersonalizowanych reklam."),
  [reg(r"HKCU\Software\Microsoft\Windows\CurrentVersion\AdvertisingInfo", "Enabled", "dword", 0),
   reg(POL + r"\AdvertisingInfo", "DisabledByGroupPolicy", "dword", 1)],
  rec=True, imp=1)

T("activity_off", "privacy", "safe",
  L("Disable activity history", "Отключить журнал действий", "Вимкнути журнал дій", "Wyłącz historię aktywności"),
  L("Windows stops collecting and uploading a history of the apps, files and sites you open.",
    "Windows перестаёт собирать и отправлять историю открытых программ, файлов и сайтов.",
    "Windows перестає збирати й надсилати історію відкритих програм, файлів і сайтів.",
    "Windows przestaje zbierać i wysyłać historię otwieranych aplikacji, plików i stron."),
  [reg(POL + r"\System", "EnableActivityFeed", "dword", 0),
   reg(POL + r"\System", "PublishUserActivities", "dword", 0),
   reg(POL + r"\System", "UploadUserActivities", "dword", 0)],
  rec=True, imp=1)

T("tailored_off", "privacy", "safe",
  L("Disable tailored experiences", "Отключить персонализацию по данным", "Вимкнути персоналізацію за даними", "Wyłącz dopasowane środowisko"),
  L("Microsoft stops using your diagnostic data to show tips, ads and recommendations.",
    "Microsoft перестаёт использовать диагностические данные для советов, рекламы и рекомендаций.",
    "Microsoft перестає використовувати діагностичні дані для порад, реклами та рекомендацій.",
    "Microsoft przestaje używać danych diagnostycznych do wskazówek, reklam i rekomendacji."),
  [reg(r"HKCU\Software\Microsoft\Windows\CurrentVersion\Privacy", "TailoredExperiencesWithDiagnosticDataEnabled", "dword", 0)],
  rec=True, imp=1)

T("feedback_off", "privacy", "safe",
  L("Stop feedback requests", "Отключить запросы отзывов", "Вимкнути запити відгуків", "Wyłącz prośby o opinię"),
  L("No more 'How likely are you to recommend Windows?' pop-ups.",
    "Больше никаких окон «Насколько вероятно, что вы порекомендуете Windows?».",
    "Більше жодних вікон «Наскільки ймовірно, що ви порекомендуєте Windows?».",
    "Koniec z oknami „Jak bardzo prawdopodobne jest, że polecisz Windows?”."),
  [reg(r"HKCU\Software\Microsoft\Siuf\Rules", "NumberOfSIUFInPeriod", "dword", 0),
   reg(POL + r"\DataCollection", "DoNotShowFeedbackNotifications", "dword", 1)],
  rec=True, imp=1)

T("typing_insights_off", "privacy", "safe",
  L("Stop inking and typing collection", "Отключить сбор рукописного ввода и набора", "Вимкнути збір рукописного введення та набору", "Wyłącz zbieranie pisma i wpisywania"),
  L("Windows stops building a personal dictionary from what you type and write and sending it to Microsoft.",
    "Windows перестаёт составлять личный словарь из набранного и рукописного текста и отправлять его в Microsoft.",
    "Windows перестає складати особистий словник із набраного та рукописного тексту й надсилати його до Microsoft.",
    "Windows przestaje budować osobisty słownik z tego, co piszesz, i wysyłać go do Microsoftu."),
  [reg(r"HKCU\Software\Microsoft\InputPersonalization", "RestrictImplicitInkCollection", "dword", 1),
   reg(r"HKCU\Software\Microsoft\InputPersonalization", "RestrictImplicitTextCollection", "dword", 1),
   reg(r"HKCU\Software\Microsoft\InputPersonalization\TrainedDataStore", "HarvestContacts", "dword", 0),
   reg(r"HKCU\Software\Microsoft\Personalization\Settings", "AcceptedPrivacyPolicy", "dword", 0)],
  rec=True, imp=1)

T("cortana_off", "privacy", "safe",
  L("Disable Cortana", "Отключить Cortana", "Вимкнути Cortana", "Wyłącz Cortanę"),
  L("Prevents Cortana from running and using CPU and disk in the background.",
    "Не даёт Cortana запускаться и тратить процессор и диск в фоне.",
    "Не дає Cortana запускатися й витрачати процесор і диск у фоні.",
    "Uniemożliwia Cortanie działanie i zużywanie CPU oraz dysku w tle."),
  [reg(POL + r"\Windows Search", "AllowCortana", "dword", 0)],
  rec=True, imp=1)

T("web_search_off", "privacy", "safe",
  L("Remove Bing from Start search", "Убрать Bing из поиска «Пуск»", "Прибрати Bing із пошуку «Пуск»", "Usuń Bing z wyszukiwania Start"),
  L("Start search shows only your apps and files. It's faster and doesn't send what you type to Bing.",
    "Поиск в «Пуске» показывает только ваши программы и файлы — быстрее и не отправляет вводимый текст в Bing.",
    "Пошук у «Пуску» показує лише ваші програми й файли — швидше й не надсилає введений текст у Bing.",
    "Wyszukiwanie Start pokazuje tylko twoje aplikacje i pliki. Jest szybsze i nie wysyła wpisywanego tekstu do Bing."),
  [reg(r"HKCU\Software\Microsoft\Windows\CurrentVersion\Search", "BingSearchEnabled", "dword", 0),
   reg(UPOL + r"\Explorer", "DisableSearchBoxSuggestions", "dword", 1),
   reg(POL + r"\Windows Search", "ConnectedSearchUseWeb", "dword", 0),
   reg(POL + r"\Windows Search", "DisableWebSearch", "dword", 1)],
  rec=True, imp=1, restart="explorer")

T("copilot_off", "privacy", "safe",
  L("Disable Copilot", "Отключить Copilot", "Вимкнути Copilot", "Wyłącz Copilot"),
  L("Removes the Windows Copilot sidebar and taskbar button.",
    "Убирает боковую панель Windows Copilot и кнопку на панели задач.",
    "Прибирає бічну панель Windows Copilot і кнопку на панелі завдань.",
    "Usuwa panel boczny Windows Copilot i przycisk na pasku zadań."),
  [reg(UPOL + r"\WindowsCopilot", "TurnOffWindowsCopilot", "dword", 1),
   reg(POL + r"\WindowsCopilot", "TurnOffWindowsCopilot", "dword", 1),
   reg(ADV, "ShowCopilotButton", "dword", 0)],
  rec=True, imp=1, req=W11, tags=["win11"], restart="explorer")

T("recall_off", "privacy", "safe",
  L("Disable Recall snapshots", "Отключить снимки Recall", "Вимкнути знімки Recall", "Wyłącz migawki Recall"),
  L("Stops Windows AI from saving periodic screenshots of everything you do. Also saves disk writes during gaming.",
    "Windows AI перестаёт сохранять периодические снимки всего, что происходит на экране. Заодно меньше записей на диск во время игры.",
    "Windows AI перестає зберігати періодичні знімки всього, що відбувається на екрані. Заодно менше записів на диск під час гри.",
    "Windows AI przestaje zapisywać okresowe zrzuty wszystkiego, co robisz. Oszczędza też zapisy na dysk podczas gry."),
  [reg(UPOL + r"\WindowsAI", "DisableAIDataAnalysis", "dword", 1),
   reg(POL + r"\WindowsAI", "DisableAIDataAnalysis", "dword", 1),
   reg(POL + r"\WindowsAI", "AllowRecallEnablement", "dword", 0)],
  rec=True, imp=1, req=W11, tags=["win11"])

T("error_reporting_off", "privacy", "safe",
  L("Disable Windows Error Reporting", "Отключить отчёты об ошибках", "Вимкнути звіти про помилки", "Wyłącz raportowanie błędów"),
  L("Crashed apps close instantly instead of collecting a dump and uploading it to Microsoft.",
    "Упавшие программы закрываются сразу, без сбора дампа и отправки его в Microsoft.",
    "Програми, що впали, закриваються одразу, без збору дампу й надсилання його до Microsoft.",
    "Zawieszone aplikacje zamykają się od razu, bez zbierania zrzutu i wysyłania go do Microsoftu."),
  [reg(POL + r"\Windows Error Reporting", "Disabled", "dword", 1)],
  imp=1)

T("firefox_telemetry_off", "privacy", "safe",
  L("Disable Firefox telemetry", "Отключить телеметрию Firefox", "Вимкнути телеметрію Firefox", "Wyłącz telemetrię Firefoksa"),
  L("Stops Firefox from sending usage and performance data to Mozilla and disables its default-browser agent task.",
    "Firefox перестаёт отправлять статистику использования и производительности в Mozilla, а задача «агент браузера по умолчанию» отключается.",
    "Firefox перестає надсилати статистику використання та продуктивності до Mozilla, а завдання «агент браузера за замовчуванням» вимикається.",
    "Firefox przestaje wysyłać dane o użyciu i wydajności do Mozilli, a zadanie agenta domyślnej przeglądarki zostaje wyłączone."),
  [reg(r"HKLM\SOFTWARE\Policies\Mozilla\Firefox", "DisableTelemetry", "dword", 1),
   reg(r"HKLM\SOFTWARE\Policies\Mozilla\Firefox", "DisableDefaultBrowserAgent", "dword", 1)],
  imp=1)

T("app_tracking_off", "privacy", "safe",
  L("Don't track app launches", "Не отслеживать запуск программ", "Не відстежувати запуск програм", "Nie śledź uruchomień aplikacji"),
  L("Windows stops recording which apps you open to rank Start and search results.",
    "Windows перестаёт записывать, какие программы вы открываете, для сортировки «Пуска» и поиска.",
    "Windows перестає записувати, які програми ви відкриваєте, для сортування «Пуску» та пошуку.",
    "Windows przestaje zapisywać, które aplikacje otwierasz, do układania menu Start i wyników wyszukiwania."),
  [reg(ADV, "Start_TrackProgs", "dword", 0)],
  imp=1)

T("llmnr_off", "privacy", "safe",
  L("Disable LLMNR", "Отключить LLMNR", "Вимкнути LLMNR", "Wyłącz LLMNR"),
  L("Closes a legacy name-resolution protocol often abused on public Wi-Fi to steal password hashes. Also cuts multicast chatter on your network.",
    "Отключает устаревший протокол разрешения имён, через который в публичных Wi-Fi часто крадут хэши паролей. Заодно меньше широковещательного трафика.",
    "Вимикає застарілий протокол розпізнавання імен, через який у публічних Wi-Fi часто крадуть хеші паролів. Заразом менше широкомовного трафіку.",
    "Wyłącza przestarzały protokół rozwiązywania nazw, często wykorzystywany w publicznym Wi-Fi do kradzieży skrótów haseł. Mniej ruchu multicast."),
  [reg(r"HKLM\SOFTWARE\Policies\Microsoft\Windows NT\DNSClient", "EnableMulticast", "dword", 0)],
  rec=True, imp=1)

T("remote_assist_off", "privacy", "safe",
  L("Disable Remote Assistance", "Отключить удалённого помощника", "Вимкнути віддаленого помічника", "Wyłącz Pomoc zdalną"),
  L("Blocks Remote Assistance invitations — a common target in tech-support scams. Remote Desktop is not affected.",
    "Блокирует приглашения удалённого помощника — частую цель мошенников под видом техподдержки. Удалённый рабочий стол не затрагивается.",
    "Блокує запрошення віддаленого помічника — часту ціль шахраїв під виглядом техпідтримки. Віддалений робочий стіл не зачіпається.",
    "Blokuje zaproszenia Pomocy zdalnej — częsty cel oszustów podszywających się pod pomoc techniczną. Pulpit zdalny działa dalej."),
  [reg(r"HKLM\SYSTEM\CurrentControlSet\Control\Remote Assistance", "fAllowToGetHelp", "dword", 0, 1)],
  rec=True, imp=1)

T("vbs_off", "privacy", "advanced",
  L("Disable Memory Integrity (VBS)", "Отключить целостность памяти (VBS)", "Вимкнути цілісність пам'яті (VBS)", "Wyłącz integralność pamięci (VBS)"),
  L("Microsoft confirms virtualization-based security can cost FPS in games. Turning it off gains performance but removes protection against kernel-level malware.",
    "Microsoft подтверждает, что защита на основе виртуализации может снижать FPS в играх. Отключение даёт прирост, но убирает защиту от вредоносного кода уровня ядра.",
    "Microsoft підтверджує, що захист на основі віртуалізації може знижувати FPS в іграх. Вимкнення дає приріст, але прибирає захист від шкідливого коду рівня ядра.",
    "Microsoft potwierdza, że zabezpieczenia oparte na wirtualizacji mogą obniżać FPS w grach. Wyłączenie daje wzrost, ale usuwa ochronę przed złośliwym kodem jądra."),
  [reg(r"HKLM\SYSTEM\CurrentControlSet\Control\DeviceGuard\Scenarios\HypervisorEnforcedCodeIntegrity", "Enabled", "dword", 0),
   reg(r"HKLM\SYSTEM\CurrentControlSet\Control\DeviceGuard", "EnableVirtualizationBasedSecurity", "dword", 0)],
  imp=3, restart="pc", tags=["security"],
  warn=L("Lowers system security. Some anti-cheats (Valorant, FACEIT) may ask you to turn it back on.",
         "Снижает защищённость системы. Некоторые античиты (Valorant, FACEIT) могут потребовать включить обратно.",
         "Знижує захищеність системи. Деякі античити (Valorant, FACEIT) можуть вимагати ввімкнути назад.",
         "Obniża bezpieczeństwo systemu. Niektóre anty-cheaty (Valorant, FACEIT) mogą wymagać ponownego włączenia."))


# ============================================================ ADDED IN 1.0.5
T("paging_executive", "fps", "safe",
  L("Keep Windows core in RAM", "Держать ядро Windows в памяти", "Тримати ядро Windows у пам'яті", "Trzymaj jądro Windows w RAM"),
  L("Stops Windows from moving its own kernel code and drivers to the page file. With 16 GB or more this removes small stalls when the system pulls them back from disk.",
    "Windows перестаёт выгружать код ядра и драйверы в файл подкачки. При 16 ГБ и больше это убирает мелкие подвисания, когда система подгружает их обратно с диска.",
    "Windows перестає вивантажувати код ядра й драйвери у файл підкачки. За 16 ГБ і більше це прибирає дрібні підвисання, коли система підвантажує їх назад із диска.",
    "Windows przestaje przenosić kod jądra i sterowniki do pliku stronicowania. Przy 16 GB i więcej usuwa to drobne przycięcia, gdy system wczytuje je z dysku."),
  [reg(r"HKLM\SYSTEM\CurrentControlSet\Control\Session Manager\Memory Management", "DisablePagingExecutive", "dword", 1, 0)],
  rec=True, imp=1, restart="pc", when={"ram16": True})

T("fth_off", "fps", "moderate",
  L("Disable Fault Tolerant Heap", "Отключить отказоустойчивую кучу (FTH)", "Вимкнути відмовостійку купу (FTH)", "Wyłącz Fault Tolerant Heap"),
  L("After a program crashes a few times, Windows silently runs it with slower memory handling. This often hits games after a few driver crashes and never switches back. Turning it off restores full speed.",
    "Если программа несколько раз вылетела, Windows незаметно запускает её с более медленной работой памяти. Это часто случается с играми после пары вылетов драйвера и само не отключается. Отключение возвращает полную скорость.",
    "Якщо програма кілька разів аварійно завершилася, Windows непомітно запускає її з повільнішою роботою пам'яті. Це часто стається з іграми після кількох збоїв драйвера й саме не вимикається. Вимкнення повертає повну швидкість.",
    "Gdy program kilka razy się zawiesi, Windows po cichu uruchamia go z wolniejszą obsługą pamięci. Często dotyka to gier po kilku awariach sterownika i samo się nie wyłącza. Wyłączenie przywraca pełną szybkość."),
  [reg(r"HKLM\SOFTWARE\Microsoft\FTH", "Enabled", "dword", 0, 1)],
  imp=1, restart="pc",
  warn=L("Programs that really do crash often may crash a bit more without it.",
         "Программы, которые действительно часто вылетают, без неё могут вылетать чуть чаще.",
         "Програми, які справді часто аварійно завершуються, без неї можуть падати трохи частіше.",
         "Programy, które naprawdę często się zawieszają, mogą bez tego zawieszać się nieco częściej."))

T("gamebar_tips_off", "qol", "safe",
  L("No Game Bar pop-ups in games", "Без подсказок Game Bar в играх", "Без підказок Game Bar в іграх", "Bez podpowiedzi Game Bar w grach"),
  L("Stops the \"Press Win+G to open Game Bar\" banner that appears when a game starts.",
    "Убирает баннер «Нажмите Win+G, чтобы открыть Game Bar», который появляется при запуске игры.",
    "Прибирає банер «Натисніть Win+G, щоб відкрити Game Bar», що з'являється під час запуску гри.",
    "Usuwa baner „Naciśnij Win+G, aby otworzyć Game Bar”, który pojawia się przy uruchomieniu gry."),
  [reg(r"HKCU\Software\Microsoft\GameBar", "ShowStartupPanel", "dword", 0)],
  rec=True, imp=1)

T("audio_ducking_off", "qol", "safe",
  L("Don't lower game sound during calls", "Не приглушать звук игры при звонках", "Не приглушувати звук гри під час дзвінків", "Nie ściszaj gry podczas rozmów"),
  L("Windows turns other sounds down by 80% when it detects a voice call (Discord, Teams). This keeps your game at full volume.",
    "Windows приглушает остальные звуки на 80%, когда замечает голосовой звонок (Discord, Teams). Звук игры останется на полной громкости.",
    "Windows приглушує інші звуки на 80%, коли помічає голосовий дзвінок (Discord, Teams). Звук гри залишиться на повній гучності.",
    "Windows ścisza inne dźwięki o 80%, gdy wykryje rozmowę głosową (Discord, Teams). Gra zachowa pełną głośność."),
  [reg(r"HKCU\Software\Microsoft\Multimedia\Audio", "UserDuckingPreference", "dword", 3, 1)],
  rec=True, imp=1)

T("storage_sense_on", "qol", "safe",
  L("Turn on Storage Sense", "Включить контроль памяти", "Увімкнути контроль пам'яті", "Włącz Czujnik pamięci"),
  L("Windows automatically deletes old temporary files and empties the Recycle Bin of items older than 30 days, so the system drive doesn't fill up.",
    "Windows сам удаляет старые временные файлы и очищает корзину от файлов старше 30 дней, чтобы системный диск не переполнялся.",
    "Windows сам видаляє старі тимчасові файли й очищає кошик від файлів, старших за 30 днів, щоб системний диск не переповнювався.",
    "Windows sam usuwa stare pliki tymczasowe i opróżnia Kosz z plików starszych niż 30 dni, aby dysk systemowy się nie zapełniał."),
  [reg(r"HKCU\Software\Microsoft\Windows\CurrentVersion\StorageSense\Parameters\StoragePolicy", "01", "dword", 1, 0)],
  rec=True, imp=1)

T("search_highlights_off", "qol", "safe",
  L("Disable search highlights", "Отключить «Основные моменты поиска»", "Вимкнути «Основні моменти пошуку»", "Wyłącz wyróżnienia wyszukiwania"),
  L("Removes the daily illustrations and trending web content from the Start search box, so search opens faster and stays clean.",
    "Убирает ежедневные картинки и популярный веб-контент из поиска в меню «Пуск», поиск открывается быстрее.",
    "Прибирає щоденні картинки й популярний веб-контент із пошуку в меню «Пуск», пошук відкривається швидше.",
    "Usuwa codzienne ilustracje i popularne treści z sieci z wyszukiwania w menu Start, dzięki czemu otwiera się szybciej."),
  [reg(r"HKCU\Software\Microsoft\Windows\CurrentVersion\SearchSettings", "IsDynamicSearchBoxEnabled", "dword", 0, 1)],
  rec=True, imp=1)

T("quick_access_recent_off", "privacy", "safe",
  L("Hide recent files in Explorer", "Скрыть недавние файлы в проводнике", "Сховати нещодавні файли в провіднику", "Ukryj ostatnie pliki w Eksploratorze"),
  L("Explorer stops listing recently opened files and frequent folders on its home page. It opens faster and doesn't show what you worked on.",
    "Проводник перестаёт показывать недавно открытые файлы и частые папки на главной странице. Открывается быстрее и не показывает, с чем вы работали.",
    "Провідник перестає показувати нещодавно відкриті файли й часті папки на головній сторінці. Відкривається швидше й не показує, з чим ви працювали.",
    "Eksplorator przestaje pokazywać ostatnio otwierane pliki i częste foldery na stronie głównej. Otwiera się szybciej i nie pokazuje, nad czym pracowałeś."),
  [reg(r"HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer", "ShowRecent", "dword", 0, 1),
   reg(r"HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer", "ShowFrequent", "dword", 0, 1)],
  imp=1, restart="explorer")

T("folder_discovery_off", "qol", "safe",
  L("Faster opening of large folders", "Быстрее открывать большие папки", "Швидше відкривати великі папки", "Szybsze otwieranie dużych folderów"),
  L("Stops Explorer from scanning every file to guess the folder type (music, pictures…). Folders with thousands of files, like game installs, open much faster.",
    "Проводник перестаёт сканировать все файлы, чтобы угадать тип папки (музыка, фото…). Папки с тысячами файлов, например с играми, открываются намного быстрее.",
    "Провідник перестає сканувати всі файли, щоб вгадати тип папки (музика, фото…). Папки з тисячами файлів, наприклад з іграми, відкриваються набагато швидше.",
    "Eksplorator przestaje skanować wszystkie pliki, by zgadnąć typ folderu (muzyka, zdjęcia…). Foldery z tysiącami plików, np. z grami, otwierają się dużo szybciej."),
  [reg(r"HKCU\Software\Classes\Local Settings\Software\Microsoft\Windows\Shell\Bags\AllFolders\Shell", "FolderType", "sz", "NotSpecified")],
  rec=True, imp=1, restart="explorer")


T("input_latency", "input", "moderate",
  L("Shorter mouse and keyboard buffers", "Короткие буферы мыши и клавиатуры", "Короткі буфери миші та клавіатури", "Krótsze bufory myszy i klawiatury"),
  L("Lowers the mouse and keyboard driver event queues from 100 to 50 entries, so clicks, key presses and movement reach the game with less buffering. The effect is small but measurable on high polling-rate devices.",
    "Уменьшает очереди событий драйверов мыши и клавиатуры со 100 до 50, чтобы клики, нажатия и движения доходили до игры с меньшей буферизацией. Эффект небольшой, но заметен на устройствах с высокой частотой опроса.",
    "Зменшує черги подій драйверів миші та клавіатури зі 100 до 50, щоб кліки, натискання й рухи доходили до гри з меншою буферизацією. Ефект невеликий, але помітний на пристроях із високою частотою опитування.",
    "Zmniejsza kolejki zdarzeń sterowników myszy i klawiatury ze 100 do 50, aby kliknięcia, naciśnięcia i ruchy docierały do gry z mniejszym buforowaniem. Efekt jest niewielki, ale zauważalny przy wysokiej częstotliwości odpytywania."),
  [reg(r"HKLM\SYSTEM\CurrentControlSet\Services\mouclass\Parameters", "MouseDataQueueSize", "dword", 50, 100),
   reg(r"HKLM\SYSTEM\CurrentControlSet\Services\kbdclass\Parameters", "KeyboardDataQueueSize", "dword", 50, 100)],
  imp=1, restart="pc",
  warn=L("If your mouse ever skips movement (rare, mostly 4000–8000 Hz mice), turn this off.",
         "Если мышь начнёт пропускать движения (редко, в основном мыши на 4000–8000 Гц), выключите.",
         "Якщо миша почне пропускати рухи (рідко, переважно миші на 4000–8000 Гц), вимкніть.",
         "Jeśli mysz zacznie gubić ruchy (rzadko, głównie myszy 4000–8000 Hz), wyłącz to."))

# ============================================================ QUALITY OF LIFE
T("startup_delay_off", "qol", "safe",
  L("Remove startup app delay", "Убрать задержку автозапуска", "Прибрати затримку автозапуску", "Usuń opóźnienie autostartu"),
  L("Windows normally waits about 10 seconds after sign-in before starting startup apps. This removes the artificial wait.",
    "Обычно Windows ждёт около 10 секунд после входа, прежде чем запускать программы из автозагрузки. Твик убирает искусственную паузу.",
    "Зазвичай Windows чекає близько 10 секунд після входу, перш ніж запускати програми з автозавантаження. Твік прибирає штучну паузу.",
    "Windows zwykle czeka ok. 10 sekund po zalogowaniu, zanim uruchomi programy autostartu. Ta zmiana usuwa sztuczne oczekiwanie."),
  [reg(r"HKCU\Software\Microsoft\Windows\CurrentVersion\Explorer\Serialize", "StartupDelayInMSec", "dword", 0)],
  rec=True, imp=1)

T("menu_delay", "qol", "safe",
  L("Snappier menus", "Быстрые меню", "Швидкі меню", "Szybsze menu"),
  L("Cuts the delay before submenus open from 400 ms to 50 ms.",
    "Сокращает задержку перед открытием подменю с 400 до 50 мс.",
    "Скорочує затримку перед відкриттям підменю з 400 до 50 мс.",
    "Skraca opóźnienie otwierania podmenu z 400 do 50 ms."),
  [reg(DESK, "MenuShowDelay", "sz", "50", "400")],
  rec=True, imp=1, restart="signout")

T("fast_shutdown", "qol", "safe",
  L("Faster shutdown", "Быстрое выключение", "Швидке вимкнення", "Szybsze zamykanie"),
  L("Windows waits 2 seconds instead of up to 20 for frozen apps and services before shutting down. Apps with unsaved work still ask first.",
    "Windows ждёт зависшие программы и службы 2 секунды вместо 20 перед выключением. Программы с несохранёнными данными всё равно спросят.",
    "Windows чекає завислі програми та служби 2 секунди замість 20 перед вимкненням. Програми з незбереженими даними все одно запитають.",
    "Windows czeka 2 sekundy zamiast do 20 na zawieszone aplikacje i usługi przed zamknięciem. Aplikacje z niezapisaną pracą nadal zapytają."),
  [reg(DESK, "HungAppTimeout", "sz", "2000"),
   reg(DESK, "WaitToKillAppTimeout", "sz", "2000"),
   reg(r"HKLM\SYSTEM\CurrentControlSet\Control", "WaitToKillServiceTimeout", "sz", "2000", "5000")],
  rec=True, imp=1)

T("show_extensions", "qol", "safe",
  L("Show file extensions", "Показывать расширения файлов", "Показувати розширення файлів", "Pokazuj rozszerzenia plików"),
  L("See the real type of every file — and spot fakes like 'cheat.pdf.exe' before you open them.",
    "Видно настоящий тип каждого файла — и можно заметить подделки вроде «cheat.pdf.exe» до открытия.",
    "Видно справжній тип кожного файлу — і можна помітити підробки на кшталт «cheat.pdf.exe» до відкриття.",
    "Widać prawdziwy typ każdego pliku — i można zauważyć podróbki typu „cheat.pdf.exe” przed otwarciem."),
  [reg(ADV, "HideFileExt", "dword", 0, 1)],
  rec=True, imp=1, restart="explorer")

T("ads_off", "qol", "safe",
  L("Remove ads and suggestions", "Убрать рекламу и подсказки", "Прибрати рекламу та підказки", "Usuń reklamy i sugestie"),
  L("Removes promoted content from Start, Settings and the lock screen tips, plus the 'Let's finish setting up your device' nag.",
    "Убирает продвигаемый контент из «Пуска», «Параметров» и советов на экране блокировки, а также напоминание «Давайте завершим настройку устройства».",
    "Прибирає просуваний вміст із «Пуску», «Параметрів» і порад на екрані блокування, а також нагадування «Завершімо налаштування пристрою».",
    "Usuwa promowane treści z menu Start, Ustawień i wskazówek ekranu blokady oraz przypomnienie „Dokończmy konfigurowanie urządzenia”."),
  [reg(CDM, "SubscribedContent-338393Enabled", "dword", 0, 1),
   reg(CDM, "SubscribedContent-353694Enabled", "dword", 0, 1),
   reg(CDM, "SubscribedContent-353696Enabled", "dword", 0, 1),
   reg(CDM, "SubscribedContent-338388Enabled", "dword", 0, 1),
   reg(CDM, "SubscribedContent-338389Enabled", "dword", 0, 1),
   reg(CDM, "SystemPaneSuggestionsEnabled", "dword", 0, 1),
   reg(CDM, "SoftLandingEnabled", "dword", 0, 1),
   reg(r"HKCU\Software\Microsoft\Windows\CurrentVersion\UserProfileEngagement", "ScoobeSystemSettingEnabled", "dword", 0)],
  rec=True, imp=1)

T("silent_apps_off", "qol", "safe",
  L("Stop sponsored app installs", "Запретить установку спонсорских приложений", "Заборонити встановлення спонсорських застосунків", "Zablokuj instalację sponsorowanych aplikacji"),
  L("Stops Windows from quietly installing promoted apps and games like Candy Crush in the background.",
    "Windows перестаёт тихо устанавливать в фоне продвигаемые приложения и игры вроде Candy Crush.",
    "Windows перестає тихо встановлювати у фоні просувані застосунки та ігри на кшталт Candy Crush.",
    "Windows przestaje po cichu instalować w tle promowane aplikacje i gry, takie jak Candy Crush."),
  [reg(CDM, "SilentInstalledAppsEnabled", "dword", 0, 1),
   reg(CDM, "PreInstalledAppsEnabled", "dword", 0, 1),
   reg(CDM, "OemPreInstalledAppsEnabled", "dword", 0, 1),
   reg(POL + r"\CloudContent", "DisableWindowsConsumerFeatures", "dword", 1)],
  rec=True, imp=1)

T("no_auto_reboot", "qol", "safe",
  L("No forced restarts after updates", "Без принудительных перезагрузок", "Без примусових перезавантажень", "Bez wymuszonych restartów"),
  L("Windows Update won't restart the PC on its own while you're signed in — no more losing a match to an update.",
    "Центр обновления не будет сам перезагружать ПК, пока вы в системе, — больше никаких проигранных из-за обновления матчей.",
    "Центр оновлення не перезавантажуватиме ПК сам, поки ви в системі, — більше жодних програних через оновлення матчів.",
    "Windows Update nie zrestartuje komputera samodzielnie, gdy jesteś zalogowany — koniec z przegranymi meczami przez aktualizację."),
  [reg(POL + r"\WindowsUpdate\AU", "NoAutoRebootWithLoggedOnUsers", "dword", 1)],
  rec=True, imp=2)

T("no_driver_updates", "qol", "moderate",
  L("Keep Windows Update away from drivers", "Не обновлять драйверы через Windows Update", "Не оновлювати драйвери через Windows Update", "Nie aktualizuj sterowników przez Windows Update"),
  L("Stops Windows Update from replacing the GPU driver you installed with an older generic one. You'll need to update drivers yourself.",
    "Windows Update перестаёт заменять установленный вами драйвер видеокарты на старый универсальный. Драйверы придётся обновлять самостоятельно.",
    "Windows Update перестає замінювати встановлений вами драйвер відеокарти на старий універсальний. Драйвери доведеться оновлювати самостійно.",
    "Windows Update przestaje zastępować zainstalowany przez ciebie sterownik GPU starszym, ogólnym. Sterowniki trzeba będzie aktualizować samodzielnie."),
  [reg(POL + r"\WindowsUpdate", "ExcludeWUDriversInQualityUpdate", "dword", 1)],
  imp=1)

T("classic_menu", "qol", "safe",
  L("Classic right-click menu", "Классическое контекстное меню", "Класичне контекстне меню", "Klasyczne menu kontekstowe"),
  L("Brings back the full Windows 10 context menu in Windows 11 — no more 'Show more options'.",
    "Возвращает полное контекстное меню Windows 10 в Windows 11 — без «Показать дополнительные параметры».",
    "Повертає повне контекстне меню Windows 10 у Windows 11 — без «Показати додаткові параметри».",
    "Przywraca pełne menu kontekstowe z Windows 10 w Windows 11 — bez „Pokaż więcej opcji”."),
  [regkey(r"HKCU\Software\Classes\CLSID\{86ca1aa0-34aa-4e8b-a509-50c905bae2a2}\InprocServer32", "")],
  imp=1, req=W11, tags=["win11"], restart="explorer")

T("end_task_taskbar", "qol", "safe",
  L("'End task' in taskbar menu", "«Завершить задачу» на панели задач", "«Завершити завдання» на панелі завдань", "„Zakończ zadanie” na pasku zadań"),
  L("Adds End task to the right-click menu of taskbar apps — kill a frozen game without opening Task Manager.",
    "Добавляет «Завершить задачу» в меню правой кнопки на панели задач — можно закрыть зависшую игру без диспетчера задач.",
    "Додає «Завершити завдання» до меню правої кнопки на панелі завдань — можна закрити завислу гру без диспетчера завдань.",
    "Dodaje „Zakończ zadanie” do menu kontekstowego aplikacji na pasku zadań — zamkniesz zawieszoną grę bez Menedżera zadań."),
  [reg(ADV + r"\TaskbarDeveloperSettings", "TaskbarEndTask", "dword", 1)],
  imp=1, req=W11, tags=["win11"])

T("start_recommended_off", "qol", "safe",
  L("Hide Start recommendations", "Скрыть рекомендации в «Пуске»", "Приховати рекомендації в «Пуску»", "Ukryj polecane w menu Start"),
  L("Removes recommended files and promoted apps from the Windows 11 Start menu.",
    "Убирает рекомендуемые файлы и продвигаемые приложения из меню «Пуск» Windows 11.",
    "Прибирає рекомендовані файли та просувані застосунки з меню «Пуск» Windows 11.",
    "Usuwa polecane pliki i promowane aplikacje z menu Start w Windows 11."),
  [reg(ADV, "Start_IrisRecommendations", "dword", 0),
   reg(ADV, "Start_TrackDocs", "dword", 0)],
  imp=1, req=W11, tags=["win11"], restart="explorer")

T("launch_this_pc", "qol", "safe",
  L("Open Explorer to This PC", "Открывать проводник в «Этот компьютер»", "Відкривати провідник у «Цей ПК»", "Otwieraj Eksplorator w „Ten komputer”"),
  L("File Explorer opens straight to your drives instead of Home / Quick access.",
    "Проводник сразу открывает диски вместо «Главной» или «Быстрого доступа».",
    "Провідник одразу відкриває диски замість «Головної» чи «Швидкого доступу».",
    "Eksplorator plików otwiera się od razu na dyskach zamiast na Stronie głównej / Szybkim dostępie."),
  [reg(ADV, "LaunchTo", "dword", 1)],
  imp=1)

T("verbose_status", "qol", "safe",
  L("Show detailed boot messages", "Подробные сообщения при загрузке", "Докладні повідомлення під час завантаження", "Szczegółowe komunikaty startu"),
  L("Shows what Windows is doing during startup and shutdown instead of a spinning circle. Doesn't speed things up, but helps find what's slow.",
    "Показывает, что делает Windows при загрузке и выключении, вместо вращающегося круга. Не ускоряет, но помогает найти, что тормозит.",
    "Показує, що робить Windows під час завантаження та вимкнення, замість кола, що обертається. Не пришвидшує, але допомагає знайти, що гальмує.",
    "Pokazuje, co Windows robi podczas uruchamiania i zamykania, zamiast kręcącego się kółka. Nie przyspiesza, ale pomaga znaleźć wąskie gardło."),
  [reg(r"HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System", "VerboseStatus", "dword", 1)],
  imp=1)

T("numlock_on", "qol", "safe",
  L("NumLock on at startup", "NumLock при загрузке", "NumLock під час завантаження", "NumLock włączony przy starcie"),
  L("The number pad works right away on the sign-in screen.",
    "Цифровая клавиатура работает сразу на экране входа.",
    "Цифрова клавіатура працює одразу на екрані входу.",
    "Klawiatura numeryczna działa od razu na ekranie logowania."),
  [reg(r"HKU\.DEFAULT\Control Panel\Keyboard", "InitialKeyboardIndicators", "sz", "2", "2147483648")],
  imp=1)

T("long_paths", "qol", "safe",
  L("Enable long file paths", "Включить длинные пути", "Увімкнути довгі шляхи", "Włącz długie ścieżki"),
  L("Lifts the 260-character path limit, which breaks some mod managers and deep game folders.",
    "Снимает ограничение пути в 260 символов, из-за которого ломаются некоторые менеджеры модов и глубокие папки игр.",
    "Знімає обмеження шляху в 260 символів, через яке ламаються деякі менеджери модів і глибокі теки ігор.",
    "Znosi limit ścieżki 260 znaków, przez który psują się niektóre menedżery modów i głębokie foldery gier."),
  [reg(FS, "LongPathsEnabled", "dword", 1, 0)],
  imp=1)

# ============================================================ SERVICES (Debloat)
def S(id, name, desc, svcs, rec=True, risk="safe", when=None, warn=None):
    T(id, "services", risk, name, desc, [svc(n, "disabled", d) for n, d in svcs],
      rec=rec, imp=1, when=when, warn=warn)


S("svc_fax", L("Fax", "Факс", "Факс", "Faks"),
  L("Sends and receives faxes. Nobody games on a fax.", "Отправка и приём факсов.", "Надсилання й отримання факсів.", "Wysyłanie i odbieranie faksów."),
  [("Fax", "demand")])
S("svc_retaildemo", L("Retail Demo", "Демонстрация в магазине", "Демонстрація в магазині", "Tryb demonstracyjny"),
  L("Store display mode for PCs on a shop shelf.", "Режим витрины для ПК в магазине.", "Режим вітрини для ПК у магазині.", "Tryb wystawowy dla komputerów w sklepie."),
  [("RetailDemo", "demand")])
S("svc_maps", L("Downloaded Maps Manager", "Диспетчер скачанных карт", "Диспетчер завантажених карт", "Menedżer pobranych map"),
  L("Updates offline maps for the Maps app.", "Обновляет офлайн-карты приложения «Карты».", "Оновлює офлайн-карти застосунку «Карти».", "Aktualizuje mapy offline aplikacji Mapy."),
  [("MapsBroker", "delayed-auto")])
S("svc_wmpnet", L("Media Player network sharing", "Общий доступ Windows Media Player", "Спільний доступ Windows Media Player", "Udostępnianie sieciowe Media Player"),
  L("Shares your media library with other devices over the network.", "Раздаёт медиатеку другим устройствам в сети.", "Роздає медіатеку іншим пристроям у мережі.", "Udostępnia bibliotekę multimediów innym urządzeniom w sieci."),
  [("WMPNetworkSvc", "demand")])
S("svc_insider", L("Windows Insider service", "Служба предварительной оценки", "Служба попередньої оцінки", "Usługa Windows Insider"),
  L("Only needed if you're in the Windows Insider Program.", "Нужна только участникам программы Windows Insider.", "Потрібна лише учасникам програми Windows Insider.", "Potrzebna tylko uczestnikom programu Windows Insider."),
  [("wisvc", "demand")])
S("svc_trkwks", L("Distributed Link Tracking", "Отслеживание изменившихся связей", "Відстеження змінених зв'язків", "Śledzenie łączy rozproszonych"),
  L("Tracks shortcuts to files moved across network computers. Useless on a home PC.", "Отслеживает ярлыки на файлы, перемещённые между компьютерами сети. Бесполезна дома.", "Відстежує ярлики на файли, переміщені між комп'ютерами мережі. Марна вдома.", "Śledzi skróty do plików przenoszonych między komputerami w sieci. Zbędna w domu."),
  [("TrkWks", "auto")])
S("svc_wpc", L("Parental Controls", "Родительский контроль", "Батьківський контроль", "Kontrola rodzicielska"),
  L("Enforces Microsoft Family limits. Keep it if a child account uses this PC.", "Применяет ограничения Microsoft Family. Оставьте, если ПК пользуется ребёнок.", "Застосовує обмеження Microsoft Family. Залиште, якщо ПК користується дитина.", "Egzekwuje limity Microsoft Family. Zostaw, jeśli korzysta z niego dziecko."),
  [("WpcMonSvc", "demand")], rec=False)
S("svc_diaghub", L("Diagnostics Hub collector", "Сборщик концентратора диагностики", "Збирач концентратора діагностики", "Kolektor centrum diagnostyki"),
  L("Collects traces for Visual Studio diagnostics. Not needed for gaming.", "Собирает трассировки для диагностики Visual Studio. Для игр не нужна.", "Збирає трасування для діагностики Visual Studio. Для ігор не потрібна.", "Zbiera ślady dla diagnostyki Visual Studio. Niepotrzebna do grania."),
  [("diagnosticshub.standardcollector.service", "demand")])
S("svc_wer", L("Error Reporting service", "Служба отчётов об ошибках", "Служба звітів про помилки", "Usługa raportowania błędów"),
  L("Collects crash reports to send to Microsoft.", "Собирает отчёты о сбоях для отправки в Microsoft.", "Збирає звіти про збої для надсилання до Microsoft.", "Zbiera raporty o awariach do wysłania do Microsoftu."),
  [("WerSvc", "demand")], rec=False)
S("svc_pca", L("Program Compatibility Assistant", "Помощник по совместимости программ", "Помічник із сумісності програм", "Asystent zgodności programów"),
  L("Watches every program you start for old-app compatibility issues.", "Следит за каждой запущенной программой на предмет проблем совместимости.", "Стежить за кожною запущеною програмою щодо проблем сумісності.", "Obserwuje każdy uruchamiany program pod kątem problemów ze zgodnością."),
  [("PcaSvc", "auto")], rec=False, risk="moderate")
S("svc_spooler", L("Print Spooler", "Диспетчер печати", "Диспетчер друку", "Bufor wydruku"),
  L("Needed only for printing. Disabling it also closes the PrintNightmare attack surface.", "Нужен только для печати. Отключение заодно закрывает уязвимости класса PrintNightmare.", "Потрібен лише для друку. Вимкнення заразом закриває вразливості класу PrintNightmare.", "Potrzebny tylko do drukowania. Wyłączenie zamyka też lukę typu PrintNightmare."),
  [("Spooler", "auto")], risk="moderate", when={"noprinter": True},
  warn=L("You won't be able to print or save to PDF via 'Microsoft Print to PDF'.",
         "Печать и «Microsoft Print to PDF» перестанут работать.",
         "Друк і «Microsoft Print to PDF» перестануть працювати.",
         "Drukowanie i „Microsoft Print to PDF” przestaną działać."))
S("svc_xbox", L("Xbox Live services", "Службы Xbox Live", "Служби Xbox Live", "Usługi Xbox Live"),
  L("Xbox sign-in, cloud saves and networking.", "Вход в Xbox, облачные сохранения и сеть.", "Вхід у Xbox, хмарні збереження та мережа.", "Logowanie Xbox, zapisy w chmurze i sieć."),
  [("XblAuthManager", "demand"), ("XblGameSave", "demand"), ("XboxNetApiSvc", "demand")],
  rec=False, risk="moderate",
  warn=L("Breaks PC Game Pass, Xbox app sign-in and cloud saves in Microsoft Store games. Controllers keep working.",
         "Сломает PC Game Pass, вход в приложение Xbox и облачные сохранения игр из Microsoft Store. Геймпады продолжат работать.",
         "Зламає PC Game Pass, вхід у застосунок Xbox і хмарні збереження ігор із Microsoft Store. Геймпади й далі працюватимуть.",
         "Psuje PC Game Pass, logowanie w aplikacji Xbox i zapisy w chmurze gier ze Sklepu. Kontrolery działają dalej."))
S("svc_ndu", L("Network Data Usage monitor", "Монитор использования сетевых данных", "Монітор використання мережевих даних", "Monitor użycia danych sieciowych"),
  L("Counts traffic per app for the Data usage page in Settings.", "Считает трафик каждого приложения для страницы «Использование данных».", "Рахує трафік кожного застосунку для сторінки «Використання даних».", "Liczy ruch każdej aplikacji dla strony Użycie danych w Ustawieniach."),
  [("Ndu", "auto")], rec=False, risk="moderate")
S("svc_remote_registry", L("Remote Registry", "Удалённый реестр", "Віддалений реєстр", "Rejestr zdalny"),
  L("Lets other computers edit this PC's registry over the network. Should never be on at home.", "Позволяет другим компьютерам менять реестр этого ПК по сети. Дома не нужна никогда.", "Дозволяє іншим комп'ютерам змінювати реєстр цього ПК через мережу. Вдома не потрібна ніколи.", "Pozwala innym komputerom zmieniać rejestr tego PC przez sieć. W domu zawsze zbędna."),
  [("RemoteRegistry", "disabled")])

# ============================================================ COMPONENTS (Debloat)
def C(id, name, desc, op, rec=False, risk="safe", warn=None, store=None, restart="pc"):
    T(id, "components", risk, name, desc, [op], rec=rec, imp=1, warn=warn, store=store, restart=restart)


T("comp_onedrive", "components", "moderate",
  L("OneDrive sync", "Синхронизация OneDrive", "Синхронізація OneDrive", "Synchronizacja OneDrive"),
  L("Stops OneDrive from starting and syncing in the background. Your files stay where they are; nothing is deleted.",
    "OneDrive перестаёт запускаться и синхронизировать файлы в фоне. Файлы остаются на месте, ничего не удаляется.",
    "OneDrive перестає запускатися й синхронізувати файли у фоні. Файли залишаються на місці, нічого не видаляється.",
    "OneDrive przestaje uruchamiać się i synchronizować w tle. Pliki zostają na miejscu, nic nie jest usuwane."),
  [reg(POL + r"\OneDrive", "DisableFileSyncNGSC", "dword", 1)],
  imp=1, restart="pc",
  warn=L("Files kept 'online-only' in OneDrive won't open until you turn it back on.",
         "Файлы OneDrive, хранящиеся только в облаке, не откроются, пока не включите обратно.",
         "Файли OneDrive, що зберігаються лише в хмарі, не відкриються, доки не ввімкнете назад.",
         "Pliki OneDrive dostępne tylko online nie otworzą się, dopóki go nie włączysz."))
C("comp_smb1", L("SMB 1.0 file sharing", "Общий доступ SMB 1.0", "Спільний доступ SMB 1.0", "Udostępnianie SMB 1.0"),
  L("A 30-year-old file sharing protocol used by WannaCry. Only very old NAS boxes need it.", "Протокол обмена файлами 30-летней давности, через который распространялся WannaCry. Нужен лишь очень старым NAS.", "Протокол обміну файлами 30-річної давнини, через який поширювався WannaCry. Потрібен лише дуже старим NAS.", "30-letni protokół udostępniania plików wykorzystany przez WannaCry. Potrzebny tylko bardzo starym NAS."),
  feature("SMB1Protocol"), rec=True)
C("comp_ps2", L("Windows PowerShell 2.0", "Windows PowerShell 2.0", "Windows PowerShell 2.0", "Windows PowerShell 2.0"),
  L("Legacy engine attackers use to bypass modern PowerShell logging. Current PowerShell is unaffected.", "Устаревший движок, через который злоумышленники обходят журналирование PowerShell. Современный PowerShell не затрагивается.", "Застарілий рушій, через який зловмисники обходять журналювання PowerShell. Сучасний PowerShell не зачіпається.", "Przestarzały silnik, którym atakujący omijają logowanie PowerShell. Obecny PowerShell działa dalej."),
  feature("MicrosoftWindowsPowerShellV2Root"), rec=True)
C("comp_ie", L("Internet Explorer 11", "Internet Explorer 11", "Internet Explorer 11", "Internet Explorer 11"),
  L("Retired browser still installed on Windows 10.", "Устаревший браузер, всё ещё установленный в Windows 10.", "Застарілий браузер, досі встановлений у Windows 10.", "Wycofana przeglądarka wciąż zainstalowana w Windows 10."),
  feature("Internet-Explorer-Optional-amd64"))
C("comp_wmp", L("Windows Media Player Legacy", "Windows Media Player (старый)", "Windows Media Player (старий)", "Windows Media Player (starszy)"),
  L("The old Media Player. The new Media Player app is not affected.", "Старый проигрыватель. Новое приложение «Медиаплеер» не затрагивается.", "Старий програвач. Новий застосунок «Медіапрогравач» не зачіпається.", "Stary odtwarzacz. Nowa aplikacja Odtwarzacz multimedialny działa dalej."),
  feature("WindowsMediaPlayer"))
C("comp_xps", L("XPS Services", "Службы XPS", "Служби XPS", "Usługi XPS"),
  L("Support for Microsoft's XPS document format that almost nobody uses.", "Поддержка формата документов XPS, которым почти никто не пользуется.", "Підтримка формату документів XPS, яким майже ніхто не користується.", "Obsługa formatu dokumentów XPS, którego prawie nikt nie używa."),
  feature("Printing-XPSServices-Features"))
C("comp_workfolders", L("Work Folders client", "Клиент рабочих папок", "Клієнт робочих тек", "Klient Folderów roboczych"),
  L("Syncs folders with a corporate Windows Server.", "Синхронизирует папки с корпоративным Windows Server.", "Синхронізує теки з корпоративним Windows Server.", "Synchronizuje foldery z firmowym Windows Server."),
  feature("WorkFolders-Client"))
C("comp_recall", L("Recall", "Recall", "Recall", "Recall"),
  L("Removes the Windows AI Recall feature on Copilot+ PCs.", "Удаляет функцию Recall на ПК Copilot+.", "Видаляє функцію Recall на ПК Copilot+.", "Usuwa funkcję Recall na komputerach Copilot+."),
  feature("Recall"), rec=True)
C("comp_steps", L("Steps Recorder", "Средство записи действий", "Засіб записування дій", "Rejestrator kroków"),
  L("Old screen-steps recording tool, deprecated by Microsoft.", "Старая утилита записи действий, Microsoft больше её не развивает.", "Стара утиліта записування дій, Microsoft більше її не розвиває.", "Stare narzędzie do nagrywania kroków, wycofane przez Microsoft."),
  cap("App.StepsRecorder"), store="Steps Recorder")
C("comp_math", L("Math Recognizer", "Распознаватель математики", "Розпізнавач математики", "Rozpoznawanie matematyki"),
  L("Handwriting recognition for math formulas.", "Распознавание рукописных математических формул.", "Розпізнавання рукописних математичних формул.", "Rozpoznawanie odręcznych wzorów matematycznych."),
  cap("MathRecognizer"))
C("comp_wordpad", L("WordPad", "WordPad", "WordPad", "WordPad"),
  L("Discontinued text editor.", "Снятый с поддержки текстовый редактор.", "Знятий із підтримки текстовий редактор.", "Wycofany edytor tekstu."),
  cap("Microsoft.Windows.WordPad"))
C("comp_faxscan", L("Windows Fax and Scan", "Факсы и сканирование", "Факси та сканування", "Faksowanie i skanowanie"),
  L("Fax and scanner app. Your scanner's own software keeps working.", "Приложение для факса и сканера. ПО вашего сканера продолжит работать.", "Застосунок для факсу та сканера. ПЗ вашого сканера й далі працюватиме.", "Aplikacja do faksu i skanera. Oprogramowanie skanera działa dalej."),
  cap("Print.Fax.Scan"))

# ============================================================ APPS (Debloat)
def A(id, name, patterns, cat="apps", rec=True, warn=None, risk="safe", store=None):
    desc = L("Preinstalled app. Can be reinstalled from Microsoft Store.",
             "Предустановленное приложение. Можно вернуть из Microsoft Store.",
             "Передвстановлений застосунок. Можна повернути з Microsoft Store.",
             "Preinstalowana aplikacja. Można ją przywrócić ze Sklepu Microsoft.") if cat == "apps" else \
        L("Sponsored app pushed by Microsoft or your PC maker.",
          "Спонсорское приложение от Microsoft или производителя ПК.",
          "Спонсорський застосунок від Microsoft або виробника ПК.",
          "Sponsorowana aplikacja od Microsoftu lub producenta PC.")
    if isinstance(name, str):
        name = L(name, name, name, name)
    T(id, cat, risk, name, desc, [appx(*patterns)], rec=rec, imp=1, warn=warn,
      store=store or name["en"])


A("app_news", L("Microsoft News", "Microsoft Новости", "Microsoft Новини", "Microsoft Wiadomości"), ["Microsoft.BingNews"])
A("app_weather", L("Weather", "Погода", "Погода", "Pogoda"), ["Microsoft.BingWeather"])
A("app_gethelp", L("Get Help", "Техническая поддержка", "Технічна підтримка", "Uzyskaj pomoc"), ["Microsoft.GetHelp"])
A("app_tips", L("Tips", "Советы", "Поради", "Porady"), ["Microsoft.Getstarted"])
A("app_officehub", "Microsoft 365 (Office)", ["Microsoft.MicrosoftOfficeHub"])
A("app_solitaire", "Solitaire & Casual Games", ["Microsoft.MicrosoftSolitaireCollection"])
A("app_people", L("People", "Люди", "Люди", "Kontakty"), ["Microsoft.People"])
A("app_pad", "Power Automate", ["Microsoft.PowerAutomateDesktop"])
A("app_todo", "Microsoft To Do", ["Microsoft.Todos"])
A("app_feedback", L("Feedback Hub", "Центр отзывов", "Центр відгуків", "Centrum opinii"), ["Microsoft.WindowsFeedbackHub"])
A("app_maps", L("Maps", "Карты", "Карти", "Mapy"), ["Microsoft.WindowsMaps"])
A("app_movies", L("Movies & TV", "Кино и ТВ", "Кіно й ТБ", "Filmy i TV"), ["Microsoft.ZuneVideo"])
A("app_clipchamp", "Clipchamp", ["Clipchamp.Clipchamp"])
A("app_cortana", "Cortana", ["Microsoft.549981C3F5F10"])
A("app_teams", "Microsoft Teams (personal)", ["MicrosoftTeams", "MSTeams"])
A("app_skype", "Skype", ["Microsoft.SkypeApp"])
A("app_mail", L("Mail and Calendar", "Почта и Календарь", "Пошта й Календар", "Poczta i Kalendarz"), ["microsoft.windowscommunicationsapps"])
A("app_outlook", "Outlook (new)", ["Microsoft.OutlookForWindows"], rec=False)
A("app_devhome", "Dev Home", ["Microsoft.Windows.DevHome"])
A("app_3dviewer", "3D Viewer", ["Microsoft.Microsoft3DViewer"])
A("app_mixedreality", "Mixed Reality Portal", ["Microsoft.MixedReality.Portal"])
A("app_paint3d", "Paint 3D", ["Microsoft.MSPaint"])
A("app_onenote", "OneNote for Windows 10", ["Microsoft.Office.OneNote"], rec=False)
A("app_bingsearch", "Bing Search", ["Microsoft.BingSearch"])
A("app_copilot", "Copilot", ["Microsoft.Copilot", "Microsoft.Windows.Ai.Copilot.Provider"])
A("app_family", "Microsoft Family", ["MicrosoftCorporationII.MicrosoftFamily"])
A("app_soundrec", L("Sound Recorder", "Звукозапись", "Звукозапис", "Rejestrator dźwięku"), ["Microsoft.WindowsSoundRecorder"], rec=False)
A("app_alarms", L("Clock (Alarms)", "Часы (Будильник)", "Годинник (Будильник)", "Zegar (Alarmy)"), ["Microsoft.WindowsAlarms"], rec=False)
A("app_phonelink", L("Phone Link", "Связь с телефоном", "Зв'язок із телефоном", "Łącze z telefonem"), ["Microsoft.YourPhone"], rec=False)
A("app_xbox_speech", "Xbox Speech to Text", ["Microsoft.XboxSpeechToTextOverlay"])
A("app_gamebar", "Xbox Game Bar", ["Microsoft.XboxGamingOverlay"], rec=False, risk="moderate",
  warn=L("Removes the Win+G overlay, its FPS counter and screen capture.",
         "Пропадут оверлей Win+G, его счётчик FPS и запись экрана.",
         "Зникнуть оверлей Win+G, його лічильник FPS і запис екрана.",
         "Zniknie nakładka Win+G, jej licznik FPS i nagrywanie ekranu."))
A("app_xbox", "Xbox app", ["Microsoft.GamingApp", "Microsoft.XboxApp"], rec=False, risk="moderate",
  warn=L("You need the Xbox app to play PC Game Pass games.",
         "Приложение Xbox нужно для игр PC Game Pass.",
         "Застосунок Xbox потрібен для ігор PC Game Pass.",
         "Aplikacja Xbox jest potrzebna do gier PC Game Pass."))

A("sp_candy", "Candy Crush (King)", ["king.com.*"], cat="sponsored")
A("sp_tiktok", "TikTok", ["*TikTok*"], cat="sponsored")
A("sp_disney", "Disney+", ["*Disney*"], cat="sponsored")
A("sp_netflix", "Netflix", ["*Netflix*"], cat="sponsored", rec=False)
A("sp_prime", "Prime Video", ["AmazonVideo.PrimeVideo"], cat="sponsored")
A("sp_facebook", "Facebook", ["*Facebook*"], cat="sponsored")
A("sp_instagram", "Instagram", ["*Instagram*"], cat="sponsored")
A("sp_linkedin", "LinkedIn", ["*LinkedInforWindows*"], cat="sponsored")
A("sp_spotify", "Spotify", ["SpotifyAB.SpotifyMusic"], cat="sponsored", rec=False)
A("sp_casual", L("Sponsored casual games", "Спонсорские казуальные игры", "Спонсорські казуальні ігри", "Sponsorowane gry casual"),
  ["*BubbleWitch*", "*MarchofEmpires*", "*HiddenCity*", "*CookingFever*", "*FarmVille*", "*Royal Revolt*", "*DisneyMagicKingdoms*"], cat="sponsored")
A("sp_messenger", "Messenger", ["*FacebookMessenger*", "*Messenger*"], cat="sponsored", rec=False)
A("sp_ecosystem", L("PC maker promo apps", "Промо-приложения производителя", "Промозастосунки виробника", "Aplikacje promocyjne producenta"),
  ["*McAfee*", "*Booking*", "*ExpressVPN*", "*Dropbox*Promotion*"], cat="sponsored")


# ============================================================ PRESETS
PRESETS = [
    {"id": "recommended", "tweaks": "auto:rec",
     "name": L("Recommended", "Рекомендуемые", "Рекомендовані", "Zalecane"),
     "desc": L("Safe tweaks picked for your hardware. A great first step.",
               "Безопасные твики, подобранные под ваше железо. Лучший первый шаг.",
               "Безпечні твіки, дібрані під ваше залізо. Найкращий перший крок.",
               "Bezpieczne zmiany dobrane do twojego sprzętu. Najlepszy pierwszy krok.")},
    {"id": "competitive", "tweaks": "auto:competitive",
     "name": L("Competitive", "Соревновательный", "Змагальний", "Rywalizacja"),
     "desc": L("Recommended plus extra latency and network tweaks for shooters and MOBAs.",
               "Рекомендуемые плюс дополнительные твики задержки и сети для шутеров и MOBA.",
               "Рекомендовані плюс додаткові твіки затримки та мережі для шутерів і MOBA.",
               "Zalecane oraz dodatkowe zmiany opóźnień i sieci dla strzelanek i MOBA.")},
    {"id": "privacy", "tweaks": "auto:privacy",
     "name": L("Privacy", "Приватность", "Приватність", "Prywatność"),
     "desc": L("Every privacy and telemetry tweak that doesn't reduce security.",
               "Все твики приватности и телеметрии, которые не снижают безопасность.",
               "Усі твіки приватності й телеметрії, які не знижують безпеку.",
               "Wszystkie zmiany prywatności i telemetrii, które nie obniżają bezpieczeństwa.")},
]
COMPETITIVE_EXTRA = ["fso_off", "background_apps_off", "timer_res_global", "msi_gpu", "hags_on",
                     "search_index_off", "keyboard_fast", "qos_limit_off", "cpu_min_100"]


EXTRA_LANGS = ("es", "de", "fr", "ja", "cs")  # 1.1.1: translations keyed by the English text


def add_languages(node):
    if isinstance(node, list):
        for x in node:
            add_languages(x)
        return
    if not isinstance(node, dict):
        return
    if isinstance(node.get("en"), str) and "ru" in node:
        for lang in EXTRA_LANGS:
            tr = TRANSLATIONS[lang].get(node["en"])
            assert tr, ("missing", lang, node["en"][:60])
            node[lang] = tr
        return
    for v in node.values():
        add_languages(v)


TRANSLATIONS = {}
for _l in EXTRA_LANGS:
    with open(os.path.join(os.path.dirname(__file__), "catalog_i18n", _l + ".json"), encoding="utf-8") as _f:
        TRANSLATIONS[_l] = json.load(_f)


def main():
    ids = [t["id"] for t in TWEAKS]
    assert len(ids) == len(set(ids)), "duplicate ids"
    for t in TWEAKS:
        for f in ("name", "desc"):
            for lang in ("en", "ru", "uk", "pl"):
                assert t[f].get(lang), (t["id"], f, lang)
        for op in t["ops"]:
            if op["t"] == "ps":
                for k in ("apply", "revert", "check"):
                    assert all(ord(c) < 128 for c in op[k]), (t["id"], "non-ascii PowerShell")
    PRESETS[1]["extra"] = COMPETITIVE_EXTRA
    data = {"version": 1, "tweaks": TWEAKS, "presets": PRESETS}
    add_languages(data)
    with open(OUT, "w", encoding="utf-8") as f:
        json.dump(data, f, ensure_ascii=False, indent=1)
    cats = {}
    for t in TWEAKS:
        cats[t["cat"]] = cats.get(t["cat"], 0) + 1
    print("tweaks:", len(TWEAKS), cats, file=sys.stderr)


if __name__ == "__main__":
    main()
