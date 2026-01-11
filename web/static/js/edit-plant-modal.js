document.addEventListener("DOMContentLoaded", () => {
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
        const startDate = document.getElementById("startDate").value;

        const zoneVal = zoneSelect.value;
        const strainVal = strainSelect.value;

        const payload = {
            plant_id: parseInt(plantId, 10),
            plant_name: document.getElementById("plantName").value,
            plant_description: document.getElementById("plantDescription").value,
            zone_id: zoneVal && zoneVal !== 'new' ? parseInt(zoneVal, 10) : null,
            new_zone: zoneVal === 'new' ? newZoneName.value : '',
            strain_id: strainVal && strainVal !== 'new' ? parseInt(strainVal, 10) : null,
            new_strain: strainVal === 'new' ? {
                name: newStrainName.value,
                breeder_id: breederSelect.value === "new" ? null : parseInt(breederSelect.value, 10),
                new_breeder: breederSelect.value === "new" ? newBreederName.value : null
            } : null,
            clone: document.getElementById("cloneCheckbox").checked,
            start_date: startDate,
            harvest_weight: parseFloat(harvestWeight.value),
        };

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
            .then(() => {
                location.reload(); // Reload the page to reflect changes
            })
            .catch(error => {
                console.error("Error:", error);
                alert("{{ .lcl.failed_to_change_status }}");
            });
    });
});