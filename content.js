const REPOSITORY = repositoryURL(location.href);
let opening = false;

function repositoryURL(raw) {
  const url = new URL(raw);
  const parts = url.pathname.split("/").filter(Boolean);
  if (url.hostname === "github.com") {
    const reserved = new Set(["collections", "codespaces", "enterprise", "events", "explore", "features", "issues", "login", "marketplace", "new", "notifications", "orgs", "pricing", "pulls", "search", "settings", "signup", "sponsors", "topics", "trending"]);
    if (parts.length < 2 || reserved.has(parts[0])) return null;
  } else if (url.hostname === "gitlab.com") {
    const reserved = new Set(["admin", "dashboard", "explore", "groups", "help", "projects", "search", "users"]);
    const marker = parts.indexOf("-");
    const project = marker < 0 ? parts : parts.slice(0, marker);
    if (project.length < 2 || reserved.has(project[0])) return null;
  } else {
    return null;
  }
  return url.href;
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
  if (!REPOSITORY || opening) return;
  opening = true;
  notice("Opening in px0…");
  let response;
  try {
    response = await chrome.runtime.sendMessage({ action: "open", url: location.href });
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

if (REPOSITORY) {
  setTimeout(() => {
    chrome.runtime.sendMessage({ action: "warm", url: location.href }).catch(() => {});
  }, 500);

  addEventListener("keydown", (event) => {
    if (event.key !== "." || event.repeat || event.ctrlKey || event.metaKey || event.altKey || editable(event.target)) return;
    event.preventDefault();
    event.stopImmediatePropagation();
    openInPx0();
  }, true);
}
