document.addEventListener("DOMContentLoaded", () => {
    // Handle Linking Sensors
    const linkSensorForm = document.getElementById("linkSensorForm");
    linkSensorForm.addEventListener("submit", (e) => {
        e.preventDefault();
        const plantId = document.getElementById("plantId").value;
        const formData = new FormData(linkSensorForm);
        const sensorIds = formData.getAll("sensorIds").map(id => parseInt(id, 10));
        const payload = {
            plant_id: plantId,
            sensor_ids: sensorIds
        };

        fetch("/plant/link-sensors", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify(payload),
        })
            .then(response => {
                if (!response.ok) throw new Error("{{ .lcl.failed_to_link_sensors }}");
                //alert("Sensors linked successfully!");
                location.reload();
            })
            .catch(error => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t('failed_to_link_sensors') || '{{ .lcl.failed_to_link_sensors }}', 'danger');
            });
    });
});