# Browser Extension

The optional px0 browser extension turns `.` on a GitHub or GitLab repository
page into **Open in px0**. It uses the locally installed px0 binary, so source
code and Git credentials never pass through an intermediary service. A small
native helper, `px0-extension-host`, manages the repository cache and starts
px0; px0 itself needs no extension-specific support.

## How warm-up works

After a supported repository page remains open for 0.5 seconds, the extension
sends a `warm` message to the helper through Chrome Native Messaging. The
helper starts a depth-one, no-tags, single-branch partial clone in the
background and skips Git LFS payload smudging. Pressing `.` sends `open`; it
joins an existing clone job when one is running and otherwise reuses the cached
checkout. The helper then launches px0 on it, or reuses a px0 viewer already
serving that checkout.

Cached repositories live under the operating system's user cache directory,
not inside an existing workspace. The helper keeps at most ten clean entries.
It skips repositories with working-tree changes and repositories served by an
active viewer, so cache eviction does not discard agent edits or remove files
from a running session.

## Supported pages

- GitHub repositories and single-segment branch/file URLs.
- GitLab.com repositories, including nested group paths, and single-segment
  branch/file URLs.
- Source-line fragments such as `#L42` open the corresponding line in px0.

Branch names containing `/` are ambiguous in forge URLs and are not supported
yet. GitHub pull-request pages currently warm and open the repository checkout;
the existing full PR review flow remains available from `px0 <pr-url>`.

## Security boundary

Both the extension service worker and the native host independently accept only
credential-free HTTPS URLs on `github.com` and `gitlab.com`. The extension sends
only the current page URL. Git authentication remains inside the user's normal
Git credential helper.

## Installing the helper

Browsers cannot download or register native programs, so the extension's
setup page asks for one terminal command. [install.sh](../install.sh):

1. Finds px0, or installs it from `https://px0.ai/install.sh`.
2. Downloads the gzipped `px0-extension-host` for your OS and CPU from the
   latest GitHub release and checks it against `checksums.txt`.
3. Installs it to `~/Library/Application Support/px0/` (macOS) or
   `~/.local/share/px0/` (Linux). It records px0's path next to the helper,
   because browsers start native hosts with a minimal `PATH`.
4. Registers `ai.px0.launcher` with Chrome and any Chromium, Brave, Edge or
   Arc profile it finds. Only your extension ID is allowed to use it.

Re-run it after moving px0 or to update the helper.
`px0-extension-host -check` prints the helper version and the px0 it will
launch.

## How the helper runs px0

The helper uses only px0's public CLI:

```sh
px0 -host 127.0.0.1 -port <free port> -no-open <repo>[/<file>:<line>]
```

A viewer counts as ready once `GET /static/app.js` succeeds. It runs in its
own process group, detached from Chrome, so it keeps running after Chrome
stops the helper. Its output goes to `viewer.log` in the repository's cache
entry.

px0 is looked up in this order: `$PX0_BINARY`, `px0-extension-host.json`,
`PATH`, then `/opt/homebrew/bin`, `/usr/local/bin`, `~/go/bin` and
`~/.local/bin`.

## Protocol

Messages use protocol version 1:

```json
{"version":1,"id":1,"action":"warm","url":"https://github.com/px0-ai/px0"}
```

Actions are `warm`, `open`, `status` and `ping`. Responses echo `id` and
return `{"ok":true}` or `{"ok":false,"error":"..."}`. A successful `open` also
returns `viewerUrl`. `ping` takes no URL and returns `hostVersion`, `px0` and
`px0Version`; the setup page uses it to detect the install.

## Releasing

Pushing a `v*` tag runs [release.yml](../.github/workflows/release.yml). It
publishes the helper for `{darwin,linux}-{amd64,arm64}` (plain and gzipped),
`install.sh`, `checksums.txt` and a zip of the extension. Asset names carry no
version, so `releases/latest/download/...` always points at the newest helper.

When building px0 itself from source, use `make build`, not plain `go build`.
Without it the frontend bundle is missing and px0 serves a blank page.
