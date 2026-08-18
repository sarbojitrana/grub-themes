# Contributing

Thanks for taking an interest. Themes, code, docs and bug reports are all
welcome — and you do **not** need GRUB installed, or to reboot, to contribute.

Read [AGENTS.md](AGENTS.md) first if you are touching code. It documents
GRUB's silent-failure modes, and most of the odd-looking decisions in this
repository exist because one of them bit us.

## Adding a theme

This is the easiest way in, and the most useful.

> **Interim flow.** Copying an existing theme is what works today, but it means
> inheriting that theme's build scripts and design decisions. `grub-themes new`
> will scaffold a fresh theme from a built-in template so you never have to
> read another theme — see *Planned: making themes independent of each other*
> in [AGENTS.md](AGENTS.md). Until then, feel free to delete anything in
> `tools/` you are not using.

1. Copy an existing theme as a starting point:

   ```bash
   cp -r themes/jarvis themes/your-theme
   ```

2. Edit `themes/your-theme/theme.toml` — id, name, description, your name,
   licence. The `id` must match the directory name.

3. Make your art. Keep the generators working: if you change the background,
   edit the SVG under `tools/` and re-run its build script rather than editing
   the PNG directly, so the change is reproducible.

4. **Encode every PNG as colour-type 6, bit depth 8.** This is not optional —
   GRUB decodes nothing else, and it fails silently:

   ```bash
   magick ... -define png:color-type=6 -define png:bit-depth=8 PNG32:out.png
   ```

5. Validate and preview:

   ```bash
   go run ./cmd/grub-themes lint your-theme
   go run ./cmd/grub-themes preview your-theme    # writes a PNG
   ```

6. Include `preview.png` in your theme directory and reference it from
   `theme.toml`. That is what people see when browsing.

`lint` must pass. CI runs it on every pull request and attaches the rendered
preview, so reviewers can see your theme without booting anything.

## Testing without GRUB

Three levels, in increasing cost:

| Command | Needs | What it tells you |
|---|---|---|
| `grub-themes lint` | nothing | manifest, file references, PNG encoding, font names |
| `grub-themes preview` | nothing | what the layout looks like |
| `grub-themes qemu` | `qemu`, `grub-mkrescue` | real GRUB rendering, no reboot |

`lint` and `preview` are pure Go and run anywhere. Use `qemu` if you have it —
it boots a throwaway image, so it cannot affect your own bootloader.

Only install a theme on hardware you can afford to reboot. `sudo ./install.sh
--dry-run` shows every step without changing anything.

## Building

```bash
git clone https://github.com/sarbojitrana/grub-themes.git
cd grub-themes
go build ./...
go run ./cmd/grub-themes list
```

Go 1.22 or newer.

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
Fix pixmap encoding in Cyberpunk theme
```

Explanation belongs in code comments or `AGENTS.md`, next to what it explains.

## Pull requests

- One change per PR. One theme per PR.
- Say what you tested and how (`lint`, `preview`, `qemu`, or real hardware).
- For code touching `internal/install`, say how you verified the rollback path.
  That code exists to stop people losing their boot configuration, and it is
  the part of this project where a bug is genuinely dangerous.

## Reporting bugs

Include your distro, GRUB version, screen resolution, and `/etc/default/grub`.
If a theme renders as nothing at all, run `grub-themes lint` first — it is
usually the PNG encoding.

## Hacktoberfest

This repository takes part in Hacktoberfest. Please make your contribution a
real one: a theme you designed, a bug you fixed, documentation that was
genuinely missing. Spam PRs will be marked as such.

Good first issues are labelled `good first issue`. Adding a theme is a great
first contribution and needs no Go.

## License

By contributing you agree that your work is licensed under the MIT License, as
in [LICENSE](LICENSE). Themes you submit must be yours to license, or carry a
compatible licence noted in the theme's `theme.toml`.
