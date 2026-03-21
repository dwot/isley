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
                location.reload();
            })
            .catch(error => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t('failed_to_link_sensors') || '{{ .lcl.failed_to_link_sensors }}', 'danger');
            });
    });

    // Handle Unlink All Sensors
    const unlinkAllBtn = document.getElementById("unlinkAllSensorsBtn");
    if (unlinkAllBtn) {
        unlinkAllBtn.addEventListener("click", () => {
            // Close the link-sensor modal first to avoid stacked-modal conflicts
            const linkModalEl = document.getElementById("linkSensorModal");
            const linkModal = bootstrap.Modal.getInstance(linkModalEl);
            if (linkModal) linkModal.hide();

            // Wait for modal to fully close before showing confirm
            const onHidden = () => {
                linkModalEl.removeEventListener("hidden.bs.modal", onHidden);

                const msg = uiMessages.t('confirm_unlink_all_sensors') || 'Unlink all sensors from this plant?';
                const doUnlink = (window.uiMessages && typeof uiMessages.showConfirm === "function")
                    ? uiMessages.showConfirm(msg)
                    : Promise.resolve(confirm(msg));

                doUnlink.then(confirmed => {
                    if (!confirmed) return;
                    const plantId = document.getElementById("plantId").value;
                    fetch("/plant/link-sensors", {
                        method: "POST",
                        headers: { "Content-Type": "application/json" },
                        body: JSON.stringify({ plant_id: plantId, sensor_ids: [] }),
                    })
                        .then(response => {
                            if (!response.ok) throw new Error("Failed");
                            location.reload();
                        })
                        .catch(error => {
                            console.error("Error:", error);
                            uiMessages.showToast(uiMessages.t('failed_to_link_sensors') || 'Failed to unlink sensors', 'danger');
                        });
                });
            };

            linkModalEl.addEventListener("hidden.bs.modal", onHidden);
        });
    }
});
