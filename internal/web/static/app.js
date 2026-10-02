/* app.js — JS dùng chung toàn dashboard aicos (R2-W3).
   Một nguồn cho: toast, modal xác nhận, sidebar active, file-picker,
   theme toggle, polling job Studio. Không định nghĩa lại nhãn trạng
   thái — nhãn/class do server render sẵn. */

(function () {
  'use strict';

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

  // ------------------------------------------------------ sidebar active
  (function () {
    var p = location.pathname;
    document.querySelectorAll('.sidebar nav a').forEach(function (a) {
      var h = a.getAttribute('href');
      if (h === p || (h !== '/' && p.indexOf(h) === 0)) a.classList.add('active');
    });
  })();

  // -------------------------------------------------------- file-picker
  document.querySelectorAll('.file-pick input[type=file]').forEach(function (inp) {
    inp.addEventListener('change', function () {
      var name = inp.closest('.file-pick').querySelector('.file-name');
      if (name) name.textContent = inp.files.length ? inp.files[0].name : 'Chưa chọn file';
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
  // Chọn bài nhạc trending vào form affiliate.
  window.pickTrend = function (sel) {
    var v = sel.value.split('|');
    document.getElementById('music_title').value = v[0] || '';
    document.getElementById('music_artist').value = v[1] || '';
  };

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
        if (d.job && d.job.caption) {
          h += '<div class="shot"><div class="shot-head">📝 Chú thích + hashtag ' +
            '<button class="btn btn-secondary btn-sm" onclick="navigator.clipboard.writeText(' +
            'document.getElementById(\'cap-' + id + '\').textContent)">Sao chép</button></div>' +
            '<div class="hint" id="cap-' + id + '">' + d.job.caption.replace(/\n/g, '<br>') + '</div></div>';
        }
        (d.assets || []).forEach(function (a) {
          var name;
          if (a.kind === 'shot') name = 'Shot ' + (a.seq + 1);
          else if (a.kind === 'trailer') name = '📱 Trailer ' + (a.idx - 1999);
          else if (a.kind === 'clip') name = 'Cảnh ' + (a.idx + 1);
          else if (a.kind === 'portrait') name = 'Chân dung';
          else name = a.kind + ' ' + (a.idx + 1);
          var pid = 'prompt-' + id + '-' + a.id;
          (window.__shotPrompts = window.__shotPrompts || {})[pid] = a.prompt || '';
          h += '<div class="shot"><div class="shot-head">' + name +
            ' <span class="badge badge-' + a.class + '">' + a.label + '</span>' +
            (a.method_label ? ' <span class="badge">' + a.method_label + '</span>' : '') +
            (a.trailer ? ' <span class="badge" title="Shot đắt giá — đã dùng cắt trailer">📱 trailer</span>' : '') +
            '</div>';
          if (a.preview) {
            if (a.kind === 'portrait' || a.kind === 'photo') { h += '<img src="' + a.preview + '" style="max-width:220px">'; }
            else { h += '<video controls preload="metadata" src="' + a.preview + '" style="max-width:220px"></video>'; }
          }
          h += '<div class="hint" id="' + pid + '" style="display:none;white-space:pre-wrap;max-height:220px;overflow:auto"></div>';
          h += '<div><button type="button" class="btn btn-secondary btn-sm" onclick="togglePrompt(\'' + pid + '\')">Xem prompt</button>';
          if (a.kind === 'shot' && (a.status === 'done' || a.status === 'failed')) {
            h += ' <form method="post" action="/studio/jobs/' + id + '/shots/' + a.seq + '/rerender" style="display:inline" data-confirm="Quay lại shot ' + (a.seq + 1) + '? Shot này sẽ render lại (tốn chi phí Veo) rồi dựng lại phim.">' +
              '<button class="btn btn-sm">🎬 Quay lại shot này</button></form>';
          }
          h += '</div></div>';
        });
        box.innerHTML = h || '<p class="empty">Chưa có cảnh nào.</p>';
      });
  };

  // Cập nhật trạng thái + tiến độ job tại chỗ (không tải lại trang).
  (function () {
    var cards = document.querySelectorAll('[data-job]');
    if (!cards.length) return;
    var timer = setInterval(function () {
      fetch('/studio/jobs', {headers: {'Accept': 'application/json'}})
        .then(function (r) { return r.json(); })
        .then(function (d) {
          var anyActive = false;
          (d.jobs || []).forEach(function (j) {
            if (j.status === 'running' || j.status === 'queued') anyActive = true;
            var card = document.querySelector('[data-job="' + j.id + '"]');
            if (!card) return;
            var b = card.querySelector('.job-status');
            if (b) { b.className = 'badge job-status badge-' + j.class; b.textContent = j.label; }
            var p = card.querySelector('.prog');
            if (p) p.textContent = j.progress + '%';
            if (j.output && !card.querySelector('video.player')) {
              var v = document.createElement('video');
              v.className = 'player'; v.controls = true; v.src = '/media/' + j.output;
              var btn = card.querySelector('button');
              card.insertBefore(v, btn);
            }
            if (boards[j.id]) { boards[j.id] = false; toggleBoard(j.id); }
          });
          if (!anyActive) clearInterval(timer);
        })
        .catch(function () { /* mạng chập chờn: giữ nguyên trạng thái hiện tại */ });
    }, 5000);
  })();
})();

  // Ước tính chi phí/ETA trên form phim (Film Wave 1 / P0-4): cập nhật theo
  // ô thời lượng. Server render sẵn dòng cho 90s mặc định (no-JS vẫn thấy).
  (function () {
    var sec = document.getElementById('film-seconds');
    var est = document.getElementById('film-est');
    if (!sec || !est) return;
    var rate = parseFloat(est.getAttribute('data-rate') || '0.05') || 0.05;
    function upd() {
      var s = parseInt(sec.value || '90', 10) || 90;
      if (s < 30) s = 30;
      if (s > 3600) s = 3600;
      var shots = Math.ceil(s / 8);
      var cost = (shots * 8 * rate).toFixed(2);
      var eta = shots * 3;
      est.textContent = '≈ ' + shots + ' shot × 8s Veo × $' + rate + '/s ≈ $' + cost +
        ' · render ~' + eta + ' phút (ước tính chưa kiểm chứng)';
    }
    sec.addEventListener('input', upd);
    upd();
  })();
