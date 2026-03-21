/**
 * IsleyAutocomplete — Progressive enhancement that converts a standard <select>
 * into a searchable autocomplete dropdown, matching the pattern used in the
 * strain lineage editor.
 *
 * Usage:
 *   const ac = new IsleyAutocomplete(selectElement, {
 *       placeholder: "Search strains...",    // input placeholder
 *       sublabelKey: null,                   // optional: function(option) => sublabel string
 *       onSelect:    null,                   // optional: callback(value, label, isNew)
 *       allowNew:    true,                   // whether the "Add New" option is supported
 *       newValue:    "new",                  // the <option> value that triggers "add new"
 *       minChars:    1,                      // minimum chars before showing suggestions
 *       maxResults:  10,                     // max dropdown items
 *   });
 *
 *   // Programmatic control:
 *   ac.reset();           // clear the input + selection
 *   ac.setValue(id);       // select an option by value
 *   ac.getValue();         // get current select value
 *   ac.destroy();          // tear down and restore original select
 */
class IsleyAutocomplete {
    constructor(selectEl, opts = {}) {
        if (!selectEl || selectEl.tagName !== "SELECT") {
            console.warn("IsleyAutocomplete: expected a <select> element", selectEl);
            return;
        }

        this.select = selectEl;
        this.opts = Object.assign({
            placeholder: "Type to search...",
            sublabelKey: null,
            onSelect: null,
            allowNew: true,
            newValue: "new",
            minChars: 1,
            maxResults: 10,
        }, opts);

        this._items = [];
        this._newLabel = "";
        this._built = false;
        this._build();
    }

    /* ---- public API ---- */

    /** Reset to blank / no selection */
    reset() {
        this.input.value = "";
        this.select.value = "";
        this._hideDropdown();
        // Fire change so dependent logic runs
        this.select.dispatchEvent(new Event("change", { bubbles: true }));
    }

    /** Programmatically select a value */
    setValue(val) {
        const item = this._items.find(i => String(i.value) === String(val));
        if (item) {
            this.input.value = item.label;
            this.select.value = item.value;
        } else if (val === this.opts.newValue && this.opts.allowNew) {
            this.input.value = "";
            this.select.value = this.opts.newValue;
        } else {
            this.select.value = val;
            const opt = this.select.querySelector(`option[value="${val}"]`);
            if (opt) this.input.value = opt.textContent.trim();
        }
        this.select.dispatchEvent(new Event("change", { bubbles: true }));
    }

    /** Get the current select value */
    getValue() {
        return this.select.value;
    }

    /** Tear down and restore original select visibility */
    destroy() {
        if (!this._built) return;
        if (this.wrapper && this.wrapper.parentNode) {
            this.wrapper.parentNode.insertBefore(this.select, this.wrapper);
            this.wrapper.remove();
        }
        this.select.style.display = "";
        this.select.removeAttribute("tabindex");
        this._built = false;
    }

    /** Refresh items list from the current select options (call after dynamically adding options) */
    refreshItems() {
        this._extractItems();
    }

    /* ---- private ---- */

    _build() {
        this._extractItems();

        // Hide the original <select> but keep it in the DOM for form submission
        this.select.style.display = "none";
        this.select.setAttribute("tabindex", "-1");

        // Create wrapper
        this.wrapper = document.createElement("div");
        this.wrapper.className = "isley-ac-wrapper position-relative";

        // Text input
        this.input = document.createElement("input");
        this.input.type = "text";
        this.input.className = "form-control isley-ac-input";
        this.input.placeholder = this.opts.placeholder;
        this.input.autocomplete = "off";

        // Mirror the required attribute so the visible input validates
        if (this.select.required) {
            this.input.required = true;
            this.select.required = false; // prevent hidden-field validation quirks
        }

        // If the select has a pre-selected value, show it
        const preselected = this.select.value;
        if (preselected && preselected !== "" && preselected !== this.opts.newValue) {
            const item = this._items.find(i => String(i.value) === String(preselected));
            if (item) {
                this.input.value = item.label;
            }
        }

        // Dropdown container
        this.dropdown = document.createElement("div");
        this.dropdown.className = "isley-ac-dropdown";

        this.wrapper.appendChild(this.input);
        this.wrapper.appendChild(this.dropdown);

        // Insert wrapper right after the hidden select
        this.select.parentNode.insertBefore(this.wrapper, this.select.nextSibling);

        // ---- Event listeners ----

        let debounceTimer = null;

        this.input.addEventListener("input", () => {
            clearTimeout(debounceTimer);
            const query = this.input.value.trim();

            if (query.length < this.opts.minChars) {
                this._hideDropdown();
                // If the field is cleared, reset selection
                if (query.length === 0) {
                    this.select.value = "";
                    this.select.dispatchEvent(new Event("change", { bubbles: true }));
                }
                return;
            }

            debounceTimer = setTimeout(() => {
                const matches = this._search(query);
                this._renderDropdown(matches, query);
            }, 150);
        });

        this.input.addEventListener("focus", () => {
            const query = this.input.value.trim();
            if (query.length >= this.opts.minChars) {
                const matches = this._search(query);
                this._renderDropdown(matches, query);
            } else if (query.length === 0) {
                // Show all items on focus when empty
                this._renderDropdown(this._items.slice(0, this.opts.maxResults), "");
            }
        });

        this.input.addEventListener("blur", () => {
            setTimeout(() => {
                this._hideDropdown();
                // If the user typed something but didn't select, try to match
                this._resolveInput();
            }, 200);
        });

        // Keyboard navigation
        this.input.addEventListener("keydown", (e) => {
            if (this.dropdown.style.display !== "block") return;
            const items = this.dropdown.querySelectorAll(".isley-ac-item");
            let active = this.dropdown.querySelector(".isley-ac-item.active");
            let idx = Array.from(items).indexOf(active);

            if (e.key === "ArrowDown") {
                e.preventDefault();
                idx = Math.min(idx + 1, items.length - 1);
                this._setActiveItem(items, idx);
            } else if (e.key === "ArrowUp") {
                e.preventDefault();
                idx = Math.max(idx - 1, 0);
                this._setActiveItem(items, idx);
            } else if (e.key === "Enter") {
                e.preventDefault();
                if (active) {
                    active.dispatchEvent(new Event("mousedown"));
                }
            } else if (e.key === "Escape") {
                this._hideDropdown();
            }
        });

        this._built = true;
    }

    _extractItems() {
        this._items = [];
        const options = this.select.querySelectorAll("option");
        options.forEach(opt => {
            const val = opt.value;
            if (!val || val === this.opts.newValue) {
                if (val === this.opts.newValue) {
                    this._newLabel = opt.textContent.trim();
                }
                return;
            }
            // Skip disabled placeholder options (e.g. "Select strain...")
            if (opt.disabled) return;

            const label = this.opts.sublabelKey
                ? this.opts.sublabelKey(opt)
                : opt.textContent.trim();
            this._items.push({
                value: val,
                label: opt.textContent.trim(),
                displayLabel: label,
            });
        });
    }

    _search(query) {
        const lower = query.toLowerCase();
        return this._items.filter(item =>
            item.label.toLowerCase().includes(lower)
        ).slice(0, this.opts.maxResults);
    }

    _renderDropdown(matches, query) {
        this.dropdown.innerHTML = "";

        if (matches.length === 0 && !this.opts.allowNew) {
            this._hideDropdown();
            return;
        }

        matches.forEach(item => {
            const div = document.createElement("div");
            div.className = "isley-ac-item";
            div.innerHTML = this._highlightMatch(item.label, query);
            div.addEventListener("mousedown", (e) => {
                e.preventDefault();
                this._selectItem(item);
            });
            this.dropdown.appendChild(div);
        });

        // "Add New" option at the bottom if enabled
        if (this.opts.allowNew && this._newLabel) {
            const sep = document.createElement("div");
            sep.className = "isley-ac-separator";
            this.dropdown.appendChild(sep);

            const addNew = document.createElement("div");
            addNew.className = "isley-ac-item isley-ac-new-item";
            addNew.innerHTML = `<i class="fa-solid fa-plus me-1"></i> ${this._escapeHtml(this._newLabel)}`;
            addNew.addEventListener("mousedown", (e) => {
                e.preventDefault();
                this._selectNew();
            });
            this.dropdown.appendChild(addNew);
        }

        this.dropdown.style.display = "block";
    }

    _selectItem(item) {
        this.input.value = item.label;
        this.select.value = item.value;
        this._hideDropdown();
        this.select.dispatchEvent(new Event("change", { bubbles: true }));
        if (this.opts.onSelect) {
            this.opts.onSelect(item.value, item.label, false);
        }
    }

    _selectNew() {
        this.input.value = this._newLabel;
        this.select.value = this.opts.newValue;
        this._hideDropdown();
        this.select.dispatchEvent(new Event("change", { bubbles: true }));
        if (this.opts.onSelect) {
            this.opts.onSelect(this.opts.newValue, this._newLabel, true);
        }
    }

    /** After blur, if the input text doesn't match the current selection, try to resolve */
    _resolveInput() {
        const text = this.input.value.trim();
        if (!text) return;

        const currentVal = this.select.value;
        const currentItem = this._items.find(i => String(i.value) === String(currentVal));

        // If current selection matches the input text, we're good
        if (currentItem && currentItem.label === text) return;

        // Try exact match
        const exact = this._items.find(i => i.label.toLowerCase() === text.toLowerCase());
        if (exact) {
            this._selectItem(exact);
        }
        // Otherwise leave the input as-is but don't change the select
        // (user might be mid-typing; the form validation will catch it)
    }

    _hideDropdown() {
        this.dropdown.style.display = "none";
        this.dropdown.innerHTML = "";
    }

    _setActiveItem(items, idx) {
        items.forEach(i => i.classList.remove("active"));
        if (items[idx]) {
            items[idx].classList.add("active");
            items[idx].scrollIntoView({ block: "nearest" });
        }
    }

    _highlightMatch(label, query) {
        if (!query) return this._escapeHtml(label);
        const escaped = this._escapeHtml(label);
        const regex = new RegExp(`(${this._escapeRegex(query)})`, "gi");
        return escaped.replace(regex, "<strong>$1</strong>");
    }

    _escapeHtml(str) {
        const div = document.createElement("div");
        div.textContent = str;
        return div.innerHTML;
    }

    _escapeRegex(str) {
        return str.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
    }
}
