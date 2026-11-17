# UI flow for Larik 1.0.8 (pictures, hardware tips) against the demo server, CSP enforced.
import json, sys, time, urllib.request
from playwright.sync_api import sync_playwright

BASE = "http://127.0.0.1:8765"
OUT = sys.argv[1] if len(sys.argv) > 1 else "tools/shots"
LANG = sys.argv[2] if len(sys.argv) > 2 else "ru"

def post(path, body):
    req = urllib.request.Request(BASE + path, data=json.dumps(body).encode(), method="POST",
                                 headers={"X-Larik-Token": "test", "Content-Type": "application/json"})
    return json.load(urllib.request.urlopen(req))

post("/api/auth/skip", {})
VER = json.load(urllib.request.urlopen(urllib.request.Request(BASE + "/api/bootstrap", headers={"X-Larik-Token": "test"})))["version"]
post("/api/settings", {"onboarded": True, "acctAsked": True, "seenVersion": VER, "lang": LANG})
errors, bad = [], []
with sync_playwright() as p:
    b = p.chromium.launch()
    pg = b.new_page(viewport={"width": 1440, "height": 960})
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    pg.on("pageerror", lambda e: errors.append(str(e)))
    pg.on("response", lambda r: bad.append(r.url) if "/api/cover/" in r.url and r.status != 200 else None)
    pg.goto(BASE + "/#t=test")
    pg.wait_for_selector(".rail-item[data-page=games]")
    time.sleep(2.5)
    pg.screenshot(path=f"{OUT}/home-108.png")
    tips = pg.locator(".tip h4").all_inner_texts()
    print("home tips:", tips)
    pg.click(".rail-item[data-page=games]")
    pg.click("[data-a=game-scan]")
    pg.wait_for_selector(".found-item .cover.big")
    pg.click("[data-a=game-add][data-i='1']")
    time.sleep(0.6)
    pg.click("[data-a=game-add][data-i='7']")
    time.sleep(1.5)
    n_img = pg.locator(".found .cover.pic img").count()
    pg.screenshot(path=f"{OUT}/games-108.png", full_page=True)
    assert n_img >= 8, n_img
    pg.locator(".ses").first.scroll_into_view_if_needed()
    time.sleep(0.5)
    pg.screenshot(path=f"{OUT}/sessions-108.png")
    pg.click(".rail-item[data-page=startup]")
    pg.wait_for_selector(".su-row")
    time.sleep(1)
    pg.screenshot(path=f"{OUT}/startup-108.png")
    pg.click(".rail-item[data-page=system]")
    pg.wait_for_selector(".specs")
    time.sleep(0.5)
    pg.screenshot(path=f"{OUT}/system-108.png", full_page=True)
    pg.click(".rail-item[data-page=health]")
    pg.wait_for_selector(".disk-card")
    time.sleep(0.5)
    medias = pg.locator(".disk-card .ses-head p").all_inner_texts()
    print("disks:", medias)
    assert all(("HDD" in m or "SSD" in m) for m in medias), medias
    b.close()

print("bad covers:", bad)
errors = [e for e in errors if "favicon" not in e and "404" not in e]
print("console errors:", errors if errors else "none")
sys.exit(1 if errors else 0)
