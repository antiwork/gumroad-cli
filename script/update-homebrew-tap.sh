#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 3 ]]; then
    echo "usage: $0 <tag> <checksums.txt> <tap-dir>" >&2
    exit 1
fi

TAG=$1
CHECKSUMS_FILE=$2
TAP_DIR=$3

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

mkdir -p "$TAP_DIR"
"$SCRIPT_DIR/render-homebrew-formula.sh" "$TAG" "$CHECKSUMS_FILE" "$TAP_DIR/Formula/gumroad.rb"

cat >"$TAP_DIR/README.md" <<'EOF'
# homebrew-cli

Homebrew tap for the Gumroad CLI.

```sh
brew install antiwork/cli/gumroad
```

If you previously installed the cask, switch once with:

```sh
brew uninstall --cask antiwork/cli/gumroad
brew install antiwork/cli/gumroad
```

This repo contains the generated Homebrew formula published from [antiwork/gumroad-cli](https://github.com/antiwork/gumroad-cli) on each release.
EOF

rm -f "$TAP_DIR/Casks/gumroad.rb"
rmdir "$TAP_DIR/Casks" 2>/dev/null || true
