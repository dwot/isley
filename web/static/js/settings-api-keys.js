// Load the configured API keys and render the list.
function loadAPIKeys() {
    const showError = () => {
        const list = document.getElementById('apiKeysList');
        if (list) list.textContent = uiMessages.t('failed_to_load_api_keys') || 'Failed to load API keys.';
    };
    fetch('/settings/api-keys')
        .then(r => r.json().then(data => ({ ok: r.ok, data })))
        .then(({ ok, data }) => {
            if (!ok) {
                showError();
                return;
            }
            renderAPIKeys(data.keys || []);
        })
        .catch(showError);
}

function renderAPIKeys(keys) {
    const list = document.getElementById('apiKeysList');
    if (!list) return;
    list.innerHTML = '';

    if (!keys.length) {
        const empty = document.createElement('div');
        empty.className = 'text-muted small';
        empty.textContent = uiMessages.t('settings_no_api_key') || 'No API key configured.';
        list.appendChild(empty);
        return;
    }

    const table = document.createElement('table');
    table.className = 'table table-sm align-middle mb-0';

    const thead = document.createElement('thead');
    const headRow = document.createElement('tr');
    [
        uiMessages.t('api_key_col_name') || 'Name',
        uiMessages.t('api_key_col_key') || 'Key',
        uiMessages.t('api_key_col_access') || 'Access',
        uiMessages.t('api_key_col_last_used') || 'Last used',
        ''
    ].forEach(label => {
        const th = document.createElement('th');
        th.scope = 'col';
        th.textContent = label;
        headRow.appendChild(th);
    });
    thead.appendChild(headRow);
    table.appendChild(thead);

    const tbody = document.createElement('tbody');
    keys.forEach(key => tbody.appendChild(buildAPIKeyRow(key)));
    table.appendChild(tbody);
    list.appendChild(table);
}

function buildAPIKeyRow(key) {
    const tr = document.createElement('tr');

    const nameTd = document.createElement('td');
    nameTd.textContent = key.name;
    tr.appendChild(nameTd);

    const keyTd = document.createElement('td');
    if (key.prefix) {
        const code = document.createElement('code');
        code.textContent = key.prefix + '…';
        keyTd.appendChild(code);
    } else {
        const span = document.createElement('span');
        span.className = 'text-muted';
        span.title = uiMessages.t('api_key_legacy_hint') || 'Carried over from a previous version';
        span.textContent = uiMessages.t('api_key_unknown_prefix') || '—';
        keyTd.appendChild(span);
    }
    tr.appendChild(keyTd);

    tr.appendChild(buildAPIKeyAccessCell(key));

    const usedTd = document.createElement('td');
    usedTd.className = 'small text-muted';
    usedTd.textContent = key.last_used ? formatTimestamp(key.last_used)
        : (uiMessages.t('api_key_never_used') || 'Never');
    tr.appendChild(usedTd);

    const actionTd = document.createElement('td');
    actionTd.className = 'text-end text-nowrap';

    const regenBtn = document.createElement('button');
    regenBtn.type = 'button';
    regenBtn.className = 'btn btn-outline-warning btn-sm me-1';
    regenBtn.dataset.action = 'regenerate';
    regenBtn.dataset.id = key.id;
    regenBtn.dataset.name = key.name;
    regenBtn.title = uiMessages.t('api_key_regenerate') || 'Regenerate';
    regenBtn.innerHTML = '<i class="fa fa-sync"></i>';
    actionTd.appendChild(regenBtn);

    const revokeBtn = document.createElement('button');
    revokeBtn.type = 'button';
    revokeBtn.className = 'btn btn-outline-danger btn-sm';
    revokeBtn.dataset.action = 'revoke';
    revokeBtn.dataset.id = key.id;
    revokeBtn.dataset.name = key.name;
    revokeBtn.title = uiMessages.t('api_key_revoke') || 'Revoke';
    revokeBtn.innerHTML = '<i class="fa fa-trash"></i>';
    actionTd.appendChild(revokeBtn);

    tr.appendChild(actionTd);
    return tr;
}

// A key's scopes are an access level (read < write < admin) plus,
// separately, sensor ingest. Admin includes ingest, so the ingest box is
// shown ticked and locked while admin is selected.
const API_KEY_LEVELS = ['read', 'write', 'admin'];

function scopesToControls(scopes) {
    const list = scopes || [];
    const level = list.find(s => API_KEY_LEVELS.includes(s)) || '';
    return { level: level, ingest: level === 'admin' || list.includes('ingest') };
}

function controlsToScopes(level, ingest) {
    const scopes = [];
    if (level) scopes.push(level);
    if (ingest && level !== 'admin') scopes.push('ingest');
    return scopes;
}

function syncIngestBox(levelSelect, ingestBox) {
    if (levelSelect.value === 'admin') {
        ingestBox.checked = true;
        ingestBox.disabled = true;
    } else if (ingestBox.disabled) {
        // Leaving admin: ingest was only implied, so clear it.
        ingestBox.checked = false;
        ingestBox.disabled = false;
    }
}

function buildLevelSelect(selected) {
    const select = document.createElement('select');
    select.className = 'form-select form-select-sm w-auto';
    [
        ['read', uiMessages.t('api_key_level_read') || 'Read only'],
        ['write', uiMessages.t('api_key_level_write') || 'Read and write'],
        ['admin', uiMessages.t('api_key_level_admin') || 'Admin (full access)'],
        ['', uiMessages.t('api_key_level_none') || 'No data access'],
    ].forEach(([value, label]) => {
        const opt = document.createElement('option');
        opt.value = value;
        opt.textContent = label;
        opt.selected = value === selected;
        select.appendChild(opt);
    });
    return select;
}

function buildAPIKeyAccessCell(key) {
    const td = document.createElement('td');
    const wrap = document.createElement('div');
    wrap.className = 'd-flex flex-wrap align-items-center gap-2';
    wrap.dataset.keyId = key.id;

    const current = scopesToControls(key.scopes);
    const select = buildLevelSelect(current.level);
    select.dataset.role = 'level';
    select.setAttribute('aria-label', (uiMessages.t('api_key_col_access') || 'Access') + ': ' + key.name);
    wrap.appendChild(select);

    const check = document.createElement('div');
    check.className = 'form-check mb-0';
    const box = document.createElement('input');
    box.type = 'checkbox';
    box.className = 'form-check-input';
    box.id = 'apiKeyIngest-' + key.id;
    box.dataset.role = 'ingest';
    box.checked = current.ingest;
    box.disabled = current.level === 'admin';
    const label = document.createElement('label');
    label.className = 'form-check-label small';
    label.htmlFor = box.id;
    label.textContent = uiMessages.t('api_key_ingest_label') || 'Sensor ingest';
    check.appendChild(box);
    check.appendChild(label);
    wrap.appendChild(check);

    td.appendChild(wrap);
    return td;
}

// Saves a key's access as soon as its level or ingest box changes. The
// list is reloaded on failure so the controls show what is stored.
function onAPIKeyAccessChange(e) {
    const wrap = e.target.closest('[data-key-id]');
    if (!wrap) return;
    const select = wrap.querySelector('[data-role="level"]');
    const box = wrap.querySelector('[data-role="ingest"]');
    if (e.target === select) syncIngestBox(select, box);

    const scopes = controlsToScopes(select.value, box.checked);
    if (!scopes.length) {
        uiMessages.showToast(uiMessages.t('api_invalid_api_key_scopes') || 'Choose what the key may do.', 'warning');
        loadAPIKeys();
        return;
    }
    fetch('/settings/api-keys/' + encodeURIComponent(wrap.dataset.keyId) + '/scopes', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ scopes: scopes })
    })
        .then(r => r.json().then(data => ({ ok: r.ok, data })))
        .then(({ ok, data }) => {
            if (!ok) {
                uiMessages.showToast(data.error || uiMessages.t('failed_to_update_api_key_scopes') || "Failed to change the key's access.", 'danger');
                loadAPIKeys();
                return;
            }
            uiMessages.showToast(data.message || uiMessages.t('api_api_key_scopes_updated') || 'API key access updated.', 'success');
        })
        .catch(() => {
            uiMessages.showToast(uiMessages.t('failed_to_update_api_key_scopes') || "Failed to change the key's access.", 'danger');
            loadAPIKeys();
        });
}

// Delegated handler for the regenerate/revoke buttons in the key list.
function onAPIKeyAction(e) {
    const btn = e.target.closest('button[data-action]');
    if (!btn) return;
    const id = btn.dataset.id;
    const name = btn.dataset.name || '';
    if (btn.dataset.action === 'regenerate') regenerateAPIKey(id, name);
    else if (btn.dataset.action === 'revoke') revokeAPIKey(id, name);
}

function createAPIKey() {
    const input = document.getElementById('newApiKeyName');
    const name = (input.value || '').trim();
    if (!name) {
        uiMessages.showToast(uiMessages.t('api_key_name_required') || 'Enter a name for the key.', 'warning');
        return;
    }
    const scopes = controlsToScopes(
        document.getElementById('newApiKeyLevel').value,
        document.getElementById('newApiKeyIngest').checked
    );
    if (!scopes.length) {
        uiMessages.showToast(uiMessages.t('api_invalid_api_key_scopes') || 'Choose what the key may do.', 'warning');
        return;
    }
    fetch('/settings/api-keys', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name, scopes: scopes })
    })
        .then(r => r.json().then(data => ({ ok: r.ok, data })))
        .then(({ ok, data }) => {
            if (!ok || !data.api_key) {
                uiMessages.showToast(data.error || uiMessages.t('failed_to_save_api_key') || 'Failed to create key.', 'danger');
                return;
            }
            input.value = '';
            showRevealedKey(data.api_key, data.key && data.key.id);
            loadAPIKeys();
        })
        .catch(() => uiMessages.showToast(uiMessages.t('failed_to_save_api_key') || 'Failed to create key.', 'danger'));
}

function regenerateAPIKey(id, name) {
    const msg = uiMessages.t('api_key_regenerate_confirm', 'Regenerate this key? The current value will stop working immediately.');
    uiMessages.showConfirm(name ? (msg + ' (' + name + ')') : msg, {
        title: uiMessages.t('settings_regenerate_key', 'Regenerate API Key'),
        confirmText: uiMessages.t('api_key_regenerate', 'Regenerate'),
        variant: 'danger',
    }).then(confirmed => {
        if (!confirmed) return;
        fetch('/settings/api-keys/' + encodeURIComponent(id) + '/regenerate', { method: 'POST' })
            .then(r => r.json().then(data => ({ ok: r.ok, data })))
            .then(({ ok, data }) => {
                if (!ok || !data.api_key) {
                    uiMessages.showToast(data.error || uiMessages.t('failed_to_save_api_key') || 'Failed to regenerate key.', 'danger');
                    return;
                }
                showRevealedKey(data.api_key, data.key && data.key.id);
                loadAPIKeys();
            })
            .catch(() => uiMessages.showToast(uiMessages.t('failed_to_save_api_key') || 'Failed to regenerate key.', 'danger'));
    });
}

function revokeAPIKey(id, name) {
    const msg = uiMessages.t('api_key_revoke_confirm', 'Revoke this key? Any system using it will be locked out immediately.');
    uiMessages.showConfirm(name ? (msg + ' (' + name + ')') : msg, {
        title: uiMessages.t('api_key_revoke', 'Revoke'),
        confirmText: uiMessages.t('api_key_revoke', 'Revoke'),
        variant: 'danger',
    }).then(confirmed => {
        if (!confirmed) return;
        fetch('/settings/api-keys/' + encodeURIComponent(id), { method: 'DELETE' })
            .then(r => r.json().then(data => ({ ok: r.ok, data })))
            .then(({ ok, data }) => {
                if (!ok) {
                    uiMessages.showToast(data.error || uiMessages.t('failed_to_delete_api_key') || 'Failed to revoke key.', 'danger');
                    return;
                }
                uiMessages.showToast(data.message || uiMessages.t('api_key_revoked') || 'API key revoked.', 'success');
                // If the one-time reveal is showing this key, clear it.
                if (document.getElementById('apiKeyReveal').dataset.keyId === String(id)) {
                    hideRevealedKey();
                }
                loadAPIKeys();
            })
            .catch(() => uiMessages.showToast(uiMessages.t('failed_to_delete_api_key') || 'Failed to revoke key.', 'danger'));
    });
}

function showRevealedKey(plaintext, keyId) {
    const reveal = document.getElementById('apiKeyReveal');
    reveal.dataset.keyId = keyId == null ? '' : String(keyId);
    document.getElementById('apiKeyDisplay').value = plaintext;
    reveal.classList.remove('d-none');
    document.getElementById('apiKeyCopyStatus').textContent = '';
}

// Hide and clear the one-time reveal so a revealed key can't linger (and be
// copied) after the key it belongs to has been revoked.
function hideRevealedKey() {
    const reveal = document.getElementById('apiKeyReveal');
    reveal.classList.add('d-none');
    reveal.dataset.keyId = '';
    document.getElementById('apiKeyDisplay').value = '';
    document.getElementById('apiKeyCopyStatus').textContent = '';
}

// Render a stored timestamp (RFC3339 or "YYYY-MM-DD HH:MM:SS" UTC) in the
// instance zone, falling back to the raw value if it can't be parsed.
function formatTimestamp(value) {
    return IsleyTime.formatDateTime(value.includes('T') ? value : value.replace(' ', 'T') + 'Z') || value;
}

function copyAPIKey() {
    const apiKeyInput = document.getElementById('apiKeyDisplay');
    const statusEl = document.getElementById('apiKeyCopyStatus');
    const done = () => {
        statusEl.textContent = uiMessages.t('copied_to_clipboard') || 'Copied to clipboard.';
        setTimeout(() => { statusEl.textContent = ''; }, 3000);
    };
    const fail = (err) => {
        console.error('Copy failed', err);
        statusEl.textContent = uiMessages.t('copy_failed') || 'Copy failed — select the key and copy it manually.';
    };
    // navigator.clipboard is unavailable over plain HTTP (insecure context),
    // which is a common LAN deployment — fall back to selecting the field.
    try {
        if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard.writeText(apiKeyInput.value).then(done).catch(fail);
        } else {
            apiKeyInput.focus();
            apiKeyInput.select();
            document.execCommand('copy');
            done();
        }
    } catch (err) {
        fail(err);
    }
}
