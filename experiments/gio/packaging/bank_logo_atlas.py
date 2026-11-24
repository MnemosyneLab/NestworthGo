"""Regenerate Gio's embedded bank-logo atlas from the Wails presentation set.

Run with Python 3, Pillow and Inkscape. Ordinary Gio builds read the checked-in
atlas and need only their existing prerequisites.
"""

from concurrent.futures import ThreadPoolExecutor
import hashlib
from io import BytesIO
import json
from pathlib import Path
import subprocess

from PIL import Image

TILE = 64
COLUMNS = 16
GIO = Path(__file__).resolve().parents[1]
FRONTEND = GIO.parents[1] / "frontend"


def render(path):
    result = subprocess.run(
        ["inkscape", str(path), "--export-type=png", f"--export-width={TILE}",
         "--export-filename=-"],
        check=True, capture_output=True,
    )
    with Image.open(BytesIO(result.stdout)) as source:
        tile = source.convert("RGBA")
    tile.thumbnail((TILE, TILE), Image.Resampling.LANCZOS)
    if tile.getbbox() is None:
        raise ValueError(f"Bank logo rendered empty: {path}")
    return tile


def main():
    source = (FRONTEND / "src/lib/bankLogoPresentation.json").read_bytes()
    logos = json.loads(source)
    paths = [FRONTEND / "public/bank-logos" / logo["file"] for logo in logos]
    digest = hashlib.sha256(source)
    for path in paths:
        digest.update(hashlib.sha256(path.read_bytes()).digest())

    rows = (len(logos) + COLUMNS - 1) // COLUMNS
    atlas = Image.new("RGBA", (COLUMNS * TILE, rows * TILE))
    with ThreadPoolExecutor(max_workers=4) as workers:
        for i, tile in enumerate(workers.map(render, paths)):
            x = (i % COLUMNS) * TILE + (TILE - tile.width) // 2
            y = (i // COLUMNS) * TILE + (TILE - tile.height) // 2
            atlas.paste(tile, (x, y))

    output = GIO / "internal/banklogos"
    atlas.save(output / "atlas.png", optimize=True)
    entries = ",\n".join(
        "    " + json.dumps(logo, ensure_ascii=False, separators=(",", ":"))
        for logo in logos
    )
    catalog = f'{{\n  "sourceDigest": "{digest.hexdigest()}",\n  "presentations": [\n{entries}\n  ]\n}}\n'
    (output / "catalog.json").write_text(catalog, encoding="utf-8")


if __name__ == "__main__":
    main()
