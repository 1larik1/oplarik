# UI flow for Larik 1.0.9 (starting games) against the demo server, CSP enforced.
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
found = call("/api/games/detect", {})["found"]
for f in found:
    if f["exe"] in ("cs2.exe", "FortniteClient-Win64-Shipping.exe", "VALORANT-Win64-Shipping.exe", "Among Us.exe", "dota2.exe"):
        call("/api/games/add", f)
# a game added by hand before 1.0.9: Steam path, no launch info
call("/api/games/add", {"name": "PUBG", "exe": "TslGame.exe", "path": r"D:\SteamLibrary\steamapps\common\PUBG\TslGame\Binaries\Win64\TslGame.exe", "source": "manual"})

errors = []
with sync_playwright() as p:
    b = p.chromium.launch()
    pg = b.new_page(viewport={"width": 1440, "height": 960})
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    pg.on("pageerror", lambda e: errors.append(str(e)))
    pg.goto(BASE + "/#t=test")
    pg.wait_for_selector(".rail-item[data-page=games]")
    time.sleep(1)
    pg.click(".rail-item[data-page=games]")
    pg.wait_for_selector(".ql-item")
    time.sleep(1.5)
    n = pg.locator(".ql-item").count()
    first = pg.locator(".ql-meta b").first.inner_text()
    print("quick launch:", n, "first:", first)
    pg.locator(".ql").scroll_into_view_if_needed()
    pg.screenshot(path=f"{OUT}/home-play.png")
    pg.locator(".ql-item .play-btn").first.click()
    pg.wait_for_selector(".toast.good")
    print("toast:", pg.locator(".toast.good").first.inner_text().replace("\n", " | "))
    pg.wait_for_selector(".game")
    games = {g["name"]: g["id"] for g in call("/api/games")["games"]}
    for name in ("Fortnite", "VALORANT", "PUBG"):
        pg.click(f".ql [data-a=game-play][data-id='{games[name]}']")
        time.sleep(0.6)
    time.sleep(1)
    print("error toast:", pg.locator(".toast.bad").first.inner_text().replace("\n", " | "))  # VALORANT: no Riot Client in the demo
    pg.locator(".game").first.scroll_into_view_if_needed()
    pg.screenshot(path=f"{OUT}/games-play.png")
    b.close()

errors = [e for e in errors if "favicon" not in e and "400" not in e]
print("console errors:", errors if errors else "none")
sys.exit(1 if errors else 0)
