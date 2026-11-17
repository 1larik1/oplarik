#!/usr/bin/env python3
"""Pre-renders the overlay glyph atlas (Inter) so the overlay can draw crisp,
anti-aliased text with per-pixel alpha without GDI. Output: overlay/atlas.png
(8-bit alpha) + overlay/atlas.json (glyph boxes per scale and font)."""
import json
import os
from PIL import Image, ImageDraw, ImageFont

ROOT = os.path.join(os.path.dirname(__file__), "..")
OUT = os.path.join(ROOT, "overlay")
FONTS = {
    "label": ("/usr/share/fonts/opentype/inter/Inter-SemiBold.otf", 11.0),
    "value": ("/usr/share/fonts/opentype/inter/InterDisplay-Bold.otf", 15.0),
}
SCALES = [0.85, 1.0, 1.2, 1.5, 1.8, 2.2, 2.6]

chars = [chr(c) for c in range(32, 127)]
chars += [chr(c) for c in range(0x0400, 0x0460)]           # Cyrillic incl. Є І Ї є і ї
chars += list("ҐґĄąĆćĘęŁłŃńÓóŚśŹźŻż°—·…")
chars += [chr(c) for c in range(0xC0, 0x180) if chr(c) not in chars]  # es/de/fr/cs accents (1.1.1)
chars += [c for c in "¿¡«»" if c not in chars]

WIDTH = 1024


def main():
    os.makedirs(OUT, exist_ok=True)
    entries = []
    # First pass: measure.
    for scale in SCALES:
        for name, (path, size) in FONTS.items():
            px = max(6, round(size * scale))
            font = ImageFont.truetype(path, px)
            asc, desc = font.getmetrics()
            for ch in chars:
                l, t, r, b = font.getbbox(ch, anchor="ls")
                entries.append((scale, name, font, ch, l, t, r - l, b - t, font.getlength(ch), asc, desc, px))
    # Shelf packing.
    x = y = row_h = 0
    placed = []
    for e in entries:
        w, h = max(e[6], 0), max(e[7], 0)
        if x + w + 2 > WIDTH:
            x, y, row_h = 0, y + row_h + 2, 0
        placed.append((x + 1, y + 1))
        x += w + 2
        row_h = max(row_h, h)
    height = y + row_h + 2
    img = Image.new("L", (WIDTH, height), 0)
    draw = ImageDraw.Draw(img)
    meta = {}
    for e, (px_, py_) in zip(entries, placed):
        scale, name, font, ch, l, t, w, h, adv, asc, desc, size = e
        if w > 0 and h > 0:
            draw.text((px_ - l, py_ - t), ch, font=font, fill=255, anchor="ls")
        sk = f"{scale:g}"
        s = meta.setdefault(sk, {})
        f = s.setdefault(name, {"size": size, "ascent": asc, "descent": desc, "glyphs": {}})
        f["glyphs"][ch] = [px_, py_, w, h, l, t, round(adv, 3)]
    img.save(os.path.join(OUT, "atlas.png"), optimize=True)
    with open(os.path.join(OUT, "atlas.json"), "w", encoding="utf-8") as fp:
        json.dump({"scales": SCALES, "fonts": meta}, fp, ensure_ascii=False, separators=(",", ":"))
    print("atlas", WIDTH, "x", height, "glyphs", len(entries))


if __name__ == "__main__":
    main()
