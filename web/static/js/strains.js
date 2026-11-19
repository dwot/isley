document.addEventListener("DOMContentLoaded", () => {
    const cardsContainer = document.getElementById("strainsCards");
    const tableWrap = document.getElementById("strainsTable");
    const tableBody = document.getElementById("strainsTableBody");
    const emptyState = document.getElementById("emptyState");
    const searchInput = document.getElementById("searchStrains");
    const filterBreeder = document.getElementById("filterBreeder");
    const filterType = document.getElementById("filterType");
    const filterFeminized = document.getElementById("filterFeminized");
    const sortBy = document.getElementById("sortBy");
    const clearBtn = document.getElementById("clearFilters");
    const resultCount = document.getElementById("resultCount");
    const viewButtons = document.querySelectorAll(".view-button");
    const layoutButtons = document.querySelectorAll(".layout-toggle");

    // ----- Table header sort click -----
    const sortableTable = new IsleySortableTable(sortBy, {
        prefix: "st",
        headerToDropdown: {
            name:    { asc: "name-asc", desc: "name-desc" },
            breeder: { asc: "breeder-asc", desc: "breeder-desc" },
            indica:  { asc: "sativa-desc", desc: "indica-desc" },
            seeds:   { asc: "seeds-asc", desc: "seeds-desc" },
            feminized: { asc: "feminized-asc", desc: "feminized-desc" },
        },
        dropdownToHeader: { name: "name", breeder: "breeder", indica: "indica", sativa: { key: "indica", invert: true }, seeds: "seeds", feminized: "feminized" },
        onChange: applyFilters,
    });

    let currentView = "in-stock";
    let currentLayout = localStorage.getItem("strainsLayout") || "cards";
    let allStrains = [];

    // Delegated click handler for strain table rows (replaces inline onclick)
    tableBody.addEventListener("click", (e) => {
        const row = e.target.closest("tr[data-strain-id]");
        if (row) window.location = "/strain/" + row.dataset.strainId;
    });

    // Set initial layout button state
    layoutButtons.forEach(b => {
        b.classList.toggle("active", b.dataset.layout === currentLayout);
    });
    syncLayoutVisibility();

    // ----- Fetch strains -----
    async function fetchStrains(view) {
        cardsContainer.innerHTML = `<div class="text-center py-5" style="flex-basis:100%"><div class="spinner-border text-primary" role="status"></div></div>`;
        tableBody.innerHTML = `<tr><td colspan="7" class="text-center py-4"><div class="spinner-border text-primary" role="status"></div></td></tr>`;
        try {
            const res = await fetch(`/strains/${view}`);
            allStrains = await res.json() || [];
            populateBreederFilter();
            applyFilters();
        } catch (e) {
            cardsContainer.innerHTML = `<p class="text-danger text-center" style="flex-basis:100%">Failed to load strains.</p>`;
            tableBody.innerHTML = `<tr><td colspan="6" class="text-danger text-center">Failed to load strains.</td></tr>`;
        }
    }

    // ----- Populate breeder dropdown -----
    function populateBreederFilter() {
        populateFilter(filterBreeder, allStrains, "breeder");
    }

    // ----- Filter + Sort -----
    function applyFilters() {
        let data = [...allStrains];
        const query = searchInput.value.toLowerCase().trim();
        const breeder = filterBreeder.value;
        const type = filterType.value;
        const feminized = filterFeminized.value;

        data = list.filterByText(data, query, ["name", "breeder", "short_desc", "lineage"]);
        if (breeder) {
            data = data.filter(s => s.breeder === breeder);
        }
        if (type === "auto") {
            data = data.filter(s => s.autoflower);
        } else if (type === "photo") {
            data = data.filter(s => !s.autoflower);
        }
        if (feminized === "feminized") {
            data = data.filter(s => s.feminized);
        } else if (feminized === "regular") {
            data = data.filter(s => !s.feminized);
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
                case "breeder":
                    va = (a.breeder || "").toLowerCase();
                    vb = (b.breeder || "").toLowerCase();
                    return asc ? va.localeCompare(vb) : vb.localeCompare(va);
                case "indica":
                    return asc ? a.indica - b.indica : b.indica - a.indica;
                case "sativa":
                    return asc ? a.sativa - b.sativa : b.sativa - a.sativa;
                case "seeds":
                    return asc ? a.seed_count - b.seed_count : b.seed_count - a.seed_count;
                case "feminized":
                    if (a.feminized === b.feminized) return 0;
                    return asc ? (a.feminized ? 1 : -1) : (a.feminized ? -1 : 1);
                default:
                    return 0;
            }
        });

        resultCount.textContent = pluralize(data.length, "strain");

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

    function renderBadges(s) {
        const typeBadge = s.autoflower
            ? `<span class="strain-badge-auto"><i class="fa-solid fa-bolt me-1"></i>Auto</span>`
            : `<span class="strain-badge-photo"><i class="fa-solid fa-sun me-1"></i>Photo</span>`;
        const femBadge = s.feminized
            ? `<span class="strain-badge-feminized"><i class="fa-solid fa-venus me-1"></i>Fem</span>`
            : `<span class="strain-badge-regular"><i class="fa-solid fa-venus-mars me-1"></i>Reg</span>`;
        return { typeBadge, femBadge };
    }

    // ----- Render card grid -----
    function renderCards(data) {
        cardsContainer.innerHTML = data.map(s => {
            const { typeBadge, femBadge } = renderBadges(s);

            const dominance = s.indica > s.sativa ? "Indica" : s.sativa > s.indica ? "Sativa" : "Hybrid";

            const seedBadge = s.seed_count > 0
                ? `<span class="badge bg-success">${s.seed_count}</span>`
                : `<span class="badge bg-secondary">0</span>`;

            const lineageHtml = s.lineage
                ? `<div class="sc-lineage"><i class="fa-solid fa-sitemap me-1"></i>${esc(s.lineage)}</div>`
                : "";

            const shortDesc = s.short_desc
                ? `<p class="sc-desc">${esc(s.short_desc)}</p>`
                : "";

            const linkIcon = s.url
                ? `<span class="sc-ext-link" data-href="${esc(s.url)}" title="Breeder page"><i class="fa-solid fa-arrow-up-right-from-square"></i></span>`
                : "";

            return `
            <div class="sc-card-link" data-href="/strain/${s.id}">
                <div class="sc-card">
                    <div class="sc-card-header">
                        <div class="sc-card-title-row">
                            <h2 class="h5 sc-name">${esc(s.name)}</h2>
                            ${linkIcon}
                        </div>
                        <span class="sc-breeder">${esc(s.breeder || "Unknown")}</span>
                    </div>
                    <div class="sc-card-body">
                        ${shortDesc}
                        ${lineageHtml}
                    </div>
                    <div class="sc-card-footer">
                        <div class="sc-meta-left">
                            ${typeBadge}
                            ${femBadge}
                            <span class="sc-dominance">${dominance} ${s.indica}/${s.sativa}</span>
                        </div>
                        <div class="sc-ratio-mini">
                            <div class="sc-ratio-mini-fill" style="width:${s.indica}%"></div>
                        </div>
                        <div class="sc-meta-right">
                            <i class="fa-solid fa-seedling me-1"></i>${seedBadge}
                        </div>
                    </div>
                </div>
            </div>`;
        }).join("");
    }

    // ----- Render table -----
    function renderTable(data) {
        tableBody.innerHTML = data.map(s => {
            const { typeBadge, femBadge } = renderBadges(s);

            const seedBadge = s.seed_count > 0
                ? `<span class="badge bg-success">${s.seed_count}</span>`
                : `<span class="badge bg-secondary">0</span>`;

            const extLink = s.url
                ? ` <a href="${esc(s.url)}" target="_blank" class="text-muted" style="font-size:0.75rem"><i class="fa-solid fa-arrow-up-right-from-square"></i></a>`
                : "";

            return `<tr class="st-row" data-strain-id="${s.id}">
                <td>
                    <a href="/strain/${s.id}" class="st-strain-name">${esc(s.name)}</a>${extLink}
                    ${s.short_desc ? `<div class="st-short-desc">${esc(s.short_desc)}</div>` : ""}
                </td>
                <td class="st-cell-breeder">${esc(s.breeder || "")}</td>
                <td class="st-cell-lineage">${s.lineage ? esc(s.lineage) : '<span class="text-muted">—</span>'}</td>
                <td class="text-center text-nowrap">${typeBadge}</td>
                <td class="text-center text-nowrap">${femBadge}</td>
                <td>
                    <div class="st-ratio-wrap">
                        <div class="sc-ratio-mini" style="width:60px">
                            <div class="sc-ratio-mini-fill" style="width:${s.indica}%"></div>
                        </div>
                        <span class="st-ratio-label">${s.indica}/${s.sativa}</span>
                    </div>
                </td>
                <td class="text-center">${seedBadge}</td>
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
            localStorage.setItem("strainsLayout", currentLayout);
            layoutButtons.forEach(b => b.classList.remove("active"));
            btn.classList.add("active");
            syncLayoutVisibility();
        });
    });

    // ----- Card click delegation -----
    cardsContainer.addEventListener("click", (e) => {
        // External link click
        const extLink = e.target.closest(".sc-ext-link");
        if (extLink) {
            e.stopPropagation();
            window.open(extLink.dataset.href, "_blank");
            return;
        }
        // Card click → navigate to strain
        const card = e.target.closest(".sc-card-link");
        if (card && card.dataset.href) {
            window.location = card.dataset.href;
        }
    });

    // ----- Event listeners -----
    searchInput.addEventListener("input", debounce(applyFilters, 200));
    filterBreeder.addEventListener("change", applyFilters);
    filterType.addEventListener("change", applyFilters);
    filterFeminized.addEventListener("change", applyFilters);

    clearBtn.addEventListener("click", () => {
        clearFilters({
            searchInput,
            selects: [filterBreeder, filterType, filterFeminized],
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
            sortableTable.resetToDefault();
            fetchStrains(currentView);
        });
    });

    // ----- Initial load -----
    fetchStrains(currentView);
});
