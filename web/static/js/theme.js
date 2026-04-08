(function () {
    const THEME_KEY = 'isley-theme';

    function setTheme(theme) {
        document.body.setAttribute('data-bs-theme', theme);
        localStorage.setItem(THEME_KEY, theme);

        var themeIcon = document.getElementById('themeToggleIcon');
        if (themeIcon) {
            themeIcon.classList.toggle('fa-moon', theme === 'dark');
            themeIcon.classList.toggle('fa-sun', theme !== 'dark');
        }
    }

    function toggleTheme() {
        var current = document.body.getAttribute('data-bs-theme') || 'dark';
        setTheme(current === 'dark' ? 'light' : 'dark');
    }

    function loadTheme() {
        var saved = localStorage.getItem(THEME_KEY);
        var preferred = saved || (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
        setTheme(preferred);
    }

    document.addEventListener('DOMContentLoaded', function () {
        loadTheme();
        var toggle = document.getElementById('themeToggle');
        if (toggle) {
            toggle.addEventListener('click', toggleTheme);
        }
    });
})();
