# Argus Proctoring — Chrome Extension (MV3)

Environment-isolation and presence-verification companion to the Argus browser SDK.
Mirrors the role of AeroExam's "Aero proctoring" extension, but integrates with the
existing Argus SDK `postMessage` protocol instead of shipping its own transport.

> Status: **v0.1 scaffold.** Core wiring (session handshake, tab/window/monitor/navigation
> monitoring, presence heartbeat) is implemented. See "Roadmap" for what's next.

## What it does (v0.1)

| Signal | Source API | Emitted event | Severity |
|---|---|---|---|
| Session start/stop | page SDK `postMessage({argus:…})` | `EXTENSION_PRESENT` | INFO |
| New tab opened during exam | `chrome.tabs.onCreated` | `TAB_OPENED` | WARNING |
| Switch away from exam tab | `chrome.tabs.onActivated` | `TAB_SWITCH` | WARNING |
| Browser window loses focus | `chrome.windows.onFocusChanged` | `WINDOW_BLUR` | WARNING |
| Navigate off exam origin | `chrome.webNavigation.onBeforeNavigate` | `NAVIGATION_AWAY` | CRITICAL |
| Second monitor attached | `chrome.system.display` | `EXTERNAL_DISPLAY_DETECTED` | CRITICAL |
| Extension alive beacon | timer | `EXTENSION_HEARTBEAT` | INFO |

Events are relayed to the page via `window.postMessage({ argusExt: { type, payload, severity, ts } })`.

## Message protocol

```
Page  -> Ext : window.postMessage({ argus:    { type:'SESSION_START', payload:{ sessionToken, serverUrl } } })
Page  -> Ext : window.postMessage({ argus:    { type:'SESSION_END' } })
Ext   -> Page: window.postMessage({ argusExt: { type, payload, severity, ts } })
```

The SDK already sends `SESSION_START` / `SESSION_END` (see `argus-sdk/src/index.ts`
`_notifyExtensionStart` / `_notifyExtensionStop`).

## Integration hook needed in the SDK (small, follow-up)

The SDK must listen for `argusExt` messages and ingest them as SYSTEM-source events,
and use `EXTENSION_PRESENT` / `EXTENSION_HEARTBEAT` for the strict-mode preflight gate:

```ts
window.addEventListener('message', (e) => {
  if (e.source !== window) return;
  const m = (e.data as any)?.argusExt;
  if (!m) return;
  if (m.type === 'EXTENSION_PRESENT')  this._extensionPresent = true;   // preflight: pass
  if (m.type === 'EXTENSION_HEARTBEAT') this._extLastBeat = Date.now(); // watchdog
  // Map extension event types -> EventType and ingest:
  //   TAB_SWITCH -> EventType.TAB_SWITCH, EXTERNAL_DISPLAY_DETECTED -> 35, etc.
  this._session?.sendLifecycleEvent(mapExtType(m.type), mapSeverity(m.severity),
                                    EventSource.SYSTEM, m.type, m.payload?.confidence ?? 1, m.payload);
});
```

In `strictness: 'high'` exams, preflight must FAIL if `EXTENSION_PRESENT` was never
received, and a missing heartbeat for >10s during the exam must raise a tamper event.

## Load for local testing

1. `chrome://extensions` → enable **Developer mode**.
2. **Load unpacked** → select this `extension/` folder.
3. Open an exam page on `https://argusai.kz` and start a session; the popup shows "Active".

## Roadmap (see ТЗ §6)

- [ ] Fixed `key` in manifest → stable `extension_id`; backend `/api/v1/extension/verify` handshake with signed nonce.
- [ ] Hard-enforce fullscreen + `desktopCapture` "entire screen" share (AERO steps 23–24).
- [ ] Block (not just log) navigation away via `declarativeNetRequest`.
- [ ] Hard-block on second monitor / extension removal in strict mode.
- [ ] i18n (kk/ru/en) for popup + `_locales/`.
- [ ] Chrome Web Store listing + enterprise (force-install) policy for universities.
- [ ] Playwright e2e launching Chromium with `--load-extension`.
