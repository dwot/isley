// theme-editor.js — Settings > Appearance > edit a theme.
//
// The editor works on a theme package's manifest (docs/themes.md) held in
// the page. Every change is laid over the preview panel at once, and sent
// to the server to be checked: the server says whether the theme would be
// accepted and which colour pairs are hard to read, with the same code
// that checks an uploaded package. Saving sends the manifest, and any new
// logo, to be installed.
//
// In the page a theme is two lists of values, one per mode. Colours,
// shadows and the page background belong to a mode. Corner radius, fonts,
// text size and shade are the same in both, so setting one sets it in
// each. The manifest's "tokens" (what both modes share) is worked out
// when it is written.
document.addEventListener('DOMContentLoaded', () => {
    const page = document.getElementById('themeEditor');
    if (!page) return;

    const isNew = page.dataset.new === 'true';
    const from = page.dataset.from;
    const source = JSON.parse(page.dataset.package);
    const preview = document.getElementById('tePreview');
    const status = document.getElementById('teStatus');
    const saveButton = document.getElementById('teSave');
    const nameInput = document.getElementById('teName');
    const idInput = document.getElementById('teId');
    const descInput = document.getElementById('teDescription');
    const rows = Array.from(page.querySelectorAll('.te-row'));
    const SHARED = { radius: true, font: true, 'font-size': true, percent: true };
    const kindOf = {};
    rows.forEach((row) => { kindOf[row.dataset.token] = row.dataset.kind; });

    // ---- state -----------------------------------------------------------
    const has = { light: source.modes.indexOf('light') >= 0, dark: source.modes.indexOf('dark') >= 0 };
    const values = {
        light: Object.assign({}, has.light ? source.tokens : {}, source.light),
        dark: Object.assign({}, has.dark ? source.tokens : {}, source.dark),
    };
    // A shared kind has one value: light's if the package gave two.
    Object.keys(kindOf).forEach((token) => {
        if (!SHARED[kindOf[token]]) return;
        const v = values.light[token] !== undefined ? values.light[token] : values.dark[token];
        if (v !== undefined) values.light[token] = values.dark[token] = v;
    });
    let mode = has.dark ? 'dark' : 'light';
    const logos = { light: null, dark: null }; // object URLs of newly chosen files
    let idTouched = !isNew;
    let dirty = false;

    function manifest() {
        const m = {
            isley_theme: 1,
            id: idInput.value.trim(),
            name: nameInput.value.trim(),
            modes: ['light', 'dark'].filter((x) => has[x]),
        };
        if (source.author) m.author = source.author;
        if (descInput.value.trim()) m.description = descInput.value.trim();
        if (has.light && has.dark) {
            m.tokens = {}; m.light = {}; m.dark = {};
            Object.keys(values.light).forEach((t) => {
                if (values.dark[t] === values.light[t]) m.tokens[t] = values.light[t];
                else m.light[t] = values.light[t];
            });
            Object.keys(values.dark).forEach((t) => {
                if (values.dark[t] !== values.light[t]) m.dark[t] = values.dark[t];
            });
        } else {
            const only = has.light ? 'light' : 'dark';
            m[only] = Object.assign({}, values[only]);
        }
        return m;
    }

    // ---- reading colours and defaults ------------------------------------
    // A hidden Classic scope per mode answers "what would this token be if
    // the theme left it alone", and turns any colour into one the picker
    // takes.
    const probes = {};
    ['light', 'dark'].forEach((m) => {
        const el = document.createElement('div');
        el.setAttribute('data-theme', 'classic');
        el.setAttribute('data-bs-theme', m);
        el.hidden = true;
        document.body.appendChild(el);
        probes[m] = el;
    });
    function defaultValue(token) {
        return getComputedStyle(probes[mode]).getPropertyValue(token).trim().replace(/\s+/g, ' ');
    }
    function toHex(value) {
        const probe = probes[mode];
        probe.style.color = '';
        probe.style.color = value;
        if (!probe.style.color) return null; // not a colour
        // "rgb(1, 2, 3)" or, for some colour spaces, "color(srgb 0.1 0.2 0.3)".
        const computed = getComputedStyle(probe).color;
        probe.style.color = '';
        const nums = (computed.replace(/^color\(\s*[a-z0-9-]+/, '').match(/-?[\d.]+/g) || []).map(Number);
        if (nums.length < 3) return null;
        const scale = computed.indexOf('color(') === 0 ? 255 : 1;
        return '#' + nums.slice(0, 3).map((n) => Math.round(n * scale).toString(16).padStart(2, '0')).join('');
    }

    // ---- drawing ---------------------------------------------------------
    function drawRow(row) {
        const token = row.dataset.token;
        const set = values[mode][token] !== undefined;
        const value = set ? values[mode][token] : defaultValue(token);
        row.classList.toggle('is-set', set);
        const text = row.querySelector('.te-text');
        if (document.activeElement !== text) text.value = value;
        const pick = row.querySelector('.te-pick');
        if (pick) {
            const hex = toHex(value);
            if (hex) pick.value = hex;
        }
        const range = row.querySelector('.te-range');
        if (range && document.activeElement !== range) {
            const n = parseFloat(value);
            if (!isNaN(n) && value.endsWith(range.dataset.unit)) range.value = n;
        }
        row.querySelector('.te-reset').hidden = !set;
    }

    function drawPreview() {
        preview.setAttribute('data-bs-theme', mode);
        // Start from Classic: drop everything laid on before.
        Array.from(preview.style).forEach((prop) => { if (prop.indexOf('--') === 0) preview.style.removeProperty(prop); });
        Object.keys(values[mode]).forEach((t) => preview.style.setProperty(t, values[mode][t]));
        // The logo: a newly chosen file, or the one the theme started with.
        const chosen = (mode === 'dark' && logos.dark) || logos.light;
        if (chosen) {
            preview.style.setProperty('--logo', 'url("' + chosen + '")');
        } else {
            const src = document.createElement('div');
            src.setAttribute('data-theme', from);
            src.setAttribute('data-bs-theme', mode);
            src.hidden = true;
            document.body.appendChild(src);
            preview.style.setProperty('--logo', getComputedStyle(src).getPropertyValue('--logo'));
            src.remove();
        }
    }

    function drawChrome() {
        page.querySelectorAll('[data-edit-mode]').forEach((b) => {
            const m = b.dataset.editMode;
            b.classList.toggle('active', m === mode);
            b.setAttribute('aria-pressed', m === mode ? 'true' : 'false');
            b.disabled = !has[m];
        });
        page.querySelectorAll('[data-has-mode]').forEach((box) => {
            box.checked = has[box.dataset.hasMode];
            // A theme has at least one mode.
            box.disabled = has[box.dataset.hasMode] && !(has.light && has.dark);
        });
        page.querySelectorAll('.te-group').forEach((group) => {
            const n = group.querySelectorAll('.te-row.is-set').length;
            const badge = group.querySelector('.te-group-count');
            badge.hidden = n === 0;
            badge.textContent = n;
        });
    }

    function draw() {
        rows.forEach(drawRow);
        drawPreview();
        drawChrome();
    }

    // ---- changes ---------------------------------------------------------
    function setToken(token, value) {
        const targets = SHARED[kindOf[token]] ? ['light', 'dark'] : [mode];
        targets.forEach((m) => {
            if (value === null || value === '') delete values[m][token];
            else values[m][token] = value;
        });
        changed();
    }

    let checkTimer = null;
    function changed() {
        dirty = true;
        draw();
        clearTimeout(checkTimer);
        checkTimer = setTimeout(check, 350);
    }

    rows.forEach((row) => {
        const token = row.dataset.token;
        const text = row.querySelector('.te-text');
        const pick = row.querySelector('.te-pick');
        const range = row.querySelector('.te-range');
        text.addEventListener('input', () => setToken(token, text.value.trim().replace(/\s+/g, ' ')));
        text.addEventListener('blur', () => drawRow(row));
        if (pick) pick.addEventListener('input', () => { text.value = pick.value; setToken(token, pick.value); });
        if (range) range.addEventListener('input', () => { text.value = range.value + range.dataset.unit; setToken(token, text.value); });
        row.querySelector('.te-reset').addEventListener('click', () => { setToken(token, null); });
    });

    page.querySelectorAll('[data-edit-mode]').forEach((b) => b.addEventListener('click', () => {
        if (!has[b.dataset.editMode]) return;
        mode = b.dataset.editMode;
        draw();
    }));
    page.querySelectorAll('[data-has-mode]').forEach((box) => box.addEventListener('change', () => {
        const m = box.dataset.hasMode, other = m === 'light' ? 'dark' : 'light';
        has[m] = box.checked;
        if (has[m]) {
            // A mode switched on starts with what the two share.
            values[m] = {};
            Object.keys(values[other]).forEach((t) => { if (SHARED[kindOf[t]]) values[m][t] = values[other][t]; });
            mode = m;
        } else {
            mode = other;
        }
        changed();
    }));

    function slug(name) {
        return name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '').slice(0, 40);
    }
    nameInput.addEventListener('input', () => {
        if (!idTouched) idInput.value = slug(nameInput.value);
        changed();
    });
    idInput.addEventListener('input', () => { idTouched = true; changed(); });
    descInput.addEventListener('input', changed);

    [['teLogo', 'light'], ['teLogoDark', 'dark']].forEach(([inputId, key]) => {
        document.getElementById(inputId).addEventListener('change', (e) => {
            if (logos[key]) URL.revokeObjectURL(logos[key]);
            const file = e.target.files[0];
            logos[key] = file ? URL.createObjectURL(file) : null;
            dirty = true;
            drawPreview();
        });
    });

    // ---- the server's opinion -------------------------------------------
    const t = (key, fallback) => uiMessages.t(key, fallback);
    let checkSeq = 0;
    let acceptable = true;

    function showStatus(kind, title, lines) {
        status.className = 'te-status te-status--' + kind;
        status.textContent = '';
        const head = document.createElement('div');
        head.className = 'te-status-title';
        const icon = document.createElement('i');
        icon.className = 'fa me-1 ' + ({ ok: 'fa-circle-check', warn: 'fa-triangle-exclamation', bad: 'fa-circle-xmark' })[kind];
        head.append(icon, title);
        status.append(head);
        if (lines && lines.length) {
            const list = document.createElement('ul');
            lines.forEach((line) => { const li = document.createElement('li'); li.textContent = line; list.append(li); });
            status.append(list);
        }
    }

    async function check() {
        const seq = ++checkSeq;
        try {
            const res = await fetch('/settings/themes/check', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(manifest()),
            });
            const data = await res.json();
            if (seq !== checkSeq) return; // an older answer
            if (!res.ok) throw new Error(data.error || res.statusText);
            acceptable = !!data.ok;
            saveButton.disabled = !acceptable;
            if (!data.ok) {
                showStatus('bad', t('theme_editor_not_ok', 'This cannot be saved yet'), [data.error]);
                return;
            }
            const modeName = { light: t('theme_mode_light', 'Light'), dark: t('theme_mode_dark', 'Dark') };
            const lines = data.contrast.map((c) =>
                (Math.floor(c.ratio * 10) / 10).toFixed(1) + ':1  ' + c.text + ' ' + t('theme_contrast_on', 'on') + ' ' + c.background +
                (has.light && has.dark ? ' (' + modeName[c.mode] + ')' : ''));
            if (lines.length) {
                showStatus('warn', t('theme_editor_contrast', '{n} colour pair(s) hard to read').replace('{n}', lines.length), lines);
            } else {
                showStatus('ok', t('theme_editor_ok', 'Ready to save. Every colour pair this theme sets is readable.'));
            }
        } catch (err) {
            if (seq === checkSeq) showStatus('bad', err.message || 'Check failed');
        }
    }

    // ---- save ------------------------------------------------------------
    saveButton.addEventListener('click', async () => {
        const form = new FormData();
        form.append('manifest', JSON.stringify(manifest()));
        form.append('from', from);
        form.append('new', isNew ? 'true' : 'false');
        const logo = document.getElementById('teLogo').files[0];
        const logoDark = document.getElementById('teLogoDark').files[0];
        if (logo) form.append('logo', logo);
        if (logoDark) form.append('logo_dark', logoDark);
        saveButton.disabled = true;
        try {
            const res = await fetch('/settings/themes/save', { method: 'POST', body: form });
            const data = await res.json().catch(() => ({}));
            if (!res.ok) throw new Error(data.error || res.statusText);
            const notices = [{ message: (data.message || t('theme_updated', 'Theme saved')) + ': ' + data.theme.name, level: 'success' }];
            if (data.contrast && data.contrast.length) {
                notices.push({
                    message: t('theme_contrast_notice', '{n} colour pair(s) in this theme are hard to read. They are listed under the theme.')
                        .replace('{n}', data.contrast.length),
                    level: 'warning',
                });
            }
            // theme-settings.js shows these after the move.
            try { sessionStorage.setItem('isley-theme-notice', JSON.stringify(notices)); } catch (e) { /* not shown */ }
            dirty = false;
            window.location.href = '/settings#logo-content';
        } catch (err) {
            showStatus('bad', t('theme_editor_save_failed', 'The theme was not saved'), [err.message]);
            saveButton.disabled = !acceptable;
        }
    });

    window.addEventListener('beforeunload', (e) => { if (dirty) e.preventDefault(); });

    // ---- start -----------------------------------------------------------
    nameInput.value = source.name || '';
    idInput.value = source.id || '';
    descInput.value = source.description || '';
    draw();
    // The first answer waits for the translations it is worded in.
    (uiMessages.ready || Promise.resolve()).then(check);
});
