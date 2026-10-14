/**
 * IsleySortableTable — Synchronizes clickable <th> column-header sorting
 * with a companion "Sort by" <select> dropdown, matching the pattern
 * used across the plants, strains, sensors, and activities list views.
 *
 * Usage:
 *   const sortableTable = new IsleySortableTable(sortBySelectEl, {
 *       prefix:           "pt",     // CSS class prefix, e.g. ".pt-sortable"
 *       headerToDropdown: {},       // data-sort key -> { asc, desc } dropdown option values
 *       dropdownToHeader: {},       // dropdown value prefix -> data-sort key, [keys], or
 *                                   //   { key, invert: true } when the dropdown option is the
 *                                   //   reverse of the header (e.g. "sativa-desc" == indica asc)
 *       onChange:         null,     // callback(state) -> call your applyFilters()
 *   });
 *   Any headerToDropdown / dropdownToHeader entry may be a function returning the entry,
 *   for columns that mean different things in different views.
 *
 *   // Programmatic control:
 *   sortableTable.getSort();               // { key, asc } - single source of truth for applyFilters()
 *   sortableTable.resetToDefault("name-asc"); // used by "Clear filters" buttons / view switches
 *
 * Pass `null` as the first argument for header-only tables with no
 * companion dropdown (e.g. activities) - header-click sorting still works,
 * dropdown sync is just skipped.
 */
class IsleySortableTable {
    constructor(dropdownEl, opts = {}) {
        this.opts = Object.assign({
            prefix: "",
            headerToDropdown: {},
            dropdownToHeader: {},
            onChange: null,
        }, opts);

        if (!this.opts.prefix) {
            console.warn("IsleySortableTable: no `prefix` provided");
        }

        this.dropdown = dropdownEl || null;
        this.state = { key: null, asc: true };

        this._headers = document.querySelectorAll(`.${this.opts.prefix}-sortable`);
        this._bindHeaders();
        if (this.dropdown) { this._bindDropdown(); this._syncIconsFromDropdown(); }
    }

    /* ---- public API ---- */

    /** Single source of truth each page's applyFilters() should read. */
    getSort() {
        if (this.state.key) return { key: this.state.key, asc: this.state.asc };
        if (this.dropdown) {
            const [k, d] = this.dropdown.value.split("-");
            return { key: k, asc: d === "asc" };
        }
        return { key: null, asc: true };
    }

    /** Used by "Clear filters" buttons and view-switch handlers. */
    resetToDefault(defaultDropdownValue) {
        this.state = { key: null, asc: true };
        if (this.dropdown && defaultDropdownValue) {
            this.dropdown.value = defaultDropdownValue;
        }
        this._syncIconsFromDropdown();
    }

    /* ---- private ---- */

    // A mapping entry may be a function, for pages whose columns change meaning per view.
    _resolve(entry) {
        return typeof entry === "function" ? entry() : entry;
    }

    _syncIconsFromDropdown() {
        this._resetIcons();
        if (!this.dropdown) return;
        const [key, dir] = this.dropdown.value.split("-");
        const asc = dir === "asc";
        const mapped = this._resolve(this.opts.dropdownToHeader[key]);
        if (!mapped) return;
        (Array.isArray(mapped) ? mapped : [mapped]).forEach(m => {
            const target = typeof m === "string" ? { key: m } : m;
            this._setIcon(target.key, target.invert ? !asc : asc);
        });
    }

    _resetIcons() {
        document.querySelectorAll(`.${this.opts.prefix}-sortable i`).forEach(icon => {
            icon.className = "fa-solid fa-sort ms-1 text-muted";
        });
    }

    _setIcon(key, asc) {
        const th = document.querySelector(`.${this.opts.prefix}-sortable[data-sort="${key}"]`);
        const icon = th && th.querySelector("i");
        if (icon) icon.className = `fa-solid fa-sort-${asc ? "up" : "down"} ms-1`;
    }

    _bindHeaders() {
        this._headers.forEach(th => {
            th.style.cursor = "pointer";
            th.addEventListener("click", () => {
                const key = th.dataset.sort;
                const icon = th.querySelector("i");
                const current = this.state.key === key ? this.state.asc
                    : icon && icon.classList.contains("fa-sort-up") ? true
                    : icon && icon.classList.contains("fa-sort-down") ? false
                    : null;
                this.state.key = key;
                this.state.asc = current === null ? true : !current;

                this._resetIcons();
                this._setIcon(key, this.state.asc);

                if (this.dropdown) {
                    const mapped = this._resolve(this.opts.headerToDropdown[key]);
                    const value = mapped && (this.state.asc ? mapped.asc : mapped.desc);
                    if (value && this.dropdown.querySelector(`option[value="${value}"]`)) {
                        this.dropdown.value = value;
                    }
                }

                if (this.opts.onChange) this.opts.onChange(this.state);
            });
        });
    }

    _bindDropdown() {
        this.dropdown.addEventListener("change", () => {
            this.state = { key: null, asc: true };
            this._syncIconsFromDropdown();

            if (this.opts.onChange) this.opts.onChange(this.state);
        });
    }
}
