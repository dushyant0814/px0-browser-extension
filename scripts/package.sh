#!/bin/sh
# Builds the Chrome Web Store upload: dist/px0-extension-<version>.zip.
# Usage: scripts/package.sh [VERSION]   (VERSION also stamps manifest.json in the zip)
set -eu
cd "$(dirname "$0")/.."

version=${1:-$(sed -n 's/.*"version": *"\([^"]*\)".*/\1/p' manifest.json | head -n 1)}
version=${version#v}
stage=$(mktemp -d)
trap 'rm -rf "$stage"' EXIT

cp -R manifest.json background.js content.js setup.html setup.js icons LICENSE "$stage/"
sed -i.bak "s/\"version\": *\"[^\"]*\"/\"version\": \"$version\"/" "$stage/manifest.json"
rm "$stage/manifest.json.bak"

mkdir -p dist
out="$PWD/dist/px0-extension-$version.zip"
rm -f "$out"
(cd "$stage" && zip -qr -X "$out" .)
echo "$out"
