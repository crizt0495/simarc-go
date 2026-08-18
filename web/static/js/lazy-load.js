/**
 * Lazy Load Script - Optimized Image Loading
 * Improves page load performance by deferring off-screen images
 */

(function() {
    'use strict';

    // Lazy load images with data-src attribute
    function initLazyLoad() {
        const lazyImages = document.querySelectorAll('img[data-src]:not([src])');
        
        if ('IntersectionObserver' in window) {
            const imageObserver = new IntersectionObserver((entries, observer) => {
                entries.forEach(entry => {
                    if (entry.isIntersecting) {
                        const img = entry.target;
                        img.src = img.dataset.src;
                        
                        if (img.dataset.srcset) {
                            img.srcset = img.dataset.srcset;
                        }
                        
                        img.classList.add('loaded');
                        observer.unobserve(img);
                    }
                });
            }, {
                rootMargin: '50px 0px',
                threshold: 0.01
            });

            lazyImages.forEach(img => imageObserver.observe(img));
        } else {
            // Fallback for browsers without IntersectionObserver
            lazyImages.forEach(img => {
                img.src = img.dataset.src;
                if (img.dataset.srcset) {
                    img.srcset = img.dataset.srcset;
                }
            });
        }
    }

    // Lazy load background images
    function initLazyBackgrounds() {
        const lazyElements = document.querySelectorAll('[data-bg]');
        
        if ('IntersectionObserver' in window) {
            const bgObserver = new IntersectionObserver((entries, observer) => {
                entries.forEach(entry => {
                    if (entry.isIntersecting) {
                        const el = entry.target;
                        el.style.backgroundImage = `url(${el.dataset.bg})`;
                        el.classList.add('loaded');
                        observer.unobserve(el);
                    }
                });
            }, {
                rootMargin: '100px 0px',
                threshold: 0.01
            });

            lazyElements.forEach(el => bgObserver.observe(el));
        } else {
            lazyElements.forEach(el => {
                el.style.backgroundImage = `url(${el.dataset.bg})`;
            });
        }
    }

    // Initialize when DOM is ready
    if (document.readyState === 'loading') {
        document.addEventListener('DOMContentLoaded', initLazyLoad);
        document.addEventListener('DOMContentLoaded', initLazyBackgrounds);
    } else {
        initLazyLoad();
        initLazyBackgrounds();
    }
})();

/**
 * Date Input Enhancement
 * Adds calendar icon styling and quick-date buttons for better UX
 */
document.addEventListener('DOMContentLoaded', function() {
    // Enhance all date inputs with better UX
    var dateInputs = document.querySelectorAll('input[type="date"]');
    dateInputs.forEach(function(input) {
        // Add a wrapper for better styling
        if (!input.closest('.date-wrapper')) {
            var wrapper = document.createElement('div');
            wrapper.className = 'date-wrapper';
            wrapper.style.cssText = 'position:relative;display:flex;align-items:center;';
            input.parentNode.insertBefore(wrapper, input);
            wrapper.appendChild(input);

            // Add calendar icon button
            var btn = document.createElement('button');
            btn.type = 'button';
            btn.className = 'date-today-btn';
            btn.innerHTML = '<i class="bi bi-calendar3"></i>';
            btn.title = 'Pilih tanggal';
            btn.setAttribute('aria-label', 'Buka kalender');
            btn.style.cssText = 'position:absolute;right:8px;top:50%;transform:translateY(-50%);border:none;background:transparent;color:#64748b;cursor:pointer;padding:4px;z-index:2;line-height:1;';
            
            // On click, focus the input which triggers the native date picker
            btn.addEventListener('click', function(e) {
                e.preventDefault();
                input.showPicker ? input.showPicker() : input.focus();
            });
            
            wrapper.appendChild(btn);
            
            // Add padding to input so text doesn't overlap icon
            var currentPadding = parseInt(window.getComputedStyle(input).paddingRight);
            if (!isNaN(currentPadding) && currentPadding < 38) {
                input.style.paddingRight = '38px';
            }
        }

        // For premium forms, add a "Hari Ini" quick-link
        var parent = input.closest('.form-group');
        if (parent && !input.id && !parent.querySelector('.date-quick')) {
            var quick = document.createElement('small');
            quick.className = 'date-quick';
            quick.style.cssText = 'display:block;margin-top:4px;font-size:11px;color:#3b82f6;cursor:pointer;font-weight:600;';
            quick.innerHTML = '<i class="bi bi-arrow-repeat me-1"></i>Hari Ini';
            quick.addEventListener('click', function() {
                var today = new Date();
                var yyyy = today.getFullYear();
                var mm = String(today.getMonth() + 1).padStart(2, '0');
                var dd = String(today.getDate()).padStart(2, '0');
                input.value = yyyy + '-' + mm + '-' + dd;
                input.dispatchEvent(new Event('change', { bubbles: true }));
            });
            parent.appendChild(quick);
        }
    });

    // Fix: input[type=date] native picker icon on Chrome is hidden when using custom icon
    // Show the native picker on wrapper click too
    document.querySelectorAll('.date-wrapper').forEach(function(w) {
        w.addEventListener('click', function(e) {
            if (e.target === w) {
                var inp = w.querySelector('input[type="date"]');
                if (inp) inp.showPicker ? inp.showPicker() : inp.focus();
            }
        });
    });
});
