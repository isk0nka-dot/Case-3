/*!
 * Argus Proctoring SDK v2.0.0
 * (c) Argus AI — https://argusai.kz
 *
 * Works with ALL loading methods:
 *   1. <script> tag           → window.ArgusProctoring
 *   2. dynamic import()       → default export
 *   3. require()              → the constructor directly
 */
(function (global, factory) {
  var ctor = factory();
  if (typeof define === 'function' && define.amd) {
    define([], function () { return ctor; });
  } else if (typeof module !== 'undefined' && module.exports) {
    module.exports = ctor;
    module.exports.default = ctor;
    module.exports.ArgusProctoring = ctor;
  } else {
    global.ArgusProctoring = ctor;
    global.ArgusSDK = { ArgusProctoring: ctor, VERSION: ctor.VERSION };
  }
}(typeof globalThis !== 'undefined' ? globalThis : typeof window !== 'undefined' ? window : this, function () {
  'use strict';

  var VERSION = '2.0.0';
  var LK_CDN_URL = 'https://cdn.jsdelivr.net/npm/livekit-client@2.17.1/dist/livekit-client.umd.min.js';

  // ─── Event type constants (mirrors backend valueobject) ───────────────────────
  var EVT = { TAB_SWITCH: 30, COPY_PASTE: 31, CONTEXT_MENU: 33, FULLSCREEN_EXIT: 34 };
  var SEV = { INFO: 1, WARNING: 2, CRITICAL: 3 };
  var SRC_BROWSER = 4;

  // ─── Utilities ────────────────────────────────────────────────────────────────
  function log() { console.log.apply(console, ['[Argus]'].concat(Array.prototype.slice.call(arguments))); }
  function logError() { console.error.apply(console, ['[Argus] ERROR:'].concat(Array.prototype.slice.call(arguments))); }

  function loadScript(src) {
    return new Promise(function (resolve, reject) {
      if (document.querySelector('script[src="' + src + '"]')) {
        var ex = document.querySelector('script[src="' + src + '"]');
        if (ex.dataset.loaded) { resolve(); return; }
        ex.addEventListener('load', resolve);
        ex.addEventListener('error', function () { reject(new Error('Script load failed: ' + src)); });
        return;
      }
      var s = document.createElement('script');
      s.src = src; s.crossOrigin = 'anonymous';
      s.onload = function () { s.dataset.loaded = '1'; resolve(); };
      s.onerror = function () { reject(new Error('Script load failed: ' + src)); };
      document.head.appendChild(s);
    });
  }

  function apiFetch(url, token, method, body) {
    return fetch(url, {
      method: method || 'POST',
      headers: { 'Authorization': 'Bearer ' + token, 'Content-Type': 'application/json' },
      body: body ? JSON.stringify(body) : undefined,
    });
  }

  // ─── Preflight UI ─────────────────────────────────────────────────────────────
  function preflightRow(id, label) {
    return '<div style="display:flex;align-items:center;justify-content:space-between;padding:10px 14px;background:#1a2440;border-radius:8px;">' +
      '<span style="font-size:14px;">' + label + '</span>' +
      '<span id="argus-pf-' + id + '" style="font-size:13px;color:#64748b;">—</span>' +
      '</div>';
  }

  function buildPreflightHTML() {
    return '<div id="argus-preflight" style="position:fixed;inset:0;background:rgba(10,15,25,0.97);z-index:2147483647;display:flex;align-items:center;justify-content:center;font-family:system-ui,sans-serif;color:#fff;">' +
      '<div style="width:420px;padding:32px;background:#131b2e;border-radius:16px;border:1px solid #1e2d4a;">' +
      '<h2 style="margin:0 0 24px;font-size:18px;font-weight:600;text-align:center;">Проверка перед началом теста</h2>' +
      '<div style="display:flex;flex-direction:column;gap:12px;margin-bottom:24px;">' +
        preflightRow('cam',        'Камера') +
        preflightRow('mic',        'Микрофон') +
        preflightRow('browser',    'Браузер') +
        preflightRow('fullscreen', 'Полный экран') +
        preflightRow('network',    'Сеть') +
      '</div>' +
      '<label style="display:flex;align-items:center;gap:10px;margin-bottom:20px;cursor:pointer;font-size:14px;">' +
        '<input type="checkbox" id="argus-consent" style="width:18px;height:18px;cursor:pointer;">' +
        '<span>Я согласен на видеонаблюдение во время экзамена</span>' +
      '</label>' +
      '<button id="argus-pf-btn" style="width:100%;padding:14px;background:#3b82f6;color:#fff;border:none;border-radius:8px;font-size:15px;font-weight:600;cursor:pointer;">Начать проверку</button>' +
      '<div id="argus-pf-error" style="margin-top:12px;color:#f87171;font-size:13px;text-align:center;display:none;"></div>' +
      '</div></div>';
  }

  function setPreflightStatus(id, text, ok) {
    var el = document.getElementById('argus-pf-' + id);
    if (!el) return;
    el.textContent = text;
    el.style.color = ok === true ? '#22c55e' : ok === false ? '#ef4444' : '#f59e0b';
  }

  // ─── Student Widget ───────────────────────────────────────────────────────────
  function buildWidget() {
    if (document.getElementById('argus-widget')) return;
    var style = document.createElement('style');
    style.textContent = '@keyframes argus-pulse{0%,100%{opacity:1}50%{opacity:.4}}';
    document.head.appendChild(style);
    var w = document.createElement('div');
    w.id = 'argus-widget';
    w.style.cssText = 'position:fixed;bottom:16px;right:16px;z-index:2147483646;background:rgba(10,15,25,0.92);border:1px solid #1e3a5f;border-radius:10px;padding:10px 14px;min-width:170px;backdrop-filter:blur(8px);font-family:system-ui,sans-serif;color:#fff;';
    w.innerHTML =
      '<div style="display:flex;align-items:center;gap:8px;margin-bottom:6px;">' +
        '<span style="width:8px;height:8px;border-radius:50%;background:#22c55e;animation:argus-pulse 2s infinite;flex-shrink:0;"></span>' +
        '<span style="font-size:11px;font-weight:600;color:#22c55e;">● REC</span>' +
        '<span id="argus-w-violations" style="margin-left:auto;font-size:11px;color:#94a3b8;">0 нарушений</span>' +
      '</div>' +
      '<div style="display:flex;gap:8px;align-items:center;margin-bottom:4px;">' +
        '<span id="argus-w-cam"  style="font-size:10px;color:#22c55e;">📷 Камера</span>' +
        '<span id="argus-w-mic"  style="font-size:10px;color:#22c55e;">🎙 Микрофон</span>' +
      '</div>' +
      '<div style="display:flex;gap:8px;align-items:center;">' +
        '<span id="argus-w-scr" style="font-size:10px;color:#f59e0b;">🖥 Экран</span>' +
        '<span id="argus-w-fs"  style="font-size:10px;color:#22c55e;">⛶ Fullscreen</span>' +
      '</div>' +
      '<div id="argus-w-warn" style="display:none;margin-top:8px;font-size:11px;color:#fbbf24;text-align:center;font-weight:600;"></div>';
    document.body.appendChild(w);
  }

  function widgetSet(id, ok, label) {
    var el = document.getElementById('argus-w-' + id);
    if (!el) return;
    el.style.color = ok ? '#22c55e' : '#ef4444';
    if (label) el.textContent = label;
  }

  function widgetWarn(msg) {
    var el = document.getElementById('argus-w-warn');
    if (!el) return;
    el.textContent = msg; el.style.display = msg ? 'block' : 'none';
  }

  function widgetViolations(n) {
    var el = document.getElementById('argus-w-violations');
    if (!el) return;
    el.textContent = n + (n === 1 ? ' нарушение' : n < 5 ? ' нарушения' : ' нарушений');
    el.style.color = n > 0 ? '#f87171' : '#94a3b8';
  }

  // ─── ArgusProctoring ──────────────────────────────────────────────────────────
  function ArgusProctoring(config) {
    this.config = Object.assign({ apiUrl: 'https://argusai.kz', locale: 'ru', skipPreflight: false }, config || {});
    if (!this.config.sessionId) throw new Error('[Argus] sessionId is required');
    if (!this.config.token)     throw new Error('[Argus] token is required');
    this._room = null;
    this._screenStream = null;
    this._heartbeatTimer = null;
    this._mounted = false;
    this._violations = 0;
    this._handlers = {};
    log('SDK v' + VERSION + ' initialized, session=' + this.config.sessionId);
  }

  ArgusProctoring.prototype.mount = function () {
    if (this._mounted) return Promise.resolve();
    this._mounted = true;
    return this.config.skipPreflight ? this._startSession() : this._runPreflight();
  };

  // ─── Preflight ────────────────────────────────────────────────────────────────
  ArgusProctoring.prototype._runPreflight = function () {
    var self = this;
    return new Promise(function (resolveMain) {
      document.body.insertAdjacentHTML('beforeend', buildPreflightHTML());
      var btn = document.getElementById('argus-pf-btn');
      var errEl = document.getElementById('argus-pf-error');
      var results = {};
      var checked = false;

      btn.addEventListener('click', function () {
        if (btn.dataset.phase === 'proceed') {
          if (!document.getElementById('argus-consent').checked) {
            errEl.textContent = 'Подтвердите согласие на видеонаблюдение';
            errEl.style.display = 'block'; return;
          }
          var overlay = document.getElementById('argus-preflight');
          if (overlay) overlay.remove();
          resolveMain();
          self._startSession().catch(function (e) {
            logError('session start failed:', e && e.message);
            if (typeof self.config.onError === 'function') self.config.onError(e);
          });
          return;
        }
        if (checked) return;
        checked = true;
        btn.disabled = true; btn.textContent = 'Проверяем...';
        errEl.style.display = 'none';

        self._runChecks(results).then(function () {
          var failed = Object.keys(results).filter(function (k) { return !results[k]; });
          btn.disabled = false;
          if (failed.length > 0) {
            errEl.textContent = 'Не пройдено: ' + failed.join(', ');
            errEl.style.display = 'block';
            btn.textContent = 'Повторить'; checked = false;
          } else {
            btn.textContent = 'Начать экзамен';
            btn.dataset.phase = 'proceed';
          }
        });
      });
    });
  };

  ArgusProctoring.prototype._runChecks = function (results) {
    var self = this;
    // Browser check
    var browserOk = !!(navigator.mediaDevices && navigator.mediaDevices.getUserMedia && document.documentElement.requestFullscreen);
    results.browser = browserOk;
    setPreflightStatus('browser', browserOk ? 'OK' : 'Не поддерживается', browserOk);
    // Fullscreen check
    results.fullscreen = !!document.documentElement.requestFullscreen;
    setPreflightStatus('fullscreen', results.fullscreen ? 'OK' : 'Недоступен', results.fullscreen);

    var networkCheck = fetch(self.config.apiUrl + '/healthz').then(function (r) {
      results.network = r.ok;
      setPreflightStatus('network', r.ok ? 'OK' : 'Ошибка ' + r.status, r.ok);
    }).catch(function () {
      results.network = false;
      setPreflightStatus('network', 'Недоступен', false);
    });

    setPreflightStatus('cam', '...', null);
    setPreflightStatus('mic', '...', null);
    var mediaCheck = navigator.mediaDevices.getUserMedia({ video: true, audio: true })
      .then(function (stream) {
        stream.getTracks().forEach(function (t) { t.stop(); });
        results.cam = results.mic = true;
        setPreflightStatus('cam', 'OK', true);
        setPreflightStatus('mic', 'OK', true);
      })
      .catch(function (err) {
        results.cam = results.mic = false;
        var msg = err.name === 'NotAllowedError' ? 'Нет разрешения' : 'Ошибка';
        setPreflightStatus('cam', msg, false);
        setPreflightStatus('mic', msg, false);
      });

    return Promise.all([networkCheck, mediaCheck]);
  };

  // ─── Session start ────────────────────────────────────────────────────────────
  ArgusProctoring.prototype._startSession = function () {
    var self = this;
    return self._loadLiveKit()
      .then(function () { return self._requestFullscreen(); })
      .then(function () { return self._requestScreenCapture(); })
      .then(function () {
        self._startHeartbeat();
        return self._getStudentToken();
      })
      .then(function (td) { return self._connectAndPublish(td); })
      .then(function () { return self._signalRecordingReady(); })
      .then(function () {
        buildWidget();
        self._attachSecureListeners();
        log('Proctoring active v' + VERSION);
        if (typeof self.config.onVerified === 'function') self.config.onVerified({ networkMode: 'webrtc' });
      })
      .catch(function (err) {
        logError('mount failed:', err && err.message ? err.message : err);
        // Show user-friendly overlay for known errors
        if (err && err.code === 'SESSION_EXPIRED') {
          self._showError(err.userMessage || 'Сессия истекла. Обновите страницу.');
        }
        if (typeof self.config.onError === 'function') {
          self.config.onError({ message: err && err.message, code: err && err.code, userMessage: err && err.userMessage });
        }
      });
  };

  // ─── Fullscreen ───────────────────────────────────────────────────────────────
  ArgusProctoring.prototype._requestFullscreen = function () {
    if (!document.documentElement.requestFullscreen) return Promise.resolve();
    if (document.fullscreenElement) return Promise.resolve();
    return document.documentElement.requestFullscreen().catch(function () {});
  };

  // ─── Screen capture ───────────────────────────────────────────────────────────
  ArgusProctoring.prototype._requestScreenCapture = function () {
    var self = this;
    if (!navigator.mediaDevices || !navigator.mediaDevices.getDisplayMedia) {
      log('getDisplayMedia not available — screen capture skipped');
      return Promise.resolve();
    }
    return navigator.mediaDevices.getDisplayMedia({ video: { frameRate: 5 }, audio: false })
      .then(function (stream) {
        var track = stream.getVideoTracks()[0];
        self._screenStream = stream;
        var settings = track.getSettings ? track.getSettings() : {};
        log('Screen capture started, displaySurface=' + (settings.displaySurface || 'unknown'));
        track.addEventListener('ended', function () {
          self._screenStream = null;
          widgetSet('scr', false, '🖥 Экран');
          self._sendViolation(EVT.TAB_SWITCH, SEV.CRITICAL, 'screen_share_stopped', 'Захват экрана остановлен', 0.95);
          widgetWarn('Захват экрана остановлен!');
        });
        widgetSet('scr', true, '🖥 Экран');
      })
      .catch(function (err) {
        log('Screen capture declined:', err.message);
        widgetSet('scr', false, '🖥 Экран');
      });
  };

  // ─── LiveKit ──────────────────────────────────────────────────────────────────
  ArgusProctoring.prototype._loadLiveKit = function () {
    if (window.LivekitClient && window.LivekitClient.Room) return Promise.resolve();
    log('Loading LiveKit...');
    return loadScript(LK_CDN_URL);
  };

  ArgusProctoring.prototype._getStudentToken = function () {
    var cfg = this.config;
    log('Requesting student token...');
    return apiFetch(cfg.apiUrl + '/api/v1/external/sessions/' + cfg.sessionId + '/student-token', cfg.token)
      .then(function (r) {
        if (r.status === 401) {
          var err = new Error('SESSION_EXPIRED');
          err.code = 'SESSION_EXPIRED';
          err.userMessage = 'Сессия истекла. Обновите страницу или обратитесь к преподавателю.';
          throw err;
        }
        if (!r.ok) return r.text().then(function (b) { throw new Error('student-token HTTP ' + r.status + ': ' + b); });
        return r.json();
      });
  };

  ArgusProctoring.prototype._connectAndPublish = function (tokenData) {
    var LK = window.LivekitClient;
    var self = this;
    if (!LK || !LK.Room) return Promise.reject(new Error('LivekitClient not available'));

    var room = new LK.Room({
      adaptiveStream: true, dynacast: true,
      videoCaptureDefaults: { resolution: { width: 640, height: 480, frameRate: 15 } },
      audioCaptureDefaults: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
    });
    this._room = room;

    room.on(LK.RoomEvent.ConnectionStateChanged, function (state) { log('LiveKit state:', state); });
    room.on(LK.RoomEvent.Disconnected, function () {
      self._stopHeartbeat();
      widgetSet('cam', false); widgetSet('mic', false);
    });

    var wsUrl = tokenData.livekitUrl || tokenData.livekit_url || 'wss://argusai.kz/livekit';
    log('Connecting to LiveKit, room=' + tokenData.room);

    return room.connect(wsUrl, tokenData.livekitToken || tokenData.livekit_token)
      .then(function () {
        return room.localParticipant.enableCameraAndMicrophone();
      })
      .then(function () {
        widgetSet('cam', true); widgetSet('mic', true);
        log('Camera and microphone enabled');

        // Attach camera preview to containerId
        var containerId = self.config.containerId;
        if (containerId) {
          var container = document.getElementById(containerId);
          if (container) {
            room.localParticipant.videoTrackPublications.forEach(function (pub) {
              if (pub.track && typeof pub.track.attach === 'function') {
                var el = pub.track.attach();
                el.autoplay = true; el.muted = true; el.playsInline = true;
                el.style.cssText = 'width:100%;height:100%;object-fit:cover;border-radius:inherit;transform:scaleX(-1);';
                container.innerHTML = ''; container.appendChild(el);
                log('Camera attached to #' + containerId);
              }
            });
          }
        }

        // Publish screen track
        if (self._screenStream) {
          var screenTrack = self._screenStream.getVideoTracks()[0];
          if (screenTrack && LK.LocalVideoTrack) {
            var lkScreenTrack = new LK.LocalVideoTrack(screenTrack, undefined, false);
            return room.localParticipant.publishTrack(lkScreenTrack, {
              source: LK.Track.Source.ScreenShare,
              simulcast: false,
            }).then(function () {
              widgetSet('scr', true, '🖥 Экран');
              log('Screen track published');
            }).catch(function (e) { log('Screen publish failed:', e.message); });
          }
        }
      });
  };

  ArgusProctoring.prototype._signalRecordingReady = function () {
    var cfg = this.config;
    return apiFetch(cfg.apiUrl + '/api/v1/external/sessions/' + cfg.sessionId + '/recording-ready', cfg.token)
      .then(function (r) { log('recording-ready:', r.status); })
      .catch(function (e) { logError('recording-ready failed:', e.message); });
  };

  // ─── Secure mode listeners ────────────────────────────────────────────────────
  ArgusProctoring.prototype._attachSecureListeners = function () {
    var self = this, doc = document, win = window;

    self._handlers.fullscreen = function () {
      var inFS = !!doc.fullscreenElement;
      widgetSet('fs', inFS, '⛶ Fullscreen');
      if (!inFS) {
        widgetWarn('Вернитесь в полный экран!');
        self._sendViolation(EVT.FULLSCREEN_EXIT, SEV.CRITICAL, 'fullscreen_exit', 'Выход из полного экрана', 1.0);
        setTimeout(function () {
          if (!doc.fullscreenElement && doc.documentElement.requestFullscreen) {
            doc.documentElement.requestFullscreen().catch(function () {});
          }
        }, 800);
      } else {
        widgetWarn('');
      }
    };
    doc.addEventListener('fullscreenchange', self._handlers.fullscreen);

    self._handlers.visibility = function () {
      if (doc.hidden) {
        self._sendViolation(EVT.TAB_SWITCH, SEV.CRITICAL, 'tab_switch', 'Переключение вкладки', 1.0);
        widgetWarn('Не переключайте вкладки!');
      } else {
        widgetWarn('');
      }
    };
    doc.addEventListener('visibilitychange', self._handlers.visibility);

    self._handlers.blur = function () {
      self._sendViolation(EVT.TAB_SWITCH, SEV.WARNING, 'window_blur', 'Потеря фокуса окна', 0.8);
    };
    win.addEventListener('blur', self._handlers.blur);

    self._handlers.beforeunload = function (e) {
      self._sendViolation(EVT.TAB_SWITCH, SEV.CRITICAL, 'page_unload', 'Попытка закрыть страницу', 1.0);
      e.preventDefault(); e.returnValue = '';
    };
    win.addEventListener('beforeunload', self._handlers.beforeunload);

    self._handlers.copy = function () {
      self._sendViolation(EVT.COPY_PASTE, SEV.WARNING, 'copy_attempt', 'Копирование текста', 0.9);
    };
    self._handlers.paste = function () {
      self._sendViolation(EVT.COPY_PASTE, SEV.WARNING, 'paste_attempt', 'Вставка текста', 0.9);
    };
    doc.addEventListener('copy', self._handlers.copy);
    doc.addEventListener('paste', self._handlers.paste);

    self._handlers.contextmenu = function (e) {
      e.preventDefault();
      self._sendViolation(EVT.CONTEXT_MENU, SEV.WARNING, 'right_click', 'Правая кнопка мыши', 0.7);
    };
    doc.addEventListener('contextmenu', self._handlers.contextmenu);

    self._handlers.keydown = function (e) {
      var label = '';
      if (e.key === 'F12') label = 'F12';
      else if (e.ctrlKey && e.shiftKey && 'IJC'.indexOf(e.key) !== -1) label = 'DevTools (' + e.key + ')';
      else if (e.ctrlKey && e.key === 'u') label = 'Ctrl+U';
      else if (e.key === 'PrintScreen') label = 'PrintScreen';
      else if (e.altKey && e.key === 'Tab') label = 'Alt+Tab';
      if (label) {
        e.preventDefault();
        self._sendViolation(EVT.CONTEXT_MENU, SEV.WARNING, 'forbidden_key', label, 0.85);
      }
    };
    doc.addEventListener('keydown', self._handlers.keydown);

    log('Secure listeners attached');
  };

  // ─── Violation reporting ──────────────────────────────────────────────────────
  ArgusProctoring.prototype._sendViolation = function (eventType, severity, source, label, confidence) {
    this._violations++;
    widgetViolations(this._violations);
    apiFetch(this.config.apiUrl + '/api/v1/external/events', this.config.token, 'POST', {
      events: [{ event_type: eventType, severity: severity, source: SRC_BROWSER, confidence: confidence || 0.9, label: label }],
    }).catch(function () {});
    log('Violation:', label, '(type=' + eventType + ', sev=' + severity + ')');
  };

  // ─── Heartbeat ────────────────────────────────────────────────────────────────
  ArgusProctoring.prototype._startHeartbeat = function () {
    var self = this, url = self.config.apiUrl + '/api/v1/external/heartbeat';
    function beat() { apiFetch(url, self.config.token).catch(function () {}); }
    beat();
    self._heartbeatTimer = setInterval(beat, 30000);
    log('Heartbeat started');
  };

  ArgusProctoring.prototype._stopHeartbeat = function () {
    if (this._heartbeatTimer) { clearInterval(this._heartbeatTimer); this._heartbeatTimer = null; }
  };

  ArgusProctoring.prototype._showError = function (msg) {
    if (document.getElementById('argus-error-overlay')) return;
    var el = document.createElement('div');
    el.id = 'argus-error-overlay';
    el.style.cssText = 'position:fixed;inset:0;background:rgba(10,15,25,0.97);z-index:2147483647;display:flex;align-items:center;justify-content:center;font-family:system-ui,sans-serif;';
    el.innerHTML = '<div style="text-align:center;padding:32px;max-width:400px;">' +
      '<div style="font-size:48px;margin-bottom:16px;">⚠️</div>' +
      '<h2 style="color:#fff;margin:0 0 12px;font-size:20px;">Ошибка прокторинга</h2>' +
      '<p style="color:#94a3b8;font-size:15px;margin:0 0 24px;">' + msg + '</p>' +
      '<button onclick="location.reload()" style="padding:12px 24px;background:#3b82f6;color:#fff;border:none;border-radius:8px;font-size:15px;cursor:pointer;">Обновить страницу</button>' +
      '</div>';
    document.body.appendChild(el);
  };

  // ─── Destroy ──────────────────────────────────────────────────────────────────
  ArgusProctoring.prototype.destroy = function () {
    this._stopHeartbeat();
    var h = this._handlers, doc = document, win = window;
    if (h.fullscreen)   doc.removeEventListener('fullscreenchange', h.fullscreen);
    if (h.visibility)   doc.removeEventListener('visibilitychange', h.visibility);
    if (h.blur)         win.removeEventListener('blur', h.blur);
    if (h.beforeunload) win.removeEventListener('beforeunload', h.beforeunload);
    if (h.copy)         doc.removeEventListener('copy', h.copy);
    if (h.paste)        doc.removeEventListener('paste', h.paste);
    if (h.contextmenu)  doc.removeEventListener('contextmenu', h.contextmenu);
    if (h.keydown)      doc.removeEventListener('keydown', h.keydown);
    if (this._screenStream) this._screenStream.getTracks().forEach(function (t) { t.stop(); });
    if (this._room) { this._room.disconnect(); this._room = null; }
    var w = document.getElementById('argus-widget');
    if (w) w.remove();
    log('SDK destroyed');
  };

  ArgusProctoring.VERSION = VERSION;

  // ─── Auto-init via data-attributes ───────────────────────────────────────────
  function tryAutoInit() {
    var scripts = document.querySelectorAll('script[src]');
    var s = null;
    for (var i = scripts.length - 1; i >= 0; i--) {
      if (scripts[i].src && scripts[i].src.indexOf('argus-sdk') !== -1) { s = scripts[i]; break; }
    }
    if (!s) return;
    var sessionId = s.getAttribute('data-session-id');
    var token     = s.getAttribute('data-token');
    var apiUrl    = s.getAttribute('data-api-url') || 'https://argusai.kz';
    if (!sessionId || !token) return;
    log('Auto-init, session=' + sessionId);
    var sdk = new ArgusProctoring({ sessionId: sessionId, token: token, apiUrl: apiUrl });
    if (typeof window !== 'undefined') window._argusSDKInstance = sdk;
    sdk.mount();
  }

  if (typeof document !== 'undefined') {
    if (document.readyState === 'loading') document.addEventListener('DOMContentLoaded', tryAutoInit);
    else setTimeout(tryAutoInit, 0);
  }

  return ArgusProctoring;
}));
