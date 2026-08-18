/**
 * SIMARC — Modal Manager v4.0
 * Complete redesign — Pure vanilla JS, Portal-based, no Bootstrap dependency
 *
 * Architecture:
 *   All modals rendered inside <div id="sm-portal"> appended to document.body
 *   Modal content is either created dynamically (confirm/toast) or
 *   cloned from inline <template data-modal="id"> elements in the page.
 *
 * Features:
 *   · Portal rendering (modals always at top layer)
 *   · Focus trap (Tab / Shift+Tab)
 *   · Body scroll lock (position:fixed)
 *   · Escape to close topmost modal
 *   · Backdrop click closes modal (unless data-backdrop="static")
 *   · Stacked modals (z-index managed dynamically)
 *   · ARIA: role="dialog", aria-modal, aria-labelledby, aria-describedby
 *   · Custom events: onShow, onShown, onHide, onHidden
 *   · Swipe-to-dismiss on mobile (< 576px)
 *   · Smooth enter/exit animations via CSS
 *   · Memory-leak safe: all handlers cleaned on hide
 *   · Toast notifications
 *   · Confirmation dialog helper
 *   · Loading overlay helper
 */
(function () {
  'use strict';

  /* ─── Z-index constants ─── */
  var Z = {
    MODAL:  1080,
    BACKDROP: 1070,
    TOAST:  9999,
    STACK_STEP: 20,
  };

  /* ─── State ─── */
  var openModals = [];          // stack of modal IDs (top = last)
  var bodyLockCount = 0;
  var scrollBarWidth = 0;
  var _keyHandler = null;
  var _callbacks = { onShow: [], onShown: [], onHide: [], onHidden: [] };

  /* ─── Helpers ─── */

  function getScrollBarWidth() {
    if (!scrollBarWidth) {
      scrollBarWidth = window.innerWidth - document.documentElement.clientWidth;
    }
    return scrollBarWidth;
  }

  function isVisible(el) {
    if (!el || !el.offsetParent) return false;
    var s = window.getComputedStyle(el);
    // Treat unset/empty opacity as fully visible (covers jsdom and some browsers)
    var o = parseFloat(s.opacity);
    return s.display !== 'none' && s.visibility !== 'hidden' && (isNaN(o) || o > 0);
  }

  function getFocusable(el) {
    if (!el) return [];
    var sel = 'a[href]:not([disabled]),button:not([disabled]),input:not([disabled]),' +
      'select:not([disabled]),textarea:not([disabled]),[tabindex]:not([tabindex="-1"]),' +
      'area[href],iframe,object,embed,[contenteditable]';
    return Array.from(el.querySelectorAll(sel)).filter(isVisible);
  }

  /** Minimal HTML escaping */
  function esc(str) {
    if (typeof str !== 'string') return '';
    var d = document.createElement('div');
    d.appendChild(document.createTextNode(str));
    return d.innerHTML;
  }

  /** Fire callbacks */
  function fire(ev, id) {
    (_callbacks[ev] || []).forEach(function (fn) {
      try { fn(id); } catch (e) { console.warn('[SmModal] cb err', ev, e); }
    });
  }

  /* ─── Ensure portal container ─── */
  function ensurePortal() {
    var p = document.getElementById('sm-portal');
    if (!p) {
      p = document.createElement('div');
      p.id = 'sm-portal';
      // Portal sits at the end of <body>, above all page content
      p.style.cssText = 'position:fixed;inset:0;pointer-events:none;z-index:' + Z.MODAL + ';' +
        'overflow:visible;visibility:hidden;'; // visibility hidden until a modal is active
      document.body.appendChild(p);
    }
    return p;
  }

  /* ═══════════════════════════════════════════════════
     BODY SCROLL LOCK
     ═══════════════════════════════════════════════════ */
  function lockBody() {
    bodyLockCount++;
    if (bodyLockCount > 1) return;
    var sbw = getScrollBarWidth();
    var y = window.pageYOffset || document.documentElement.scrollTop;
    document.body._smY = y;
    var css = 'position:fixed;top:-' + y + 'px;left:0;right:0;overflow:hidden;width:100%';
    if (sbw > 0) css += ';padding-right:' + sbw + 'px';
    document.body.style.cssText += css;
  }

  function unlockBody() {
    bodyLockCount--;
    if (bodyLockCount > 0) return;
    bodyLockCount = 0;
    var y = document.body._smY || 0;
    document.body.style.position = '';
    document.body.style.top = '';
    document.body.style.left = '';
    document.body.style.right = '';
    document.body.style.overflow = '';
    document.body.style.width = '';
    document.body.style.paddingRight = '';
    window.scrollTo(0, y);
    delete document.body._smY;
  }

  /* ═══════════════════════════════════════════════════
     FOCUS TRAP
     ═══════════════════════════════════════════════════ */
  function trapFocus(id) {
    var el = document.getElementById(id);
    if (!el) return;

    /** Try to install the trap + initial focus. Returns false while the modal is
     *  still animating in (visibility/opacity transition), which makes isVisible()
     *  reject every focusable element. */
    function install() {
      if (!el.classList.contains('sm-show')) return false;
      var f = getFocusable(el);
      if (!f.length) return false;
      var first = f[0], last = f[f.length - 1];

      function handler(e) {
        if (e.key !== 'Tab') return;
        if (!el.classList.contains('sm-show')) return;
        if (e.shiftKey && document.activeElement === first) {
          e.preventDefault(); last.focus();
        } else if (!e.shiftKey && document.activeElement === last) {
          e.preventDefault(); first.focus();
        }
      }
      el._smFocusTrap = handler;
      document.addEventListener('keydown', handler);

      // Move focus into the modal (first focusable element)
      if (!document.activeElement || !el.contains(document.activeElement)) {
        first.focus();
      }
      return true;
    }

    // Install immediately when the modal is already visible (jsdom & fast paths).
    // Otherwise retry briefly until the enter animation makes the modal visible.
    if (install()) return;

    var attempts = 0;
    var maxAttempts = 40; // ~2s at 50ms — modal enter animation is ~300ms
    var timer = setInterval(function () {
      attempts++;
      if (install() || attempts >= maxAttempts || !el.classList.contains('sm-show')) {
        clearInterval(timer);
      }
    }, 50);
  }

  function releaseFocus(id) {
    var el = document.getElementById(id);
    if (el && el._smFocusTrap) {
      document.removeEventListener('keydown', el._smFocusTrap);
      delete el._smFocusTrap;
    }
  }

  /* ═══════════════════════════════════════════════════
     KEYBOARD (Escape)
     ═══════════════════════════════════════════════════ */
  function initKeyHandler() {
    if (_keyHandler) return;
    _keyHandler = function (e) {
      if (e.key === 'Escape' && openModals.length) {
        var topId = openModals[openModals.length - 1];
        var el = document.getElementById(topId);
        if (el && el.getAttribute('data-backdrop') !== 'static') {
          e.preventDefault();
          hide(topId);
        }
      }
    };
    document.addEventListener('keydown', _keyHandler);
  }

  function destroyKeyHandler() {
    if (_keyHandler) {
      document.removeEventListener('keydown', _keyHandler);
      _keyHandler = null;
    }
  }

  /* ═══════════════════════════════════════════════════
     SWIPE TO DISMISS (mobile)
     ═══════════════════════════════════════════════════ */
  function initSwipe(id) {
    var el = document.getElementById(id);
    if (!el || window.innerWidth > 576) return;
    var content = el.querySelector('.sm-content');
    if (!content || content.closest('.sm-fullscreen,.sm-lg,.sm-xl')) return;

    var startY = 0, currY = 0, dragging = false;
    function ts(e) {
      if (e.target.closest('button,a,input,select,textarea')) return;
      startY = e.touches[0].clientY; dragging = true;
      content.style.transition = 'none';
    }
    function tm(e) {
      if (!dragging) return;
      currY = e.touches[0].clientY;
      var diff = currY - startY;
      if (diff < 0) return;
      var ty = Math.min(diff * 0.5, 150);
      content.style.transform = 'translateY(' + ty + 'px)';
      content.style.opacity = Math.max(1 - diff / 500, 0.5);
    }
    function te() {
      if (!dragging) return;
      dragging = false;
      content.style.transition = '';
      content.style.transform = '';
      content.style.opacity = '';
      if (currY - startY > 80) hide(id);
      startY = currY = 0;
    }
    content.addEventListener('touchstart', ts, { passive: true });
    content.addEventListener('touchmove', tm, { passive: true });
    content.addEventListener('touchend', te, { passive: true });
    el._smSwipe = function () {
      content.removeEventListener('touchstart', ts);
      content.removeEventListener('touchmove', tm);
      content.removeEventListener('touchend', te);
    };
  }

  function cleanupSwipe(id) {
    var el = document.getElementById(id);
    if (el && el._smSwipe) { el._smSwipe(); delete el._smSwipe; }
  }

  /* ═══════════════════════════════════════════════════
     BUILD MODAL HTML (internal)
     ═══════════════════════════════════════════════════ */

  /**
   * Build the full modal DOM nodes from an options object or a template element.
   * @param {string} id  Modal ID
   * @param {Object} [opts]
   * @returns {HTMLElement}  The modal wrapper (.sm)
   */
  function buildModal(id, opts) {
    opts = opts || {};

    var size = opts.size || 'md';
    var sizeClass = 'sm-' + size; // sm-sm, sm-md, sm-lg, sm-xl, sm-fullscreen, sm-xs
    var anim = opts.animation !== false;

    /* ─── Modal wrapper ─── */
    var sm = document.createElement('div');
    sm.className = 'sm' + (anim ? ' sm-anim' : '') + ' ' + sizeClass;
    sm.id = id;
    sm.setAttribute('role', 'dialog');
    sm.setAttribute('aria-modal', 'true');
    if (opts.backdrop === 'static') sm.setAttribute('data-backdrop', 'static');
    if (opts.title) {
      sm.setAttribute('aria-labelledby', id + '-title');
    }
    if (opts.describedby) {
      sm.setAttribute('aria-describedby', opts.describedby);
    }

    /* ─── Overlay ─── */
    var overlay = document.createElement('div');
    overlay.className = 'sm-overlay';

    /* ─── Dialog ─── */
    var dialog = document.createElement('div');
    dialog.className = 'sm-dialog';

    /* ─── Content ─── */
    var content = document.createElement('div');
    content.className = 'sm-content';

    /* ─── Header ─── */
    if (!opts.noHeader) {
      var hdr = document.createElement('div');
      hdr.className = 'sm-header';

      /* Icon (optional) */
      if (opts.icon) {
        var iconWrap = document.createElement('div');
        var iconColor = opts.iconColor || 'primary';
        iconWrap.className = 'sm-icon sm-icon-' + iconColor;
        iconWrap.innerHTML = '<i class="bi bi-' + opts.icon + '"></i>';
        hdr.appendChild(iconWrap);
      }

      /* Title group */
      var tg = document.createElement('div');
      tg.className = 'sm-title-group';

      if (opts.title) {
        var titleEl = document.createElement('h5');
        titleEl.className = 'sm-title';
        titleEl.id = id + '-title';
        titleEl.textContent = opts.title;
        tg.appendChild(titleEl);
      }

      if (opts.subtitle) {
        var subEl = document.createElement('p');
        subEl.className = 'sm-subtitle';
        subEl.textContent = opts.subtitle;
        tg.appendChild(subEl);
      }

      hdr.appendChild(tg);

      /* Close button */
      if (!opts.noClose) {
        var closeBtn = document.createElement('button');
        closeBtn.className = 'sm-close';
        closeBtn.setAttribute('aria-label', 'Tutup modal');
        closeBtn.innerHTML = '<i class="bi bi-x-lg"></i>';
        closeBtn.addEventListener('click', function () { hide(id); });
        hdr.appendChild(closeBtn);
      }

      content.appendChild(hdr);
    }

    /* ─── Body ─── */
    var body = document.createElement('div');
    body.className = 'sm-body';

    if (opts.body) {
      // If body is HTML string, set as innerHTML
      body.innerHTML = opts.body;
    } else if (opts.message) {
      body.innerHTML = opts.message;
    }
    content.appendChild(body);

    /* ─── Footer ─── */
    if (!opts.noFooter) {
      var ftr = document.createElement('div');
      ftr.className = 'sm-footer';

      /* Cancel button */
      if (opts.cancelText !== undefined) {
        var cancelBtn = document.createElement('button');
        cancelBtn.className = 'sm-btn sm-btn-cancel';
        cancelBtn.textContent = opts.cancelText || 'Batal';
        cancelBtn.addEventListener('click', function () { hide(id); });
        ftr.appendChild(cancelBtn);
      }

      /* Confirm button */
      if (opts.confirmText !== undefined) {
        var confirmBtn = document.createElement('button');
        confirmBtn.className = 'sm-btn sm-btn-primary' + (opts.danger ? ' sm-btn-danger' : '');
        confirmBtn.textContent = opts.confirmText || 'Ya, Lanjutkan';
        confirmBtn.addEventListener('click', function () {
          if (opts.onConfirm) opts.onConfirm();
          hide(id);
        });
        ftr.appendChild(confirmBtn);
      }

      content.appendChild(ftr);
    }

    dialog.appendChild(content);
    sm.appendChild(overlay);
    sm.appendChild(dialog);

    return sm;
  }

  /* ═══════════════════════════════════════════════════
     SHOW / HIDE / TOGGLE
     ═══════════════════════════════════════════════════ */

  /**
   * Show a modal by ID.
   * If modal HTML exists in-page as <template data-modal="ID">, it will be
   * cloned into the portal. Otherwise a dynamic modal is built from opts.
   *
   * @param {string} id
   * @param {Object} [opts]  Options for dynamic modals
   * @returns {boolean}
   */
  function show(id, opts) {
    var portal = ensurePortal();

    // Already visible?
    var existing = document.getElementById(id);
    if (existing && existing.classList.contains('sm-show')) return true;

    var el;

    // Check if there's an inline <template data-modal="id"> in the page
    if (!existing) {
      var tmpl = document.querySelector('template[data-modal="' + id + '"]');
      if (tmpl) {
        var clone = document.importNode(tmpl.content, true);
        el = clone.firstElementChild;
        if (!el || !el.classList.contains('sm')) {
          // Wrap in sm div if needed
          var wrapper = document.createElement('div');
          wrapper.className = 'sm sm-anim sm-' + ((opts && opts.size) || 'md');
          wrapper.id = id;
          wrapper.setAttribute('role', 'dialog');
          wrapper.setAttribute('aria-modal', 'true');

          // Build overlay + dialog + content from the template content
          var overlay = document.createElement('div');
          overlay.className = 'sm-overlay';
          wrapper.appendChild(overlay);

          var dialog = document.createElement('div');
          dialog.className = 'sm-dialog';
          var content = document.createElement('div');
          content.className = 'sm-content';

          // Read children from template
          var children = Array.from(clone.children);
          children.forEach(function (c) { content.appendChild(c); });

          dialog.appendChild(content);
          wrapper.appendChild(dialog);
          el = wrapper;
        }
        portal.appendChild(el);
      } else {
        // Dynamic modal
        el = buildModal(id, opts || {});
        portal.appendChild(el);
      }
    } else {
      el = existing;
      // Cancel any pending removal timer from a previous hide() so a fast
      // re-show doesn't get the element yanked out from under us
      if (el._smRemoveTimer) {
        clearTimeout(el._smRemoveTimer);
        el._smRemoveTimer = null;
      }
      // Detach the stale transitionend removal handler from a previous hide()
      if (el._smRmHandler) {
        el.removeEventListener('transitionend', el._smRmHandler);
        el._smRmHandler = null;
      }
    }

    fire('onShow', id);

    /* ─── Trigger show animation ─── */
    // Force reflow
    void el.offsetWidth;

    el.classList.add('sm-show');
    portal.style.visibility = 'visible';
    portal.style.pointerEvents = 'auto';

    // Track in stack
    if (openModals.indexOf(id) === -1) openModals.push(id);

    // Set z-index based on stack depth. The .sm-overlay must stay BELOW the
    // .sm-dialog (CSS: overlay z-index 0, dialog z-index 1) so it only receives
    // clicks in the backdrop area — otherwise it intercepts wheel/click/touch
    // events aimed at the modal content and the modal becomes unscrollable.
    var depth = openModals.indexOf(id);
    var zBase = Z.MODAL + depth * Z.STACK_STEP;
    el.style.zIndex = zBase;

    // Lock body (first modal only)
    lockBody();

    // Keyboard handler
    if (openModals.length === 1) initKeyHandler();

    // Focus trap
    trapFocus(id);

    // Swipe
    initSwipe(id);

    // Overlay click to close (default behaviour, unless backdrop is 'static')
    var overlayEl = el.querySelector('.sm-overlay');
    if (overlayEl && !el._smOverlayClick && (!opts || opts.backdrop !== 'static')) {
      el._smOverlayClick = true;
      overlayEl.addEventListener('click', function (e) {
        if (e.target === overlayEl) hide(id);
      });
    }

    fire('onShown', id);
    // Notify other components (e.g. sidebar.js) that a modal opened,
    // so they can manage body overflow / close overlays consistently.
    document.dispatchEvent(new CustomEvent('sm:shown', { detail: { id: id } }));
    return true;
  }

  function hide(id) {
    var el = document.getElementById(id);
    if (!el || !el.classList.contains('sm-show')) return;

    fire('onHide', id);

    el.classList.remove('sm-show');

    // Remove from stack
    var idx = openModals.indexOf(id);
    if (idx !== -1) openModals.splice(idx, 1);

    // Release focus
    releaseFocus(id);

    // Cleanup swipe
    cleanupSwipe(id);

    // Unlock body
    unlockBody();

    // Notify other components (e.g. sidebar.js) that a modal closed.
    // Dispatched AFTER the body scroll lock is released so listeners always
    // observe a consistent (unlocked) body state.
    document.dispatchEvent(new CustomEvent('sm:hidden', { detail: { id: id } }));

    // Keyboard
    if (!openModals.length) destroyKeyHandler();

    // Update portal visibility
    if (!openModals.length) {
      var portalEl = document.getElementById('sm-portal');
      if (portalEl) {
        portalEl.style.visibility = 'hidden';
        portalEl.style.pointerEvents = 'none';
      }
    }

    // Remove from DOM after animation completes
    var tid = setTimeout(function () {
      el._smRemoveTimer = null;
      el._smRmHandler = null;
      if (el.parentNode) el.parentNode.removeChild(el);
    }, 300);
    el._smRemoveTimer = tid;

    var rmHandler = function rm() {
      clearTimeout(tid);
      el._smRemoveTimer = null;
      el._smRmHandler = null;
      if (el.parentNode) el.parentNode.removeChild(el);
      el.removeEventListener('transitionend', rmHandler);
    };
    el._smRmHandler = rmHandler;
    el.addEventListener('transitionend', rmHandler);

    fire('onHidden', id);
  }

  function toggle(id, opts) {
    var el = document.getElementById(id);
    if (el && el.classList.contains('sm-show')) { hide(id); return false; }
    return show(id, opts);
  }

  /* ═══════════════════════════════════════════════════
     CONFIRM DIALOG
     ═══════════════════════════════════════════════════ */
  function confirm(opts) {
    opts = opts || {};
    var id = 'sm-confirm-' + Date.now();
    opts.id = id;
    opts.size = opts.size || 'sm';
    opts.noHeader = true;
    opts.confirmText = opts.confirmText || 'Ya, Lanjutkan';
    opts.cancelText = opts.cancelText !== false ? (opts.cancelText || 'Batal') : undefined;

    // Build custom HTML
    var icon = opts.icon || 'exclamation-triangle-fill';
    var iconColor = opts.iconColor || 'danger';

    var msg = '';
    if (opts.title) {
      msg += '<h5 class="sm-confirm-title">' + esc(opts.title) + '</h5>';
    }
    if (opts.message) {
      msg += '<p class="sm-confirm-message">' + opts.message + '</p>';
    }

    opts.body =
      '<div class="sm-confirm-icon sm-icon-' + iconColor + '">' +
        '<i class="bi bi-' + icon + '"></i>' +
      '</div>' +
      '<div class="sm-confirm-body">' + msg + '</div>';

    // Override confirm to fire callback
    var origConfirm = opts.onConfirm;
    opts.onConfirm = function () {
      if (origConfirm) origConfirm();
    };

    return show(id, opts);
  }

  /* ═══════════════════════════════════════════════════
     TOAST
     ═══════════════════════════════════════════════════ */
  function toast(message, type, duration) {
    type = type || 'info';
    duration = duration !== undefined ? duration : 4000;

    var icons = { success: 'bi-check-circle-fill', error: 'bi-x-circle-fill', warning: 'bi-exclamation-triangle-fill', info: 'bi-info-circle-fill' };
    var colors = { success: '#10b981', error: '#ef4444', warning: '#f59e0b', info: '#3b82f6' };
    var icon = icons[type] || icons.info;
    var color = colors[type] || colors.info;

    var container = document.getElementById('sm-toast-container');
    if (!container) {
      container = document.createElement('div');
      container.id = 'sm-toast-container';
      container.style.cssText = 'position:fixed;top:20px;right:20px;z-index:' + Z.TOAST + ';' +
        'display:flex;flex-direction:column;gap:10px;max-width:400px;pointer-events:none;';
      document.body.appendChild(container);
    }

    var id = 'sm-toast-' + Date.now();
    var el = document.createElement('div');
    el.id = id;
    el.setAttribute('role', 'alert');
    el.setAttribute('aria-live', 'polite');
    el.style.cssText = 'display:flex;align-items:center;gap:12px;padding:14px 18px;' +
      'background:var(--surface,#fff);border-radius:16px;' +
      'box-shadow:0 10px 40px rgba(0,0,0,0.12),0 0 0 1px rgba(0,0,0,0.05);' +
      'animation:smToastIn 0.3s cubic-bezier(0.16,1,0.3,1) both;' +
      'border-left:4px solid ' + color + ';' +
      'pointer-events:auto;cursor:default;';

    el.innerHTML =
      '<i class="bi ' + icon + '" style="color:' + color + ';font-size:1.3rem;flex-shrink:0;"></i>' +
      '<span style="flex:1;font-size:0.875rem;color:var(--ink,#1e293b);font-weight:500;line-height:1.4;">' +
        esc(message) +
      '</span>' +
      '<button type="button" class="sm-toast-close" aria-label="Tutup notifikasi">' +
        '<i class="bi bi-x-lg"></i>' +
      '</button>';

    container.appendChild(el);

    var closeBtn = el.querySelector('.sm-toast-close');
    if (closeBtn) {
      closeBtn.addEventListener('click', function (e) {
        e.stopPropagation();
        dismiss(el);
      });
    }

    function dismiss(target) {
      target.style.transition = 'all 0.25s cubic-bezier(0.4,0,0.2,1)';
      target.style.opacity = '0';
      target.style.transform = 'translateX(24px)';
      setTimeout(function () {
        if (target.parentNode) target.parentNode.removeChild(target);
      }, 300);
    }

    if (duration > 0) {
      setTimeout(function () { dismiss(el); }, duration);
    }

    return el;
  }

  /* ═══════════════════════════════════════════════════
     LOADING STATE
     ═══════════════════════════════════════════════════ */
  function setLoading(id, state, text) {
    var el = document.getElementById(id);
    if (!el) return;
    var overlay = el.querySelector('.sm-loading');
    if (state) {
      if (!overlay) {
        overlay = document.createElement('div');
        overlay.className = 'sm-loading';
        overlay.innerHTML =
          '<div class="sm-loading-spinner">' +
            '<div class="spinner-border text-primary" role="status">' +
              '<span class="visually-hidden">Loading...</span>' +
            '</div>' +
            '<p class="sm-loading-text">' + esc(text || 'Memproses...') + '</p>' +
          '</div>';
        el.querySelector('.sm-body').appendChild(overlay);
      }
      overlay.style.display = 'flex';
    } else {
      if (overlay) overlay.style.display = 'none';
    }
  }

  /* ═══════════════════════════════════════════════════
     EVENT SYSTEM
     ═══════════════════════════════════════════════════ */
  function onEvent(ev, fn) {
    if (_callbacks[ev] && typeof fn === 'function') _callbacks[ev].push(fn);
    return this;
  }
  function offEvent(ev, fn) {
    if (_callbacks[ev]) {
      _callbacks[ev] = fn ? _callbacks[ev].filter(function (f) { return f !== fn; }) : [];
    }
    return this;
  }

  /* ═══════════════════════════════════════════════════
     EXPOSE
     ═══════════════════════════════════════════════════ */
  window.SimarcModal = {
    /* Core */
    show: show,
    hide: hide,
    toggle: toggle,
    openModals: openModals,
    Z: Z,

    /* Helpers */
    confirm: confirm,
    toast: toast,
    setLoading: setLoading,

    /* Events */
    on: onEvent,
    off: offEvent,

    /* Internal */
    trapFocus: trapFocus,
    releaseFocus: releaseFocus,
    lockScroll: lockBody,
    unlockScroll: unlockBody,
  };

  /* ─── Init ─── */
  ensurePortal();

  /* ─── Clean up on unload ─── */
  window.addEventListener('beforeunload', function () {
    document.body.style.position = '';
    document.body.style.top = '';
    document.body.style.left = '';
    document.body.style.overflow = '';
    document.body.style.width = '';
    document.body.style.paddingRight = '';
    destroyKeyHandler();
  });

})();
