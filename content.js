const WARM_DELAY_MS = 500;
const HOVER_DELAY_MS = 300;
let opening = false;
let warmTimer;
let lastURL;
let lastWarmed;
let hoverLink;
let hoverTimer;
const prefetched = new Set();

// repositoryKey returns "host/project@ref" for a repository page, or null for
// any other page. It identifies what the helper will clone, so moving between
// files of the same repository does not trigger another warm-up.
function repositoryKey(raw) {
  return parseRepository(raw)?.key ?? null;
}

// repositoryLinkKey is repositoryKey for links that point at the repository's
// code (its root, a tree or a blob), not at its issues, pull requests or other
// pages, which are rarely followed by ".".
function repositoryLinkKey(raw) {
  const repo = parseRepository(raw);
  return repo && (repo.tail.length === 0 || repo.tail[0] === "tree" || repo.tail[0] === "blob") ? repo.key : null;
}

function parseRepository(raw) {
  let url;
  try {
    url = new URL(raw);
  } catch {
    return null;
  }
  const parts = url.pathname.split("/").filter(Boolean);
  let project, tail;
  if (url.hostname === "github.com") {
    const reserved = new Set(["about", "account", "apps", "collections", "codespaces", "customer-stories", "dashboard", "enterprise", "events", "explore", "features", "issues", "login", "marketplace", "new", "notifications", "orgs", "pricing", "pulls", "readme", "resources", "search", "security", "sessions", "settings", "signup", "site", "solutions", "sponsors", "team", "topics", "trending", "users"]);
    if (parts.length < 2 || reserved.has(parts[0])) return null;
    project = parts.slice(0, 2);
    tail = parts.slice(2);
  } else if (url.hostname === "gitlab.com") {
    const reserved = new Set(["admin", "dashboard", "explore", "groups", "help", "projects", "search", "users"]);
    const marker = parts.indexOf("-");
    project = marker < 0 ? parts : parts.slice(0, marker);
    if (project.length < 2 || reserved.has(project[0])) return null;
    tail = marker < 0 ? [] : parts.slice(marker + 1);
  } else {
    return null;
  }
  const ref = (tail[0] === "blob" || tail[0] === "tree") && tail[1] ? tail[1] : "";
  return { key: `${url.hostname}/${project.join("/").toLowerCase()}@${ref}`, tail };
}

function editable(target) {
  return target instanceof Element && Boolean(target.closest("input, textarea, select, [contenteditable='true'], [role='textbox']"));
}

function notice(text, failed = false) {
  let node = document.getElementById("px0-extension-notice");
  if (!node) {
    node = document.createElement("div");
    node.id = "px0-extension-notice";
    Object.assign(node.style, {
      position: "fixed", top: "16px", left: "50%", transform: "translateX(-50%)",
      zIndex: "2147483647", padding: "10px 14px", borderRadius: "7px",
      font: "13px/1.4 ui-monospace, SFMono-Regular, Menlo, monospace",
      color: "#f0f6fc", background: "#161b22", border: "1px solid #30363d",
      boxShadow: "0 8px 24px #0006"
    });
    document.documentElement.appendChild(node);
  }
  node.style.borderColor = failed ? "#f85149" : "#30363d";
  node.textContent = text;
}

async function openInPx0() {
  if (opening) return;
  opening = true;
  const url = location.href;
  // Say whether the user is waiting on a clone, which is the slow part.
  const status = await chrome.runtime.sendMessage({ action: "status", url }).catch(() => null);
  notice(status?.state === "ready" ? "Opening in px0…" : "Cloning repository for px0…");
  let response;
  try {
    response = await chrome.runtime.sendMessage({ action: "open", url });
  } catch (error) {
    response = { ok: false, error: error.message };
  }
  if (response?.ok && response.viewerUrl) {
    location.replace(response.viewerUrl);
    return;
  }
  opening = false;
  notice(response?.error || "Could not connect to px0", true);
}

// GitHub and GitLab switch pages without reloading, so the URL is checked
// continuously rather than once at load. Each repository is warmed once,
// shortly after the user lands on it.
function checkLocation() {
  if (location.href === lastURL) return;
  lastURL = location.href;
  clearTimeout(warmTimer);
  const key = repositoryKey(location.href);
  if (!key || key === lastWarmed) return;
  warmTimer = setTimeout(() => {
    lastWarmed = key;
    chrome.runtime.sendMessage({ action: "warm", url: location.href }).catch(() => {});
  }, WARM_DELAY_MS);
}

checkLocation();
setInterval(checkLocation, 500);
addEventListener("popstate", checkLocation);

// Hovering a repository link for a moment starts cloning it before the click.
// The helper keeps these in a separate, smaller cache budget and limits how
// many run at once, so hovering never evicts repositories you opened.
function prefetch(link) {
  const key = repositoryLinkKey(link.href);
  // Links within the current repository are covered by its own warm-up.
  const project = (k) => k?.slice(0, k.lastIndexOf("@"));
  if (!key || project(key) === project(repositoryKey(location.href)) || prefetched.has(key)) return;
  prefetched.add(key);
  chrome.runtime.sendMessage({ action: "prefetch", url: link.href })
    .then((response) => {
      // Busy or failed: let a later hover try again.
      if (!response?.ok || response.state === "skipped") prefetched.delete(key);
    })
    .catch(() => prefetched.delete(key));
}

document.addEventListener("mouseover", (event) => {
  const link = event.target instanceof Element ? event.target.closest("a[href]") : null;
  if (!link || link === hoverLink) return;
  clearTimeout(hoverTimer);
  hoverLink = link;
  hoverTimer = setTimeout(() => {
    if (hoverLink === link) prefetch(link);
  }, HOVER_DELAY_MS);
}, { passive: true });

document.addEventListener("mouseout", (event) => {
  if (hoverLink && !hoverLink.contains(event.relatedTarget)) {
    clearTimeout(hoverTimer);
    hoverLink = undefined;
  }
}, { passive: true });

addEventListener("keydown", (event) => {
  if (event.key !== "." || event.repeat || event.ctrlKey || event.metaKey || event.altKey || editable(event.target)) return;
  // Off repository pages, leave "." to the site.
  if (!repositoryKey(location.href)) return;
  event.preventDefault();
  event.stopImmediatePropagation();
  openInPx0();
}, true);
