document.addEventListener("DOMContentLoaded", () => {
    const form = document.getElementById("addMultiPlantActivityForm");
    const addMultiPlantActivityModal = document.getElementById("addMultiPlantActivityModal");
    const activityMultiDateInput = document.getElementById("activityMultiDate");

    // Set default date to today
    //const setMultiDefaultDate = () => {
    //    const today = new Date().toISOString().split("T")[0];
    //    activityMultiDateInput.value = today;
    //};

    // Set default date when the modal is shown
    //addMultiPlantActivityModal.addEventListener("show.bs.modal", setMultiDefaultDate);

    form.addEventListener("submit", (e) => {
        e.preventDefault();

        // Gather selected plant IDs
        const selectedPlants = Array.from(document.getElementById("plantSelection").selectedOptions).map(opt => parseInt(opt.value));

        // Gather other form data
        const payload = {
            plant_ids: selectedPlants,
            activity_id: parseInt(document.getElementById("activityMultiName").value),
            note: document.getElementById("activityMultiNote").value,
            date: document.getElementById("activityMultiDate").value
        };

        // Send POST request to backend
        fetch("/record-multi-activity", {
            method: "POST",
            headers: {
                "Content-Type": "application/json"
            },
            body: JSON.stringify(payload)
        })
            .then(response => {
                if (!response.ok) throw new Error("Failed to record activity");
                return response.json();
            })
            .then(() => {
                const modal = bootstrap.Modal.getInstance(document.getElementById("addMultiPlantActivityModal"));
                modal.hide();
                location.reload(); // Reload page to reflect changes
            })
            .catch(error => {
                console.error("Error:", error);
                alert("Error recording activity. Please try again.");
            });
    });
});