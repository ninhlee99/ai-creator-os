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

  // ------------------------------------------------------ modal xác nhận
  // Form có data-confirm="..." sẽ hiện modal thay vì confirm() của trình
  // duyệt; chỉ submit khi người dùng bấm Xác nhận.
  var modal = null, modalMsg = null, pendingForm = null;
  function ensureModal() {
    if (modal) return;
    modal = document.getElementById('confirm-modal');
    modalMsg = document.getElementById('confirm-modal-msg');
    if (!modal || !modalMsg) { modal = null; return; }
    document.getElementById('confirm-modal-cancel').addEventListener('click', function () {
      modal.hidden = true;
      pendingForm = null;
    });
    modal.addEventListener('click', function (ev) {
      if (ev.target === modal) { modal.hidden = true; pendingForm = null; }
    });
    document.getElementById('confirm-modal-ok').addEventListener('click', function () {
      modal.hidden = true;
      var f = pendingForm;
      pendingForm = null;
      if (f) { f._aicosConfirmed = true; f.requestSubmit(); }
    });
    document.addEventListener('keydown', function (ev) {
      if (ev.key === 'Escape' && modal && !modal.hidden) {
        modal.hidden = true;
        pendingForm = null;
      }
    });
  }
  document.addEventListener('submit', function (ev) {
    var f = ev.target;
    if (!f || !f.getAttribute) return;
    var msg = f.getAttribute('data-confirm');
    if (!msg || f._aicosConfirmed) return;
    ev.preventDefault();
    ensureModal();
    if (!modal) { // không có modal trong trang: submit thẳng
      f._aicosConfirmed = true;
      f.requestSubmit();
      return;
    }
    modalMsg.textContent = msg;
    pendingForm = f;
    modal.hidden = false;
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
      videoModalFrame.style.aspectRatio = '16 / 9';
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
    }
    if (c) c.addEventListener('click', close);
    m.addEventListener('click', function (e) { if (e.target === m) close(); });
    document.addEventListener('keydown', function (e) {
      if (e.key === 'Escape' && !m.hidden) close();
    });
  })();

  // ------------------------------------------------------ sidebar active & drawer
  (function () {
    var p = location.pathname;
    var bestMatch = null;
    var bestLen = -1;
    document.querySelectorAll('.sidebar nav a').forEach(function (a) {
      var h = a.getAttribute('href');
      if (!h) return;
      var clean = h.split('?')[0];
      if (clean === '/workflow' && (p === '/workflow' || p === '/flow' || p === '/studio/flow' || p === '/studio/workflow')) {
        bestMatch = a;
        bestLen = 9999;
      } else if (clean === p) {
        bestMatch = a;
        bestLen = 9999;
      } else if (clean !== '/' && p.indexOf(clean) === 0 && clean.length > bestLen) {
        bestMatch = a;
        bestLen = clean.length;
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
  })();

  // ----------------------------------------------------- emoji spacing
  // Chrome/macOS vẽ khoảng trắng ngay sau emoji bằng font emoji (gần như
  // rộng 0) → "📖Kể chuyện". Bọc emoji vào span.ico có lề phải, bỏ khoảng
  // trắng thừa. Bỏ qua ô nhập/code để không đổi dữ liệu người dùng.
  var isMutatingEmoji = false;
  function spaceEmoji(root) {
    if (isMutatingEmoji || !root) return;
    var re = /(\p{Extended_Pictographic}(?:\uFE0F|\u200D\p{Extended_Pictographic})*\uFE0F?)[ \u00A0]+(?=\S)/u;
    var walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
      acceptNode: function (n) {
        var p = n.parentNode;
        if (!p || /^(SCRIPT|STYLE|TEXTAREA|CODE|PRE|OPTION|INPUT|VIDEO|AUDIO)$/.test(p.nodeName) ||
            (p.classList && p.classList.contains('ico'))) return NodeFilter.FILTER_REJECT;
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
  if (window.MutationObserver) {
    new MutationObserver(function (muts) {
      if (isMutatingEmoji) return;
      muts.forEach(function (mu) {
        mu.addedNodes.forEach(function (n) {
          if (n.nodeType === 1 && !isMutatingEmoji) spaceEmoji(n);
        });
      });
    }).observe(document.body, { childList: true, subtree: true });
  }

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

  window.toggleBoard = function (id) {
    var box = document.querySelector('[data-job="' + id + '"] .board');
    if (!box) return;
    if (boards[id]) { box.innerHTML = ''; boards[id] = false; return; }
    boards[id] = true;
    fetch('/studio/jobs/' + id, {headers: {'Accept': 'application/json'}})
      .then(function (r) { return r.json(); })
      .then(function (d) {
        var h = '';
        var logEl = document.getElementById('log-' + id);
        if (d.job && d.job.log) { logEl.textContent = d.job.log; logEl.hidden = false; }
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
            var name;
            if (a.kind === 'clip') name = 'Cảnh ' + (a.idx + 1);
            else name = a.kind + ' ' + (a.idx + 1);
            var pid = 'prompt-' + id + '-' + a.id;
            (window.__shotPrompts = window.__shotPrompts || {})[pid] = a.prompt || '';
            h += '<div class="shot board-shot-card" style="background:var(--bg3);border:1px solid var(--border);border-radius:10px;padding:10px;display:flex;flex-direction:column;justify-content:space-between"><div class="shot-head" style="display:flex;justify-content:space-between;align-items:center;margin-bottom:6px"><b style="font-size:12px">' + name +
              '</b> <span class="badge badge-' + a.class + '" style="font-size:9.5px">' + a.label + '</span>' +
              '</div>';

            var hasImg = a.image_url || (a.preview && /\.(png|jpe?g|webp)$/i.test(a.preview));
            var hasVid = a.video_url || (a.preview && /\.(mp4|mov|webm)$/i.test(a.preview));
            var imgSrc = a.image_url || (hasImg ? a.preview : '');
            var vidSrc = a.video_url || (hasVid ? a.preview : '');

            if (imgSrc) {
              h += '<div style="margin:4px 0">' +
                   '<div style="font-size:10px;font-weight:700;color:var(--muted);margin-bottom:3px">🖼️ Keyframe:</div>' +
                   '<div style="width:100%;aspect-ratio:9/16;background:#000;border-radius:8px;overflow:hidden;border:1px solid var(--border);position:relative">' +
                   '<img src="' + esc(imgSrc) + '" style="width:100%;height:100%;object-fit:contain;display:block;cursor:pointer" onclick="window.open(\'' + esc(imgSrc) + '\', \'_blank\')" title="Bấm để xem ảnh gốc 4K">' +
                   '</div></div>';
            }
            if (vidSrc) {
              h += '<div style="margin:4px 0">' +
                   '<div style="font-size:10px;font-weight:700;color:var(--muted);margin-bottom:3px">🎬 Video AI:</div>' +
                   '<div style="width:100%;aspect-ratio:9/16;background:#000;border-radius:8px;overflow:hidden;border:1px solid var(--border);position:relative">' +
                   '<video controls preload="metadata" src="' + esc(vidSrc) + '" style="width:100%;height:100%;aspect-ratio:9/16;object-fit:contain;display:block"></video>' +
                   '</div></div>';
            }
            if (!imgSrc && !vidSrc && a.preview) {
              h += '<div style="margin:6px 0"><a href="' + esc(a.preview) + '" target="_blank" class="btn btn-secondary btn-sm" style="font-size:11px;padding:3px 8px">Xem file ' + esc(a.preview) + '</a></div>';
            }

            h += '<div class="hint" id="' + pid + '" style="display:none;white-space:pre-wrap;max-height:160px;overflow:auto;margin:6px 0;background:var(--bg);padding:6px;border-radius:6px;font-size:11px"></div>';
            h += '<div style="margin-top:6px"><button type="button" class="btn btn-secondary btn-sm" onclick="togglePrompt(\'' + pid + '\')" style="font-size:10.5px;padding:3px 8px">Xem prompt</button>';
            h += '</div></div>';
          });
          h += '</div>';
        }
        box.innerHTML = h || '<p class="empty">Chưa có cảnh nào.</p>';
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
            if (boards[j.id]) { boards[j.id] = false; toggleBoard(j.id); }
          });
          if (Object.keys(pending).length === 0) clearInterval(timer);
        })
        .catch(function () { /* mạng chập chờn: giữ nguyên trạng thái hiện tại */ });
    }, 5000);
  })();
})();
