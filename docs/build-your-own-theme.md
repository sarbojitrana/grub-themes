# Build your own GRUB theme

You can make a theme for your own machine in about ten minutes, keep it
entirely to yourself, and never open a pull request. If you later want it in
the collection, that is one directory copy and a PR — [see the end](#share-it).

Nothing here needs Go, ImageMagick, or a reboot.

---

## 1. Scaffold it

```bash
grub-themes new mytheme
```

That writes a complete, working theme to
`~/.local/share/grub-themes/themes/mytheme` and immediately builds it, so what
you have is a real theme rather than a folder of placeholders. It already
passes `grub-themes lint`, and it already shows up in the browser:

```bash
grub-themes            # your theme is in the list
```

```
mytheme/
├── theme.toml            the manifest — colours and settings live here
├── theme.txt             the GRUB layout (menu position, fonts, sizes)
├── tools/background.svg  the art. This is the part that is actually yours
├── background.png        generated from the SVG
├── select_*.png          generated: the highlight behind the selected entry
├── terminal_box_*.png    generated
├── progress_*.png        generated
├── font-*.pf2            generated: GRUB's bitmap fonts
└── preview.png           generated: what the browser shows
```

Everything marked *generated* is rebuilt by `grub-themes build mytheme`. Do not
hand-edit those files; change the manifest or the SVG and rebuild.

## 2. Set the colours

Open `theme.toml`. The interesting part is `[assets]`:

```toml
[assets.selection]
style  = "pill"      # pill | bar | underline | none
fill   = "#00d9ff"   # the highlight behind the selected entry
text   = "#05202a"   # the text drawn on it
radius = 10
height = 44          # keep equal to item_height in theme.txt

[assets.terminal_box]
fill = "transparent"

[assets.progress]
track = "#141c24e6"  # 8-digit hex is #rrggbbaa
fill  = "#00d9ff"
```

`style` changes the shape of the highlight:

| style | looks like |
|---|---|
| `pill` | rounded capsule — soft, modern |
| `bar` | square-cornered slab — angular (`radius` still applies) |
| `underline` | a rule under the entry (`thickness` sets how thick) |
| `none` | no pixmap; only the text colour changes |

There is also an optional offset ghost behind the highlight, which is how the
`arachne` theme gets its out-of-register comic look and `gotham` its bright
leading edge:

```toml
shadow    = "#2bd9ff"
shadow_dx = -7        # negative: to the left
shadow_dy = 6         # positive: downwards
```

Then rebuild:

```bash
grub-themes build mytheme
```

## 3. Make the art

Edit `tools/background.svg` in any editor — Inkscape, or a text editor, both
work. Two things worth knowing:

- **The menu is drawn over one half of the screen** (the left, in the template),
  so keep that side quiet and put the artwork on the other side. Change which
  half in `theme.txt`.
- **Design for 1920×1080.** GRUB scales the image to the screen, and the layout
  in `theme.txt` is in percentages, so every 16:9 mode works from the one file.

`grub-themes build` renders the SVG (with `rsvg-convert`, or ImageMagick as a
fallback) and re-encodes the result so GRUB can read it. If you would rather
paint a background in GIMP or use a photograph, that is fine too — drop it in
as `background.png` and delete the `[assets.background]` section.

## 4. Move things around

`theme.txt` is GRUB's own format, and it controls the layout:

```
+ boot_menu {
  left   = 7%          # where the menu sits
  top    = 32%
  width  = 44%
  height = 44%

  item_font           = "JetBrainsMono NF Regular 20"
  item_color          = "#c8d4de"
  selected_item_font  = "JetBrainsMono NF Bold 18"
  selected_item_color = "#0b0f14"

  item_height  = 44    # keep equal to assets.selection.height
  item_spacing = 12
  item_padding = 10

  selected_item_pixmap_style = "select_*.png"
}
```

Percentages are of the screen; `100%-58` also works. The other blocks in the
file are a `progress_bar` (the countdown) and `label`s (the countdown text and
the key hints). [GRUB's theme reference][grub-theme-ref] documents the rest.

The font names must be the names **baked inside** the `.pf2` files — not the
filenames. `grub-themes build` prints them:

```
  font-20.pf2              -> "JetBrainsMono NF Regular 20"
```

To use a different typeface, point `[assets.fonts]` at any TTF on your system
and rebuild; the names it prints are what go in `theme.txt`.

## 5. Check it

```bash
grub-themes lint mytheme      # catches the silent failures, instantly
grub-themes preview mytheme   # renders preview.png — look at it
```

`lint` is not a formality. GRUB has essentially no error reporting: it drops
what it cannot read and carries on booting, so a mistake shows up as a *broken
menu*, not a broken file. The three that bite everybody:

- **A PNG that is not colour-type 6.** GRUB decodes nothing else. Anything
  `grub-themes build` writes is correct; a hand-made PNG very often is not.
- **A font name that no `.pf2` contains.** The text silently disappears.
- **`GRUB_TERMINAL_OUTPUT=console` in `/etc/default/grub`.** Graphics are off
  entirely, so no theme renders at all. `grub-themes status` tells you, and
  applying a theme comments it out for you.

`preview` draws the menu from your real `theme.txt`, real pixmaps and real
fonts. It is a layout check rather than an emulator — but if the preview looks
right, the boot menu almost always does too.

## 6. Use it

```bash
grub-themes apply mytheme
```

This needs root, because it copies the theme into the system themes directory,
sets `GRUB_THEME` in `/etc/default/grub` and regenerates `grub.cfg`. Before
touching anything it backs both files up to `/var/lib/grub-themes/backups/`,
and afterwards it checks that your UKI entries, your `custom.cfg` include and
your menu entry count all survived — restoring the backup and failing loudly if
they did not.

Prefer to look before you leap:

```bash
grub-themes apply mytheme --dry-run   # prints every step, changes nothing
```

And to go back:

```bash
grub-themes remove
```

---

## Share it

A theme you build is yours; nothing has to be published. But if you want it in
the collection:

1. Fork <https://github.com/sarbojitrana/grub-themes>.
2. Copy your theme directory into `themes/` in the fork:
   ```bash
   cp -r ~/.local/share/grub-themes/themes/mytheme themes/mytheme
   ```
3. Make sure `theme.toml` has your name, a real description, and a licence you
   are entitled to grant. If you used a font or image you did not make, say so
   there.
4. `grub-themes lint mytheme` must pass, and `preview.png` must be committed —
   that is what people see when browsing.
5. Commit (**signed** — `main` requires it), push, open a pull request.

CI runs `lint` on every PR and attaches the rendered preview, so reviewers can
see your theme without booting anything.

[CONTRIBUTING.md](../CONTRIBUTING.md) has the details on signing and on what a
good PR looks like.

---

## Troubleshooting

| What you see | Usually means |
|---|---|
| No theme at all, plain GRUB text | `GRUB_TERMINAL_OUTPUT=console` is set — run `grub-themes status` |
| The selected entry looks blanked out | The highlight PNG is not colour-type 6, so only the text colour applied |
| Menu text missing entirely | A font name in `theme.txt` matches no `.pf2` |
| A black slab while the kernel loads | The terminal box has a visible fill; make it `transparent` |
| Theme renders but at the wrong size | It is being scaled from another resolution — check `designed_for` |
| Changes do nothing after `apply` | Something else regenerated `grub.cfg` afterwards; re-run `grub-themes apply` |

If a theme renders as nothing at all, run `grub-themes lint` first. It is
almost always the PNG encoding.

[grub-theme-ref]: https://www.gnu.org/software/grub/manual/grub/html_node/Theme-file-format.html
