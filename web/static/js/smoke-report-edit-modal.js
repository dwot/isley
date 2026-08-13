document.addEventListener("DOMContentLoaded", () => {
    const form = document.getElementById("editSmokeReportForm");
    if (!form) return;

    const modalEl = document.getElementById("editSmokeReportModal");

    // The list module calls this when an edit button is clicked.
    window.smokeReports.onEdit = (id) => {
        const r = window.smokeReports.find(id);
        if (!r) return;

        document.getElementById("editSmokeReportId").value = r.id;
        document.getElementById("editSmokeReportDate").value = r.report_date.slice(0, 10);
        document.getElementById("editSmokeConsumptionMethod").value = r.consumption_method || "";
        document.getElementById("editSmokeDescription").value = r.description || "";
        window.starRating.set(document.getElementById("editSmokeRating"), r.rating);

        // Prefill every row, including the ones this report does not score:
        // starRating.set clears a group when passed 0, which is what keeps a
        // previously-opened report's stars from bleeding into the next one.
        const byCriterion = {};
        (r.scores || []).forEach((s) => { byCriterion[s.criterion_id] = s.value; });
        document.querySelectorAll("#editSmokeReportForm .edit-smoke-score-row").forEach((row) => {
            const cid = parseInt(row.dataset.criterionId, 10);
            window.starRating.set(row.querySelector(".star-rating"), byCriterion[cid] || 0);
        });

        bootstrap.Modal.getOrCreateInstance(modalEl).show();
    };

    // csrf.js injects X-CSRF-Token into these fetches automatically.
    function send(method, url, payload) {
        return fetch(url, {
            method: method,
            headers: { "Content-Type": "application/json" },
            body: payload ? JSON.stringify(payload) : null,
        }).then((response) => {
            if (!response.ok) {
                return response.json().catch(() => ({})).then((b) => {
                    throw new Error(b.error || "request failed");
                });
            }
            bootstrap.Modal.getInstance(modalEl).hide();
            return window.smokeReports.reload();
        });
    }

    form.addEventListener("submit", (e) => {
        e.preventDefault();

        // A criterion the user cleared has no checked radio and drops out of the
        // payload, which is what removes its score on the server.
        const scores = [];
        document.querySelectorAll("#editSmokeReportForm .edit-smoke-score-row").forEach((row) => {
            const value = window.starRating.value(row.querySelector(".star-rating"));
            if (value) {
                scores.push({
                    criterion_id: parseInt(row.dataset.criterionId, 10),
                    value: value,
                });
            }
        });

        const id = document.getElementById("editSmokeReportId").value;
        send("PUT", "/smoke-reports/" + id, {
            report_date: document.getElementById("editSmokeReportDate").value,
            consumption_method: document.getElementById("editSmokeConsumptionMethod").value,
            rating: window.starRating.value(document.getElementById("editSmokeRating")),
            description: document.getElementById("editSmokeDescription").value,
            scores: scores,
        }).catch((err) => uiMessages.showToast(err.message, "danger"));
    });

    // deleteSmokeReportBtn sits inside the modal's {{ if .loggedIn }} footer,
    // so it does not exist in the DOM for an anonymous guest-mode visitor —
    // guard the lookup before attaching a listener.
    const deleteBtn = document.getElementById("deleteSmokeReportBtn");
    if (deleteBtn) {
        deleteBtn.addEventListener("click", () => {
            // Every destructive action in this codebase confirms first — see
            // activity-edit-modal.js, measurement-edit-modal.js,
            // status-history-edit-modal.js. A smoke report is hand-written prose
            // that cannot be recovered, so this one especially.
            uiMessages.showConfirm(uiMessages.t('delete_smoke_report_confirm')).then((confirmed) => {
                if (!confirmed) return;
                const id = document.getElementById("editSmokeReportId").value;
                send("DELETE", "/smoke-reports/" + id, null)
                    .catch((err) => uiMessages.showToast(err.message, "danger"));
            });
        });
    }
});
