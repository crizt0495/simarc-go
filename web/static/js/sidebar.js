/**
 * ArsipPro Sidebar JavaScript
 * 🏆 Consolidated & Robust
 */

document.addEventListener('DOMContentLoaded', function() {
    const sidebar = document.querySelector('.sidebar');
    const sidebarOverlay = document.getElementById('sidebarOverlay');
    const sidebarToggle = document.getElementById('sidebarToggle');

    // Robust Toggle Function
    window.toggleSidebar = function() {
        if (!sidebar) return;
        
        sidebar.classList.toggle('active');
        if (sidebarOverlay) {
            sidebarOverlay.classList.toggle('active');
        }
        
        // Prevent body scroll on mobile when sidebar is active
        // Don't reset overflow if modal is open (modal-manager handles modal scroll lock)
        if (window.innerWidth < 992) {
            const isActive = sidebar.classList.contains('active');
            if (isActive) {
                document.body.style.overflow = 'hidden';
            } else if (!isModalOpen()) {
                document.body.style.overflow = '';
            }
        }
    };

    // Explicit listener for the hamburger button
    if (sidebarToggle) {
        sidebarToggle.addEventListener('click', function(e) {
            e.preventDefault();
            e.stopPropagation();
            toggleSidebar();
        });
    }

    // Helper: check if any modal is open (Bootstrap legacy + SIMARC V4 .sm)
    function isModalOpen() {
        return document.querySelector('.modal.show, .sm.sm-show') !== null;
    }

    // Close on overlay click (but don't break modal scroll lock)
    if (sidebarOverlay) {
        sidebarOverlay.addEventListener('click', function() {
            sidebar.classList.remove('active');
            sidebarOverlay.classList.remove('active');
            // Only reset overflow if no modal is open (modal-manager handles modal scroll)
            if (!isModalOpen()) {
                document.body.style.overflow = '';
            }
        });
    }

    // Handle Resize
    window.addEventListener('resize', function() {
        if (window.innerWidth >= 992) {
            if (sidebar) sidebar.classList.remove('active');
            if (sidebarOverlay) sidebarOverlay.classList.remove('active');
            // Don't reset overflow if modal is open
            if (!isModalOpen()) {
                document.body.style.overflow = '';
            }
        }
    });

    // Listen for modal events to release body overflow when sidebar closes
    // (covers legacy Bootstrap modals and SIMARC V4 modals, which dispatch sm:shown)
    function closeSidebarForModal() {
        // If sidebar was open when modal opened, ensure overflow is modal-manager managed
        if (window.innerWidth < 992 && sidebar && sidebar.classList.contains('active')) {
            sidebar.classList.remove('active');
            if (sidebarOverlay) sidebarOverlay.classList.remove('active');
        }
    }
    document.addEventListener('shown.bs.modal', closeSidebarForModal);
    document.addEventListener('sm:shown', closeSidebarForModal);

    // If the sidebar is still open when the last modal closes, re-apply the
    // body scroll lock (modal-manager just cleared it via sm:hidden).
    document.addEventListener('sm:hidden', function() {
        if (window.innerWidth < 992 && sidebar && sidebar.classList.contains('active')) {
            document.body.style.overflow = 'hidden';
        }
    });

    // Replace inline onclicks with event listeners if needed
    // (Optional, but better for CSP)
});
