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

    window.list = list;
    window.pluralize = pluralize;
    window.populateFilter = populateFilter;
    window.clearFilters = clearFilters;
})();
