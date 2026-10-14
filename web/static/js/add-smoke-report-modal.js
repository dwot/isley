document.addEventListener("DOMContentLoaded", () => {
    const form = document.getElementById("addSmokeReportForm");
    if (!form) return;

    const modalEl = document.getElementById("addSmokeReportModal");

    modalEl.addEventListener("show.bs.modal", () => {
        document.getElementById("smokeReportDate").value =
            new Date().toISOString().slice(0, 10);
    });

    // form.reset() unchecks the radios but cannot repaint the stars, since the
    // fill lives in a class star-rating.js applies. Re-sync after a successful
    // submit so the next open starts from a clean widget.
    modalEl.addEventListener("hidden.bs.modal", () => {
        form.querySelectorAll(".star-rating").forEach((group) => {
            window.starRating.set(group, window.starRating.value(group));
        });
    });

    form.addEventListener("submit", (e) => {
        e.preventDefault();

        // One row per criterion; an unrated criterion has no checked radio and
        // is simply left out of the payload — scores are optional one by one.
        const scores = [];
        document.querySelectorAll("#addSmokeReportForm .smoke-score-row").forEach((row) => {
            const value = window.starRating.value(row.querySelector(".star-rating"));
            if (value) {
                scores.push({
                    criterion_id: parseInt(row.dataset.criterionId, 10),
                    value: value,
                });
            }
        });

        const payload = {
            report_date: document.getElementById("smokeReportDate").value,
            consumption_method: document.getElementById("smokeConsumptionMethod").value,
            rating: window.starRating.value(document.getElementById("smokeRating")),
            description: document.getElementById("smokeDescription").value,
            scores: scores,
        };

        // csrf.js injects X-CSRF-Token into this fetch automatically.
        fetch("/plant/" + form.dataset.plantId + "/smoke-reports", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then((response) => {
                if (!response.ok) {
                    return response.json().catch(() => ({})).then((b) => {
                        throw new Error(b.error || "request failed");
                    });
                }
                bootstrap.Modal.getInstance(modalEl).hide();
                form.reset();
                return window.smokeReports.reload();
            })
            .catch((err) => uiMessages.showToast(err.message, "danger"));
    });
});
