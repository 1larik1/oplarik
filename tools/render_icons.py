import asyncio, pathlib, sys
from playwright.async_api import async_playwright
from PIL import Image
root = pathlib.Path(__file__).resolve().parent.parent
svg = (root/'web/img/logo.svg').read_text()
out = root/'rsrc'
async def main():
    async with async_playwright() as p:
        b = await p.chromium.launch()
        pg = await b.new_page()
        for n in (16, 20, 24, 32, 40, 48, 64, 128, 256):
            await pg.set_viewport_size({'width': n, 'height': n})
            await pg.set_content(f'<html><body style="margin:0;background:transparent">{svg.replace("<svg ", f"<svg width=\"{n}\" height=\"{n}\" ", 1)}</body></html>')
            await pg.screenshot(path=str(out/f'icon-{n}.png'), omit_background=True)
        await b.close()
asyncio.run(main())
imgs = [Image.open(out/f'icon-{n}.png').convert('RGBA') for n in (16, 20, 24, 32, 40, 48, 64, 128, 256)]
imgs[-1].save(out/'larik.ico', sizes=[(i.width, i.height) for i in imgs], append_images=imgs[:-1])
import shutil
shutil.copy(out/'icon-64.png', root/'web/img/icon-64.png')
print('ok', (out/'larik.ico').stat().st_size)
