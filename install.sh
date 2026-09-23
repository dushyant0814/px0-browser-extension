#!/bin/sh
# Installs px0-extension-host, the native helper for the px0 browser extension.
#
# Usage (the extension's setup page shows this with your extension ID filled in):
#   curl -fsSL https://github.com/dushyant0814/px0-browser-extension/releases/latest/download/install.sh | sh -s -- EXTENSION_ID
#
# Environment variables:
#   HOST_VERSION        - helper release to install, e.g. "0.1.0" (default: latest)
#   PX0_EXTENSION_REPO  - GitHub repository hosting releases (default: dushyant0814/px0-browser-extension)
#   PX0_BINARY          - px0 to launch (default: px0 on PATH or a common install location)
#   PX0_NO_INSTALL=1    - fail instead of installing px0 when it is missing
#   PX0_HOST_BASE_URL   - download from this URL instead of GitHub releases (mirrors, testing)
#   PX0_HOST_BINARY     - install this local helper build instead of downloading one
set -eu

REPO="${PX0_EXTENSION_REPO:-dushyant0814/px0-browser-extension}"
HOST_VERSION="${HOST_VERSION:-latest}"
HOST_NAME="ai.px0.launcher"

say() { printf ' \033[38;5;71m✓\033[0m %s\n' "$1"; }
step() { printf ' \033[38;5;208m›\033[0m %s\n' "$1"; }
die() { printf ' \033[38;5;167m✗\033[0m %s\n' "$1" >&2; exit 1; }

if [ "$#" -ne 1 ]; then
  die "usage: install.sh EXTENSION_ID"
fi
extension_id=$1
case "$extension_id" in
  *[!a-p]*|'') die "invalid Chrome extension ID: $extension_id" ;;
esac
[ "${#extension_id}" -eq 32 ] || die "Chrome extension IDs must contain 32 characters"

case "$(uname -s)" in
  Darwin)
    os=darwin
    support_dir="$HOME/Library/Application Support/px0"
    ;;
  Linux)
    os=linux
    support_dir="${XDG_DATA_HOME:-$HOME/.local/share}/px0"
    ;;
  *) die "the px0 extension helper currently supports macOS and Linux" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "unsupported architecture: $(uname -m)" ;;
esac

fetch() {
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL --connect-timeout 10 --max-time 300 --retry 2 -o "$2" "$1"
  elif command -v wget >/dev/null 2>&1; then
    wget -q -T 30 -t 3 -O "$2" "$1"
  else
    die "curl or wget is required"
  fi
}

sha256() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  else
    shasum -a 256 "$1" | cut -d' ' -f1
  fi
}

json_escape() {
  printf '%s' "$1" | sed 's/\\/\\\\/g; s/"/\\"/g'
}

find_px0() {
  # An explicit PX0_BINARY is returned as-is; the -version check rejects a bad one.
  if [ -n "${PX0_BINARY:-}" ]; then
    printf '%s\n' "$PX0_BINARY"
    return
  fi
  if p=$(command -v px0 2>/dev/null) && [ -x "$p" ]; then
    printf '%s\n' "$p"
    return
  fi
  for d in "$HOME/.local/bin" "$HOME/bin" /usr/local/bin /opt/homebrew/bin "$HOME/go/bin"; do
    if [ -x "$d/px0" ]; then
      printf '%s\n' "$d/px0"
      return
    fi
  done
}

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT INT TERM

# 1. px0 itself. The helper launches the user's px0; it does not bundle one.
px0_binary=$(find_px0 || true)
if [ -z "$px0_binary" ]; then
  [ "${PX0_NO_INSTALL:-}" = 1 ] && die "px0 not found; install it from https://px0.ai or set PX0_BINARY"
  step "px0 not found; installing it with https://px0.ai/install.sh"
  fetch https://px0.ai/install.sh "$tmp/px0-install.sh"
  sh "$tmp/px0-install.sh"
  px0_binary=$(find_px0 || true)
  [ -n "$px0_binary" ] || die "px0 installed but could not be located; re-run with PX0_BINARY=/path/to/px0"
fi
case "$px0_binary" in
  /*) ;;
  *) px0_binary=$(cd "$(dirname "$px0_binary")" && pwd)/$(basename "$px0_binary") ;;
esac
"$px0_binary" -version >/dev/null 2>&1 || die "$px0_binary does not run as px0 ('px0 -version' failed)"
say "Using px0 at $px0_binary"

# 2. The helper binary, verified against the release's checksums.
asset="px0-extension-host-$os-$arch"
if [ -n "${PX0_HOST_BINARY:-}" ]; then
  cp "$PX0_HOST_BINARY" "$tmp/$asset"
else
  if [ -n "${PX0_HOST_BASE_URL:-}" ]; then
    base=${PX0_HOST_BASE_URL%/}
  elif [ "$HOST_VERSION" = latest ]; then
    base="https://github.com/$REPO/releases/latest/download"
  else
    base="https://github.com/$REPO/releases/download/v${HOST_VERSION#v}"
  fi
  step "Downloading $asset ($HOST_VERSION)"
  fetch "$base/$asset" "$tmp/$asset" || die "download failed: $base/$asset"
  fetch "$base/checksums.txt" "$tmp/checksums.txt" || die "download failed: $base/checksums.txt"
  want=$(awk -v f="$asset" '$2 == f || $2 == "*" f { print $1 }' "$tmp/checksums.txt")
  [ -n "$want" ] || die "checksums.txt has no entry for $asset"
  [ "$(sha256 "$tmp/$asset")" = "$want" ] || die "checksum mismatch for $asset"
  say "Verified checksum"
fi

mkdir -p "$support_dir"
host_binary="$support_dir/px0-extension-host"
# Replace via rename so a running host is never left with a half-written file.
cp "$tmp/$asset" "$support_dir/.px0-extension-host.new"
chmod 755 "$support_dir/.px0-extension-host.new"
mv -f "$support_dir/.px0-extension-host.new" "$host_binary"
# Earlier development installs registered px0 itself, via this symlink.
[ -L "$support_dir/px0-native-host" ] && rm "$support_dir/px0-native-host"

# Chrome starts native hosts with a minimal PATH, so record px0's location.
cat >"$support_dir/px0-extension-host.json" <<EOF
{"px0": "$(json_escape "$px0_binary")"}
EOF
"$host_binary" -check >/dev/null || die "installed helper cannot run px0; see: '$host_binary' -check"
say "Installed $host_binary ($("$host_binary" -check | head -n 1))"

# 3. Register with every Chromium-family browser present. Chrome is always
#    registered so installing the browser later still works.
if [ "$os" = darwin ]; then
  app="$HOME/Library/Application Support"
  set -- "$app/Google/Chrome" "$app/Google/Chrome Beta" "$app/Chromium" \
    "$app/BraveSoftware/Brave-Browser" "$app/Microsoft Edge" "$app/Arc/User Data"
else
  cfg="${XDG_CONFIG_HOME:-$HOME/.config}"
  set -- "$cfg/google-chrome" "$cfg/google-chrome-beta" "$cfg/chromium" \
    "$cfg/BraveSoftware/Brave-Browser" "$cfg/microsoft-edge"
fi
chrome_dir=$1
for browser_dir in "$@"; do
  [ -d "$browser_dir" ] || [ "$browser_dir" = "$chrome_dir" ] || continue
  mkdir -p "$browser_dir/NativeMessagingHosts"
  cat >"$browser_dir/NativeMessagingHosts/$HOST_NAME.json" <<EOF
{
  "name": "$HOST_NAME",
  "description": "Opens GitHub and GitLab repositories in px0",
  "path": "$(json_escape "$host_binary")",
  "type": "stdio",
  "allowed_origins": ["chrome-extension://$extension_id/"]
}
EOF
  say "Registered with ${browser_dir#"$HOME"/}"
done

say "Done. Return to the px0 setup tab; it connects automatically."
