document.addEventListener("DOMContentLoaded", () => {
    const editStatusModal = new bootstrap.Modal(document.getElementById("editStatusModal"));
    const statusForm = document.getElementById("editStatusForm");
    const deleteStatusButton = document.getElementById("deleteStatus");

    document.querySelectorAll(".status-row").forEach(row => {
        row.addEventListener("click", () => {
            const statusData = JSON.parse(row.getAttribute("data-status"));

            document.getElementById("statusId").value = statusData.id;
            const date = new Date(statusData.date);
            document.getElementById("editStatusDate").value = date.toISOString().slice(0, 19);

            // Disable delete if this is the only status for the plant
            const totalStatuses = document.querySelectorAll('.status-row').length;
            if (totalStatuses <= 1) {
                deleteStatusButton.disabled = true;
                deleteStatusButton.title = uiMessages.t('cannot_delete_last_status') || 'Cannot delete the last status entry for this plant';
            } else {
                deleteStatusButton.disabled = false;
                deleteStatusButton.title = '';
            }

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
            .catch(() => uiMessages.showToast(uiMessages.t('failed_to_update_status'), 'danger'));
    });

    deleteStatusButton.addEventListener("click", () => {
        if (deleteStatusButton.disabled) {
            uiMessages.showToast(uiMessages.t('cannot_delete_last_status') || 'Cannot delete the last status entry for this plant', 'warning');
            return;
        }

        const statusId = document.getElementById("statusId").value;

        uiMessages.showConfirm(uiMessages.t('confirm_delete_status')).then(confirmed => {
            if (!confirmed) return;
            fetch(`/plantStatus/delete/${statusId}`, { method: "DELETE" })
                .then(async response => {
                    const data = await response.json().catch(() => ({}));
                    if (!response.ok) {
                        const msg = data.error || uiMessages.t('failed_to_delete_status') || 'Failed to delete status';
                        uiMessages.showToast(msg, 'danger');
                        return;
                    }
                    location.reload();
                })
                .catch(() => uiMessages.showToast(uiMessages.t('failed_to_delete_status'), 'danger'));
        });
    });
});