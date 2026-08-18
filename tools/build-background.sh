#!/usr/bin/env bash
# Re-render theme/background.png from tools/background.svg.
# Edit the SVG to change colours, then run this.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
command -v magick >/dev/null || { echo "ImageMagick (magick) required"; exit 1; }
magick -background none tools/background.svg theme/background.png
echo "wrote theme/background.png ($(magick identify -format '%wx%h' theme/background.png))"
