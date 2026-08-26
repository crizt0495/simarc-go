/**
 * ArsipPro Sidebar JavaScript
 * Consolidated & Robust
 */

document.addEventListener('DOMContentLoaded', function() {
    const sidebar = document.querySelector('.sidebar');
    const sidebarOverlay = document.getElementById('sidebarOverlay');
    const sidebarToggle = document.getElementById('sidebarToggle');

    // Check if we're on mobile (Android-style bottom nav mode)
    function isMobile() {
        return window.innerWidth < 992;
    }

    // Robust Toggle Function
    window.toggleSidebar = function() {
        // On mobile, sidebar is hidden - use bottom nav instead
        if (isMobile()) return;
        
        if (!sidebar) return;
        
        sidebar.classList.toggle('active');
        if (sidebarOverlay) {
            sidebarOverlay.classList.toggle('active');
        }
        
        // Prevent body scroll when sidebar is active
        if (!isModalOpen()) {
            document.body.style.overflow = sidebar.classList.contains('active') ? 'hidden' : '';
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

    // Close on overlay click
    if (sidebarOverlay) {
        sidebarOverlay.addEventListener('click', function() {
            sidebar.classList.remove('active');
            sidebarOverlay.classList.remove('active');
            if (!isModalOpen()) {
                document.body.style.overflow = '';
            }
        });
    }

    // Handle Resize
    window.addEventListener('resize', function() {
        if (!isModalOpen()) {
            document.body.style.overflow = '';
        }
    });

    // Listen for modal events
    function closeSidebarForModal() {
        if (sidebar && sidebar.classList.contains('active')) {
            sidebar.classList.remove('active');
            if (sidebarOverlay) sidebarOverlay.classList.remove('active');
        }
    }
    document.addEventListener('shown.bs.modal', closeSidebarForModal);
    document.addEventListener('sm:shown', closeSidebarForModal);

    document.addEventListener('sm:hidden', function() {
        if (sidebar && sidebar.classList.contains('active')) {
            document.body.style.overflow = 'hidden';
        }
    });
});
