/**
 * Strain Lineage Editor
 * Manages parent strain entries with autocomplete search against existing strains.
 * Wikipedia-style: matched strains get a blue link, unmatched get a red link.
 */
document.addEventListener("DOMContentLoaded", () => {
    const entriesContainer = document.getElementById("lineageEntries");
    const addParentBtn = document.getElementById("addParentBtn");

    if (!entriesContainer || !addParentBtn || typeof currentStrainID === "undefined") return;

    let allStrains = [];

    // Load strains list and existing lineage in parallel
    Promise.all([
        fetch("/strains/in-stock").then(r => r.json()).catch(() => []),
        fetch("/strains/out-of-stock").then(r => r.json()).catch(() => []),
        fetch(`/strains/${currentStrainID}/lineage`).then(r => r.json()).catch(() => [])
    ]).then(([inStock, outOfStock, lineage]) => {
        allStrains = [...(inStock || []), ...(outOfStock || [])];

        // Populate existing lineage entries
        if (lineage && lineage.length > 0) {
            lineage.forEach(entry => {
                addParentRow(entry.parent_name, entry.parent_strain_id);
            });
        }
    });

    addParentBtn.addEventListener("click", () => {
        addParentRow("", null);
    });

    // Expose function for strain-edit.js to call before redirect
    window.saveLineage = function() {
        const parents = collectParents();
        return fetch(`/strains/${currentStrainID}/lineage`, {
            method: "PUT",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ parents: parents })
        });
    };

    function collectParents() {
        const rows = entriesContainer.querySelectorAll(".lineage-entry-row");
        const parents = [];
        rows.forEach(row => {
            const nameInput = row.querySelector(".lineage-parent-name");
            const strainIdInput = row.querySelector(".lineage-parent-strain-id");
            const name = nameInput ? nameInput.value.trim() : "";
            const strainId = strainIdInput ? strainIdInput.value : "";
            if (name) {
                parents.push({
                    parent_name: name,
                    parent_strain_id: strainId ? parseInt(strainId, 10) : null
                });
            }
        });
        return parents;
    }

    function addParentRow(name, strainId) {
        const row = document.createElement("div");
        row.className = "lineage-entry-row d-flex align-items-center gap-2 mb-2";

        const inputWrapper = document.createElement("div");
        inputWrapper.className = "flex-grow-1 position-relative";

        const nameInput = document.createElement("input");
        nameInput.type = "text";
        nameInput.className = "form-control form-control-sm lineage-parent-name";
        nameInput.placeholder = "Parent strain name...";
        nameInput.value = name || "";
        nameInput.autocomplete = "off";

        const strainIdInput = document.createElement("input");
        strainIdInput.type = "hidden";
        strainIdInput.className = "lineage-parent-strain-id";
        strainIdInput.value = strainId || "";

        // Match indicator
        const matchBadge = document.createElement("span");
        matchBadge.className = "lineage-match-badge ms-2";
        updateMatchBadge(matchBadge, strainId);

        // Autocomplete dropdown
        const dropdown = document.createElement("div");
        dropdown.className = "lineage-autocomplete-dropdown";

        inputWrapper.appendChild(nameInput);
        inputWrapper.appendChild(strainIdInput);
        inputWrapper.appendChild(dropdown);

        const removeBtn = document.createElement("button");
        removeBtn.type = "button";
        removeBtn.className = "btn btn-sm btn-outline-danger";
        removeBtn.innerHTML = '<i class="fa-solid fa-xmark"></i>';
        removeBtn.addEventListener("click", () => row.remove());

        row.appendChild(inputWrapper);
        row.appendChild(matchBadge);
        row.appendChild(removeBtn);
        entriesContainer.appendChild(row);

        // Autocomplete logic
        let debounceTimer = null;
        nameInput.addEventListener("input", () => {
            clearTimeout(debounceTimer);
            const query = nameInput.value.trim();

            if (query.length < 2) {
                dropdown.innerHTML = "";
                dropdown.style.display = "none";
                strainIdInput.value = "";
                updateMatchBadge(matchBadge, null);
                return;
            }

            debounceTimer = setTimeout(() => {
                const matches = searchLocalStrains(query);
                renderDropdown(dropdown, matches, nameInput, strainIdInput, matchBadge);
            }, 150);
        });

        nameInput.addEventListener("blur", () => {
            setTimeout(() => {
                dropdown.style.display = "none";
            }, 200);
        });

        nameInput.addEventListener("focus", () => {
            const query = nameInput.value.trim();
            if (query.length >= 2) {
                const matches = searchLocalStrains(query);
                renderDropdown(dropdown, matches, nameInput, strainIdInput, matchBadge);
            }
        });
    }

    function searchLocalStrains(query) {
        if (!allStrains || allStrains.length === 0) return [];
        const lower = query.toLowerCase();
        return allStrains.filter(s =>
            s.name.toLowerCase().includes(lower) && s.id !== currentStrainID
        ).slice(0, 8);
    }

    function renderDropdown(dropdown, matches, nameInput, strainIdInput, matchBadge) {
        dropdown.innerHTML = "";
        if (matches.length === 0) {
            dropdown.style.display = "none";
            return;
        }

        matches.forEach(strain => {
            const item = document.createElement("div");
            item.className = "lineage-autocomplete-item";
            item.innerHTML = `<strong>${escapeHtml(strain.name)}</strong> <small class="text-muted">${escapeHtml(strain.breeder || "")}</small>`;
            item.addEventListener("mousedown", (e) => {
                e.preventDefault();
                nameInput.value = strain.name;
                strainIdInput.value = strain.id;
                updateMatchBadge(matchBadge, strain.id);
                dropdown.style.display = "none";
            });
            dropdown.appendChild(item);
        });

        dropdown.style.display = "block";
    }

    function updateMatchBadge(badge, strainId) {
        if (strainId) {
            badge.innerHTML = '<i class="fa-solid fa-link fa-xs"></i>';
            badge.className = "lineage-match-badge lineage-matched";
            badge.title = "Linked to existing strain";
        } else {
            badge.innerHTML = '<i class="fa-solid fa-link-slash fa-xs"></i>';
            badge.className = "lineage-match-badge lineage-unmatched";
            badge.title = "Not linked — will appear as a red link";
        }
    }

    function escapeHtml(str) {
        const div = document.createElement("div");
        div.textContent = str;
        return div.innerHTML;
    }
});
