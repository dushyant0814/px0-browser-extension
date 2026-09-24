const HOST = "ai.px0.launcher";
const ALLOWED_HOSTS = new Set(["github.com", "gitlab.com"]);
const SETUP_URL = chrome.runtime.getURL("setup.html");

let port;
let nextID = 1;
const pending = new Map();

function safeRepositoryURL(raw) {
  let url;
  try {
    url = new URL(raw);
  } catch {
    return null;
  }
  if (url.protocol !== "https:" || !ALLOWED_HOSTS.has(url.hostname)) return null;
  if (url.username || url.password) return null;
  return url.href;
}

// Chrome reports an unregistered host, or one whose manifest does not list
// this extension, with these messages.
function hostMissing(error) {
  return /native messaging host not found|access to the specified native messaging host is forbidden/i.test(error?.message || "");
}

function nativePort() {
  if (port) return port;
  port = chrome.runtime.connectNative(HOST);
  port.onMessage.addListener((message) => {
    const waiter = pending.get(message.id);
    if (!waiter) return;
    pending.delete(message.id);
    waiter.resolve(message);
  });
  port.onDisconnect.addListener(() => {
    const error = chrome.runtime.lastError?.message || "px0 native host disconnected";
    port = undefined;
    for (const waiter of pending.values()) waiter.reject(new Error(error));
    pending.clear();
  });
  return port;
}

function postNative(message) {
  const id = nextID++;
  return new Promise((resolve, reject) => {
    pending.set(id, { resolve, reject });
    nativePort().postMessage({ version: 1, id, ...message });
  });
}

function sendNative(action, rawURL) {
  const url = safeRepositoryURL(rawURL);
  if (!url) return Promise.reject(new Error("Unsupported repository URL"));
  return postNative({ action, url });
}

async function openSetup() {
  const [existing] = await chrome.runtime.getContexts({ contextTypes: ["TAB"], documentUrls: [SETUP_URL] });
  if (existing) {
    await chrome.tabs.update(existing.tabId, { active: true });
    await chrome.windows.update(existing.windowId, { focused: true });
    return;
  }
  await chrome.tabs.create({ url: SETUP_URL });
}

// A user who presses "." without the helper is sent to the setup page rather
// than left with a bare connection error.
async function openRepository(url) {
  try {
    return await sendNative("open", url);
  } catch (error) {
    if (hostMissing(error)) {
      openSetup();
      return { ok: false, error: "px0 helper not installed. Opened the setup page." };
    }
    return { ok: false, error: error.message };
  }
}

chrome.runtime.onInstalled.addListener(({ reason }) => {
  if (reason === chrome.runtime.OnInstalledReason.INSTALL) openSetup();
});

chrome.runtime.onMessage.addListener((message, sender, sendResponse) => {
  if (sender.id !== chrome.runtime.id) return false;
  let reply;
  if (message?.action === "ping") {
    reply = postNative({ action: "ping" });
  } else if (message?.action === "open") {
    reply = openRepository(message.url);
  } else if (["warm", "prefetch", "status"].includes(message?.action)) {
    reply = sendNative(message.action, message.url);
  } else {
    return false;
  }
  reply.then(sendResponse).catch((error) => sendResponse({ ok: false, error: error.message }));
  return true;
});

chrome.action.onClicked.addListener(async (tab) => {
  const url = safeRepositoryURL(tab.url);
  if (!url || tab.id == null) {
    openSetup();
    return;
  }
  const response = await openRepository(url);
  if (response.ok && response.viewerUrl) chrome.tabs.update(tab.id, { url: response.viewerUrl });
});
