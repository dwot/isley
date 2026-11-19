(function() {
    // The table body carries the statuses and sensors as JSON.
    const tbody = document.getElementById('vpdStageTableBody');
    const parse = function(raw) { try { return JSON.parse(raw || "[]") || []; } catch (e) { return []; } };
    const statuses = tbody ? parse(tbody.dataset.statuses) : [];
    const sensors = tbody ? parse(tbody.dataset.sensors) : [];
    if (tbody && statuses) {
        statuses.forEach(function(st) {
            const tr = document.createElement('tr');
            tr.dataset.statusId = st.id;
            tr.innerHTML =
                '<td>' + st.status + '</td>' +
                '<td><input type="number" class="form-control form-control-sm vpd-low" step="0.01" value="' + (st.vpd_low != null ? st.vpd_low.toFixed(2) : '') + '" placeholder="—"></td>' +
                '<td><input type="number" class="form-control form-control-sm vpd-high" step="0.01" value="' + (st.vpd_high != null ? st.vpd_high.toFixed(2) : '') + '" placeholder="—"></td>' +
                '<td><button type="button" class="btn btn-sm btn-primary vpd-save-btn">Save</button></td>';
            tbody.appendChild(tr);
        });
        tbody.querySelectorAll('.vpd-save-btn').forEach(function(btn) {
            btn.addEventListener('click', function() {
                const row = btn.closest('tr');
                const statusId = row.dataset.statusId;
                const lowVal = row.querySelector('.vpd-low').value;
                const highVal = row.querySelector('.vpd-high').value;
                const payload = {
                    vpd_low:  lowVal  !== '' ? parseFloat(lowVal)  : null,
                    vpd_high: highVal !== '' ? parseFloat(highVal) : null,
                };
                fetch('/statuses/' + statusId + '/vpd', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify(payload),
                }).then(function(r) { return r.json(); })
                  .then(function() { uiMessages.showToast('VPD range saved', 'success'); })
                  .catch(function() { uiMessages.showToast('Failed to save VPD range', 'danger'); });
            });
        });
    }

    // Populate VPD sensor selects when the zone edit modal opens.
    // This runs after the modal JS below, so we use a custom event approach.
    window._vpdSensors = sensors || [];
})();
