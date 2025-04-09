document.addEventListener("DOMContentLoaded", () => {
    const addPlantForm = document.getElementById("addPlantForm");
    const zoneSelect = document.getElementById("zoneSelect");
    const newZoneInput = document.getElementById("newZoneInput");
    const strainSelect = document.getElementById("strainSelect");
    const parentPlantDropdown = document.getElementById("parentPlantDropdown");
    const parentPlantSelect = document.getElementById("parentPlantSelect");
    const newStrainInputs = document.getElementById("newStrainInputs");
    const breederSelect = document.getElementById("breederSelect");
    const newBreederInput = document.getElementById("newBreederInput");
    const plantName = document.getElementById("plantName");
    const statusSelect = document.getElementById("statusSelect");
    const newZoneName = document.getElementById("newZoneName");
    const newStrainName = document.getElementById("newStrainName");
    const newBreederName = document.getElementById("newBreederName");
    const startDt = document.getElementById("startDate");
    const addPlantModal = document.getElementById("addPlantModal");
    const isClone = document.getElementById("isClone");
    const decrementSeedCount = document.getElementById("decrementSeedCount");

    // Set default date to today
    //const setDefaultDate = () => {
    //    const today = new Date().toISOString().split("T")[0];
    //    startDt.value = today;
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

    // Reset Parent Plant Selection
    const resetParentPlantSelection = () => {
        parentPlantSelect.value = "0";
        parentPlantDropdown.classList.add("d-none");
    };

    isClone.addEventListener("change", () => {
        if (isClone.checked) {
            parentPlantDropdown.classList.remove("d-none");
        } else {
            parentPlantDropdown.classList.add("d-none");
            parentPlantSelect.innerHTML = '<option value="0">{{ .lcl.title_none }}</option>';
        }
    });

    strainSelect.addEventListener("change", () => {
        if (strainSelect.value === "new") {
            parentPlantDropdown.classList.add("d-none");
            parentPlantSelect.innerHTML = '<option value="0">{{ .lcl.title_none }}</option>';
        } else {
            fetch(`/plants/by-strain/${strainSelect.value}`)
                .then(response => response.json())
                .then(plants => {
                    parentPlantSelect.innerHTML = '<option value="0">{{ .lcl.title_none }}</option>';
                    plants.forEach(plant => {
                        parentPlantSelect.innerHTML += `<option value="${plant.id}">${plant.name}</option>`;
                    });
                    //parentPlantDropdown.classList.remove("d-none");
                });
        }
    });

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

    addPlantModal.addEventListener("show.bs.modal", () => {
        resetZoneSelection();
        resetStrainSelection();
    });

    addPlantForm.addEventListener("submit", (e) => {
        e.preventDefault();
        const payload = {
            name: plantName.value,
            zone_id: zoneSelect.value === "new" ? null : parseInt(zoneSelect.value, 10),
            new_zone: zoneSelect.value === "new" ? newZoneName.value : null,
            strain_id: strainSelect.value === "new" ? null : parseInt(strainSelect.value, 10),
            new_strain: strainSelect.value === "new" ? {
                name: newStrainName.value,
                breeder_id: breederSelect.value === "new" ? null : parseInt(breederSelect.value, 10),
                new_breeder: breederSelect.value === "new" ? newBreederName.value : null
            } : null,
            status_id: parseInt(statusSelect.value, 10),
            date: startDt.value,
            clone: isClone.checked ? 1 : 0,
            parent_id: parseInt(parentPlantSelect.value, 10),
            decrement_seed_count: decrementSeedCount.checked,
        };
        fetch("/plants", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then(response => {
                if (!response.ok) throw new Error("Failed to add plant");
                location.reload();
            })
            .catch(error => alert("Failed to add plant. Try again."));
    });
});
