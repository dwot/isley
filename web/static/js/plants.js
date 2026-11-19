document.addEventListener("DOMContentLoaded", () => {
    const cardsContainer = document.getElementById("plantsCards");
    const tableWrap = document.getElementById("plantsTable");
    const tableBody = document.getElementById("plantsTableBody");
    const emptyState = document.getElementById("emptyState");
    const searchInput = document.getElementById("searchPlants");
    const filterZone = document.getElementById("filterZone");
    const filterZoneGroup = document.getElementById("filterZoneGroup");
    const filterBreeder = document.getElementById("filterBreeder");
    const filterBreederGroup = document.getElementById("filterBreederGroup");
    const filterStatus = document.getElementById("filterStatus");
    const filterStatusGroup = document.getElementById("filterStatusGroup");
    const sortBy = document.getElementById("sortBy");
    const clearBtn = document.getElementById("clearFilters");
    const resultCount = document.getElementById("resultCount");
    const viewButtons = document.querySelectorAll(".view-button");
    const layoutButtons = document.querySelectorAll(".layout-toggle");
    const headerDynamic1 = document.getElementById("dynamicHeader1");
    const headerDynamic2 = document.getElementById("dynamicHeader2");
    const headerActions = document.getElementById("dynamicHeaderActions");

    // The page says whether this visitor may edit, and the statuses a plant
    // can be given.
    const page = document.getElementById("plantsPage");
    const canEdit = page.dataset.canEdit === "true";
    // Harvest weights arrive in grams; this is the unit to show them in first.
    const weightUnit = page.dataset.weightUnit || "g";
    let parsedStatusOptions = [];
    try { parsedStatusOptions = JSON.parse(page.dataset.statuses || "[]"); } catch (e) { /* none */ }
    const statusOptions = Array.isArray(parsedStatusOptions) ? parsedStatusOptions : [];
    const statusIdByName = new Map(statusOptions.map(s => [s.status, s.id]));

    let currentView = "living";
    let currentLayout = localStorage.getItem("plantsLayout") || "cards";
    let allPlants = [];

    // ----- Table header sort click -----
    const sortableTable = new IsleySortableTable(sortBy, {
        prefix: "pt",
        headerToDropdown: {
            name:   { asc: "name-asc",   desc: "name-desc" },
            strain: { asc: "strain-asc", desc: "strain-desc" },
            zone:   { asc: "zone-asc",   desc: "zone-desc" },
            status: { asc: "status-asc", desc: "status-desc" },
            // col1 is harvest weight in the harvested view, plant age otherwise
            col1:   () => currentView === "harvested"
                        ? { asc: "weight-asc", desc: "weight-desc" }
                        : { asc: "day-asc",    desc: "day-desc" },
            col2:   { asc: "day-asc", desc: "day-desc" },
        },
        dropdownToHeader: {
            name: "name", strain: "strain", zone: "zone", status: "status", weight: "col1",
            day: () => currentView === "harvested" ? "col2"
                     : currentView === "dead"      ? "col1"
                     : ["col1", "col2"],
        },
        onChange: applyFilters,
    });

    // Set initial layout button state
    layoutButtons.forEach(b => {
        b.classList.toggle("active", b.dataset.layout === currentLayout);
    });
    syncLayoutVisibility();

    // ----- Fetch plants -----
    async function fetchPlants(view, checkIfEmpty = false) {
        cardsContainer.innerHTML = `<div class="text-center py-5" style="flex-basis:100%"><div class="spinner-border text-primary" role="status"></div></div>`;
        tableBody.innerHTML = `<tr><td colspan="7" class="text-center py-4"><div class="spinner-border text-primary" role="status"></div></td></tr>`;
        try {
            const res = await fetch(`/plants/${view}`);
            allPlants = await res.json() || [];

            // If this is the initial load and there are no living plants, switch to harvested
            if (checkIfEmpty && allPlants.length === 0 && view === "living") {
                currentView = "harvested";
                viewButtons.forEach(b => b.classList.remove("active"));
                viewButtons.forEach(b => {
                    if (b.dataset.view === "harvested") b.classList.add("active");
                });
                return fetchPlants("harvested", false);
            }

            updateHeaders(view);
            // updateHeaders rewrites the dynamic column labels, so redraw the sort arrows.
            sortableTable.refreshIcons();
            populateFilters();
            applyFilters();
        } catch (e) {
            cardsContainer.innerHTML = `<p class="text-danger text-center" style="flex-basis:100%">Failed to load plants.</p>`;
            tableBody.innerHTML = `<tr><td colspan="7" class="text-danger text-center">Failed to load plants.</td></tr>`;
        }
    }

    // ----- Update table headers based on view -----
    // A harvest weight that flips between grams and ounces when clicked
    // (weight-units.js does the flipping).
    function weightStat(grams) {
        if (!grams) return "—";
        const hint = esc(uiMessages.t("weight_toggle_hint", ""));
        return `<span class="isley-weight-toggle" role="button" tabindex="0" data-isley-weight data-grams="${grams}" data-unit="${weightUnit}" title="${hint}">${IsleyWeight.format(grams, weightUnit)}</span>`;
    }

    const sortIcon = ' <i class="fa-solid fa-sort ms-1 text-muted"></i>';
    function updateHeaders(view) {
        if (view === "harvested") {
            headerDynamic1.innerHTML = uiMessages.t("harvest_weight") + sortIcon;
            headerDynamic2.innerHTML = uiMessages.t("harvest_date") + sortIcon;
            headerDynamic2.style.display = "";
            if (headerActions) headerActions.style.display = "";
        } else if (view === "dead") {
            headerDynamic1.innerHTML = uiMessages.t("dead_date") + sortIcon;
            headerDynamic2.style.display = "none";
            if (headerActions) headerActions.style.display = "none";
        } else {
            headerDynamic1.innerHTML = uiMessages.t("current_week") + sortIcon;
            headerDynamic2.innerHTML = uiMessages.t("current_day") + sortIcon;
            headerDynamic2.style.display = "";
            if (headerActions) headerActions.style.display = "";
        }
    }

    // ----- Populate filter dropdowns -----
    function populateFilters() {
        // Zone filter — only relevant for living plants
        if (currentView === "living") {
            filterZoneGroup.style.display = "";
            populateFilter(filterZone, allPlants, "zone_name");
        } else {
            filterZoneGroup.style.display = "none";
            filterZone.value = "";
        }

        populateFilter(filterBreeder, allPlants, "breeder_name");
        populateFilter(filterStatus, allPlants, "status");

        // Hide sort options that only make sense in one view; fall back to Name A–Z if one is selected
        const viewOnlySorts = { "water-desc": "living", "weight-desc": "harvested", "weight-asc": "harvested" };
        Object.entries(viewOnlySorts).forEach(([value, view]) => {
            const opt = sortBy.querySelector(`option[value="${value}"]`);
            if (opt) opt.style.display = currentView === view ? "" : "none";
        });
        if (viewOnlySorts[sortBy.value] && viewOnlySorts[sortBy.value] !== currentView) {
            sortableTable.resetToDefault("name-asc");
        }
    }

    // ----- Filter + Sort -----
    function applyFilters() {
        let data = [...allPlants];
        const query = searchInput.value.toLowerCase().trim();
        const zone = filterZone.value;
        const breeder = filterBreeder.value;
        const status = filterStatus.value;

        data = list.filterByText(data, query, ["name", "strain_name", "breeder_name", "zone_name"]);
        if (zone) data = data.filter(p => p.zone_name === zone);
        if (breeder) data = data.filter(p => p.breeder_name === breeder);
        if (status) data = data.filter(p => p.status === status);

        // Use table header sort if active, otherwise use dropdown
        const { key: sortKey, asc: sortAsc } = sortableTable.getSort();

        data.sort((a, b) => {
            let va, vb;
            switch (sortKey) {
                case "name":
                    va = (a.name || "").toLowerCase();
                    vb = (b.name || "").toLowerCase();
                    return sortAsc ? va.localeCompare(vb) : vb.localeCompare(va);
                case "day":
                    if (currentView === "harvested" || currentView === "dead") {   // harvest / dead date
                        va = a.harvest_date || "";
                        vb = b.harvest_date || "";
                        return sortAsc ? (va < vb ? -1 : va > vb ? 1 : 0) : (vb < va ? -1 : vb > va ? 1 : 0);
                    }
                    va = a.current_day || 0;
                    vb = b.current_day || 0;
                    return sortAsc ? va - vb : vb - va;
                case "weight":
                    va = a.harvest_weight || 0;
                    vb = b.harvest_weight || 0;
                    return sortAsc ? va - vb : vb - va;
                case "col1":
                    if (currentView === "dead") {   // col1 is the dead date
                        va = a.harvest_date || "";
                        vb = b.harvest_date || "";
                        return sortAsc ? (va < vb ? -1 : va > vb ? 1 : 0) : (vb < va ? -1 : vb > va ? 1 : 0);
                    }
                    if (currentView === "harvested") {
                        va = a.harvest_weight || 0;
                        vb = b.harvest_weight || 0;
                    } else {
                        va = a.current_day || 0;
                        vb = b.current_day || 0;
                    }
                    return sortAsc ? va - vb : vb - va;
                case "col2":
                    if (currentView === "harvested") {
                        va = a.harvest_date || "";
                        vb = b.harvest_date || "";
                    } else {
                        va = a.current_day || 0;
                        vb = b.current_day || 0;
                    }
                    return sortAsc ? (va < vb ? -1 : va > vb ? 1 : 0) : (vb < va ? -1 : vb > va ? 1 : 0);
                case "strain":
                    va = (a.strain_name || "").toLowerCase();
                    vb = (b.strain_name || "").toLowerCase();
                    return sortAsc ? va.localeCompare(vb) : vb.localeCompare(va);
                case "zone":
                    va = (a.zone_name || "").toLowerCase();
                    vb = (b.zone_name || "").toLowerCase();
                    return sortAsc ? va.localeCompare(vb) : vb.localeCompare(va);
                case "status":
                    va = (a.status || "").toLowerCase();
                    vb = (b.status || "").toLowerCase();
                    return sortAsc ? va.localeCompare(vb) : vb.localeCompare(va);
                case "water":
                    va = a.days_since_last_watering ?? 999;
                    vb = b.days_since_last_watering ?? 999;
                    return sortAsc ? va - vb : vb - va;
                default:
                    return 0;
            }
        });

        resultCount.textContent = pluralize(data.length, "plant");

        if (data.length === 0) {
            cardsContainer.innerHTML = "";
            tableBody.innerHTML = "";
            emptyState.style.display = "flex";
            return;
        }
        emptyState.style.display = "none";
        renderCards(data);
        renderTable(data);
    }

    // ----- Status badge helper -----
    function statusBadge(status) {
        const cls = {
            "Germination": "pc-status-germ",
            "Seedling": "pc-status-seed",
            "Vegetative": "pc-status-veg",
            "Flowering": "pc-status-flower",
            "Drying": "pc-status-dry",
            "Curing": "pc-status-cure",
            "Success": "pc-status-harvest",
            "Dead": "pc-status-dead"
        }[status] || "pc-status-default";
        return `<span class="pc-status-badge ${cls}">${esc(status)}</span>`;
    }

    // ----- Render card grid -----
    function renderCards(data) {
        cardsContainer.innerHTML = data.map(p => {
            const dayInfo = currentView === "harvested"
                ? `<span class="pc-stat"><i class="fa-solid fa-weight-scale me-1"></i>${weightStat(p.harvest_weight)}</span>`
                : currentView === "dead"
                ? `<span class="pc-stat"><i class="fa-solid fa-calendar-xmark me-1"></i>${IsleyTime.formatDate(p.harvest_date) || "—"}</span>`
                : `<span class="pc-stat"><i class="fa-solid fa-calendar-day me-1"></i>Day ${p.current_day || "—"}</span>
                   <span class="pc-stat"><i class="fa-solid fa-clock me-1"></i>Week ${p.current_week || "—"}</span>`;

            const waterInfo = (currentView === "living" && p.days_since_last_watering != null && p.days_since_last_watering >= 0)
                ? `<span class="pc-water-indicator${p.days_since_last_watering > 2 ? " pc-water-warn" : ""}"><i class="fa-solid fa-droplet me-1"></i>${p.days_since_last_watering}d</span>`
                : "";

            const feedInfo = (currentView === "living" && p.days_since_last_feeding != null && p.days_since_last_feeding >= 0)
                ? `<span class="pc-feed-indicator${p.days_since_last_feeding > 5 ? " pc-feed-warn" : ""}"><i class="fa-solid fa-flask me-1"></i>${p.days_since_last_feeding}d</span>`
                : "";

            const typeBadge = p.autoflower
                ? `<span class="strain-badge-auto" style="font-size:0.65rem;padding:0.15em 0.5em"><i class="fa-solid fa-bolt me-1"></i>Auto</span>`
                : `<span class="strain-badge-photo" style="font-size:0.65rem;padding:0.15em 0.5em"><i class="fa-solid fa-sun me-1"></i>Photo</span>`;

            const cloneBadge = p.clone
                ? `<span class="pc-clone-badge"><i class="fa-solid fa-clone me-1"></i>Clone</span>`
                : "";

            const isTerminal = /^(dead|success|drying|curing)$/i.test(p.status);
            const flowerDays = (p.flowering_days && !isTerminal)
                ? `<span class="pc-stat pc-flower-days"><i class="fa-solid fa-fire me-1"></i>F${p.flowering_days}</span>`
                : "";

            const quickActions = (canEdit && currentView === "living") ? `
                <div class="pc-quick-actions">
                    <button type="button" class="pc-action-btn plant-status-edit" data-id="${p.id}" data-status-id="${statusIdByName.get(p.status) || ""}" data-name="${encodeURIComponent(p.name || "")}" title="${esc(uiMessages.t("change_status"))}">
                        <i class="fa-solid fa-pen-to-square"></i>
                    </button>
                    <button type="button" class="pc-action-btn plant-measurement-add" data-id="${p.id}" data-name="${encodeURIComponent(p.name || "")}" title="${esc(uiMessages.t("add_measurement"))}">
                        <i class="fa-solid fa-ruler-combined"></i>
                    </button>
                    <button type="button" class="pc-action-btn plant-activity-add" data-id="${p.id}" data-name="${encodeURIComponent(p.name || "")}" title="${esc(uiMessages.t("add_activity"))}">
                        <i class="fa-solid fa-clipboard-list"></i>
                    </button>
                </div>` : "";

            return `
            <div class="pc-card-link" data-href="/plant/${p.id}">
                <div class="pc-card">
                    <div class="pc-card-header">
                        <div class="pc-card-title-row">
                            <h2 class="h5 pc-name">${esc(p.name)}</h2>
                            ${quickActions}
                        </div>
                        <div class="pc-subtitle">
                            <span class="pc-strain">${esc(p.strain_name || "Unknown")}</span>
                            <span class="pc-breeder">${esc(p.breeder_name || "")}</span>
                        </div>
                    </div>
                    <div class="pc-card-body">
                        <div class="pc-info-row">
                            <span class="pc-zone"><i class="fa-solid fa-location-dot me-1"></i>${esc(p.zone_name || "—")}</span>
                            ${statusBadge(p.status)}
                        </div>
                        <div class="pc-info-row">
                            ${cloneBadge}
                            ${typeBadge}
                            ${flowerDays}
                        </div>
                    </div>
                    <div class="pc-card-footer">
                        <div class="pc-meta-left">
                            ${dayInfo}
                        </div>
                        <div class="pc-meta-right">
                            ${waterInfo}
                            ${feedInfo}
                        </div>
                    </div>
                </div>
            </div>`;
        }).join("");
    }

    // ----- Render table -----
    function renderTable(data) {
        tableBody.innerHTML = data.map(p => {
            let col1 = "", col2 = "";
            if (currentView === "harvested") {
                col1 = weightStat(p.harvest_weight);
                col2 = IsleyTime.formatDate(p.harvest_date) || "—";
            } else if (currentView === "dead") {
                col1 = IsleyTime.formatDate(p.harvest_date) || "—";
                col2 = "";
            } else {
                col1 = `Wk ${p.current_week || "—"}`;
                col2 = `Day ${p.current_day || "—"}`;
            }

            const waterBadge = (currentView === "living" && p.days_since_last_watering != null && p.days_since_last_watering >= 0)
                ? `<span class="pc-water-indicator${p.days_since_last_watering > 2 ? " pc-water-warn" : ""}" style="font-size:0.72rem;margin-left:0.5rem"><i class="fa-solid fa-droplet me-1"></i>${p.days_since_last_watering}d</span>`
                : "";

            let actionsHtml = "";
            if (canEdit && currentView === "living") {
                actionsHtml = `
                <td class="text-center">
                    <div class="d-inline-flex gap-1">
                        <button type="button" class="btn btn-sm btn-outline-secondary btn-icon-action plant-status-edit" data-id="${p.id}" data-status-id="${statusIdByName.get(p.status) || ""}" data-name="${encodeURIComponent(p.name || "")}">
                            <i class="fa-solid fa-pen-to-square"></i>
                        </button>
                        <button type="button" class="btn btn-sm btn-outline-primary btn-icon-action plant-measurement-add" data-id="${p.id}" data-name="${encodeURIComponent(p.name || "")}">
                            <i class="fa-solid fa-ruler-combined"></i>
                        </button>
                        <button type="button" class="btn btn-sm btn-outline-success btn-icon-action plant-activity-add" data-id="${p.id}" data-name="${encodeURIComponent(p.name || "")}">
                            <i class="fa-solid fa-clipboard-list"></i>
                        </button>
                    </div>
                </td>`;
            } else if (canEdit && currentView === "harvested") {
                actionsHtml = `
                <td class="text-center">
                    <button type="button" class="btn btn-sm btn-outline-secondary btn-icon-action plant-status-edit" data-id="${p.id}" data-status-id="${statusIdByName.get(p.status) || ""}" data-name="${encodeURIComponent(p.name || "")}">
                        <i class="fa-solid fa-pen-to-square"></i>
                    </button>
                </td>`;
            }

            return `<tr class="pt-row" data-href="/plant/${p.id}">
                <td>
                    <a href="/plant/${p.id}" class="pt-plant-name">${esc(p.name)}</a>
                    ${p.clone ? '<span class="pc-clone-badge ms-1" style="font-size:0.65rem">Clone</span>' : ""}
                    ${waterBadge}
                </td>
                <td class="pt-cell-strain">
                    ${esc(p.strain_name || "")}
                    ${p.strain_url ? ` <a href="${esc(p.strain_url)}" target="_blank" class="text-muted" style="font-size:0.75rem"><i class="fa-solid fa-arrow-up-right-from-square"></i></a>` : ""}
                    <div class="pt-short-desc">${esc(p.breeder_name || "")}</div>
                </td>
                <td>${esc(p.zone_name || "")}</td>
                <td>${statusBadge(p.status)}</td>
                <td>${col1}</td>
                <td${currentView === 'dead' ? ' style="display:none"' : ''}>${col2}</td>
                ${actionsHtml}
            </tr>`;
        }).join("");
    }

    // ----- Layout switching -----
    function syncLayoutVisibility() {
        if (currentLayout === "cards") {
            cardsContainer.style.display = "";
            tableWrap.style.display = "none";
        } else {
            cardsContainer.style.display = "none";
            tableWrap.style.display = "";
        }
    }

    layoutButtons.forEach(btn => {
        btn.addEventListener("click", () => {
            currentLayout = btn.dataset.layout;
            localStorage.setItem("plantsLayout", currentLayout);
            layoutButtons.forEach(b => b.classList.remove("active"));
            btn.classList.add("active");
            syncLayoutVisibility();
        });
    });

    // ----- Card click delegation -----
    cardsContainer.addEventListener("click", (e) => {
        // Quick action button clicks should not navigate
        if (e.target.closest(".pc-action-btn")) return;
        const card = e.target.closest(".pc-card-link");
        if (card && card.dataset.href) {
            window.location = card.dataset.href;
        }
    });

    // ----- Table row click delegation -----
    tableWrap.addEventListener("click", (e) => {
        if (e.target.closest(".btn-icon-action") || e.target.closest("a")) return;
        const row = e.target.closest(".pt-row");
        if (row && row.dataset.href) {
            window.location = row.dataset.href;
        }
    });

    // ----- Helpers -----
    // An instant as a datetime-local value in the instance zone.
    const formatDateTimeLocal = IsleyTime.toInput;

    // ----- Quick action modal handling -----
    const quickStatusModalEl = document.getElementById("quickStatusModal");
    const quickStatusForm = document.getElementById("quickStatusForm");
    const quickStatusPlantIdInput = document.getElementById("quickStatusPlantId");
    const quickStatusPlantNameInput = document.getElementById("quickStatusPlantName");
    const quickStatusSelect = document.getElementById("quickStatusSelect");
    const quickStatusDate = document.getElementById("quickStatusDate");
    const quickStatusModal = quickStatusModalEl && window.bootstrap ? new bootstrap.Modal(quickStatusModalEl) : null;

    const quickMeasurementModalEl = document.getElementById("quickMeasurementModal");
    const quickMeasurementForm = document.getElementById("quickMeasurementForm");
    const quickMeasurementPlantIdInput = document.getElementById("quickMeasurementPlantId");
    const quickMeasurementPlantNameInput = document.getElementById("quickMeasurementPlantName");
    const quickMeasurementModal = quickMeasurementModalEl && window.bootstrap ? new bootstrap.Modal(quickMeasurementModalEl) : null;

    const quickActivityModalEl = document.getElementById("quickActivityModal");
    const quickActivityForm = document.getElementById("quickActivityForm");
    const quickActivityPlantIdInput = document.getElementById("quickActivityPlantId");
    const quickActivityPlantNameInput = document.getElementById("quickActivityPlantName");
    const quickActivityModal = quickActivityModalEl && window.bootstrap ? new bootstrap.Modal(quickActivityModalEl) : null;

    // Delegate quick action handlers (works for both card and table views)
    document.addEventListener("click", (e) => {
        const statusBtn = e.target.closest(".plant-status-edit");
        if (statusBtn && quickStatusModal) {
            e.stopPropagation();
            quickStatusPlantIdInput.value = statusBtn.dataset.id;
            quickStatusPlantNameInput.value = decodeURIComponent(statusBtn.dataset.name || "");
            if (statusBtn.dataset.statusId) quickStatusSelect.value = statusBtn.dataset.statusId;
            quickStatusDate.value = formatDateTimeLocal(new Date());
            quickStatusModal.show();
            return;
        }

        const measBtn = e.target.closest(".plant-measurement-add");
        if (measBtn && quickMeasurementModal) {
            e.stopPropagation();
            quickMeasurementPlantIdInput.value = measBtn.dataset.id;
            quickMeasurementPlantNameInput.value = decodeURIComponent(measBtn.dataset.name || "");
            document.getElementById("quickMeasurementValue").value = "";
            document.getElementById("quickMeasurementDate").value = formatDateTimeLocal(new Date());
            quickMeasurementModal.show();
            return;
        }

        const actBtn = e.target.closest(".plant-activity-add");
        if (actBtn && quickActivityModal) {
            e.stopPropagation();
            quickActivityPlantIdInput.value = actBtn.dataset.id;
            quickActivityPlantNameInput.value = decodeURIComponent(actBtn.dataset.name || "");
            document.getElementById("quickActivityNote").value = "";
            document.getElementById("quickActivityDate").value = formatDateTimeLocal(new Date());
            quickActivityModal.show();
            return;
        }
    });

    // Quick status form submission
    if (canEdit && quickStatusForm) {
        quickStatusForm.addEventListener("submit", (e) => {
            e.preventDefault();
            const plantId = Number(quickStatusPlantIdInput.value);
            const statusId = Number(quickStatusSelect.value);
            const date = quickStatusDate.value;
            if (!plantId || !statusId) return;

            fetch("/plant/status", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ plant_id: plantId, status_id: statusId, date: date || formatDateTimeLocal(new Date()) }),
            })
            .then(r => { if (!r.ok) throw new Error(); return r.json(); })
            .then(() => { if (quickStatusModal) quickStatusModal.hide(); fetchPlants(currentView); })
            .catch(() => uiMessages.showToast(uiMessages.t("failed_to_change_status", "Failed to update status"), "danger"));
        });
    }

    // Quick measurement form submission
    if (canEdit && quickMeasurementForm) {
        quickMeasurementForm.addEventListener("submit", (e) => {
            e.preventDefault();
            const plantId = Number(quickMeasurementPlantIdInput.value);
            const metricId = Number(document.getElementById("quickMeasurementMetric").value);
            const value = Number(document.getElementById("quickMeasurementValue").value);
            const date = document.getElementById("quickMeasurementDate").value;
            if (!plantId || !metricId || Number.isNaN(value)) return;

            fetch("/plantMeasurement", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ plant_id: plantId, metric_id: metricId, value: value, date: date || formatDateTimeLocal(new Date()) }),
            })
            .then(r => { if (!r.ok) throw new Error(); return r.json(); })
            .then(() => { if (quickMeasurementModal) quickMeasurementModal.hide(); fetchPlants(currentView); })
            .catch(() => uiMessages.showToast(uiMessages.t("failed_to_add_measurement", "Failed to add measurement"), "danger"));
        });
    }

    // Quick activity form submission
    if (canEdit && quickActivityForm) {
        quickActivityForm.addEventListener("submit", (e) => {
            e.preventDefault();
            const plantId = Number(quickActivityPlantIdInput.value);
            const activityId = Number(document.getElementById("quickActivityName").value);
            const note = document.getElementById("quickActivityNote").value;
            const date = document.getElementById("quickActivityDate").value;
            if (!plantId || !activityId) return;

            fetch("/plantActivity", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify({ plant_id: plantId, activity_id: activityId, note: note, date: date || formatDateTimeLocal(new Date()) }),
            })
            .then(r => { if (!r.ok) throw new Error(); return r.json(); })
            .then(() => { if (quickActivityModal) quickActivityModal.hide(); fetchPlants(currentView); })
            .catch(() => uiMessages.showToast(uiMessages.t("failed_to_add_activity", "Failed to add activity"), "danger"));
        });
    }

    // ----- Event listeners -----
    searchInput.addEventListener("input", debounce(applyFilters, 200));
    filterZone.addEventListener("change", applyFilters);
    filterBreeder.addEventListener("change", applyFilters);
    filterStatus.addEventListener("change", applyFilters);

    clearBtn.addEventListener("click", () => {
        clearFilters({
            searchInput,
            selects: [filterZone, filterBreeder, filterStatus],
            sortableTable,
            defaultSort: "name-asc"
        });
        applyFilters();
    });

    viewButtons.forEach(btn => {
        btn.addEventListener("click", () => {
            currentView = btn.dataset.view;
            viewButtons.forEach(b => b.classList.remove("active"));
            btn.classList.add("active");
            sortableTable.resetToDefault(); // no forced value — let the water-desc/view logic below decide
            fetchPlants(currentView);
        });
    });

    // ----- Initial load -----
    // Column headers and button titles are translated, so wait for those.
    uiMessages.ready.then(() => fetchPlants(currentView, true));
});
