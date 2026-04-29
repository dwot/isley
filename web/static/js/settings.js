document.addEventListener("DOMContentLoaded", () => {
    const pollingSlider = document.getElementById("pollingInterval");
    const pollingValue = document.getElementById("pollingValue");

    // Attach API key button listeners (replaces inline onclick)
    const genBtn = document.getElementById("btnGenerateAPIKey");
    if (genBtn) genBtn.addEventListener("click", generateNewAPIKey);
    const copyBtn = document.getElementById("btnCopyAPIKey");
    if (copyBtn) copyBtn.addEventListener("click", copyAPIKey);

    // Update display when slider is adjusted
    pollingSlider.addEventListener("input", () => {
        pollingValue.textContent = `${pollingSlider.value} seconds`;
    });

    const streamGrabSlider = document.getElementById("streamGrabInterval");
    const streamGrabValue = document.getElementById("streamGrabIntervalValue");

    // Update display when slider is adjusted
    streamGrabSlider.addEventListener("input", () => {
        const minutes = streamGrabSlider.value;
        const seconds = minutes * 60;
        streamGrabValue.textContent = `${minutes} {{ .lcl.time_minutes }}`;
        streamGrabSlider.setAttribute('data-seconds', seconds);
    });

    // Set initial value in seconds
    streamGrabSlider.setAttribute('data-seconds', streamGrabSlider.value * 60);
});

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
            zone_id: streamZone,
            url: streamURL,
            visible: streamVisibility,
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
                window.location.reload(); // Refresh page to show updated data
            })
            .catch((error) => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t('failed_to_add_stream') || 'Failed to add stream', 'danger');
            });

    })
})

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
                window.location.reload(); // Refresh page to show updated data
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

    document.querySelectorAll(".zone-row").forEach(row => {
        row.addEventListener("click", () => {
            const zoneData = JSON.parse(row.getAttribute("data-zone"));

            document.getElementById("zoneId").value = zoneData.id;
            document.getElementById("editZoneName").value = zoneData.name;

            editZoneModal.show();
        });
    });

    zoneForm.addEventListener("submit", (e) => {
        e.preventDefault();
        const zoneId = document.getElementById("zoneId").value;
        const payload = {
            zone_name: document.getElementById("editZoneName").value,
        };

        fetch(`/zones/${zoneId}`, {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then(response => response.json())
            .then(() => location.reload())
            .catch(err => uiMessages.showToast(uiMessages.t('failed_to_update_zone'), 'danger'));
    });

    deleteZoneButton.addEventListener("click", () => {
        const zoneId = document.getElementById("zoneId").value;

        uiMessages.showConfirm(uiMessages.t('delete_zone_confirm')).then(confirmed => {
            if (!confirmed) return;
            fetch(`/zones/${zoneId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => location.reload())
                .catch(err => uiMessages.showToast(uiMessages.t('failed_to_delete_zone'), 'danger'));
        });
    });
});

document.addEventListener("DOMContentLoaded", () => {
    const aciLoginForm = document.getElementById("aciLoginForm");

    aciLoginForm.addEventListener("submit", (e) => {
        e.preventDefault();

        const email = document.getElementById("aciEmail").value;
        const password = document.getElementById("aciPassword").value;

        fetch("/aci/login", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify({ email, password }),
        })
            .then((response) => response.json())
            .then((data) => {
                if (data.success) {
                    const modal = bootstrap.Modal.getInstance(document.getElementById("aciLoginModal"));
                    modal.hide();
                    document.getElementById("tokenSetBadge").textContent = "{{ .lcl.token_set }}";
                    document.getElementById("tokenSetBadge").classList.remove("bg-danger");
                    document.getElementById("tokenSetBadge").classList.add("bg-success");
                } else {
                    uiMessages.showToast(uiMessages.t('failed_to_fetch_token') + ': ' + data.message, 'danger');
                }
            })
            .catch((error) => {
                console.error("{{ .lcl.error_fetching_token }}", error);
                uiMessages.showToast(uiMessages.t('generic_error'), 'danger');
            });
    });
});

document.addEventListener("DOMContentLoaded", () => {
    // Populate timezone selector with IANA timezones
    (function() {
        const sel = document.getElementById('timezone');
        if (!sel) return;
        const saved = sel.getAttribute('data-saved') || '';
        try {
            const zones = Intl.supportedValuesOf('timeZone');
            zones.forEach(function(tz) {
                const opt = document.createElement('option');
                opt.value = tz;
                opt.textContent = tz.replace(/_/g, ' ');
                if (tz === saved) opt.selected = true;
                sel.appendChild(opt);
            });
        } catch(e) {
            var browserTZ = Intl.DateTimeFormat().resolvedOptions().timeZone;
            var opt = document.createElement('option');
            opt.value = browserTZ;
            opt.textContent = browserTZ.replace(/_/g, ' ');
            if (browserTZ === saved) opt.selected = true;
            sel.appendChild(opt);
        }
        if (saved && sel.value !== saved) {
            var opt2 = document.createElement('option');
            opt2.value = saved;
            opt2.textContent = saved.replace(/_/g, ' ');
            opt2.selected = true;
            sel.insertBefore(opt2, sel.options[1]);
        }
        if (!saved) {
            var browserTZ2 = Intl.DateTimeFormat().resolvedOptions().timeZone;
            for (var i = 0; i < sel.options.length; i++) {
                if (sel.options[i].value === browserTZ2) {
                    sel.selectedIndex = i;
                    break;
                }
            }
        }
    })();

    const form = document.getElementById("settingsForm");

    form.addEventListener("submit", (e) => {
        e.preventDefault();

        // Get stream grab interval and convert to seconds
        const streamGrabInterval = document.getElementById("streamGrabInterval").value * 60;

        // Gather form data
        const settings = {
            aci: {
                enabled: document.getElementById("aciEnabled").checked,
            },
            ec: {
                enabled: document.getElementById("ecEnabled").checked,
            },
            polling_interval: document.getElementById("pollingInterval").value,
            guest_mode: document.getElementById("guestMode").checked,
            stream_grab_enabled: document.getElementById("streamGrabEnabled").checked,
            stream_grab_interval: streamGrabInterval.toString(),
            api_key: "",
            disable_api_ingest: document.getElementById("disableApiIngest").checked,
            sensor_retention_days: document.getElementById("sensorRetentionDays").value,
            log_level: document.getElementById("logLevel").value,
            max_backup_size_mb: document.getElementById("maxBackupSizeMB").value,
            timezone: document.getElementById("timezone").value,
        };

        // Send POST request to save settings
        fetch("/settings", {
            method: "POST",
            headers: {
                "Content-Type": "application/json",
            },
            body: JSON.stringify(settings),
        })
            .then((response) => {
                if (!response.ok) {
                    throw new Error("{{ .lcl.failed_to_save_settings }}");
                }
                uiMessages.showToast(uiMessages.t('save_settings') || 'Settings saved', 'success');
            })
            .catch((error) => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t('failed_to_save_settings'), 'danger');
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
                    throw new Error("{{ .lcl.failed_to_add_activity }}");
                }
                return response.json();
            })
            .then((data) => {
                // Success: Close modal and reload page
                const modal = bootstrap.Modal.getInstance(document.getElementById("addActivityModal"));
                modal.hide();
                window.location.reload(); // Refresh page to show updated data
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
            .then(() => location.reload())
            .catch(err => uiMessages.showToast(uiMessages.t('failed_to_update_activity'), 'danger'));
    });

    deleteActivityButton.addEventListener("click", () => {
        const activityId = document.getElementById("activityId").value;

        uiMessages.showConfirm(uiMessages.t('delete_activity_confirm')).then(confirmed => {
            if (!confirmed) return;
            fetch(`/activities/${activityId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => location.reload())
                .catch(err => uiMessages.showToast(uiMessages.t('failed_to_delete_activity'), 'danger'));
        });
    });
});

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
            zone_id: document.getElementById("editStreamZone").value,
            url: document.getElementById("editStreamURL").value,
            visible: document.getElementById("editStreamVisibility").value,
        };

        fetch(`/streams/${streamId}`, {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        })
            .then(response => response.json())
            .then(() => location.reload())
            .catch(err => uiMessages.showToast(uiMessages.t('failed_to_update_stream'), 'danger'));
    });

    deleteStreamButton.addEventListener("click", () => {
        const streamId = document.getElementById("streamId").value;

        uiMessages.showConfirm(uiMessages.t('delete_stream_confirm')).then(confirmed => {
            if (!confirmed) return;
            fetch(`/streams/${streamId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => location.reload())
                .catch(err => uiMessages.showToast(uiMessages.t('failed_to_delete_stream'), 'danger'));
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
                    throw new Error("{{ .lcl.failed_to_add_metric }}");
                }
                return response.json();
            })
            .then((data) => {
                // Success: Close modal and reload page
                const modal = bootstrap.Modal.getInstance(document.getElementById("addMetricModal"));
                modal.hide();
                window.location.reload(); // Refresh page to show updated data
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
            .then(() => location.reload())
            .catch(err => uiMessages.showToast(uiMessages.t('failed_to_update_metric'), 'danger'));
    });

    deleteMetricButton.addEventListener("click", () => {
        const metricId = document.getElementById("metricId").value;

        uiMessages.showConfirm(uiMessages.t('delete_metric_confirm')).then(confirmed => {
            if (!confirmed) return;
            fetch(`/metrics/${metricId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => location.reload())
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
            .then((response) => {
                if (!response.ok) {
                    throw new Error("{{ .lcl.failed_to_add_breeder }}");
                }
                return response.json();
            })
            .then((data) => {
                // Success: Close modal and reload page
                const modal = bootstrap.Modal.getInstance(document.getElementById("addBreederModal"));
                modal.hide();
                window.location.reload(); // Refresh page to show updated data
            })
            .catch((error) => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t('failed_to_add_breeder'), 'danger');
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
            .then(response => response.json())
            .then(() => location.reload())
            .catch(err => uiMessages.showToast(uiMessages.t('failed_to_update_breeder'), 'danger'));
    });

    deleteBreederButton.addEventListener("click", () => {
        const breederId = document.getElementById("breederId").value;

        uiMessages.showConfirm(uiMessages.t('delete_breeder_confirm')).then(confirmed => {
            if (!confirmed) return;
            fetch(`/breeders/${breederId}`, { method: "DELETE" })
                .then(response => response.json())
                .then(() => location.reload())
                .catch(err => uiMessages.showToast(uiMessages.t('failed_to_delete_breeder'), 'danger'));
        });
    });
});

document.addEventListener("DOMContentLoaded", () => {
    const dropArea = document.getElementById("dropArea");
    const logoInput = document.getElementById("logoInput");
    const logoPreviewContainer = document.getElementById("logoPreviewContainer");
    const saveLogoButton = document.getElementById("saveLogoButton");

    dropArea.addEventListener("click", () => {
        logoInput.click();
    });

    logoInput.addEventListener("change", (e) => {
        const file = e.target.files[0];
        if (file) {
            const reader = new FileReader();
            reader.onload = (event) => {
                logoPreviewContainer.innerHTML = `<img src="${event.target.result}" class="img-fluid" alt="Preview">`;
            };
            reader.readAsDataURL(file);
        }
    });

    saveLogoButton.addEventListener("click", () => {
        const file = logoInput.files[0];
        if (file) {
            const formData = new FormData();
            formData.append("logo", file);

            fetch("/settings/upload-logo", {
                method: "POST",
                body: formData,
            })
                .then((response) => {
                    if (response.ok) {
                        uiMessages.showToast(uiMessages.t('logo_uploaded_successfully') || 'Logo uploaded', 'success');
                        location.reload();
                    } else {
                        uiMessages.showToast(uiMessages.t('error_uploading_logo') || 'Error uploading logo', 'danger');
                    }
                })
                .catch((error) => {
                    console.error("Error:", error);
                    uiMessages.showToast(uiMessages.t('error_uploading_logo') || 'Error uploading logo', 'danger');
                });
        } else {
            uiMessages.showToast(uiMessages.t('select_logo_image') || 'Select a logo image', 'warning');
        }
    });
});

// Function to generate a new API key
function generateNewAPIKey() {
    uiMessages.showConfirm(uiMessages.t('generate_new_key_confirm')).then(confirmed => {
        if (!confirmed) return;
        fetch('/settings', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ api_key: 'generate' })
        })
        .then(response => response.json())
        .then(data => {
            if (data.api_key) {
                // Show the one-time key display
                document.getElementById('apiKeyDisplay').value = data.api_key;
                document.getElementById('apiKeyReveal').classList.remove('d-none');
                document.getElementById('apiKeyCopyStatus').textContent = '';

                // Update status to show key is now configured
                document.getElementById('apiKeyStatus').innerHTML =
                    '<div class="d-flex align-items-center mb-2">' +
                    '<span class="badge bg-success me-2"><i class="fa fa-check"></i></span>' +
                    '<span class="text-success fw-semibold">API key is configured.</span>' +
                    '</div>';

                // Switch button to "Regenerate"
                document.getElementById('apiKeyActions').innerHTML =
                    '<button class="btn btn-outline-warning btn-sm" type="button" id="btnGenerateAPIKey">' +
                    '<i class="fa fa-sync me-1"></i> Regenerate API Key</button>';
                // Re-attach listener to the dynamically inserted button
                document.getElementById('btnGenerateAPIKey').addEventListener('click', generateNewAPIKey);
            }
        })
        .catch(error => console.error('Error:', error));
    });
}

function copyAPIKey() {
    const apiKeyInput = document.getElementById('apiKeyDisplay');
    navigator.clipboard.writeText(apiKeyInput.value).then(() => {
        document.getElementById('apiKeyCopyStatus').textContent = 'Copied to clipboard.';
        setTimeout(() => {
            document.getElementById('apiKeyCopyStatus').textContent = '';
        }, 3000);
    });
}

document.addEventListener("DOMContentLoaded", function () {
    // Clear any previously stored tab so the page always starts on the Settings tab
    localStorage.removeItem('activeSettingsTab');
});

document.addEventListener("DOMContentLoaded", () => {
    const logsTab = document.getElementById("logs-tab");
    const refreshBtn = document.getElementById("refreshLogsBtn");
    const logOutput = document.getElementById("logOutput");
    const logLineCount = document.getElementById("logLineCount");
    const logFileSelect = document.getElementById("logFileSelect");
    const downloadBtn = document.getElementById("downloadLogsBtn");

    function updateDownloadHref() {
        downloadBtn.href = `/settings/logs/download?file=${encodeURIComponent(logFileSelect.value)}`;
    }

    function loadLogs() {
        const lines = logLineCount.value;
        const file = logFileSelect.value;
        logOutput.textContent = "Loading...";
        fetch(`/settings/logs?lines=${encodeURIComponent(lines)}&file=${encodeURIComponent(file)}`)
            .then(r => r.json())
            .then(data => {
                logOutput.textContent = data.lines || "(empty)";
                logOutput.scrollTop = logOutput.scrollHeight;
            })
            .catch(() => {
                logOutput.textContent = "Failed to load logs.";
            });
    }

    logsTab.addEventListener("shown.bs.tab", loadLogs);
    refreshBtn.addEventListener("click", loadLogs);
    logLineCount.addEventListener("change", loadLogs);
    logFileSelect.addEventListener("change", () => { updateDownloadHref(); loadLogs(); });
    updateDownloadHref();
});

// ---- Backup Management ----
document.addEventListener("DOMContentLoaded", function () {
    // Localized strings for backup UI
    const bkT = {
        backupFailed:         "{{ .lcl.backup_failed }}",
        backupCreated:        "{{ .lcl.backup_created }}",
        loadFailed:           "{{ .lcl.backup_load_failed }}",
        selectZip:            "{{ .lcl.backup_select_zip }}",
        selectDb:             "{{ .lcl.backup_select_db }}",
        fileTooLargeHint:     "{{ .lcl.backup_file_too_large_hint }}",
        sqliteTooLargeHint:   "{{ .lcl.backup_sqlite_too_large_hint }}",
        confirmRestore:       "{{ .lcl.backup_confirm_restore }}",
        confirmSqliteReplace: "{{ .lcl.backup_confirm_sqlite_replace }}",
        uploading:            "{{ .lcl.backup_uploading }}",
        uploadProgress:       "{{ .lcl.backup_upload_progress }}",
        restoreFailed:        "{{ .lcl.backup_restore_failed }}",
        uploadFailed:         "{{ .lcl.backup_upload_failed }}",
        networkError:         "{{ .lcl.backup_network_error }}",
        phaseTruncating:      "{{ .lcl.backup_phase_truncating }}",
        progressTruncating:   "{{ .lcl.backup_progress_truncating }}",
        phaseRestoring:       "{{ .lcl.backup_phase_restoring }}",
        progressBatch:        "{{ .lcl.backup_progress_batch }}",
        tablesComplete:       "{{ .lcl.backup_tables_complete }}",
        of:                   "{{ .lcl.backup_of }}",
        restoring:            "{{ .lcl.backup_restoring }}",
        phaseSequences:       "{{ .lcl.backup_phase_sequences }}",
        progressAlmost:       "{{ .lcl.backup_progress_almost }}",
        phaseExtracting:      "{{ .lcl.backup_phase_extracting }}",
        progressExtracting:   "{{ .lcl.backup_progress_extracting }}",
        restoreComplete:      "{{ .lcl.backup_restore_complete }}",
        tablesFilesRestored:  "{{ .lcl.backup_tables_files_restored }}",
        filesRestored:        "{{ .lcl.backup_files_restored }}",
        unknownStatus:        "{{ .lcl.backup_unknown_status }}",
        uploadingDb:          "{{ .lcl.backup_uploading_db }}",
    };

    // --- Create Backup ---
    const createBtn = document.getElementById("createBackupBtn");
    const createStatus = document.getElementById("createBackupStatus");
    const sensorDays = document.getElementById("backupSensorDays");
    const includeImages = document.getElementById("backupIncludeImages");

    createBtn.addEventListener("click", () => {
        const params = new URLSearchParams({
            images: includeImages.checked,
            sensor_days: sensorDays.value,
        });
        createBtn.disabled = true;
        createStatus.style.display = "block";

        fetch("/settings/backup/create?" + params.toString(), { method: "POST" })
        .then(r => r.json())
        .then(data => {
            if (data.error) {
                createStatus.innerHTML = '<div class="alert alert-danger alert-sm mb-0">' + data.error + '</div>';
                createBtn.disabled = false;
                return;
            }
            // Poll for completion
            pollBackupStatus();
        })
        .catch(err => {
            createStatus.innerHTML = '<div class="alert alert-danger alert-sm mb-0">' + err.message + '</div>';
            createBtn.disabled = false;
        });
    });

    function pollBackupStatus() {
        fetch("/settings/backup/status")
        .then(r => r.json())
        .then(status => {
            if (status.in_progress) {
                setTimeout(pollBackupStatus, 2000);
            } else {
                createBtn.disabled = false;
                if (status.error) {
                    createStatus.innerHTML = '<div class="alert alert-danger alert-sm mb-0">' + bkT.backupFailed + status.error + '</div>';
                } else {
                    createStatus.innerHTML = '<div class="alert alert-success alert-sm mb-0">' + bkT.backupCreated + (status.filename || 'done') + '</div>';
                    setTimeout(() => { createStatus.style.display = "none"; }, 5000);
                    loadBackupList();
                }
            }
        })
        .catch(() => setTimeout(pollBackupStatus, 3000));
    }

    // --- Backup List ---
    const listLoading = document.getElementById("backupListLoading");
    const listTable = document.getElementById("backupListTable");
    const listBody = document.getElementById("backupListBody");
    const listEmpty = document.getElementById("backupListEmpty");
    const refreshBtn = document.getElementById("refreshBackupsBtn");

    function loadBackupList() {
        listLoading.style.display = "block";
        listTable.style.display = "none";
        listEmpty.style.display = "none";

        fetch("/settings/backup/list")
        .then(r => r.json())
        .then(backups => {
            listLoading.style.display = "none";
            if (!backups || backups.length === 0) {
                listEmpty.style.display = "block";
                return;
            }
            listTable.style.display = "table";
            listBody.innerHTML = "";
            backups.forEach(b => {
                const tr = document.createElement("tr");
                const created = new Date(b.created_at).toLocaleString();
                tr.innerHTML =
                    '<td><code style="font-size:0.8rem">' + b.name + '</code></td>' +
                    '<td>' + b.size_mb + ' MB</td>' +
                    '<td>' + created + '</td>' +
                    '<td>' +
                        '<a class="btn btn-sm btn-outline-primary me-1" href="/settings/backup/download/' + encodeURIComponent(b.name) + '"><i class="fa fa-download"></i></a>' +
                        '<button class="btn btn-sm btn-outline-danger backup-delete-btn" data-name="' + b.name + '"><i class="fa fa-trash"></i></button>' +
                    '</td>';
                listBody.appendChild(tr);
            });

            // Wire delete buttons
            document.querySelectorAll(".backup-delete-btn").forEach(btn => {
                btn.addEventListener("click", () => {
                    const name = btn.dataset.name;
                    if (!confirm("Delete backup " + name + "?")) return;
                    fetch("/settings/backup/" + encodeURIComponent(name), { method: "DELETE" })
                    .then(() => loadBackupList());
                });
            });
        })
        .catch(() => {
            listLoading.style.display = "none";
            listEmpty.style.display = "block";
            listEmpty.textContent = bkT.loadFailed;
        });
    }

    refreshBtn.addEventListener("click", loadBackupList);
    // Load list when backup tab is shown
    const backupTab = document.getElementById("backup-tab");
    backupTab.addEventListener("shown.bs.tab", loadBackupList);

    // --- Import / Restore ---
    const dropArea = document.getElementById("backupDropArea");
    const fileInput = document.getElementById("backupFileInput");
    const fileInfo = document.getElementById("backupFileInfo");
    const fileName = document.getElementById("backupFileName");
    const clearBtn = document.getElementById("backupClearBtn");
    const importBtn = document.getElementById("importBackupBtn");
    const progress = document.getElementById("backupProgress");
    const result = document.getElementById("backupResult");
    let selectedFile = null;

    dropArea.addEventListener("click", () => fileInput.click());
    dropArea.addEventListener("dragover", (e) => { e.preventDefault(); dropArea.classList.add("drop-highlight"); });
    dropArea.addEventListener("dragleave", () => dropArea.classList.remove("drop-highlight"));
    dropArea.addEventListener("drop", (e) => {
        e.preventDefault();
        dropArea.classList.remove("drop-highlight");
        if (e.dataTransfer.files.length) selectFile(e.dataTransfer.files[0]);
    });
    fileInput.addEventListener("change", () => { if (fileInput.files.length) selectFile(fileInput.files[0]); });

    function selectFile(file) {
        if (!file.name.endsWith(".zip")) {
            result.style.display = "block";
            result.innerHTML = '<div class="alert alert-danger mb-0">' + bkT.selectZip + '</div>';
            return;
        }
        const maxMB = parseInt(document.getElementById("maxBackupSizeMB").value) || 5120;
        if (file.size > maxMB * 1024 * 1024) {
            result.style.display = "block";
            result.innerHTML = '<div class="alert alert-danger mb-0">' +
                (file.size / 1024 / 1024).toFixed(1) + ' MB — ' + maxMB + ' MB limit. ' +
                bkT.fileTooLargeHint + '</div>';
            return;
        }
        selectedFile = file;
        fileName.textContent = file.name + " (" + (file.size / 1024 / 1024).toFixed(1) + " MB)";
        fileInfo.style.display = "inline";
        importBtn.disabled = false;
        result.style.display = "none";
    }

    clearBtn.addEventListener("click", () => {
        selectedFile = null;
        fileInput.value = "";
        fileInfo.style.display = "none";
        importBtn.disabled = true;
        result.style.display = "none";
    });

    const phaseText = document.getElementById("restorePhaseText");
    const progressBar = document.getElementById("restoreProgressBar");
    const detailText = document.getElementById("restoreDetailText");

    importBtn.addEventListener("click", () => {
        if (!selectedFile) return;
        if (!confirm(bkT.confirmRestore)) return;

        const formData = new FormData();
        formData.append("backup", selectedFile);
        const skipSensorEl = document.getElementById("skipSensorData");
        if (skipSensorEl && skipSensorEl.checked) {
            formData.append("skip_sensor_data", "true");
        }

        importBtn.disabled = true;
        progress.style.display = "block";
        result.style.display = "none";
        phaseText.textContent = bkT.uploading;
        progressBar.style.width = "100%";
        progressBar.textContent = bkT.uploadProgress;
        detailText.textContent = "";

        fetch("/settings/backup/restore", {
            method: "POST",
            body: formData,
        })
        .then(r => r.json().then(data => ({ok: r.ok, status: r.status, data})))
        .then(({ok, status, data}) => {
            if (status === 202) {
                // Async restore started — poll for progress
                pollRestoreStatus();
            } else if (!ok) {
                progress.style.display = "none";
                result.style.display = "block";
                result.innerHTML = '<div class="alert alert-danger mb-0">' + bkT.restoreFailed +
                    (data.error || data.message || "") + '</div>';
                importBtn.disabled = false;
            }
        })
        .catch(err => {
            progress.style.display = "none";
            result.style.display = "block";
            result.innerHTML = '<div class="alert alert-danger mb-0">' + bkT.networkError + err.message + '</div>';
            importBtn.disabled = false;
        });
    });

    function pollRestoreStatus() {
        fetch("/settings/backup/restore/status")
        .then(r => r.json())
        .then(status => {
            if (status.error) {
                progress.style.display = "none";
                result.style.display = "block";
                result.innerHTML = '<div class="alert alert-danger mb-0">' + bkT.restoreFailed + status.error + '</div>';
                importBtn.disabled = false;
                return;
            }

            if (status.in_progress) {
                // Update progress UI based on phase
                const phase = status.phase || "working";
                if (phase === "truncating") {
                    phaseText.textContent = bkT.phaseTruncating;
                    progressBar.style.width = "100%";
                    progressBar.textContent = bkT.progressTruncating;
                    detailText.textContent = "";
                } else if (phase === "restoring" && status.total_batches > 0) {
                    // Batched large table (sensor_data)
                    const pct = Math.round((status.batch_num / status.total_batches) * 100);
                    phaseText.textContent = bkT.phaseRestoring + status.current_table + "...";
                    progressBar.classList.remove("progress-bar-animated");
                    progressBar.style.width = pct + "%";
                    progressBar.textContent = bkT.progressBatch + status.batch_num + " / " + status.total_batches;
                    const done = (status.total_tables || 0) - (status.tables_left || 0) - 1;
                    detailText.textContent = Math.max(0, done) + bkT.of + (status.total_tables || 0) + bkT.tablesComplete;
                } else if (phase === "restoring") {
                    phaseText.textContent = bkT.phaseRestoring + (status.current_table || "data") + "...";
                    progressBar.style.width = "100%";
                    progressBar.textContent = bkT.restoring;
                    const done = (status.total_tables || 0) - (status.tables_left || 0);
                    detailText.textContent = done + bkT.of + (status.total_tables || 0) + bkT.tablesComplete;
                } else if (phase === "sequences") {
                    phaseText.textContent = bkT.phaseSequences;
                    progressBar.style.width = "95%";
                    progressBar.textContent = bkT.progressAlmost;
                    detailText.textContent = "";
                } else if (phase === "extracting") {
                    phaseText.textContent = bkT.phaseExtracting;
                    progressBar.style.width = "98%";
                    progressBar.textContent = bkT.progressExtracting;
                    detailText.textContent = "";
                }
                setTimeout(pollRestoreStatus, 1000);
            } else {
                // Complete
                if (status.phase === "complete") {
                    progress.style.display = "none";
                    result.style.display = "block";
                    result.innerHTML = '<div class="alert alert-success mb-0"><strong>' +
                        bkT.restoreComplete + '</strong>' + (status.tables || 0) + bkT.tablesFilesRestored +
                        (status.files || 0) + bkT.filesRestored + '</div>';
                    setTimeout(() => location.reload(), 2000);
                } else {
                    progress.style.display = "none";
                    result.style.display = "block";
                    result.innerHTML = '<div class="alert alert-warning mb-0">' + bkT.unknownStatus + '</div>';
                    importBtn.disabled = false;
                }
            }
        })
        .catch(() => {
            // Network blip — keep polling
            setTimeout(pollRestoreStatus, 2000);
        });
    }

    // --- SQLite File Upload ---
    const sqliteDropArea = document.getElementById("sqliteDropArea");
    const sqliteFileInput = document.getElementById("sqliteFileInput");
    const sqliteFileInfo = document.getElementById("sqliteFileInfo");
    const sqliteFileName = document.getElementById("sqliteFileName");
    const sqliteClearBtn = document.getElementById("sqliteClearBtn");
    const uploadSqliteBtn = document.getElementById("uploadSqliteBtn");
    const sqliteResult = document.getElementById("sqliteResult");
    let selectedSqliteFile = null;

    if (sqliteDropArea) {
        sqliteDropArea.addEventListener("click", () => sqliteFileInput.click());
        sqliteDropArea.addEventListener("dragover", (e) => { e.preventDefault(); sqliteDropArea.classList.add("drop-highlight"); });
        sqliteDropArea.addEventListener("dragleave", () => sqliteDropArea.classList.remove("drop-highlight"));
        sqliteDropArea.addEventListener("drop", (e) => {
            e.preventDefault();
            sqliteDropArea.classList.remove("drop-highlight");
            if (e.dataTransfer.files.length) selectSqliteFile(e.dataTransfer.files[0]);
        });
        sqliteFileInput.addEventListener("change", () => { if (sqliteFileInput.files.length) selectSqliteFile(sqliteFileInput.files[0]); });

        function selectSqliteFile(file) {
            if (!file.name.endsWith(".db")) {
                sqliteResult.style.display = "block";
                sqliteResult.innerHTML = '<div class="alert alert-danger mb-0">' + bkT.selectDb + '</div>';
                return;
            }
            const maxMB = parseInt(document.getElementById("maxBackupSizeMB").value) || 5120;
            if (file.size > maxMB * 1024 * 1024) {
                sqliteResult.style.display = "block";
                sqliteResult.innerHTML = '<div class="alert alert-danger mb-0">' +
                    (file.size / 1024 / 1024).toFixed(1) + ' MB — ' + maxMB + ' MB limit. ' +
                    bkT.sqliteTooLargeHint + '</div>';
                return;
            }
            selectedSqliteFile = file;
            sqliteFileName.textContent = file.name + " (" + (file.size / 1024 / 1024).toFixed(1) + " MB)";
            sqliteFileInfo.style.display = "inline";
            uploadSqliteBtn.disabled = false;
            sqliteResult.style.display = "none";
        }

        sqliteClearBtn.addEventListener("click", () => {
            selectedSqliteFile = null;
            sqliteFileInput.value = "";
            sqliteFileInfo.style.display = "none";
            uploadSqliteBtn.disabled = true;
            sqliteResult.style.display = "none";
        });

        uploadSqliteBtn.addEventListener("click", () => {
            if (!selectedSqliteFile) return;
            if (!confirm(bkT.confirmSqliteReplace)) return;

            const formData = new FormData();
            formData.append("database", selectedSqliteFile);

            uploadSqliteBtn.disabled = true;
            sqliteResult.style.display = "none";
            progress.style.display = "block";
            phaseText.textContent = bkT.uploadingDb;
            progressBar.style.width = "100%";
            progressBar.classList.add("progress-bar-animated");
            progressBar.textContent = bkT.uploadProgress;
            detailText.textContent = "";

            fetch("/settings/backup/sqlite/upload", {
                method: "POST",
                body: formData,
            })
            .then(r => r.json().then(data => ({ok: r.ok, status: r.status, data})))
            .then(({ok, status, data}) => {
                if (status === 202) {
                    pollRestoreStatus();
                } else if (!ok) {
                    progress.style.display = "none";
                    sqliteResult.style.display = "block";
                    sqliteResult.innerHTML = '<div class="alert alert-danger mb-0">' + bkT.uploadFailed +
                        (data.error || data.message || "") + '</div>';
                    uploadSqliteBtn.disabled = false;
                }
            })
            .catch(err => {
                progress.style.display = "none";
                sqliteResult.style.display = "block";
                sqliteResult.innerHTML = '<div class="alert alert-danger mb-0">' + bkT.networkError + err.message + '</div>';
                uploadSqliteBtn.disabled = false;
            });
        });
    }
});
