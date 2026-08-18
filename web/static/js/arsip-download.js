/**
 * SIMARC — Arsip Digital Download Guard
 *
 * Intercepts clicks on digital-file download buttons for archives and their
 * file versions:
 *   · /arsip/:id/download           → checked via GET /arsip/:id/check-file
 *   · /arsip/version/:id/download   → checked via GET /arsip/version/:id/check-file
 *   · /arsip/:id/versions/:vid/download → checked via GET /arsip/version/:vid/check-file
 *
 * Before letting the browser start the download, the server is asked whether
 * the digital file actually exists. If the file is missing, a warning modal
 * "File Belum Diupload" is shown instead of navigating to a bare 404 page.
 *
 * Covers every page that renders an arsip download link (detail, list, search,
 * laporan digital, versions, history) via a single delegated listener — no
 * per-button wiring needed. Dynamically rendered links are covered too.
 *
 * Dependencies: window.SimarcModal (web/static/js/modal-manager.js) — loaded
 * before this file in layouts/app.html.
 */
(function () {
  'use strict';

  // Matches the digital-file and version download links, capturing the id.
  // QR downloads (/arsip/{id}/qrcode/download) and the template download
  // (/arsip/download-template) are excluded.
  var DOWNLOAD_RE = /^\/arsip\/([^/]+)\/download$/;
  var VERSION_DOWNLOAD_RE = /^\/arsip\/version\/([^/]+)\/download$/;
  var ALT_VERSION_DOWNLOAD_RE = /^\/arsip\/[^/]+\/versions\/([^/]+)\/download$/;
  var CHECK_TIMEOUT_MS = 6000;

  /**
   * Return the check-file endpoint for a download href, or null if the link is
   * not one this guard handles.
   */
  function getCheckEndpoint(href) {
    var m = DOWNLOAD_RE.exec(href);
    if (m) return { checkUrl: '/arsip/' + m[1] + '/check-file', version: false };
    m = VERSION_DOWNLOAD_RE.exec(href);
    if (m) return { checkUrl: '/arsip/version/' + m[1] + '/check-file', version: true };
    m = ALT_VERSION_DOWNLOAD_RE.exec(href);
    if (m) return { checkUrl: '/arsip/version/' + m[1] + '/check-file', version: true };
    return null;
  }

  function showFileMissingModal(version) {
    if (typeof SimarcModal === 'undefined' || !SimarcModal.confirm) return false;
    SimarcModal.confirm({
      title: 'File Belum Diupload',
      message: version
        ? 'File digital pada versi ini belum diupload.'
        : 'Arsip ini belum memiliki file digital. Silakan upload file terlebih dahulu melalui menu Edit.',
      icon: 'exclamation-triangle-fill',
      iconColor: 'warning',
      confirmText: 'Mengerti',
      cancelText: false
    });
    return true;
  }

  function proceedToDownload(link) {
    var href = link.getAttribute('href');
    if (href) window.location.href = href;
  }

  function checkAndDownload(link, checkUrl, version) {
    var controller = typeof AbortController !== 'undefined' ? new AbortController() : null;
    var timer = controller ? setTimeout(function () { controller.abort(); }, CHECK_TIMEOUT_MS) : null;

    var opts = { headers: { 'Accept': 'application/json' }, credentials: 'same-origin' };
    if (controller) opts.signal = controller.signal;

    fetch(checkUrl, opts)
      .then(function (res) { return res.json(); })
      .then(function (data) {
        if (data && data.exists) {
          proceedToDownload(link);
        } else if (!showFileMissingModal(version)) {
          // Modal unavailable — never block a legitimate download silently.
          proceedToDownload(link);
        }
      })
      .catch(function () {
        // Network/server failure or timeout — fall back to the normal download
        // behaviour so the feature never blocks a legit download.
        proceedToDownload(link);
      })
      .finally(function () {
        if (timer) clearTimeout(timer);
        link._smDownloadChecking = false;
      });
  }

  document.addEventListener('click', function (e) {
    // Plain left-clicks only (no modifier keys, no middle/right button).
    if (e.button !== 0) return;
    if (e.ctrlKey || e.metaKey || e.shiftKey || e.altKey) return;

    var link = e.target && e.target.closest ? e.target.closest('a[href]') : null;
    if (!link) return;

    var href = link.getAttribute('href');
    var check = getCheckEndpoint(href);
    if (!check) return;

    // Guard against rapid double-clicks while the check is still pending.
    if (link._smDownloadChecking) return;
    link._smDownloadChecking = true;

    e.preventDefault();
    checkAndDownload(link, check.checkUrl, check.version);
  });
})();
