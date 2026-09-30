#!/usr/bin/env python3
"""
Generate the SIMARC desktop application icons from the brand mark.

The source design is web/static/images/logo-icon.svg (navy rounded square,
metallic-gold "A", gold border, decorative accents). No SVG rasteriser is
available in the build environment, so the same geometry is redrawn here with
Pillow and written out in the three formats the platform packagers need:

    build/icon.png            512x512 master (also build/linux/icon.png)
    build/windows/icon.ico    16..256, PNG-compressed (Vista+)
    build/darwin/icon.icns    32..1024, PNG-backed icns

Everything is drawn at 4x and downsampled, which gives clean antialiased edges
without needing a vector renderer. Re-run after changing the brand colours:

    python3 scripts/make-desktop-icons.py
"""

import os
import struct
import sys

from PIL import Image, ImageDraw

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
# Wails reads its packaging assets from <wails-project>/build, and the desktop
# project's root is desktop/ because that is where the package main lives in
# (Wails builds the package in its working directory, with no package argument).
BUILD = os.path.join(ROOT, "desktop", "build")

SS = 4  # supersampling factor
BASE = 512  # master size in px

# ── brand colours (mirrored from logo-icon.svg) ───────────────────────────────
NAVY_A = (0x0B, 0x1C, 0x2D)
NAVY_B = (0x06, 0x10, 0x18)
GOLD = (0xD4, 0xAF, 0x37)
GOLD_LIGHT = (0xF5, 0xE6, 0xA8)
GOLD_DARK = (0x9A, 0x7B, 0x0A)

ICO_SIZES = [16, 24, 32, 48, 64, 128, 256]
ICNS_ENTRIES = [(16, b"icp4"), (32, b"icp5"), (128, b"ic07"),
                (256, b"ic08"), (512, b"ic09"), (1024, b"ic10")]


def diagonal_gradient(size, c0, c1):
    """Smooth diagonal gradient, built small and upscaled (fast + smooth)."""
    n = 64
    g = Image.new("RGB", (n, n))
    px = g.load()
    for y in range(n):
        for x in range(n):
            t = (x + y) / (2.0 * (n - 1))
            px[x, y] = (
                int(c0[0] + (c1[0] - c0[0]) * t),
                int(c0[1] + (c1[1] - c0[1]) * t),
                int(c0[2] + (c1[2] - c0[2]) * t),
            )
    return g.resize((size, size), Image.BICUBIC)


def gold_metallic(size):
    """Left-to-right metallic gold ramp used for the border and the "A"."""
    n = 64
    stops = [(0.0, GOLD_DARK), (0.25, GOLD), (0.5, GOLD_LIGHT),
             (0.75, GOLD), (1.0, GOLD_DARK)]
    row = Image.new("RGB", (n, 1))
    px = row.load()
    for x in range(n):
        t = x / (n - 1)
        for i in range(len(stops) - 1):
            t0, c0 = stops[i]
            t1, c1 = stops[i + 1]
            if t0 <= t <= t1:
                k = (t - t0) / (t1 - t0) if t1 > t0 else 0.0
                px[x, 0] = tuple(int(c0[j] + (c1[j] - c0[j]) * k) for j in range(3))
                break
    return row.resize((size, size), Image.BICUBIC)


def s(v):
    """SVG user units (200x200 viewBox) -> supersampled px."""
    return v * BASE * SS / 200.0


def quad_bezier(p0, p1, p2, steps=48):
    pts = []
    for i in range(steps + 1):
        t = i / steps
        u = 1 - t
        x = u * u * p0[0] + 2 * u * t * p1[0] + t * t * p2[0]
        y = u * u * p0[1] + 2 * u * t * p1[1] + t * t * p2[1]
        pts.append((s(x), s(y)))
    return pts


def render_master():
    size = BASE * SS
    img = Image.new("RGBA", (size, size), (0, 0, 0, 0))

    # 1. navy rounded-square background
    grad = diagonal_gradient(size, NAVY_A, NAVY_B).convert("RGBA")
    mask = Image.new("L", (size, size), 0)
    ImageDraw.Draw(mask).rounded_rectangle(
        [s(10), s(10), s(190), s(190)], radius=s(36), fill=255)
    img.paste(grad, (0, 0), mask)

    d = ImageDraw.Draw(img)

    # 2. metallic gold border
    gold = gold_metallic(size)
    border = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    bd = ImageDraw.Draw(border)
    w = max(1, int(s(2.5)))
    edge = gold.convert("RGBA").getpixel((size // 2, size // 2))[:3] + (230,)
    bd.rounded_rectangle([s(12), s(12), s(188), s(188)], radius=s(34),
                         outline=edge, width=w)
    img.alpha_composite(border)

    # 3. faint inner border
    d.rounded_rectangle([s(18), s(18), s(182), s(182)], radius=s(28),
                        outline=(255, 255, 255, 20), width=max(1, int(s(1))))

    gcol = GOLD + (255,)

    # 4. the "A"
    a_pts = [(100, 45), (65, 135), (77, 135), (85, 115),
             (115, 115), (123, 135), (135, 135)]
    d.polygon([(s(x), s(y)) for x, y in a_pts], fill=gcol)

    # 5. crossbar of the A (navy, punched out)
    d.line([s(88), s(102), s(112), s(102)], fill=NAVY_A + (230,),
           width=max(1, int(s(5))))

    # 6. top accent star
    star = [(100, 28), (102, 34), (108, 34), (103, 38), (105, 44),
            (100, 40), (95, 44), (97, 38), (92, 34), (98, 34)]
    d.polygon([(s(x), s(y)) for x, y in star], fill=GOLD + (217,))

    # 7. side dots
    for cx, cy in ((55, 100), (145, 100)):
        r = s(3)
        d.ellipse([s(cx) - r, s(cy) - r, s(cx) + r, s(cy) + r], fill=GOLD + (179,))

    # 8. bottom arc
    d.line(quad_bezier((70, 155), (100, 165), (130, 155)),
           fill=GOLD + (179,), width=max(1, int(s(2.5))), joint="curve")

    # 9. corner embellishments
    for cx, cy in ((30, 30), (170, 30), (30, 170), (170, 170)):
        r = s(2.5)
        d.ellipse([s(cx) - r, s(cy) - r, s(cx) + r, s(cy) + r], fill=GOLD + (179,))

    # 10. glossy overlay
    gloss = Image.new("RGBA", (size, size), (0, 0, 0, 0))
    ImageDraw.Draw(gloss).ellipse([s(30), s(30), s(170), s(170)],
                                  fill=GOLD_LIGHT + (20,))
    img.alpha_composite(gloss)

    return img.resize((BASE, BASE), Image.LANCZOS)


def write_ico(master, path):
    """ICO container with PNG-compressed entries (supported Windows Vista+)."""
    entries = []
    for size in ICO_SIZES:
        img = master.resize((size, size), Image.LANCZOS)
        import io
        buf = io.BytesIO()
        img.save(buf, format="PNG", optimize=True)
        entries.append((size, buf.getvalue()))

    out = bytearray(struct.pack("<HHH", 0, 1, len(entries)))
    offset = 6 + 16 * len(entries)
    header = bytearray()
    payload = bytearray()
    for size, data in entries:
        w = 0 if size >= 256 else size
        h = 0 if size >= 256 else size
        header += struct.pack("<BBBBHHII", w, h, 0, 0, 1, 32, len(data), offset)
        payload += data
        offset += len(data)
    out += header + payload
    with open(path, "wb") as f:
        f.write(bytes(out))
    return len(entries)


def write_icns(master, path):
    """icns container with PNG-backed entries (macOS accepts PNG since 10.7)."""
    import io
    body = bytearray()
    for size, ostype in ICNS_ENTRIES:
        img = master.resize((size, size), Image.LANCZOS)
        buf = io.BytesIO()
        img.save(buf, format="PNG", optimize=True)
        data = buf.getvalue()
        body += ostype + struct.pack(">I", len(data) + 8) + data
    with open(path, "wb") as f:
        f.write(b"icns" + struct.pack(">I", len(body) + 8) + bytes(body))
    return len(ICNS_ENTRIES)


def main():
    for sub in ("", "linux", "windows", "darwin"):
        os.makedirs(os.path.join(BUILD, sub), exist_ok=True)

    master = render_master()

    png = os.path.join(BUILD, "appicon.png")
    master.save(png, format="PNG", optimize=True)
    master.save(os.path.join(BUILD, "linux", "icon.png"), format="PNG", optimize=True)
    print("wrote %s (512x512)" % os.path.relpath(png, ROOT))
    print("wrote build/linux/icon.png (512x512)")

    n = write_ico(master, os.path.join(BUILD, "windows", "icon.ico"))
    print("wrote build/windows/icon.ico (%d sizes: %s)"
          % (n, ", ".join(str(x) for x in ICO_SIZES)))

    n = write_icns(master, os.path.join(BUILD, "darwin", "icon.icns"))
    print("wrote build/darwin/icon.icns (%d sizes)" % n)
    return 0


if __name__ == "__main__":
    sys.exit(main())
