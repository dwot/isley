document.addEventListener("DOMContentLoaded", () => {
    const editPlantForm = document.getElementById("editPlantForm");
    const zoneSelect = document.getElementById("zoneSelect");
    const newZoneInput = document.getElementById("newZoneInput");
    const strainSelect = document.getElementById("strainSelect");
    const newStrainCard = document.getElementById("newStrainCard");
    const breederSelect = document.getElementById("breederSelect");
    const newBreederInput = document.getElementById("newBreederInput");
    const deletePlantButton = document.getElementById("deletePlantButton");
    const plantId = document.getElementById("plantId").value;

    // Initialize autocomplete on strain and zone selects
    const zoneAC = new IsleyAutocomplete(zoneSelect, {
        placeholder: "Type to search zones...",
    });
    const strainAC = new IsleyAutocomplete(strainSelect, {
        placeholder: "Type to search strains...",
    });

    // Show/Hide New Zone Input
    zoneSelect.addEventListener("change", () => {
        newZoneInput.classList.toggle("d-none", zoneSelect.value !== "new");
    });

    // Show/Hide New Strain Card
    strainSelect.addEventListener("change", () => {
        newStrainCard.classList.toggle("d-none", strainSelect.value !== "new");
    });

    // Show/Hide New Breeder Input
    if (breederSelect) {
        breederSelect.addEventListener("change", () => {
            newBreederInput.classList.toggle("d-none", breederSelect.value !== "new");
        });
    }

    // Form submission
    editPlantForm.addEventListener("submit", (e) => {
        e.preventDefault();

        const submitBtn = editPlantForm.querySelector('button[type="submit"]');
        submitBtn.classList.add("is-loading");
        submitBtn.disabled = true;

        const zoneVal = zoneSelect.value;
        const strainVal = strainSelect.value;
        const newZoneName = document.getElementById("newZoneName");
        const newStrainName = document.getElementById("newStrainName");
        const newBreederName = document.getElementById("newBreederName");

        const payload = {
            plant_id: parseInt(plantId, 10),
            plant_name: document.getElementById("plantName").value,
            plant_description: document.getElementById("plantDescription").value,
            zone_id: zoneVal && zoneVal !== "new" ? parseInt(zoneVal, 10) : null,
            new_zone: zoneVal === "new" ? newZoneName.value : "",
            strain_id: strainVal && strainVal !== "new" ? parseInt(strainVal, 10) : null,
            new_strain: strainVal === "new" ? {
                name: newStrainName.value,
                breeder_id: breederSelect && breederSelect.value !== "new" ? parseInt(breederSelect.value, 10) : null,
                new_breeder: breederSelect && breederSelect.value === "new" ? newBreederName.value : null
            } : null,
            clone: document.getElementById("cloneCheckbox").checked,
            start_date: document.getElementById("startDate").value,
            harvest_weight: parseFloat(document.getElementById("harvestWeight").value) || 0,
        };

        fetch("/plant", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
        .then(response => {
            if (!response.ok) throw new Error("Failed to update plant");
            return response.json();
        })
        .then(() => {
            window.location.href = `/plant/${plantId}`;
        })
        .catch(error => {
            submitBtn.classList.remove("is-loading");
            submitBtn.disabled = false;
            console.error("Error:", error);
            uiMessages.showToast(uiMessages.t('failed_save_changes') || 'Failed to save changes', 'danger');
        });
    });

    // Delete plant
    if (deletePlantButton) {
        deletePlantButton.addEventListener("click", () => {
            // Use the global confirm modal if available
            const confirmModal = document.getElementById("confirmModal");
            if (confirmModal && window.bootstrap) {
                const confirmBody = confirmModal.querySelector(".modal-body");
                const confirmBtn = confirmModal.querySelector(".confirm-action-btn");
                if (confirmBody) confirmBody.textContent = "Are you sure you want to delete this plant? This action cannot be undone.";
                if (confirmBtn) {
                    const newBtn = confirmBtn.cloneNode(true);
                    confirmBtn.parentNode.replaceChild(newBtn, confirmBtn);
                    newBtn.addEventListener("click", () => {
                        doDelete();
                        bootstrap.Modal.getInstance(confirmModal)?.hide();
                    });
                }
                new bootstrap.Modal(confirmModal).show();
            } else {
                if (confirm("Are you sure you want to delete this plant?")) {
                    doDelete();
                }
            }
        });
    }

    function doDelete() {
        fetch(`/plant/delete/${plantId}`, { method: "DELETE" })
            .then(response => {
                if (!response.ok) throw new Error("Failed to delete plant");
                window.location.href = "/plants";
            })
            .catch(error => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t('api_failed_to_delete_plant') || 'Failed to delete plant', 'danger');
            });
    }
});
