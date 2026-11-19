// activities.js — the activity log page.
//
// The server filters, sorts and pages the log (/activities/list); this
// script turns the controls into that request and draws the page of rows
// that comes back. Nothing is filtered here, so the page costs the same
// with a hundred activities or a hundred thousand.
document.addEventListener("DOMContentLoaded", () => {
    const tableBody = document.getElementById("activitiesTableBody");
    const emptyState = document.getElementById("emptyState");
    const searchInput = document.getElementById("searchActivities");
    const filterPlant = document.getElementById("filterPlant");
    const filterActivity = document.getElementById("filterActivity");
    const filterZone = document.getElementById("filterZone");
    const filterFrom = document.getElementById("filterFrom");
    const filterTo = document.getElementById("filterTo");
    const clearBtn = document.getElementById("clearFilters");
    const resultCount = document.getElementById("resultCount");
    const exportCsv = document.getElementById("exportCsv");
    const exportXlsx = document.getElementById("exportXlsx");

    const PAGE_SIZE = 50;
    let currentPage = 1;
    let shown = [];   // the rows on screen, for the edit dialog
    let latest = 0;   // the newest request; an older answer arriving late is dropped

    const pager = list.pager(document.getElementById("activitiesPager"), {
        pageSize: PAGE_SIZE,
        onChange: (page) => {
            load(page).then(() => document.getElementById("activitiesTable").scrollIntoView({ block: "start" }));
        },
    });

    // Table header sorting: a change of sort starts again from page 1.
    const sortableTable = new IsleySortableTable(null, { prefix: "al", onChange: () => load(1) });

    // The server's name for the sort the headers show. Newest first when
    // no header has been clicked.
    function orderParam() {
        const { key, asc } = sortableTable.getSort();
        switch (key) {
            case "date": return asc ? "date_asc" : "";
            case "plant": return asc ? "plant" : "plant_desc";
            case "activity": return asc ? "activity" : "activity_desc";
            default: return "";
        }
    }

    // The filters as query parameters, shared by the list and the exports.
    function filterParams() {
        const params = new URLSearchParams();
        const search = searchInput.value.trim();
        if (search) params.set("search", search);
        if (filterPlant.value) params.set("plant_id", filterPlant.value);
        if (filterActivity.value) params.set("activity_id", filterActivity.value);
        if (filterZone.value) params.set("zone_id", filterZone.value);
        if (filterFrom.value) params.set("from", filterFrom.value);
        if (filterTo.value) params.set("to", filterTo.value);
        const order = orderParam();
        if (order) params.set("order", order);
        return params;
    }

    async function load(page) {
        const request = ++latest;
        const params = filterParams();
        updateExportLinks(params);
        params.set("page", page);
        params.set("page_size", PAGE_SIZE);
        if (!shown.length) {
            tableBody.innerHTML = `<tr><td colspan="6" class="text-center py-4"><div class="spinner-border text-primary" role="status"></div></td></tr>`;
        }
        try {
            const res = await fetch(`/activities/list?${params}`);
            if (!res.ok) throw new Error(res.statusText);
            const data = await res.json();
            if (request !== latest) return;
            shown = data.entries || [];
            currentPage = data.page || 1;
            pager.set(data.total || 0, currentPage);
            resultCount.textContent = pluralize(data.total || 0, "activity", "activities");
            emptyState.style.display = shown.length ? "none" : "flex";
            renderTable(shown);
        } catch (e) {
            if (request !== latest) return;
            shown = [];
            pager.set(0, 1);
            resultCount.textContent = "";
            emptyState.style.display = "none";
            tableBody.innerHTML = `<tr><td colspan="6" class="text-danger text-center">${esc(uiMessages.t("activity_log_load_failed", "Failed to load activities."))}</td></tr>`;
        }
    }

    function renderTable(rows) {
        tableBody.innerHTML = rows.map((a, i) => `
            <tr class="al-row" data-index="${i}">
                <td class="al-cell-date">${IsleyTime.formatDateTime(a.date)}</td>
                <td class="al-cell-plant">
                    <a href="/plant/${a.plant_id}" class="text-decoration-none">
                        ${esc(a.plant_name)}
                    </a>
                </td>
                <td class="al-cell-strain">${esc(a.strain_name || "")}</td>
                <td class="al-cell-zone">${esc(a.zone_name || "")}</td>
                <td class="al-cell-activity">${esc(a.activity_name)}</td>
                <td class="al-cell-note">${esc(a.note || "")}</td>
            </tr>
        `).join("");
    }

    // The exports take the same filters and sort, and every matching row
    // rather than one page.
    function updateExportLinks(params) {
        if (!exportCsv || !exportXlsx) return; // guests can't export
        const query = params.toString();
        exportCsv.href = `/activities/export/csv${query ? "?" + query : ""}`;
        exportXlsx.href = `/activities/export/xlsx${query ? "?" + query : ""}`;
    }

    // A click on a row opens the edit dialog, for those who have one.
    document.getElementById("activitiesTable").addEventListener("click", (e) => {
        const row = e.target.closest(".al-row");
        if (!row || e.target.closest("a")) return;
        const activity = shown[Number(row.dataset.index)];
        const modal = document.getElementById("activityEditModal");
        if (activity && modal && typeof showActivityModal === "function") {
            showActivityModal(activity);
        }
    });

    clearBtn.addEventListener("click", () => {
        clearFilters({
            searchInput,
            selects: [filterPlant, filterActivity, filterZone],
            inputs: [filterFrom, filterTo],
            onClear: () => sortableTable.resetToDefault()
        });
        load(1);
    });

    searchInput.addEventListener("input", debounce(() => load(1), 250));
    [filterPlant, filterActivity, filterZone, filterFrom, filterTo].forEach((el) => {
        el.addEventListener("change", () => load(1));
    });

    // The pager's labels are translated, so wait for those.
    uiMessages.ready.then(() => load(1));
});
