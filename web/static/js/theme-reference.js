// theme-reference.js — Settings > Appearance > Theme tokens.
//
// The page lists every token a theme package may set. This script shows
// each one as the theme on screen has it: a preview, its value, and for
// the text-on-surface pairs the contrast between them. Switching theme or
// mode in the header redraws it, so the page doubles as a way to look a
// theme over.
document.addEventListener('DOMContentLoaded', () => {
    const page = document.getElementById('themeReference');
    if (!page) return;
    const min = parseFloat(page.dataset.minContrast) || 4.5;

    // Previews point at the token itself, so they follow the theme with
    // no help; only the text below needs redrawing.
    page.querySelectorAll('.theme-ref-preview').forEach((el) => {
        const token = 'var(' + el.dataset.token + ')';
        switch (el.dataset.kind) {
            case 'color': el.style.backgroundColor = token; break;
            case 'image': el.style.backgroundImage = token; break;
            case 'shadow': el.style.boxShadow = token; break;
            case 'radius': el.style.borderRadius = token; break;
            case 'font': el.style.fontFamily = token; el.textContent = 'Aa'; break;
            case 'font-size': el.style.fontSize = token; el.textContent = 'Aa'; break;
            default: el.textContent = '%';
        }
    });
    page.querySelectorAll('[data-pair-text]').forEach((row) => {
        const sample = row.querySelector('.theme-ref-sample');
        sample.style.color = 'var(' + row.dataset.pairText + ')';
        sample.style.backgroundColor = 'var(' + row.dataset.pairBackground + ')';
    });

    // "rgb(1, 2, 3)", "rgba(1, 2, 3, 0.5)" or "color(srgb 0.1 0.2 0.3 / 0.5)"
    // to [r, g, b, a], each 0..1.
    function parse(css) {
        const nums = (css.match(/-?[\d.]+(?:e-?\d+)?%?/g) || []).map((n) => n.endsWith('%') ? parseFloat(n) / 100 : parseFloat(n));
        if (nums.length < 3) return null;
        const scale = css.indexOf('color(') === 0 ? 1 : 255;
        return [nums[0] / scale, nums[1] / scale, nums[2] / scale, nums.length > 3 ? nums[3] : 1];
    }
    function luminance(c) {
        const lin = (v) => v <= 0.04045 ? v / 12.92 : Math.pow((v + 0.055) / 1.055, 2.4);
        return 0.2126 * lin(c[0]) + 0.7152 * lin(c[1]) + 0.0722 * lin(c[2]);
    }
    function ratio(text, bg) {
        const a = text[3];
        const landed = [0, 1, 2].map((i) => text[i] * a + bg[i] * (1 - a));
        const l1 = luminance(landed), l2 = luminance(bg);
        return (Math.max(l1, l2) + 0.05) / (Math.min(l1, l2) + 0.05);
    }

    function draw() {
        const now = IsleyTheme.current();
        const item = document.querySelector('#themeMenu [data-theme-id="' + now.theme + '"]');
        const modeItem = document.querySelector('#themeMenu [data-theme-mode="' + now.mode + '"]');
        document.getElementById('themeRefCurrent').textContent =
            (item ? item.textContent.trim() : now.theme) + (modeItem ? ' · ' + modeItem.textContent.trim() : '');

        const style = getComputedStyle(document.documentElement);
        page.querySelectorAll('[data-value-of]').forEach((el) => {
            el.textContent = style.getPropertyValue(el.dataset.valueOf).trim().replace(/\s+/g, ' ');
        });
        page.querySelectorAll('[data-pair-text]').forEach((row) => {
            const out = row.querySelector('.theme-ref-ratio');
            const text = parse(IsleyTheme.color(row.dataset.pairText));
            const bg = parse(IsleyTheme.color(row.dataset.pairBackground));
            out.classList.remove('is-low');
            if (!text || !bg || bg[3] < 1) { out.textContent = '–'; return; }
            const r = ratio(text, bg);
            // Cut, not rounded: 4.49 must not read as 4.5.
            out.textContent = (Math.floor(r * 10) / 10).toFixed(1) + ':1';
            const low = r < min;
            out.classList.toggle('is-low', low);
            out.title = low ? uiMessages.t('theme_contrast_low', 'Below the mark') : '';
            if (low) {
                const icon = document.createElement('i');
                icon.className = 'fa fa-triangle-exclamation me-1';
                icon.setAttribute('aria-hidden', 'true');
                out.prepend(icon);
            }
        });
    }

    draw();
    document.addEventListener('isley:themechange', draw);
});
