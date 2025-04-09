document.addEventListener("DOMContentLoaded", () => {
    const addStrainForm = document.getElementById("addStrainForm");
    const breederSelect = document.getElementById("breederSelect");
    const newBreederInput = document.getElementById("newBreederInput");
    const indicaSativaSlider = document.getElementById("indicaSativaSlider");
    const indicaLabel = document.getElementById("indicaLabel");
    const sativaLabel = document.getElementById("sativaLabel");
    const cycleTime = document.getElementById("cycleTime");
    const url = document.getElementById("url");

    // Show/Hide New Breeder Input
    breederSelect.addEventListener("change", () => {
        if (breederSelect.value === "new") {
            newBreederInput.classList.remove("d-none");
        } else {
            newBreederInput.classList.add("d-none");
        }
    });

    // Update labels dynamically as the slider changes
    indicaSativaSlider.addEventListener("input", (e) => {
        const indica = e.target.value;
        const sativa = 100 - indica;
        indicaLabel.textContent = `Indica: ${indica}%`;
        sativaLabel.textContent = `Sativa: ${sativa}%`;
    });

    // If no breeders exist, show the new breeder input by default
    if (document.getElementById("breederSelect").length === 1) {
        newBreederInput.classList.remove("d-none");
    }

    addStrainForm.addEventListener("submit", (e) => {
        e.preventDefault();

        // Gather form data
        const payload = {
            name: document.getElementById("strainName").value,
            breeder_id: breederSelect.value === "new" ? null : parseInt(breederSelect.value, 10),
            new_breeder: breederSelect.value === "new" ? document.getElementById("newBreederName").value : null,
            indica: parseInt(indicaSativaSlider.value, 10),
            sativa: 100 - parseInt(indicaSativaSlider.value, 10),
            autoflower: document.getElementById("autoflower").value === "true",
            seed_count: parseInt(document.getElementById("seedCount").value, 10),
            description: document.getElementById("strainDescription").value,
            short_desc: document.getElementById("strainShortDescription").value,
            cycle_time: parseInt(cycleTime.value, 10),
            url: url.value
        };

        // Send POST request to add the strain
        fetch("/strains", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify(payload),
        })
            .then((response) => {
                if (!response.ok) {
                    throw new Error("{{ .lcl.strain_add_fail }}");
                }
                return response.json();
            })
            .then((data) => {
                location.reload(); // Reload the page to show the new strain
            })
            .catch((error) => {
                console.error("Error:", error);
                alert("{{ .lcl.strain_add_error }}");
            });
    });
});