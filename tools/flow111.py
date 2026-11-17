# UI flow for Larik 1.1.1 (settings window: Language category, no motion switch, 9 languages), CSP enforced.
import json, sys, time, urllib.request
from playwright.sync_api import sync_playwright

BASE = "http://127.0.0.1:8765"
OUT = sys.argv[1] if len(sys.argv) > 1 else "tools/shots"
LANG = sys.argv[2] if len(sys.argv) > 2 else "ru"

def call(path, body=None):
    req = urllib.request.Request(BASE + path, data=None if body is None else json.dumps(body).encode(),
                                 method="GET" if body is None else "POST",
                                 headers={"X-Larik-Token": "test", "Content-Type": "application/json"})
    return json.load(urllib.request.urlopen(req))

call("/api/auth/skip", {})
VER = call("/api/bootstrap")["version"]
call("/api/settings", {"onboarded": True, "acctAsked": True, "seenVersion": VER, "lang": LANG})
for f in call("/api/games/detect", {})["found"]:
    if f["exe"] in ("cs2.exe", "dota2.exe", "Among Us.exe", "FortniteClient-Win64-Shipping.exe"):
        call("/api/games/add", f)

errors = []
with sync_playwright() as p:
    b = p.chromium.launch()
    pg = b.new_page(viewport={"width": 1440, "height": 960})
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    pg.on("pageerror", lambda e: errors.append(str(e)))
    pg.goto(BASE + "/#t=test")
    pg.wait_for_selector(".rail-item[data-page=home]")
    time.sleep(1.5)
    assert pg.locator(".ql-item").count() == 0, "play row still on Home"
    pg.click(".rail-item[data-page=games]")
    pg.wait_for_selector(".play-lib .ql-item")
    print("games posters:", pg.locator(".play-lib .ql-item").count(), "table play buttons:", pg.locator(".game .play-btn").count())
    pg.screenshot(path=f"{OUT}/games-110.png")
    # settings window over the Games page
    pg.click(".rail-item[data-page=settings]")
    pg.wait_for_selector(".set-layer.on #set-theme")
    time.sleep(0.4)
    pg.screenshot(path=f"{OUT}/settings-appearance.png")
    assert pg.locator("#sw-body #set-lang").count() == 0, "language still under Appearance"
    assert pg.locator("[data-a=sw-sub]").count() == 0, "Appearance still has sub-sections"
    assert pg.locator("[data-a=lang-menu]").count() == 0, "language button still in the top bar"
    assert "reduced" not in (pg.get_attribute("html", "class") or ""), "animations off"
    pg.click("[data-a=theme][data-v=ocean]")
    time.sleep(0.6)
    pg.fill("#sw-q", "FPS")
    time.sleep(0.4)
    items = pg.locator("#sw-list .sw-item").all_inner_texts()
    print("search FPS:", items)
    pg.screenshot(path=f"{OUT}/settings-search.png")
    pg.fill("#sw-q", "")
    pg.click("[data-a=sw-cat][data-v=overlay]")
    pg.wait_for_selector("#set-ov-metrics")
    pg.click("[data-a=ov-flag][data-k=enabled]")
    time.sleep(1.2)
    top = pg.evaluate("document.getElementById('sw-body').scrollTop")
    pg.locator("#set-ov-profiles").scroll_into_view_if_needed()
    time.sleep(0.8)
    pg.screenshot(path=f"{OUT}/settings-overlay.png")
    pg.click("[data-a=sw-cat][data-v=account]")
    time.sleep(0.4)
    pg.screenshot(path=f"{OUT}/settings-account.png")
    pg.keyboard.press("Escape")
    pg.wait_for_selector(".set-layer.on", state="hidden")
    assert pg.locator(".play-lib").count() == 1, "background page lost"
    s = call("/api/settings")
    assert s["theme"] == "ocean" and s["overlay"]["enabled"] is True, (s["theme"], s["overlay"]["enabled"])
    # reopen in English: the window rebuilds in the new language
    pg.click(".rail-item[data-page=settings]")
    pg.click("[data-a=sw-cat][data-v=lang]")
    pg.wait_for_selector("#sw-body #set-lang")
    codes = pg.locator("#sw-body [data-a=lang]").evaluate_all("els => els.map(e => e.dataset.v)")
    print("languages:", codes)
    assert codes == ["en", "uk", "ru", "pl", "es", "de", "fr", "ja", "cs"], codes
    pg.screenshot(path=f"{OUT}/settings-lang.png")
    for code in ("es", "de", "fr", "ja", "cs", "en"):
        pg.click(f"[data-a=lang][data-v={code}]")
        time.sleep(0.6)
        assert call("/api/settings")["lang"] == code
        print(code, "->", pg.locator("#sw-title").inner_text(), "|", " · ".join(pg.locator("#sw-list .sw-item").all_inner_texts()[:4]))
        if code in ("ja", "de"):
            pg.screenshot(path=f"{OUT}/settings-lang-{code}.png")
    pg.click("[data-a=sw-close]")
    pg.wait_for_selector(".set-layer.on", state="hidden")
    # every new language has every English key
    miss = pg.evaluate("""async () => { const m = await import('/js/i18n.js'); const en = Object.keys(m.STRINGS.en);
        return Object.fromEntries(Object.keys(m.STRINGS).map(l => [l, en.filter(k => !(k in m.STRINGS[l])).length])); }""")
    print("missing keys per language:", miss)
    assert all(v == 0 for k, v in miss.items() if k in ("es", "de", "fr", "ja", "cs")), miss
    b.close()

errors = [e for e in errors if "favicon" not in e]
print("console errors:", errors if errors else "none")
sys.exit(1 if errors else 0)
