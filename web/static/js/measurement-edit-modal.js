document.addEventListener("DOMContentLoaded", () => {
    const editMeasurementModal = new bootstrap.Modal(document.getElementById("editMeasurementModal"));
    const measurementForm = document.getElementById("editMeasurementForm");
    const deleteMeasurementButton = document.getElementById("deleteMeasurement");

    document.querySelectorAll(".measurement-row").forEach(row => {
        row.addEventListener("click", () => {
            const measurementData = JSON.parse(row.getAttribute("data-measurement"));

            document.getElementById("measurementId").value = measurementData.id;
            const formattedDate = measurementData.date.split("T")[0];
            document.getElementById("editMeasurementDate").value = formattedDate;
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
            .catch(err => alert("{{ .lcl.failed_to_update_measurement }}"));
    });

    deleteMeasurementButton.addEventListener("click", () => {
        const measurementId = document.getElementById("measurementId").value;

        if (confirm("{{ .lcl.confirm_delete_measurement }}")) {
            fetch(`/plantMeasurement/delete/${measurementId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => location.reload())
                .catch(err => alert("{{ .lcl.failed_to_delete_measurement }}"));
        }
    });
});