document.addEventListener("DOMContentLoaded", () => {
    const form = document.getElementById("addMultiPlantActivityForm");
    const addMultiPlantActivityModal = document.getElementById("addMultiPlantActivityModal");
    const activityMultiDateInput = document.getElementById("activityMultiDate");
    const activitySelect = document.getElementById("activityMultiName");
    const measurementsContainer = document.getElementById("multiActivityMeasurements");

    async function updateMeasurementInputs() {
        const links = activityMetrics.getLinksFromSelect(activitySelect);
        await activityMetrics.renderInputs(measurementsContainer, links);
    }

    activitySelect.addEventListener("change", updateMeasurementInputs);

    // Set default date/time when the modal is shown
    addMultiPlantActivityModal.addEventListener("show.bs.modal", () => {
        formHelpers.setDateTimeNow("activityMultiDate");
        updateMeasurementInputs();
    });

    form.addEventListener("submit", (e) => {
        e.preventDefault();

        // Gather selected plant IDs
        const selectedPlants = Array.from(document.getElementById("plantSelection").selectedOptions).map(opt => parseInt(opt.value));

        // Gather other form data
        const payload = {
            plant_ids: selectedPlants,
            activity_id: parseInt(document.getElementById("activityMultiName").value),
            note: document.getElementById("activityMultiNote").value,
            date: document.getElementById("activityMultiDate").value,
            measurements: activityMetrics.collectValues(measurementsContainer),
        };

        // Send POST request to backend
        fetch("/record-multi-activity", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify(payload)
        })
            .then(response => {
                if (!response.ok) throw new Error("Failed to record activity");
                return response.json();
            })
            .then(() => {
                const modal = bootstrap.Modal.getInstance(document.getElementById("addMultiPlantActivityModal"));
                modal.hide();
                location.reload(); // Reload page to reflect changes
            })
            .catch(error => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t('error_recording_activity') || 'Error recording activity. Please try again.', 'danger');
            });
    });
});