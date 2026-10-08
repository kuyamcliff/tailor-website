#!/usr/bin/env python3
"""Download CC0 fabric PBR textures from ambientCG and prepare web-ready texture sets.

Output: frontend/public/textures/<fabric-key>/{color,normal,roughness}.webp (1024 px, tileable)
        plus swatch.webp and LICENSE.json with source metadata.

These are development fabrics only. In production every fabric (swatch photo and textures) comes
from the atelier's own catalog, uploaded through the owner dashboard.

Usage: python3 scripts/fetch_dev_textures.py   (requires curl and Pillow)
"""
import io, json, os, subprocess, sys, zipfile
from PIL import Image, ImageOps

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(ROOT, "frontend", "public", "textures")

# fabric key -> (ambientCG asset id, tint applied to the color map for the swatch, or None to keep)
SETS = {
    "worsted-wool-twill": ("Fabric031", None),
    "irish-linen": ("Fabric062", None),
    "cotton-poplin": ("Fabric032", None),
    "houndstooth-wool": ("Fabric065", None),
    "silk-crepe": ("Fabric060", None),
    "cotton-velvet": ("Fabric026", None),
    "heritage-tartan": ("Fabric051", None),
}

def fetch(asset):
    url = f"https://ambientcg.com/get?file={asset}_1K-JPG.zip"
    data = subprocess.run(["curl", "-sfL", "--max-time", "120", url], check=True, capture_output=True).stdout
    return zipfile.ZipFile(io.BytesIO(data))

def pick(z, suffix):
    for n in z.namelist():
        if n.endswith(suffix):
            return Image.open(io.BytesIO(z.read(n)))
    raise KeyError(suffix)

def main():
    for key, (asset, _) in SETS.items():
        d = os.path.join(OUT, key)
        if os.path.exists(os.path.join(d, "LICENSE.json")) and "--force" not in sys.argv:
            print("skip", key); continue
        os.makedirs(d, exist_ok=True)
        z = fetch(asset)
        color = pick(z, "_Color.jpg").convert("RGB").resize((1024, 1024), Image.LANCZOS)
        normal = pick(z, "_NormalGL.jpg").convert("RGB").resize((1024, 1024), Image.LANCZOS)
        rough = ImageOps.grayscale(pick(z, "_Roughness.jpg")).resize((1024, 1024), Image.LANCZOS)
        color.save(os.path.join(d, "color.webp"), "WEBP", quality=82, method=6)
        normal.save(os.path.join(d, "normal.webp"), "WEBP", quality=90, method=6)
        rough.save(os.path.join(d, "roughness.webp"), "WEBP", quality=80, method=6)
        color.crop((0, 0, 512, 512)).save(os.path.join(d, "swatch.webp"), "WEBP", quality=85, method=6)
        json.dump({"source": f"https://ambientcg.com/view?id={asset}", "author": "ambientCG (Lennart Demes)",
                   "license": "CC0 1.0 Universal", "licenseUrl": "https://docs.ambientcg.com/license/",
                   "asset": asset, "resolution": "1024", "usage": "development fabric texture"},
                  open(os.path.join(d, "LICENSE.json"), "w"), indent=2)
        print("ok", key)

if __name__ == "__main__":
    main()
