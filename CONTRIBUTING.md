# Contributing

Thanks for taking an interest. Themes, code, docs and bug reports are all
welcome — and you do **not** need GRUB installed, or to reboot, to contribute.

Read [AGENTS.md](AGENTS.md) first if you are touching code. It documents
GRUB's silent-failure modes, and most of the odd-looking decisions in this
repository exist because one of them bit us.

## Adding a theme

This is the easiest way in, and the most useful.

```bash
grub-themes new nord --dir themes    # scaffold, inside a checkout
$EDITOR themes/nord/theme.toml       # colours, name, licence, your name
$EDITOR themes/nord/tools/background.svg
grub-themes build nord               # regenerate every asset from the above
grub-themes lint nord
grub-themes preview nord
```

`new` writes a complete, working, lint-passing theme from a template built into
the binary. You never have to read another theme to understand the format, and
you never have to run ImageMagick: `build` renders the background, draws the
pixmaps and bakes the fonts, all correctly encoded.

[docs/build-your-own-theme.md](docs/build-your-own-theme.md) walks through it
properly, including what each manifest field does.

Before you open the PR:

- **`grub-themes lint` must pass.** CI runs it on every pull request.
- **Commit `preview.png`.** That is what people see when browsing. Generate it
  with `grub-themes preview`, do not hand-make it — the point is that it shows
  what the theme actually renders.
- **Commit the generated files** (`background.png`, the pixmaps, the `.pf2`
  fonts). A theme has to work for someone who has neither `librsvg` nor
  `grub-mkfont`.
- **Do not commit `tools/` output by hand.** Edit the SVG and rebuild, so the
  art stays reproducible.

### Artwork and licensing

Your theme must be yours to license under MIT, or carry a compatible licence
noted in `theme.toml`.

- If you use a font, it must be freely redistributable, and you should say
  which one and under what licence in the pull request.
- Draw your own artwork. Themes *inspired by* something are very welcome — the
  ones here are — but please do not submit traced logos, trademarked marks, or
  images lifted from a film, game or brand. That is what would make this
  repository unsafe for other people to use.

## Testing without GRUB

Four levels, in increasing cost:

| Command | Needs | What it tells you |
|---|---|---|
| `grub-themes build` | nothing (more with SVG art) | that every asset is generated correctly |
| `grub-themes lint` | nothing | manifest, file references, PNG encoding, font names, contrast |
| `grub-themes preview` | nothing | what the layout looks like |
| `grub-themes qemu` | `qemu`, `grub-mkrescue` | real GRUB rendering, no reboot *(not built yet)* |

`lint` and `preview` are pure Go and run anywhere.

Only apply a theme on hardware you can afford to reboot. `grub-themes apply ID
--dry-run` shows every step without changing anything.

## Building

```bash
git clone https://github.com/sarbojitrana/grub-themes.git
cd grub-themes
make build            # or: go build ./cmd/grub-themes
make test
make lint             # go vet + lint every theme, which is what CI runs
./grub-themes
```

Go 1.24 or newer.

## Code

- `internal/` is genuinely internal, with no stability promise. The CLI is the
  public interface; design changes there deliberately.
- Anything that writes a PNG must go through `internal/paint`. It pins the
  encoding to colour-type 6, which GRUB is the only consumer of and the only
  thing it can read.
- For code touching `internal/install`, say in the PR how you verified the
  rollback path. That code exists to stop people losing their boot
  configuration, and it is the part of this project where a bug is genuinely
  dangerous. There are tests in `internal/install/install_test.go` covering the
  config edits and the verification probes; add to them.

## Signed commits are required

**Every commit must carry a verified signature.** Unsigned commits will not be
merged; `main` is protected to reject them.

```bash
# GPG
gpg --quick-generate-key "Your Name <you@example.com>" ed25519 sign 1y
gpg --list-secret-keys --keyid-format=long        # note the key id
git config --global user.signingkey <KEY_ID>
git config --global commit.gpgsign true
gpg --armor --export <KEY_ID>                     # add this to GitHub
```

Or SSH, simpler if you already push over SSH:

```bash
git config --global gpg.format ssh
git config --global user.signingkey ~/.ssh/id_ed25519.pub
git config --global commit.gpgsign true
```

Add the key to GitHub under *Settings → SSH and GPG keys* as a **signing** key,
not only an authentication key. Verify with `git log --format='%h %G? %s'` —
`G` is good, `N` is unsigned.

`git rebase` and `git filter-branch` **drop signatures**. If you rewrite
history, re-sign before pushing:

```bash
git rebase --root --exec 'git commit --amend --no-edit -S'
```

## Commit messages

One short subject line, imperative mood, no body. Aim for ~50 characters.

```
Add Nord theme
Fix pixmap encoding in the Cyberpunk theme
```

Explanation belongs in code comments or `AGENTS.md`, next to what it explains.

## Pull requests

- One change per PR. One theme per PR.
- Say what you tested and how (`lint`, `preview`, real hardware).
- Screenshots of a real boot menu are always welcome, but never required.

## Reporting bugs

Include your distro, GRUB version, screen resolution, and the output of
`grub-themes status`. If a theme renders as nothing at all, run `grub-themes
lint` first — it is usually the PNG encoding, or `GRUB_TERMINAL_OUTPUT=console`.

## Hacktoberfest

This repository takes part in Hacktoberfest. Please make your contribution a
real one: a theme you designed, a bug you fixed, documentation that was
genuinely missing. Spam PRs will be marked as such.

Good first issues are labelled `good first issue`. Adding a theme is a great
first contribution and needs no Go.

## License

By contributing you agree that your work is licensed under the MIT License, as
in [LICENSE](LICENSE).
