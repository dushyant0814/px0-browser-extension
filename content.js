const WARM_DELAY_MS = 500;
let opening = false;
let warmTimer;
let lastURL;
let lastWarmed;

// repositoryKey returns "host/project@ref" for a repository page, or null for
// any other page. It identifies what the helper will clone, so moving between
// files of the same repository does not trigger another warm-up.
function repositoryKey(raw) {
  const url = new URL(raw);
  const parts = url.pathname.split("/").filter(Boolean);
  let project, tail;
  if (url.hostname === "github.com") {
    const reserved = new Set(["collections", "codespaces", "enterprise", "events", "explore", "features", "issues", "login", "marketplace", "new", "notifications", "orgs", "pricing", "pulls", "search", "settings", "signup", "sponsors", "topics", "trending"]);
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
  return `${url.hostname}/${project.join("/").toLowerCase()}@${ref}`;
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

addEventListener("keydown", (event) => {
  if (event.key !== "." || event.repeat || event.ctrlKey || event.metaKey || event.altKey || editable(event.target)) return;
  // Off repository pages, leave "." to the site.
  if (!repositoryKey(location.href)) return;
  event.preventDefault();
  event.stopImmediatePropagation();
  openInPx0();
}, true);
