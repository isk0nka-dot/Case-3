/*!
 * Argus Proctoring SDK v1.3.0 (ESM)
 * (c) Argus AI — https://argusai.kz
 *
 * ES module — use with dynamic import() or <script type="module">
 *   const { default: ArgusProctoring } = await import('https://argusai.kz/sdk/v1/argus-sdk.esm.js')
 *   new ArgusProctoring({ sessionId, token, apiUrl }).mount()
 */
'use strict';

var VERSION = '1.3.0';
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

// Set global for <script type="module"> users
if (typeof globalThis !== 'undefined') {
  globalThis.ArgusProctoring = ArgusProctoring;
  globalThis.ArgusSDK = { ArgusProctoring: ArgusProctoring, VERSION: VERSION };
}

export default ArgusProctoring;
export { ArgusProctoring, VERSION };
