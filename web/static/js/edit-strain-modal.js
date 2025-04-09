document.addEventListener("DOMContentLoaded", () => {
    const editStrainModal = new bootstrap.Modal(document.getElementById("editStrainModal"));
    const editStrainForm = document.getElementById("editStrainForm");
    const deleteStrainButton = document.getElementById("deleteStrainButton");
    const editBreederSelect = document.getElementById("editBreederSelect");
    const editNewBreederInput = document.getElementById("editNewBreederInput");
    const editIndicaSativaSlider = document.getElementById("editIndicaSativaSlider");
    const editIndicaLabel = document.getElementById("editIndicaLabel");
    const editSativaLabel = document.getElementById("editSativaLabel");
    const editCycleTime = document.getElementById("editCycleTime");
    const editUrl = document.getElementById("editUrl");

    // Show/Hide New Breeder Input
    editBreederSelect.addEventListener("change", () => {
        if (editBreederSelect.value === "new") {
            editNewBreederInput.classList.remove("d-none");
        } else {
            editNewBreederInput.classList.add("d-none");
        }
    });

    // Update Indica/Sativa labels dynamically
    editIndicaSativaSlider.addEventListener("input", () => {
        const indica = editIndicaSativaSlider.value;
        const sativa = 100 - indica;
        editIndicaLabel.textContent = `Indica: ${indica}%`;
        editSativaLabel.textContent = `Sativa: ${sativa}%`;
    });

    // Handle form submission
    editStrainForm.addEventListener("submit", (e) => {
        e.preventDefault();

        const strainId = document.getElementById("editStrainId").value;
        const payload = {
            name: document.getElementById("editStrainName").value,
            breeder_id: editBreederSelect.value === "new" ? null : parseInt(editBreederSelect.value, 10),
            new_breeder: editBreederSelect.value === "new" ? document.getElementById("editNewBreederName").value : null,
            indica: parseInt(editIndicaSativaSlider.value, 10),
            sativa: 100 - parseInt(editIndicaSativaSlider.value, 10),
            autoflower: document.getElementById("editAutoflower").value === "true",
            seed_count: parseInt(document.getElementById("editSeedCount").value, 10),
            description: document.getElementById("editStrainDescription").value,
            short_desc: document.getElementById("editStrainShortDescription").value,
            cycle_time: parseInt(editCycleTime.value, 10),
            url: editUrl.value
        };
        fetch(`/strains/${strainId}`, {
            method: "PUT",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify(payload),
        })
            .then(response => {
                if (!response.ok) throw new Error("{{ .lcl.strain_update_fail }}");
                location.reload();
            })
            .catch(error => {
                console.error("{{ .lcl.strain_update_error }}", error);
                alert("{{ .lcl.update_error }}");
            });
    });

    deleteStrainButton.addEventListener("click", () => {
        const strainId = document.getElementById("editStrainId").value;

        if (confirm("Are you sure you want to delete this strain?")) {
            fetch(`/strains/${strainId}`, { method: "DELETE" })
                .then(response => {
                    if (!response.ok) throw new Error("{{ .lcl.delete_fail }}");

                    // ✅ Redirect only after successful deletion
                    window.location.href = "/strains";
                })
                .catch(error => {
                    console.error("Error deleting strain:", error);
                    alert("{{ .lcl.delete_error }}");
                });
        }
    });

});

// Populate modal fields when editing
const editModal = document.getElementById('editStrainModal');

editModal.addEventListener('show.bs.modal', function (event) {
    const button = event.relatedTarget;
    const encodedStrain = button.getAttribute('data-strain');

    if (!encodedStrain) return;

    // Decode HTML entities like &#34;
    const parser = new DOMParser();
    const decodedHtml = parser.parseFromString(encodedStrain, 'text/html').documentElement.textContent;

    // Now parse the JSON
    const strainData = JSON.parse(decodedHtml);

    // Populate form fields
    document.getElementById("editStrainId").value = strainData.id;
    document.getElementById("editStrainName").value = strainData.name;
    editBreederSelect.value = strainData.breeder_id || "new";

    if (strainData.breeder_id) {
        editNewBreederInput.classList.add("d-none");
        document.getElementById("editNewBreederName").value = "";
    } else {
        editNewBreederInput.classList.remove("d-none");
        document.getElementById("editNewBreederName").value = strainData.new_breeder || "";
    }

    document.getElementById("editIndicaSativaSlider").value = strainData.indica;
    editIndicaLabel.textContent = `Indica: ${strainData.indica}%`;
    editSativaLabel.textContent = `Sativa: ${100 - strainData.indica}%`;
    if (strainData.autoflower === "true" || strainData.autoflower === true || strainData.autoflower === 1) {
        document.getElementById("editAutoflower").value = "true";
    } else {
        document.getElementById("editAutoflower").value = "false";
    }
    document.getElementById("editAutoflower").value = strainData.autoflower;
    document.getElementById("editSeedCount").value = strainData.seed_count;
    document.getElementById("editStrainDescription").value = strainData.description;
    document.getElementById("editCycleTime").value = strainData.cycle_time;
    document.getElementById("editUrl").value = strainData.url;
    document.getElementById("editStrainShortDescription").value = strainData.short_desc;

    // Show the modal
    editModal.show();
});