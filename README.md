# px0 browser extension

Open GitHub and GitLab repositories in local px0 by pressing `.` or clicking
the extension button.

This repository contains two parts:

- The Manifest V3 browser client (`manifest.json`, `background.js`, `content.js`).
- `host/`, a small Go program, `px0-extension-host`, that Chrome starts through
  Native Messaging. It clones repositories into a bounded cache and launches
  your installed, unmodified `px0` on them.

The extension never receives Git credentials or repository contents. See
[docs/browser-extension.md](docs/browser-extension.md) for behavior and limits.

## Install

Browsers cannot download or register native programs, so installing takes
one terminal command after the extension:

1. Install the extension. Its setup page opens automatically. It also opens
   when you click the toolbar button off a repository page, or press `.`
   before the helper is installed.
2. Copy the command it shows and run it in a terminal:

   ```sh
   curl -fsSL https://github.com/dushyant0814/px0-browser-extension/releases/latest/download/install.sh | sh -s -- EXTENSION_ID
   ```

   [install.sh](install.sh) downloads `px0-extension-host` for your platform
   from the latest GitHub release and verifies it against the release's
   `checksums.txt`. If px0 is missing it installs it from
   `https://px0.ai/install.sh`. It then installs the helper to
   `~/Library/Application Support/px0/` (macOS) or `~/.local/share/px0/`
   (Linux) and records the px0 path next to it, because browsers start native
   hosts with a minimal `PATH`. Finally it registers `ai.px0.launcher` with
   Chrome and with any Chromium, Brave, Edge or Arc profile it finds.
3. The setup page detects the helper and shows **Connected**.

Supported: macOS and Linux, amd64 and arm64, Chrome 116+.

The extension begins warming a repository 0.5 seconds after a supported page
loads. Pressing `.` sends an `open` request and replaces the current tab with
the local px0 URL when it is ready.

Re-run the installer after moving px0 or to update the helper.
`px0-extension-host -check` prints the helper version and the px0 it will
launch.

## Releasing

Pushing a `v*` tag runs [.github/workflows/release.yml](.github/workflows/release.yml).
It publishes `px0-extension-host-{darwin,linux}-{amd64,arm64}`, `install.sh`,
`checksums.txt` and a zip of the extension. Asset names carry no version, so
`releases/latest/download/...` always points at the newest helper.

## How the host runs px0

The host uses only px0's public CLI:

```sh
px0 -host 127.0.0.1 -port <free port> -no-open <repo>[/<file>:<line>]
```

A viewer counts as ready once `GET /static/app.js` succeeds. The viewer runs
in its own process group with stdio detached from Chrome's pipes, so it keeps
running after Chrome stops the host. Its output goes to `viewer.log` in the
repository's cache entry.

px0 is resolved in this order: `$PX0_BINARY`, `px0-extension-host.json`,
`PATH`, then `/opt/homebrew/bin`, `/usr/local/bin`, `~/go/bin` and
`~/.local/bin`.

## Development

1. Open `chrome://extensions`, enable Developer mode, choose **Load unpacked**
   and select this directory.
2. Build the helper from source and register it the same way the release
   installer does (needs Go 1.22+):

   ```sh
   ./install-host.sh EXTENSION_ID [/absolute/path/to/px0]
   ```

3. Run the tests:

   ```sh
   cd host && go test ./... && go vet ./...
   ```

   The tests run the test binary as a fake px0, so they do not need a px0
   build. When building px0 itself from source, use `make build`, not plain
   `go build`. Without `make build` the frontend bundle is missing and px0
   serves a blank page.

## Protocol

Messages use protocol version 1:

```json
{"version":1,"id":1,"action":"warm","url":"https://github.com/px0-ai/px0"}
```

Actions are `warm`, `open`, `status` and `ping`. Responses echo `id` and
return `{"ok":true}` or `{"ok":false,"error":"..."}`. A successful `open` also
contains `viewerUrl`. `ping` takes no URL and returns `hostVersion`, `px0` and
`px0Version`. The setup page uses it to confirm the install.

Both sides accept only HTTPS URLs on `github.com` and `gitlab.com`.
