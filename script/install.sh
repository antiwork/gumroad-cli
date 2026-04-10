#!/usr/bin/env bash
set -euo pipefail

# Install script for the Gumroad CLI.
# Usage:
#   curl -fsSL https://gumroad.com/install-cli | bash
#   GUMROAD_BIN_DIR=~/.bin bash install.sh

REPO="antiwork/gumroad-cli"

if [[ -z "${HOME:-}" ]]; then
    echo "Error: HOME is not set." >&2
    exit 1
fi

BIN_DIR="${GUMROAD_BIN_DIR:-$HOME/.local/bin}"

if command -v cygpath &>/dev/null; then
    BIN_DIR=$(cygpath -u "$BIN_DIR")
fi

SHA_CMD=""
WORK_DIR=""

binary_name() {
    if [[ "$OS" == "windows" ]]; then
        echo "gumroad.exe"
    else
        echo "gumroad"
    fi
}

main() {
    detect_platform
    check_requirements
    resolve_version

    WORK_DIR=$(mktemp -d)
    trap 'rm -rf "$WORK_DIR"' EXIT

    download_and_verify
    install_binary
    setup_path

    echo "gumroad ${VERSION} installed to ${BIN_DIR}/$(binary_name)"
}

check_requirements() {
    if ! command -v curl &>/dev/null; then
        echo "Error: curl is required but not found." >&2
        exit 1
    fi

    if command -v sha256sum &>/dev/null; then
        SHA_CMD="sha256sum"
    elif command -v shasum &>/dev/null; then
        SHA_CMD="shasum -a 256"
    else
        echo "Error: sha256sum or shasum is required but neither was found." >&2
        exit 1
    fi

    if [[ "$OS" == "windows" ]]; then
        if ! command -v unzip &>/dev/null; then
            echo "Error: unzip is required on Windows but not found. Install it with: pacman -S unzip" >&2
            exit 1
        fi
    else
        if ! command -v tar &>/dev/null; then
            echo "Error: tar is required but not found." >&2
            exit 1
        fi
    fi
}

detect_platform() {
    local os arch

    os=$(uname -s | tr '[:upper:]' '[:lower:]')
    case "$os" in
        darwin)               OS="darwin" ;;
        linux)                OS="linux" ;;
        mingw*|msys*|cygwin*) OS="windows" ;;
        *)
            echo "Error: unsupported OS: $os. Supported: macOS, Linux, Windows (Git Bash/MSYS2)." >&2
            exit 1
            ;;
    esac

    arch=$(uname -m)
    case "$arch" in
        x86_64|amd64)   ARCH="amd64" ;;
        aarch64|arm64)  ARCH="arm64" ;;
        *)
            echo "Error: unsupported architecture: $arch. Supported: x86_64, arm64." >&2
            exit 1
            ;;
    esac
}

resolve_version() {
    local url version
    url=$(curl -fsS -o /dev/null -w '%{redirect_url}' --max-redirs 0 \
        "https://github.com/${REPO}/releases/latest" 2>/dev/null) || true
    version="${url##*/}"

    if [[ ! $version =~ ^v[0-9]+\.[0-9]+\.[0-9]+ ]]; then
        echo "Error: no published release found. Check https://github.com/${REPO}/releases" >&2
        exit 1
    fi

    VERSION="$version"
}

download_and_verify() {
    local ext="tar.gz"
    if [[ "$OS" == "windows" ]]; then
        ext="zip"
    fi

    local archive="gumroad-cli_${OS}_${ARCH}.${ext}"
    local base_url="https://github.com/${REPO}/releases/download/${VERSION}"

    echo "Downloading gumroad ${VERSION} for ${OS}/${ARCH}..."

    local pid_archive pid_checksums
    curl -fsSL "${base_url}/${archive}" -o "${WORK_DIR}/${archive}" &
    pid_archive=$!
    curl -fsSL "${base_url}/checksums.txt" -o "${WORK_DIR}/checksums.txt" &
    pid_checksums=$!

    if ! wait "$pid_archive"; then
        echo "Error: failed to download ${archive}" >&2
        exit 1
    fi
    if ! wait "$pid_checksums"; then
        echo "Error: failed to download checksums.txt" >&2
        exit 1
    fi

    local expected actual
    expected=$(awk -v f="$archive" '$2 == f || $2 == ("*" f) {print $1; exit}' "${WORK_DIR}/checksums.txt")
    if [[ -z "$expected" ]]; then
        echo "Error: archive ${archive} not found in checksums.txt" >&2
        exit 1
    fi

    actual=$(cd "$WORK_DIR" && $SHA_CMD "$archive" | awk '{print $1}')
    if [[ "$expected" != "$actual" ]]; then
        echo "Error: checksum mismatch for ${archive}" >&2
        echo "  expected: ${expected}" >&2
        echo "  got:      ${actual}" >&2
        exit 1
    fi

    if [[ "$ext" == "zip" ]]; then
        unzip -q "${WORK_DIR}/${archive}" -d "$WORK_DIR"
    else
        tar -xzf "${WORK_DIR}/${archive}" -C "$WORK_DIR"
    fi
}

install_binary() {
    local name
    name=$(binary_name)

    mkdir -p "$BIN_DIR"
    cp "${WORK_DIR}/${name}" "${BIN_DIR}/${name}"
    if [[ "$OS" != "windows" ]]; then
        chmod +x "${BIN_DIR}/${name}"
    fi

    if [[ -d "${WORK_DIR}/man" ]] && [[ "$OS" != "windows" ]]; then
        local man_dir="${HOME}/.local/share/man/man1"
        mkdir -p "$man_dir"
        cp "${WORK_DIR}"/man/*.1 "$man_dir/" 2>/dev/null || true
    fi
}

setup_path() {
    if [[ ":$PATH:" == *":$BIN_DIR:"* ]]; then
        return 0
    fi

    local shell_rc=""
    case "${SHELL:-}" in
        */zsh)  shell_rc="$HOME/.zshrc" ;;
        */bash)
            if [[ "$OS" == "windows" ]]; then
                # Git Bash opens login shells that read these in order
                if [[ -f "$HOME/.bash_profile" ]]; then
                    shell_rc="$HOME/.bash_profile"
                elif [[ -f "$HOME/.bash_login" ]]; then
                    shell_rc="$HOME/.bash_login"
                else
                    shell_rc="$HOME/.profile"
                fi
            elif [[ -f "$HOME/.bashrc" ]]; then
                shell_rc="$HOME/.bashrc"
            elif [[ -f "$HOME/.bash_profile" ]]; then
                shell_rc="$HOME/.bash_profile"
            elif [[ -f "$HOME/.bash_login" ]]; then
                shell_rc="$HOME/.bash_login"
            else
                shell_rc="$HOME/.profile"
            fi
            ;;
        */fish) shell_rc="$HOME/.config/fish/config.fish" ;;
        *)      shell_rc="$HOME/.profile" ;;
    esac

    if [[ -f "$shell_rc" ]] && grep -qF "$BIN_DIR" "$shell_rc" 2>/dev/null; then
        return 0
    fi

    mkdir -p "$(dirname "$shell_rc")"

    if [[ "${SHELL:-}" == */fish ]]; then
        echo "" >> "$shell_rc"
        echo "# Added by gumroad installer" >> "$shell_rc"
        echo "fish_add_path -- \"$BIN_DIR\"" >> "$shell_rc"
    else
        echo "" >> "$shell_rc"
        echo "# Added by gumroad installer" >> "$shell_rc"
        echo "export PATH=\"$BIN_DIR:\$PATH\"" >> "$shell_rc"
    fi

    echo "Added ${BIN_DIR} to PATH in ${shell_rc}"
    echo "Run: source ${shell_rc}"
}

main
