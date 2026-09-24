// Dropdown di dalam hero yang overflow:hidden (mis. tombol Download pada
// halaman pemusnahan) tidak boleh terpotong oleh batas hero. Dengan
// data-bs-display="static" Bootstrap tidak menulis inline positioning, jadi
// popup diposisikan manual sebagai position:fixed dekat tombol — selalu
// di dalam viewport (tidak turun melewati batas bawah hero).
//
// Catatan: event `show.bs.dropdown` Bootstrap memancarkan `relatedTarget`
// = elemen pemicu (tombol), BUKAN menu → cari menu via .dropdown scope.
(function () {
  'use strict';

  function place(btn, menu) {
    const b = btn.getBoundingClientRect();
    const mw = menu.offsetWidth || 140;
    const mh = menu.offsetHeight || 96;
    let left = b.right - mw;
    let top = b.bottom + 4;
    if (left < 8) left = 8;
    if (left + mw > window.innerWidth - 8) left = window.innerWidth - mw - 8;
    if (top + mh > window.innerHeight - 8) {
      top = b.top - mh - 4;
      if (top < 8) top = 8;
    }
    menu.style.left = left + 'px';
    menu.style.top = top + 'px';
    menu.style.margin = '0';
    menu.style.transform = 'none';
  }

  function menuFor(btn) {
    if (!btn || !btn.matches('[data-bs-toggle="dropdown"]')) return null;
    const scope = btn.closest('.dropdown');
    return scope ? scope.querySelector('.dropdown-menu') : null;
  }

  function onToggle(e) {
    const btn = e.target;
    const menu = menuFor(btn);
    if (menu && menu.classList.contains('hero-dropdown-menu')) {
      place(btn, menu);
    }
  }

  document.addEventListener('show.bs.dropdown', onToggle);

  // pasang ulang setelah .show aktif agar memakai ukuran menu yang sebenarnya
  document.addEventListener('shown.bs.dropdown', onToggle);
})();