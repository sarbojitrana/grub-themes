#!/usr/bin/env bash
#
#   grub-themes — installer for the application itself.
#
#   Usage:
#     ./install.sh              build and install into /usr/local (uses sudo)
#     ./install.sh --user       install into ~/.local, no root needed
#     ./install.sh --uninstall  remove it again
#
#   This installs the browser. It does not touch your bootloader.
#
set -euo pipefail

MODE=system
ACTION=install
for a in "$@"; do
  case "$a" in
    --user)      MODE=user ;;
    --uninstall) ACTION=uninstall ;;
    -h|--help)   sed -n '2,12p' "$0"; exit 0 ;;
    *) echo "unknown option: $a" >&2; exit 1 ;;
  esac
done

cd "$(dirname "${BASH_SOURCE[0]}")"

C=$'\e[38;5;51m'; G=$'\e[38;5;46m'; R=$'\e[38;5;196m'; Z=$'\e[0m'
say() { printf '%s\n' "${C}::${Z} $*"; }
ok()  { printf '  %s✓%s %s\n' "$G" "$Z" "$*"; }
die() { printf '  %s✗%s %s\n' "$R" "$Z" "$*" >&2; exit 1; }

command -v go >/dev/null || die "Go is required to build from source. Install it, or use a package from the releases page."

if [[ $MODE == user ]]; then
  say "Installing into ${HOME}/.local"
  make "$ACTION" PREFIX="$HOME/.local"
  case ":$PATH:" in
    *":$HOME/.local/bin:"*) ;;
    *) printf '\n  Note: %s/.local/bin is not on your PATH.\n' "$HOME" ;;
  esac
else
  say "Installing into /usr/local (sudo)"
  make build
  sudo make "$ACTION" PREFIX=/usr/local
fi

if [[ $ACTION == install ]]; then
  echo
  ok "Run 'grub-themes', or launch \"GRUB Themes\" from your application menu or rofi."
  ok "Nothing about your boot configuration has changed yet."
fi
