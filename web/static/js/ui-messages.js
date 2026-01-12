// ui-messages.js
// Lightweight UI messaging helper used across templates. Provides:
// - uiMessages.t(key) -> returns translation if loaded, otherwise key
// - uiMessages.showToast(message, level)
// - uiMessages.showConfirm(message) -> Promise<boolean>

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

    function t(key) {
        if (!key) return '';
        return translations[key] || key;
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

        const timeout = (opts.timeout !== undefined) ? opts.timeout : 4000;
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

    function showConfirm(message) {
        return new Promise((resolve) => {
            // Create a modal-like confirm using bootstrap modal markup
            const modalId = 'uiConfirmModal';
            let modalEl = document.getElementById(modalId);
            if (!modalEl) {
                modalEl = document.createElement('div');
                modalEl.id = modalId;
                modalEl.className = 'modal fade';
                modalEl.tabIndex = -1;
                modalEl.innerHTML = `
                <div class="modal-dialog modal-dialog-centered">
                  <div class="modal-content">
                    <div class="modal-header">
                      <h5 class="modal-title">${t('confirm') || 'Confirm'}</h5>
                      <button type="button" class="btn-close" data-bs-dismiss="modal" aria-label="Close"></button>
                    </div>
                    <div class="modal-body">
                      <p id="uiConfirmMessage"></p>
                    </div>
                    <div class="modal-footer">
                      <button type="button" class="btn btn-secondary" id="uiConfirmCancel">${t('cancel') || 'Cancel'}</button>
                      <button type="button" class="btn btn-primary" id="uiConfirmOk">${t('ok') || 'OK'}</button>
                    </div>
                  </div>
                </div>
                `;
                document.body.appendChild(modalEl);
            }

            const msgEl = modalEl.querySelector('#uiConfirmMessage');
            const okBtn = modalEl.querySelector('#uiConfirmOk');
            const cancelBtn = modalEl.querySelector('#uiConfirmCancel');

            msgEl.textContent = message;

            const bsModal = new bootstrap.Modal(modalEl);
            const cleanup = () => {
                okBtn.removeEventListener('click', okHandler);
                cancelBtn.removeEventListener('click', cancelHandler);
                try { bsModal.hide(); } catch (e) {}
            };
            const okHandler = () => { cleanup(); resolve(true); };
            const cancelHandler = () => { cleanup(); resolve(false); };
            okBtn.addEventListener('click', okHandler);
            cancelBtn.addEventListener('click', cancelHandler);

            bsModal.show();
        });
    }

    // Start loading translations immediately
    loadTranslations();

    window.uiMessages = {
        t: t,
        loadTranslations: loadTranslations,
        showToast: showToast,
        showConfirm: showConfirm,
        _ready: () => readyPromise,
    };
})();