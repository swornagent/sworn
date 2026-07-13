#!/bin/sh
# sworn installer — downloads the latest release binary for your OS/arch.
#
#   curl -fsSL https://sworn.sh/install.sh | sh
#
# Overrides (env vars):
#   SWORN_VERSION       pin a version tag, e.g. v0.1.0 (default: latest release)
#   SWORN_INSTALL_DIR   where to install (default: /usr/local/bin, else ~/.local/bin)
#
# Windows users: use Scoop instead — `scoop install swornagent/sworn`.
set -eu

REPO="swornagent/sworn"
BIN="sworn"

err() { echo "install: $*" >&2; exit 1; }

# --- pick a downloader ---
if command -v curl >/dev/null 2>&1; then
  dl() { curl -fsSL "$1"; }
  dlf() { curl -fsSL "$1" -o "$2"; }
elif command -v wget >/dev/null 2>&1; then
  dl() { wget -qO- "$1"; }
  dlf() { wget -qO "$2" "$1"; }
else
  err "need curl or wget"
fi

# --- detect os/arch (must match GoReleaser's name_template) ---
os=$(uname -s)
case "$os" in
  Linux)  os=linux ;;
  Darwin) os=darwin ;;
  *) err "unsupported OS '$os' — on Windows use: scoop install swornagent/$BIN" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) err "unsupported arch '$arch' (supported: amd64, arm64)" ;;
esac

# --- resolve version tag ---
tag="${SWORN_VERSION:-}"
if [ -z "$tag" ]; then
  tag=$(dl "https://api.github.com/repos/$REPO/releases/latest" \
        | grep '"tag_name":' | head -1 | cut -d'"' -f4)
  [ -n "$tag" ] || err "could not resolve the latest release tag"
fi
version="${tag#v}"   # GoReleaser archive names carry the version without the leading 'v'

archive="${BIN}_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"

echo "install: $BIN $tag ($os/$arch)"

# --- download + verify + extract in a temp dir ---
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
dlf "$base/$archive" "$tmp/$archive" || err "download failed: $base/$archive"

# Optional but preferred: verify against the published checksums.
if dlf "$base/checksums.txt" "$tmp/checksums.txt" 2>/dev/null; then
  sum=""
  if command -v sha256sum >/dev/null 2>&1; then sum="sha256sum"; elif command -v shasum >/dev/null 2>&1; then sum="shasum -a 256"; fi
  if [ -n "$sum" ]; then
    want=$(grep " ${archive}\$" "$tmp/checksums.txt" | awk '{print $1}')
    got=$(cd "$tmp" && $sum "$archive" | awk '{print $1}')
    [ -n "$want" ] && [ "$want" = "$got" ] || err "checksum mismatch for $archive"
    echo "install: checksum ok"
  fi
fi

tar -xzf "$tmp/$archive" -C "$tmp"
[ -f "$tmp/$BIN" ] || err "archive did not contain the $BIN binary"
chmod +x "$tmp/$BIN"

# --- choose an install dir (writable, on PATH if possible) ---
dir="${SWORN_INSTALL_DIR:-}"
if [ -z "$dir" ]; then
  if [ -w /usr/local/bin ]; then dir=/usr/local/bin
  elif command -v sudo >/dev/null 2>&1; then dir=/usr/local/bin; sudo=1
  else dir="$HOME/.local/bin"; fi
fi
mkdir -p "$dir" 2>/dev/null || true

if [ "${sudo:-0}" = 1 ]; then
  sudo mv "$tmp/$BIN" "$dir/$BIN"
else
  mv "$tmp/$BIN" "$dir/$BIN" 2>/dev/null || err "cannot write to $dir (set SWORN_INSTALL_DIR to a writable path)"
fi

echo "install: $BIN -> $dir/$BIN"
case ":$PATH:" in
  *":$dir:"*) : ;;
  *) echo "install: note — $dir is not on your PATH; add it, e.g.  export PATH=\"$dir:\$PATH\"" ;;
esac
"$dir/$BIN" version 2>/dev/null || true
