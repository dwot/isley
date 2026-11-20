// Regression test for Settings > Appearance. Switching the default theme
// saves it and reloads the page; that reload must come back to the tab the
// admin was on, not the first Settings tab.
const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

const jsDir = path.join(__dirname, '..', '..', 'web', 'static', 'js');
const source = ['settings-common.js', 'theme-settings.js']
    .map((name) => fs.readFileSync(path.join(jsDir, name), 'utf8'))
    .join('\n');

// The scripts run against the browser's DOM and fetch. Only those boundaries
// are faked; the tab-saving and tab-restoring code under test is the real
// code shipped to the browser.
function harness(themeChoice) {
    const domReady = [];
    const shownTabs = [];
    const reloads = [];
    const store = new Map();

    function option(id, selected) {
        const el = {
            dataset: { defaultTheme: id },
            selected,
            setAttribute() {},
        };
        el.classList = {
            contains: (name) => name === 'is-selected' && el.selected,
            toggle: (name, on) => { if (name === 'is-selected') el.selected = on; },
        };
        return el;
    }

    const options = {
        classic: option('classic', true),
        walnut: option('walnut', false),
    };
    const pickerHandlers = [];
    const picker = {
        addEventListener(type, fn) { if (type === 'click') pickerHandlers.push(fn); },
        querySelectorAll() { return Object.values(options); },
    };
    const activeTabButton = {
        getAttribute(name) { return name === 'data-bs-target' ? '#logo-content' : null; },
    };
    const appearanceTab = {
        addEventListener() {},
        classList: { contains: () => false },
    };
    const document = {
        documentElement: { setAttribute() {} },
        addEventListener(type, fn) { if (type === 'DOMContentLoaded') domReady.push(fn); },
        getElementById: (id) => id === 'defaultThemePicker' ? picker : null,
        querySelector(selector) {
            if (selector === '#settingsTabs .nav-link.active') return activeTabButton;
            if (selector === '#settingsTabs [data-bs-target="#logo-content"]') return appearanceTab;
            return null;
        },
    };
    const window = {
        scrollY: 240,
        scrollTo() {},
        location: { hash: '', reload: () => reloads.push('reload') },
    };
    const sessionStorage = {
        getItem: (key) => store.has(key) ? store.get(key) : null,
        setItem: (key, value) => store.set(key, String(value)),
        removeItem: (key) => store.delete(key),
    };

    const context = {
        document, window, sessionStorage, console,
        localStorage: { getItem: () => null, setItem() {}, removeItem() {} },
        fetch: async () => ({ ok: true, json: async () => ({}) }),
        uiMessages: {
            showToast() {},
            t: (_key, fallback) => fallback,
            confirmDelete: async () => true,
        },
        IsleyTheme: { current: () => ({ themeChoice }) },
        bootstrap: {
            Tab: { getOrCreateInstance: () => ({ show: () => shownTabs.push('#logo-content') }) },
        },
    };
    vm.createContext(context);
    vm.runInContext(source, context);

    return {
        initialLoad: () => domReady.forEach((fn) => fn()),
        clickTheme: async (id) => {
            const event = { target: { closest: (sel) => sel === '[data-default-theme]' ? options[id] : null } };
            for (const handler of pickerHandlers) await handler(event);
            await new Promise((resolve) => setImmediate(resolve));
        },
        reload: () => domReady[0](), // settings-common.js restores the saved tab
        shownTabs,
        reloads,
    };
}

test('switching the default theme restores the Appearance tab after the reload', async () => {
    const page = harness(''); // admin has made no personal theme choice
    page.initialLoad();

    await page.clickTheme('walnut');
    assert.equal(page.reloads.length, 1, 'the picker reloads to render the new default');
    page.reload();

    assert.deepEqual(page.shownTabs, ['#logo-content']);
});
