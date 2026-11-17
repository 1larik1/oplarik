package main

type LiveStats struct {
	CPU      float64 `json:"cpu"`
	RAMUsed  uint64  `json:"ramUsed"`
	RAMTotal uint64  `json:"ramTotal"`
	GPU      float64 `json:"gpu"`
	VRAM     uint64  `json:"vram"`
	GPUTemp  float64 `json:"gpuTemp"`
	CPUTemp  float64 `json:"cpuTemp"`
	At       int64   `json:"at"`
}

type LiveSampler interface {
	Sample() LiveStats
}

// Native dialog strings (installer/uninstaller message boxes).
var nativeText = map[string]map[string]string{
	"en": {
		"title":          "OpLarik",
		"uninstallQ":     "Uninstall OpLarik from this PC?",
		"revertQ":        "OpLarik changed %d Windows settings. Restore them to their original values before uninstalling?\n\nChoose No to keep your optimizations.",
		"uninstallOK":    "OpLarik has been removed.",
		"engineFail":     "OpLarik could not start its engine. Try running OpLarik again, or right-click it and choose 'Run as administrator'.",
		"edgeMissing":    "Microsoft Edge was not found, so OpLarik will open in your default browser.",
		"trayOpen":       "Open OpLarik",
		"trayBoost":      "Game Boost",
		"trayOverlay":    "Show overlay",
		"trayOverlayOff": "Overlay is off in Settings",
		"trayHide":       "Hide this icon",
		"trayBoostOn":    "OpLarik — Game Boost on",
		"trayNeedAdmin":  "Game Boost (needs administrator)",
		"sesTitle":       "Game summary: %s",
		"sesBody":        "%s · avg %s FPS · 1%% low %s · min %s",
		"sesGPU":         " · GPU up to %.0f °C",
		"durH":           "%d h %d min",
		"durM":           "%d min",
		"trayLast":       "Last game: %s, avg %s FPS",
	},
	"ru": {
		"title":          "OpLarik",
		"uninstallQ":     "Удалить OpLarik с этого компьютера?",
		"revertQ":        "OpLarik изменил параметров Windows: %d. Вернуть их исходные значения перед удалением?\n\nНажмите «Нет», чтобы сохранить оптимизации.",
		"uninstallOK":    "OpLarik удалён.",
		"engineFail":     "Не удалось запустить движок OpLarik. Попробуйте открыть OpLarik ещё раз или запустите его от имени администратора.",
		"edgeMissing":    "Microsoft Edge не найден, поэтому OpLarik откроется в браузере по умолчанию.",
		"trayOpen":       "Открыть OpLarik",
		"trayBoost":      "Игровой буст",
		"trayOverlay":    "Показывать оверлей",
		"trayOverlayOff": "Оверлей выключен в настройках",
		"trayHide":       "Убрать значок",
		"trayBoostOn":    "OpLarik — буст включён",
		"trayNeedAdmin":  "Игровой буст (нужны права администратора)",
		"sesTitle":       "Итог игры: %s",
		"sesBody":        "%s · средний %s FPS · 1%% low %s · мин %s",
		"sesGPU":         " · ГП до %.0f °C",
		"durH":           "%d ч %d мин",
		"durM":           "%d мин",
		"trayLast":       "Последняя игра: %s, в среднем %s FPS",
	},
	"uk": {
		"title":          "OpLarik",
		"uninstallQ":     "Видалити OpLarik із цього комп'ютера?",
		"revertQ":        "OpLarik змінив параметрів Windows: %d. Повернути їхні початкові значення перед видаленням?\n\nНатисніть «Ні», щоб зберегти оптимізації.",
		"uninstallOK":    "OpLarik видалено.",
		"engineFail":     "Не вдалося запустити рушій OpLarik. Спробуйте відкрити OpLarik ще раз або запустіть його від імені адміністратора.",
		"edgeMissing":    "Microsoft Edge не знайдено, тому OpLarik відкриється у браузері за замовчуванням.",
		"trayOpen":       "Відкрити OpLarik",
		"trayBoost":      "Ігровий буст",
		"trayOverlay":    "Показувати оверлей",
		"trayOverlayOff": "Оверлей вимкнено в налаштуваннях",
		"trayHide":       "Прибрати значок",
		"trayBoostOn":    "OpLarik — буст увімкнено",
		"trayNeedAdmin":  "Ігровий буст (потрібні права адміністратора)",
		"sesTitle":       "Підсумок гри: %s",
		"sesBody":        "%s · середній %s FPS · 1%% low %s · мін %s",
		"sesGPU":         " · ГП до %.0f °C",
		"durH":           "%d год %d хв",
		"durM":           "%d хв",
		"trayLast":       "Остання гра: %s, у середньому %s FPS",
	},
	"pl": {
		"title":          "OpLarik",
		"uninstallQ":     "Odinstalować OpLarik z tego komputera?",
		"revertQ":        "OpLarik zmienił ustawienia Windows (%d). Przywrócić ich pierwotne wartości przed odinstalowaniem?\n\nWybierz Nie, aby zachować optymalizacje.",
		"uninstallOK":    "OpLarik został usunięty.",
		"engineFail":     "Nie udało się uruchomić silnika OpLarik. Spróbuj ponownie lub uruchom OpLarik jako administrator.",
		"edgeMissing":    "Nie znaleziono Microsoft Edge, więc OpLarik otworzy się w domyślnej przeglądarce.",
		"trayOpen":       "Otwórz OpLarik",
		"trayBoost":      "Game Boost",
		"trayOverlay":    "Pokazuj nakładkę",
		"trayOverlayOff": "Nakładka jest wyłączona w ustawieniach",
		"trayHide":       "Ukryj ikonę",
		"trayBoostOn":    "OpLarik — Game Boost włączony",
		"trayNeedAdmin":  "Game Boost (wymaga administratora)",
		"sesTitle":       "Podsumowanie gry: %s",
		"sesBody":        "%s · średnio %s FPS · 1%% low %s · min %s",
		"sesGPU":         " · GPU do %.0f °C",
		"durH":           "%d h %d min",
		"durM":           "%d min",
		"trayLast":       "Ostatnia gra: %s, średnio %s FPS",
	},
}

func nt(lang, key string) string {
	if m, ok := nativeText[lang]; ok {
		if s, ok := m[key]; ok {
			return s
		}
	}
	return nativeText["en"][key]
}
