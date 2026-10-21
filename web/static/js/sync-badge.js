/**
 * sync-badge.js — indikator status sinkronisasi offline-first (mode desktop).
 *
 * • Menanyakan /api/sync/status setiap ~15 detik (+ saat koneksi browser
 *   berubah online/offline dan setelah tombol ditekan).
 * • Menyembunyikan diri secara permanen selama mode sinkronisasi tidak aktif
 *   (server MySQL biasa) sehingga build web sama sekali tidak terpengaruh.
 * • Klik badge = picu sinkronisasi segera (POST /api/sync/run).
 */
(function () {
  'use strict';

  var slot = null;
  var started = false;
  var hiddenForever = false;

  function fmtTime(iso) {
    if (!iso) return '';
    try {
      var d = new Date(iso);
      return d.toLocaleTimeString('id-ID', { hour: '2-digit', minute: '2-digit' });
    } catch (e) {
      return '';
    }
  }

  function pickColor(st) {
    if (st.running) return '#f59e0b';           // amber — siklus berjalan
    if (st.last_error) return '#ef4444';        // merah  — error
    if (st.online) return '#22c55e';            // hijau — online & sinkron
    return '#94a3b8';                           // abu — offline, tetap bekerja
  }

  function render(st) {
    if (!slot) return;
    var dot = '<span style="width:8px;height:8px;border-radius:50%;background:' + pickColor(st) + ';display:inline-block;flex-shrink:0"></span>';
    var label;
    if (st.running) {
      label = 'Sinkron…';
    } else if (!st.online) {
      label = 'Offline';
    } else {
      label = 'Online';
    }
    var extra = '';
    if (st.pending > 0) {
      extra = ' · ' + st.pending + ' antre';
    } else if (st.online && st.last_sync) {
      extra = ' · ' + fmtTime(st.last_sync);
    }
    slot.innerHTML =
      '<button id="syncStatusBadge" type="button" class="nav-icon-btn" style="display:inline-flex;align-items:center;gap:7px;padding:6px 12px;border-radius:10px;height:auto" aria-label="Status sinkronisasi">' +
        dot +
        '<span style="font-size:12px;font-weight:600;color:var(--ink-2)">' + label + extra + '</span>' +
      '</button>';

    var title = 'Sinkronisasi otomatis';
    if (st.online) {
      title = 'Terhubung ke pusat — terakhir sync ' + fmtTime(st.last_sync);
    } else if (st.pending > 0) {
      title = st.pending + ' perubahan menunggu dikirim saat koneksi pulih';
    } else {
      title = 'Pusat tidak terjangkau — aplikasi berjalan normal dari data lokal';
    }
    if (st.last_error) {
      title += '\nError terakhir: ' + st.last_error;
    }
    if (st.pending > 0 && st.online) {
      title += '\n' + st.pending + ' perubahan menunggu siklus berikutnya';
    }
    document.getElementById('syncStatusBadge').title = title;
  }

  function hide() {
    if (slot) slot.innerHTML = '';
  }

  function poll() {
    if (hiddenForever) return;
    fetch('/api/sync/status', { headers: { 'Accept': 'application/json' }, credentials: 'same-origin' })
      .then(function (r) { return r.ok ? r.json() : Promise.reject(r.status); })
      .then(function (st) {
        if (!st || st.enabled === false) {
          hiddenForever = true;
          hide();
          return;
        }
        render(st);
      })
      .catch(function () {
        // Server lokal aplikasi sendiri tidak seharusnya gagal; coba lagi nanti.
        hide();
      });
  }

  function schedule() {
    if (started) return;
    started = true;
    poll();
    setInterval(poll, 15000);
  }

  function bindSlot() {
    slot = document.getElementById('syncStatusSlot');
    if (!slot) return;
    slot.addEventListener('click', function () {
      slot.innerHTML =
        '<button type="button" class="nav-icon-btn" style="padding:6px 12px;border-radius:10px;height:auto">' +
          '<span style="font-size:12px;font-weight:600;color:var(--ink-2)">Sinkron…</span>' +
        '</button>';
      fetch('/api/sync/run', { method: 'POST', credentials: 'same-origin' })
        .catch(function () { /* status berikutnya akan menunjukkan error */ })
        .finally(function () { setTimeout(poll, 1200); });
    });
    schedule();
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', bindSlot, { once: true });
  } else {
    bindSlot();
  }
})();
