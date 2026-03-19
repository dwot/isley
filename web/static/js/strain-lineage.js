/**
 * Strain Lineage Tree Renderer
 * Renders a collapsible ancestry tree with Wikipedia-style blue/red links.
 * Blue link = strain exists in the database (clickable)
 * Red link = strain not yet added (links to add-strain flow)
 */
document.addEventListener("DOMContentLoaded", () => {
    const treeContainer = document.getElementById("lineageTree");
    const emptyMsg = document.getElementById("lineageEmpty");
    const lineageCard = document.getElementById("lineageCard");

    if (!treeContainer || !lineageCard || typeof currentStrainID === "undefined") return;

    // Fetch lineage data via API
    fetch(`/strains/${currentStrainID}/lineage`)
        .then(res => res.json())
        .then(lineageData => {
            if (!lineageData || lineageData.length === 0) {
                emptyMsg.style.display = "block";
                return;
            }
            const tree = buildTree(lineageData);
            treeContainer.appendChild(tree);
        })
        .catch(err => {
            console.error("Failed to load lineage:", err);
            emptyMsg.style.display = "block";
        });
});

function buildTree(parents) {
    const ul = document.createElement("ul");
    ul.className = "lineage-tree";

    parents.forEach(parent => {
        const li = document.createElement("li");
        li.className = "lineage-node";

        const nameSpan = document.createElement("span");
        nameSpan.className = "lineage-name";

        if (parent.parent_strain_id) {
            // Blue link - strain exists
            const link = document.createElement("a");
            link.href = `/strain/${parent.parent_strain_id}`;
            link.className = "lineage-link-exists";
            link.textContent = parent.parent_name;
            nameSpan.appendChild(link);
        } else {
            // Red link - strain doesn't exist yet, click to add
            const link = document.createElement("a");
            link.href = `/strain/new?name=${encodeURIComponent(parent.parent_name)}`;
            link.className = "lineage-link-missing";
            link.title = `${parent.parent_name} — click to add as a new strain`;
            link.textContent = parent.parent_name;
            nameSpan.appendChild(link);
        }

        li.appendChild(nameSpan);

        // Recursively add children (grandparents, etc.)
        if (parent.children && parent.children.length > 0) {
            const toggle = document.createElement("button");
            toggle.className = "lineage-toggle";
            toggle.innerHTML = '<i class="fa-solid fa-chevron-down fa-xs"></i>';
            toggle.title = "Show ancestry";
            nameSpan.insertBefore(toggle, nameSpan.firstChild);

            const childTree = buildTree(parent.children);
            childTree.className += " lineage-subtree";
            li.appendChild(childTree);

            toggle.addEventListener("click", () => {
                const isCollapsed = childTree.classList.toggle("collapsed");
                toggle.innerHTML = isCollapsed
                    ? '<i class="fa-solid fa-chevron-right fa-xs"></i>'
                    : '<i class="fa-solid fa-chevron-down fa-xs"></i>';
            });
        }

        ul.appendChild(li);
    });

    return ul;
}
