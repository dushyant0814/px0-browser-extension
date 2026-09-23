# Chrome Web Store listing

Copy these into the developer dashboard.

## Store listing tab

**Name** (from manifest): Open in px0

**Summary** (from manifest, 132 characters max):
Press . on a GitHub or GitLab repository to open it in px0.

**Category:** Developer Tools
**Language:** English

**Description:**

```
Press "." on any GitHub or GitLab repository to open it in px0, a fast code viewer that runs on your own machine.

HOW IT WORKS
• Open a repository page. In the background the extension starts a quick, shallow clone to your computer.
• Press "." (or click the toolbar button). The same tab opens the repository in px0 at http://127.0.0.1.
• File links and line anchors carry over: github.com/org/repo/blob/main/app.go#L42 opens app.go at line 42.

WHY
• Full-speed search, symbol navigation and go-to-definition on your own machine.
• Your code stays local: the clone uses your own git and your own credentials.
• Repositories you have visited reopen almost instantly from a bounded local cache.

SETUP (ONE TIME)
Browsers cannot run programs by themselves, so px0 needs a small helper. After installing, the extension opens a setup page with a single command to paste into a terminal. It downloads the helper from the project's GitHub releases, checks its checksum and registers it with your browser. The page shows "Connected" when it is done.

Supports macOS and Linux. Works on github.com and gitlab.com.

Open source: https://github.com/dushyant0814/px0-browser-extension
```

**Graphics** (in this folder):

| Asset | File | Size |
|---|---|---|
| Store icon | `../icons/icon128.png` | 128×128 |
| Screenshot 1 | `screenshot-1-px0.jpg` | 1280×800 |
| Screenshot 2 | `screenshot-2-how-it-works.jpg` | 1280×800 |
| Screenshot 3 | `screenshot-3-setup.jpg` | 1280×800 |
| Small promo tile | `promo-small.jpg` | 440×280 |

**Homepage URL:** https://github.com/dushyant0814/px0-browser-extension
**Support URL:** https://github.com/dushyant0814/px0-browser-extension/issues

## Privacy practices tab

**Single purpose:**

```
Opens the GitHub or GitLab repository you are viewing in px0, a code viewer running on your own computer, when you press "." or click the toolbar button.
```

**Permission justifications:**

- **nativeMessaging**
  ```
  Required to talk to px0-extension-host, the companion program the user installs locally. The extension sends it the current repository URL. The helper clones the repository and starts px0, which a browser extension cannot do itself.
  ```
- **activeTab**
  ```
  When the user clicks the toolbar button, the extension reads the current tab's URL to find which repository to open, then points that tab at the local px0 viewer.
  ```
- **Host permissions** (the content script on github.com and gitlab.com)
  ```
  The content script listens for the "." key on GitHub and GitLab repository pages and reads the page URL, so it can start a background clone and open the repository in px0. It does not read or change page content.
  ```

**Remote code:** No, I am not using remote code.

**Data usage:** tick nothing. The page URL goes only to a program on the
user's own machine. Nothing is collected by, or sent to, the developer or
anyone else. Then tick all three certifications:

- I do not sell or transfer user data to third parties, outside of the approved use cases
- I do not use or transfer user data for purposes that are unrelated to my item's single purpose
- I do not use or transfer user data to determine creditworthiness or for lending purposes

**Privacy policy URL:**
https://github.com/dushyant0814/px0-browser-extension/blob/main/PRIVACY.md

## Distribution tab

- **Visibility:** Public, or Unlisted while testing (anyone with the link can install it).
- **Regions:** All regions.
