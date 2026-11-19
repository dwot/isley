document.addEventListener("DOMContentLoaded", () => {
    const addStreamForm = document.getElementById("addStreamForm");
    const addStreamModal = document.getElementById("addStreamModal");

    addStreamForm.addEventListener("submit", (e) => {
        e.preventDefault();

        // Gather form data
        const streamName = document.getElementById("streamName").value;
        const streamZone = document.getElementById("streamZone").value;
        const streamURL = document.getElementById("streamURL").value;
        const streamVisibility = document.getElementById("streamVisibility").value;

        // Construct the payload
        const payload = {
            stream_name: streamName,
            zone_id: parseInt(streamZone, 10),
            url: streamURL,
            visible: streamVisibility === "true",
        };

        // Send POST request to /streams
        fetch("/streams", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify(payload),
        })
            .then((response) => {
                if (!response.ok) {
                    throw new Error("Failed to add stream");
                }
                return response.json();
            })
            .then((data) => {
                // Success: Close modal and reload page
                const modal = bootstrap.Modal.getInstance(document.getElementById("addStreamModal"));
                modal.hide();
                reloadKeepingPlace(); // Refresh page to show updated data
            })
            .catch((error) => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t('failed_to_add_stream') || 'Failed to add stream', 'danger');
            });

    })
})

document.addEventListener("DOMContentLoaded", ()=> {
    const editStreamModal = new bootstrap.Modal(document.getElementById("editStreamModal"));
    const streamForm = document.getElementById("editStreamForm");
    const deleteStreamButton = document.getElementById("deleteStream");

    document.querySelectorAll(".stream-row").forEach(row => {
        row.addEventListener("click", () => {
            const streamData = JSON.parse(row.getAttribute("data-stream"));

            document.getElementById("streamId").value = streamData.id;
            document.getElementById("editStreamName").value = streamData.name;
            document.getElementById("editStreamZone").value = streamData.zone_id;
            document.getElementById("editStreamURL").value = streamData.url;
            document.getElementById("editStreamVisibility").value = streamData.visible;

            editStreamModal.show();
        });
    });

    streamForm.addEventListener("submit", (e) => {
        e.preventDefault();
        const streamId = document.getElementById("streamId").value;
        const payload = {
            stream_name: document.getElementById("editStreamName").value,
            zone_id: parseInt(document.getElementById("editStreamZone").value, 10),
            url: document.getElementById("editStreamURL").value,
            visible: document.getElementById("editStreamVisibility").value === "true",
        };

        fetch(`/streams/${streamId}`, {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then(async response => {
                const data = await response.json().catch(() => ({}));
                if (!response.ok) throw new Error(data.error || '');
                reloadKeepingPlace();
            })
            .catch(err => uiMessages.showToast(err.message || uiMessages.t('failed_to_update_stream'), 'danger'));
    });

    deleteStreamButton.addEventListener("click", () => {
        const streamId = document.getElementById("streamId").value;

        uiMessages.confirmDelete(uiMessages.t('delete_stream_confirm'), uiMessages.t('delete_stream', 'Delete Stream')).then(confirmed => {
            if (!confirmed) return;
            fetch(`/streams/${streamId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => reloadKeepingPlace())
                .catch(err => uiMessages.showToast(uiMessages.t('failed_to_delete_stream'), 'danger'));
        });
    });
});
