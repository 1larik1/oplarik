# UI flow for Larik 1.1.2 (light themes, overlay follows the theme, no overlay sub-sections), CSP enforced.
import json, sys, time, urllib.request
from playwright.sync_api import sync_playwright

BASE = "http://127.0.0.1:8765"
OUT = sys.argv[1] if len(sys.argv) > 1 else "tools/shots"

def call(path, body=None):
    req = urllib.request.Request(BASE + path, data=None if body is None else json.dumps(body).encode(),
                                 method="GET" if body is None else "POST",
                                 headers={"X-Larik-Token": "test", "Content-Type": "application/json"})
    return json.load(urllib.request.urlopen(req))

def raw(path):
    req = urllib.request.Request(BASE + path, headers={"X-Larik-Token": "test"})
    return urllib.request.urlopen(req).read()

call("/api/auth/skip", {})
VER = call("/api/bootstrap")["version"]
call("/api/settings", {"onboarded": True, "acctAsked": True, "seenVersion": VER, "lang": "ru", "theme": "violet"})

errors = []
with sync_playwright() as p:
    b = p.chromium.launch()
    pg = b.new_page(viewport={"width": 1440, "height": 960})
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    pg.on("pageerror", lambda e: errors.append(str(e)))
    pg.goto(BASE + "/#t=test")
    pg.wait_for_selector(".rail-item[data-page=home]")
    time.sleep(1)
    pg.click(".rail-item[data-page=settings]")
    pg.wait_for_selector(".set-layer.on #set-theme")
    assert pg.locator("#set-theme .theme").count() == 14, pg.locator("#set-theme .theme").count()
    print("groups:", pg.locator("#set-theme .th-group").all_inner_texts())
    pg.screenshot(path=f"{OUT}/themes-112.png")
    previews = {}
    for th in ("violet", "ocean", "light", "sky", "lagoon", "mint", "sand", "rose", "paper"):
        pg.click(f"[data-a=theme][data-v={th}]")
        time.sleep(0.5)
        tone = pg.get_attribute("html", "data-tone")
        bg = pg.evaluate("getComputedStyle(document.body).backgroundColor")
        assert call("/api/settings")["theme"] == th
        previews[th] = raw("/api/overlay/preview.png?t=test")
        print(th, tone, bg)
        assert tone == ("dark" if th in ("violet", "ocean") else "light"), (th, tone)
        if th in ("sky", "sand"):
            pg.screenshot(path=f"{OUT}/settings-{th}.png")
    assert len(set(previews.values())) == len(previews), "overlay preview does not follow the theme"
    pg.click("[data-a=sw-cat][data-v=overlay]")
    pg.wait_for_selector("#set-ov-metrics")
    assert pg.locator("[data-a=sw-sub]").count() == 0, "overlay still has sub-sections"
    time.sleep(1)
    pg.screenshot(path=f"{OUT}/settings-overlay-112.png")
    pg.keyboard.press("Escape")
    time.sleep(0.6)
    pg.screenshot(path=f"{OUT}/home-sand.png")
    # reload keeps the light tone before scripts run (theme-boot)
    pg.reload()
    pg.wait_for_selector(".rail-item[data-page=home]")
    assert pg.get_attribute("html", "data-tone") == "light"
    b.close()

errors = [e for e in errors if "favicon" not in e]
print("console errors:", errors if errors else "none")
sys.exit(1 if errors else 0)
