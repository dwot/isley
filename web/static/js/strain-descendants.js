/**
 * Strain Descendants Renderer
 * Fetches and displays strains that have the current strain as a parent.
 * Two-line layout: strain name + cross on top, breeder below.
 */
document.addEventListener("DOMContentLoaded", () => {
    const descendantsCard = document.getElementById("descendantsCard");
    const descendantsList = document.getElementById("descendantsList");

    if (!descendantsCard || !descendantsList || typeof currentStrainID === "undefined") return;

    fetch(`/strains/${currentStrainID}/descendants`)
        .then(res => res.json())
        .then(data => {
            if (!data || data.length === 0) return;

            descendantsCard.style.display = "block";

            const list = document.createElement("div");
            list.className = "descendants-list";

            data.forEach(strain => {
                const item = document.createElement("div");
                item.className = "descendant-item";

                // Line 1: icon + strain name + cross
                const topLine = document.createElement("div");
                topLine.className = "descendant-top";

                const icon = document.createElement("i");
                icon.className = "fa-solid fa-seedling fa-xs descendant-icon";
                topLine.appendChild(icon);

                const link = document.createElement("a");
                link.href = `/strain/${strain.id}`;
                link.className = "lineage-link-exists";
                link.textContent = strain.name;
                topLine.appendChild(link);

                if (strain.other_parents) {
                    const cross = document.createElement("span");
                    cross.className = "descendant-cross";
                    cross.textContent = `${currentStrainName} x ${strain.other_parents}`;
                    topLine.appendChild(cross);
                }

                item.appendChild(topLine);

                // Line 2: breeder
                if (strain.breeder) {
                    const breederLine = document.createElement("div");
                    breederLine.className = "descendant-breeder";
                    breederLine.textContent = strain.breeder;
                    item.appendChild(breederLine);
                }

                list.appendChild(item);
            });

            descendantsList.appendChild(list);
        })
        .catch(err => {
            console.error("Failed to load descendants:", err);
        });
});
