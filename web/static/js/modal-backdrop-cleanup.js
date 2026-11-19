// modal-backdrop-cleanup.js — Bootstrap's backdrop can linger when the page
// reloads while a modal's fade-out is still running. Remove any left behind
// each time a modal closes.
document.addEventListener("DOMContentLoaded", () => {
    document.addEventListener("hidden.bs.modal", () => {
        document.querySelectorAll(".modal-backdrop").forEach((b) => b.remove());
        document.body.classList.remove("modal-open");
        document.body.style.overflow = "";
    });
});
