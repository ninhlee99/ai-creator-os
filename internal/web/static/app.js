/* app.js — JS dùng chung toàn dashboard aicos (R2-W3).
   Một nguồn cho: toast, modal xác nhận, sidebar active, file-picker,
   theme toggle, polling job Studio. Không định nghĩa lại nhãn trạng
   thái — nhãn/class do server render sẵn. */

(function () {
  'use strict';

  // esc: escape HTML entities — chống stored XSS khi nối dữ liệu (caption do
  // LLM/user sinh, preview, link API) vào innerHTML (đợt AF bảo mật).
  function esc(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  // ------------------------------------------------------------- toast
  var toastTimer = null;
  function toastEl() {
    var t = document.getElementById('toast');
    if (!t) {
      t = document.createElement('div');
      t.id = 'toast';
      t.className = 'toast';
      t.hidden = true;
      document.body.appendChild(t);
    }
    return t;
  }
  window.showToast = function (msg, ok) {
    var t = toastEl();
    t.textContent = msg;
    t.hidden = false;
    t.className = 'toast ' + (ok === false ? 'err' : 'ok');
    clearTimeout(toastTimer);
    toastTimer = setTimeout(function () { t.hidden = true; }, 4000);
  };
  // Kết quả thao tác từ redirect ?ok= / ?err= (R2-13): hiện toast rồi
  // dọn query để F5 không hiện lại.
  try {
    var q = new URLSearchParams(location.search);
    var ok = q.get('ok'), err = q.get('err');
    if (ok || err) {
      window.showToast(ok || err, !err);
      q.delete('ok'); q.delete('err');
      var qs = q.toString();
      history.replaceState(null, '', location.pathname + (qs ? '?' + qs : '') + location.hash);
    }
  } catch (e) { /* bỏ qua */ }

  // ------------------------------------------------------ shared app dialog
  // One in-app modal handles notices, confirmations and text/password prompts.
  var dialogBackdrop = document.getElementById('aicos-dialog-backdrop');
  var dialog = document.getElementById('aicos-dialog');
  var dialogQueue = [];
  var activeDialog = null;
  var dialogParts = dialog ? {
    title: document.getElementById('aicos-dialog-title'),
    message: document.getElementById('aicos-dialog-message'),
    icon: document.getElementById('aicos-dialog-icon'),
    field: document.getElementById('aicos-dialog-field'),
    label: document.getElementById('aicos-dialog-label'),
    input: document.getElementById('aicos-dialog-input'),
    textarea: document.getElementById('aicos-dialog-textarea'),
    error: document.getElementById('aicos-dialog-error'),
    cancel: document.getElementById('aicos-dialog-cancel'),
    confirm: document.getElementById('aicos-dialog-confirm')
  } : null;

  function activeDialogField() {
    if (!activeDialog || activeDialog.options.mode !== 'prompt' || !dialogParts) return null;
    return activeDialog.options.multiline ? dialogParts.textarea : dialogParts.input;
  }

  function hideDialog(result) {
    if (!activeDialog || !dialogBackdrop) return;
    var finished = activeDialog;
    activeDialog = null;
    dialogBackdrop.hidden = true;
    document.body.classList.remove('aicos-dialog-open');
    if (finished.previousFocus && finished.previousFocus.focus) finished.previousFocus.focus();
    finished.resolve(result);
    setTimeout(showNextDialog, 0);
  }

  function confirmDialog() {
    if (!activeDialog || !dialogParts) return;
    if (activeDialog.options.mode === 'prompt') {
      var field = activeDialogField();
      var value = field ? field.value : '';
      if (activeDialog.options.required && !String(value).trim()) {
        dialogParts.error.textContent = activeDialog.options.requiredMessage || 'Vui lòng nhập giá trị.';
        if (field) field.focus();
        return;
      }
      hideDialog(value);
      return;
    }
    hideDialog(activeDialog.options.mode === 'confirm');
  }

  function showNextDialog() {
    if (activeDialog || !dialogQueue.length || !dialogBackdrop || !dialog || !dialogParts) return;
    var item = dialogQueue.shift();
    var options = item.options;
    activeDialog = {
      options: options,
      resolve: item.resolve,
      previousFocus: document.activeElement
    };

    var mode = options.mode || 'notice';
    var tone = options.tone || (mode === 'confirm' ? 'warning' : 'info');
    var icons = { info: 'i', success: '✓', warning: '!', danger: '×', error: '×' };
    dialog.dataset.tone = tone;
    dialogParts.title.textContent = options.title || (mode === 'confirm' ? 'Xác nhận' : mode === 'prompt' ? 'Nhập thông tin' : 'Thông báo');
    dialogParts.message.textContent = options.message == null ? '' : String(options.message);
    dialogParts.icon.textContent = options.icon || icons[tone] || icons.info;
    dialogParts.field.hidden = mode !== 'prompt';
    dialogParts.error.textContent = '';
    dialogParts.cancel.hidden = mode === 'notice';
    dialogParts.cancel.textContent = options.cancelText || 'Hủy';
    dialogParts.confirm.textContent = options.confirmText || (mode === 'notice' ? 'Đóng' : mode === 'prompt' ? 'Tiếp tục' : 'Xác nhận');
    dialogParts.confirm.className = 'btn ' + (options.danger ? 'btn-danger' : 'btn-primary');
    dialogParts.input.hidden = Boolean(options.multiline);
    dialogParts.textarea.hidden = !options.multiline;
    if (mode === 'prompt') {
      var field = options.multiline ? dialogParts.textarea : dialogParts.input;
      if (!options.multiline) field.type = options.inputType || 'text';
      field.value = options.defaultValue == null ? '' : String(options.defaultValue);
      field.placeholder = options.placeholder || '';
      field.readOnly = Boolean(options.readOnly);
      dialogParts.label.textContent = options.label || 'Giá trị';
      dialogParts.label.htmlFor = options.multiline ? 'aicos-dialog-textarea' : 'aicos-dialog-input';
      field.setAttribute('aria-label', options.label || 'Giá trị');
      dialogParts.field.hidden = false;
    } else {
      dialogParts.input.value = '';
      dialogParts.textarea.value = '';
      dialogParts.input.readOnly = false;
      dialogParts.textarea.readOnly = false;
    }

    dialogBackdrop.hidden = false;
    document.body.classList.add('aicos-dialog-open');
    var focusTarget = mode === 'prompt' ? activeDialogField() : (mode === 'notice' ? dialogParts.confirm : dialogParts.cancel);
    setTimeout(function () {
      if (focusTarget && focusTarget.focus) {
        focusTarget.focus();
        if (mode === 'prompt' && options.selectOnOpen && focusTarget.select) focusTarget.select();
      }
    }, 0);
  }

  function showDialog(options) {
    return new Promise(function (resolve) {
      if (!dialogBackdrop || !dialog || !dialogParts) {
        if ((options || {}).mode === 'notice') window.showToast((options || {}).message || '', false);
        resolve((options || {}).mode === 'confirm' ? false : null);
        return;
      }
      dialogQueue.push({ options: options || {}, resolve: resolve });
      showNextDialog();
    });
  }

  window.showAppNotice = function (message, options) {
    return showDialog(Object.assign({}, options || {}, { mode: 'notice', message: message }));
  };
  window.showAppConfirm = function (message, options) {
    return showDialog(Object.assign({}, options || {}, { mode: 'confirm', message: message }));
  };
  window.showAppPrompt = function (message, defaultValue, options) {
    return showDialog(Object.assign({}, options || {}, { mode: 'prompt', message: message, defaultValue: defaultValue }));
  };

  if (dialogParts) {
    dialogParts.cancel.addEventListener('click', function () {
      if (!activeDialog) return;
      hideDialog(activeDialog.options.mode === 'prompt' ? null : false);
    });
    dialogParts.confirm.addEventListener('click', confirmDialog);
    dialogBackdrop.addEventListener('click', function (event) {
      if (event.target === dialogBackdrop && activeDialog && activeDialog.options.dismissible !== false) {
        hideDialog(activeDialog.options.mode === 'prompt' ? null : false);
      }
    });
    document.addEventListener('keydown', function (event) {
      if (!activeDialog) return;
      if (event.key === 'Escape' && activeDialog.options.dismissible !== false) {
        event.preventDefault();
        hideDialog(activeDialog.options.mode === 'prompt' ? null : false);
        return;
      }
      if (event.key === 'Enter' && activeDialog.options.mode === 'prompt' && event.target !== dialogParts.textarea) {
        event.preventDefault();
        confirmDialog();
        return;
      }
      if (event.key === 'Tab') {
        var focusable = Array.from(dialog.querySelectorAll('button:not([hidden]), input:not([hidden]), textarea:not([hidden])')).filter(function (el) { return !el.disabled; });
        if (!focusable.length) return;
        var first = focusable[0], last = focusable[focusable.length - 1];
        if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
        else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
      }
    });
  }

  // Forms marked data-confirm submit only after the in-app confirmation.
  var confirmingForms = new WeakSet();
  document.addEventListener('submit', function (event) {
    var form = event.target;
    if (!form || !form.getAttribute) return;
    var submitter = event.submitter;
    var confirmSource = submitter && submitter.hasAttribute('data-confirm') ? submitter : form;
    var message = confirmSource.getAttribute('data-confirm');
    if (!message || form._aicosConfirmed) return;
    event.preventDefault();
    if (confirmingForms.has(form)) return;
    confirmingForms.add(form);
    var danger = confirmSource.hasAttribute('data-confirm-danger') || /x[oó]a|dừng khẩn|tắt công tắc/i.test(message);
    window.showAppConfirm(message, {
      title: confirmSource.getAttribute('data-confirm-title') || 'Xác nhận thao tác',
      confirmText: confirmSource.getAttribute('data-confirm-label') || 'Xác nhận',
      danger: danger,
      tone: danger ? 'danger' : 'warning'
    }).then(function (confirmed) {
      if (!confirmed) return;
      form._aicosConfirmed = true;
      if (submitter && submitter.form === form) form.requestSubmit(submitter);
      else form.requestSubmit();
      setTimeout(function () { form._aicosConfirmed = false; }, 0);
    }).finally(function () {
      confirmingForms.delete(form);
    });
  }, true);

  // Replace htmx's native htmx:confirm() prompt with the shared dialog.
  document.body.addEventListener('htmx:confirm', function (event) {
    var detail = event.detail || {};
    if (!detail.question) return;
    event.preventDefault();
    window.showAppConfirm(detail.question).then(function (confirmed) {
      if (confirmed && typeof detail.issueRequest === 'function') detail.issueRequest(true);
    });
  });

  // ------------------------------------------------------ video modal
  function videoSpec(video) {
    var width = video.videoWidth, height = video.videoHeight;
    if (!width || !height) return '';
    var ratio = width / height, aspect = '';
    if (Math.abs(ratio - 9 / 16) < 0.01) aspect = ' · 9:16';
    else if (Math.abs(ratio - 16 / 9) < 0.02) aspect = ' · 16:9';
    else if (Math.abs(ratio - 1) < 0.01) aspect = ' · 1:1';
    return width + '×' + height + aspect;
  }
  document.querySelectorAll('video[data-video-spec]').forEach(function (video) {
    var badge = video.parentElement && video.parentElement.querySelector('.video-badge-spec');
    if (!badge) return;
    function updateBadge() {
      var spec = videoSpec(video);
      if (spec) badge.textContent = '🎞 ' + spec;
    }
    video.addEventListener('error', function () { badge.textContent = 'Không đọc được khổ video'; });
    video.addEventListener('loadedmetadata', updateBadge);
    if (video.readyState >= 1) updateBadge();
  });

  var videoModalPlayer = document.getElementById('video-modal-player');
  var videoModalFrame = document.getElementById('video-modal-frame');
  var videoModalSpec = document.getElementById('video-modal-spec');
  function fitVideoModal() {
    var video = videoModalPlayer, frame = videoModalFrame;
    if (!video || !frame || !video.videoWidth || !video.videoHeight) return;
    var style = window.getComputedStyle(frame.parentElement);
    var maxWidth = frame.parentElement.clientWidth -
      parseFloat(style.paddingLeft || 0) - parseFloat(style.paddingRight || 0);
    var maxHeight = window.innerHeight * 0.72;
    var ratio = video.videoWidth / video.videoHeight;
    var width = maxWidth, height = width / ratio;
    if (height > maxHeight) {
      height = maxHeight;
      width = height * ratio;
    }
    frame.style.width = Math.floor(width) + 'px';
    frame.style.height = Math.floor(height) + 'px';
    frame.style.aspectRatio = video.videoWidth + ' / ' + video.videoHeight;
    if (videoModalSpec) videoModalSpec.textContent = '🎞 ' + videoSpec(video);
  }
  if (videoModalPlayer) videoModalPlayer.addEventListener('loadedmetadata', fitVideoModal);
  if (videoModalPlayer) videoModalPlayer.addEventListener('error', function () {
    if (videoModalSpec) videoModalSpec.textContent = 'Không đọc được thông số video';
  });
  window.addEventListener('resize', fitVideoModal);

  window.playVideoModal = function (src, title, qcVerified) {
    var m = document.getElementById('video-modal');
    var p = document.getElementById('video-modal-player');
    var t = document.getElementById('video-modal-title');
    var d = document.getElementById('video-modal-download');
    var pub = document.getElementById('video-modal-publish');
    if (!m || !p) return;
    if (t) t.textContent = title || 'Xem trước video';
    if (videoModalSpec) videoModalSpec.textContent = 'Đang đọc thông số video…';
    if (videoModalFrame) {
      videoModalFrame.style.width = '';
      videoModalFrame.style.height = '';
      videoModalFrame.style.aspectRatio = '9 / 16';
    }
    p.src = src;
    if (d) d.href = src;
    if (pub) {
      pub.hidden = qcVerified !== true;
      pub.style.display = qcVerified === true ? '' : 'none';
      if (qcVerified === true) {
        var params = new URLSearchParams();
        params.set('video', src);
        if (title) params.set('title', title);
        pub.href = '/publishers?' + params.toString();
      }
    }
    m.hidden = false;
    m.style.display = 'flex';
    p.play().catch(function () {});
  };
  (function () {
    var m = document.getElementById('video-modal');
    var p = document.getElementById('video-modal-player');
    var c = document.getElementById('video-modal-close');
    if (!m) return;
    function close() {
      if (p) { p.pause(); p.src = ''; }
      m.hidden = true;
      m.style.display = 'none';
    }
    if (c) c.addEventListener('click', close);
    m.addEventListener('click', function (e) { if (e.target === m) close(); });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && !m.hidden) close();
    });
  })();

  // ------------------------------------------------------ sidebar active & drawer
  (function () {
    var full = location.pathname + location.search;
    var p = location.pathname;
    var bestMatch = null;
    var bestLen = -1;
    document.querySelectorAll('.sidebar nav a').forEach(function (a) {
      var h = a.getAttribute('href');
      if (!h) return;
      if (h === full) {
        bestMatch = a;
        bestLen = 99999;
      } else if (bestLen < 99999) {
        var clean = h.split('?')[0];
        if (clean === p && clean !== '/') {
          bestMatch = a;
          bestLen = 9999;
        } else if (clean !== '/' && p.indexOf(clean) === 0 && clean.length > bestLen) {
          bestMatch = a;
          bestLen = clean.length;
        }
      }
    });
    if (bestMatch) {
      bestMatch.classList.add('active');
      bestMatch.setAttribute('aria-current', 'page');
    }

    // Mobile drawer toggle
    var toggleBtn = document.getElementById('nav-toggle');
    var sidebar = document.querySelector('.sidebar');
    var backdrop = document.getElementById('sidebar-backdrop');
    if (toggleBtn && sidebar) {
      toggleBtn.addEventListener('click', function (e) {
        e.stopPropagation();
        var open = sidebar.classList.toggle('open');
        if (backdrop) backdrop.classList.toggle('open', open);
      });
      if (backdrop) {
        backdrop.addEventListener('click', function () {
          sidebar.classList.remove('open');
          backdrop.classList.remove('open');
        });
      }
      sidebar.querySelectorAll('nav a').forEach(function (a) {
        a.addEventListener('click', function () {
          sidebar.classList.remove('open');
          if (backdrop) backdrop.classList.remove('open');
        });
      });
      document.addEventListener('keydown', function (e) {
        if (e.key === 'Escape' && sidebar.classList.contains('open')) {
          sidebar.classList.remove('open');
          if (backdrop) backdrop.classList.remove('open');
        }
      });
    }

    // Desktop sidebar collapse / expand toggle
    var layout = document.querySelector('.layout');
    var topbarToggle = document.getElementById('topbar-sidebar-toggle');
    var wfSidebarToggle = document.getElementById('wf-sidebar-toggle');
    var brandInfo = document.querySelector('.sidebar .brand-info');

    try {
      if (localStorage.getItem('aicos-sidebar-collapsed') === '1' && layout) {
        layout.classList.add('sidebar-collapsed');
      }
    } catch (e) {}

    function toggleSidebarCollapse() {
      if (!layout) return;
      var isCollapsed = layout.classList.toggle('sidebar-collapsed');
      try {
        localStorage.setItem('aicos-sidebar-collapsed', isCollapsed ? '1' : '0');
      } catch (e) {}

      // Notify window and canvas of layout change
      window.dispatchEvent(new Event('resize'));
      if (window.flowCanvas && typeof window.flowCanvas.renderWires === 'function') {
        setTimeout(function () {
          window.flowCanvas.renderWires();
        }, 220);
      }
    }

    if (brandInfo) {
      brandInfo.addEventListener('click', function () {
        if (layout && layout.classList.contains('sidebar-collapsed')) {
          toggleSidebarCollapse();
        }
      });
    }
    if (topbarToggle) topbarToggle.addEventListener('click', toggleSidebarCollapse);
    if (wfSidebarToggle) wfSidebarToggle.addEventListener('click', toggleSidebarCollapse);

    document.addEventListener('keydown', function (e) {
      if (e.key === '[' && !['INPUT', 'TEXTAREA', 'SELECT'].includes(e.target ? e.target.tagName : '')) {
        toggleSidebarCollapse();
      }
    });
  })();

  // ----------------------------------------------------- emoji spacing
  // Chrome/macOS vẽ khoảng trắng ngay sau emoji bằng font emoji (gần như
  // rộng 0) → "📖Kể chuyện". Bọc emoji vào span.ico có lề phải, bỏ khoảng
  // trắng thừa. Bỏ qua ô nhập/code/canvas để không tốn CPU khi kéo thả node.
  var isMutatingEmoji = false;
  function spaceEmoji(root) {
    if (isMutatingEmoji || !root || !root.nodeType) return;
    if (root.closest && root.closest('#flow-canvas-wrap, .flow-canvas')) return;
    var re = /(\p{Extended_Pictographic}(?:\uFE0F|\u200D\p{Extended_Pictographic})*\uFE0F?)[ \u00A0]+(?=\S)/u;
    var walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
      acceptNode: function (n) {
        var p = n.parentNode;
        if (!p || /^(SCRIPT|STYLE|TEXTAREA|CODE|PRE|OPTION|INPUT|VIDEO|AUDIO|SVG)$/.test(p.nodeName) ||
            (p.classList && (p.classList.contains('ico') || p.classList.contains('flow-node')))) return NodeFilter.FILTER_REJECT;
        return re.test(n.nodeValue) ? NodeFilter.FILTER_ACCEPT : NodeFilter.FILTER_SKIP;
      }
    });
    var nodes = [];
    while (walker.nextNode()) nodes.push(walker.currentNode);
    if (!nodes.length) return;
    isMutatingEmoji = true;
    try {
      nodes.forEach(function (n) {
        var m, rest = n.nodeValue, frag = document.createDocumentFragment();
        while ((m = re.exec(rest))) {
          if (m.index) frag.appendChild(document.createTextNode(rest.slice(0, m.index)));
          var sp = document.createElement('span');
          sp.className = 'ico';
          sp.textContent = m[1];
          frag.appendChild(sp);
          rest = rest.slice(m.index + m[0].length);
        }
        frag.appendChild(document.createTextNode(rest));
        if (n.parentNode) n.parentNode.replaceChild(frag, n);
      });
    } finally {
      isMutatingEmoji = false;
    }
  }
  spaceEmoji(document.body);
  document.body.addEventListener('htmx:afterSwap', function (ev) {
    if (ev.detail && ev.detail.target) spaceEmoji(ev.detail.target);
  });

  // -------------------------------------------------------- file-picker
  document.querySelectorAll('.file-pick input[type=file]').forEach(function (inp) {
    inp.addEventListener('change', function () {
      var name = inp.closest('.file-pick').querySelector('.file-name');
      if (name) name.textContent = inp.files.length > 1 ? inp.files.length + ' ảnh đã chọn'
        : inp.files.length ? inp.files[0].name : 'Chưa chọn file';
    });
  });

  // ------------------------------------------------------ theme toggle
  (function () {
    var btn = document.getElementById('theme-toggle');
    if (!btn) return;
    function sync() {
      btn.textContent = document.documentElement.dataset.theme === 'dark' ? '☀️' : '🌙';
    }
    sync();
    btn.addEventListener('click', function () {
      var next = document.documentElement.dataset.theme === 'dark' ? 'light' : 'dark';
      document.documentElement.dataset.theme = next;
      try { localStorage.setItem('aicos-theme', next); } catch (e) {}
      sync();
    });
  })();

  // ----------------------------------------------------- studio helpers
  // Storyboard từng job: dùng nhãn/class do server trả (không định nghĩa
  // lại nhãn ở client — R2-W3).
  var boards = {};
  // Xem prompt đầy đủ của một shot (storyboard QC — Film Wave 2 / P1-3).
  window.togglePrompt = function (pid) {
    var el = document.getElementById(pid);
    if (!el) return;
    if (el.style.display === 'none') {
      el.textContent = (window.__shotPrompts || {})[pid] || '(không có prompt)';
      el.style.display = 'block';
    } else {
      el.style.display = 'none';
    }
  };

  function renderShotCardHTML(id, a) {
    var name = a.kind === 'clip' ? 'Cảnh ' + (a.idx + 1) : a.kind + ' ' + (a.idx + 1);
    var pid = 'prompt-' + id + '-' + a.id;
    (window.__shotPrompts = window.__shotPrompts || {})[pid] = a.prompt || '';
    var h = '<div class="shot board-shot-card" data-asset-id="' + esc(a.id) + '" style="background:var(--bg3);border:1px solid var(--border);border-radius:10px;padding:10px;display:flex;flex-direction:column;justify-content:space-between"><div class="shot-head" style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px"><b style="font-size:12px">' + esc(name) +
      '</b> <span class="badge shot-status-badge badge-' + esc(a.class) + '" style="font-size:9.5px">' + esc(a.label) + '</span>' +
      '</div><div class="shot-media-slot">';

    var hasImg = a.image_url || (a.preview && /\.(png|jpe?g|webp)$/i.test(a.preview));
    var hasVid = a.video_url || (a.preview && /\.(mp4|mov|webm)$/i.test(a.preview));
    var imgSrc = a.image_url || (hasImg ? a.preview : '');
    var vidSrc = a.video_url || (hasVid ? a.preview : '');

    h += renderShotMediaInner(imgSrc, vidSrc, a.preview);
    h += '</div>';
    h += '<div class="hint" id="' + pid + '" style="display:none;white-space:pre-wrap;max-height:160px;overflow:auto;margin:6px 0;background:var(--bg);padding:6px;border-radius:6px;font-size:11px"></div>';
    h += '<div style="margin-top:6px"><button type="button" class="btn btn-secondary btn-sm" onclick="togglePrompt(\'' + pid + '\')" style="font-size:10.5px;padding:3px 8px">Xem prompt</button>';
    h += '</div></div>';
    return h;
  }

  function renderShotMediaInner(imgSrc, vidSrc, preview) {
    var h = '';
    if (imgSrc) {
      h += '<div class="shot-img-wrap" data-src="' + esc(imgSrc) + '" style="margin:4px 0">' +
           '<div style="font-size:10px;font-weight:700;color:var(--muted);margin-bottom:3px">🖼️ Keyframe:</div>' +
           '<div style="width:100%;aspect-ratio:9/16;background:#000;border-radius:8px;overflow:hidden;border:1px solid var(--border);position:relative">' +
           '<img src="' + esc(imgSrc) + '" style="width:100%;height:100%;object-fit:contain;display:block;cursor:pointer" onclick="window.open(\'' + esc(imgSrc) + '\', \'_blank\')" title="Bấm để xem ảnh gốc 4K">' +
           '</div></div>';
    }
    if (vidSrc) {
      h += '<div class="shot-vid-wrap" data-src="' + esc(vidSrc) + '" style="margin:4px 0">' +
           '<div style="font-size:10px;font-weight:700;color:var(--muted);margin-bottom:3px">🎬 Video AI:</div>' +
           '<div style="width:100%;aspect-ratio:9/16;background:#000;border-radius:8px;overflow:hidden;border:1px solid var(--border);position:relative">' +
           '<video controls preload="metadata" src="' + esc(vidSrc) + '" style="width:100%;height:100%;aspect-ratio:9/16;object-fit:contain;display:block"></video>' +
           '</div></div>';
    }
    if (!imgSrc && !vidSrc && preview) {
      h += '<div style="margin:6px 0"><a href="' + esc(preview) + '" target="_blank" class="btn btn-secondary btn-sm" style="font-size:11px;padding:3px 8px">Xem file ' + esc(preview) + '</a></div>';
    }
    return h;
  }

  function refreshOpenBoard(id) {
    var box = document.querySelector('[data-job="' + id + '"] .board');
    if (!box || !boards[id]) return;
    fetch('/studio/jobs/' + id, {headers: {'Accept': 'application/json'}})
      .then(function (r) { return r.json(); })
      .then(function (d) {
        if (!boards[id]) return;
        var logEl = document.getElementById('log-' + id);
        if (logEl && d.job && d.job.log) {
          if (logEl.textContent !== d.job.log) logEl.textContent = d.job.log;
          logEl.hidden = false;
        }
        var grid = box.querySelector('.board-shots-grid');
        if (!grid || !d.assets || !d.assets.length) {
          populateBoard(id, box, d);
          return;
        }
        d.assets.forEach(function (a) {
          var pid = 'prompt-' + id + '-' + a.id;
          (window.__shotPrompts = window.__shotPrompts || {})[pid] = a.prompt || '';
          var card = grid.querySelector('[data-asset-id="' + a.id + '"]');
          if (!card) {
            grid.insertAdjacentHTML('beforeend', renderShotCardHTML(id, a));
            return;
          }
          var badge = card.querySelector('.shot-status-badge');
          if (badge) {
            badge.className = 'badge shot-status-badge badge-' + a.class;
            badge.textContent = a.label;
          }
          var hasImg = a.image_url || (a.preview && /\.(png|jpe?g|webp)$/i.test(a.preview));
          var hasVid = a.video_url || (a.preview && /\.(mp4|mov|webm)$/i.test(a.preview));
          var imgSrc = a.image_url || (hasImg ? a.preview : '');
          var vidSrc = a.video_url || (hasVid ? a.preview : '');
          var slot = card.querySelector('.shot-media-slot');
          if (slot) {
            var curImg = slot.querySelector('.shot-img-wrap');
            var curVid = slot.querySelector('.shot-vid-wrap');
            var sameImg = (!imgSrc && !curImg) || (curImg && curImg.getAttribute('data-src') === imgSrc);
            var sameVid = (!vidSrc && !curVid) || (curVid && curVid.getAttribute('data-src') === vidSrc);
            if (!sameImg || !sameVid) {
              slot.innerHTML = renderShotMediaInner(imgSrc, vidSrc, a.preview);
            }
          }
        });
      })
      .catch(function () {});
  }

  function populateBoard(id, box, d) {
    var h = '';
    var logEl = document.getElementById('log-' + id);
    if (logEl && d.job && d.job.log) { logEl.textContent = d.job.log; logEl.hidden = false; }
    if (d.trend) {
      h += '<div class="shot"><div class="shot-head">📈 Sẵn sàng lên xu hướng: <b>' + d.trend.score + '/' + d.trend.total + '</b></div><ul class="clean">';
      (d.trend.checks || []).forEach(function (c) {
        h += '<li>' + (c.ok ? '✅ ' : '⚠️ ') + esc(c.label) + (c.ok ? '' : ' — <span class="hint">' + esc(c.tip) + '</span>') + '</li>';
      });
      h += '</ul><p class="hint">Checklist yếu tố phân phối đã biết — không phải cam kết viral.</p></div>';
    }
    if (d.job && d.job.caption) {
      h += '<div class="shot"><div class="shot-head">📝 Chú thích + hashtag ' +
        '<button class="btn btn-secondary btn-sm" onclick="navigator.clipboard.writeText(' +
        'document.getElementById(\'cap-' + id + '\').textContent)">Sao chép</button></div>' +
        '<div class="hint" id="cap-' + id + '">' + esc(d.job.caption).replace(/\n/g, '<br>') + '</div></div>';
    }
    if (d.assets && d.assets.length > 0) {
      h += '<div class="board-shots-grid" style="margin-top:12px">';
      d.assets.forEach(function (a) {
        h += renderShotCardHTML(id, a);
      });
      h += '</div>';
    }
    box.innerHTML = h || '<p class="empty">Chưa có cảnh nào.</p>';
  }

  window.toggleBoard = function (id) {
    var box = document.querySelector('[data-job="' + id + '"] .board');
    if (!box) return;
    if (boards[id]) { box.innerHTML = ''; boards[id] = false; return; }
    boards[id] = true;
    fetch('/studio/jobs/' + id, {headers: {'Accept': 'application/json'}})
      .then(function (r) { return r.json(); })
      .then(function (d) {
        if (!boards[id]) return;
        populateBoard(id, box, d);
      });
  };

  // Cập nhật trạng thái + tiến độ job tại chỗ (không tải lại trang).
  (function () {
    // Chỉ poll card trong khu vực đang xử lý. Tab hoàn tất không có live cards.
    var cards = document.querySelectorAll('.studio-live-job[data-job]');
    if (!cards.length) return;
    var pending = Object.create(null);
    Array.prototype.forEach.call(cards, function (card) {
      pending[card.getAttribute('data-job')] = card;
    });
    var timer = setInterval(function () {
      var query = Object.keys(pending).map(function (id) {
        return 'id=' + encodeURIComponent(id);
      }).join('&');
      fetch('/studio/jobs?' + query, {headers: {'Accept': 'application/json'}})
        .then(function (r) { return r.json(); })
        .then(function (d) {
          (d.jobs || []).forEach(function (j) {
            var card = pending[j.id];
            if (!card) return;
            var b = card.querySelector('.job-status');
            if (b) { b.className = 'badge job-status badge-' + j.class; b.textContent = j.label; }
            var p = card.querySelector('.prog');
            if (p) p.textContent = j.progress + '%';
            var progBar = card.querySelector('div[style*="linear-gradient"]');
            if (progBar) progBar.style.width = j.progress + '%';
            if (j.output && !card.querySelector('video.player')) {
              var v = document.createElement('video');
              v.className = 'player'; v.controls = true; v.src = '/media/' + j.output;
              var btn = card.querySelector('button');
              card.insertBefore(v, btn);
            }
            if (j.status === 'done' || j.status === 'failed') {
              var cancelForm = card.querySelector('form[action$="/cancel"]');
              if (cancelForm) cancelForm.remove();
              delete pending[j.id];
            }
            if (boards[j.id]) { refreshOpenBoard(j.id); }
          });
          if (Object.keys(pending).length === 0) clearInterval(timer);
        })
        .catch(function () { /* mạng chập chờn: giữ nguyên trạng thái hiện tại */ });
    }, 5000);
  })();
})();
