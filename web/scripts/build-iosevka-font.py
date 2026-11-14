#!/usr/bin/env python3
"""Losslessly wrap a supplied Iosevka Term Nerd Font Mono Regular TTF as WOFF2.

Requires fonttools[woff]==4.68.0. Run from any directory:
  python web/scripts/build-iosevka-font.py /path/to/IosevkaTermNerdFontMono-Regular.ttf
The binary is committed so normal web builds need neither Python nor the TTF.
"""
import argparse
from pathlib import Path

from fontTools.ttLib import TTFont

parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("source", type=Path)
args = parser.parse_args()
font = TTFont(args.source, recalcTimestamp=False)
assert font["name"].getDebugName(1) == "IosevkaTerm NFM", "Expected Iosevka Term Nerd Font Mono"
cmap = font.getBestCmap()
for codepoint in [0xE0A0, 0xE0B0, 0xF013, 0xF120, 0xF418, 0xF0001]:
    assert codepoint in cmap, f"Missing Nerd Font glyph U+{codepoint:04X}"
output = Path(__file__).resolve().parents[1] / "public/fonts/iosevka-term-nerd-font-mono-regular.woff2"
font.flavor = "woff2"
font.save(output)
converted = TTFont(output)
assert converted.getBestCmap() == cmap, "Character coverage changed"
assert converted["hmtx"].metrics == font["hmtx"].metrics, "Glyph spacing changed"
print(f"Saved {output.name}: {len(cmap):,} code points, {output.stat().st_size:,} bytes")
