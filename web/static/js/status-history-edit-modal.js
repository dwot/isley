
document.addEventListener("DOMContentLoaded", () => {
    const editStatusModal = new bootstrap.Modal(document.getElementById("editStatusModal"));
    const statusForm = document.getElementById("editStatusForm");
    const deleteStatusButton = document.getElementById("deleteStatus");

    document.querySelectorAll(".status-row").forEach(row => {
        row.addEventListener("click", () => {
            const statusData = JSON.parse(row.getAttribute("data-status"));

            document.getElementById("statusId").value = statusData.id;
            console.log(statusData.date);

            const date = new Date(statusData.date);
            const formattedDate = date.toISOString().slice(0, 16); // Removes seconds and 'Z'
            document.getElementById("editStatusDate").value = formattedDate;


            editStatusModal.show();
        });
    });

    statusForm.addEventListener("submit", (e) => {
        e.preventDefault();

        const payload = {
            id: parseInt(document.getElementById("statusId").value, 10),
            date: document.getElementById("editStatusDate").value,
        };

        fetch("/plantStatus/edit", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then(response => response.json())
            .then(() => location.reload())
            .catch(err => alert("{{ .lcl.failed_to_update_status }}"));
    });

    deleteStatusButton.addEventListener("click", () => {
        const statusId = document.getElementById("statusId").value;

        if (confirm("{{ .lcl.confirm_delete_status }}")) {
            fetch(`/plantStatus/delete/${statusId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => location.reload())
                .catch(err => alert("{{ .lcl.failed_to_delete_status }}"));
        }
    });
});