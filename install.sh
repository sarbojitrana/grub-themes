#!/usr/bin/env bash
#
#   J.A.R.V.I.S. GRUB theme — installer
#
#   Usage:
#     sudo ./install.sh              install and regenerate grub.cfg
#     sudo ./install.sh --dry-run    show every step, change nothing
#     sudo ./install.sh --no-regen   install but leave grub.cfg alone
#     sudo ./install.sh --uninstall  remove and restore the previous theme
#     sudo ./install.sh --boot-themes  install under /boot/grub/themes instead
#
set -uo pipefail

DRY=0; REGEN=1; UNINSTALL=0; BOOT_THEMES=0
for a in "$@"; do
  case "$a" in
    --dry-run)   DRY=1 ;;
    --no-regen)  REGEN=0 ;;
    --uninstall) UNINSTALL=1 ;;
    --boot-themes) BOOT_THEMES=1 ;;
    -h|--help)   sed -n '2,10p' "$0"; exit 0 ;;
    *) echo "unknown option: $a" >&2; exit 1 ;;
  esac
done

C=$'\e[38;5;51m'; G=$'\e[38;5;46m'; A=$'\e[38;5;214m'; R=$'\e[38;5;196m'; Z=$'\e[0m'; D=$'\e[2m'
say()  { printf '%s\n' "${C}::${Z} $*"; }
ok()   { printf '  %s✓%s %s\n' "$G" "$Z" "$*"; }
warn() { printf '  %s!%s %s\n' "$A" "$Z" "$*"; }
die()  { printf '  %s✗%s %s\n' "$R" "$Z" "$*" >&2; exit 1; }
run()  { (( DRY )) && { printf '  %s[dry-run] %s%s\n' "$D" "$*" "$Z"; return 0; }; eval "$@"; }

SRC="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
NAME=jarvis

(( DRY )) || [[ $EUID -eq 0 ]] || die "Run me with sudo."

# ---------------------------------------------------------------- detect
# Layout differs across distros: Debian/Arch use grub/, Fedora/openSUSE grub2/.
GRUB_CFG=""
for c in /boot/grub/grub.cfg /boot/grub2/grub.cfg /boot/efi/EFI/*/grub.cfg; do
  [[ -f "$c" ]] && { GRUB_CFG="$c"; break; }
done
[[ -n "$GRUB_CFG" ]] || die "could not find grub.cfg"

MKCONFIG=""
for m in grub-mkconfig grub2-mkconfig; do command -v "$m" >/dev/null && { MKCONFIG="$m"; break; }; done
[[ -n "$MKCONFIG" ]] || die "neither grub-mkconfig nor grub2-mkconfig found"

DEFAULTS=/etc/default/grub
[[ -f "$DEFAULTS" ]] || die "$DEFAULTS not found"

# /usr/share/grub/themes is the standard location and lives on the root
# filesystem, which GRUB reads directly. /boot/grub/themes is only preferred
# when /usr/share/grub does not exist, or when asked for explicitly -- on a
# UEFI system /boot is often a small FAT ESP, and themes are megabytes.
if (( BOOT_THEMES )); then
  THEMES_DIR=/boot/grub/themes
  [[ -d /boot/grub2 ]] && THEMES_DIR=/boot/grub2/themes
elif [[ -d /usr/share/grub ]]; then
  THEMES_DIR=/usr/share/grub/themes
elif [[ -d /boot/grub2 ]]; then
  THEMES_DIR=/boot/grub2/themes
else
  THEMES_DIR=/boot/grub/themes
fi
DEST="$THEMES_DIR/$NAME"

say "grub.cfg   $GRUB_CFG"
say "generator  $MKCONFIG"
say "themes     $THEMES_DIR"

STAMP="$(date +%Y%m%d-%H%M%S)"
BK="/root/jarvis-grub-backup-$STAMP"

# ------------------------------------------------------------- uninstall
if (( UNINSTALL )); then
  say "Uninstalling"
  run "rm -rf '$DEST'"
  prev=$(ls -td /root/jarvis-grub-backup-* 2>/dev/null | head -1)
  if [[ -n "$prev" && -f "$prev/default-grub" ]]; then
    run "cp -a '$prev/default-grub' '$DEFAULTS'"
    ok "restored $DEFAULTS from $prev"
  else
    run "sed -i 's|^GRUB_THEME=.*|#GRUB_THEME=|' '$DEFAULTS'"
    warn "no backup found — commented GRUB_THEME out instead"
  fi
  (( REGEN )) && run "$MKCONFIG -o '$GRUB_CFG'"
  ok "done"
  exit 0
fi

# ---------------------------------------------------------------- install
[[ -f "$SRC/theme/theme.txt" ]] || die "theme/theme.txt missing — run this from the repo root"

run "mkdir -p '$BK'"
run "cp -a '$GRUB_CFG' '$BK/grub.cfg'"
run "cp -a '$DEFAULTS' '$BK/default-grub'"
ok "backed up grub.cfg and $DEFAULTS to $BK"

run "rm -rf '$DEST'"
run "mkdir -p '$DEST'"
run "cp -a '$SRC/theme/.' '$DEST/'"
run "chown -R root:root '$DEST'"
run "chmod -R a+rX '$DEST'"
ok "theme installed to $DEST"

if grep -q '^[#[:space:]]*GRUB_THEME=' "$DEFAULTS"; then
  run "sed -i 's|^[#[:space:]]*GRUB_THEME=.*|GRUB_THEME=\"$DEST/theme.txt\"|' '$DEFAULTS'"
else
  run "printf 'GRUB_THEME=\"%s/theme.txt\"\n' '$DEST' >> '$DEFAULTS'"
fi
# a theme needs a graphical terminal
grep -q '^GRUB_TERMINAL_OUTPUT=console' "$DEFAULTS" && \
  { run "sed -i 's|^GRUB_TERMINAL_OUTPUT=console|#&|' '$DEFAULTS'"; warn "commented out GRUB_TERMINAL_OUTPUT=console (themes need gfxterm)"; }
ok "GRUB_THEME set"

if (( ! REGEN )); then
  warn "--no-regen: run '$MKCONFIG -o $GRUB_CFG' yourself to apply"
  exit 0
fi

say "Regenerating grub.cfg"
if (( DRY )); then
  printf '  %s[dry-run] %s -o %s%s\n' "$D" "$MKCONFIG" "$GRUB_CFG" "$Z"
  exit 0
fi

# Record what the current config relies on, so we can prove the new one kept it.
had_uki=0;    grep -qE '^\s*uki|15_uki'   "$BK/grub.cfg" && had_uki=1
had_custom=0; grep -q  'custom.cfg'       "$BK/grub.cfg" && had_custom=1
n_entries_before=$(grep -c '^menuentry' "$BK/grub.cfg")

if ! $MKCONFIG -o "$GRUB_CFG" 2>&1 | sed 's/^/    /'; then
  cp -a "$BK/grub.cfg" "$GRUB_CFG"
  die "$MKCONFIG failed — previous grub.cfg restored, boot unchanged"
fi

fail=0
(( had_uki ))    && { grep -qE '^\s*uki|15_uki' "$GRUB_CFG" || { warn "UKI support disappeared"; fail=1; }; }
(( had_custom )) && { grep -q 'custom.cfg'      "$GRUB_CFG" || { warn "custom.cfg include disappeared"; fail=1; }; }
grep -q "themes/$NAME/theme.txt" "$GRUB_CFG" || { warn "theme not referenced in the new grub.cfg"; fail=1; }
n_after=$(grep -c '^menuentry' "$GRUB_CFG")
(( n_after < n_entries_before )) && { warn "menu entries dropped from $n_entries_before to $n_after"; fail=1; }

if (( fail )); then
  cp -a "$BK/grub.cfg" "$GRUB_CFG"
  die "verification failed — previous grub.cfg restored, boot unchanged"
fi
ok "verified: theme applied, $n_after menu entries, UKI + custom.cfg intact"

printf '\n'
say "Done — reboot to see it."
say "Remove with: sudo $0 --uninstall"
printf '\n'
