document.addEventListener("DOMContentLoaded", () => {
    const tableBody = document.getElementById("sensorsTableBody");
    const emptyState = document.getElementById("emptyState");
    const emptyMessage = document.getElementById("emptyMessage");
    const searchInput = document.getElementById("searchSensors");
    const filterZone = document.getElementById("filterZone");
    const filterSource = document.getElementById("filterSource");
    const filterVisibility = document.getElementById("filterVisibility");
    const sortBy = document.getElementById("sortBy");
    const clearBtn = document.getElementById("clearFilters");
    const resultCount = document.getElementById("resultCount");

    // ----- Table header sort click -----
    const sortableTable = new IsleySortableTable(sortBy, {
        prefix: "sn",
        headerToDropdown: {
            name:       { asc: "name-asc",       desc: "name-desc" },
            zone:       { asc: "zone-asc",       desc: "zone-desc" },
            source:     { asc: "source-asc",     desc: "source-desc" },
            device:     { asc: "device-asc",     desc: "device-desc" },
            type:       { asc: "type-asc",       desc: "type-desc" },
            unit:       { asc: "unit-asc",       desc: "unit-desc" },
            visibility: { asc: "visibility-asc", desc: "visibility-desc" },
        },
        dropdownToHeader: { name: "name", zone: "zone", source: "source", device: "device",
                            type: "type", unit: "unit", visibility: "visibility" },
        onChange: applyFilters,
    });

    let allSensors = [];

    // Visibility labels, looked up when used
    const visLabels = {
        get zone_plant() { return uiMessages.t("vis_zone_plant"); },
        get zone() { return uiMessages.t("vis_zone"); },
        get plant() { return uiMessages.t("vis_plant"); },
        get hide() { return uiMessages.t("title_hide"); },
    };

    // ----- Load sensor data from embedded JSON -----
    function loadSensors() {
        try {
            const raw = document.getElementById("sensorData").textContent.trim();
            allSensors = (raw && raw !== "null") ? (JSON.parse(raw) || []) : [];
        } catch (e) {
            allSensors = [];
        }
        populateFilters();
        applyFilters();
    }

    // ----- Populate filter dropdowns from data -----
    function populateFilters() {
        populateFilter(filterZone, allSensors, "zone");
        populateFilter(filterSource, allSensors, "source");
    }

    // ----- Filter + Sort -----
    function applyFilters() {
        let data = [...allSensors];
        const query = searchInput.value.toLowerCase().trim();
        const zone = filterZone.value;
        const source = filterSource.value;
        const visibility = filterVisibility.value;

        data = list.filterByText(data, query, ["name", "zone", "source", "device", "type", "unit"]);
        if (zone) {
            data = data.filter(s => s.zone === zone);
        }
        if (source) {
            data = data.filter(s => s.source === source);
        }
        if (visibility) {
            data = data.filter(s => s.visibility === visibility);
        }

        // Use table header sort if active, otherwise use dropdown
        const { key, asc } = sortableTable.getSort();

        data.sort((a, b) => {
            let va, vb;
            switch (key) {
                case "name":
                    va = (a.name || "").toLowerCase();
                    vb = (b.name || "").toLowerCase();
                    return asc ? va.localeCompare(vb) : vb.localeCompare(va);
                case "zone":
                    va = (a.zone || "").toLowerCase();
                    vb = (b.zone || "").toLowerCase();
                    return asc ? va.localeCompare(vb) : vb.localeCompare(va);
                case "source":
                    va = (a.source || "").toLowerCase();
                    vb = (b.source || "").toLowerCase();
                    return asc ? va.localeCompare(vb) : vb.localeCompare(va);
                case "device":
                    va = (a.device || "").toLowerCase();
                    vb = (b.device || "").toLowerCase();
                    return asc ? va.localeCompare(vb) : vb.localeCompare(va);
                case "type":
                    va = (a.type || "").toLowerCase();
                    vb = (b.type || "").toLowerCase();
                    return asc ? va.localeCompare(vb) : vb.localeCompare(va);
                case "unit":
                    va = (a.unit || "").toLowerCase();
                    vb = (b.unit || "").toLowerCase();
                    return asc ? va.localeCompare(vb) : vb.localeCompare(va);
                case "visibility":
                    va = (a.visibility || "").toLowerCase();
                    vb = (b.visibility || "").toLowerCase();
                    return asc ? va.localeCompare(vb) : vb.localeCompare(va);
                case "updated":
                    va = a.update_dt || "";
                    vb = b.update_dt || "";
                    return asc ? va.localeCompare(vb) : vb.localeCompare(va);
                default:
                    return 0;
            }
        });

        resultCount.textContent = pluralize(data.length, "sensor");

        if (data.length === 0) {
            tableBody.innerHTML = "";
            document.getElementById("sensorsTable").style.display = "none";
            emptyState.style.display = "flex";
            emptyMessage.textContent = allSensors.length === 0
                ? uiMessages.t("no_sensors_yet")
                : uiMessages.t("no_sensors_match");
            return;
        }
        emptyState.style.display = "none";
        document.getElementById("sensorsTable").style.display = "";
        renderTable(data);
    }

    // ----- Render table -----
    function renderTable(data) {
        tableBody.innerHTML = data.map(s => {
            const visLabel = visLabels[s.visibility] || s.visibility;
            const visBadgeClass = s.visibility === "hide" ? "bg-secondary" :
                                  s.visibility === "zone_plant" ? "bg-success" :
                                  s.visibility === "zone" ? "bg-info" : "bg-warning";

            return `<tr class="sn-row" data-sensor='${esc(JSON.stringify(s))}'>
                <td>
                    <span class="sn-sensor-name">${esc(s.name)}</span>
                </td>
                <td class="sn-cell-zone">${esc(s.zone || "")}</td>
                <td class="sn-cell-source">${esc(s.source || "")}</td>
                <td class="sn-cell-device">${esc(s.device || "")}</td>
                <td class="sn-cell-type">${esc(s.type || "")}</td>
                <td class="sn-cell-unit">${esc(s.unit || "")}</td>
                <td><span class="badge ${visBadgeClass}">${esc(visLabel)}</span></td>
            </tr>`;
        }).join("");

        // Attach row click handlers for edit modal
        tableBody.querySelectorAll(".sn-row").forEach(row => {
            row.addEventListener("click", () => {
                const sensorData = JSON.parse(row.getAttribute("data-sensor"));
                openEditModal(sensorData);
            });
        });
    }

    // ----- Event listeners -----
    searchInput.addEventListener("input", debounce(applyFilters, 200));
    filterZone.addEventListener("change", applyFilters);
    filterSource.addEventListener("change", applyFilters);
    filterVisibility.addEventListener("change", applyFilters);

    clearBtn.addEventListener("click", () => {
        clearFilters({
            searchInput,
            selects: [filterZone, filterSource, filterVisibility],
            sortableTable,
            defaultSort: "name-asc"
        });
        applyFilters();
    });

    // ----- Edit Sensor Modal -----
    const editSensorModal = new bootstrap.Modal(document.getElementById("editSensorModal"));
    const sensorForm = document.getElementById("editSensorForm");
    const deleteButton = document.getElementById("deleteSensor");
    let currentSensor = null;

    function openEditModal(sensorData) {
        currentSensor = sensorData;
        document.getElementById("sensorId").value = sensorData.id;
        document.getElementById("sensorName").value = sensorData.name;
        document.getElementById("sensorIdentity").textContent = sensorData.source + " / " + sensorData.device + " / " + sensorData.type;
        document.getElementById("sensorVisibility").value = sensorData.visibility || "zone_plant";
        document.getElementById("sensorUnit").value = sensorData.unit;
        document.getElementById("sensorZone").value = sensorData.zone_id;
        editSensorModal.show();
    }

    // Save changes
    sensorForm.addEventListener("submit", (e) => {
        e.preventDefault();
        const payload = {
            id: parseInt(document.getElementById("sensorId").value, 10),
            name: document.getElementById("sensorName").value,
            visibility: document.getElementById("sensorVisibility").value,
            zone_id: parseInt(document.getElementById("sensorZone").value, 10),
            unit: document.getElementById("sensorUnit").value,
        };

        fetch("/sensors/edit", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then(response => {
                if (!response.ok) throw new Error(uiMessages.t("failed_save_changes"));
                return response.json();
            })
            .then(data => {
                window.location.reload();
            })
            .catch(error => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t("failed_update_sensor"), 'danger');
            });
    });

    // Delete sensor
    deleteButton.addEventListener("click", () => {
        uiMessages.confirmDelete(uiMessages.t("confirm_delete_sensor"), uiMessages.t("delete_sensor")).then(confirmed => {
            if (!confirmed) return;
            const spinner = document.getElementById('deleteSpinner');
            const saveBtn = sensorForm.querySelector('button[type="submit"]');
            deleteButton.disabled = true;
            if (saveBtn) saveBtn.disabled = true;
            if (spinner) spinner.classList.remove('d-none');

            fetch(`/sensors/delete/${currentSensor.id}`, { method: "DELETE" })
                .then(response => {
                    if (!response.ok) throw new Error(uiMessages.t("failed_delete_sensor"));
                    window.location.reload();
                })
                .catch(error => {
                    console.error("Error:", error);
                    if (spinner) spinner.classList.add('d-none');
                    deleteButton.disabled = false;
                    if (saveBtn) saveBtn.disabled = false;
                    uiMessages.showToast(uiMessages.t("failed_delete_sensor"), 'danger');
                });
        });
    });

    // ----- Scan Modal Logic -----
    let currentScanEndpoint = "";

    document.getElementById("scanACI")?.addEventListener("click", () => {
        currentScanEndpoint = "/sensors/scanACI";
        showZoneModal();
    });

    document.getElementById("scanEC")?.addEventListener("click", () => {
        currentScanEndpoint = "/sensors/scanEC";
        showEcoWittZoneModal();
    });

    // ACI Modal: Handle Zone Dropdown Change
    document.getElementById("zoneSelect")?.addEventListener("change", (e) => {
        const newZoneInput = document.getElementById("newZoneInput");
        if (e.target.value === "new") {
            newZoneInput.classList.remove("d-none");
        } else {
            newZoneInput.classList.add("d-none");
        }
    });

    // EcoWitt Modal: Handle Zone Dropdown Change
    document.getElementById("ecZoneSelect")?.addEventListener("change", (e) => {
        const ecNewZoneInput = document.getElementById("ecNewZoneInput");
        if (e.target.value === "new") {
            ecNewZoneInput.classList.remove("d-none");
        } else {
            ecNewZoneInput.classList.add("d-none");
        }
    });

    const showZoneModal = () => {
        const zoneSelect = document.getElementById("zoneSelect");
        const newZoneInput = document.getElementById("newZoneInput");
        const noZoneHelp = document.getElementById("noZoneHelp");
        zoneSelect.value = "";
        newZoneInput.classList.add("d-none");
        noZoneHelp.style.display = "none";
        if (zoneSelect.options.length <= 1) {
            zoneSelect.value = "new";
            newZoneInput.classList.remove("d-none");
            noZoneHelp.style.display = "block";
        }
        const modal = new bootstrap.Modal(document.getElementById("zoneModal"));
        modal.show();
    };

    const showEcoWittZoneModal = () => {
        const ecZoneSelect = document.getElementById("ecZoneSelect");
        const ecNewZoneInput = document.getElementById("ecNewZoneInput");
        const ecNewZoneName = document.getElementById("ecNewZoneName");
        ecZoneSelect.value = "";
        ecNewZoneInput.classList.add("d-none");
        ecNewZoneName.value = "";
        if (ecZoneSelect.options.length <= 1) {
            ecZoneSelect.value = "new";
            ecNewZoneInput.classList.remove("d-none");
        }
        const modal = new bootstrap.Modal(document.getElementById("ecScanModal"));
        modal.show();
    };

    // Handle Zone Form Submission
    document.getElementById("zoneForm").addEventListener("submit", (e) => {
        e.preventDefault();
        const zoneSelect = document.getElementById("zoneSelect");
        const newZoneName = document.getElementById("newZoneName").value;
        const payload = {
            zone_id: zoneSelect.value === "new" ? null : parseInt(zoneSelect.value, 10),
            new_zone: zoneSelect.value === "new" ? newZoneName : null,
        };
        if (!currentScanEndpoint) {
            uiMessages.showToast('No scan endpoint defined.', 'warning');
            return;
        }
        fetch(currentScanEndpoint, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then((response) => {
                if (!response.ok) throw new Error(uiMessages.t("failed_sensor_scan"));
                return response.json();
            })
            .then(() => { window.location.reload(); })
            .catch((error) => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t("failed_sensor_scan"), 'danger');
            });
    });

    // Handle EcoWitt-specific Zone Form Submission
    document.getElementById("ecScanForm")?.addEventListener("submit", (e) => {
        e.preventDefault();
        const ecZoneSelect = document.getElementById("ecZoneSelect");
        const ecNewZoneName = document.getElementById("ecNewZoneName").value;
        const serverAddress = document.getElementById("serverAddress").value;
        const payload = {
            zone_id: ecZoneSelect.value === "new" ? null : parseInt(ecZoneSelect.value, 10),
            new_zone: ecZoneSelect.value === "new" ? ecNewZoneName : null,
            server_address: serverAddress,
        };
        if (!currentScanEndpoint) {
            uiMessages.showToast(uiMessages.t("no_scan_endpoint"), 'warning');
            return;
        }
        fetch(currentScanEndpoint, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then((response) => {
                if (!response.ok) throw new Error(uiMessages.t("failed_sensor_scan"));
                return response.json();
            })
            .then(() => { window.location.reload(); })
            .catch((error) => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t("failed_sensor_scan"), 'danger');
            });
    });

    // ----- ACI Dump Logic -----
    const fetchDumpACI = async (showModal = false) => {
        const statusEl = document.getElementById('dumpACIStatus');
        const contentEl = document.getElementById('dumpACIContent');
        const redactCheckbox = document.getElementById('dumpACIRedact');
        const redact = redactCheckbox && redactCheckbox.checked ? 'true' : 'false';
        statusEl.innerHTML = 'Fetching...';
        contentEl.textContent = '';
        try {
            const resp = await fetch('/sensors/dumpACI?redact=' + redact, { method: 'GET' });
            if (!resp.ok) throw new Error('Failed to fetch AC Infinity JSON');
            const text = await resp.text();
            contentEl.textContent = text;
            statusEl.innerHTML = '';
            if (showModal) {
                const modal = new bootstrap.Modal(document.getElementById('dumpACIModal'));
                modal.show();
            }
        } catch (err) {
            statusEl.innerHTML = '<div class="text-danger">Error fetching AC Infinity JSON</div>';
            console.error(err);
        }
    };

    document.getElementById("dumpACI")?.addEventListener("click", async () => {
        await fetchDumpACI(true);
    });

    document.getElementById("dumpACIRefresh")?.addEventListener("click", async () => {
        await fetchDumpACI(false);
    });

    document.getElementById('dumpACIRedact')?.addEventListener('change', async (e) => {
        const modalEl = document.getElementById('dumpACIModal');
        if (modalEl.classList.contains('show')) {
            await fetchDumpACI(false);
        }
    });

    document.getElementById('dumpACICopy')?.addEventListener('click', async () => {
        const contentEl = document.getElementById('dumpACIContent');
        const statusEl = document.getElementById('dumpACIStatus');
        const text = contentEl.textContent || '';
        if (!text) {
            statusEl.innerHTML = '<div class="text-muted">Nothing to copy</div>';
            setTimeout(() => statusEl.innerHTML = '', 1500);
            return;
        }
        try {
            if (navigator.clipboard && navigator.clipboard.writeText) {
                await navigator.clipboard.writeText(text);
            } else {
                const textarea = document.createElement('textarea');
                textarea.value = text;
                document.body.appendChild(textarea);
                textarea.select();
                document.execCommand('copy');
                textarea.remove();
            }
            statusEl.innerHTML = '<div class="text-success">Copied to clipboard</div>';
            setTimeout(() => statusEl.innerHTML = '', 1500);
        } catch (err) {
            console.error('Copy failed', err);
            statusEl.innerHTML = '<div class="text-danger">Failed to copy</div>';
            setTimeout(() => statusEl.innerHTML = '', 2000);
        }
    });

    // ----- Initial load -----
    // The visibility labels are translated, so wait for those.
    uiMessages.ready.then(loadSensors);
});
