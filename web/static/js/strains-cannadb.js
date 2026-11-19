(function () {
    const form = document.getElementById("cannadbSearchForm");
    const input = document.getElementById("cannadbSearchInput");
    const results = document.getElementById("cannadbResults");
    const status = document.getElementById("cannadbSearchStatus");
    if (!form) return;

    const lcl = {
        get searching() { return uiMessages.t("cannadb_searching"); },
        get noResults() { return uiMessages.t("cannadb_no_results"); },
        get searchFailed() { return uiMessages.t("api_cannadb_search_failed"); },
        get importBtn() { return uiMessages.t("cannadb_import_action"); },
        get importing() { return uiMessages.t("cannadb_importing"); },
        get imported() { return uiMessages.t("api_cannadb_imported"); },
        get importFailed() { return uiMessages.t("api_cannadb_import_failed"); },
        get match() { return uiMessages.t("cannadb_match"); },
    };

    function esc(s) {
        const d = document.createElement("div");
        d.textContent = s == null ? "" : String(s);
        return d.innerHTML;
    }

    function renderResults(rows) {
        results.innerHTML = "";
        if (!rows || rows.length === 0) {
            status.textContent = lcl.noResults;
            return;
        }
        status.textContent = "";
        rows.forEach((r) => {
            const item = document.createElement("div");
            item.className = "list-group-item";
            const pct = Math.round((r.similarity || 0) * 100);
            item.innerHTML =
                '<div class="d-flex justify-content-between align-items-start gap-3">' +
                    '<div class="flex-grow-1">' +
                        '<div class="fw-semibold">' + esc(r.name) +
                            (r.breeder_name ? ' <span class="text-muted fw-normal">— ' + esc(r.breeder_name) + '</span>' : '') +
                        '</div>' +
                        (r.description ? '<div class="small text-muted">' + esc(r.description) + '</div>' : '') +
                        '<div class="small text-secondary mt-1">' + lcl.match + ': ' + pct + '%</div>' +
                    '</div>' +
                    '<button type="button" class="btn btn-sm btn-success flex-shrink-0">' + esc(lcl.importBtn) + '</button>' +
                '</div>';
            const btn = item.querySelector("button");
            btn.addEventListener("click", () => importStrain(r.uri, btn));
            results.appendChild(item);
        });
    }

    function importStrain(uri, btn) {
        btn.disabled = true;
        btn.textContent = lcl.importing;
        fetch("/strains/cannadb/import", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ uri: uri }),
        })
            .then((resp) => resp.json().then((data) => ({ ok: resp.ok, data })))
            .then(({ ok, data }) => {
                if (!ok) throw new Error((data && (data.message || data.error)) || lcl.importFailed);
                if (window.uiMessages) uiMessages.showToast((data && data.message) || lcl.imported, "success");
                btn.classList.remove("btn-success");
                btn.classList.add("btn-outline-secondary");
                btn.innerHTML = '<i class="fa-solid fa-check me-1"></i>' + esc(lcl.imported);
                if (typeof fetchStrains === "function") fetchStrains(typeof currentView !== "undefined" ? currentView : "in-stock");
            })
            .catch((err) => {
                if (window.uiMessages) uiMessages.showToast(err.message || lcl.importFailed, "danger");
                btn.disabled = false;
                btn.textContent = lcl.importBtn;
            });
    }

    form.addEventListener("submit", (e) => {
        e.preventDefault();
        const q = input.value.trim();
        if (!q) return;
        results.innerHTML = "";
        status.textContent = lcl.searching;
        fetch("/strains/cannadb/search?q=" + encodeURIComponent(q) + "&limit=10")
            .then((resp) => resp.json().then((data) => ({ ok: resp.ok, data })))
            .then(({ ok, data }) => {
                if (!ok) throw new Error((data && (data.message || data.error)) || lcl.searchFailed);
                renderResults(data.results || []);
            })
            .catch((err) => {
                status.textContent = err.message || lcl.searchFailed;
            });
    });
})();
