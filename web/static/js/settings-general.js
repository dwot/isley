document.addEventListener("DOMContentLoaded", () => {
    const pollingSlider = document.getElementById("pollingInterval");
    const pollingValue = document.getElementById("pollingValue");

    // API key management: load the current list and wire up the controls.
    loadAPIKeys();
    const createBtn = document.getElementById("btnCreateAPIKey");
    if (createBtn) createBtn.addEventListener("click", createAPIKey);
    const newKeyName = document.getElementById("newApiKeyName");
    if (newKeyName) newKeyName.addEventListener("keydown", (e) => {
        if (e.key === "Enter") { e.preventDefault(); createAPIKey(); }
    });
    const copyBtn = document.getElementById("btnCopyAPIKey");
    if (copyBtn) copyBtn.addEventListener("click", copyAPIKey);
    const keysList = document.getElementById("apiKeysList");
    if (keysList) keysList.addEventListener("click", onAPIKeyAction);
    if (keysList) keysList.addEventListener("change", onAPIKeyAccessChange);
    const newKeyLevel = document.getElementById("newApiKeyLevel");
    const newKeyIngest = document.getElementById("newApiKeyIngest");
    if (newKeyLevel && newKeyIngest) {
        newKeyLevel.addEventListener("change", () => syncIngestBox(newKeyLevel, newKeyIngest));
    }

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
        streamGrabValue.textContent = `${minutes} ${uiMessages.t("time_minutes")}`;
        streamGrabSlider.setAttribute('data-seconds', seconds);
    });

    // Set initial value in seconds
    streamGrabSlider.setAttribute('data-seconds', streamGrabSlider.value * 60);
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
                    document.getElementById("tokenSetBadge").textContent = uiMessages.t("token_set");
                    document.getElementById("tokenSetBadge").classList.remove("bg-danger");
                    document.getElementById("tokenSetBadge").classList.add("bg-success");
                } else {
                    uiMessages.showToast(uiMessages.t('failed_to_fetch_token') + ': ' + data.message, 'danger');
                }
            })
            .catch((error) => {
                console.error(uiMessages.t("error_fetching_token"), error);
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
            cannadb: {
                enabled: document.getElementById("cannadbEnabled").checked,
                base_url: document.getElementById("cannadbBaseUrl").value.trim(),
            },
            polling_interval: document.getElementById("pollingInterval").value,
            guest_mode: document.getElementById("guestMode").checked,
            stream_grab_enabled: document.getElementById("streamGrabEnabled").checked,
            stream_grab_interval: streamGrabInterval.toString(),
            api_key: "",
            disable_api_ingest: document.getElementById("disableApiIngest").checked,
            mcp_enabled: document.getElementById("mcpEnabled").checked,
            sensor_retention_days: document.getElementById("sensorRetentionDays").value,
            log_level: document.getElementById("logLevel").value,
            max_backup_size_mb: document.getElementById("maxBackupSizeMB").value,
            timezone: document.getElementById("timezone").value,
            weight_unit: document.getElementById("weightUnit").value,
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
                    throw new Error(uiMessages.t("failed_to_save_settings"));
                }
                uiMessages.showToast(uiMessages.t('save_settings') || 'Settings saved', 'success');
            })
            .catch((error) => {
                console.error("Error:", error);
                uiMessages.showToast(uiMessages.t('failed_to_save_settings'), 'danger');
            });
    });
});
