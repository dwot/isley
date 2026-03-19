document.addEventListener("DOMContentLoaded", () => {
    const form = document.getElementById("addStrainForm");
    const editBreederSelect = document.getElementById("editBreederSelect");
    const editNewBreederInput = document.getElementById("editNewBreederInput");
    const editIndicaSativaSlider = document.getElementById("editIndicaSativaSlider");
    const editIndicaLabel = document.getElementById("editIndicaLabel");
    const editSativaLabel = document.getElementById("editSativaLabel");
    const editRatioFill = document.getElementById("editRatioFill");
    const descriptionTextarea = document.getElementById("editStrainDescription");
    const markdownPreview = document.getElementById("markdownPreview");

    // --- Breeder select: show/hide new breeder input ---
    if (editBreederSelect) {
        editBreederSelect.addEventListener("change", () => {
            if (editBreederSelect.value === "new") {
                editNewBreederInput.classList.remove("d-none");
            } else {
                editNewBreederInput.classList.add("d-none");
            }
        });
    }

    // --- Indica/Sativa slider with live ratio bar preview ---
    if (editIndicaSativaSlider) {
        editIndicaSativaSlider.addEventListener("input", () => {
            const indica = editIndicaSativaSlider.value;
            const sativa = 100 - indica;
            if (editIndicaLabel) editIndicaLabel.textContent = `Indica: ${indica}%`;
            if (editSativaLabel) editSativaLabel.textContent = `Sativa: ${sativa}%`;
            if (editRatioFill) editRatioFill.style.width = `${indica}%`;
        });
    }

    // --- Markdown live preview (debounced) ---
    let previewTimeout = null;
    if (descriptionTextarea && markdownPreview) {
        descriptionTextarea.addEventListener("input", () => {
            clearTimeout(previewTimeout);
            previewTimeout = setTimeout(() => {
                renderMarkdownPreview(descriptionTextarea.value);
            }, 300);
        });
    }

    function renderMarkdownPreview(md) {
        let html = md;
        html = html.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
        html = html.replace(/^######\s+(.+)$/gm, '<h6>$1</h6>');
        html = html.replace(/^#####\s+(.+)$/gm, '<h5>$1</h5>');
        html = html.replace(/^####\s+(.+)$/gm, '<h4>$1</h4>');
        html = html.replace(/^###\s+(.+)$/gm, '<h3>$1</h3>');
        html = html.replace(/^##\s+(.+)$/gm, '<h2>$1</h2>');
        html = html.replace(/^#\s+(.+)$/gm, '<h1>$1</h1>');
        html = html.replace(/\*\*\*(.+?)\*\*\*/g, '<strong><em>$1</em></strong>');
        html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
        html = html.replace(/\*(.+?)\*/g, '<em>$1</em>');
        html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank">$1</a>');
        html = html.replace(/`([^`]+)`/g, '<code>$1</code>');
        html = html.replace(/^[-*]\s+(.+)$/gm, '<li>$1</li>');
        html = html.replace(/(<li>.*<\/li>\n?)+/g, '<ul>$&</ul>');
        html = html.replace(/^---+$/gm, '<hr>');
        html = html.replace(/\n\n/g, '</p><p>');
        html = '<p>' + html + '</p>';
        html = html.replace(/<p>\s*<\/p>/g, '');
        html = html.replace(/<p>\s*(<h[1-6]>)/g, '$1');
        html = html.replace(/(<\/h[1-6]>)\s*<\/p>/g, '$1');
        html = html.replace(/<p>\s*(<ul>)/g, '$1');
        html = html.replace(/(<\/ul>)\s*<\/p>/g, '$1');
        html = html.replace(/<p>\s*(<hr>)/g, '$1');
        html = html.replace(/(<hr>)\s*<\/p>/g, '$1');
        markdownPreview.innerHTML = html;
    }

    // --- Form submission (POST to create new strain) ---
    if (form) {
        form.addEventListener("submit", (e) => {
            e.preventDefault();

            const payload = {
                name: document.getElementById("editStrainName").value,
                breeder_id: editBreederSelect.value === "new" ? null : parseInt(editBreederSelect.value, 10),
                new_breeder: editBreederSelect.value === "new" ? document.getElementById("editNewBreederName").value : null,
                indica: parseInt(editIndicaSativaSlider.value, 10),
                sativa: 100 - parseInt(editIndicaSativaSlider.value, 10),
                autoflower: document.getElementById("editAutoflower").value === "true",
                seed_count: parseInt(document.getElementById("editSeedCount").value, 10),
                description: descriptionTextarea.value,
                short_desc: document.getElementById("editStrainShortDescription").value,
                cycle_time: parseInt(document.getElementById("editCycleTime").value, 10),
                url: document.getElementById("editUrl").value
            };

            fetch("/strains", {
                method: "POST",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(payload),
            })
                .then(response => {
                    if (!response.ok) throw new Error("Failed to add strain");
                    return response.json();
                })
                .then(data => {
                    // Redirect to the new strain's detail page
                    window.location.href = `/strain/${data.id}`;
                })
                .catch(error => {
                    console.error("Error adding strain:", error);
                    if (typeof uiMessages !== 'undefined') {
                        uiMessages.showToast(uiMessages.t('strain_add_fail') || 'Failed to add strain', 'danger');
                    }
                });
        });
    }
});

// --- Markdown toolbar helpers (same as strain-edit.js) ---
function mdWrap(before, after) {
    const ta = document.getElementById("editStrainDescription");
    if (!ta) return;
    const start = ta.selectionStart;
    const end = ta.selectionEnd;
    const selected = ta.value.substring(start, end);
    const replacement = before + (selected || 'text') + after;
    ta.setRangeText(replacement, start, end, 'select');
    ta.focus();
    ta.dispatchEvent(new Event('input'));
}

function mdPrefix(prefix) {
    const ta = document.getElementById("editStrainDescription");
    if (!ta) return;
    const start = ta.selectionStart;
    const end = ta.selectionEnd;
    const selected = ta.value.substring(start, end);
    const lines = selected ? selected.split('\n') : [''];
    const replacement = lines.map(l => prefix + l).join('\n');
    ta.setRangeText(replacement, start, end, 'select');
    ta.focus();
    ta.dispatchEvent(new Event('input'));
}

function mdLink() {
    const ta = document.getElementById("editStrainDescription");
    if (!ta) return;
    const start = ta.selectionStart;
    const end = ta.selectionEnd;
    const selected = ta.value.substring(start, end);
    const replacement = `[${selected || 'link text'}](url)`;
    ta.setRangeText(replacement, start, end, 'select');
    ta.focus();
    ta.dispatchEvent(new Event('input'));
}
