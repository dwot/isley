/**
 * list-utils.js — Shared utilities for list/filter/sort views
 * (plants, strains, sensors, activities)
 */
(function() {
    'use strict';

    /**
     * Escape HTML to prevent XSS
     */
    function esc(str) {
        if (str == null) return '';
        const d = document.createElement('div');
        d.textContent = String(str);
        return d.innerHTML;
    }

    window.esc = esc;

    /**
     * Debounce function calls
     */
    function debounce(fn, ms) {
        let t;
        return function() {
            const args = arguments;
            const ctx = this;
            clearTimeout(t);
            t = setTimeout(function() {
                fn.apply(ctx, args);
            }, ms);
        };
    }

    window.debounce = debounce;

    /**
     * Format number with pluralization
     */
    function pluralize(count, singular, plural) {
        plural = plural || singular + 's';
        return count + ' ' + (count === 1 ? singular : plural);
    }

    /**
     * Populate a filter dropdown from unique values in data
     */
    function populateFilter(selectEl, data, key, currentValue, labelFn) {
        if (!selectEl) return;
        labelFn = labelFn || function(v) { return v; };
        const values = [];
        const seen = new Set();
        (data || []).forEach(function(item) {
            const v = item[key];
            if (v != null && v !== '' && !seen.has(v)) {
                seen.add(v);
                values.push(v);
            }
        });
        values.sort();
        
        const current = currentValue !== undefined ? currentValue : selectEl.value;
        selectEl.innerHTML = '<option value="">' + (selectEl.getAttribute('data-all-label') || 'All') + '</option>';
        values.forEach(function(v) {
            const opt = document.createElement('option');
            opt.value = v;
            opt.textContent = labelFn(v);
            if (v === current) opt.selected = true;
            selectEl.appendChild(opt);
        });
    }

    /**
     * Clear all filter inputs in a container
     */
    function clearFilters(config) {
        if (config.searchInput) config.searchInput.value = '';
        (config.selects || []).forEach(function(sel) {
            if (sel) sel.value = '';
        });
        (config.inputs || []).forEach(function(inp) {
            if (inp) inp.value = '';
        });
        if (config.sortableTable && config.defaultSort) {
            config.sortableTable.resetToDefault(config.defaultSort);
        }
        if (config.onClear) config.onClear();
    }

    /**
     * Simple list filter/sort helper
     */
    const list = {
        filterByText: function(data, query, fields) {
            if (!query) return data;
            query = query.toLowerCase();
            return data.filter(function(item) {
                for (var i = 0; i < fields.length; i++) {
                    var v = item[fields[i]];
                    if (v && String(v).toLowerCase().indexOf(query) !== -1) {
                        return true;
                    }
                }
                return false;
            });
        },
        
        filterBySelect: function(data, value, field) {
            if (!value) return data;
            return data.filter(function(item) {
                return item[field] === value;
            });
        },
        
        sort: function(data, key, asc, getValue) {
            var sorted = data.slice();
            getValue = getValue || function(item, k) { return item[k]; };
            sorted.sort(function(a, b) {
                var va = getValue(a, key);
                var vb = getValue(b, key);
                if (va == null && vb == null) return 0;
                if (va == null) return asc ? -1 : 1;
                if (vb == null) return asc ? 1 : -1;
                if (typeof va === 'string') {
                    va = va.toLowerCase();
                    vb = String(vb).toLowerCase();
                    return asc ? va.localeCompare(vb) : vb.localeCompare(va);
                }
                if (va < vb) return asc ? -1 : 1;
                if (va > vb) return asc ? 1 : -1;
                return 0;
            });
            return sorted;
        }
    };

    /**
     * Paging for a list that is filtered and sorted in the browser.
     *
     *   const pager = list.pager(navEl, { pageSize: 50, onChange: draw });
     *   pager.reset(rows.length);        // after a filter or sort: back to page 1
     *   draw(pager.slice(rows));         // the rows of the current page
     *
     * For a list the server pages, say where things stand after each
     * fetch instead, and fetch again from onChange:
     *
     *   pager.set(total, page);
     *
     * navEl is an empty <nav>; it is hidden while everything fits on one
     * page. onChange(page) runs when the visitor moves to another page.
     */
    list.pager = function(navEl, opts) {
        var pageSize = (opts && opts.pageSize) || 50;
        var onChange = (opts && opts.onChange) || function() {};
        var page = 1;
        var total = 0;

        function pages() { return Math.max(1, Math.ceil(total / pageSize)); }

        function render() {
            if (!navEl) return;
            var n = pages();
            navEl.hidden = n < 2;
            if (n < 2) { navEl.innerHTML = ''; return; }
            var t = window.uiMessages ? window.uiMessages.t : function(k, f) { return f; };
            var status = t('pager_status', 'Page {page} of {pages}').replace('{page}', page).replace('{pages}', n);
            navEl.innerHTML =
                '<button type="button" class="btn btn-sm btn-outline-secondary" data-page="' + (page - 1) + '"' + (page <= 1 ? ' disabled' : '') +
                    ' aria-label="' + esc(t('pager_previous', 'Previous page')) + '"><i class="fa-solid fa-chevron-left" aria-hidden="true"></i></button>' +
                '<span class="list-pager-status" aria-live="polite">' + esc(status) + '</span>' +
                '<button type="button" class="btn btn-sm btn-outline-secondary" data-page="' + (page + 1) + '"' + (page >= n ? ' disabled' : '') +
                    ' aria-label="' + esc(t('pager_next', 'Next page')) + '"><i class="fa-solid fa-chevron-right" aria-hidden="true"></i></button>';
        }

        if (navEl) {
            navEl.addEventListener('click', function(e) {
                var btn = e.target.closest('[data-page]');
                if (!btn || btn.disabled) return;
                page = Math.min(pages(), Math.max(1, parseInt(btn.getAttribute('data-page'), 10) || 1));
                render();
                onChange(page);
            });
        }

        return {
            reset: function(count) { total = count; page = 1; render(); },
            set: function(count, current) { total = count; page = Math.min(pages(), Math.max(1, current || 1)); render(); },
            slice: function(rows) { return rows.slice((page - 1) * pageSize, page * pageSize); },
            page: function() { return page; },
        };
    };

    // The "Filters" button on small screens shows how many filters are set,
    // since the filters themselves are folded away there. A filter counts
    // when it is off its first option (selects) or has a value (inputs).
    document.addEventListener('DOMContentLoaded', function() {
        document.querySelectorAll('.list-filters-toggle').forEach(function(toggle) {
            var row = document.querySelector(toggle.getAttribute('data-bs-target'));
            var badge = toggle.querySelector('.list-filters-count');
            if (!row || !badge) return;
            function sync() {
                var n = 0;
                row.querySelectorAll('select, input').forEach(function(el) {
                    if (el.tagName === 'SELECT' ? el.selectedIndex > 0 : !!el.value) n++;
                });
                badge.textContent = n;
                badge.hidden = n === 0;
            }
            row.addEventListener('change', sync);
            // "Clear" sets values from script, which fires no change event.
            row.addEventListener('click', function() { setTimeout(sync, 0); });
            sync();
        });
    });

    window.list = list;
    window.pluralize = pluralize;
    window.populateFilter = populateFilter;
    window.clearFilters = clearFilters;
})();
