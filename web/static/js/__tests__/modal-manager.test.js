/**
 * Unit tests for SimarcModal v4.0 — Portal-based Modal Manager (no Bootstrap dependency)
 *
 * API under test (web/static/js/modal-manager.js):
 *   · show(id, opts) / hide(id) / toggle(id, opts)
 *   · trapFocus(id) / releaseFocus(id)
 *   · lockScroll() / unlockScroll()
 *   · openModals stack, Z constants
 *   · dynamic modals built from opts, or cloned from <template data-modal="id">
 *   · beforeunload cleanup
 */

import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it } from 'vitest';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const modalManagerSrc = fs.readFileSync(
  path.resolve(__dirname, '../modal-manager.js'),
  'utf-8'
);

// ─── Helpers ───

/** Show a (dynamic) modal via SimarcModal.show and return the element. */
function showModal(id, opts) {
  const res = SimarcModal.show(id, opts);
  return { res, el: document.getElementById(id) };
}

/** Create a shown .sm modal element in the DOM (for focus-trap tests). */
function makeShownModal(id) {
  const m = document.createElement('div');
  m.id = id;
  m.className = 'sm sm-show';
  m.innerHTML =
    '<div class="sm-overlay"></div>' +
    '<div class="sm-dialog"><div class="sm-content">' +
    '<button id="' + id + '-b1">First</button>' +
    '<button id="' + id + '-b2">Last</button>' +
    '</div></div>';
  document.body.appendChild(m);
  // jsdom has no layout: force offsetParent so isVisible() passes
  m.querySelectorAll('button').forEach(function (b) {
    Object.defineProperty(b, 'offsetParent', { value: m, configurable: true });
  });
  return m;
}

// ─── Load modal-manager once ───
beforeAll(() => {
  // jsdom does not implement scrollTo; modal-manager's unlockScroll calls it
  window.scrollTo = function () {};
  eval(modalManagerSrc);
});

afterAll(() => {
  delete globalThis.SimarcModal;
});

beforeEach(() => {
  document.body.innerHTML = '';
  const portal = document.getElementById('sm-portal');
  if (portal) portal.remove();
  if (globalThis.SimarcModal) {
    // Reset the module's scroll-lock counter by unlocking until body is released
    while (document.body.style.position === 'fixed') SimarcModal.unlockScroll();
    SimarcModal.openModals.length = 0;
  }
});

afterEach(() => {
  document.body.innerHTML = '';
});

// ═══════════════════════════════════════════════
// TESTS
// ═══════════════════════════════════════════════

describe('SimarcModal.show()', () => {
  it('returns true and builds a dynamic modal for an unknown id', () => {
    const { res, el } = showModal('newModal', { title: 'Halo' });
    expect(res).toBe(true);
    expect(el).toBeTruthy();
    expect(el.classList.contains('sm')).toBe(true);
    expect(el.classList.contains('sm-show')).toBe(true);
  });

  it('renders the modal inside #sm-portal', () => {
    showModal('newModal');
    const portal = document.getElementById('sm-portal');
    expect(portal).toBeTruthy();
    expect(portal.contains(document.getElementById('newModal'))).toBe(true);
  });

  it('is idempotent when the modal is already shown', () => {
    showModal('newModal');
    expect(document.querySelectorAll('.sm').length).toBe(1);
    showModal('newModal');
    expect(document.querySelectorAll('.sm').length).toBe(1);
  });

  it('renders title, body and footer text from options', () => {
    showModal('newModal', { title: 'Judul', message: 'Isi pesan', confirmText: 'OK' });
    expect(document.querySelector('.sm-title').textContent).toBe('Judul');
    expect(document.querySelector('.sm-body').textContent).toContain('Isi pesan');
    expect(document.querySelector('.sm-btn-primary').textContent).toBe('OK');
  });

  it('sets data-backdrop="static" when backdrop option is static', () => {
    showModal('newModal', { backdrop: 'static' });
    expect(document.getElementById('newModal').getAttribute('data-backdrop')).toBe('static');
  });

  it('tracks the modal id in openModals', () => {
    showModal('newModal');
    expect(SimarcModal.openModals).toContain('newModal');
  });

  it('clones an inline <template data-modal> when the id matches', () => {
    document.body.innerHTML =
      '<template data-modal="tplModal">' +
      '<div class="sm sm-md" id="tplModal">' +
      '<div class="sm-overlay"></div>' +
      '<div class="sm-dialog"><div class="sm-content"><div class="sm-body">Isi template</div></div></div>' +
      '</div></template>';
    const { res, el } = showModal('tplModal');
    expect(res).toBe(true);
    expect(el).toBeTruthy();
    expect(el.classList.contains('sm')).toBe(true);
    expect(el.querySelector('.sm-body').textContent).toBe('Isi template');
  });

  it('wraps partial templates (header/body/footer) into a proper .sm shell', () => {
    document.body.innerHTML =
      '<template data-modal="partialModal">' +
      '<div class="sm-header"><h5 class="sm-title">Judul</h5></div>' +
      '<div class="sm-body">Isi</div>' +
      '<div class="sm-footer"><button class="sm-btn sm-btn-primary">OK</button></div>' +
      '</template>';
    const { res, el } = showModal('partialModal');
    expect(res).toBe(true);
    expect(el).toBeTruthy();
    expect(el.classList.contains('sm')).toBe(true);
    expect(el.querySelector('.sm-overlay')).toBeTruthy();
    expect(el.querySelector('.sm-dialog .sm-content .sm-title').textContent).toBe('Judul');
  });

  it('respects the size option for template-based modals', () => {
    document.body.innerHTML =
      '<template data-modal="sizeModal">' +
      '<div class="sm-header"></div><div class="sm-body"></div>' +
      '</template>';
    showModal('sizeModal', { size: 'lg' });
    expect(document.getElementById('sizeModal').classList.contains('sm-lg')).toBe(true);
  });
});

describe('SimarcModal.hide()', () => {
  it('removes the shown state and stops tracking the modal', () => {
    showModal('newModal');
    const el = document.getElementById('newModal');
    expect(el.classList.contains('sm-show')).toBe(true);
    SimarcModal.hide('newModal');
    expect(el.classList.contains('sm-show')).toBe(false);
    expect(SimarcModal.openModals).not.toContain('newModal');
  });

  it('does not throw when the modal id does not exist', () => {
    expect(() => SimarcModal.hide('nonexistent')).not.toThrow();
  });

  it('does not throw when the modal exists but is not shown', () => {
    const el = document.createElement('div');
    el.id = 'plainModal';
    el.className = 'sm';
    document.body.appendChild(el);
    expect(() => SimarcModal.hide('plainModal')).not.toThrow();
  });
});

describe('Focus Trap', () => {
  it('exposes trapFocus and releaseFocus functions', () => {
    expect(typeof SimarcModal.trapFocus).toBe('function');
    expect(typeof SimarcModal.releaseFocus).toBe('function');
  });

  it('stores the trap handler on the modal element', () => {
    const modal = makeShownModal('fm');
    SimarcModal.trapFocus('fm');
    expect(modal._smFocusTrap).toBeDefined();
  });

  it('releases the trap handler when releaseFocus is called', () => {
    const modal = makeShownModal('fm');
    SimarcModal.trapFocus('fm');
    expect(modal._smFocusTrap).toBeDefined();
    SimarcModal.releaseFocus('fm');
    expect(modal._smFocusTrap).toBeUndefined();
  });

  it('focuses the first focusable element when the trap is set', async () => {
    makeShownModal('fm');
    SimarcModal.trapFocus('fm');
    // Wait for the requestAnimationFrame scheduled inside trapFocus
    await new Promise((resolve) => {
      const raf = window.requestAnimationFrame || ((cb) => setTimeout(cb, 20));
      raf(resolve);
    });
    expect(document.activeElement).toBe(document.getElementById('fm-b1'));
    SimarcModal.releaseFocus('fm');
  });
});

describe('Backdrop / Overlay', () => {
  it('creates an overlay element when a modal is shown', () => {
    showModal('newModal');
    expect(document.querySelector('.sm-overlay')).toBeTruthy();
  });

  it('closes the modal when the overlay is clicked (default behaviour)', () => {
    showModal('newModal');
    const el = document.getElementById('newModal');
    expect(el.classList.contains('sm-show')).toBe(true);
    el.querySelector('.sm-overlay').click();
    expect(el.classList.contains('sm-show')).toBe(false);
  });

  it('keeps the modal open on overlay click when backdrop is static', () => {
    showModal('newModal', { backdrop: 'static' });
    const el = document.getElementById('newModal');
    el.querySelector('.sm-overlay').click();
    expect(el.classList.contains('sm-show')).toBe(true);
  });

  it('assigns stacked z-index based on open depth', () => {
    showModal('m1');
    showModal('m2');
    expect(document.getElementById('m1').style.zIndex).toBe('1080');
    expect(document.getElementById('m2').style.zIndex).toBe('1100');
  });

  it('keeps the overlay BELOW the dialog so modal content stays interactive', () => {
    // Regression: the overlay used to get an inline z-index (zBase - 1) that
    // painted it ABOVE the dialog (which had no z-index), so the invisible
    // overlay intercepted every wheel/click/touch event over the modal body
    // and the modal could not be scrolled.
    showModal('stackModal', { title: 'T', body: '<button id="stackBtn">Klik</button>' });
    const el = document.getElementById('stackModal');
    const overlay = el.querySelector('.sm-overlay');
    const dialog = el.querySelector('.sm-dialog');

    // No inline z-index on the overlay — stacking is handled by CSS
    // (.sm-overlay z-index:0, .sm-dialog z-index:1), which keeps the dialog
    // above the overlay inside the modal's own stacking context.
    expect(overlay.style.zIndex).toBe('');
    expect(dialog).toBeTruthy();
    expect(dialog.style.zIndex).toBe('');

    // Sanity: the modal itself still gets a stacking-context z-index,
    // and the overlay click-to-close still works.
    expect(el.style.zIndex).toBe('1080');
    expect(el.querySelector('#stackBtn')).toBeTruthy();
    overlay.click();
    expect(el.classList.contains('sm-show')).toBe(false);
  });
});

describe('Body Scroll Lock', () => {
  it('locks body scroll', () => {
    SimarcModal.lockScroll();
    expect(document.body.style.overflow).toBe('hidden');
  });

  it('unlocks body scroll', () => {
    SimarcModal.lockScroll();
    SimarcModal.unlockScroll();
    expect(document.body.style.overflow).toBe('');
  });

  it('exposes lockScroll and unlockScroll functions', () => {
    expect(typeof SimarcModal.lockScroll).toBe('function');
    expect(typeof SimarcModal.unlockScroll).toBe('function');
  });
});

describe('Z-index Constants', () => {
  it('has correct z-index values', () => {
    expect(SimarcModal.Z.MODAL).toBe(1080);
    expect(SimarcModal.Z.BACKDROP).toBe(1070);
    expect(SimarcModal.Z.TOAST).toBe(9999);
  });
});

describe('Open Modals Stack', () => {
  it('starts as an empty array', () => {
    expect(Array.isArray(SimarcModal.openModals)).toBe(true);
    expect(SimarcModal.openModals.length).toBe(0);
  });
});

describe('ARIA Attributes', () => {
  it('adds role and aria-modal on shown modals', () => {
    showModal('newModal', { title: 'T' });
    const el = document.getElementById('newModal');
    expect(el.getAttribute('role')).toBe('dialog');
    expect(el.getAttribute('aria-modal')).toBe('true');
  });
});

describe('DOM Events (sm:shown / sm:hidden)', () => {
  it('dispatches sm:shown on open and sm:hidden on close with the modal id', () => {
    const events = [];
    const onShown = (e) => events.push('shown:' + e.detail.id);
    const onHidden = (e) => events.push('hidden:' + e.detail.id);
    document.addEventListener('sm:shown', onShown);
    document.addEventListener('sm:hidden', onHidden);
    try {
      showModal('eventModal', { title: 'E' });
      expect(events).toEqual(['shown:eventModal']);
      SimarcModal.hide('eventModal');
      expect(events).toEqual(['shown:eventModal', 'hidden:eventModal']);
    } finally {
      document.removeEventListener('sm:shown', onShown);
      document.removeEventListener('sm:hidden', onHidden);
    }
  });

  it('dispatches sm:hidden only for the actual modal being closed (stacked modals)', () => {
    const events = [];
    const onHidden = (e) => events.push('hidden:' + e.detail.id);
    document.addEventListener('sm:hidden', onHidden);
    try {
      showModal('stackA', { title: 'A' });
      showModal('stackB', { title: 'B' });
      SimarcModal.hide('stackA');
      expect(events).toEqual(['hidden:stackA']);
    } finally {
      document.removeEventListener('sm:hidden', onHidden);
    }
  });
});

describe('Edge Cases', () => {
  it('beforeunload cleans up body styles', () => {
    document.body.style.overflow = 'hidden';
    document.body.style.paddingRight = '15px';
    window.dispatchEvent(new Event('beforeunload'));
    expect(document.body.style.overflow).toBe('');
    expect(document.body.style.paddingRight).toBe('');
  });
});
