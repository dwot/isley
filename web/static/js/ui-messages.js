// ui-messages.js
// Lightweight UI messaging helper used across templates. Provides:
// - uiMessages.t(key, fallback) -> translation if loaded, else fallback, else key
// - uiMessages.has(key) -> true if a translation for key is loaded
// - uiMessages.showToast(message, level)
// - uiMessages.confirmDelete(message, title) -> showConfirm with danger "Delete" button
// - uiMessages.showConfirm(message, opts) -> Promise<boolean>
//     opts: { title, confirmText, cancelText, variant }
//     Resolves true only when the confirm button is clicked; any other
//     dismissal (cancel, X, Esc, backdrop) resolves false.

(() => {
    const translations = {};
    let readyResolve;
    const readyPromise = new Promise((res) => { readyResolve = res; });

    async function loadTranslations(retries = 1) {
        try {
            const resp = await fetch('/api/translations', { cache: 'no-store', credentials: 'same-origin', headers: { 'Accept': 'application/json' } });
            if (!resp.ok) return {};
            const data = await resp.json();
            Object.assign(translations, data || {});
        } catch (e) {
            if (retries > 0) {
                // brief backoff and retry once
                await new Promise(r => setTimeout(r, 250));
                return loadTranslations(retries - 1);
            }
            // ignore final failure
        } finally {
            readyResolve(true);
        }
    }

    function has(key) {
        return !!key && Object.prototype.hasOwnProperty.call(translations, key) && !!translations[key];
    }

    // Returns the translation for key. When the key is missing (or translations
    // have not loaded) returns `fallback` if given, otherwise the key itself.
    function t(key, fallback) {
        if (!key) return '';
        if (has(key)) return translations[key];
        return (fallback !== undefined) ? fallback : key;
    }

    function showToast(message, level = 'info', opts = {}) {
        // level -> 'info' | 'success' | 'warning' | 'danger'
        const container = document.getElementById('uiToastContainer') || createToastContainer();
        const toast = document.createElement('div');
        toast.className = `toast align-items-center text-bg-${level} border-0 show`; // bootstrap 5 classes
        toast.role = 'alert';
        toast.ariaLive = 'assertive';
        toast.ariaAtomic = 'true';

        const body = document.createElement('div');
        body.className = 'd-flex';
        body.style.gap = '0.5rem';

        const txt = document.createElement('div');
        txt.className = 'toast-body';
        txt.textContent = message;

        const closeBtn = document.createElement('button');
        closeBtn.type = 'button';
        closeBtn.className = 'btn-close btn-close-white me-2 m-auto';
        closeBtn.ariaLabel = 'Close';
        closeBtn.addEventListener('click', () => {
            toast.remove();
        });

        body.appendChild(txt);
        body.appendChild(closeBtn);
        toast.appendChild(body);
        container.appendChild(toast);

        // Error/danger toasts persist until dismissed; others auto-close after 5s
        const defaultTimeout = (level === 'danger' || level === 'warning') ? 0 : 5000;
        const timeout = (opts.timeout !== undefined) ? opts.timeout : defaultTimeout;
        if (timeout > 0) setTimeout(() => { toast.remove(); }, timeout);
    }

    function createToastContainer() {
        const footer = document.querySelector('footer') || document.body;
        const container = document.createElement('div');
        container.id = 'uiToastContainer';
        container.style.position = 'fixed';
        container.style.right = '1rem';
        container.style.bottom = '1rem';
        container.style.zIndex = '1055';
        footer.appendChild(container);
        return container;
    }

    let pendingConfirmResolve = null;

    function ensureConfirmModal() {
        const modalId = 'uiConfirmModal';
        let modalEl = document.getElementById(modalId);
        if (modalEl) return modalEl;
        modalEl = document.createElement('div');
        modalEl.id = modalId;
        modalEl.className = 'modal fade';
        modalEl.tabIndex = -1;
        modalEl.setAttribute('aria-labelledby', 'uiConfirmTitle');
        modalEl.setAttribute('aria-hidden', 'true');
        // Text is filled in on every showConfirm() call so it always reflects
        // the current translations and the caller's options.
        modalEl.innerHTML = `
        <div class="modal-dialog modal-dialog-centered">
          <div class="modal-content">
            <div class="modal-header">
              <h5 class="modal-title" id="uiConfirmTitle"></h5>
              <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
            </div>
            <div class="modal-body">
              <p id="uiConfirmMessage" class="mb-0"></p>
            </div>
            <div class="modal-footer">
              <button type="button" class="btn btn-secondary" id="uiConfirmCancel"></button>
              <button type="button" class="btn" id="uiConfirmOk"></button>
            </div>
          </div>
        </div>
        `;
        document.body.appendChild(modalEl);
        return modalEl;
    }

    // opts: { title, confirmText, cancelText, variant }  (variant: Bootstrap
    // button colour for the confirm button, default 'primary')
    function showConfirm(message, opts = {}) {
        return readyPromise.then(() => new Promise((resolve) => {
            // A second confirm while one is open cancels the first.
            if (pendingConfirmResolve) {
                pendingConfirmResolve(false);
                pendingConfirmResolve = null;
            }

            const modalEl = ensureConfirmModal();
            const titleEl = modalEl.querySelector('#uiConfirmTitle');
            const msgEl = modalEl.querySelector('#uiConfirmMessage');
            const okBtn = modalEl.querySelector('#uiConfirmOk');
            const cancelBtn = modalEl.querySelector('#uiConfirmCancel');

            titleEl.textContent = opts.title || t('confirm', 'Confirm');
            msgEl.textContent = message;
            okBtn.textContent = opts.confirmText || t('ok', 'OK');
            cancelBtn.textContent = opts.cancelText || t('cancel', 'Cancel');
            okBtn.className = 'btn btn-' + (opts.variant || 'primary');

            const bsModal = bootstrap.Modal.getOrCreateInstance(modalEl);
            let result = false;
            let settled = false;

            const settle = (value) => {
                if (settled) return;
                settled = true;
                okBtn.removeEventListener('click', okHandler);
                cancelBtn.removeEventListener('click', cancelHandler);
                modalEl.removeEventListener('hidden.bs.modal', hiddenHandler);
                if (pendingConfirmResolve === settle) pendingConfirmResolve = null;
                resolve(value);
            };
            const okHandler = () => { result = true; bsModal.hide(); };
            const cancelHandler = () => { result = false; bsModal.hide(); };
            // Fires for every way the dialog can close (buttons, X, Esc,
            // backdrop), so the promise always settles.
            const hiddenHandler = () => settle(result);

            okBtn.addEventListener('click', okHandler);
            cancelBtn.addEventListener('click', cancelHandler);
            modalEl.addEventListener('hidden.bs.modal', hiddenHandler);
            pendingConfirmResolve = settle;

            bsModal.show();
        }));
    }

    // Convenience wrapper for destructive confirms: danger-styled confirm
    // button labelled "Delete". `title` should be an already-translated string
    // such as t('delete_strain').
    function confirmDelete(message, title, opts = {}) {
        return showConfirm(message, Object.assign({
            title: title,
            confirmText: t('delete', 'Delete'),
            variant: 'danger',
        }, opts));
    }

    // Start loading translations immediately
    loadTranslations();

    window.uiMessages = {
        t: t,
        has: has,
        loadTranslations: loadTranslations,
        showToast: showToast,
        showConfirm: showConfirm,
        confirmDelete: confirmDelete,
        _ready: () => readyPromise,
    };
})();