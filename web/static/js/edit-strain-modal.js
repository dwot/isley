document.addEventListener("DOMContentLoaded", () => {
    const editStrainForm = document.getElementById("editStrainForm");
    const deleteStrainButton = document.getElementById("deleteStrainButton");
    const editBreederSelect = document.getElementById("editBreederSelect");
    const editNewBreederInput = document.getElementById("editNewBreederInput");
    const editIndicaSativaSlider = document.getElementById("editIndicaSativaSlider");
    const editIndicaLabel = document.getElementById("editIndicaLabel");
    const editSativaLabel = document.getElementById("editSativaLabel");
    const editCycleTime = document.getElementById("editCycleTime");
    const editUrl = document.getElementById("editUrl");

    // Helper to populate modal fields from a strain object
    function populateFields(strainData) {
        // strainData may use snake_case keys from API or keys used in list rendering
        const id = strainData.id || strainData.ID || strainData.Id;
        const name = strainData.name || strainData.Name;
        const breeder_id = strainData.breeder_id || strainData.breederId || strainData.BreederID || strainData.Breeder || null;
        const new_breeder = strainData.new_breeder || strainData.NewBreeder || "";
        const indica = (typeof strainData.indica !== 'undefined') ? strainData.indica : (strainData.Indica || 50);
        const autoflower = (typeof strainData.autoflower !== 'undefined') ? strainData.autoflower : (strainData.Autoflower || false);
        const seed_count = strainData.seed_count || strainData.SeedCount || 0;
        const description = strainData.description || strainData.Description || "";
        const cycle_time = strainData.cycle_time || strainData.CycleTime || 56;
        const url = strainData.url || strainData.Url || "";
        const short_desc = strainData.short_desc || strainData.ShortDescription || strainData.ShortDescription || "";

        // Populate form fields
        if (document.getElementById("editStrainId")) document.getElementById("editStrainId").value = id || "";
        if (document.getElementById("editStrainName")) document.getElementById("editStrainName").value = name || "";

        if (editBreederSelect) {
            // If breeder_id is present and matches an option, select it; otherwise select "new"
            let selected = "new";
            if (breeder_id) {
                // Try to find matching option
                const opt = Array.from(editBreederSelect.options).find(o => o.value === String(breeder_id) || o.text === breeder_id);
                if (opt) selected = opt.value;
            }
            editBreederSelect.value = selected;

            if (selected === "new") {
                if (editNewBreederInput) editNewBreederInput.classList.remove("d-none");
                if (document.getElementById("editNewBreederName")) document.getElementById("editNewBreederName").value = new_breeder || "";
            } else {
                if (editNewBreederInput) editNewBreederInput.classList.add("d-none");
                if (document.getElementById("editNewBreederName")) document.getElementById("editNewBreederName").value = "";
            }
        }

        if (editIndicaSativaSlider) {
            editIndicaSativaSlider.value = indica;
            if (editIndicaLabel) editIndicaLabel.textContent = `Indica: ${indica}%`;
            if (editSativaLabel) editSativaLabel.textContent = `Sativa: ${100 - indica}%`;
        }

        if (document.getElementById("editAutoflower")) {
            // Support boolean true/false or 1/0 or string
            const af = (autoflower === true || autoflower === 1 || autoflower === "true" || autoflower === "1");
            document.getElementById("editAutoflower").value = af ? "true" : "false";
        }

        if (document.getElementById("editSeedCount")) document.getElementById("editSeedCount").value = seed_count || 0;
        if (document.getElementById("editStrainDescription")) document.getElementById("editStrainDescription").value = description || "";
        if (document.getElementById("editCycleTime")) document.getElementById("editCycleTime").value = cycle_time || "";
        if (document.getElementById("editUrl")) document.getElementById("editUrl").value = url || "";
        if (document.getElementById("editStrainShortDescription")) document.getElementById("editStrainShortDescription").value = short_desc || "";
    }

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

    // Populate modal when it's shown (supports triggers from strain list & details)
    const modalEl = document.getElementById("editStrainModal");
    if (modalEl) {
        modalEl.addEventListener('show.bs.modal', (event) => {
            const trigger = event.relatedTarget;
            if (!trigger) return;

            const strainAttr = trigger.getAttribute('data-strain');
            const strainIdAttr = trigger.getAttribute('data-id') || trigger.getAttribute('data-id');

            if (strainAttr) {
                let parsed = null;
                try {
                    // try decodeURIComponent (list encodes JSON)
                    parsed = JSON.parse(decodeURIComponent(strainAttr));
                } catch (e) {
                    try {
                        parsed = JSON.parse(strainAttr);
                    } catch (err) {
                        console.error('Failed to parse strain data attribute', err);
                    }
                }
                if (parsed) {
                    populateFields(parsed);
                    return;
                }
            }

            // Fallback: fetch strain by ID
            if (strainIdAttr) {
                fetch(`/strains/${strainIdAttr}`)
                    .then(r => r.json())
                    .then(data => populateFields(data))
                    .catch(err => console.error('Failed to load strain data:', err));
            }
        });
    }

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