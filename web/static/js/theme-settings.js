// theme-settings.js — Settings > Appearance: the instance's default
// theme, and the themes installed on top of the ones Isley ships.
//
// Picking a theme saves it at once. It is the theme every visitor gets
// until they choose their own in the header menu, so this page only
// changes in front of the admin if they have made no choice of their own.
//
// A theme is added from a package (a zip, or a bare theme.json) and can be
// downloaded as one; the server checks the package and writes the
// stylesheet. The page reloads after a change, since the theme list and
// its stylesheet are part of the page.
document.addEventListener('DOMContentLoaded', () => {
    const picker = document.getElementById('defaultThemePicker');
    if (!picker) return;

    const select = (id) => {
        picker.querySelectorAll('[data-default-theme]').forEach((option) => {
            const on = option.dataset.defaultTheme === id;
            option.classList.toggle('is-selected', on);
            option.setAttribute('aria-checked', on ? 'true' : 'false');
        });
    };

    picker.addEventListener('click', async (e) => {
        const option = e.target.closest('[data-default-theme]');
        if (!option || option.classList.contains('is-selected')) return;
        const id = option.dataset.defaultTheme;
        try {
            const res = await fetch('/settings/theme', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ theme: id }),
            });
            const data = await res.json().catch(() => ({}));
            if (!res.ok) throw new Error(data.error || data.message || res.statusText);
            select(id);
            uiMessages.showToast(uiMessages.t('theme_default_saved', 'Default theme saved'), 'success');
            // Show it now if this browser follows the instance default.
            document.documentElement.setAttribute('data-theme-default', id);
            if (!IsleyTheme.current().themeChoice) reloadKeepingPlace();
        } catch (err) {
            uiMessages.showToast(err.message || uiMessages.t('api_failed_to_save_settings', 'Failed to save settings'), 'danger');
        }
    });

    // A notice that has to outlive the reload that follows a change.
    const NOTICE_KEY = 'isley-theme-notice';
    const leaveNotice = (notices) => {
        try { sessionStorage.setItem(NOTICE_KEY, JSON.stringify(notices)); } catch (e) { /* shown not at all */ }
    };
    try {
        const notices = JSON.parse(sessionStorage.getItem(NOTICE_KEY) || '[]');
        sessionStorage.removeItem(NOTICE_KEY);
        notices.forEach((n) => uiMessages.showToast(n.message, n.level));
    } catch (e) { /* nothing to show */ }

    const input = document.getElementById('themePackageInput');
    const addButton = document.getElementById('addThemeButton');
    if (input && addButton) {
        addButton.addEventListener('click', () => input.click());
        input.addEventListener('change', async () => {
            const file = input.files[0];
            input.value = '';
            if (!file) return;
            const form = new FormData();
            form.append('theme', file);
            addButton.disabled = true;
            try {
                const res = await fetch('/settings/themes', { method: 'POST', body: form });
                const data = await res.json().catch(() => ({}));
                if (!res.ok) throw new Error(data.error || res.statusText);
                const notices = [{
                    message: (data.replaced
                        ? uiMessages.t('theme_updated', 'Theme updated')
                        : uiMessages.t('theme_installed', 'Theme installed')) + ': ' + data.theme.name,
                    level: 'success',
                }];
                (data.warnings || []).forEach((w) => notices.push({ message: w, level: 'warning' }));
                if (data.contrast && data.contrast.length) {
                    notices.push({
                        message: uiMessages.t('theme_contrast_notice', '{n} colour pair(s) in this theme are hard to read. They are listed under the theme.')
                            .replace('{n}', data.contrast.length),
                        level: 'warning',
                    });
                }
                leaveNotice(notices);
                reloadKeepingPlace();
            } catch (err) {
                uiMessages.showToast(err.message || uiMessages.t('theme_add_failed', 'The theme could not be added'), 'danger');
                addButton.disabled = false;
            }
        });
    }

    picker.addEventListener('click', async (e) => {
        const button = e.target.closest('[data-remove-theme]');
        if (!button) return;
        const id = button.dataset.removeTheme;
        const ok = await uiMessages.confirmDelete(
            uiMessages.t('theme_remove_confirm', 'Remove this theme? Anyone using it goes back to the default theme.'),
            button.dataset.themeName);
        if (!ok) return;
        try {
            const res = await fetch('/settings/themes/' + encodeURIComponent(id), { method: 'DELETE' });
            const data = await res.json().catch(() => ({}));
            if (!res.ok) throw new Error(data.error || res.statusText);
            leaveNotice([{ message: data.message || uiMessages.t('theme_removed', 'Theme removed'), level: 'success' }]);
            reloadKeepingPlace();
        } catch (err) {
            uiMessages.showToast(err.message || uiMessages.t('theme_remove_failed', 'The theme could not be removed'), 'danger');
        }
    });
});
