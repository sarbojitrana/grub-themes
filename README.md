# grub-themes

A collection of GRUB2 themes, and a small application to browse and apply them
safely.

![preview](themes/jarvis/preview.png)

Changing your GRUB theme normally means copying files into
`/usr/share/grub/themes`, hand-editing `/etc/default/grub`, and hoping
`grub-mkconfig` does not quietly drop a boot entry. This does it for you, and
checks afterwards that your boot configuration survived.

> **Status: early.** `list` and `lint` work. `preview`, `apply` and the TUI are
> being built. See [AGENTS.md](AGENTS.md) for the plan.

## Install

Nothing packaged yet. From source:

```bash
git clone https://github.com/sarbojitrana/grub-themes.git
cd grub-themes
go build -o grub-themes ./cmd/grub-themes
```

Packages for apt, dnf and pacman are planned — the binary is static, so this is
mostly packaging boilerplate rather than porting work.

## Use

```bash
grub-themes list              # what is available
grub-themes lint              # validate every theme
grub-themes lint jarvis       # validate one
```

Planned:

```bash
grub-themes preview jarvis    # render to a PNG, no GRUB needed
grub-themes apply jarvis      # install it, with rollback if grub.cfg breaks
grub-themes                   # TUI browser
```

## Themes

| Theme | Description |
|---|---|
| **jarvis** | Arc-reactor HUD in cyan on blue-black. Menu left, reactor right. |

Adding one is the easiest way to contribute and needs no Go — see
[CONTRIBUTING.md](CONTRIBUTING.md).

## Why the application is careful

GRUB gives almost no feedback. It drops what it cannot read and carries on
booting, so a broken theme looks like a broken *menu*, not a broken file. Three
examples this project has already hit in practice:

- A pixmap saved as a palette PNG is silently ignored. The selected entry then
  looks *blanked out* rather than highlighted, because only its text colour
  changed.
- A font is matched by a name baked inside the `.pf2` file, not by filename.
  Rename it and the text disappears.
- `GRUB_TERMINAL_OUTPUT=console` disables graphics entirely, so no theme
  renders at all — the most common reason a GRUB theme "does nothing".

`grub-themes lint` catches all of these before you reboot. Applying a theme
backs up `grub.cfg` and `/etc/default/grub`, then verifies that UKI entries,
`custom.cfg` includes and the menu entry count all survived regeneration —
restoring the backup and failing loudly if not.

## Contributing

Themes, code and bug reports all welcome. **Signed commits are required.** You
do not need GRUB installed or a reboot to contribute — `lint` and `preview` run
anywhere, and `qemu` gives real GRUB rendering without touching your own
bootloader.

See [CONTRIBUTING.md](CONTRIBUTING.md), and [AGENTS.md](AGENTS.md) for the
architecture and constraints.

This repository takes part in **Hacktoberfest**.

## License

MIT — see [LICENSE](LICENSE). Fonts in the JARVIS theme are built from
[JetBrains Mono](https://github.com/JetBrains/JetBrainsMono) (SIL OFL 1.1),
patched by [Nerd Fonts](https://github.com/ryanoasis/nerd-fonts).
