// Strain page tasting card. Hidden entirely when the strain has no reports —
// same convention as the descendants card above it.
document.addEventListener("DOMContentLoaded", () => {
    const card = document.getElementById("smokeSummaryCard");
    if (!card || typeof currentStrainID === "undefined") return;

    function stars(value) {
        const n = Math.max(0, Math.min(5, Math.round(Number(value) || 0)));
        return "★".repeat(n) + "☆".repeat(5 - n);
    }

    function escapeHTML(s) {
        const div = document.createElement("div");
        div.textContent = s;
        return div.innerHTML;
    }

    fetch("/strains/" + currentStrainID + "/smoke-summary")
        .then((r) => r.json())
        .then((summary) => {
            if (!summary || !summary.count) return;

            document.getElementById("smokeSummaryStars").textContent =
                stars(summary.average) + "  " + summary.average.toFixed(1) + " / 5";

            const template = uiMessages.t("smoke_summary_caption") || "average over {n} harvests";
            document.getElementById("smokeSummaryCaption").textContent =
                template.replace("{n}", summary.count);

            document.getElementById("smokeSummaryList").innerHTML = summary.entries
                .map((e) =>
                    '<li>' +
                    '  <span class="strain-info-label">' +
                    '    <a href="/plant/' + e.plant_id + '">' + escapeHTML(e.plant_name) + '</a>' +
                    '  </span>' +
                    '  <span class="strain-info-value">' + stars(e.rating) + " " + e.rating.toFixed(1) +
                    '    <small class="text-muted ms-1">' + escapeHTML(e.report_date.slice(0, 10)) + '</small>' +
                    '  </span>' +
                    '</li>')
                .join("");

            card.style.display = "";
        })
        .catch((err) => console.error("smoke summary:", err));
});
