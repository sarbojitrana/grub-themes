# Notes for agents and contributors

Context for anyone — human or AI — working on this repository. Read this before
changing anything; GRUB fails silently in several interesting ways and most of
the odd-looking decisions here are load-bearing.

## What this is

A collection of GRUB2 themes **plus an application to browse and apply them**.
The application is the point: picking a GRUB theme normally means copying files
into `/usr/share/grub/themes`, hand-editing `/etc/default/grub` and hoping
`grub-mkconfig` does not eat your boot entries. This wraps that safely.

Scope, decided 2026-08-19:

- **Go**, single static binary. Packaging across apt/dnf/pacman is the main
  cross-distro cost, and one binary makes it nearly free.
- **TUI + CLI.** No GTK/Qt dependency, works over SSH, no display server.
- **Themes are data, not code.** A theme is a directory under `themes/` with a
  `theme.toml` manifest. Contributors adding a theme never touch Go.

## Layout

```
cmd/grub-themes/      CLI entry point
internal/
  theme/              manifest parsing + discovery      [done]
  lint/               validation, no GRUB needed        [done]
  preview/            render theme.txt to a PNG         [TODO]
  install/            apply/remove, with rollback       [TODO]
  tui/                bubbletea browser                 [TODO]
themes/<id>/          one directory per theme
  theme.toml          manifest
  theme.txt           GRUB gfxmenu definition
  *.png *.pf2         assets
  preview.png         what the browser shows
  tools/              that theme's asset generators
packaging/            nfpm config, PKGBUILD             [TODO]
```

## State

| Piece | Status |
|---|---|
| `theme` package, `theme.toml` schema | done |
| `lint` — catches all three known silent failures | done |
| `list`, `lint` subcommands | done |
| `preview` renderer | not started |
| `install` / `apply` with rollback | logic exists in the old `install.sh`, needs porting |
| TUI | not started |
| QEMU harness | not started |
| Packaging | not started |

The old shell installer is the reference implementation for `internal/install`.
Its safety contract is described below and **must** be preserved.

## GRUB's silent failure modes

These are why `lint` exists. Each one produces a broken boot menu with no error
anywhere.

**PNG colour-type.** GRUB decodes **only colour-type 6 (truecolour+alpha) at
bit depth 8**. Ask ImageMagick for a flat-coloured rectangle and it optimises to
a 1-bit palette PNG, which GRUB drops entirely. Symptoms do not look like an
image problem: the selected entry appears *blanked out* rather than highlighted
(only `selected_item_color` took effect), and a themed terminal box becomes an
opaque black slab while the kernel loads. `PNG32:` alone is not enough — you
need `-define png:color-type=6 -define png:bit-depth=8`. Do **not** verify with
`magick identify -format '%[type]'`; it reports `PaletteAlpha` for a valid
colour-type 6 file because it describes content, not encoding. Read the IHDR
byte, as `internal/lint` does.

**Font names.** GRUB matches fonts by a name baked inside the `.pf2` by
`grub-mkfont`, never by filename. `theme.txt` must reference that exact string
(`"JetBrainsMono NF Regular 20"`). Rename or resize a font and the text silently
disappears.

**Pixmap slice sets.** `select_*.png` means GRUB looks for `_c`, `_w`, `_e`
(and corner variants for boxes). A missing slice is not reported.

**`GRUB_TERMINAL_OUTPUT=console`** in `/etc/default/grub` disables graphics
entirely, so no theme renders at all. This is the single most common reason a
GRUB theme "does nothing". The installer comments it out.

**The terminal box appears on Enter, not on timeout.** GRUB draws it whenever an
entry prints output. Pressing Enter does; letting the countdown expire does not.
Any visible fill therefore reads as a black slab for as long as the kernel takes
to load. The JARVIS theme ships fully transparent slices for this reason, but
keeps the `terminal-box` property — removing it makes GRUB fall back to its own
solid console.

## Testing without GRUB — the three tiers

Contributors will not all have GRUB, and nobody should install a bootloader or
reboot to submit a theme. Hence:

1. **`grub-themes lint`** — validates manifest, file references, PNG encoding
   and font names. Instant, pure Go, no external tools. Catches every failure
   above. **This runs in CI on every PR.**
2. **`grub-themes preview`** — renders `theme.txt` to a PNG approximating what
   GRUB would draw: background, then the selection pill tiled from its slices
   at the geometry declared in `theme.txt`, then item text in the declared
   fonts and colours. Not pixel-exact — it is a layout check, not an emulator.
   **CI should attach this to the PR** so reviewers see the theme without
   booting anything.
3. **`grub-themes qemu`** — builds a throwaway image with `grub-mkrescue`, boots
   it in QEMU and screenshots the menu. Real GRUB, real renderer, no reboot and
   no risk to the host. Opt-in, since it needs `qemu` and `grub-mkrescue`.

Tier 2 is the one that makes this project pleasant to contribute to. Build it
early.

## The installer's safety contract

Non-negotiable. `install.sh` (to be ported to `internal/install`) does this and
any replacement must too:

- Probe for the layout rather than assuming: `grub` vs `grub2` directories,
  `grub-mkconfig` vs `grub2-mkconfig`.
- Back up `grub.cfg` and `/etc/default/grub` before touching anything.
- Before regenerating, **record** whether the existing config used UKI entries
  (`uki` / `15_uki`), whether it sourced `custom.cfg`, and how many
  `menuentry` lines there were.
- After regenerating, verify all three survived. If any did not, **restore the
  backup and exit non-zero**.

Someone else's recovery entries must not vanish because they tried a theme.
This matters more than any feature.

## Conventions

- Commit messages: one short subject line, imperative, no body.
- **Signed commits are required**; `main` enforces `required_signatures`. Note
  that `git rebase` and `git filter-branch` silently drop signatures — re-sign
  with `git rebase --root --exec 'git commit --amend --no-edit -S'`.
- Assets have generators under `themes/<id>/tools/`. Never hand-edit a
  generated file; edit the source and rebuild so the change is reproducible.
- `internal/` is genuinely internal — no stability promise. Design the CLI as
  the public interface.
