document.addEventListener("DOMContentLoaded", () => {
    const form = document.getElementById("addMeasurementForm");
    const addMeasurementModal = document.getElementById("addMeasurementModal");
    const measurementDateInput = document.getElementById("measureDate");

    // Set default date to today
    //const setDefaultDate = () => {
    //    const today = new Date().toISOString().split("T")[0];
    //    measurementDateInput.value = today;
    //};

    // Set default date when the modal is shown
    //addMeasurementModal.addEventListener("show.bs.modal", setDefaultDate);

    form.addEventListener("submit", (e) => {
        e.preventDefault();

        // Gather form data
        const plantId = document.getElementById("plantId").value;
        const metricId = document.getElementById("measurementName").value;
        const value = document.getElementById("measurementValue").value;
        const date = measurementDateInput.value;

        // Construct the payload
        const payload = {
            plant_id: parseInt(plantId, 10),
            metric_id: parseInt(metricId, 10),
            value: parseFloat(value),
            date: date,
        };

        // Send POST request to /plantMeasurement
        fetch("/plantMeasurement", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify(payload),
        })
            .then((response) => {
                if (!response.ok) {
                    throw new Error("{{ .lcl.failed_to_add_measurement }}");
                }
                return response.json();
            })
            .then((data) => {
                // Success: Close modal and reload page
                const modal = bootstrap.Modal.getInstance(document.getElementById("addMeasurementModal"));
                modal.hide();
                window.location.reload(); // Refresh page to show updated data
            })
            .catch((error) => {
                console.error("Error:", error);
                alert("{{ .lcl.failed_to_add_measurement }}");
            });
    });
});