BINARY := gumroad
VERSION ?= dev
LDFLAGS := -ldflags "-s -w -X github.com/antiwork/gumroad-cli/internal/cmd.Version=$(VERSION)"
PREFIX ?= /usr/local
DESTDIR ?=
BIN_DIR := $(DESTDIR)$(PREFIX)/bin
SHARE_DIR := $(DESTDIR)$(PREFIX)/share
MAN_DIR := $(DESTDIR)$(PREFIX)/share/man/man1
BASH_COMPLETION_DIR := $(SHARE_DIR)/bash-completion/completions
ZSH_COMPLETION_DIR := $(SHARE_DIR)/zsh/site-functions
FISH_COMPLETION_DIR := $(SHARE_DIR)/fish/vendor_completions.d
POWERSHELL_COMPLETION_DIR := $(SHARE_DIR)/powershell/Modules/gumroad
INSTALL ?= install

.PHONY: build install test test-cover test-race test-smoke lint clean snapshot man

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/gumroad

install: build man
	@mkdir -p "$(BIN_DIR)" "$(MAN_DIR)" \
		"$(BASH_COMPLETION_DIR)" \
		"$(ZSH_COMPLETION_DIR)" \
		"$(FISH_COMPLETION_DIR)" \
		"$(POWERSHELL_COMPLETION_DIR)"
	$(INSTALL) -m 0755 "$(BINARY)" "$(BIN_DIR)/$(BINARY)"
	@find man -name '*.1' -type f -exec $(INSTALL) -m 0644 {} "$(MAN_DIR)/" \;
	./$(BINARY) completion bash > "$(BASH_COMPLETION_DIR)/gumroad"
	./$(BINARY) completion zsh > "$(ZSH_COMPLETION_DIR)/_gumroad"
	./$(BINARY) completion fish > "$(FISH_COMPLETION_DIR)/gumroad.fish"
	./$(BINARY) completion powershell > "$(POWERSHELL_COMPLETION_DIR)/gumroad.ps1"

test:
	go test ./...

test-smoke:
	GUMROAD_SMOKE=1 go test ./test -run Smoke -count=1

COVER_MIN_PKG := 85
COVER_MIN_INFRA := 90

# Coverage thresholds intentionally target the substantive internal packages.
# Thin entrypoints/tooling packages such as cmd/gumroad, cmd/gen-man, and
# support-only packages like internal/testutil are validated by tests and CI,
# but are not threshold-gated here.
test-cover:
	go test ./... -cover > /tmp/gumroad-cover.out 2>&1; \
		TEST_EXIT=$$?; \
		cat /tmp/gumroad-cover.out; \
		if [ $$TEST_EXIT -ne 0 ]; then echo "FAIL: go test exited with status $$TEST_EXIT"; exit $$TEST_EXIT; fi
	@echo "---"
	@awk ' \
		/^ok/ && /coverage:/ { \
			gsub(/%/, ""); cov = $$(NF-2) + 0; pkg = $$2; \
			if (pkg ~ /internal\/(api|cmdutil|config|output|prompt)$$/) { \
				if (cov < $(COVER_MIN_INFRA)) { printf "FAIL: %s %.1f%% < $(COVER_MIN_INFRA)%%\n", pkg, cov; fail = 1 } \
				else { printf "OK: %s %.1f%%\n", pkg, cov } \
			} else if (pkg ~ /internal\/cmd(\/.*)?$$/) { \
				if (cov < $(COVER_MIN_PKG)) { printf "FAIL: %s %.1f%% < $(COVER_MIN_PKG)%%\n", pkg, cov; fail = 1 } \
				else { printf "OK: %s %.1f%%\n", pkg, cov } \
			} \
		} \
		END { if (fail) exit 1 }' /tmp/gumroad-cover.out
	@rm -f /tmp/gumroad-cover.out

test-race:
	go test -race ./...

lint:
	golangci-lint run

man:
	go run ./cmd/gen-man

clean:
	rm -f $(BINARY)
	rm -rf dist/ man/

snapshot:
	goreleaser release --snapshot --clean
