# UI flow for Larik 1.0.7 against the demo server (CSP enforced).
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
post("/api/settings", {"onboarded": True, "acctAsked": True, "seenVersion": VER, "lang": LANG,
                       "overlay": {"enabled": True}})

errors = []
with sync_playwright() as p:
    b = p.chromium.launch()
    ctx = b.new_context(viewport={"width": 1440, "height": 960}, accept_downloads=True)
    pg = ctx.new_page()
    pg.on("console", lambda m: errors.append(m.text) if m.type == "error" else None)
    pg.on("pageerror", lambda e: errors.append(str(e)))
    pg.goto(BASE + "/#t=test")
    pg.wait_for_selector(".rail-item[data-page=health]")
    time.sleep(2.5)
    pg.click(".rail-item[data-page=health]")
    pg.wait_for_selector(".disk-card")
    pg.click("[data-a=drv-check]")
    pg.wait_for_selector(".drv-latest")
    pg.screenshot(path=f"{OUT}/health-top.png")
    pg.click("[data-a=net-run]")
    pg.wait_for_selector(".net-prog")
    time.sleep(3)
    pg.screenshot(path=f"{OUT}/net-running.png")
    pg.wait_for_selector(".net-tips", timeout=60000)
    pg.click("[data-a=cln-toggle]")
    time.sleep(0.6)
    pg.click("[data-a=cln-day][data-v='6']")
    time.sleep(0.6)
    pg.select_option("[data-cln-hour]", "21")
    time.sleep(0.8)
    pg.click("[data-a=cln-run]")
    pg.wait_for_selector(".cln-run")
    sec = pg.locator("section.sec").nth(1)
    sec.scroll_into_view_if_needed()
    pg.screenshot(path=f"{OUT}/health-bottom.png", full_page=True)
    s = json.load(urllib.request.urlopen(urllib.request.Request(BASE + "/api/settings", headers={"X-Larik-Token": "test"})))
    assert s["autoClean"] == {"enabled": True, "day": 6, "hour": 21}, s["autoClean"]

    # Boost: automatic game mode + NVIDIA
    pg.click(".rail-item[data-page=boost]")
    pg.wait_for_selector(".auto-game")
    pg.click("[data-a=ag-toggle]")
    time.sleep(0.8)
    pg.wait_for_selector(".nv-row")
    pg.click("[data-a=nv-set][data-k=power]")
    time.sleep(0.6)
    pg.click("[data-a=nv-set][data-k=latency]")
    time.sleep(0.6)
    pg.fill("#nv-fps", "57")
    pg.click("button[data-a=nv-fps]:not([data-v])")
    time.sleep(0.8)
    pg.locator(".auto-game").scroll_into_view_if_needed()
    pg.screenshot(path=f"{OUT}/boost-auto-nv.png", full_page=True)
    nv = json.load(urllib.request.urlopen(urllib.request.Request(BASE + "/api/nvidia", headers={"X-Larik-Token": "test"})))
    got = {x["key"]: (x["value"], x["ours"]) for x in nv["settings"]}
    assert got["power"] == (1, True) and got["latency"] == (1, True) and got["fpslimit"] == (57, True), got
    s = json.load(urllib.request.urlopen(urllib.request.Request(BASE + "/api/settings", headers={"X-Larik-Token": "test"})))
    assert s["autoGame"]["enabled"] is True

    # Games: before / after
    pg.click(".rail-item[data-page=games]")
    pg.wait_for_selector("[data-a=cmp-open]")
    pg.click("[data-a=cmp-open]")
    pg.wait_for_selector(".cmp-tiles")
    pg.locator("#cmp").scroll_into_view_if_needed()
    time.sleep(0.5)
    pg.screenshot(path=f"{OUT}/compare.png")
    pg.locator("#cmp").screenshot(path=f"{OUT}/compare-card.png")
    # switch "after" to Fortnite: only one session -> hint
    opts = pg.locator("select[data-cmp=b] option").all()
    fort = [o.get_attribute("value") for o in opts if "fortnite" in o.get_attribute("value")][0]
    pg.select_option("select[data-cmp=b]", fort)
    pg.wait_for_selector(".cmp .empty")
    pg.click("[data-a=cmp-close]")

    # Settings: overlay profile, updates, export/import
    pg.click(".rail-item[data-page=settings]")
    pg.wait_for_selector(".set-layer.on")
    pg.click("[data-a=sw-cat][data-v=overlay]")
    pg.wait_for_selector("[data-a=pf-add]")
    pg.click("[data-a=pf-add]")
    pg.wait_for_selector("[data-pick]")
    pg.click("[data-pick='cs2.exe']")
    pg.wait_for_selector(".pf-metrics")
    pg.click(".pf-metrics [data-m=ping]")
    pg.click(".pf-metrics [data-m=ftgraph]")
    pg.click(".pf-screen [data-pos=bl]")
    pg.click("[data-pf-seg=layout] [data-v=column]")
    pg.screenshot(path=f"{OUT}/profile-editor.png")
    pg.click(".modal-foot .btn.primary")
    pg.wait_for_selector(".pf-row")
    s = json.load(urllib.request.urlopen(urllib.request.Request(BASE + "/api/settings", headers={"X-Larik-Token": "test"})))
    pf = s["overlay"]["profiles"][0]
    assert pf["exe"] == "cs2.exe" and pf["position"] == "bl" and pf["layout"] == "column" and "ping" in pf["metrics"], pf
    pg.locator(".ov-profiles").scroll_into_view_if_needed()
    pg.screenshot(path=f"{OUT}/profiles.png")

    pg.click("[data-a=sw-cat][data-v=updates]")
    pg.wait_for_selector("#upd-repo")
    pg.fill("#upd-repo", "https://github.com/someone/larik")
    pg.click("[data-a=upd-repo]")
    time.sleep(0.6)
    assert pg.input_value("#upd-repo") == "someone/larik"
    pg.click("[data-a=upd-check]")
    pg.wait_for_selector(".upd-new")
    pg.locator(".upd-new").screenshot(path=f"{OUT}/updates.png")
    pg.click("[data-a=sw-cat][data-v=backup]")
    pg.wait_for_selector("[data-a=io-export]")
    with pg.expect_download() as dl:
        pg.click("[data-a=io-export]")
    path = dl.value.path()
    exp = json.load(open(path))
    assert exp["app"] == "OpLarik" and exp["nvidia"]["fpslimit"] == 57
    pg.screenshot(path=f"{OUT}/updates-io.png")
    # import into a changed setup: turn the NVIDIA limit off first
    post("/api/nvidia", {"key": "fpslimit", "on": False})
    with pg.expect_file_chooser() as fc:
        pg.click("[data-a=io-import]")
    fc.value.set_files(path)
    pg.wait_for_selector(".modal .mini-list")
    pg.screenshot(path=f"{OUT}/import.png")
    pg.click(".modal-foot .btn.primary")
    time.sleep(1.5)
    nv = json.load(urllib.request.urlopen(urllib.request.Request(BASE + "/api/nvidia", headers={"X-Larik-Token": "test"})))
    assert {x["key"]: x["value"] for x in nv["settings"]}["fpslimit"] == 57
    # what's new
    pg.keyboard.press("Escape")
    pg.wait_for_selector(".set-layer.on", state="hidden")
    pg.click(".rail [data-a=changelog]")
    pg.wait_for_selector(".cl")
    pg.screenshot(path=f"{OUT}/whats-new.png")
    b.close()

errors = [e for e in errors if "favicon" not in e]
print("console errors:", errors if errors else "none")
sys.exit(1 if errors else 0)
