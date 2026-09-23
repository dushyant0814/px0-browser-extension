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
