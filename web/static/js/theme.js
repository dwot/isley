(function () {
    const THEME_KEY = 'isley-theme';

    function setTheme(theme) {
        const htmlElement = document.body;
        const themeIcon = document.getElementById('themeToggleIcon');

        htmlElement.setAttribute('data-bs-theme', theme);
        if (themeIcon) {
            if (theme === 'dark') {
                themeIcon.classList.remove('fa-sun');
                themeIcon.classList.add('fa-moon');
            } else {
                themeIcon.classList.remove('fa-moon');
                themeIcon.classList.add('fa-sun');
            }
        }

        localStorage.setItem(THEME_KEY, theme);

        if (theme === 'dark') {
            htmlElement.classList.add('dark-mode');
            htmlElement.classList.remove('light-mode');
        } else {
            htmlElement.classList.add('light-mode');
            htmlElement.classList.remove('dark-mode');
        }

        // Update bg-dark/bg-light classes
        const allBgElements = document.querySelectorAll('.bg-dark, .bg-light');
        allBgElements.forEach(el => {
            if (theme === 'dark') {
                el.classList.add('bg-dark');
                el.classList.remove('bg-light');
            } else {
                el.classList.add('bg-light');
                el.classList.remove('bg-dark');
            }
        });

        // Update text-dark/text-light classes
        const allTextElements = document.querySelectorAll('.text-dark, .text-light');
        allTextElements.forEach(el => {
            if (theme === 'dark') {
                el.classList.add('text-light');
                el.classList.remove('text-dark');
            } else {
                el.classList.add('text-dark');
                el.classList.remove('text-light');
            }
        });

        // Update table-dark/table-light classes
        const allTableElements = document.querySelectorAll('.table-dark, .table-light');
        allTableElements.forEach(el => {
            if (theme === 'dark') {
                el.classList.add('table-dark');
                el.classList.remove('table-light');
            } else {
                el.classList.add('table-light');
                el.classList.remove('table-dark');
            }
        });
    }

    function toggleTheme() {
        const htmlElement = document.body;
        const currentTheme = htmlElement.getAttribute('data-bs-theme') || 'dark';
        const newTheme = currentTheme === 'dark' ? 'light' : 'dark';
        setTheme(newTheme);
    }

    function loadTheme() {
        const savedTheme = localStorage.getItem(THEME_KEY);
        const preferredTheme = savedTheme || (window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light');
        setTheme(preferredTheme);
    }

    // Listen for toggle action
    document.addEventListener('DOMContentLoaded', function () {
        const themeToggle = document.getElementById('themeToggle');
        // Wait for the DOM to be fully loaded before accessing elements
        loadTheme();
        if (themeToggle) {
            themeToggle.addEventListener('click', toggleTheme);
        }
    });
})();