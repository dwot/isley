document.addEventListener("DOMContentLoaded", () => {
    const editMeasurementModal = new bootstrap.Modal(document.getElementById("editMeasurementModal"));
    const measurementForm = document.getElementById("editMeasurementForm");
    const deleteMeasurementButton = document.getElementById("deleteMeasurement");

    document.querySelectorAll(".measurement-row").forEach(row => {
        row.addEventListener("click", () => {
            const measurementData = JSON.parse(row.getAttribute("data-measurement"));

            document.getElementById("measurementId").value = measurementData.id;
            document.getElementById("editMeasurementDate").value = IsleyTime.toInput(measurementData.date);
            document.getElementById("editMeasurementValue").value = measurementData.value;

            editMeasurementModal.show();
        });
    });

    measurementForm.addEventListener("submit", (e) => {
        e.preventDefault();

        const payload = {
            id: parseInt(document.getElementById("measurementId").value, 10),
            date: document.getElementById("editMeasurementDate").value,
            value: parseFloat(document.getElementById("editMeasurementValue").value),
        };

        fetch("/plantMeasurement/edit", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then(response => response.json())
            .then(() => location.reload())
            .catch(err => uiMessages.showToast(uiMessages.t('failed_to_update_measurement'), 'danger'));
    });

    // Only logged-in users get a Delete button.
    if (deleteMeasurementButton) deleteMeasurementButton.addEventListener("click", () => {
        const measurementId = document.getElementById("measurementId").value;

        uiMessages.confirmDelete(uiMessages.t('confirm_delete_measurement', 'Are you sure you want to delete this measurement?'), uiMessages.t('delete_measurement', 'Delete Measurement')).then(confirmed => {
            if (!confirmed) return;
            fetch(`/plantMeasurement/delete/${measurementId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => location.reload())
                .catch(err => uiMessages.showToast(uiMessages.t('failed_to_delete_measurement'), 'danger'));
        });
    });
});