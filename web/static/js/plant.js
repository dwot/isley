document.addEventListener("DOMContentLoaded", () => {
    // ===== Timeline =====
    const card = document.getElementById("timelineCard");
    if (!card) return;

    const plantId = parseInt(card.dataset.plantId, 10);
    const loggedIn = card.dataset.loggedIn === "true";
    const currentStatusId = parseInt(card.dataset.currentStatusId || "0", 10);
    const daysLabel = card.dataset.daysLabel || "Days";

    function safeParseJSON(raw) {
        try {
            let parsed = JSON.parse(raw || "[]");
            if (typeof parsed === "string") parsed = JSON.parse(parsed);
            return Array.isArray(parsed) ? parsed : [];
        } catch (e) { return []; }
    }

    const statusHistory = safeParseJSON(card.dataset.statusHistory);

    const reachedByStatusId = {};
    statusHistory.forEach(s => {
        const sid = s.status_id || s.StatusID || s.statusId || s.StatusId || s.Status_id;
        if (sid && !reachedByStatusId[sid]) reachedByStatusId[sid] = s;
    });

    const statusOptions = Array.from(document.querySelectorAll("#plantStatusData .status-option"));
    let currentStepIndex = -1;
    statusOptions.forEach((el, idx) => {
        if (parseInt(el.dataset.statusId, 10) === currentStatusId) currentStepIndex = idx;
    });

    // Day counts come from the server (service.PlantDetail), so the
    // timeline agrees with the plant's Day and the dashboard cards.
    const currentStage = card.dataset.currentStage || "";
    const isTerminalPlant = /^(dead|success)$/i.test(currentStage);
    const plantIsDead = /^dead$/i.test(currentStage);
    let stageDays = {};
    try { stageDays = JSON.parse(card.dataset.stageDays || "{}") || {}; } catch (e) { stageDays = {}; }
    const totalDays = parseInt(card.dataset.growDays || "0", 10);

    // Helpers
    function stageIcon(name) {
        const n = (name || "").toLowerCase();
        if (n.includes("germ")) return "fa-lemon";
        if (n.includes("plant") && !n.includes("ing")) return "fa-bucket";
        if (n.includes("seedling")) return "fa-seedling";
        if (n.includes("veg")) return "fa-plant-wilt";
        if (n.includes("flower")) return "fa-cannabis";
        if (n.includes("dry")) return "fa-sun";
        if (n.includes("cur")) return "fa-jar";
        if (n.includes("success")) return "fa-award";
        if (n.includes("dead")) return "fa-skull-crossbones";
        return "fa-circle";
    }

    function formatDays(d) {
        return Number.isFinite(d) && d > 0 ? `${d}` : "0";
    }

    // Total days
    const totalDaysEl = document.getElementById("growthTotalDays");
    if (totalDaysEl) totalDaysEl.textContent = formatDays(totalDays);

    // Build horizontal timeline
    const track = document.getElementById("htlTrack");
    if (!track) return;
    track.innerHTML = "";

    let renderedCount = 0;

    statusOptions.forEach((opt, i) => {
        const sid = parseInt(opt.dataset.statusId, 10);
        const name = opt.dataset.statusName;
        const label = opt.dataset.statusLabel;
        const idx = parseInt(opt.dataset.statusIndex, 10);
        const days = stageDays[name] || 0;
        const reached = !!reachedByStatusId[sid];
        const isCurrent = idx === currentStepIndex;
        const isFuture = idx > currentStepIndex && currentStepIndex >= 0;

        // Hide Dead from timeline unless plant is actually dead
        if (/^dead$/i.test(name) && !plantIsDead) return;

        // For terminal plants (Dead/Success), skip unreached stages
        if (isTerminalPlant && !reached && !isCurrent) return;

        const node = document.createElement("div");
        node.className = "htl-node";
        if (isCurrent) node.classList.add("htl-active");
        if (isFuture) node.classList.add("htl-future");
        if (reached && !isCurrent) node.classList.add("htl-reached");

        // Connector line (skip before first rendered node)
        if (renderedCount > 0) {
            const conn = document.createElement("div");
            conn.className = "htl-connector";
            if (reached || isCurrent) conn.classList.add("htl-connector-filled");
            track.appendChild(conn);
        }

        // Icon circle
        const icon = document.createElement("div");
        icon.className = "htl-icon";
        icon.innerHTML = `<i class="fa-solid ${stageIcon(name)}"></i>`;

        // Label
        const lbl = document.createElement("div");
        lbl.className = "htl-label";
        lbl.textContent = label;

        // Day pill (only if this stage was reached and has duration)
        const meta = document.createElement("div");
        meta.className = "htl-meta";
        if (days > 0) {
            meta.innerHTML = `<span class="htl-days">${formatDays(days)}d</span>`;
        } else if (reached) {
            const hist = reachedByStatusId[sid];
            const shown = IsleyTime.formatDate(hist.date || hist.Date, { month: "short", day: "numeric" });
            if (shown) {
                meta.innerHTML = `<span class="htl-date">${shown}</span>`;
            }
        }

        node.appendChild(icon);
        node.appendChild(lbl);
        node.appendChild(meta);

        // Click handler for edit / advance
        if (loggedIn) {
            node.style.cursor = "pointer";
            node.addEventListener("click", () => {
                if (reached) {
                    const hist = reachedByStatusId[sid];
                    if (hist) openEditStatusModal(hist);
                } else if (isFuture) {
                    advanceToStatus(sid, name);
                }
            });
        }

        track.appendChild(node);
        renderedCount++;
    });

    // Kill button wiring
    const killBtn = document.getElementById("killPlantBtn");
    if (killBtn) {
        const deadOpt = statusOptions.find(o => /^dead$/i.test(o.dataset.statusName));
        if (deadOpt) {
            const deadSid = parseInt(deadOpt.dataset.statusId, 10);
            killBtn.addEventListener("click", () => advanceToStatus(deadSid, "Dead"));
        }
    }

    // Status handlers
    if (typeof window.postStatus === "undefined") {
        window.postStatus = function(plant_id, status_id, date) {
            const payload = { plant_id, status_id };
            if (date) payload.date = date;
            return fetch("/plant/status", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(payload)
            }).then(r => { if (!r.ok) throw new Error("Failed"); return r.json(); });
        };
    }

    function openEditStatusModal(statusObj) {
        try {
            const editModalEl = document.getElementById("editStatusModal");
            if (!editModalEl) return;
            const statusIdInput = document.getElementById("statusId");
            const editDateInput = document.getElementById("editStatusDate");
            if (!statusIdInput || !editDateInput) return;
            statusIdInput.value = statusObj.id || statusObj.ID;
            editDateInput.value = IsleyTime.toInput(statusObj.date || statusObj.Date);
            new bootstrap.Modal(editModalEl).show();
        } catch (e) {
            console.error("Failed to open edit status modal", e);
        }
    }

    async function advanceToStatus(sid, name) {
        const isTerminal = /dead|success/i.test(name);
        if (isTerminal) {
            const label = uiMessages.t(`${String(name).toLowerCase()}_label`, name);
            const msg = uiMessages.t('confirm_set_status_to', 'Are you sure you want to set status to {status}?').replace('{status}', label);
            const confirmed = await uiMessages.showConfirm(msg, {
                title: uiMessages.t('change_status', 'Change Status'),
                confirmText: uiMessages.t('confirm', 'Confirm'),
                variant: /dead/i.test(name) ? 'danger' : 'primary',
            });
            if (!confirmed) return;
        }
        try {
            await window.postStatus(plantId, sid);
            location.reload();
        } catch (err) {
            console.error(err);
            if (window.uiMessages) uiMessages.showToast(uiMessages.t('failed_to_advance_status', 'Failed to advance status'), "danger");
        }
    }

    // ===== Grouped Measurements =====
    const mGroupEl = document.getElementById("measurementGroupList");
    if (mGroupEl) {
        let measurements = [];
        try {
            let raw = JSON.parse(mGroupEl.dataset.measurements || "[]");
            if (typeof raw === "string") raw = JSON.parse(raw);
            measurements = Array.isArray(raw) ? raw : [];
        } catch(e) { /* empty */ }

        // Group by name, keep order of first appearance
        const grouped = new Map();
        measurements.forEach(m => {
            const name = m.name || m.Name;
            if (!grouped.has(name)) grouped.set(name, []);
            grouped.get(name).push(m);
        });

        mGroupEl.innerHTML = "";

        grouped.forEach((entries, name) => {
            // entries are sorted newest-first from the server
            const latest = entries[0];
            const latestVal = latest.value ?? latest.Value;
            const dateStr = IsleyTime.formatDate(latest.date || latest.Date, { month: "short", day: "numeric", year: "numeric" });

            const row = document.createElement("div");
            row.className = "pm-group";

            // Summary row (always visible)
            const summary = document.createElement("div");
            summary.className = "pm-summary";
            summary.innerHTML =
                `<span class="pm-name">${name}</span>` +
                `<span class="pm-latest">` +
                    `<strong class="pm-val">${latestVal}</strong>` +
                    `<span class="pm-date">${dateStr}</span>` +
                `</span>`;

            row.appendChild(summary);

            // History rows (hidden by default)
            if (entries.length > 1) {
                const histWrap = document.createElement("div");
                histWrap.className = "pm-history";
                histWrap.style.display = "none";

                entries.forEach((m, idx) => {
                    const mVal = m.value ?? m.Value;
                    const mDateStr = IsleyTime.formatDate(m.date || m.Date, { month: "short", day: "numeric", year: "numeric" });
                    const hRow = document.createElement("div");
                    hRow.className = "pm-history-row clickable-row measurement-row";
                    hRow.dataset.measurement = JSON.stringify(m);
                    hRow.innerHTML =
                        `<span class="pm-hist-date">${mDateStr}</span>` +
                        `<span class="pm-hist-val">${mVal}</span>`;
                    histWrap.appendChild(hRow);
                });

                row.appendChild(histWrap);

                // Toggle on click
                const toggle = document.createElement("span");
                toggle.className = "pm-toggle";
                toggle.innerHTML = `<i class="fa-solid fa-chevron-down fa-xs"></i> ${entries.length}`;
                toggle.title = `${entries.length} entries`;
                summary.appendChild(toggle);

                summary.style.cursor = "pointer";
                summary.addEventListener("click", () => {
                    const open = histWrap.style.display !== "none";
                    histWrap.style.display = open ? "none" : "";
                    toggle.innerHTML = open
                        ? `<i class="fa-solid fa-chevron-down fa-xs"></i> ${entries.length}`
                        : `<i class="fa-solid fa-chevron-up fa-xs"></i> ${entries.length}`;
                });
            }

            mGroupEl.appendChild(row);
        });
    }

    // ===== Plant navigation (arrow keys) =====
    var plantPage = document.querySelector('[data-prev-plant-id]');
    var prevId = plantPage ? Number(plantPage.dataset.prevPlantId) : 0;
    var nextId = plantPage ? Number(plantPage.dataset.nextPlantId) : 0;
    document.addEventListener('keydown', function(e) {
        // Don't navigate if the image modal is open...
        var imageModal = document.getElementById('imageModal');
        if (imageModal && imageModal.classList.contains('show')) return;
        // ...focus is on an input, textarea, select...
        var tag = document.activeElement && document.activeElement.tagName;
        if (tag === 'INPUT' || tag === 'TEXTAREA' || tag === 'SELECT') return;
        // ...or on a contenteditable enabled
        if (document.activeElement && document.activeElement.isContentEditable) return;
        if (e.key === 'ArrowLeft' && prevId) {
            window.location.href = '/plant/' + prevId;
        } else if (e.key === 'ArrowRight' && nextId) {
            window.location.href = '/plant/' + nextId;
        }
    });
});
