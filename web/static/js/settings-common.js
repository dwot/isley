// Add/edit/delete handlers on this page refresh by reloading. The page
// deliberately opens on the Settings tab on a normal visit, so a plain
// reload dropped you back there at the top (e.g. after deleting a breeder
// on the Tables tab). Remember the active tab and scroll position for the
// one reload that follows, then restore them and forget them.
const SETTINGS_RELOAD_KEY = 'settingsReloadState';

function reloadKeepingPlace() {
    try {
        const activeTab = document.querySelector('#settingsTabs .nav-link.active');
        sessionStorage.setItem(SETTINGS_RELOAD_KEY, JSON.stringify({
            tab: activeTab ? activeTab.getAttribute('data-bs-target') : null,
            scrollY: window.scrollY,
        }));
    } catch (e) {
        // Storage unavailable: fall back to a plain reload.
    }
    window.location.reload();
}

document.addEventListener("DOMContentLoaded", () => {
    let state = null;
    try {
        state = JSON.parse(sessionStorage.getItem(SETTINGS_RELOAD_KEY) || 'null');
        sessionStorage.removeItem(SETTINGS_RELOAD_KEY);
    } catch (e) {
        return;
    }
    if (!state) {
        // A link from elsewhere can name a tab: /settings#logo-content.
        const linked = /^#[\w-]+$/.test(window.location.hash)
            ? document.querySelector(`#settingsTabs [data-bs-target="${window.location.hash}"]`) : null;
        if (linked) bootstrap.Tab.getOrCreateInstance(linked).show();
        return;
    }

    const restoreScroll = () => window.scrollTo(0, state.scrollY || 0);
    const tabBtn = state.tab ? document.querySelector(`#settingsTabs [data-bs-target="${state.tab}"]`) : null;
    if (tabBtn && !tabBtn.classList.contains('active')) {
        // Scroll once the tab pane has finished fading in and has height.
        tabBtn.addEventListener('shown.bs.tab', restoreScroll, { once: true });
        bootstrap.Tab.getOrCreateInstance(tabBtn).show();
    } else {
        restoreScroll();
    }
});

document.addEventListener("DOMContentLoaded", function () {
    // Clear any previously stored tab so the page always starts on the Settings tab
    localStorage.removeItem('activeSettingsTab');
});
