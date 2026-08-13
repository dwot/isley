// Renders the plant page's smoke-report list. Exposes window.smokeReports so
// the add/edit modals can refresh the list after a write without a full page
// reload, and so the edit modal can look a report up by id.
(function () {
    const VISIBLE_BY_DEFAULT = 3;

    const state = { reports: [], expanded: false };

    function stars(value) {
        const n = Math.max(0, Math.min(5, Math.round(Number(value) || 0)));
        return "★".repeat(n) + "☆".repeat(5 - n);
    }

    function escapeHTML(s) {
        const div = document.createElement("div");
        div.textContent = s;
        return div.innerHTML;
    }

    function cureLabel(days) {
        if (days === undefined || days === null) return "";
        const weeks = Math.floor(days / 7);
        const unit = weeks >= 1
            ? (uiMessages.t("smoke_cure_weeks") || "wk cure")
            : (uiMessages.t("smoke_cure_days") || "d cure");
        return (weeks >= 1 ? weeks : days) + " " + unit;
    }

    function renderReport(r, loggedIn) {
        // One line per criterion. Concatenating them inline wrapped mid-criterion
        // in the narrow sidebar — a name on one line and its stars on the next —
        // so each pair gets its own flex row instead.
        const scores = (r.scores || [])
            .map((s) =>
                '<div class="smoke-score-line">' +
                '  <span class="smoke-score-name">' + escapeHTML(s.name) + '</span>' +
                '  <span class="smoke-score-stars">' + stars(s.value) + '</span>' +
                '</div>')
            .join("");

        const editBtn = loggedIn
            ? '<button class="btn btn-sm btn-link p-0 smoke-report-edit" data-report-id="' + r.id + '">' +
              '<i class="fa-solid fa-pen-to-square"></i></button>'
            : "";

        return '' +
            '<div class="smoke-report-entry border-bottom pb-2 mb-2">' +
            '  <div class="d-flex justify-content-between align-items-center">' +
            '    <span class="text-muted plant-mini-date">' + escapeHTML(r.report_date.slice(0, 10)) + '</span>' +
            '    <span class="smoke-report-rating">' + stars(r.rating) + " " + r.rating.toFixed(1) + '</span>' +
            '  </div>' +
            '  <div class="text-muted small">' + escapeHTML(cureLabel(r.cure_days)) +
            (r.consumption_method ? " &middot; " + escapeHTML(r.consumption_method) : "") + '</div>' +
            (scores ? '  <div class="small mt-1">' + scores + '</div>' : "") +
            (r.description ? '  <div class="small mt-1 fst-italic">' + escapeHTML(r.description) + '</div>' : "") +
            '  <div class="text-end">' + editBtn + '</div>' +
            '</div>';
    }

    function render() {
        const card = document.getElementById("smokeReportCard");
        if (!card) return;

        const list = document.getElementById("smokeReportList");
        const empty = document.getElementById("smokeReportEmpty");
        const showMore = document.getElementById("smokeReportShowMore");
        const count = document.getElementById("smokeReportCount");
        const loggedIn = card.dataset.loggedIn === "true";

        count.textContent = state.reports.length;

        if (state.reports.length === 0) {
            list.innerHTML = "";
            empty.style.display = "";
            showMore.style.display = "none";
            return;
        }
        empty.style.display = "none";

        const visible = state.expanded
            ? state.reports
            : state.reports.slice(0, VISIBLE_BY_DEFAULT);

        list.innerHTML = visible.map((r) => renderReport(r, loggedIn)).join("");

        showMore.style.display =
            state.reports.length > VISIBLE_BY_DEFAULT && !state.expanded ? "" : "none";

        list.querySelectorAll(".smoke-report-edit").forEach((btn) => {
            btn.addEventListener("click", () => {
                const id = parseInt(btn.dataset.reportId, 10);
                if (window.smokeReports.onEdit) window.smokeReports.onEdit(id);
            });
        });
    }

    function reload() {
        const card = document.getElementById("smokeReportCard");
        if (!card) return Promise.resolve();
        const plantId = card.dataset.plantId;
        return fetch("/plant/" + plantId + "/smoke-reports")
            .then((r) => r.json())
            .then((data) => {
                state.reports = Array.isArray(data) ? data : [];
                render();
            })
            .catch((err) => console.error("smoke reports:", err));
    }

    window.smokeReports = {
        reload: reload,
        find: (id) => state.reports.find((r) => r.id === id),
        onEdit: null,
    };

    document.addEventListener("DOMContentLoaded", () => {
        const showMore = document.getElementById("smokeReportShowMore");
        if (showMore) {
            showMore.addEventListener("click", () => {
                state.expanded = true;
                render();
            });
        }
        reload();
    });
})();
