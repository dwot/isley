// theme.js — which theme and which mode this page shows.
//
// A page has a theme (data-theme on <html>: "classic", "forest", ...) and a
// mode (data-bs-theme: "light" or "dark"). The server writes the instance's
// default theme on <html>, so the first paint is themed. This script runs
// in <head>, before the body exists, and applies what this browser chose
// instead, if anything:
//
//   localStorage "isley-theme-name"  a theme id; absent = instance default
//   localStorage "isley-theme"       "light" or "dark"; absent = follow the
//                                    system
//
// A theme may have one mode only; the page then uses that mode whatever
// was chosen.
//
// When either changes it fires "isley:themechange" on document, for
// anything that paints with theme colours itself (charts).
// IsleyTheme.color("--chart-1") resolves a token to a colour a canvas
// accepts.
(function () {
    const MODE_KEY = 'isley-theme';
    const THEME_KEY = 'isley-theme-name';
    const root = document.documentElement;
    const instanceDefault = root.getAttribute('data-theme-default') || 'classic';
    const systemDark = window.matchMedia('(prefers-color-scheme: dark)');

    // { id: ["light", "dark"] } from <meta name="isley-themes">.
    const themes = {};
    const meta = document.querySelector('meta[name="isley-themes"]');
    ((meta && meta.content) || '').split(';').forEach(function (entry) {
        const parts = entry.split(':');
        if (parts[0] && parts[1]) themes[parts[0]] = parts[1].split(' ');
    });
    if (!themes[instanceDefault]) themes[instanceDefault] = ['light', 'dark'];

    function read(key) {
        try { return localStorage.getItem(key); } catch (e) { return null; }
    }
    function write(key, value) {
        try {
            if (value) localStorage.setItem(key, value);
            else localStorage.removeItem(key);
        } catch (e) { /* private mode: the choice lasts for this page */ }
    }

    // What the browser asked for, and what the page shows given the themes
    // that exist.
    let chosenTheme = read(THEME_KEY);
    let chosenMode = read(MODE_KEY);

    function current() {
        const theme = themes[chosenTheme] ? chosenTheme : instanceDefault;
        const wanted = (chosenMode === 'light' || chosenMode === 'dark')
            ? chosenMode
            : (systemDark.matches ? 'dark' : 'light');
        const modes = themes[theme];
        return {
            theme: theme,
            mode: modes.indexOf(wanted) >= 0 ? wanted : modes[0],
            modes: modes,
            // '' and 'system' are "no choice made".
            themeChoice: themes[chosenTheme] ? chosenTheme : '',
            modeChoice: (chosenMode === 'light' || chosenMode === 'dark') ? chosenMode : 'system',
        };
    }

    function apply() {
        const now = current();
        const changed = root.getAttribute('data-theme') !== now.theme ||
            root.getAttribute('data-bs-theme') !== now.mode;
        root.setAttribute('data-theme', now.theme);
        root.setAttribute('data-bs-theme', now.mode);
        syncMenu(now);
        if (changed) document.dispatchEvent(new CustomEvent('isley:themechange', { detail: now }));
    }

    function syncMenu(now) {
        const icon = document.getElementById('themeToggleIcon');
        if (icon) {
            icon.classList.toggle('fa-moon', now.mode === 'dark');
            icon.classList.toggle('fa-sun', now.mode !== 'dark');
        }
        document.querySelectorAll('[data-theme-mode]').forEach(function (item) {
            const mode = item.getAttribute('data-theme-mode');
            const on = mode === now.modeChoice;
            item.classList.toggle('active', on);
            item.setAttribute('aria-pressed', on ? 'true' : 'false');
            // A one-mode theme has nothing to switch to.
            item.disabled = now.modes.length < 2 && mode !== now.mode;
        });
        document.querySelectorAll('[data-theme-id]').forEach(function (item) {
            const on = item.getAttribute('data-theme-id') === now.themeChoice;
            item.classList.toggle('active', on);
            item.setAttribute('aria-pressed', on ? 'true' : 'false');
        });
    }

    function setTheme(id) {
        chosenTheme = themes[id] ? id : null;
        write(THEME_KEY, chosenTheme);
        apply();
    }

    function setMode(mode) {
        chosenMode = (mode === 'light' || mode === 'dark') ? mode : null;
        write(MODE_KEY, chosenMode);
        apply();
    }

    // color resolves a token (or any CSS colour) to its computed value by
    // asking the browser, so mixed tokens come back as one colour.
    function color(token) {
        const probe = document.createElement('span');
        probe.style.color = token.indexOf('--') === 0 ? 'var(' + token + ')' : token;
        probe.style.display = 'none';
        (document.body || root).appendChild(probe);
        const value = getComputedStyle(probe).color;
        probe.remove();
        return value;
    }

    apply();

    systemDark.addEventListener('change', apply);
    // Another tab changed the choice.
    window.addEventListener('storage', function (e) {
        if (e.key !== MODE_KEY && e.key !== THEME_KEY) return;
        chosenTheme = read(THEME_KEY);
        chosenMode = read(MODE_KEY);
        apply();
    });

    document.addEventListener('DOMContentLoaded', function () {
        syncMenu(current());
        document.addEventListener('click', function (e) {
            const item = e.target.closest('[data-theme-mode], [data-theme-id]');
            if (!item || item.disabled) return;
            if (item.hasAttribute('data-theme-mode')) setMode(item.getAttribute('data-theme-mode'));
            else setTheme(item.getAttribute('data-theme-id'));
        });
    });

    window.IsleyTheme = { current: current, setTheme: setTheme, setMode: setMode, color: color };
})();
