// =============================================================================
// Argus Proctoring — Content Script
// =============================================================================
// Bridges the page-level Argus SDK and the extension background worker.
//
// Protocol with the page SDK (see argus-sdk/src/index.ts):
//   Page  -> Ext : window.postMessage({ argus:   { type:'SESSION_START', payload:{ sessionToken, serverUrl } } })
//   Page  -> Ext : window.postMessage({ argus:   { type:'SESSION_END' } })
//   Ext   -> Page: window.postMessage({ argusExt:{ type, payload, severity, ts } })
//
// The page SDK should listen for `argusExt` messages and ingest them as
// SYSTEM-source events (small SDK hook — see extension/README.md §Integration).
// =============================================================================

const PAGE_ORIGIN = window.location.origin;
let examActive = false;
let heartbeatTimer = null;

// ---- Page -> Extension --------------------------------------------------
window.addEventListener('message', (event) => {
  // Only trust messages from this page's own window.
  if (event.source !== window || !event.data || typeof event.data !== 'object') return;
  const msg = event.data.argus;
  if (!msg || typeof msg.type !== 'string') return;

  if (msg.type === 'SESSION_START') {
    startExam(msg.payload || {});
  } else if (msg.type === 'SESSION_END') {
    stopExam();
  } else if (msg.type === 'PING') {
    announcePresence();
  }
});

// Reply to the SDK's pre-session probe so a strict `requireExtension` gate can
// confirm the extension is installed before the exam starts.
function announcePresence() {
  chrome.runtime.sendMessage({ cmd: 'PING' }, (resp) => {
    if (chrome.runtime.lastError || !resp) return;
    toPage('EXTENSION_PRESENT', { extensionId: resp.extensionId, version: resp.version }, 'INFO');
  });
}

function startExam(payload) {
  examActive = true;
  chrome.runtime.sendMessage(
    { cmd: 'EXAM_START', origin: PAGE_ORIGIN, payload },
    (resp) => {
      if (chrome.runtime.lastError || !resp) return;
      // Announce presence so the SDK/preflight can confirm the extension is live.
      toPage('EXTENSION_PRESENT', {
        extensionId: resp.extensionId,
        version: resp.version,
      }, 'INFO');
    },
  );
  startHeartbeat();
}

function stopExam() {
  examActive = false;
  stopHeartbeat();
  chrome.runtime.sendMessage({ cmd: 'EXAM_STOP' }, () => void chrome.runtime.lastError);
}

// ---- Extension -> Page --------------------------------------------------
chrome.runtime.onMessage.addListener((msg) => {
  if (!msg || !msg.evt) return;
  toPage(msg.evt, msg.payload || {}, msg.severity || 'WARNING');
});

function toPage(type, payload, severity) {
  window.postMessage(
    { argusExt: { type, payload, severity, ts: Date.now() } },
    PAGE_ORIGIN,
  );
}

// ---- Presence heartbeat -------------------------------------------------
// Lets the page SDK detect if the extension is disabled/removed mid-exam:
// the page arms a watchdog and treats a missing beat as a tamper signal.
function startHeartbeat() {
  stopHeartbeat();
  heartbeatTimer = setInterval(() => {
    if (!examActive) return;
    toPage('EXTENSION_HEARTBEAT', { version: chrome.runtime.getManifest().version }, 'INFO');
  }, 3000);
}

function stopHeartbeat() {
  if (heartbeatTimer) { clearInterval(heartbeatTimer); heartbeatTimer = null; }
}
