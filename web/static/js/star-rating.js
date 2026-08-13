// Star-rating widget shared by the smoke-report modals.
//
// The markup is a plain radio group (see the smoke-report modal templates), so
// keyboard navigation, form.reset() and screen-reader announcements all come
// from the browser rather than from ARIA bolted on afterwards. This file only
// adds the two things radios cannot express on their own: filling every star up
// to the selected one, and previewing a value on hover or focus.
//
// The fill is done here rather than in pure CSS on purpose. The usual CSS-only
// trick needs the radios in reverse DOM order (5 first) so that `:checked ~ label`
// can reach the lower values, and reversing the DOM reverses what the arrow keys
// do — pressing Right would lower the score. Keeping DOM order 1..5 and painting
// from JS costs a few lines and keeps the keyboard behaving as it looks.
(function () {
    // paint marks the first `upTo` stars of a group as lit. Passing 0 clears it.
    function paint(group, upTo) {
        group.querySelectorAll("label[data-star-value]").forEach((label) => {
            const v = parseInt(label.dataset.starValue, 10);
            label.classList.toggle("is-on", v <= upTo);
        });
    }

    // selectedValue returns the group's chosen score, or 0 when none is set.
    function selectedValue(group) {
        const checked = group.querySelector('input[type="radio"]:checked');
        return checked ? parseInt(checked.value, 10) : 0;
    }

    // repaint restores the group to its committed value, discarding any preview.
    function repaint(group) {
        paint(group, selectedValue(group));
    }

    function wire(group) {
        if (group.dataset.starWired === "true") return;
        group.dataset.starWired = "true";

        group.addEventListener("change", () => repaint(group));

        // Hover and keyboard focus both preview; leaving either restores the
        // committed value. focusin fires when arrow keys move between radios,
        // which also checks them, so change has already run by then — repainting
        // on focus is harmless and covers focus arriving without a change.
        group.addEventListener("mouseleave", () => repaint(group));
        group.addEventListener("focusout", () => repaint(group));

        group.querySelectorAll("label[data-star-value]").forEach((label) => {
            const v = parseInt(label.dataset.starValue, 10);
            label.addEventListener("mouseenter", () => paint(group, v));
        });

        repaint(group);
    }

    // wireAll is idempotent, so it is safe to call again after markup changes.
    function wireAll(root) {
        (root || document).querySelectorAll(".star-rating").forEach(wire);
    }

    // clear unchecks a group and repaints it. Used by the ✕ buttons beside the
    // optional per-criterion ratings; the overall rating has no clear button
    // because it is required.
    function clear(group) {
        if (!group) return;
        group.querySelectorAll('input[type="radio"]').forEach((input) => {
            input.checked = false;
        });
        paint(group, 0);
    }

    document.addEventListener("DOMContentLoaded", () => {
        wireAll(document);

        // Delegated so both modals are covered by one listener, and so a group
        // added to the DOM later still works.
        document.addEventListener("click", (e) => {
            const btn = e.target.closest(".star-clear");
            if (!btn) return;
            e.preventDefault();
            const group = document.getElementById(btn.dataset.starTarget);
            clear(group);
        });
    });

    window.starRating = {
        wireAll: wireAll,
        clear: clear,
        // set checks the radio for `value` (0 or a missing value clears the
        // group) and repaints. The edit modal uses this to prefill.
        set: function (group, value) {
            if (!group) return;
            const v = parseInt(value, 10);
            if (!v) {
                clear(group);
                return;
            }
            const input = group.querySelector('input[type="radio"][value="' + v + '"]');
            if (input) {
                input.checked = true;
            }
            paint(group, v);
        },
        // value reads a group's current score, 0 when unset.
        value: selectedValue,
    };
})();
