#!/usr/bin/env bash
#
# Regenerate this theme's pixmap slices (selection highlight, progress bar,
# terminal box).
#
# The one thing that matters here: every file must be PNG colour-type 6
# (truecolour+alpha) at bit-depth 8. GRUB's PNG decoder handles nothing else,
# and it fails *silently*.
#
# This is easy to get wrong. Asked for a flat-coloured rectangle, ImageMagick
# optimises it down to a 1-bit palette image, and GRUB then renders none of it:
# the selected menu entry looks blanked out rather than highlighted (only its
# text colour changed), and the terminal box falls back to a solid black slab
# over the theme during boot. Passing `PNG32:` alone is NOT enough -- the
# explicit `-define png:color-type=6` is what forces it.
#
# Do not trust `magick identify -format '%[type]'` to check this: it reports
# "PaletteAlpha" for a perfectly good colour-type 6 file, because it describes
# the content rather than the encoding. Read the IHDR byte instead, which is
# what the verification at the bottom of this script does.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
command -v magick >/dev/null || { echo "ImageMagick (magick) required"; exit 1; }

# every write goes through this
PNGOPT=(-define png:color-type=6 -define png:bit-depth=8 -define png:interlace=0)

OUT=.
H=44                      # must match item_height in theme.txt
CAP=20                    # width of the rounded end caps

# ---------------------------------------------------------------- selection
# A SOLID cyan pill. theme.txt pairs it with a dark selected_item_color, so the
# fill has to be opaque for the text to read.
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

magick -size 240x$H xc:none \
  -fill '#00d9ff' -stroke none \
  -draw "roundrectangle 0,0 239,$((H-1)) 10,10" \
  "${PNGOPT[@]}" PNG32:"$tmp/pill.png"

magick "$tmp/pill.png" -crop ${CAP}x$H+0+0            +repage "${PNGOPT[@]}" PNG32:"$OUT/select_w.png"
magick "$tmp/pill.png" -crop 32x$H+104+0              +repage "${PNGOPT[@]}" PNG32:"$OUT/select_c.png"
magick "$tmp/pill.png" -crop ${CAP}x$H+$((240-CAP))+0 +repage "${PNGOPT[@]}" PNG32:"$OUT/select_e.png"

# ------------------------------------------------------------- progress bar
magick -size 32x8 xc:'rgba(18,32,42,0.85)' "${PNGOPT[@]}" PNG32:"$OUT/progress_bar_c.png"
magick -size 32x8 xc:'#00d9ff'             "${PNGOPT[@]}" PNG32:"$OUT/progress_highlight_c.png"

# ------------------------------------------------------------- terminal box
# FULLY TRANSPARENT, on purpose.
#
# GRUB draws this box whenever an entry prints anything, which happens when you
# press Enter on an entry (but not when the countdown expires -- that path
# prints nothing). With any appreciable alpha it reads as a black slab sitting
# on the theme for as long as the kernel takes to load; at 0.82 it was
# indistinguishable from opaque. Transparent slices mean the boot messages draw
# straight over the background and the "box" is never visible at all.
#
# The property still has to exist in theme.txt: dropping `terminal-box` makes
# GRUB fall back to its own solid console, which is the very thing we are
# trying to get rid of.
for s in c n s w e nw ne sw se; do
  case $s in
    c)        size=32x32 ;;
    n|s)      size=32x2  ;;
    w|e)      size=2x32  ;;
    *)        size=2x2   ;;
  esac
  magick -size $size xc:'rgba(0,0,0,0)' "${PNGOPT[@]}" PNG32:"$OUT/terminal_box_$s.png"
done

# ------------------------------------------------------------------- verify
# Reads the IHDR header directly. GRUB needs colour-type 6 at bit-depth 8;
# anything else renders as nothing at all.
python3 - "$OUT" <<'PY'
import glob, os, struct, sys

NAMES = {0: "grey", 2: "truecolour", 3: "palette",
         4: "grey+alpha", 6: "truecolour+alpha"}
out = sys.argv[1]
bad = []
print(f"Wrote pixmaps to {out}/:")
for path in sorted(glob.glob(os.path.join(out, "*.png"))):
    data = open(path, "rb").read()
    i = data.index(b"IHDR")
    w, h, depth, ctype = struct.unpack(">IIBB", data[i + 4:i + 14])
    ok = ctype == 6 and depth == 8
    if not ok:
        bad.append(path)
    print(f"  {os.path.basename(path):<28} {w}x{h} depth={depth} "
          f"ctype={ctype} ({NAMES.get(ctype, '?')}){'' if ok else '   <-- BAD'}")

if bad:
    print("\nThese are not colour-type 6 / depth 8. GRUB will not render them:")
    for b in bad:
        print(f"  {b}")
    sys.exit(1)
print("\nAll PNGs are colour-type 6, depth 8. GRUB can decode these.")
PY
