document.addEventListener("DOMContentLoaded", () => {
    const langMenu = document.getElementById("languageMenu");
    const currentLangLabel = document.getElementById("currentLanguage");

    // Get saved language or use current one from server
    const savedLang = localStorage.getItem("language") || currentLangLabel.innerText.toLowerCase();

    // Update the displayed language
    currentLangLabel.innerText = savedLang.toUpperCase();

    // Handle language selection
    langMenu.addEventListener("click", (e) => {
        const selectedLang = e.target.closest(".lang-select")?.getAttribute("data-lang");
        if (selectedLang) {
            localStorage.setItem("language", selectedLang); // Save preference
            window.location.href = `/?lang=${selectedLang}`; // Reload page with new language
        }
    });
});
