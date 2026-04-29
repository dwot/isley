document.addEventListener("DOMContentLoaded", () => {
    const form = document.getElementById("addActivityForm");
    const addActivityModal = document.getElementById("addActivityModal");
    const activityDateInput = document.getElementById("activityDate");
    const activitySelect = document.getElementById("activityName");
    const measurementsContainer = document.getElementById("addActivityMeasurements");

    async function updateMeasurementInputs() {
        const links = activityMetrics.getLinksFromSelect(activitySelect);
        await activityMetrics.renderInputs(measurementsContainer, links);
    }

    activitySelect.addEventListener("change", updateMeasurementInputs);

    // Set default date/time when the modal is shown
    addActivityModal.addEventListener("show.bs.modal", () => {
        formHelpers.setDateTimeNow("activityDate");
        updateMeasurementInputs();
    });

    form.addEventListener("submit", (e) => {
        e.preventDefault();

        // Gather form data
        const plantId = document.getElementById("plantId").value;
        const activityId = document.getElementById("activityName").value;
        const activityNote = document.getElementById("activityNote").value;
        const date = activityDateInput.value;

        // Construct the payload
        const payload = {
            plant_id: parseInt(plantId, 10),
            activity_id: parseInt(activityId, 10),
            note: activityNote,
            date: date,
            measurements: activityMetrics.collectValues(measurementsContainer),
        };

        // Send POST request to /plantActivity
        fetch("/plantActivity", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify(payload),
        })
            .then((response) => {
                if (!response.ok) {
                    throw new Error("{{ .lcl.failed_to_add_activity }}");
                }
                return response.json();
            })
            .then((data) => {
                // Success: Close modal and reload page
                const modal = bootstrap.Modal.getInstance(document.getElementById("addActivityModal"));
                modal.hide();
                window.location.reload(); // Refresh page to show updated data
            })
            .catch((error) => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t('failed_to_add_activity') || '{{ .lcl.failed_to_add_activity }}', 'danger');
            });
    });
});