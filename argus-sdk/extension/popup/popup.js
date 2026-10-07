// Argus Proctoring — popup status view.
chrome.runtime.sendMessage({ cmd: 'PING' }, (resp) => {
  const dot = document.getElementById('dot');
  const status = document.getElementById('status');
  const version = document.getElementById('version');
  const extid = document.getElementById('extid');

  if (chrome.runtime.lastError || !resp) {
    status.textContent = 'Service worker unavailable';
    return;
  }

  version.textContent = resp.version || '—';
  extid.textContent = resp.extensionId || '—';

  if (resp.examActive) {
    dot.classList.add('active');
    status.textContent = 'Active — exam monitoring on';
  } else {
    status.textContent = 'Idle — no active exam';
  }
});
