// ---- Backup Management ----
document.addEventListener("DOMContentLoaded", function () {
    // Localized strings for backup UI
    const bkT = {
        get backupFailed() { return uiMessages.t("backup_failed"); },
        get backupCreated() { return uiMessages.t("backup_created"); },
        get loadFailed() { return uiMessages.t("backup_load_failed"); },
        get selectZip() { return uiMessages.t("backup_select_zip"); },
        get selectDb() { return uiMessages.t("backup_select_db"); },
        get fileTooLargeHint() { return uiMessages.t("backup_file_too_large_hint"); },
        get sqliteTooLargeHint() { return uiMessages.t("backup_sqlite_too_large_hint"); },
        get confirmRestore() { return uiMessages.t("backup_confirm_restore"); },
        get confirmSqliteReplace() { return uiMessages.t("backup_confirm_sqlite_replace"); },
        get uploading() { return uiMessages.t("backup_uploading"); },
        get uploadProgress() { return uiMessages.t("backup_upload_progress"); },
        get restoreFailed() { return uiMessages.t("backup_restore_failed"); },
        get uploadFailed() { return uiMessages.t("backup_upload_failed"); },
        get networkError() { return uiMessages.t("backup_network_error"); },
        get phaseTruncating() { return uiMessages.t("backup_phase_truncating"); },
        get progressTruncating() { return uiMessages.t("backup_progress_truncating"); },
        get phaseRestoring() { return uiMessages.t("backup_phase_restoring"); },
        get progressBatch() { return uiMessages.t("backup_progress_batch"); },
        get tablesComplete() { return uiMessages.t("backup_tables_complete"); },
        get of() { return uiMessages.t("backup_of"); },
        get restoring() { return uiMessages.t("backup_restoring"); },
        get phaseSequences() { return uiMessages.t("backup_phase_sequences"); },
        get progressAlmost() { return uiMessages.t("backup_progress_almost"); },
        get phaseExtracting() { return uiMessages.t("backup_phase_extracting"); },
        get progressExtracting() { return uiMessages.t("backup_progress_extracting"); },
        get restoreComplete() { return uiMessages.t("backup_restore_complete"); },
        get tablesFilesRestored() { return uiMessages.t("backup_tables_files_restored"); },
        get filesRestored() { return uiMessages.t("backup_files_restored"); },
        get unknownStatus() { return uiMessages.t("backup_unknown_status"); },
        get uploadingDb() { return uiMessages.t("backup_uploading_db"); },
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
                const created = IsleyTime.formatDateTime(b.created_at) || b.created_at;
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
                btn.addEventListener("click", async () => {
                    const name = btn.dataset.name;
                    const ok = await uiMessages.confirmDelete(
                        uiMessages.t('confirm_delete_backup', 'Delete backup {name}?').replace('{name}', name),
                        uiMessages.t('delete_backup', 'Delete Backup'));
                    if (!ok) return;
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

    importBtn.addEventListener("click", async () => {
        if (!selectedFile) return;
        const ok = await uiMessages.showConfirm(bkT.confirmRestore, {
            title: uiMessages.t('backup_restore_title', 'Restore Backup'),
            confirmText: uiMessages.t('backup_restore_btn', 'Restore Backup'),
            variant: 'danger',
        });
        if (!ok) return;

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
                // Translated when the server sent a key; text, not HTML.
                let reason = status.error_key ? uiMessages.t(status.error_key, '') : '';
                if (reason && status.error_detail) reason += ': ' + status.error_detail;
                const alert = document.createElement('div');
                alert.className = 'alert alert-danger mb-0';
                alert.textContent = bkT.restoreFailed + (reason || status.error);
                result.replaceChildren(alert);
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
                    setTimeout(() => reloadKeepingPlace(), 2000);
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

        uploadSqliteBtn.addEventListener("click", async () => {
            if (!selectedSqliteFile) return;
            const ok = await uiMessages.showConfirm(bkT.confirmSqliteReplace, {
                title: uiMessages.t('replace_database', 'Replace Database'),
                confirmText: uiMessages.t('replace_database', 'Replace Database'),
                variant: 'danger',
            });
            if (!ok) return;

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
