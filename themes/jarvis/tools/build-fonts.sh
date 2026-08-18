#!/usr/bin/env bash
#
# Regenerate the .pf2 fonts for this theme from a TTF.
#
#   ./tools/build-fonts.sh [/path/to/Font.ttf] [/path/to/Font-Bold.ttf]
#
# GRUB cannot read TTFs; grub-mkfont bakes a bitmap at a fixed size and stores
# a name inside the file. theme.txt must reference that exact name, which is
# printed at the end so you can copy it across if you swap fonts.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."

REG="${1:-/usr/share/fonts/TTF/JetBrainsMonoNerdFont-Regular.ttf}"
BOLD="${2:-/usr/share/fonts/TTF/JetBrainsMonoNerdFont-Bold.ttf}"
command -v grub-mkfont >/dev/null || { echo "grub-mkfont not found (install grub)"; exit 1; }
[[ -f "$REG"  ]] || { echo "no such font: $REG";  exit 1; }
[[ -f "$BOLD" ]] || { echo "no such font: $BOLD"; exit 1; }

for s in 12 14 20; do
  grub-mkfont -s "$s" -o "jetbrains-$s.pf2" "$REG"
done
grub-mkfont -s 18 -o jetbrains-bold-18.pf2 "$BOLD"

echo
echo "Names baked into the .pf2 files — theme.txt must match these exactly:"
for f in *.pf2; do
  printf '  %-28s %s\n' "$(basename "$f")" "$(strings "$f" | grep -m1 -E '^[A-Za-z].* [0-9]+$')"
done
