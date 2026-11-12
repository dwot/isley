document.addEventListener("DOMContentLoaded", () => {
    const editActivityModal = new bootstrap.Modal(document.getElementById("editActivityModal"));
    const activityForm = document.getElementById("editActivityForm");
    const deleteActivityButton = document.getElementById("deleteActivity");
    const activityTypeSelect = document.getElementById("editActivityType");
    const measurementsContainer = document.getElementById("editActivityMeasurements");

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
            document.getElementById("editActivityDate").value = IsleyTime.toInput(activityData.date);
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

    // Only logged-in users get a Delete button.
    if (deleteActivityButton) deleteActivityButton.addEventListener("click", () => {
        const activityId = document.getElementById("activityId").value;

        uiMessages.confirmDelete(uiMessages.t('confirm_delete_activity', 'Are you sure you want to delete this activity?'), uiMessages.t('delete_activity', 'Delete Activity')).then(confirmed => {
            if (!confirmed) return;
            fetch(`/plantActivity/delete/${activityId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => location.reload())
                .catch(err => uiMessages.showToast(uiMessages.t('failed_to_delete_activity'), 'danger'));
        });
    });
});