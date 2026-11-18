document.addEventListener("DOMContentLoaded", () => {
    const form = document.getElementById("addZoneForm");
    const addZoneModal = document.getElementById("addZoneModal");

    form.addEventListener("submit", (e) => {
        e.preventDefault();

        // Gather form data
        const zoneId = document.getElementById("zoneId").value;
        const zoneName = document.getElementById("zoneName").value;

        // Construct the payload
        const payload = {
            zone_name: zoneName,
        };

        // Send POST request to /plantMeasurement
        fetch("/zones", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify(payload),
        })
            .then((response) => {
                if (!response.ok) {
                    throw new Error("Failed to add zone");
                }
                return response.json();
            })
            .then((data) => {
                // Success: Close modal and reload page
                const modal = bootstrap.Modal.getInstance(document.getElementById("addZoneModal"));
                modal.hide();
                reloadKeepingPlace(); // Refresh page to show updated data
            })
            .catch((error) => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t('failed_to_add_zone'), 'danger');
            });
    });
});

document.addEventListener("DOMContentLoaded", () => {
    const editZoneModal = new bootstrap.Modal(document.getElementById("editZoneModal"));
    const zoneForm = document.getElementById("editZoneForm");
    const deleteZoneButton = document.getElementById("deleteZone");

    // The leaf offset is stored in °F. It is a temperature difference, so
    // converting it is a pure 5/9 scale with no 32 shift.
    const offsetInput = document.getElementById("editZoneLeafOffset");
    const offsetUnit = document.getElementById("editZoneLeafOffsetUnit");
    const roundTenth = v => Math.round(v * 10) / 10;
    const offsetForDisplay = f => offsetUnit.value === 'C' ? roundTenth(f * 5 / 9) : f;

    // The unit last picked; else the one the zone's VPD temperature sensor
    // reports, or failing that any temperature sensor in the zone.
    function preferredOffsetUnit(zoneId, tempSensorId) {
        let saved = null;
        try { saved = localStorage.getItem('isley.leafOffsetUnit'); } catch (_) {}
        if (saved === 'C' || saved === 'F') return saved;
        const isC = u => /°C|celsius|^C$/i.test(u || '');
        const isF = u => /°F|fahrenheit|^F$/i.test(u || '');
        const all = window._vpdSensors || [];
        const s = all.find(x => x.id === tempSensorId) ||
                  all.find(x => x.zone_id === zoneId && (isC(x.unit) || isF(x.unit)));
        return s && isC(s.unit) ? 'C' : 'F';
    }

    // The exact stored °F value is kept until the user types, so switching
    // units back and forth never rounds a value nobody edited.
    offsetInput.addEventListener('input', () => { delete offsetInput.dataset.offsetF; });
    offsetUnit.addEventListener('change', () => {
        if ('offsetF' in offsetInput.dataset) {
            offsetInput.value = offsetForDisplay(parseFloat(offsetInput.dataset.offsetF));
        } else if (offsetInput.value !== '') {
            const v = parseFloat(offsetInput.value);
            offsetInput.value = roundTenth(offsetUnit.value === 'C' ? v * 5 / 9 : v * 9 / 5);
        }
        try { localStorage.setItem('isley.leafOffsetUnit', offsetUnit.value); } catch (_) {}
    });

    function offsetForSave() {
        if (offsetInput.value === '') return null;
        if ('offsetF' in offsetInput.dataset) return parseFloat(offsetInput.dataset.offsetF);
        const v = parseFloat(offsetInput.value);
        return offsetUnit.value === 'C' ? Math.round(v * 9 / 5 * 100) / 100 : v;
    }

    function populateVpdSensorSelect(selectEl, zoneId, selectedId) {
        const sensors = window._vpdSensors || [];
        selectEl.innerHTML = '<option value="">— none —</option>';
        sensors.forEach(function(s) {
            if (s.source === 'derived') return; // skip derived sensors as VPD inputs
            if (s.zone_id === zoneId) {
                const opt = document.createElement('option');
                opt.value = s.id;
                opt.textContent = s.name + ' (' + s.unit + ')';
                if (selectedId && s.id === selectedId) opt.selected = true;
                selectEl.appendChild(opt);
            }
        });
    }

    document.querySelectorAll(".zone-row").forEach(row => {
        row.addEventListener("click", () => {
            const zoneData = JSON.parse(row.getAttribute("data-zone"));

            document.getElementById("zoneId").value = zoneData.id;
            document.getElementById("editZoneName").value = zoneData.name;

            offsetUnit.value = preferredOffsetUnit(zoneData.id, zoneData.vpd_temp_sensor_id);
            if (zoneData.leaf_temp_offset != null) {
                offsetInput.dataset.offsetF = zoneData.leaf_temp_offset;
                offsetInput.value = offsetForDisplay(zoneData.leaf_temp_offset);
            } else {
                delete offsetInput.dataset.offsetF;
                offsetInput.value = '';
            }

            const tempSelect = document.getElementById("editZoneVpdTempSensor");
            const humSelect  = document.getElementById("editZoneVpdHumSensor");
            populateVpdSensorSelect(tempSelect, zoneData.id, zoneData.vpd_temp_sensor_id);
            populateVpdSensorSelect(humSelect,  zoneData.id, zoneData.vpd_humidity_sensor_id);

            editZoneModal.show();
        });
    });

    zoneForm.addEventListener("submit", (e) => {
        e.preventDefault();
        const zoneId = document.getElementById("zoneId").value;
        const tempSensor = document.getElementById("editZoneVpdTempSensor").value;
        const humSensor  = document.getElementById("editZoneVpdHumSensor").value;

        const payload = {
            zone_name:              document.getElementById("editZoneName").value,
            leaf_temp_offset:       offsetForSave(),
            vpd_temp_sensor_id:     tempSensor  !== '' ? parseInt(tempSensor,  10) : null,
            vpd_humidity_sensor_id: humSensor   !== '' ? parseInt(humSensor,   10) : null,
        };

        fetch(`/zones/${zoneId}`, {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then(response => response.json())
            .then(() => reloadKeepingPlace())
            .catch(err => uiMessages.showToast(uiMessages.t('failed_to_update_zone'), 'danger'));
    });

    deleteZoneButton.addEventListener("click", () => {
        const zoneId = document.getElementById("zoneId").value;

        uiMessages.confirmDelete(uiMessages.t('delete_zone_confirm'), uiMessages.t('delete_zone', 'Delete Zone')).then(confirmed => {
            if (!confirmed) return;
            fetch(`/zones/${zoneId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => reloadKeepingPlace())
                .catch(err => uiMessages.showToast(uiMessages.t('failed_to_delete_zone'), 'danger'));
        });
    });
});

document.addEventListener("DOMContentLoaded", () => {
    const form = document.getElementById("addActivityForm");
    const addActivityModal = document.getElementById("addActivityModal");

    function selectedMetricIds(selectId) {
        return Array.from(document.getElementById(selectId).selectedOptions)
            .map(opt => parseInt(opt.value, 10))
            .filter(id => Number.isInteger(id) && id > 0);
    }

    function buildActivityMetricsPayload(requiredSelectId, optionalSelectId) {
        const requiredIDs = selectedMetricIds(requiredSelectId);
        const requiredSet = new Set(requiredIDs);
        const optionalIDs = selectedMetricIds(optionalSelectId).filter(id => !requiredSet.has(id));
        const metrics = [];

        requiredIDs.forEach(metricID => metrics.push({ metric_id: metricID, required: true }));
        optionalIDs.forEach(metricID => metrics.push({ metric_id: metricID, required: false }));
        return metrics;
    }

    form.addEventListener("submit", (e) => {
        e.preventDefault();

        // Gather form data
        const activityId = document.getElementById("activityId").value;
        const activityName = document.getElementById("activityName").value;
        const isWatering = document.getElementById("activityIsWatering").checked;
        const isFeeding = document.getElementById("activityIsFeeding").checked;

        // Construct the payload
        const payload = {
            activity_name: activityName,
            is_watering: isWatering,
            is_feeding: isFeeding,
            activity_metrics: buildActivityMetricsPayload("activityRequiredMetrics", "activityOptionalMetrics"),
        };

        // Send POST request to /plantMeasurement
        fetch("/activities", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify(payload),
        })
            .then((response) => {
                if (!response.ok) {
                    throw new Error(uiMessages.t("failed_to_add_activity"));
                }
                return response.json();
            })
            .then((data) => {
                // Success: Close modal and reload page
                const modal = bootstrap.Modal.getInstance(document.getElementById("addActivityModal"));
                modal.hide();
                reloadKeepingPlace(); // Refresh page to show updated data
            })
            .catch((error) => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t('failed_to_add_activity'), 'danger');
            });
    });
});

document.addEventListener("DOMContentLoaded", () => {
    const editActivityModal = new bootstrap.Modal(document.getElementById("editActivityModal"));
    const activityForm = document.getElementById("editActivityForm");
    const deleteActivityButton = document.getElementById("deleteActivity");

    function setSelectedOptions(selectId, values) {
        const valueSet = new Set((values || []).map(v => String(v)));
        Array.from(document.getElementById(selectId).options).forEach(opt => {
            opt.selected = valueSet.has(opt.value);
        });
    }

    function selectedMetricIds(selectId) {
        return Array.from(document.getElementById(selectId).selectedOptions)
            .map(opt => parseInt(opt.value, 10))
            .filter(id => Number.isInteger(id) && id > 0);
    }

    function buildActivityMetricsPayload(requiredSelectId, optionalSelectId) {
        const requiredIDs = selectedMetricIds(requiredSelectId);
        const requiredSet = new Set(requiredIDs);
        const optionalIDs = selectedMetricIds(optionalSelectId).filter(id => !requiredSet.has(id));
        const metrics = [];

        requiredIDs.forEach(metricID => metrics.push({ metric_id: metricID, required: true }));
        optionalIDs.forEach(metricID => metrics.push({ metric_id: metricID, required: false }));
        return metrics;
    }

    document.querySelectorAll(".activity-row").forEach(row => {
        row.addEventListener("click", () => {
            const activityData = JSON.parse(row.getAttribute("data-activity"));
            const activityMetrics = Array.isArray(activityData.metrics) ? activityData.metrics : [];
            const requiredMetricIDs = activityMetrics.filter(m => !!m.required).map(m => m.metric_id);
            const optionalMetricIDs = activityMetrics.filter(m => !m.required).map(m => m.metric_id);

            document.getElementById("activityId").value = activityData.id;
            document.getElementById("editActivityName").value = activityData.name;
            document.getElementById("editActivityIsWatering").checked = !!activityData.is_watering;
            document.getElementById("editActivityIsFeeding").checked = !!activityData.is_feeding;
            setSelectedOptions("editActivityRequiredMetrics", requiredMetricIDs);
            setSelectedOptions("editActivityOptionalMetrics", optionalMetricIDs);

            editActivityModal.show();
        });
    });

    activityForm.addEventListener("submit", (e) => {
        e.preventDefault();
        const activityId = document.getElementById("activityId").value;
        const payload = {
            activity_name: document.getElementById("editActivityName").value,
            is_watering: document.getElementById("editActivityIsWatering").checked,
            is_feeding: document.getElementById("editActivityIsFeeding").checked,
            activity_metrics: buildActivityMetricsPayload("editActivityRequiredMetrics", "editActivityOptionalMetrics"),
        };

        fetch(`/activities/${activityId}`, {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then(response => response.json())
            .then(() => reloadKeepingPlace())
            .catch(err => uiMessages.showToast(uiMessages.t('failed_to_update_activity'), 'danger'));
    });

    deleteActivityButton.addEventListener("click", () => {
        const activityId = document.getElementById("activityId").value;

        uiMessages.confirmDelete(uiMessages.t('delete_activity_confirm'), uiMessages.t('delete_activity', 'Delete Activity')).then(confirmed => {
            if (!confirmed) return;
            fetch(`/activities/${activityId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => reloadKeepingPlace())
                .catch(err => uiMessages.showToast(uiMessages.t('failed_to_delete_activity'), 'danger'));
        });
    });
});

document.addEventListener("DOMContentLoaded", () => {
    const form = document.getElementById("addMetricForm");
    const addMetricModal = document.getElementById("addMetricModal");

    form.addEventListener("submit", (e) => {
        e.preventDefault();

        // Gather form data
        const metricId = document.getElementById("metricId").value;
        const metricName = document.getElementById("metricName").value;
        const metricUnit = document.getElementById("metricUnit").value;

        // Construct the payload
        const payload = {
            metric_name: metricName,
            metric_unit: metricUnit,
        };

        // Send POST request to /plantMeasurement
        fetch("/metrics", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify(payload),
        })
            .then((response) => {
                if (!response.ok) {
                    throw new Error(uiMessages.t("failed_to_add_metric"));
                }
                return response.json();
            })
            .then((data) => {
                // Success: Close modal and reload page
                const modal = bootstrap.Modal.getInstance(document.getElementById("addMetricModal"));
                modal.hide();
                reloadKeepingPlace(); // Refresh page to show updated data
            })
            .catch((error) => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t('failed_to_add_metric'), 'danger');
            });
    });
});

document.addEventListener("DOMContentLoaded", () => {
    const editMetricModal = new bootstrap.Modal(document.getElementById("editMetricModal"));
    const metricForm = document.getElementById("editMetricForm");
    const deleteMetricButton = document.getElementById("deleteMetric");

    document.querySelectorAll(".metric-row").forEach(row => {
        row.addEventListener("click", () => {
            const metricData = JSON.parse(row.getAttribute("data-metric"));

            document.getElementById("metricId").value = metricData.id;
            document.getElementById("editMetricName").value = metricData.name;
            document.getElementById("editMetricUnit").value = metricData.unit;

            editMetricModal.show();
        });
    });

    metricForm.addEventListener("submit", (e) => {
        e.preventDefault();
        const metricId = document.getElementById("metricId").value;
        const payload = {
            metric_name: document.getElementById("editMetricName").value,
            metric_unit: document.getElementById("editMetricUnit").value,
        };

        fetch(`/metrics/${metricId}`, {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then(response => response.json())
            .then(() => reloadKeepingPlace())
            .catch(err => uiMessages.showToast(uiMessages.t('failed_to_update_metric'), 'danger'));
    });

    deleteMetricButton.addEventListener("click", () => {
        const metricId = document.getElementById("metricId").value;

        uiMessages.confirmDelete(uiMessages.t('delete_metric_confirm'), uiMessages.t('delete_metric', 'Delete Metric')).then(confirmed => {
            if (!confirmed) return;
            fetch(`/metrics/${metricId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => reloadKeepingPlace())
                .catch(err => uiMessages.showToast(uiMessages.t('failed_to_delete_metric'), 'danger'));
        });
    });
});

document.addEventListener("DOMContentLoaded", () => {
    const form = document.getElementById("addBreederForm");
    const addBreederModal = document.getElementById("addBreederModal");

    form.addEventListener("submit", (e) => {
        e.preventDefault();

        // Gather form data
        const breederId = document.getElementById("breederId").value;
        const breederName = document.getElementById("breederName").value;

        // Construct the payload
        const payload = {
            breeder_name: breederName,
        };

        // Send POST request to /plantMeasurement
        fetch("/breeders", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify(payload),
        })
            .then(async (response) => {
                const data = await response.json().catch(() => ({}));
                if (!response.ok) throw new Error(data.error || '');
                return data;
            })
            .then((data) => {
                // Success: Close modal and reload page
                const modal = bootstrap.Modal.getInstance(document.getElementById("addBreederModal"));
                modal.hide();
                reloadKeepingPlace(); // Refresh page to show updated data
            })
            .catch((error) => {
                console.error("Error:", error);
                uiMessages.showToast(error.message || uiMessages.t('failed_to_add_breeder'), 'danger');
            });
    });
});

document.addEventListener("DOMContentLoaded", () => {
    const editBreederModal = new bootstrap.Modal(document.getElementById("editBreederModal"));
    const breederForm = document.getElementById("editBreederForm");
    const deleteBreederButton = document.getElementById("deleteBreeder");

    document.querySelectorAll(".breeder-row").forEach(row => {
        row.addEventListener("click", () => {
            const breederData = JSON.parse(row.getAttribute("data-breeder"));

            document.getElementById("breederId").value = breederData.id;
            document.getElementById("editBreederName").value = breederData.name;

            editBreederModal.show();
        });
    });

    breederForm.addEventListener("submit", (e) => {
        e.preventDefault();
        const breederId = document.getElementById("breederId").value;
        const payload = {
            breeder_name: document.getElementById("editBreederName").value,
        };

        fetch(`/breeders/${breederId}`, {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then(async response => {
                const data = await response.json().catch(() => ({}));
                if (!response.ok) throw new Error(data.error || '');
                reloadKeepingPlace();
            })
            .catch(err => uiMessages.showToast(err.message || uiMessages.t('failed_to_update_breeder'), 'danger'));
    });

    deleteBreederButton.addEventListener("click", () => {
        const breederId = document.getElementById("breederId").value;

        uiMessages.confirmDelete(uiMessages.t('delete_breeder_confirm'), uiMessages.t('delete_breeder', 'Delete Breeder')).then(confirmed => {
            if (!confirmed) return;
            fetch(`/breeders/${breederId}`, { method: "DELETE" })
                .then(async response => {
                    const data = await response.json().catch(() => ({}));
                    if (!response.ok) throw new Error(data.error || '');
                    return data;
                })
                .then(data => {
                    // Remove the row in place; no reload, so the active tab
                    // and scroll position are untouched.
                    const row = document.querySelector(`.breeder-row[data-breeder-id="${breederId}"]`);
                    if (row) row.remove();
                    // Re-run the filter so the "no match" row is right.
                    const searchInput = document.getElementById("searchBreeders");
                    if (searchInput) searchInput.dispatchEvent(new Event("input"));
                    editBreederModal.hide();
                    uiMessages.showToast(data.message || uiMessages.t('api_breeder_deleted', 'Breeder deleted'), 'success');
                })
                .catch(err => uiMessages.showToast(err.message || uiMessages.t('failed_to_delete_breeder', 'Failed to delete breeder. Please try again.'), 'danger'));
        });
    });
});

// Breeder list: filter by name as you type.
document.addEventListener("DOMContentLoaded", () => {
    const searchInput = document.getElementById("searchBreeders");
    const breederTable = document.getElementById("breederTable");
    const noMatchesRow = document.getElementById("noBreedersMatch");
    if (!searchInput || !breederTable) return;

    const applyBreederFilter = () => {
        const query = searchInput.value.trim().toLowerCase();
        const rows = breederTable.querySelectorAll(".breeder-row");
        let visible = 0;

        rows.forEach(row => {
            const matches = query === "" ||
                (row.textContent || "").toLowerCase().includes(query);
            row.classList.toggle("d-none", !matches);
            if (matches) visible++;
        });

        if (noMatchesRow) {
            noMatchesRow.classList.toggle("d-none", visible > 0 || rows.length === 0);
        }
    };

    searchInput.addEventListener("input", debounce(applyBreederFilter, 150));
});
