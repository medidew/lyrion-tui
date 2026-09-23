#!/usr/bin/env bash
# Builds lyrion-tui, installs the binary to /usr/local/bin, and copies
# config.template.json to the current user's config directory (without
# overwriting an existing config).
#
# Run as your normal user, not with sudo: the config goes into *your* home
# directory, and sudo is only invoked for the step that writes to
# /usr/local/bin.
set -euo pipefail

BIN_DIR=/usr/local/bin
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/lyrion-tui"

if [[ $EUID -eq 0 ]]; then
    echo "error: run this as your normal user, not root - the config is installed into your home directory" >&2
    exit 1
fi

if ! command -v go >/dev/null; then
    echo "error: go is not installed or not on PATH" >&2
    exit 1
fi

cd "$(dirname "$0")"

build_dir=$(mktemp -d)
trap 'rm -rf "$build_dir"' EXIT

echo "Building lyrion-tui..."
go build \
    -ldflags "-X github.com/medidew/lyrion-tui/internal/config.installed=true" \
    -o "$build_dir/lyrion-tui" .

echo "Installing binary to $BIN_DIR (may prompt for your sudo password)..."
sudo install -m 755 "$build_dir/lyrion-tui" "$BIN_DIR/lyrion-tui"

mkdir -p "$CONFIG_DIR"
if [[ -e "$CONFIG_DIR/config.json" ]]; then
    echo "Keeping existing config at $CONFIG_DIR/config.json"
else
    cp config.template.json "$CONFIG_DIR/config.json"
    echo "Created $CONFIG_DIR/config.json - set server_address to your LMS server's host:port"
fi

echo "Done. Run: lyrion-tui"
