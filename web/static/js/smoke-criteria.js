// Settings › Smoke Report Criteria. Mirrors the metric CRUD JS: submit,
// reload on success, toast on failure. Delete additionally confirms first
// (uiMessages.showConfirm) and, since the endpoint may archive instead of
// hard-delete a criterion already scored on a report, shows which outcome
// happened before reloading.
document.addEventListener("DOMContentLoaded", () => {
    const addForm = document.getElementById("addSmokeCriterionForm");
    const editForm = document.getElementById("editSmokeCriterionForm");
    if (!addForm || !editForm) return;

    const editModalEl = document.getElementById("editSmokeCriterionModal");

    // No CSRF header here on purpose: csrf.js monkey-patches window.fetch and
    // injects X-CSRF-Token on every POST/PUT/DELETE/PATCH. Setting it by hand
    // would duplicate that logic — and there is no csrf.* object to read it
    // from, so an explicit attempt throws a ReferenceError.
    function post(url, method, payload) {
        return fetch(url, {
            method: method,
            headers: { "Content-Type": "application/json" },
            body: payload ? JSON.stringify(payload) : null,
        }).then((response) => {
            if (!response.ok) {
                return response.json()
                    .catch(() => ({}))
                    .then((body) => {
                        throw new Error(body.error || "request failed");
                    });
            }
            return response.json().catch(() => ({}));
        });
    }

    addForm.addEventListener("submit", (e) => {
        e.preventDefault();
        post("/smoke-criteria", "POST", {
            name: document.getElementById("smokeCriterionName").value,
            sort_order: parseInt(document.getElementById("smokeCriterionOrder").value, 10) || 0,
        })
            .then(() => window.location.reload())
            .catch((err) => uiMessages.showToast(err.message, "danger"));
    });

    // Clicking a row loads it into the edit modal.
    document.querySelectorAll(".smoke-criterion-row").forEach((row) => {
        row.addEventListener("click", () => {
            const c = JSON.parse(row.dataset.criterion);
            document.getElementById("editSmokeCriterionId").value = c.id;
            document.getElementById("editSmokeCriterionName").value = c.name;
            document.getElementById("editSmokeCriterionOrder").value = c.sort_order;
            document.getElementById("editSmokeCriterionActive").checked = c.active;
            bootstrap.Modal.getOrCreateInstance(editModalEl).show();
        });
    });

    editForm.addEventListener("submit", (e) => {
        e.preventDefault();
        const id = document.getElementById("editSmokeCriterionId").value;
        post("/smoke-criteria/" + id, "PUT", {
            name: document.getElementById("editSmokeCriterionName").value,
            sort_order: parseInt(document.getElementById("editSmokeCriterionOrder").value, 10) || 0,
            active: document.getElementById("editSmokeCriterionActive").checked,
        })
            .then(() => window.location.reload())
            .catch((err) => uiMessages.showToast(err.message, "danger"));
    });

    document.getElementById("deleteSmokeCriterionBtn").addEventListener("click", () => {
        const id = document.getElementById("editSmokeCriterionId").value;
        uiMessages.showConfirm(uiMessages.t("delete_smoke_criterion_confirm")).then((confirmed) => {
            if (!confirmed) return;
            post("/smoke-criteria/" + id, "DELETE", null)
                .then((body) => {
                    // The endpoint reports which of the two outcomes happened: a
                    // criterion already scored on a report is archived (scores
                    // kept) instead of hard-deleted. Surface that distinction
                    // before the reload wipes the toast.
                    const key = body && body.archived
                        ? "smoke_criterion_archived_notice"
                        : "smoke_criterion_deleted_notice";
                    uiMessages.showToast(uiMessages.t(key), "success");
                    setTimeout(() => window.location.reload(), 1200);
                })
                .catch((err) => uiMessages.showToast(err.message, "danger"));
        });
    });
});
