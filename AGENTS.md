# Notes for agents and contributors

Context for anyone — human or AI — working on this theme. GRUB is an unusual
target: it has a minimal image decoder, a font format of its own, and almost no
error reporting. Nearly everything below is a silent-failure mode.

## What this is

A GRUB2 gfxmenu theme. `theme/` is what gets installed; `tools/` regenerates
the assets from source; `install.sh` puts it in place and verifies your boot
config survived.

```
theme/
  theme.txt            gfxmenu definition
  background.png       1920x1080 backdrop
  *.pf2                bitmap fonts, pre-built
  select_*.png         selection pill, sliced west / centre / east
  progress_*.png       countdown bar
  terminal_box_*.png   frame GRUB draws when an entry prints output
tools/
  background.svg       source art
  build-background.sh  re-render background.png
  build-assets.sh      rebuild every pixmap, and verify GRUB can decode them
  build-fonts.sh       rebuild the .pf2 fonts from a TTF
```

## Things that will bite you

**GRUB only decodes PNG colour-type 6 (truecolour+alpha) at bit-depth 8, and it
fails silently.** This is the single biggest trap. Ask ImageMagick for a flat
coloured rectangle and it will helpfully optimise it to a 1-bit palette PNG,
which GRUB drops entirely. The symptoms do not look like an image problem:

- the selected entry appears *blanked out* rather than highlighted, because
  only `selected_item_color` took effect and it is dark;
- a solid black slab covers the screen while the kernel loads, because the
  terminal box slices failed and GRUB fell back to its own opaque console.

`PNG32:` on its own is **not** enough. You need:

```bash
-define png:color-type=6 -define png:bit-depth=8
```

And do not verify with `magick identify -format '%[type]'` — it reports
`PaletteAlpha` for a perfectly valid colour-type 6 file, because it describes
content rather than encoding. Read the IHDR byte. `tools/build-assets.sh` does
this and exits non-zero on a bad header; run it after touching any pixmap.

**Fonts are matched by the name baked into the `.pf2`, not by filename.**
`grub-mkfont` writes a name inside the file and `theme.txt` must reference that
string exactly — hence `"JetBrainsMono NF Regular 20"` rather than a path.
`tools/build-fonts.sh` prints the baked names after rebuilding.

**`selected_item_color` is dark, and that only works because the pill renders.**
Dark text plus a dropped pixmap is exactly the failure above. If you change the
highlight to something translucent, make the text light to match.

**The terminal box slices are fully transparent on purpose.** GRUB draws that
box whenever an entry prints something — pressing Enter does, letting the
countdown expire does not. Any visible fill reads as a black slab for as long
as the kernel takes to load. Keep the `terminal-box` property in `theme.txt`
though: removing it makes GRUB fall back to its own solid console, which is the
thing being avoided.

**You cannot test this without rebooting.** There is no preview mode. Compose
changes with ImageMagick first — render the background, tile the pill slices to
the menu width, draw the item text at the geometry from `theme.txt` — and look
at that before committing anyone to a reboot.

**Install location matters.** `/usr/share/grub/themes` is the default because it
lives on the root filesystem. `/boot/grub/themes` is available behind
`--boot-themes` for setups where GRUB cannot read `/usr`, but on UEFI `/boot`
is often a small FAT ESP and this theme is a couple of megabytes.

## The installer's safety contract

`install.sh` regenerates `grub.cfg`, which is the risky part. Before doing so it
records whether the existing config used UKI entries (`uki` / `15_uki`),
whether it sourced `custom.cfg`, and how many `menuentry` lines there were.
Afterwards it checks all three survived and **restores the backup and exits
non-zero** if any did not. Backups go to `/root/jarvis-grub-backup-<timestamp>/`.

Do not weaken those checks. Someone else's recovery entries should not vanish
because they tried a theme.

It also comments out `GRUB_TERMINAL_OUTPUT=console`, which is the most common
reason a GRUB theme appears to do nothing at all.
