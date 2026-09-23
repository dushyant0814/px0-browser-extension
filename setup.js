const RELEASES = "https://github.com/dushyant0814/px0-browser-extension/releases/latest";
const INSTALLER = `${RELEASES}/download/install.sh`;

const status = document.getElementById("status");
const statusText = document.getElementById("status-text");
const command = `curl -fsSL ${INSTALLER} | sh -s -- ${chrome.runtime.id}`;

document.getElementById("command").textContent = command;
document.getElementById("releases").href = RELEASES;
document.getElementById("script").href = INSTALLER;
document.getElementById("copy").addEventListener("click", async (event) => {
  await navigator.clipboard.writeText(command);
  event.target.textContent = "Copied";
  setTimeout(() => { event.target.textContent = "Copy"; }, 1500);
});

function show(state, text) {
  status.dataset.state = state;
  statusText.textContent = text;
}

async function check() {
  let response;
  try {
    response = await chrome.runtime.sendMessage({ action: "ping" });
  } catch (error) {
    response = { ok: false, error: error.message };
  }
  if (response?.ok) {
    show("ok", `Connected: helper ${response.hostVersion}, ${response.px0Version}`);
    document.getElementById("install").hidden = true;
    document.getElementById("done").hidden = false;
    return;
  }
  if (response?.hostVersion) {
    // The helper answered, so registration works; px0 itself is the problem.
    show("error", `Helper installed, but px0 cannot start: ${response.error}`);
  } else {
    show("checking", "Waiting for the px0 helper…");
  }
  setTimeout(check, 2000);
}

check();
