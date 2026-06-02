/*!
 * Argus Proctoring SDK v1.2.0
 * (c) Argus AI — https://argusai.kz
 *
 * Works with ALL loading methods:
 *   1. <script> tag           → window.ArgusProctoring (constructor)
 *   2. dynamic import()       → default export is the constructor
 *   3. require()              → the constructor directly
 *   4. data-attributes        → auto-init, no JS needed
 *
 * Usage option A (data-attributes — recommended for LMS):
 *   <script
 *     src="https://argusai.kz/sdk/v1/argus-sdk.umd.js"
 *     data-session-id="argus_ses_..."
 *     data-token="<argusSessionToken>"
 *     data-api-url="https://argusai.kz"
 *   ></script>
 *
 * Usage option B (import/require):
 *   const ArgusProctoring = (await import('https://argusai.kz/sdk/v1/argus-sdk.umd.js')).default
 *   new ArgusProctoring({ sessionId, token, apiUrl }).mount()
 *
 * Usage option C (global):
 *   new window.ArgusProctoring({ sessionId, token, apiUrl }).mount()
 */
(function (global, factory) {
  var ctor = factory();
  if (typeof define === 'function' && define.amd) {
    // AMD — return the constructor
    define([], function() { return ctor; });
  } else if (typeof module !== 'undefined' && module.exports) {
    // CommonJS / dynamic import() — export the constructor as default
    module.exports = ctor;
    module.exports.default = ctor;
    module.exports.ArgusProctoring = ctor;
  } else {
    // Browser global — set both names
    global.ArgusProctoring = ctor;
    global.ArgusSDK = { ArgusProctoring: ctor, VERSION: ctor.VERSION };
  }
}(typeof globalThis !== 'undefined' ? globalThis : typeof window !== 'undefined' ? window : this, function () {
  'use strict';

  var VERSION = '1.2.0';
  var LK_CDN_URL = 'https://cdn.jsdelivr.net/npm/livekit-client@2.17.1/dist/livekit-client.umd.min.js';

  function log(msg) {
    var args = Array.prototype.slice.call(arguments, 1);
    console.log.apply(console, ['[Argus] ' + msg].concat(args));
  }

  function logError(msg) {
    var args = Array.prototype.slice.call(arguments, 1);
    console.error.apply(console, ['[Argus] ERROR: ' + msg].concat(args));
  }

  function loadScript(src) {
    return new Promise(function (resolve, reject) {
      if (document.querySelector('script[src="' + src + '"]')) {
        // already loading — wait for it
        var existing = document.querySelector('script[src="' + src + '"]');
        if (existing.dataset.loaded) { resolve(); return; }
        existing.addEventListener('load', function () { resolve(); });
        existing.addEventListener('error', function () { reject(new Error('Script load failed: ' + src)); });
        return;
      }
      var s = document.createElement('script');
      s.src = src;
      s.crossOrigin = 'anonymous';
      s.onload = function () { s.dataset.loaded = '1'; resolve(); };
      s.onerror = function () { reject(new Error('Script load failed: ' + src)); };
      document.head.appendChild(s);
    });
  }

  function apiFetch(url, token, method, body) {
    var opts = {
      method: method || 'POST',
      headers: {
        'Authorization': 'Bearer ' + token,
        'Content-Type': 'application/json'
      }
    };
    if (body) { opts.body = JSON.stringify(body); }
    return fetch(url, opts);
  }

  // ──────────────────────────────────────────────────────────────────
  // ArgusProctoring
  // ──────────────────────────────────────────────────────────────────

  function ArgusProctoring(config) {
    this.config = Object.assign({
      apiUrl: 'https://argusai.kz',
      locale: 'ru',
    }, config || {});

    if (!this.config.sessionId) throw new Error('[Argus] sessionId is required');
    if (!this.config.token)     throw new Error('[Argus] token (argusSessionToken) is required');

    this._room = null;
    this._heartbeatTimer = null;
    this._mounted = false;

    log('SDK v' + VERSION + ' initialized, session=' + this.config.sessionId);
  }

  ArgusProctoring.prototype.mount = function () {
    if (this._mounted) return Promise.resolve();
    this._mounted = true;

    var self = this;

    return self._loadLiveKit()
      .then(function () {
        log('LiveKit client loaded');
        self._startHeartbeat();
        return self._getStudentToken();
      })
      .then(function (tokenData) {
        if (!tokenData || !tokenData.livekitToken) {
          throw new Error('student-token response missing livekitToken');
        }
        log('LiveKit state: connecting, room=' + tokenData.room);
        return self._connectAndPublish(tokenData);
      })
      .then(function () {
        log('LiveKit state: published');
        return self._signalRecordingReady();
      })
      .then(function () {
        log('LiveKit state: recording_ready sent');
        if (typeof self.config.onVerified === 'function') {
          self.config.onVerified({ networkMode: 'webrtc' });
        }
      })
      .catch(function (err) {
        logError('mount failed:', err && err.message ? err.message : err);
        if (typeof self.config.onError === 'function') {
          self.config.onError(err);
        }
      });
  };

  ArgusProctoring.prototype._loadLiveKit = function () {
    if (window.LivekitClient && window.LivekitClient.Room) {
      return Promise.resolve();
    }
    log('Loading LiveKit client from CDN...');
    return loadScript(LK_CDN_URL).catch(function (err) {
      logError('LiveKit client load failed:', err.message);
      throw err;
    });
  };

  ArgusProctoring.prototype._getStudentToken = function () {
    var cfg = this.config;
    var url = cfg.apiUrl + '/api/v1/external/sessions/' + cfg.sessionId + '/student-token';
    log('Requesting student token...');
    return apiFetch(url, cfg.token)
      .then(function (resp) {
        if (!resp.ok) {
          return resp.text().then(function (body) {
            throw new Error('student-token HTTP ' + resp.status + ': ' + body);
          });
        }
        return resp.json();
      });
  };

  ArgusProctoring.prototype._connectAndPublish = function (tokenData) {
    var LK = window.LivekitClient;
    if (!LK || !LK.Room) {
      return Promise.reject(new Error('LivekitClient not available after load'));
    }

    var self = this;
    var room = new LK.Room({
      adaptiveStream: true,
      dynacast: true,
      videoCaptureDefaults: {
        resolution: { width: 640, height: 480, frameRate: 15 }
      },
      audioCaptureDefaults: {
        echoCancellation: true,
        noiseSuppression: true,
        autoGainControl: true
      }
    });
    this._room = room;

    room.on(LK.RoomEvent.ConnectionStateChanged, function (state) {
      log('LiveKit state:', state);
    });

    room.on(LK.RoomEvent.Disconnected, function (reason) {
      log('LiveKit disconnected:', reason);
      self._stopHeartbeat();
    });

    room.on(LK.RoomEvent.MediaDevicesError, function (err) {
      logError('MediaDevicesError:', err && err.message ? err.message : err);
      if (typeof self.config.onError === 'function') {
        self.config.onError(err);
      }
    });

    return room.connect(tokenData.livekitUrl, tokenData.livekitToken)
      .then(function () {
        log('LiveKit state: connected to room', tokenData.room);
        return room.localParticipant.enableCameraAndMicrophone();
      })
      .then(function () {
        log('Camera and microphone enabled');
        var containerId = self.config.containerId;
        if (containerId) {
          var container = document.getElementById(containerId);
          if (container) {
            room.localParticipant.videoTrackPublications.forEach(function (pub) {
              if (pub.track && typeof pub.track.attach === 'function') {
                var el = pub.track.attach();
                el.autoplay = true;
                el.muted = true;
                el.playsInline = true;
                el.style.cssText = 'width:100%;height:100%;object-fit:cover;border-radius:inherit;';
                container.innerHTML = '';
                container.appendChild(el);
                log('Video attached to container:', containerId);
              }
            });
          } else {
            log('Container not found:', containerId);
          }
        }
      });
  };

  ArgusProctoring.prototype._signalRecordingReady = function () {
    var cfg = this.config;
    var url = cfg.apiUrl + '/api/v1/external/sessions/' + cfg.sessionId + '/recording-ready';
    return apiFetch(url, cfg.token)
      .then(function (resp) {
        log('recording-ready response:', resp.status);
      })
      .catch(function (err) {
        logError('recording-ready failed:', err.message);
      });
  };

  ArgusProctoring.prototype._startHeartbeat = function () {
    var self = this;
    var cfg = this.config;
    var url = cfg.apiUrl + '/api/v1/external/heartbeat';

    function beat() {
      apiFetch(url, cfg.token).catch(function () {});
    }

    beat();
    self._heartbeatTimer = setInterval(beat, 30000);
    log('Heartbeat started (30s interval)');
  };

  ArgusProctoring.prototype._stopHeartbeat = function () {
    if (this._heartbeatTimer) {
      clearInterval(this._heartbeatTimer);
      this._heartbeatTimer = null;
    }
  };

  ArgusProctoring.prototype.destroy = function () {
    this._stopHeartbeat();
    if (this._room) {
      this._room.disconnect();
      this._room = null;
    }
    log('SDK destroyed');
  };

  ArgusProctoring.VERSION = VERSION;

  // ──────────────────────────────────────────────────────────────────
  // Auto-initialization via data-attributes on the <script> tag
  //
  // Looks for the last <script> that loaded this bundle and reads:
  //   data-session-id  — argus session ID
  //   data-token       — argusSessionToken JWT
  //   data-api-url     — (optional) Argus backend URL, default: https://argusai.kz
  // ──────────────────────────────────────────────────────────────────
  function tryAutoInit() {
    var scripts = document.querySelectorAll('script[src]');
    var thisScript = null;
    for (var i = scripts.length - 1; i >= 0; i--) {
      var s = scripts[i];
      if (s.src && s.src.indexOf('argus-sdk') !== -1) {
        thisScript = s;
        break;
      }
    }
    if (!thisScript) return;

    var sessionId = thisScript.getAttribute('data-session-id');
    var token     = thisScript.getAttribute('data-token');
    var apiUrl    = thisScript.getAttribute('data-api-url') || 'https://argusai.kz';

    if (!sessionId || !token) return;

    log('Auto-init from data-attributes, session=' + sessionId);
    var sdk = new ArgusProctoring({ sessionId: sessionId, token: token, apiUrl: apiUrl });
    // Expose instance globally for external access
    if (typeof window !== 'undefined') {
      window._argusSDKInstance = sdk;
    }
    sdk.mount();
  }

  // Run auto-init after DOM is ready
  if (typeof document !== 'undefined') {
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', tryAutoInit);
    } else {
      // DOMContentLoaded already fired — run immediately (or next tick)
      setTimeout(tryAutoInit, 0);
    }
  }

  return ArgusProctoring;
}));
