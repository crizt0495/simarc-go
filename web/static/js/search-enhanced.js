/**
 * ArsipPro Enterprise - Enhanced Search Functionality
 * 
 * Features:
 * - Search suggestions/autocomplete
 * - Search result export
 * - Copy search URL
 * - Saved searches
 * - Search analytics tracking
 */

(function() {
    'use strict';

    // ============================================
    // SEARCH SUGGESTIONS / AUTOCOMPLETE
    // ============================================
    
    const searchInput = document.querySelector('input[name="search"]');
    let debounceTimer;
    let suggestionsContainer = null;

    if (searchInput) {
        // Create suggestions container
        suggestionsContainer = document.createElement('div');
        suggestionsContainer.className = 'search-suggestions-container';
        suggestionsContainer.style.cssText = `
            position: absolute;
            z-index: 1050;
            width: 100%;
            max-height: 400px;
            overflow-y: auto;
            background: white;
            border: 1px solid #dee2e6;
            border-radius: 0.5rem;
            box-shadow: 0 0.5rem 1rem rgba(0, 0, 0, 0.15);
            margin-top: 0.25rem;
            display: none;
        `;
        
        // Wrap search input in position-relative container
        const inputGroup = searchInput.parentElement;
        inputGroup.style.position = 'relative';
        inputGroup.appendChild(suggestionsContainer);

        // Input event with debounce
        searchInput.addEventListener('input', function() {
            const query = this.value.trim();
            
            // Clear previous timer
            clearTimeout(debounceTimer);
            
            // Hide suggestions if query is too short
            if (query.length < 2) {
                hideSuggestions();
                return;
            }
            
            // Debounce 300ms
            debounceTimer = setTimeout(async () => {
                await fetchSuggestions(query);
            }, 300);
        });

        // Hide suggestions on click outside
        document.addEventListener('click', function(e) {
            if (!inputGroup.contains(e.target)) {
                hideSuggestions();
            }
        });

        // Handle keyboard navigation
        searchInput.addEventListener('keydown', function(e) {
            const suggestions = suggestionsContainer.querySelectorAll('.search-suggestion-item');
            const active = suggestionsContainer.querySelector('.search-suggestion-item.active');
            
            if (e.key === 'ArrowDown') {
                e.preventDefault();
                const next = active ? active.nextElementSibling : suggestions[0];
                if (next) {
                    setActiveSuggestion(next);
                }
            } else if (e.key === 'ArrowUp') {
                e.preventDefault();
                const prev = active ? active.previousElementSibling : suggestions[suggestions.length - 1];
                if (prev) {
                    setActiveSuggestion(prev);
                }
            } else if (e.key === 'Enter') {
                if (active) {
                    e.preventDefault();
                    active.click();
                }
            } else if (e.key === 'Escape') {
                hideSuggestions();
            }
        });
    }

    async function fetchSuggestions(query) {
        try {
            const response = await fetch(`/api/arsip/search/suggestions?q=${encodeURIComponent(query)}`);
            const data = await response.json();
            
            if (data.success && data.suggestions.length > 0) {
                renderSuggestions(data.suggestions);
            } else {
                hideSuggestions();
            }
        } catch (error) {
            console.error('Error fetching suggestions:', error);
            hideSuggestions();
        }
    }

    function renderSuggestions(suggestions) {
        suggestionsContainer.innerHTML = '';
        
        suggestions.forEach(item => {
            const div = document.createElement('div');
            div.className = 'search-suggestion-item';
            div.style.cssText = `
                padding: 0.75rem 1rem;
                cursor: pointer;
                border-bottom: 1px solid #f8f9fa;
                display: flex;
                align-items: center;
                gap: 0.75rem;
            `;
            
            div.innerHTML = `
                <i class="bi bi-${item.icon}" style="font-size: 1.25rem; color: #0d6efd;"></i>
                <div style="flex: 1;">
                    <div style="font-weight: 600; color: #212529;">${escapeHtml(item.value)}</div>
                    <div style="font-size: 0.875rem; color: #6c757d;">${escapeHtml(item.highlight)}</div>
                </div>
                <span class="badge bg-light text-dark">${escapeHtml(item.type)}</span>
            `;
            
            div.addEventListener('mouseenter', function() {
                setActiveSuggestion(this);
            });
            
            div.addEventListener('click', function() {
                selectSuggestion(item);
            });
            
            suggestionsContainer.appendChild(div);
        });
        
        suggestionsContainer.style.display = 'block';
    }

    function setActiveSuggestion(element) {
        const active = suggestionsContainer.querySelector('.search-suggestion-item.active');
        if (active) active.classList.remove('active');
        
        element.classList.add('active');
        element.style.backgroundColor = '#e9ecef';
        
        // Scroll into view if needed
        element.scrollIntoView({ block: 'nearest' });
    }

    function selectSuggestion(item) {
        searchInput.value = item.value;
        hideSuggestions();
        
        // Navigate to URL or submit search
        if (item.url) {
            window.location.href = item.url;
        } else {
            searchInput.form.submit();
        }
    }

    function hideSuggestions() {
        if (suggestionsContainer) {
            suggestionsContainer.style.display = 'none';
            suggestionsContainer.innerHTML = '';
        }
    }

    function escapeHtml(text) {
        const div = document.createElement('div');
        div.textContent = text;
        return div.innerHTML;
    }

    // ============================================
    // COPY SEARCH URL FEATURE
    // ============================================
    
    const copyUrlBtn = document.getElementById('copySearchUrlBtn');
    if (copyUrlBtn) {
        copyUrlBtn.addEventListener('click', async function() {
            try {
                await navigator.clipboard.writeText(window.location.href);
                
                // Show success feedback
                const originalText = this.innerHTML;
                this.innerHTML = '<i class="bi bi-check-circle"></i> URL copied!';
                this.classList.remove('btn-outline-primary');
                this.classList.add('btn-success');
                
                setTimeout(() => {
                    this.innerHTML = originalText;
                    this.classList.remove('btn-success');
                    this.classList.add('btn-outline-primary');
                }, 2000);
            } catch (error) {
                alert('Failed to copy URL. Please copy manually: ' + window.location.href);
            }
        });
    }

    // ============================================
    // EXPORT SEARCH RESULTS
    // ============================================
    
    const exportForm = document.getElementById('exportSearchForm');
    if (exportForm) {
        exportForm.addEventListener('submit', function(e) {
            e.preventDefault();
            
            const format = this.querySelector('select[name="export_format"]').value;
            const submitBtn = this.querySelector('button[type="submit"]');
            
            // Show loading state
            submitBtn.disabled = true;
            submitBtn.innerHTML = '<i class="bi bi-hourglass-split"></i> Generating...';
            
            // Create form with current search params
            const form = document.createElement('form');
            form.method = 'POST';
            form.action = this.action;
            
            // Add current search parameters
            const params = new URLSearchParams(window.location.search);
            params.forEach((value, key) => {
                const input = document.createElement('input');
                input.type = 'hidden';
                input.name = key;
                input.value = value;
                form.appendChild(input);
            });
            
            // Add export format
            const formatInput = document.createElement('input');
            formatInput.type = 'hidden';
            formatInput.name = 'export_format';
            formatInput.value = format;
            form.appendChild(formatInput);
            
            // Add CSRF token
            const csrfInput = document.createElement('input');
            csrfInput.type = 'hidden';
            csrfInput.name = '_token';
            csrfInput.value = document.querySelector('meta[name="csrf-token"]')?.content || '';
            form.appendChild(csrfInput);
            
            document.body.appendChild(form);
            form.submit();
            
            // Reset button state
            setTimeout(() => {
                submitBtn.disabled = false;
                submitBtn.innerHTML = '<i class="bi bi-download"></i> Export';
            }, 1000);
        });
    }

    // ============================================
    // SEARCH TIPS (Show on focus)
    // ============================================
    
    const searchTips = document.getElementById('searchTips');
    if (searchTips && searchInput) {
        searchInput.addEventListener('focus', () => {
            searchTips.style.display = 'block';
        });
        
        searchInput.addEventListener('blur', () => {
            setTimeout(() => {
                searchTips.style.display = 'none';
            }, 200);
        });
    }

})();
