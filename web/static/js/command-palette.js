/* ═══════════════════════════════════════════════════════════════════
   SIMARC — Command Palette (⌘K / Ctrl+K)
   ───────────────────────────────────────────────────────────────────
   Tulang navigasi ala aplikasi desktop kelas dunia (Linear, Raycast,
   Vercel, Notion): buka cepat, cari, lompat, jalankan aksi — semua dari
   keyboard. Markup ada di components/command-palette.html.
   ═══════════════════════════════════════════════════════════════════ */
(function () {
  'use strict';

  var root = document.getElementById('cmdk');
  if (!root) return;

  var input = document.getElementById('cmdkInput');
  var list = document.getElementById('cmdkList');
  var trigger = document.getElementById('cmdkTrigger');
  var RECENT_KEY = 'simarc.cmdk.recent';
  var isMac = /Mac|iPhone|iPad|iPod/.test(navigator.platform || navigator.userAgent || '');

  /* ── Data: navigasi (cermin dari topnav) ───────────────────────── */
  var NAV_ITEMS = [
    { id: 'nav:/dashboard',           label: 'Dashboard',              href: '/dashboard',           icon: 'bi-speedometer2',       section: 'Navigasi', keywords: 'beranda home utama' },
    { id: 'nav:/arsip',               label: 'Daftar Arsip',           href: '/arsip',               icon: 'bi-file-earmark-text',  section: 'Navigasi', keywords: 'arsip dokumen daftar' },
    { id: 'nav:/arsip/create',        label: 'Tambah Arsip',           href: '/arsip/create',        icon: 'bi-plus-circle',        section: 'Navigasi', keywords: 'baru input buat arsip' },
    { id: 'nav:/pemberkasan',         label: 'Pemberkasan',            href: '/pemberkasan',         icon: 'bi-folder-plus',        section: 'Navigasi', keywords: 'berkas folder' },
    { id: 'nav:/arsip/pemindahan',    label: 'Pemindahan Arsip',       href: '/arsip/pemindahan',    icon: 'bi-map',                section: 'Navigasi', keywords: 'pindah lokasi' },
    { id: 'nav:/pemusnahan',          label: 'Pemusnahan Arsip',       href: '/pemusnahan',          icon: 'bi-trash',              section: 'Navigasi', keywords: 'musnah hapus' },
    { id: 'nav:/peminjaman/create',   label: 'Permohonan Pinjam',      href: '/peminjaman/create',   icon: 'bi-file-earmark-arrow-up', section: 'Navigasi', keywords: 'pinjam permohonan baru' },
    { id: 'nav:/peminjaman',          label: 'Daftar Peminjaman',      href: '/peminjaman',          icon: 'bi-journal-check',      section: 'Navigasi', keywords: 'pinjam layanan' },
    { id: 'nav:/search',              label: 'Pencarian Arsip',        href: '/search',              icon: 'bi-search',             section: 'Navigasi', keywords: 'cari search' },
    { id: 'nav:/unit-kerja',          label: 'Unit Kerja',             href: '/unit-kerja',          icon: 'bi-building',           section: 'Navigasi', keywords: 'master data unit' },
    { id: 'nav:/lokasi-arsip',        label: 'Lokasi Arsip',           href: '/lokasi-arsip',        icon: 'bi-geo-alt',            section: 'Navigasi', keywords: 'master data lokasi rak' },
    { id: 'nav:/kode-klasifikasi',    label: 'Kode Klasifikasi',       href: '/kode-klasifikasi',    icon: 'bi-tags',               section: 'Navigasi', keywords: 'master data klasifikasi kode' },
    { id: 'nav:/jenis-arsip',         label: 'Jenis Arsip',            href: '/jenis-arsip',         icon: 'bi-folder',             section: 'Navigasi', keywords: 'master data jenis' },
    { id: 'nav:/users',               label: 'Petugas',                href: '/users',               icon: 'bi-people',             section: 'Navigasi', keywords: 'user pengguna petugas' },
    { id: 'nav:/roles',               label: 'Manajemen Role',         href: '/roles',               icon: 'bi-person-badge',       section: 'Navigasi', keywords: 'role hak akses' },
    { id: 'nav:/laporan',             label: 'Dashboard Laporan',      href: '/laporan',             icon: 'bi-graph-up',           section: 'Navigasi', keywords: 'laporan report' },
    { id: 'nav:/laporan/arsip',       label: 'Laporan Arsip',          href: '/laporan/arsip',       icon: 'bi-file-earmark-text',  section: 'Navigasi', keywords: 'laporan arsip' },
    { id: 'nav:/laporan/digital',     label: 'Laporan Arsip Digital',  href: '/laporan/digital',     icon: 'bi-cloud-check',        section: 'Navigasi', keywords: 'laporan digital' },
    { id: 'nav:/laporan/pemberkasan', label: 'Laporan Pemberkasan',    href: '/laporan/pemberkasan', icon: 'bi-folder',             section: 'Navigasi', keywords: 'laporan berkas' },
    { id: 'nav:/laporan/lokasi',      label: 'Laporan per Lokasi',     href: '/laporan/lokasi',      icon: 'bi-geo-alt',            section: 'Navigasi', keywords: 'laporan lokasi' },
    { id: 'nav:/laporan/klasifikasi', label: 'Laporan Klasifikasi',    href: '/laporan/klasifikasi', icon: 'bi-tags',               section: 'Navigasi', keywords: 'laporan klasifikasi' },
    { id: 'nav:/laporan/retensi',     label: 'Laporan Retensi',        href: '/laporan/retensi',     icon: 'bi-hourglass-split',    section: 'Navigasi', keywords: 'laporan retensi' },
    { id: 'nav:/laporan/pemusnahan',  label: 'Laporan Pemusnahan',     href: '/laporan/pemusnahan',  icon: 'bi-trash',              section: 'Navigasi', keywords: 'laporan musnah' },
    { id: 'nav:/laporan/aktivitas',   label: 'Log Aktivitas',          href: '/laporan/aktivitas',   icon: 'bi-clock-history',      section: 'Navigasi', keywords: 'log aktivitas audit' },
    { id: 'nav:/laporan/statistik',   label: 'Statistik',              href: '/laporan/statistik',   icon: 'bi-pie-chart',          section: 'Navigasi', keywords: 'statistik grafik' },
    { id: 'nav:/monitoring/retensi',  label: 'Monitoring Retensi',     href: '/monitoring/retensi',  icon: 'bi-clock',              section: 'Navigasi', keywords: 'monitoring retensi' },
    { id: 'nav:/jadwal-retensi',      label: 'Jadwal Retensi',         href: '/jadwal-retensi',      icon: 'bi-calendar-check',     section: 'Navigasi', keywords: 'jadwal retensi' },
    { id: 'nav:/disposal',            label: 'Dashboard Penyusutan',   href: '/disposal',            icon: 'bi-arrow-right-left',   section: 'Navigasi', keywords: 'penyusutan disposal' },
    { id: 'nav:/backup',              label: 'Backup & Restore',       href: '/backup',              icon: 'bi-database',           section: 'Navigasi', keywords: 'backup restore database' },
    { id: 'nav:/blockchain',          label: 'Blockchain Audit',       href: '/blockchain',          icon: 'bi-shield-check',       section: 'Navigasi', keywords: 'blockchain audit' },
    { id: 'nav:/pengaturan',          label: 'Pengaturan',             href: '/pengaturan',          icon: 'bi-gear',               section: 'Navigasi', keywords: 'setting pengaturan' },
    { id: 'nav:/profil',              label: 'Profil Saya',            href: '/profil',              icon: 'bi-person-circle',      section: 'Navigasi', keywords: 'profil akun saya' }
  ];

  /* ── Data: aksi cepat ──────────────────────────────────────────── */
  var ACTION_ITEMS = [
    { id: 'act:theme',     label: 'Ganti Tema (Terang / Gelap)', icon: 'bi-moon-stars',    section: 'Aksi', keywords: 'tema dark light mode gelap',
      action: function () { var t = document.getElementById('themeToggle'); if (t) t.click(); } },
    { id: 'act:shortcuts', label: 'Pintasan Keyboard',           icon: 'bi-keyboard',      section: 'Aksi', keywords: 'pintasan keyboard shortcut',
      action: function () { if (window.SimarcModal && SimarcModal.show) SimarcModal.show('kbdModal'); } },
    { id: 'act:backup',    label: 'Buka Backup & Restore',       icon: 'bi-database-check', section: 'Aksi', keywords: 'backup restore',
      action: function () { window.location.href = '/backup'; } },
    { id: 'act:reload',    label: 'Muat Ulang Halaman',          icon: 'bi-arrow-clockwise', section: 'Aksi', keywords: 'reload refresh muat ulang',
      action: function () { window.location.reload(); } },
    { id: 'act:logout',    label: 'Keluar',                      icon: 'bi-box-arrow-right', section: 'Aksi', keywords: 'logout keluar sign out',
      action: function () { var f = document.getElementById('logout-form'); if (f) f.submit(); } }
  ];

  var ALL = NAV_ITEMS.concat(ACTION_ITEMS);
  var BY_ID = {};
  ALL.forEach(function (it) { BY_ID[it.id] = it; });

  var flat = [];
  var activeIndex = 0;

  /* ── Pencarian live ke database (omnibox Ctrl+K) ───────────────── */
  var DATA_ICON = {
    'arsip': 'bi-file-earmark-text',
    'kode-klasifikasi': 'bi-tags',
    'unit-kerja': 'bi-building',
    'lokasi': 'bi-geo-alt'
  };
  var remote = { q: '', items: [], timer: 0, seq: 0 };

  function clearRemote() { remote.q = ''; remote.items = []; }

  function fetchRemote(q) {
    var seq = ++remote.seq;
    fetch('/api/cmdk/search?q=' + encodeURIComponent(q), {
      headers: { 'Accept': 'application/json' },
      credentials: 'same-origin'
    })
      .then(function (r) { return r.ok ? r.json() : { data: [] }; })
      .then(function (j) {
        if (seq !== remote.seq) return;        // respons usang
        if (input.value.trim() !== q) return;  // query sudah berubah
        remote.q = q;
        remote.items = ((j && j.data) || []).map(function (r) {
          return {
            id: 'data:' + r.type + ':' + (r.id || r.url),
            label: r.title, hint: r.subtitle,
            icon: DATA_ICON[r.type] || 'bi-database',
            href: r.url
          };
        });
        render(input.value);
      })
      .catch(function () { /* offline / API tidak tersedia: abaikan */ });
  }

  function scheduleRemote(q) {
    clearTimeout(remote.timer);
    if (!q || q.trim().length < 2) { remote.seq++; clearRemote(); return; }
    remote.timer = setTimeout(function () { fetchRemote(q.trim()); }, 180);
  }

  /* ── Util ─────────────────────────────────────────────────────── */
  function esc(s) {
    return String(s).replace(/[&<>"']/g, function (c) {
      return { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c];
    });
  }

  function score(item, q) {
    var hay = (item.label + ' ' + (item.keywords || '')).toLowerCase();
    var tokens = q.toLowerCase().split(/\s+/).filter(Boolean);
    if (!tokens.length) return 0;
    var total = 0;
    for (var i = 0; i < tokens.length; i++) {
      var t = tokens[i];
      var idx = hay.indexOf(t);
      if (idx >= 0) { total += 100 - Math.min(idx, 90); continue; }
      var ti = 0;
      for (var j = 0; j < hay.length && ti < t.length; j++) { if (hay[j] === t[ti]) ti++; }
      if (ti < t.length) return -1;
      total += 1;
    }
    return total;
  }

  function rank(items, q) {
    return items
      .map(function (it) { return [score(it, q), it]; })
      .filter(function (x) { return x[0] >= 0; })
      .sort(function (a, b) { return b[0] - a[0]; })
      .map(function (x) { return x[1]; });
  }

  function readRecents() {
    try { return JSON.parse(localStorage.getItem(RECENT_KEY) || '[]') || []; } catch (e) { return []; }
  }
  function addRecent(id) {
    if (!id || id === '__search__' || id.indexOf('data:') === 0) return;
    var arr = readRecents().filter(function (x) { return x !== id; });
    arr.unshift(id);
    if (arr.length > 5) arr = arr.slice(0, 5);
    try { localStorage.setItem(RECENT_KEY, JSON.stringify(arr)); } catch (e) { /* ignore */ }
  }
  function recentItems() {
    return readRecents().map(function (id) { return BY_ID[id]; }).filter(Boolean);
  }

  /* ── Render ───────────────────────────────────────────────────── */
  function itemEl(it, idx) {
    var btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'cmdk-item';
    btn.setAttribute('role', 'option');
    btn.setAttribute('aria-selected', 'false');
    btn.dataset.idx = String(idx);
    var hint = it.hint && it.hint.trim() ? '<span class="cmdk-item-hint">' + esc(it.hint) + '</span>' : '';
    btn.innerHTML = '<i class="bi ' + esc(it.icon || 'bi-dot') + '" aria-hidden="true"></i>' +
                    '<span class="cmdk-item-label">' + esc(it.label) + '</span>' + hint;
    btn.addEventListener('mousemove', function () { setActive(idx); });
    btn.addEventListener('click', function (e) { e.preventDefault(); run(it); });
    return btn;
  }

  function render(q) {
    q = (q || '').replace(/^\s+|\s+$/g, '');
    list.innerHTML = '';
    flat = [];
    activeIndex = 0;

    var groups = [];
    if (!q) {
      var rec = recentItems();
      if (rec.length) groups.push(['Terakhir', rec]);
      groups.push(['Navigasi', NAV_ITEMS]);
      groups.push(['Aksi', ACTION_ITEMS]);
    } else {
      var pseudo = { id: '__search__', label: 'Cari arsip: "' + q + '"', icon: 'bi-search',
                     href: '/search?q=' + encodeURIComponent(q), hint: 'Enter' };
      groups.push(['Pencarian', [pseudo]]);
      if (remote.q === q && remote.items.length) groups.push(['Data', remote.items]);
      var nav = rank(NAV_ITEMS, q);
      var act = rank(ACTION_ITEMS, q);
      if (nav.length) groups.push(['Navigasi', nav]);
      if (act.length) groups.push(['Aksi', act]);
    }

    var any = false;
    groups.forEach(function (g) {
      if (!g[1].length) return;
      any = true;
      var lbl = document.createElement('div');
      lbl.className = 'cmdk-group-label';
      lbl.textContent = g[0];
      list.appendChild(lbl);
      g[1].forEach(function (it) {
        var idx = flat.length;
        flat.push(it);
        list.appendChild(itemEl(it, idx));
      });
    });

    if (!any) {
      list.innerHTML = '<div class="cmdk-empty"><i class="bi bi-search"></i>Tidak ada hasil untuk "' + esc(q) + '"</div>';
    }
    setActive(0);
  }

  function setActive(i) {
    if (!flat.length) return;
    if (i < 0) i = flat.length - 1;
    if (i >= flat.length) i = 0;
    activeIndex = i;
    var nodes = list.querySelectorAll('.cmdk-item');
    for (var k = 0; k < nodes.length; k++) {
      var on = k === i;
      nodes[k].setAttribute('aria-selected', on ? 'true' : 'false');
      if (on) nodes[k].scrollIntoView({ block: 'nearest' });
    }
  }

  function move(delta) {
    if (!flat.length) return;
    setActive(activeIndex + delta);
  }

  function run(it) {
    if (!it) return;
    addRecent(it.id);
    close(false);
    if (it.href) { window.location.href = it.href; return; }
    if (typeof it.action === 'function') it.action();
  }

  function activate() { run(flat[activeIndex]); }

  /* ── Buka / tutup ─────────────────────────────────────────────── */
  function open() {
    if (!root.hidden) return;
    root.hidden = false;
    document.documentElement.classList.add('cmdk-open');
    input.value = '';
    render('');
    window.requestAnimationFrame(function () { input.focus(); input.select(); });
  }

  function close(refocus) {
    if (root.hidden) return;
    root.hidden = true;
    document.documentElement.classList.remove('cmdk-open');
    if (refocus !== false && trigger) trigger.focus();
  }

  /* ── Events ───────────────────────────────────────────────────── */
  input.addEventListener('input', function () {
    render(input.value);
    scheduleRemote(input.value);
  });

  input.addEventListener('keydown', function (e) {
    if (e.key === 'ArrowDown') { e.preventDefault(); move(1); }
    else if (e.key === 'ArrowUp') { e.preventDefault(); move(-1); }
    else if (e.key === 'Enter') { e.preventDefault(); activate(); }
    else if (e.key === 'Home') { e.preventDefault(); setActive(0); }
    else if (e.key === 'End') { e.preventDefault(); setActive(flat.length - 1); }
    else if (e.key === 'Tab') { e.preventDefault(); }
  });

  root.addEventListener('mousedown', function (e) {
    if (e.target.closest('[data-cmdk-close]')) { e.preventDefault(); close(); }
  });

  if (trigger) trigger.addEventListener('click', function () { open(); });

  document.addEventListener('keydown', function (e) {
    var key = (e.key || '').toLowerCase();
    if ((e.ctrlKey || e.metaKey) && key === 'k') { e.preventDefault(); open(); return; }
    if (key === 'escape' && !root.hidden) { e.preventDefault(); close(); }
  });

  /* Label kbd sesuai platform */
  var kbdEl = document.getElementById('cmdkKbd');
  if (kbdEl) kbdEl.textContent = isMac ? '\u2318K' : 'Ctrl K';

  /* API publik (dipakai pintasan lain, mis. "/" di layout) */
  window.SimarcCmdK = { open: open, close: close, toggle: function () { if (root.hidden) open(); else close(); } };
})();
