/**
 * Unit tests for web/static/js/arsip-download.js
 *
 * The download guard intercepts clicks on `/arsip/{id}/download` and
 * `/arsip/version/{id}/download` links, checks file existence via the matching
 * check-file endpoint, and either proceeds with the download or shows the
 * "File Belum Diupload" warning modal.
 */

import { afterAll, afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const downloadSrc = fs.readFileSync(
  path.resolve(__dirname, '../arsip-download.js'),
  'utf-8'
);

// ─── Mocked globals (vi.stubGlobal targets globalThis, which is what the
// ─── eval'd script resolves `fetch` / `SimarcModal` against in vitest/jsdom) ───

const modalMock = { confirm: vi.fn(), show: vi.fn(), hide: vi.fn() };
let fetchMock;

// ─── Helpers ───

function mockLocation() {
  let href = 'http://localhost/arsip';
  const loc = {};
  Object.defineProperty(window, 'location', {
    configurable: true,
    value: loc,
  });
  Object.defineProperty(loc, 'href', {
    configurable: true,
    get: function () { return href; },
    set: function (v) { href = v; },
  });
  return function getHref() { return href; };
}

let getLocationHref;

function stubFetch(response) {
  fetchMock = vi.fn(function () {
    return Promise.resolve({ json: function () { return Promise.resolve(response); } });
  });
  vi.stubGlobal('fetch', fetchMock);
}

function stubFetchError() {
  fetchMock = vi.fn(function () { return Promise.reject(new Error('network down')); });
  vi.stubGlobal('fetch', fetchMock);
}

function stubPendingFetch() {
  fetchMock = vi.fn(function () { return new Promise(function () {}); });
  vi.stubGlobal('fetch', fetchMock);
}

function makeDownloadLink(id, href) {
  const a = document.createElement('a');
  a.href = href || ('/arsip/' + id + '/download');
  document.body.appendChild(a);
  return a;
}

/** Flush the microtask chain of the mocked fetch (fetch → json → then). */
function flushAsync() {
  return new Promise(function (resolve) { setTimeout(resolve, 0); });
}

function clickLink(link, opts) {
  opts = opts || {};
  const ev = new MouseEvent('click', {
    bubbles: true,
    cancelable: true,
    button: opts.button || 0,
    ctrlKey: !!opts.ctrlKey,
    metaKey: !!opts.metaKey,
    shiftKey: !!opts.shiftKey,
    altKey: !!opts.altKey,
  });
  link.dispatchEvent(ev);
  return ev;
}

// ─── Setup ───

beforeAll(() => {
  getLocationHref = mockLocation();
  vi.stubGlobal('SimarcModal', modalMock);
  // Register the delegated click listener once.
  eval(downloadSrc);
});

afterAll(() => {
  vi.unstubAllGlobals();
});

beforeEach(() => {
  modalMock.confirm.mockReset();
  modalMock.show.mockReset();
  document.body.innerHTML = '';
  // Reset the location mock and give every test a defined fetch spy.
  getLocationHref = mockLocation();
  stubFetch({ exists: true });
});

afterEach(() => {
  document.body.innerHTML = '';
});

// ═══════════════════════════════════════════════
// TESTS
// ═══════════════════════════════════════════════

describe('Link targeting', () => {
  it('does nothing for links that are not arsip downloads', () => {
    const link = makeDownloadLink('x', '/arsip');
    clickLink(link);
    expect(fetchMock).not.toHaveBeenCalled();
    expect(modalMock.confirm).not.toHaveBeenCalled();
  });

  it('does not intercept qrcode downloads', () => {
    const link = makeDownloadLink('a1', '/arsip/a1/qrcode/download');
    clickLink(link);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('does not intercept the template download', () => {
    const link = makeDownloadLink('x', '/arsip/download-template');
    clickLink(link);
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('does not intercept clicks with modifier keys (new tab etc.)', () => {
    const link = makeDownloadLink('a1');
    clickLink(link, { ctrlKey: true });
    expect(fetchMock).not.toHaveBeenCalled();
  });

  it('does not intercept right/middle button clicks', () => {
    const link = makeDownloadLink('a1');
    clickLink(link, { button: 2 });
    expect(fetchMock).not.toHaveBeenCalled();
  });
});

describe('File check behaviour', () => {
  it('prevents default and asks the server for a download click', () => {
    stubFetch({ exists: true });
    const link = makeDownloadLink('a1');
    const ev = clickLink(link);
    expect(ev.defaultPrevented).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith('/arsip/a1/check-file', expect.anything());
  });

  it('navigates to the download URL when the file exists', async () => {
    stubFetch({ exists: true });
    const link = makeDownloadLink('a1');
    clickLink(link);
    await flushAsync();
    expect(getLocationHref()).toBe('/arsip/a1/download');
    expect(modalMock.confirm).not.toHaveBeenCalled();
  });

  it('shows the "File Belum Diupload" modal when the file is missing', async () => {
    stubFetch({ exists: false });
    const link = makeDownloadLink('a1');
    clickLink(link);
    await flushAsync();
    expect(getLocationHref()).toBe('http://localhost/arsip');
    expect(modalMock.confirm).toHaveBeenCalledTimes(1);
    const opts = modalMock.confirm.mock.calls[0][0];
    expect(opts.title).toBe('File Belum Diupload');
    expect(opts.iconColor).toBe('warning');
    expect(opts.confirmText).toBe('Mengerti');
    expect(opts.message).toContain('Arsip ini belum memiliki file digital');
  });

  it('falls back to navigating when the modal library is unavailable', async () => {
    stubFetch({ exists: false });
    const realConfirm = modalMock.confirm;
    modalMock.confirm = undefined;
    try {
      const link = makeDownloadLink('a1');
      clickLink(link);
      await flushAsync();
      expect(getLocationHref()).toBe('/arsip/a1/download');
    } finally {
      modalMock.confirm = realConfirm;
    }
  });

  it('falls back to navigating when the check request fails', async () => {
    stubFetchError();
    const link = makeDownloadLink('a1');
    clickLink(link);
    await flushAsync();
    expect(getLocationHref()).toBe('/arsip/a1/download');
    expect(modalMock.confirm).not.toHaveBeenCalled();
  });
});

describe('Version downloads', () => {
  it('intercepts version download links and checks the version endpoint', () => {
    stubFetch({ exists: true });
    const link = makeDownloadLink('v1', '/arsip/version/v1/download');
    const ev = clickLink(link);
    expect(ev.defaultPrevented).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith('/arsip/version/v1/check-file', expect.anything());
  });

  it('navigates to the version download URL when the version file exists', async () => {
    stubFetch({ exists: true });
    const link = makeDownloadLink('v1', '/arsip/version/v1/download');
    clickLink(link);
    await flushAsync();
    expect(getLocationHref()).toBe('/arsip/version/v1/download');
    expect(modalMock.confirm).not.toHaveBeenCalled();
  });

  it('shows the "File Belum Diupload" modal when the version file is missing', async () => {
    stubFetch({ exists: false });
    const link = makeDownloadLink('v1', '/arsip/version/v1/download');
    clickLink(link);
    await flushAsync();
    expect(getLocationHref()).toBe('http://localhost/arsip');
    expect(modalMock.confirm).toHaveBeenCalledTimes(1);
    const opts = modalMock.confirm.mock.calls[0][0];
    expect(opts.title).toBe('File Belum Diupload');
    expect(opts.message).toContain('versi ini');
  });

  it('intercepts the alternate version route form and checks the version endpoint', () => {
    stubFetch({ exists: true });
    const link = makeDownloadLink('v2', '/arsip/a1/versions/v2/download');
    const ev = clickLink(link);
    expect(ev.defaultPrevented).toBe(true);
    expect(fetchMock).toHaveBeenCalledWith('/arsip/version/v2/check-file', expect.anything());
  });
});

describe('Double-click guard', () => {
  it('only fires one check while a previous check is pending', () => {
    stubPendingFetch();
    const link = makeDownloadLink('a1');
    clickLink(link);
    clickLink(link);
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });
});
