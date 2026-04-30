document.addEventListener("DOMContentLoaded", () => {
    const addStrainForm = document.getElementById("addStrainForm");
    const breederSelect = document.getElementById("breederSelect");
    const newBreederInput = document.getElementById("newBreederInput");
    const indicaInput = document.getElementById("indicaInput");
    const sativaInput = document.getElementById("sativaInput");
    const ruderalisInput = document.getElementById("ruderalisInput");
    const geneticsLower = document.getElementById("geneticsLower");
    const geneticsUpper = document.getElementById("geneticsUpper");
    const indicaLabel = document.getElementById("indicaLabel");
    const sativaLabel = document.getElementById("sativaLabel");
    const ruderalisLabel = document.getElementById("ruderalisLabel");
    const ratioFillIndica = document.getElementById("ratioFillIndica");
    const ratioFillSativa = document.getElementById("ratioFillSativa");
    const ratioFillRuderalis = document.getElementById("ratioFillRuderalis");
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

    function updateRatio(activeThumb) {
        if (!geneticsLower || !geneticsUpper) return;

        let lower = parseInt(geneticsLower.value, 10);
        let upper = parseInt(geneticsUpper.value, 10);
        lower = Number.isFinite(lower) ? Math.min(100, Math.max(0, lower)) : 0;
        upper = Number.isFinite(upper) ? Math.min(100, Math.max(0, upper)) : 100;

        if (lower > upper) {
            if (activeThumb === "lower") {
                upper = lower;
            } else {
                lower = upper;
            }
        }

        const indica = lower;
        const sativa = upper - lower;
        const ruderalis = 100 - upper;

        geneticsLower.value = String(lower);
        geneticsUpper.value = String(upper);
        if (indicaInput) indicaInput.value = String(indica);
        if (sativaInput) sativaInput.value = String(sativa);
        if (ruderalisInput) ruderalisInput.value = String(ruderalis);
        if (indicaLabel) indicaLabel.textContent = `Indica: ${indica}%`;
        if (sativaLabel) sativaLabel.textContent = `Sativa: ${sativa}%`;
        if (ruderalisLabel) ruderalisLabel.textContent = `Ruderalis: ${ruderalis}%`;
        if (ratioFillIndica) ratioFillIndica.style.width = `${indica}%`;
        if (ratioFillSativa) ratioFillSativa.style.width = `${sativa}%`;
        if (ratioFillRuderalis) ratioFillRuderalis.style.width = `${ruderalis}%`;

        const ratioError = document.getElementById("ratioError");
        if (indica + sativa + ruderalis !== 100) {
            if (ratioError) ratioError.classList.remove("d-none");
            return false;
        }
        if (ratioError) ratioError.classList.add("d-none");
        return true;
    }
    if (geneticsLower) geneticsLower.addEventListener("input", () => updateRatio("lower"));
    if (geneticsUpper) geneticsUpper.addEventListener("input", () => updateRatio("upper"));
    if (geneticsLower) geneticsLower.value = "50";
    if (geneticsUpper) geneticsUpper.value = "100";
    updateRatio("upper");

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
            indica: parseInt(indicaInput.value, 10) || 0,
            sativa: parseInt(sativaInput.value, 10) || 0,
            ruderalis: parseInt(ruderalisInput.value, 10) || 0,
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
            .then(() => {
                // Reset the form fields to their initial defaults so the modal isn't prepopulated next time
                try {
                    addStrainForm.reset();

                    // Reset ratio inputs
                    if (geneticsLower) geneticsLower.value = "50";
                    if (geneticsUpper) geneticsUpper.value = "100";
                    updateRatio("upper");

                    // Reset breeder select and new breeder input
                    if (breederSelect) {
                        breederSelect.selectedIndex = 0;
                    }
                    if (newBreederInput) {
                        newBreederInput.classList.add("d-none");
                        const newBreederName = document.getElementById("newBreederName");
                        if (newBreederName) newBreederName.value = "";
                    }

                    // Reset cycle time to default (56) and clear url
                    if (cycleTime) cycleTime.value = 56;
                    if (url) url.value = "";

                    // Reset autoflower to default false
                    const autoflower = document.getElementById("autoflower");
                    if (autoflower) autoflower.value = "false";

                    // Hide the modal (Bootstrap)
                    const addStrainModalEl = document.getElementById("addStrainModal");
                    if (addStrainModalEl) {
                        const addStrainModal = bootstrap.Modal.getOrCreateInstance(addStrainModalEl);
                        addStrainModal.hide();
                    }
                } catch (resetErr) {
                    // If anything goes wrong resetting, log to console but continue
                    console.warn("Failed to reset add strain form:", resetErr);
                }

                // Reload the page to show the newly added strain (keeps behavior consistent)
                location.reload();
            })
            .catch((error) => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t('strain_add_error') || '{{ .lcl.strain_add_error }}', 'danger');
            });
    });
});