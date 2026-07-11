#!/bin/sh
set -e

# Install the latest `awt` release binary.
#   curl -fsSL https://zottiben.github.io/ai-worktree/install.sh | sh

REPO="zottiben/ai-worktree"
BIN="awt"

# Prefer ~/.local/bin when it's on PATH (no sudo). Fall back to /usr/local/bin.
if echo "$PATH" | tr ':' '\n' | grep -qx "$HOME/.local/bin"; then
  INSTALL_DIR="$HOME/.local/bin"
else
  INSTALL_DIR="/usr/local/bin"
fi

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

case "$OS" in
  darwin|linux) ;;
  *) echo "Unsupported OS: $OS (on Windows use install.ps1)" >&2; exit 1 ;;
esac

VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep '"tag_name"' | head -1 | sed -E 's/.*"([^"]+)".*/\1/')"
if [ -z "$VERSION" ]; then
  echo "Could not determine the latest release. Is there a published release yet?" >&2
  exit 1
fi

VERSION_NUM="${VERSION#v}"
FILENAME="${BIN}-v${VERSION_NUM}-${OS}-${ARCH}.tar.gz"
BASE="https://github.com/${REPO}/releases/download/${VERSION}"

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

echo "Downloading ${BIN} ${VERSION} for ${OS}/${ARCH}..."
curl -fsSL "${BASE}/${FILENAME}" -o "${TMPDIR}/${FILENAME}"

# Verify the checksum when checksums.txt is published and a hasher is available.
if curl -fsSL "${BASE}/checksums.txt" -o "${TMPDIR}/checksums.txt" 2>/dev/null; then
  expected="$(grep " ${FILENAME}\$" "${TMPDIR}/checksums.txt" | awk '{print $1}')"
  if [ -n "$expected" ]; then
    actual=""
    if command -v sha256sum >/dev/null 2>&1; then
      actual="$(sha256sum "${TMPDIR}/${FILENAME}" | awk '{print $1}')"
    elif command -v shasum >/dev/null 2>&1; then
      actual="$(shasum -a 256 "${TMPDIR}/${FILENAME}" | awk '{print $1}')"
    fi
    if [ -n "$actual" ] && [ "$actual" != "$expected" ]; then
      echo "Checksum mismatch for ${FILENAME}" >&2
      exit 1
    fi
  fi
fi

tar xzf "${TMPDIR}/${FILENAME}" -C "$TMPDIR"

if mkdir -p "$INSTALL_DIR" 2>/dev/null && [ -w "$INSTALL_DIR" ]; then
  mv "${TMPDIR}/${BIN}" "${INSTALL_DIR}/${BIN}"
  chmod +x "${INSTALL_DIR}/${BIN}"
else
  echo "Installing to ${INSTALL_DIR} (requires sudo)..."
  sudo mkdir -p "$INSTALL_DIR"
  sudo mv "${TMPDIR}/${BIN}" "${INSTALL_DIR}/${BIN}"
  sudo chmod +x "${INSTALL_DIR}/${BIN}"
fi

echo "Installed ${BIN} ${VERSION} to ${INSTALL_DIR}/${BIN}"
if ! echo "$PATH" | tr ':' '\n' | grep -qx "$INSTALL_DIR"; then
  echo
  echo "Note: ${INSTALL_DIR} is not on your PATH. Add it, e.g.:"
  echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
fi
