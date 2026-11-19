// weight-units.js — harvest weights are stored in grams and may be shown in
// grams or ounces. Mirrors utils/weight.go so a value converted in the browser
// reads the same as one converted on the server.
(function () {
    const GRAMS_PER_OUNCE = 28.349523125;

    function round2(value) {
        return Math.round((value + Number.EPSILON) * 100) / 100;
    }

    function normalizeUnit(unit) {
        const u = String(unit || "").trim().toLowerCase();
        return (u === "oz" || u === "ounce" || u === "ounces") ? "oz" : "g";
    }

    // valueFromGrams converts a stored gram value to the display unit.
    function valueFromGrams(grams, unit) {
        const n = Number(grams) || 0;
        return normalizeUnit(unit) === "oz" ? round2(n / GRAMS_PER_OUNCE) : round2(n);
    }

    // format renders a stored gram value with its unit, e.g. "112.5 g".
    function format(grams, unit) {
        if (grams === null || grams === undefined || isNaN(Number(grams))) return "";
        const u = normalizeUnit(unit);
        return `${valueFromGrams(grams, u)} ${u}`;
    }

    // convert moves a value typed in one unit to another, so switching the
    // unit select in a form keeps the same physical weight.
    function convert(value, fromUnit, toUnit) {
        const n = Number(value);
        if (value === "" || !isFinite(n)) return value;
        const grams = normalizeUnit(fromUnit) === "oz" ? n * GRAMS_PER_OUNCE : n;
        return valueFromGrams(grams, toUnit);
    }

    const reduceMotion = window.matchMedia
        ? window.matchMedia("(prefers-reduced-motion: reduce)")
        : null;

    function prefersReducedMotion() {
        return Boolean(reduceMotion && reduceMotion.matches);
    }

    function setDisplay(el, grams, unit) {
        el.textContent = format(grams, unit);
        el.dataset.unit = normalizeUnit(unit);
    }

    // toggle flips an element marked with data-isley-weight between grams and
    // ounces, animating the text swap when the user allows motion.
    function toggle(el) {
        if (!el || el.dataset.swapping === "true") return;
        const grams = Number(el.dataset.grams);
        if (!isFinite(grams)) return;
        const to = normalizeUnit(el.dataset.unit) === "oz" ? "g" : "oz";

        if (prefersReducedMotion() || typeof el.animate !== "function") {
            setDisplay(el, grams, to);
            return;
        }

        el.dataset.swapping = "true";
        const out = el.animate(
            [
                { opacity: 1, transform: "translateY(0)" },
                { opacity: 0, transform: "translateY(-0.18em)" },
            ],
            { duration: 90, easing: "ease-in", fill: "forwards" }
        );
        out.onfinish = () => {
            setDisplay(el, grams, to);
            const back = el.animate(
                [
                    { opacity: 0, transform: "translateY(0.18em)" },
                    { opacity: 1, transform: "translateY(0)" },
                ],
                { duration: 140, easing: "ease-out", fill: "forwards" }
            );
            back.onfinish = () => {
                out.cancel();
                back.cancel();
                delete el.dataset.swapping;
            };
            back.oncancel = () => {
                out.cancel();
                delete el.dataset.swapping;
            };
        };
        out.oncancel = () => {
            delete el.dataset.swapping;
        };
    }

    function weightTarget(node) {
        return node instanceof Element ? node.closest("[data-isley-weight]") : null;
    }

    document.addEventListener("click", (event) => {
        const el = weightTarget(event.target);
        if (!el) return;
        // Capture keeps the click from reaching row/card navigation handlers.
        event.stopPropagation();
        toggle(el);
    }, true);

    document.addEventListener("keydown", (event) => {
        if (event.key !== "Enter" && event.key !== " ") return;
        const el = weightTarget(event.target);
        if (!el) return;
        event.preventDefault();
        event.stopPropagation();
        toggle(el);
    }, true);

    window.IsleyWeight = { GRAMS_PER_OUNCE, normalizeUnit, valueFromGrams, format, convert, toggle };
})();
