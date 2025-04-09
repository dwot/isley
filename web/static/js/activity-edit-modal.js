document.addEventListener("DOMContentLoaded", () => {
    const editActivityModal = new bootstrap.Modal(document.getElementById("editActivityModal"));
    const activityForm = document.getElementById("editActivityForm");
    const deleteActivityButton = document.getElementById("deleteActivity");

    document.querySelectorAll(".activity-row").forEach(row => {
        row.addEventListener("click", () => {
            const activityData = JSON.parse(row.getAttribute("data-activity"));

            document.getElementById("activityId").value = activityData.id;
            const formattedDate = activityData.date.split("T")[0];
            document.getElementById("editActivityDate").value = formattedDate;
            document.getElementById("editActivityType").value = activityData.activity_id;
            document.getElementById("editActivityNote").value = activityData.note;

            editActivityModal.show();
        });
    });

    activityForm.addEventListener("submit", (e) => {
        e.preventDefault();

        const payload = {
            id: parseInt(document.getElementById("activityId").value, 10),
            date: document.getElementById("editActivityDate").value,
            activity_id: parseInt(document.getElementById("editActivityType").value, 10),
            note: document.getElementById("editActivityNote").value,
        };

        fetch("/plantActivity/edit", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then(response => response.json())
            .then(() => location.reload())
            .catch(err => alert("{{ .lcl.failed_to_update_activity }}"));
    });

    deleteActivityButton.addEventListener("click", () => {
        const activityId = document.getElementById("activityId").value;

        if (confirm("{{ .lcl.confirm_delete_activity }}")) {
            fetch(`/plantActivity/delete/${activityId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => location.reload())
                .catch(err => alert("{{ .lcl.failed_to_delete_activity }}"));
        }
    });

    deletePlantButton.addEventListener("click", () => {
        const plantId = document.getElementById("plantId").value;

        if (confirm("Are you sure you want to delete this plant?")) {
            fetch(`/plant/delete/${plantId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => location.href = "/plants")
                .catch(err => alert("Failed to delete plant"));
        }
    })
});