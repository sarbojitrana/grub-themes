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
- **TUI + CLI.** No GTK/Qt dependency, works over SSH, no display server. It
  reaches application menus and rofi through a desktop entry with
  `Terminal=true`, not by growing a second UI.
- **Themes are data, not code.** A theme is a directory under `themes/` with a
  `theme.toml` manifest. Contributors adding a theme never touch Go, never
  write shell, and never run ImageMagick.

## Layout

```
cmd/grub-themes/      CLI entry point and subcommands
internal/
  theme/              manifest parsing + discovery across search paths
  paint/              raster helpers and the PNG encoder (see below)
  pf2/                GRUB bitmap font reader
  assets/             theme.toml [assets] -> pixmaps, background, fonts
  lint/               validation, no GRUB needed
  preview/            render theme.txt to a PNG
  install/            apply/remove, with backup and rollback
  scaffold/           `new`, from a template embedded in the binary
  tui/                bubbletea browser
themes/<id>/          one directory per theme
  theme.toml          manifest — the only file you hand-edit besides the art
  theme.txt           GRUB gfxmenu definition
  tools/background.svg  the art source (optional; not installed)
  *.png *.pf2         generated
  preview.png         generated; what the browser shows
packaging/            desktop entry, icon, PKGBUILD, nfpm config
docs/                 build-your-own-theme guide
```

## State

| Piece | Status |
|---|---|
| `theme` package, `theme.toml` schema | done |
| `lint` — silent failures, contrast, manifest/theme.txt agreement | done |
| `paint` — colour-type 6 PNG encoder | done |
| `pf2` — font parsing | done |
| `assets` — `build`: pixmaps, background, fonts | done |
| `preview` renderer | done |
| `install` / `apply` with rollback | done (ported from the old `install.sh`) |
| `scaffold` — `new` | done |
| TUI browser | done |
| Packaging — desktop entry, Makefile, PKGBUILD, nfpm | done |
| QEMU harness | not started |
| Release binaries / AUR package published | not started |

## GRUB's silent failure modes

These are why `lint` exists. Each one produces a broken boot menu with no error
anywhere.

**PNG colour-type.** GRUB decodes **only colour-type 6 (truecolour+alpha) at
bit depth 8**. Symptoms do not look like an image problem: the selected entry
appears *blanked out* rather than highlighted (only `selected_item_color` took
effect), and a themed terminal box becomes an opaque black slab while the
kernel loads.

Two traps, from opposite directions:

- ImageMagick optimises a flat-coloured rectangle down to a 1-bit palette PNG.
  `PNG32:` alone is not enough; it needs `-define png:color-type=6 -define
  png:bit-depth=8`.
- **Go's `image/png` encoder writes colour-type 2 whenever every pixel is
  opaque.** That is a sensible size optimisation everywhere except here. This
  is why `internal/paint` has its own encoder rather than calling
  `png.Encode`, and why `paint.Save` re-reads the IHDR it just wrote.

Everything that writes a PNG must go through `paint.Save`. Do **not** verify
encoding with `magick identify -format '%[type]'`: it reports `PaletteAlpha`
for a perfectly valid colour-type 6 file, because it describes content rather
than encoding. Read the IHDR byte, as `paint` and `lint` do.

**Font names.** GRUB matches fonts by a name baked inside the `.pf2` by
`grub-mkfont`, never by filename. `theme.txt` must reference that exact string
(`"JetBrainsMono NF Regular 20"`). Rename or resize a font and the text
silently disappears. `grub-themes build` prints the baked names; `internal/pf2`
reads them out of the NAME section, so `lint` compares against the real thing.

**Pixmap slice sets.** `select_*.png` means GRUB looks for `_c`, `_w`, `_e`
(and corner variants for boxes). A missing slice is not reported. The slice
sets `internal/assets` generates deliberately mirror the JARVIS theme, which is
the layout known to render correctly on real hardware.

**`GRUB_TERMINAL_OUTPUT=console`** in `/etc/default/grub` disables graphics
entirely, so no theme renders at all. This is the single most common reason a
GRUB theme "does nothing". `apply` comments it out; `status` reports it.

**The terminal box appears on Enter, not on timeout.** GRUB draws it whenever
an entry prints output. Pressing Enter does; letting the countdown expire does
not. Any visible fill therefore reads as a black slab for as long as the kernel
takes to load. Themes ship fully transparent slices for this reason, but keep
the `terminal-box` property — removing it makes GRUB fall back to its own solid
console.

## Testing without GRUB — the tiers

Contributors will not all have GRUB, and nobody should install a bootloader or
reboot to submit a theme. Hence:

1. **`grub-themes build`** — generates every asset from the manifest, so the
   encoding trap cannot be reached by hand in the first place.
2. **`grub-themes lint`** — validates manifest, file references, PNG encoding,
   font names, and the contrast of the selection text against its highlight.
   Instant, pure Go. **This runs in CI on every PR.**
3. **`grub-themes preview`** — renders `theme.txt` to a PNG: background, then
   the selection pill tiled from its slices at the declared geometry, then item
   text drawn from the theme's own `.pf2` files. That is why preview text is
   aliased — GRUB's fonts are 1 bit per pixel, and this is what the boot menu
   really looks like. It is a layout check, not an emulator. **CI attaches it
   to the PR.**
4. **`grub-themes qemu`** — not built yet. Would build a throwaway image with
   `grub-mkrescue`, boot it in QEMU and screenshot the menu.

## How themes stay independent of each other

**Adding a theme must never require reading or touching another theme.** It
used to: `CONTRIBUTING.md` said "copy `themes/jarvis`", which dragged along
jarvis's `tools/` scripts — written for jarvis's SVG and its particular pill —
so every new theme inherited decisions it did not make.

Three moves fixed that, and all three are in place:

1. **Asset generation lives in the binary.** `theme.toml` declares what it
   wants (`[assets.selection]`, `[assets.terminal_box]`, `[assets.progress]`,
   `[assets.fonts]`, `[assets.background]`) and `grub-themes build` emits
   correctly-encoded pixmaps, renders the SVG and bakes the fonts. The
   per-theme shell scripts are gone.
2. **`grub-themes new <id>`** scaffolds a complete, lint-passing theme from a
   template compiled into the binary — not a copy of an existing theme.
3. **Themes are data-only.** A theme directory holds a manifest, a `theme.txt`,
   generated assets and optionally one SVG. `tools/` is for bespoke art
   sources; it is never installed.

The background stays something the author supplies — that is the actual
creative work, and it should not be templated.

Fonts are baked with a restricted glyph range (`internal/assets/fonts.go`):
Latin, Greek, Cyrillic, punctuation, arrows and box drawing. The full Nerd Font
is about twenty times larger, and a repository that ships fonts per theme
cannot afford that.

## The installer's safety contract

Non-negotiable. `internal/install` does this, and any replacement must too:

- Probe for the layout rather than assuming: `grub` vs `grub2` directories,
  `grub-mkconfig` vs `grub2-mkconfig`.
- Back up `grub.cfg` and `/etc/default/grub` — to
  `/var/lib/grub-themes/backups/<timestamp>-<tag>/` — before touching anything.
- Before regenerating, **record** whether the existing config used UKI entries
  (`uki` / `15_uki`), whether it sourced `custom.cfg`, and how many
  `menuentry` lines there were.
- After regenerating, verify all three survived, plus that the theme is
  actually referenced. If any check fails, **restore the backup and return an
  error**.
- Refuse to install a theme that does not pass `lint`.

Someone else's recovery entries must not vanish because they tried a theme.
This matters more than any feature. `internal/install/install_test.go` covers
the config edits and the verification probes; extend it rather than replacing
it.

## Where themes are looked for

In order, first definition of an id winning:

1. `$GRUB_THEMES_DIR`
2. `themes/` in the repository you are standing in
3. `$XDG_DATA_HOME/grub-themes/themes` — where `new` scaffolds, so a theme you
   are writing appears in the browser immediately
4. each of `$XDG_DATA_DIRS` (`/usr/local/share`, `/usr/share`)

This is what lets `make install PREFIX=/usr/local` and a distribution package
under `/usr` both work without configuration.

## Conventions

- Commit messages: one short subject line, imperative, no body.
- **Signed commits are required**; `main` enforces `required_signatures`. Note
  that `git rebase` and `git filter-branch` silently drop signatures — re-sign
  with `git rebase --root --exec 'git commit --amend --no-edit -S'`.
- Never hand-edit a generated file. Edit `theme.toml` or the SVG and run
  `grub-themes build <id>`; commit the result.
- `internal/` is genuinely internal — no stability promise. Design the CLI as
  the public interface.
- Artwork in this repository is original. Themes may be inspired by something,
  but do not add traced logos or trademarked marks: that would make the
  repository unsafe for other people to redistribute.
