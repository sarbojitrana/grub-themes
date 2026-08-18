# Contributing

Thanks for taking an interest. Bug reports, patches and ideas are all welcome.

Read [AGENTS.md](AGENTS.md) first. GRUB fails silently in several
interesting ways, and every constraint documented there exists because
something broke.

## Signed commits are required

**Every commit must carry a verified signature.** Unsigned commits will not be
merged, and `main` is protected to reject them.

Set it up once:

```bash
# GPG
gpg --quick-generate-key "Your Name <you@example.com>" ed25519 sign 1y
gpg --list-secret-keys --keyid-format=long          # note the key id
git config --global user.signingkey <KEY_ID>
git config --global commit.gpgsign true
gpg --armor --export <KEY_ID>                       # add this to GitHub
```

Or SSH, which is simpler if you already push over SSH:

```bash
git config --global gpg.format ssh
git config --global user.signingkey ~/.ssh/id_ed25519.pub
git config --global commit.gpgsign true
```

Add the key to GitHub under *Settings → SSH and GPG keys* as a **signing** key,
not just an authentication key. Check your work:

```bash
git log --format='%h %G? %s'      # G = good signature, N = unsigned
```

`git rebase` and especially `git filter-branch` **drop signatures**. If you
rewrite history, re-sign before pushing:

```bash
git rebase --root --exec 'git commit --amend --no-edit -S'
```

## Making changes

Never hand-edit the files in `theme/` that have a generator. Edit the source
and rebuild, so the change is reproducible:

| To change | Edit | Then run |
|---|---|---|
| background art | `tools/background.svg` | `./tools/build-background.sh` |
| highlight, progress bar, terminal box | `tools/build-assets.sh` | `./tools/build-assets.sh` |
| fonts | — | `./tools/build-fonts.sh /path/Regular.ttf /path/Bold.ttf` |
| layout, colours, geometry | `theme/theme.txt` | — |

`build-assets.sh` verifies the PNG encoding and exits non-zero if GRUB would
not be able to decode the result. **Run it before every commit that touches a
pixmap.** If you swap fonts, copy the names `build-fonts.sh` prints into
`theme.txt` — GRUB matches fonts by the name inside the file, not the path.

## Testing

There is no preview mode, and a bad theme is only visible after a reboot.
Compose your change first and look at it:

```bash
# render the menu as GRUB would lay it out, using the geometry from theme.txt
magick theme/select_c.png -resize 766x44! /tmp/c.png
magick theme/select_w.png /tmp/c.png theme/select_e.png +append /tmp/pill.png
magick theme/background.png /tmp/pill.png -geometry +115+324 -composite /tmp/preview.png
```

Then, on a machine you can afford to reboot:

```bash
sudo ./install.sh --dry-run     # shows every step, changes nothing
sudo ./install.sh
```

Please say in the PR which distro, GRUB version and resolution you tested on.

## Commit messages

One short subject line, imperative mood, no body. Aim for ~50 characters.

```
Brighten theme; move menu beside the reactor
```

## Pull requests

- One change per PR.
- Include a screenshot or a composed preview for anything visual.
- If you touched `install.sh`, say how you verified the rollback path — that
  code exists to stop someone losing their boot config.

## Reporting bugs

Include your distro, GRUB version, resolution, and the contents of
`/etc/default/grub`. If something renders as nothing at all, check the encoding
first — it is almost always this:

```bash
./tools/build-assets.sh
```

## License

By contributing you agree that your work is licensed under the MIT License, as
in [LICENSE](LICENSE).
