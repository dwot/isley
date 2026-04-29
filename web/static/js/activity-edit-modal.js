document.addEventListener("DOMContentLoaded", () => {
    const editActivityModal = new bootstrap.Modal(document.getElementById("editActivityModal"));
    const activityForm = document.getElementById("editActivityForm");
    const deleteActivityButton = document.getElementById("deleteActivity");
    const activityTypeSelect = document.getElementById("editActivityType");
    const measurementsContainer = document.getElementById("editActivityMeasurements");

    // Format a Date as a local datetime-local string (YYYY-MM-DDTHH:MM:SS)
    // without converting to UTC (unlike toISOString which shifts timezone).
    function toLocalDateTimeString(d) {
        const pad = (n) => String(n).padStart(2, '0');
        return `${d.getFullYear()}-${pad(d.getMonth()+1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
    }

    async function updateMeasurementInputs(existingValues) {
        const links = activityMetrics.getLinksFromSelect(activityTypeSelect);
        await activityMetrics.renderInputs(measurementsContainer, links, existingValues);
    }

    activityTypeSelect.addEventListener("change", () => updateMeasurementInputs());

    document.querySelectorAll(".activity-row").forEach(row => {
        row.addEventListener("click", (e) => {
            // Let anchors inside the row navigate normally (e.g. plant links
            // in the /activities log) without also opening the edit modal.
            if (e.target && e.target.closest("a")) return;

            const activityData = JSON.parse(row.getAttribute("data-activity"));

            document.getElementById("activityId").value = activityData.id;
            const date = new Date(activityData.date);
            document.getElementById("editActivityDate").value = toLocalDateTimeString(date);
            document.getElementById("editActivityType").value = activityData.activity_id;
            document.getElementById("editActivityNote").value = activityData.note;

            // Build existing values map from activity measurements
            const existingValues = {};
            if (activityData.measurements) {
                activityData.measurements.forEach(m => {
                    existingValues[m.metric_id] = m.value;
                });
            }
            updateMeasurementInputs(existingValues);

            editActivityModal.show();
        });
    });

    activityForm.addEventListener("submit", (e) => {
        e.preventDefault();

        const payload = {
            id: parseInt(document.getElementById("activityId").value, 10),
            date: document.getElementById("editActivityDate").value,
            activity_id: parseInt(document.getElementById("editActivityType").value, 10),
            note: document.getElementById("editActivityNote").value,
            measurements: activityMetrics.collectValues(measurementsContainer),
        };

        fetch("/plantActivity/edit", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then(response => response.json())
            .then(() => location.reload())
            .catch(err => uiMessages.showToast(uiMessages.t('failed_to_update_activity'), 'danger'));
    });

    deleteActivityButton.addEventListener("click", () => {
        const activityId = document.getElementById("activityId").value;

        uiMessages.showConfirm(uiMessages.t('confirm_delete_activity')).then(confirmed => {
            if (!confirmed) return;
            fetch(`/plantActivity/delete/${activityId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => location.reload())
                .catch(err => uiMessages.showToast(uiMessages.t('failed_to_delete_activity'), 'danger'));
        });
    });
});