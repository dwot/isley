document.addEventListener("DOMContentLoaded", () => {
    const container = document.getElementById("plantStatusRibbonContainer");
    if (!container) return;

    // Ensure postStatus is available (fallback) to avoid ReferenceError from cached/partial script loads
    if (typeof window.postStatus === 'undefined') {
        window.postStatus = function(plant_id, status_id, date) {
            const payload = { plant_id: plant_id, status_id: status_id };
            if (date) payload.date = date;
            return fetch('/plant/status', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify(payload)
            }).then(resp => {
                if (!resp.ok) throw new Error('Failed to create status');
                return resp.json();
            });
        };
    }

    const plantId = parseInt(container.dataset.plantId, 10);
    const currentStatusId = parseInt(container.dataset.currentStatusId || '0', 10);
    const statusHistoryJson = container.dataset.statusHistory || "[]";
    let statusHistory;
    try {
        statusHistory = JSON.parse(statusHistoryJson);
    } catch (e) {
        console.error("Failed to parse status history JSON", e);
        statusHistory = [];
    }

    // Build a map of most recent history entry by status_id
    const reachedByStatusId = {};
    statusHistory.forEach(s => {
        // status_id may be available in various casing
        const sid = s.status_id || s.StatusID || s.statusId || s.StatusId || s.Status_id;
        if (sid) {
            if (!reachedByStatusId[sid]) reachedByStatusId[sid] = s;
        }
    });

    // Build a map of server-rendered formatted date strings for each status id.
    // This ensures the ribbon uses the same human-readable format the server/template uses
    // (avoids client-side timezone/locale conversions like toLocaleString producing different times).
    const serverFormattedDateByStatusId = {};
    try {
        document.querySelectorAll('.status-row').forEach(row => {
            try {
                const raw = row.getAttribute('data-status');
                if (!raw) return;
                const parsed = JSON.parse(raw);
                const sid = parsed.status_id || parsed.StatusID || parsed.statusId || parsed.StatusId || parsed.id || parsed.ID || parsed.Status_id;
                const td = row.querySelector('td');
                const text = td ? td.textContent.trim() : null;
                if (sid && text) serverFormattedDateByStatusId[sid] = text;
            } catch (e) {
                // ignore malformed rows
            }
        });
    } catch (e) {
        // if table not present, just ignore and fallback to existing behavior
    }

    const steps = Array.from(container.querySelectorAll('.status-step'));
    if (!steps.length) return;

    // Find index of the current status id among steps
    let currentIndex = -1;
    steps.forEach((el, idx) => {
        const sid = parseInt(el.dataset.statusId, 10);
        if (sid === currentStatusId) currentIndex = idx;
    });

    // If no explicit currentStatusId from server, derive from reachedByStatusId (highest index)
    if (currentIndex === -1) {
        steps.forEach((el, idx) => {
            const sid = parseInt(el.dataset.statusId, 10);
            if (reachedByStatusId[sid]) currentIndex = Math.max(currentIndex, idx);
        });
    }

    // Hydrate UI state for each step
    steps.forEach((el, idx) => {
        // Helper: derive a localized label for a given raw status string.
        function localizeStatus(raw) {
            if (!raw) return raw;
            const key = `${raw.toLowerCase()}_label`;
            // Prefer server-rendered label in the DOM if present
            const nameEl = el.querySelector('.status-name');
            if (nameEl && nameEl.textContent && nameEl.textContent.trim() !== raw) return nameEl.textContent.trim();
            if (window.uiMessages && typeof window.uiMessages.t === 'function') {
                try {
                    const t = window.uiMessages.t(key);
                    if (t && t !== key) return t;
                } catch (e) {
                    // ignore
                }
            }
            return raw;
        }

        const sid = parseInt(el.dataset.statusId, 10);
        const dateEl = el.querySelector('.status-date');
        const iconEl = el.querySelector('.status-icon i');

        if (idx <= currentIndex && currentIndex >= 0) {
            // Past or current
            el.classList.add('status-past');
            if (idx === currentIndex) el.classList.add('status-current');
            // show date if we have history for this status
            const hist = reachedByStatusId[sid];
            if (hist) {
                const d = new Date(hist.date || hist.Date);
                // Prefer server-rendered formatted date when available to avoid timezone shifts
                const serverDate = serverFormattedDateByStatusId[sid];
                if (serverDate) {
                    dateEl.textContent = serverDate;
                } else {
                    if (!isNaN(d.getTime())) dateEl.textContent = d.toLocaleString();
                }
            }
            el.classList.remove('text-muted');
        } else {
            // Future
            el.classList.add('status-future', 'text-muted');
            dateEl.textContent = '';
        }

        // Icon heuristics
        const sname = (el.dataset.statusName || '').toLowerCase();
        let iconClass = 'fa-lemon';
        if (sname.includes('germ')) iconClass = 'fa-lemon';
        else if (sname.includes('plant') && !sname.includes('ing')) iconClass = 'fa-bucket';
        else if (sname.includes('seedling')) iconClass = 'fa-seedling';
        else if (sname.includes('veg')) iconClass = 'fa-plant-wilt';
        else if (sname.includes('flower')) iconClass = 'fa-cannabis';
        else if (sname.includes('dry')) iconClass = 'fa-sun';
        else if (sname.includes('cur')) iconClass = 'fa-jar';
        else if (sname.includes('success')) iconClass = 'fa-award';
        else if (sname.includes('dead')) iconClass = 'fa-skull-crossbones';
        if (iconEl) iconEl.className = `fa-solid ${iconClass} fa-2x`;

        // Click handler
        const btn = el.querySelector('.status-icon');
        btn.addEventListener('click', async (evt) => {
            evt.preventDefault();
            // If this index is <= currentIndex -> open edit modal
            if (idx <= currentIndex && currentIndex >= 0) {
                const hist = reachedByStatusId[sid];
                if (hist) {
                    openEditStatusModal(hist);
                    return;
                }

                // No history entry exists for this past stage: compute placeholder date
                let leftDate = null;
                let rightDate = null;

                // search left for nearest reached
                for (let i = idx - 1; i >= 0; i--) {
                    const leftSid = parseInt(steps[i].dataset.statusId, 10);
                    const leftHist = reachedByStatusId[leftSid];
                    if (leftHist) { leftDate = new Date(leftHist.date || leftHist.Date); break; }
                }
                // search right for nearest reached
                for (let i = idx + 1; i < steps.length; i++) {
                    const rightSid = parseInt(steps[i].dataset.statusId, 10);
                    const rightHist = reachedByStatusId[rightSid];
                    if (rightHist) { rightDate = new Date(rightHist.date || rightHist.Date); break; }
                }

                let placeholderDate;
                if (leftDate && rightDate) {
                    const mid = new Date((leftDate.getTime() + rightDate.getTime()) / 2);
                    placeholderDate = mid.toISOString().slice(0,19);
                } else if (leftDate) {
                    const d = new Date(leftDate.getTime() + 3600*1000);
                    placeholderDate = d.toISOString().slice(0,19);
                } else if (rightDate) {
                    const d = new Date(rightDate.getTime() - 3600*1000);
                    placeholderDate = d.toISOString().slice(0,19);
                } else {
                    placeholderDate = new Date().toISOString().slice(0,19);
                }
                
                try {
                    const res = await window.postStatus(plantId, sid, placeholderDate);
                    const newId = res && res.id ? res.id : 0;
                    reachedByStatusId[sid] = { id: newId, status: el.dataset.statusName, date: placeholderDate, status_id: sid };

                     // Update UI: show date and mark as past
                     const d = new Date(placeholderDate);
                     if (!isNaN(d.getTime())) dateEl.textContent = d.toLocaleString();
                     el.classList.add('status-past');
                     el.classList.remove('status-future', 'text-muted');

                     // Don't auto-open modal — user can click again to edit the placeholder
                     return;
                 } catch (err) {
                    console.error('Failed to create placeholder status', err);
                     uiMessages.showToast(uiMessages.t('failed_to_create_placeholder_status') || 'Failed to create placeholder status', 'danger');
                     return;
                 }
            }

            // Future: advance
            const targetName = el.dataset.statusName || '';
            const isTerminal = /dead|success/i.test(targetName);
            // Only prompt for terminal statuses (e.g., 'dead', 'success'). For normal advances, proceed directly.
            if (isTerminal) {
                const displayLabelForConfirm = localizeStatus(targetName);
                const confirmMsg = (window.uiMessages && typeof uiMessages.t === 'function' && uiMessages.t('confirm_set_status_to')) ? uiMessages.t('confirm_set_status_to').replace('{status}', displayLabelForConfirm) : `Are you sure you want to set status to ${displayLabelForConfirm}?`;
                const confirmed = await uiMessages.showConfirm(confirmMsg);
                if (!confirmed) return;
            }
            
            const payload = { plant_id: plantId, status_id: sid };
            // Optimistic UI: update status text and step classes immediately
            const plantStatusTextEl = document.getElementById('plantStatusText');
            // Use localized label for display while keeping raw status for DB payload
            const displayLabel = el.querySelector('.status-name')?.textContent?.trim() || localizeStatus(el.dataset.statusName || targetName);
            if (plantStatusTextEl) plantStatusTextEl.textContent = displayLabel;
            steps.forEach((sEl, sIdx) => {
                sEl.classList.remove('status-past', 'status-current', 'status-future');
                if (sIdx <= idx) {
                    sEl.classList.add('status-past');
                    if (sIdx === idx) sEl.classList.add('status-current');
                    sEl.classList.remove('text-muted');
                } else {
                    sEl.classList.add('status-future', 'text-muted');
                }
            });
            el.classList.add('status-pending');
            fetch('/plant/status', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(payload) })
                .then(resp => { if (!resp.ok) throw new Error('Failed'); return resp.json(); })
                .then(() => { location.reload(); })
                .catch(err => { console.error(err); uiMessages.showToast(uiMessages.t('failed_to_advance_status') || 'Failed to advance status', 'danger'); el.classList.remove('status-pending'); });
        });
    });

    function openEditStatusModal(statusObj) {
        try {
            const editModalEl = document.getElementById('editStatusModal');
            if (!editModalEl) { uiMessages.showToast(uiMessages.t('edit_modal_not_found') || 'Edit modal not found', 'danger'); return; }
            const statusIdInput = document.getElementById('statusId');
            const editDateInput = document.getElementById('editStatusDate');
            if (!statusIdInput || !editDateInput) { uiMessages.showToast(uiMessages.t('edit_modal_inputs_not_found') || 'Edit modal inputs not found', 'danger'); return; }
            statusIdInput.value = statusObj.id || statusObj.ID;
            const d = new Date(statusObj.date || statusObj.Date);
            editDateInput.value = d.toISOString().slice(0,19);
            new bootstrap.Modal(editModalEl).show();
        } catch (e) {
            console.error('Failed to open edit status modal', e);
        }
    }

});