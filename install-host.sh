#!/bin/sh
# Development install: builds the helper from host/ and registers it exactly as
# the release installer (install.sh) does. End users run install.sh instead.
set -eu

if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  echo "usage: $0 EXTENSION_ID [PX0_BINARY]" >&2
  exit 2
fi
script_dir=$(cd "$(dirname "$0")" && pwd)
if [ "$#" -eq 2 ]; then
  PX0_BINARY=$2
  export PX0_BINARY
fi
command -v go >/dev/null 2>&1 || { echo "Go is required to build the helper (https://go.dev/dl/)" >&2; exit 1; }

build=$(mktemp -d)
trap 'rm -rf "$build"' EXIT INT TERM
(cd "$script_dir/host" && go build -trimpath -ldflags="-s -w" -o "$build/px0-extension-host" .)
PX0_HOST_BINARY="$build/px0-extension-host" PX0_NO_INSTALL=1 sh "$script_dir/install.sh" "$1"
