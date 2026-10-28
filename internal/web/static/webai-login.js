var currentLoginSessionId = null;
var currentLoginPlatform = null;
var currentLoginPlatformName = null;
var browserLoginPollTimer = null;
var browserLoginScreenTimer = null;
var browserLoginScreenPending = false;
var browserLoginScreenObjectURL = null;
var browserLoginStartGeneration = 0;
var browserLoginSucceeded = false;
var browserLoginInputBound = false;

function releaseBrowserLoginScreenURL() {
  if (browserLoginScreenObjectURL) {
    URL.revokeObjectURL(browserLoginScreenObjectURL);
    browserLoginScreenObjectURL = null;
  }
}

function bindBrowserLoginRemoteScreen() {
  if (browserLoginInputBound) return;
  var container = document.getElementById('bl-screen-container');
  var img = document.getElementById('bl-remote-screen');
  if (!container || !img) return;
  browserLoginInputBound = true;

  function relCoords(ev) {
    var rect = img.getBoundingClientRect();
    if (!rect.width || !rect.height) return { x: 0.5, y: 0.5 };
    var x = (ev.clientX - rect.left) / rect.width;
    var y = (ev.clientY - rect.top) / rect.height;
    if (x < 0) x = 0; else if (x > 1) x = 1;
    if (y < 0) y = 0; else if (y > 1) y = 1;
    return { x: x, y: y };
  }

  function sendInput(payload) {
    if (!currentLoginSessionId) return;
    fetch('/web-ai/browser-login/input', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'X-Login-Session': currentLoginSessionId
      },
      body: JSON.stringify(payload)
    }).then(function () {
      setTimeout(refreshBrowserLoginScreen, 120);
    }).catch(function () {});
  }

  img.addEventListener('mousedown', function (ev) {
    ev.preventDefault();
    container.focus();
    var pt = relCoords(ev);
    var btn = ev.button === 2 ? 'right' : (ev.button === 1 ? 'middle' : 'left');
    sendInput({ type: 'pointer', action: 'down', x: pt.x, y: pt.y, button: btn, click_count: ev.detail || 1 });
  });

  img.addEventListener('mouseup', function (ev) {
    ev.preventDefault();
    var pt = relCoords(ev);
    var btn = ev.button === 2 ? 'right' : (ev.button === 1 ? 'middle' : 'left');
    sendInput({ type: 'pointer', action: 'up', x: pt.x, y: pt.y, button: btn, click_count: ev.detail || 1 });
  });

  img.addEventListener('contextmenu', function (ev) {
    ev.preventDefault();
  });

  img.addEventListener('wheel', function (ev) {
    ev.preventDefault();
    var pt = relCoords(ev);
    sendInput({
      type: 'wheel',
      x: pt.x,
      y: pt.y,
      delta_x: Math.max(-1200, Math.min(1200, ev.deltaX || 0)),
      delta_y: Math.max(-1200, Math.min(1200, ev.deltaY || 0))
    });
  }, { passive: false });

  container.addEventListener('keydown', function (ev) {
    if (!currentLoginSessionId) return;
    var specialKeys = {
      Enter: true, Tab: true, Backspace: true, Escape: true, Delete: true,
      ArrowLeft: true, ArrowRight: true, ArrowUp: true, ArrowDown: true,
      Home: true, End: true, PageUp: true, PageDown: true, ' ': true
    };
    if (specialKeys[ev.key]) {
      ev.preventDefault();
      var keyName = ev.key === ' ' ? 'Space' : ev.key;
      sendInput({ type: 'key', key: keyName, x: 0, y: 0 });
      return;
    }
    if (ev.key && ev.key.length === 1 && !ev.ctrlKey && !ev.metaKey && !ev.altKey) {
      ev.preventDefault();
      sendInput({ type: 'text', text: ev.key, x: 0, y: 0 });
    }
  });

  container.addEventListener('paste', function (ev) {
    if (!currentLoginSessionId) return;
    var text = (ev.clipboardData || window.clipboardData).getData('text');
    if (text) {
      ev.preventDefault();
      sendInput({ type: 'text', text: text.slice(0, 4096), x: 0, y: 0 });
    }
  });
}

function refreshBrowserLoginScreen() {
  if (!currentLoginSessionId || browserLoginScreenPending) return;
  browserLoginScreenPending = true;
  fetch('/web-ai/browser-login/screen?t=' + Date.now(), {
    headers: { 'X-Login-Session': currentLoginSessionId },
    cache: 'no-store'
  })
  .then(function (r) {
    if (!r.ok) throw new Error('not ready');
    return r.blob();
  })
  .then(function (blob) {
    browserLoginScreenPending = false;
    if (!currentLoginSessionId) return;
    var container = document.getElementById('bl-screen-container');
    var img = document.getElementById('bl-remote-screen');
    if (!container || !img) return;
    var nextURL = URL.createObjectURL(blob);
    releaseBrowserLoginScreenURL();
    browserLoginScreenObjectURL = nextURL;
    img.src = nextURL;
    container.style.display = 'block';
  })
  .catch(function () {
    browserLoginScreenPending = false;
  });
}

function startBrowserLogin(platform, platformName) {
  var requestGeneration = ++browserLoginStartGeneration;
  currentLoginPlatform = platform;
  currentLoginPlatformName = platformName || platform;
  currentLoginSessionId = null;
  browserLoginSucceeded = false;
  bindBrowserLoginRemoteScreen();

  var closeButton = document.getElementById('bl-close-after-login');
  if (closeButton) closeButton.textContent = 'Hủy bỏ / Đóng';

  var modal = document.getElementById('browser-login-modal');
  var title = document.getElementById('bl-modal-title');
  var icon = document.getElementById('bl-status-icon');
  var statusText = document.getElementById('bl-status-text');
  var subText = document.getElementById('bl-status-subtext');
  var screenContainer = document.getElementById('bl-screen-container');
  if (screenContainer) screenContainer.style.display = 'none';
  releaseBrowserLoginScreenURL();

  if (title) title.innerHTML = '<span>🌐</span> Đăng nhập ' + (currentLoginPlatformName || 'Web AI');
  if (icon) { icon.textContent = '⏳'; icon.style.animation = 'pulse 1.8s infinite ease-in-out'; }
  if (statusText) statusText.textContent = 'Đang khởi động trình duyệt đăng nhập...';
  if (subText) subText.textContent = 'Cửa sổ trình duyệt đang được mở trên máy tính của bạn...';

  if (modal) modal.hidden = false;

  fetch('/web-ai/browser-login/start?platform=' + encodeURIComponent(platform), {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ platform: platform })
  })
  .then(function(r) { return r.json(); })
  .then(function(res) {
    if (requestGeneration !== browserLoginStartGeneration) {
      if (res.ok && res.session_id) {
        fetch('/web-ai/browser-login/cancel', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json', 'X-Login-Session': res.session_id },
          body: JSON.stringify({ session_id: res.session_id })
        }).catch(function() {});
      }
      return;
    }
    if (!res.ok) {
      if (icon) { icon.textContent = '❌'; icon.style.animation = 'none'; }
      if (statusText) statusText.textContent = 'Không thể mở trình duyệt';
      if (subText) subText.textContent = res.error || 'Vui lòng kiểm tra lại trình duyệt Edge/Chrome trên máy.';
      return;
    }
    currentLoginSessionId = res.session_id;
    if (statusText) statusText.textContent = 'Vui lòng đăng nhập trên trình duyệt hoặc màn hình bên dưới';
    if (subText) subText.textContent = 'Bạn có thể thao tác trực tiếp trên khung hình từ xa hoặc cửa sổ trình duyệt. Hệ thống sẽ tự động phát hiện và lưu phiên.';

    if (browserLoginPollTimer) clearInterval(browserLoginPollTimer);
    browserLoginPollTimer = setInterval(pollBrowserLoginStatus, 1500);
    if (browserLoginScreenTimer) clearInterval(browserLoginScreenTimer);
    refreshBrowserLoginScreen();
    browserLoginScreenTimer = setInterval(refreshBrowserLoginScreen, 1200);
  })
  .catch(function(err) {
    if (requestGeneration !== browserLoginStartGeneration) return;
    if (icon) { icon.textContent = '❌'; icon.style.animation = 'none'; }
    if (statusText) statusText.textContent = 'Lỗi kết nối';
    if (subText) subText.textContent = err.message;
  });
}

function pollBrowserLoginStatus() {
  if (!currentLoginSessionId) return;

  fetch('/web-ai/browser-login/status', { headers: { 'X-Login-Session': currentLoginSessionId } })
  .then(function(r) { return r.json(); })
  .then(function(res) {
    var icon = document.getElementById('bl-status-icon');
    var statusText = document.getElementById('bl-status-text');
    var subText = document.getElementById('bl-status-subtext');
    if (res.status === 'waiting_for_user' && res.message) {
      if (subText) subText.textContent = res.message;
    }

    if (res.status === 'success') {
      if (browserLoginPollTimer) {
        clearInterval(browserLoginPollTimer);
        browserLoginPollTimer = null;
      }
      if (browserLoginScreenTimer) {
        clearInterval(browserLoginScreenTimer);
        browserLoginScreenTimer = null;
      }
      browserLoginSucceeded = true;
      currentLoginSessionId = null;
      var screenContainer = document.getElementById('bl-screen-container');
      if (screenContainer) screenContainer.style.display = 'none';
      releaseBrowserLoginScreenURL();
      if (icon) { icon.textContent = '🎉'; icon.style.animation = 'none'; }
      if (statusText) statusText.textContent = 'Đăng nhập và lưu phiên thành công!';
      if (subText) subText.textContent = (res.email ? res.email + ' · ' : '') + 'Đã lưu phiên hoạt động.';
      var closeButton = document.getElementById('bl-close-after-login');
      if (closeButton) closeButton.textContent = 'Đóng';
      window.setTimeout(function() {
        if (browserLoginSucceeded) cancelBrowserLogin();
      }, 900);
    } else if (res.status === 'error' || res.status === 'failed' || res.status === 'cancelled' || res.status === 'canceled') {
      if (browserLoginPollTimer) {
        clearInterval(browserLoginPollTimer);
        browserLoginPollTimer = null;
      }
      if (browserLoginScreenTimer) {
        clearInterval(browserLoginScreenTimer);
        browserLoginScreenTimer = null;
      }
      var screenContainerErr = document.getElementById('bl-screen-container');
      if (screenContainerErr) screenContainerErr.style.display = 'none';
      releaseBrowserLoginScreenURL();
      if (icon) { icon.textContent = '⚠️'; icon.style.animation = 'none'; }
      if (statusText) statusText.textContent = (res.status === 'cancelled' || res.status === 'canceled') ? 'Đã hủy đăng nhập' : 'Đăng nhập không thành công';
      if (subText) subText.textContent = res.error || 'Phiên đăng nhập đã kết thúc.';
    }
  })
  .catch(function(err) {
    console.error('Lỗi kiểm tra trạng thái login:', err);
  });
}

function cancelBrowserLogin() {
  browserLoginStartGeneration++;
  if (browserLoginPollTimer) {
    clearInterval(browserLoginPollTimer);
    browserLoginPollTimer = null;
  }
  if (browserLoginScreenTimer) {
    clearInterval(browserLoginScreenTimer);
    browserLoginScreenTimer = null;
  }
  var screenContainer = document.getElementById('bl-screen-container');
  if (screenContainer) screenContainer.style.display = 'none';
  releaseBrowserLoginScreenURL();
  if (currentLoginSessionId) {
    fetch('/web-ai/browser-login/cancel', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Login-Session': currentLoginSessionId },
      body: JSON.stringify({ session_id: currentLoginSessionId })
    }).catch(function() {});
    currentLoginSessionId = null;
  }
  var modal = document.getElementById('browser-login-modal');
  if (modal) modal.hidden = true;
  if (browserLoginSucceeded) {
    browserLoginSucceeded = false;
    window.location.reload();
  }
}
