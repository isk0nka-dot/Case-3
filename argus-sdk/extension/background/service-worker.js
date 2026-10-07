// =============================================================================
// Argus Proctoring — Background Service Worker (MV3)
// =============================================================================
// Responsibilities:
//   - Track the active exam tab/window (signalled by the content script, which
//     in turn learns it from the page SDK's `SESSION_START` postMessage).
//   - Monitor environment integrity: new tabs, tab switches, window blur,
//     navigation away from the exam origin, and multi-monitor changes.
//   - Relay integrity events back to the exam tab's content script, which
//     forwards them to the page SDK for ingestion into the Argus backend.
//
// This worker never talks to the network directly in v0.1 — the page SDK owns
// the authenticated gRPC-Web transport. The extension only observes & relays.
// =============================================================================

const EXT_VERSION = chrome.runtime.getManifest().version;

/** @type {{ active: boolean, tabId: number|null, windowId: number|null, origin: string|null }} */
let exam = { active: false, tabId: null, windowId: null, origin: null };

// ---------------------------------------------------------------------------
// Content-script <-> background messaging
// ---------------------------------------------------------------------------
chrome.runtime.onMessage.addListener((msg, sender, sendResponse) => {
  if (!msg || typeof msg !== 'object') return;

  switch (msg.cmd) {
    case 'EXAM_START': {
      exam = {
        active: true,
        tabId: sender.tab ? sender.tab.id : null,
        windowId: sender.tab ? sender.tab.windowId : null,
        origin: msg.origin || (sender.tab ? new URL(sender.tab.url || '').origin : null),
      };
      // Immediate baseline monitor check (e.g. a second monitor already attached).
      checkDisplays('exam_start');
      sendResponse({ ok: true, version: EXT_VERSION, extensionId: chrome.runtime.id });
      return true;
    }
    case 'EXAM_STOP': {
      exam = { active: false, tabId: null, windowId: null, origin: null };
      sendResponse({ ok: true });
      return true;
    }
    case 'PING': {
      sendResponse({ ok: true, version: EXT_VERSION, extensionId: chrome.runtime.id, examActive: exam.active });
      return true;
    }
  }
});

// ---------------------------------------------------------------------------
// Relay a detected integrity event to the exam tab's content script.
// ---------------------------------------------------------------------------
function emit(type, payload, severity = 'WARNING') {
  if (!exam.active || exam.tabId == null) return;
  chrome.tabs.sendMessage(exam.tabId, {
    evt: type,
    severity,
    payload: payload || {},
    ts: Date.now(),
  }).catch(() => { /* tab may be closing — non-fatal */ });
}

// ---------------------------------------------------------------------------
// Tab / window monitoring
// ---------------------------------------------------------------------------
chrome.tabs.onCreated.addListener((tab) => {
  if (!exam.active) return;
  emit('TAB_OPENED', { newTabId: tab.id, url: tab.pendingUrl || tab.url || '' }, 'WARNING');
});

chrome.tabs.onActivated.addListener((info) => {
  if (!exam.active) return;
  if (info.tabId !== exam.tabId) {
    emit('TAB_SWITCH', { toTabId: info.tabId }, 'WARNING');
  }
});

chrome.windows.onFocusChanged.addListener((windowId) => {
  if (!exam.active) return;
  // chrome.windows.WINDOW_ID_NONE === -1 → focus left the browser entirely.
  if (windowId === chrome.windows.WINDOW_ID_NONE || windowId !== exam.windowId) {
    emit('WINDOW_BLUR', { focusedWindowId: windowId }, 'WARNING');
  }
});

// Navigation away from the exam origin in the exam tab.
chrome.webNavigation.onBeforeNavigate.addListener((details) => {
  if (!exam.active || details.tabId !== exam.tabId || details.frameId !== 0) return;
  try {
    const target = new URL(details.url).origin;
    if (exam.origin && target !== exam.origin) {
      emit('NAVIGATION_AWAY', { to: target }, 'CRITICAL');
    }
  } catch { /* ignore malformed URLs */ }
});

// ---------------------------------------------------------------------------
// Multi-monitor detection
// ---------------------------------------------------------------------------
async function checkDisplays(reason) {
  if (!exam.active) return;
  try {
    const displays = await chrome.system.display.getInfo();
    if (Array.isArray(displays) && displays.length > 1) {
      emit('EXTERNAL_DISPLAY_DETECTED', { count: displays.length, reason }, 'CRITICAL');
    }
  } catch { /* system.display may be unavailable — non-fatal */ }
}

chrome.system.display.onDisplayChanged.addListener(() => checkDisplays('display_changed'));

// ---------------------------------------------------------------------------
// Tamper signal: if the worker is torn down mid-exam, the content script's
// heartbeat will notice the extension went silent (handled on the page side).
// ---------------------------------------------------------------------------
