var currentLoginSessionId = null;
var currentLoginPlatform = null;
var currentLoginPlatformName = null;
var browserLoginPollTimer = null;
var browserLoginScreenTimer = null;
var browserLoginScreenURL = null;
var browserLoginScreenInFlight = false;
var browserLoginViewerActive = false;
var browserLoginScreenRefreshRequested = false;
var browserLoginInputQueue = Promise.resolve();
var browserLoginStartGeneration = 0;
var browserLoginSucceeded = false;

function startBrowserLogin(platform, platformName) {
  var requestGeneration = ++browserLoginStartGeneration;
  currentLoginPlatform = platform;
  currentLoginPlatformName = platformName || platform;
  currentLoginSessionId = null;
  browserLoginSucceeded = false;
  browserLoginScreenRefreshRequested = false;
  var closeButton = document.getElementById('bl-close-after-login');
  if (closeButton) closeButton.textContent = 'Hủy bỏ / Đóng';

  var modal = document.getElementById('browser-login-modal');
  var title = document.getElementById('bl-modal-title');
  var icon = document.getElementById('bl-status-icon');
  var statusText = document.getElementById('bl-status-text');
  var subText = document.getElementById('bl-status-subtext');
  var browserView = document.getElementById('bl-browser-view');
  if (browserView) browserView.hidden = true;
  if (title) title.innerHTML = '<span>🌐</span> Đăng nhập Web AI';
  if (icon) { icon.textContent = '⏳'; icon.style.animation = 'pulse 1.8s infinite ease-in-out'; }
  if (statusText) statusText.textContent = 'Đang khởi động trình duyệt bảo mật...';
  if (subText) subText.textContent = 'Đang mở khung trình duyệt đăng nhập bảo mật...';

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
    if (statusText) statusText.textContent = 'Đang đợi bạn đăng nhập trên trình duyệt...';
    if (subText) subText.textContent = 'Đăng nhập ngay trong khung bên dưới. MiuNova sẽ tự lưu phiên khi xong.';
    if (browserView) browserView.hidden = false;
    browserLoginViewerActive = true;
    refreshBrowserLoginScreen();

    if (browserLoginPollTimer) clearInterval(browserLoginPollTimer);
    browserLoginPollTimer = setInterval(pollBrowserLoginStatus, 1800);
  })
  .catch(function(err) {
    if (requestGeneration !== browserLoginStartGeneration) return;
    if (icon) { icon.textContent = '❌'; icon.style.animation = 'none'; }
    if (statusText) statusText.textContent = 'Lỗi kết nối';
    if (subText) subText.textContent = err.message;
  });
}

function refreshBrowserLoginScreen() {
  if (!currentLoginSessionId || !browserLoginViewerActive || document.hidden || browserLoginScreenInFlight) return;
  var sessionId = currentLoginSessionId;
  browserLoginScreenInFlight = true;
  fetch('/web-ai/browser-login/screen?_=' + Date.now(), { cache: 'no-store', headers: { 'X-Login-Session': sessionId } })
    .then(function(response) { if (!response.ok) throw new Error('screen not ready'); return response.blob(); })
    .then(function(blob) {
      if (currentLoginSessionId !== sessionId) return;
      var image = document.getElementById('bl-browser-screen');
      if (!image) return;
      var nextURL = URL.createObjectURL(blob);
      var oldURL = browserLoginScreenURL;
      browserLoginScreenURL = nextURL;
      image.src = nextURL;
      if (oldURL) URL.revokeObjectURL(oldURL);
    })
    .catch(function() {})
    .finally(function() {
      browserLoginScreenInFlight = false;
      if (browserLoginViewerActive && currentLoginSessionId && !document.hidden) {
        var delay = browserLoginScreenRefreshRequested ? 250 : 8000;
        browserLoginScreenRefreshRequested = false;
        browserLoginScreenTimer = window.setTimeout(refreshBrowserLoginScreen, delay);
      }
    });
}

function scheduleBrowserLoginScreenRefresh(delay) {
  if (!browserLoginViewerActive || !currentLoginSessionId || document.hidden) return;
  if (browserLoginScreenTimer) clearTimeout(browserLoginScreenTimer);
  browserLoginScreenTimer = window.setTimeout(function() {
    if (browserLoginScreenInFlight) {
      browserLoginScreenRefreshRequested = true;
      return;
    }
    refreshBrowserLoginScreen();
  }, delay);
}

function stopBrowserLoginViewer() {
  browserLoginViewerActive = false;
  browserLoginScreenRefreshRequested = false;
  if (browserLoginScreenTimer) {
    clearTimeout(browserLoginScreenTimer);
    browserLoginScreenTimer = null;
  }
  var image = document.getElementById('bl-browser-screen');
  if (image) image.removeAttribute('src');
  if (browserLoginScreenURL) {
    URL.revokeObjectURL(browserLoginScreenURL);
    browserLoginScreenURL = null;
  }
  var view = document.getElementById('bl-browser-view');
  if (view) view.hidden = true;
  var keyboard = document.getElementById('bl-browser-keyboard');
  if (keyboard) keyboard.value = '';
}

document.addEventListener('visibilitychange', function() {
  if (document.hidden) {
    if (browserLoginScreenTimer) {
      clearTimeout(browserLoginScreenTimer);
      browserLoginScreenTimer = null;
    }
  } else if (browserLoginViewerActive) {
    refreshBrowserLoginScreen();
  }
});

function sendBrowserLoginInput(payload) {
  if (!currentLoginSessionId) return;
  var sessionId = currentLoginSessionId;
  browserLoginInputQueue = browserLoginInputQueue.catch(function() {}).then(function() {
    return fetch('/web-ai/browser-login/input', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Login-Session': sessionId },
      body: JSON.stringify(payload)
    }).then(function(response) {
      if (response.ok && currentLoginSessionId === sessionId) {
        scheduleBrowserLoginScreenRefresh(600);
      }
      return response;
    });
  }).catch(function() {});
}

(function bindBrowserLoginControls() {
  var image = document.getElementById('bl-browser-screen');
  if (image) {
    var lastPointerMoveAt = 0;
    function coords(event) {
      var rect = image.getBoundingClientRect();
      return {
        x: Math.max(0, Math.min(1, (event.clientX - rect.left) / rect.width)),
        y: Math.max(0, Math.min(1, (event.clientY - rect.top) / rect.height))
      };
    }
    function pointerPayload(event, action) {
      var point = coords(event);
      return { type: 'pointer', action: action, button: 'left', click_count: Math.min(2, Math.max(1, event.detail || 1)), x: point.x, y: point.y };
    }
    image.addEventListener('pointerdown', function(event) {
      if (event.button !== 0) return;
      event.preventDefault();
      if (image.setPointerCapture) image.setPointerCapture(event.pointerId);
      sendBrowserLoginInput(pointerPayload(event, 'down'));
    });
    image.addEventListener('pointermove', function(event) {
      if (!event.buttons) return;
      var now = Date.now();
      if (now - lastPointerMoveAt < 90) return;
      lastPointerMoveAt = now;
      sendBrowserLoginInput(pointerPayload(event, 'move'));
    });
    image.addEventListener('pointerup', function(event) {
      event.preventDefault();
      sendBrowserLoginInput(pointerPayload(event, 'up'));
    });
    image.addEventListener('pointercancel', function(event) {
      sendBrowserLoginInput(pointerPayload(event, 'up'));
    });
    image.addEventListener('wheel', function(event) {
      event.preventDefault();
      var point = coords(event);
      sendBrowserLoginInput({ type: 'wheel', x: point.x, y: point.y, delta_x: event.deltaX, delta_y: event.deltaY });
    }, { passive: false });
    image.addEventListener('contextmenu', function(event) { event.preventDefault(); });
  }

  var keyboard = document.getElementById('bl-browser-keyboard');
  if (!keyboard) return;
  var composing = false;
  keyboard.addEventListener('compositionstart', function() { composing = true; });
  keyboard.addEventListener('compositionend', function() {
    composing = false;
    var text = keyboard.value;
    keyboard.value = '';
    if (text) sendBrowserLoginInput({ type: 'text', text: text });
  });
  keyboard.addEventListener('keydown', function(event) {
    var supported = ['Backspace', 'Delete', 'Enter', 'Tab', 'Escape', 'ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'];
    if (supported.indexOf(event.key) !== -1) {
      event.preventDefault();
      sendBrowserLoginInput({ type: 'key', key: event.key });
    }
  });
  keyboard.addEventListener('input', function() {
    if (composing) return;
    var text = keyboard.value;
    keyboard.value = '';
    if (text) sendBrowserLoginInput({ type: 'text', text: text });
  });
})();

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
      browserLoginSucceeded = true;
      stopBrowserLoginViewer();
      currentLoginSessionId = null;
      if (icon) { icon.textContent = '🎉'; icon.style.animation = 'none'; }
      if (statusText) statusText.textContent = 'Đăng nhập và lưu phiên thành công!';
      if (subText) subText.textContent = (res.email ? res.email + ' · ' : '') + 'Đã lưu phiên hoạt động.';
      var closeButton = document.getElementById('bl-close-after-login');
      if (closeButton) closeButton.textContent = 'Đóng ngay';
      window.setTimeout(function() {
        if (browserLoginSucceeded) cancelBrowserLogin();
      }, 600);
    } else if (res.status === 'error' || res.status === 'failed' || res.status === 'cancelled' || res.status === 'canceled') {
      if (browserLoginPollTimer) {
        clearInterval(browserLoginPollTimer);
        browserLoginPollTimer = null;
      }
      stopBrowserLoginViewer();
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
  stopBrowserLoginViewer();
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
