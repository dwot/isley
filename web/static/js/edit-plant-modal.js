document.addEventListener("DOMContentLoaded", () => {
    const changeStatusModal = document.getElementById("changeStatusModal");
    const statusDateInput = document.getElementById("statusDate");
    const zoneSelect = document.getElementById("zoneSelect");
    const newZoneInput = document.getElementById("newZoneInput");
    const strainSelect = document.getElementById("strainSelect");
    const newStrainInputs = document.getElementById("newStrainInputs");
    const breederSelect = document.getElementById("breederSelect");
    const newBreederInput = document.getElementById("newBreederInput");
    const newZoneName = document.getElementById("newZoneName");
    const newStrainName = document.getElementById("newStrainName");
    const newBreederName = document.getElementById("newBreederName");
    const harvestWeight = document.getElementById("harvestWeight");

    // Set default date to today
    //const setDefaultDate = () => {
    //    const today = new Date().toISOString().split("T")[0];
    //    statusDateInput.value = today;
    //};

    // Reset Zone Selection
    const resetZoneSelection = () => {
        zoneSelect.disabled = false;
        zoneSelect.value = "";
        newZoneInput.classList.add("d-none");
    };

    // Reset Strain Selection
    const resetStrainSelection = () => {
        strainSelect.value = "";
        newStrainInputs.classList.add("d-none");
        resetBreederSelection();
    };

    // Reset Breeder Selection
    const resetBreederSelection = () => {
        breederSelect.value = "";
        newBreederInput.classList.add("d-none");
    };


    // Set default date when the modal is shown
    //changeStatusModal.addEventListener("show.bs.modal", () => {
    //    setDefaultDate();
    //});

    // Show/Hide New Zone Input
    zoneSelect.addEventListener("change", () => {
        if (zoneSelect.value === "new") {
            newZoneInput.classList.remove("d-none");
        } else {
            newZoneInput.classList.add("d-none");
        }
    });

    // Show/Hide New Strain Inputs
    strainSelect.addEventListener("change", () => {
        if (strainSelect.value === "new") {
            newStrainInputs.classList.remove("d-none");
        } else {
            newStrainInputs.classList.add("d-none");
        }
    });

    // Show/Hide New Breeder Input
    breederSelect.addEventListener("change", () => {
        if (breederSelect.value === "new") {
            newBreederInput.classList.remove("d-none");
        } else {
            newBreederInput.classList.add("d-none");
        }
    });

    // Handle form submission
    const changeStatusForm = document.getElementById("changeStatusForm");
    changeStatusForm.addEventListener("submit", (e) => {
        e.preventDefault();

        // Gather form data
        const plantId = document.getElementById("plantId").value;
        const status = document.getElementById("status").value;
        const date = statusDateInput.value;
        const startDate = document.getElementById("startDate").value;

        const payload = {
            plant_id: parseInt(plantId, 10),
            plant_name: document.getElementById("plantName").value,
            plant_description: document.getElementById("plantDescription").value,
            status_id: parseInt(status, 10),
            date: date,
            zone_id: parseInt(zoneSelect.value, 10),
            new_zone: newZoneName.value,
            strain_id: parseInt(strainSelect.value, 10),
            new_strain: strainSelect.value === "new" ? {
                name: newStrainName.value,
                breeder_id: breederSelect.value === "new" ? null : parseInt(breederSelect.value, 10),
                new_breeder: breederSelect.value === "new" ? newBreederName.value : null
            } : null,
            clone: document.getElementById("cloneCheckbox").checked,
            start_date: startDate,
            harvest_weight: parseFloat(harvestWeight.value),
        };

        //alert(JSON.stringify(payload));

        // Send POST request to change status
        fetch("/plant", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify(payload),
        })
            .then(response => {
                if (!response.ok) {
                    throw new Error("{{ .lcl.failed_to_change_status }}");
                }
                return response.json();
            })
            .then(data => {
                //alert("Status changed successfully!");
                location.reload(); // Reload the page to reflect changes
            })
            .catch(error => {
                console.error("Error:", error);
                alert("{{ .lcl.failed_to_change_status }}");
            });
    });
});