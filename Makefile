# grub-themes — build and install the application.
#
#   make                 build ./grub-themes
#   sudo make install    install system-wide (PREFIX=/usr/local)
#   make install-user    install into ~/.local, no root needed
#   sudo make uninstall
#
# Packagers: pass PREFIX=/usr DESTDIR=... as usual.

PREFIX  ?= /usr/local
DESTDIR ?=
GO      ?= go
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.Version=$(VERSION)

BIN     := grub-themes
BINDIR   = $(DESTDIR)$(PREFIX)/bin
DATADIR  = $(DESTDIR)$(PREFIX)/share
THEMEDIR = $(DATADIR)/grub-themes/themes

# Theme files, minus each theme's art sources: tools/ is for contributors, not
# for the boot loader.
THEME_FILES := $(shell find themes -type f -not -path 'themes/*/tools/*' 2>/dev/null)

.PHONY: all build install install-user uninstall test lint previews clean

all: build

build:
	$(GO) build -trimpath -ldflags '$(LDFLAGS)' -o $(BIN) ./cmd/grub-themes

install: build
	install -Dm755 $(BIN) $(BINDIR)/$(BIN)
	install -Dm644 packaging/grub-themes.desktop $(DATADIR)/applications/grub-themes.desktop
	install -Dm644 packaging/grub-themes.svg $(DATADIR)/icons/hicolor/scalable/apps/grub-themes.svg
	install -Dm644 README.md $(DATADIR)/doc/grub-themes/README.md
	install -Dm644 docs/build-your-own-theme.md $(DATADIR)/doc/grub-themes/build-your-own-theme.md
	install -Dm644 LICENSE $(DATADIR)/licenses/grub-themes/LICENSE
	@for f in $(THEME_FILES); do \
		install -Dm644 "$$f" "$(THEMEDIR)/$${f#themes/}"; \
	done
	-update-desktop-database $(DATADIR)/applications 2>/dev/null || true
	-gtk-update-icon-cache -qtf $(DATADIR)/icons/hicolor 2>/dev/null || true
	@echo
	@echo "  Installed. Run 'grub-themes', or find \"GRUB Themes\" in your launcher."

# ~/.local is on the default XDG data path, so themes installed there are found
# without any configuration.
install-user:
	$(MAKE) install PREFIX=$(HOME)/.local

uninstall:
	rm -f  $(BINDIR)/$(BIN)
	rm -f  $(DATADIR)/applications/grub-themes.desktop
	rm -f  $(DATADIR)/icons/hicolor/scalable/apps/grub-themes.svg
	rm -rf $(DATADIR)/grub-themes
	rm -rf $(DATADIR)/doc/grub-themes
	rm -rf $(DATADIR)/licenses/grub-themes
	@echo "  Removed. Any theme you applied is still in place; 'grub-themes remove' undoes that."

test:
	$(GO) test ./...

# What CI runs on every pull request.
lint: build
	$(GO) vet ./...
	./$(BIN) lint

previews: build
	./$(BIN) preview

clean:
	rm -f $(BIN)
