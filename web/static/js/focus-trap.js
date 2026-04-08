/**
 * focus-trap.js — Automatic focus trapping for Bootstrap 5 modals.
 *
 * Listens for Bootstrap modal show/hide events and constrains Tab / Shift+Tab
 * focus cycling to the visible focusable elements within the open modal.
 * No per-modal setup is required — this attaches globally via event delegation.
 */
(function () {
    "use strict";

    const FOCUSABLE = [
        'a[href]:not([disabled]):not([tabindex="-1"])',
        'button:not([disabled]):not([tabindex="-1"])',
        'input:not([disabled]):not([type="hidden"]):not([tabindex="-1"])',
        'select:not([disabled]):not([tabindex="-1"])',
        'textarea:not([disabled]):not([tabindex="-1"])',
        '[tabindex]:not([tabindex="-1"])',
        '[contenteditable="true"]',
    ].join(", ");

    /** Return visible, focusable elements within a container */
    function getFocusable(container) {
        return Array.from(container.querySelectorAll(FOCUSABLE)).filter(
            el => el.offsetParent !== null && getComputedStyle(el).visibility !== "hidden"
        );
    }

    function handleKeydown(e) {
        if (e.key !== "Tab") return;

        const modal = e.currentTarget;
        const focusable = getFocusable(modal);
        if (focusable.length === 0) return;

        const first = focusable[0];
        const last = focusable[focusable.length - 1];

        if (e.shiftKey) {
            // Shift+Tab: if on first element, wrap to last
            if (document.activeElement === first || !modal.contains(document.activeElement)) {
                e.preventDefault();
                last.focus();
            }
        } else {
            // Tab: if on last element, wrap to first
            if (document.activeElement === last || !modal.contains(document.activeElement)) {
                e.preventDefault();
                first.focus();
            }
        }
    }

    // Attach focus trap when any Bootstrap modal opens
    document.addEventListener("shown.bs.modal", function (e) {
        const modal = e.target;
        modal.addEventListener("keydown", handleKeydown);

        // Ensure focus starts inside the modal
        const focusable = getFocusable(modal);
        if (focusable.length > 0 && !modal.contains(document.activeElement)) {
            focusable[0].focus();
        }
    });

    // Remove focus trap when modal closes
    document.addEventListener("hidden.bs.modal", function (e) {
        e.target.removeEventListener("keydown", handleKeydown);
    });
})();
