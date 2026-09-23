# Privacy policy: Open in px0

_Last updated: 24 September 2026_

Open in px0 does not collect, store or send any personal data to its
developers or to any third party.

## What the extension handles

- **The address of the GitHub or GitLab page you are on.** On
  `github.com` and `gitlab.com` repository pages, the extension passes the
  page URL to `px0-extension-host`, a program installed on your own computer.
  Nothing else on the page is read.
- **Nothing on other sites.** The extension does not run anywhere else.

## Where that data goes

Only to the helper program on your computer, over Chrome's native messaging.
The helper clones the repository from GitHub or GitLab with your own `git`
and opens it in px0 at `http://127.0.0.1`. Your git credentials stay in your
normal git setup. The extension never sees them.

The extension has no analytics and no remote servers, and it loads no remote
code.

## Stored data

Cloned repositories are kept in your operating system's cache folder
(`~/Library/Caches/px0/repositories` on macOS), and at most ten clean copies
are kept. You can delete that folder at any time.

## Contact

Open an issue at https://github.com/dushyant0814/px0-browser-extension/issues.
