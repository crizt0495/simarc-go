/**
 * SIMARC — UI Micro-Interactions
 * World-class feel · defensive (no-op when elements absent)
 * - Count-up animation for stat numbers [data-count]
 * - Reveal-on-scroll stagger for .reveal, .reveal-group > *
 * - Gentle 3D tilt on [data-tilt]
 */
(function () {
  'use strict';
  if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) return;

  // ── Count-up ──
  function animateCount(el) {
    var target = parseFloat(el.getAttribute('data-count'));
    if (isNaN(target)) {
      var txt = (el.textContent || '').replace(/[^\d.-]/g, '');
      target = parseFloat(txt);
      if (isNaN(target)) return;
    }
    var suffix = (el.textContent || '').replace(/[\d.,\s-]/g, '').slice(0, 3);
    var prefix = (el.textContent || '').match(/^[^\d.,-]+/);
    prefix = prefix ? prefix[0] : '';
    var decimals = Math.max(0, (String(target).split('.')[1] || '').length);
    var dur = 900;
    var start = null;
    function fmt(v) {
      return v.toLocaleString('id-ID', { minimumFractionDigits: decimals, maximumFractionDigits: decimals });
    }
    function step(ts) {
      if (!start) start = ts;
      var p = Math.min((ts - start) / dur, 1);
      var eased = 1 - Math.pow(1 - p, 3);
      el.textContent = prefix + fmt(target * eased) + suffix;
      if (p < 1) requestAnimationFrame(step);
      else el.textContent = prefix + fmt(target) + suffix;
    }
    requestAnimationFrame(step);
  }

  // ── Count-up observers (dashboard stat values & [data-count]) ──
  function initCountUp() {
    var els = Array.prototype.slice.call(
      document.querySelectorAll('[data-count], .stat-card-value[data-stat]')
    );
    if (!els.length) return;
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (e) {
        if (e.isIntersecting) {
          animateCount(e.target);
          io.unobserve(e.target);
        }
      });
    }, { threshold: 0.4 });
    els.forEach(function (el) { io.observe(el); });
  }

  // ── Reveal on scroll (explicit [data-reveal] / .reveal) ──
  function initReveal() {
    var targets = Array.prototype.slice.call(document.querySelectorAll('.reveal, [data-reveal]'));
    document.querySelectorAll('.reveal-group').forEach(function (g) {
      Array.prototype.slice.call(g.children).forEach(function (c, i) {
        if (!targets.indexOf(c) > -1) {
          c.classList.add('reveal');
          c.style.transitionDelay = (Math.min(i, 9) * 70) + 'ms';
          targets.push(c);
        }
      });
    });
    if (!targets.length) return;
    targets.forEach(function (el) {
      el.style.opacity = '0';
      el.style.transform = 'translateY(16px)';
      el.style.transition = 'opacity .5s cubic-bezier(.16,1,.3,1), transform .5s cubic-bezier(.16,1,.3,1)';
    });
    var io = new IntersectionObserver(function (entries) {
      entries.forEach(function (e) {
        if (e.isIntersecting) {
          e.target.style.opacity = '1';
          e.target.style.transform = 'translateY(0)';
          io.unobserve(e.target);
        }
      });
    }, { threshold: 0.08, rootMargin: '0px 0px -40px 0px' });
    targets.forEach(function (el) { io.observe(el); });
  }

  // ── 3D tilt ──
  function initTilt() {
    var els = Array.prototype.slice.call(document.querySelectorAll('[data-tilt]'));
    if (!els.length) return;
    els.forEach(function (el) {
      el.addEventListener('mousemove', function (ev) {
        var r = el.getBoundingClientRect();
        var x = ((ev.clientX - r.left) / r.width - 0.5) * 8;
        var y = ((ev.clientY - r.top) / r.height - 0.5) * 8;
        el.style.transform = 'rotateX(' + (-y) + 'deg) rotateY(' + x + 'deg) translateY(-2px)';
      });
      el.addEventListener('mouseleave', function () {
        el.style.transform = '';
      });
    });
  }

  function boot() {
    if (document.readyState === 'loading') {
      document.addEventListener('DOMContentLoaded', function () {
        initCountUp();
        initReveal();
        initTilt();
      });
    } else {
      setTimeout(function () {
        initCountUp();
        initReveal();
        initTilt();
      }, 50);
    }
  }
  boot();
})();