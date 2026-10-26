var currentLoginSessionId = null;
var currentLoginPlatform = null;
var currentLoginPlatformName = null;
var browserLoginPollTimer = null;
var browserLoginStartGeneration = 0;
var browserLoginSucceeded = false;

function startBrowserLogin(platform, platformName) {
  var requestGeneration = ++browserLoginStartGeneration;
  currentLoginPlatform = platform;
  currentLoginPlatformName = platformName || platform;
  currentLoginSessionId = null;
  browserLoginSucceeded = false;

  var closeButton = document.getElementById('bl-close-after-login');
  if (closeButton) closeButton.textContent = 'Hủy bỏ / Đóng';

  var modal = document.getElementById('browser-login-modal');
  var title = document.getElementById('bl-modal-title');
  var icon = document.getElementById('bl-status-icon');
  var statusText = document.getElementById('bl-status-text');
  var subText = document.getElementById('bl-status-subtext');

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
    if (statusText) statusText.textContent = 'Vui lòng đăng nhập trên trình duyệt vừa mở';
    if (subText) subText.textContent = 'Hãy hoàn tất đăng nhập trên cửa sổ trình duyệt thực tế. Hệ thống sẽ tự động phát hiện và lưu phiên.';

    if (browserLoginPollTimer) clearInterval(browserLoginPollTimer);
    browserLoginPollTimer = setInterval(pollBrowserLoginStatus, 1500);
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
      browserLoginSucceeded = true;
      currentLoginSessionId = null;
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
