document.addEventListener("DOMContentLoaded", () => {
    const editStrainForm = document.getElementById("editStrainForm");
    const deleteStrainButton = document.getElementById("deleteStrainButton");
    const editBreederSelect = document.getElementById("editBreederSelect");
    const editNewBreederInput = document.getElementById("editNewBreederInput");
    const editIndicaInput = document.getElementById("editIndicaInput");
    const editSativaInput = document.getElementById("editSativaInput");
    const editRuderalisInput = document.getElementById("editRuderalisInput");
    const editGeneticsLower = document.getElementById("editGeneticsLower");
    const editGeneticsUpper = document.getElementById("editGeneticsUpper");
    const editIndicaLabel = document.getElementById("editIndicaLabel");
    const editSativaLabel = document.getElementById("editSativaLabel");
    const editRuderalisLabel = document.getElementById("editRuderalisLabel");
    const editRatioFillIndica = document.getElementById("editRatioFillIndica");
    const editRatioFillSativa = document.getElementById("editRatioFillSativa");
    const editRatioFillRuderalis = document.getElementById("editRatioFillRuderalis");
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

    // --- Indica/Sativa/Ruderalis dual-thumb slider ---
    function updateRatioPreview(activeThumb) {
        if (!editGeneticsLower || !editGeneticsUpper) return;

        let lower = parseInt(editGeneticsLower.value, 10);
        let upper = parseInt(editGeneticsUpper.value, 10);
        lower = Number.isFinite(lower) ? Math.min(100, Math.max(0, lower)) : 0;
        upper = Number.isFinite(upper) ? Math.min(100, Math.max(0, upper)) : 100;

        if (lower > upper) {
            if (activeThumb === "lower") {
                upper = lower;
            } else {
                lower = upper;
            }
        }

        const indica = lower;
        const sativa = upper - lower;
        const ruderalis = 100 - upper;

        editGeneticsLower.value = String(lower);
        editGeneticsUpper.value = String(upper);
        if (editIndicaInput) editIndicaInput.value = String(indica);
        if (editSativaInput) editSativaInput.value = String(sativa);
        if (editRuderalisInput) editRuderalisInput.value = String(ruderalis);

        if (editIndicaLabel) editIndicaLabel.textContent = `Indica: ${indica}%`;
        if (editSativaLabel) editSativaLabel.textContent = `Sativa: ${sativa}%`;
        if (editRuderalisLabel) editRuderalisLabel.textContent = `Ruderalis: ${ruderalis}%`;
        if (editRatioFillIndica) editRatioFillIndica.style.width = `${indica}%`;
        if (editRatioFillSativa) editRatioFillSativa.style.width = `${sativa}%`;
        if (editRatioFillRuderalis) editRatioFillRuderalis.style.width = `${ruderalis}%`;
        const ratioError = document.getElementById("editRatioError");
        if (indica + sativa + ruderalis !== 100) {
            if (ratioError) ratioError.classList.remove("d-none");
        } else {
            if (ratioError) ratioError.classList.add("d-none");
        }
    }

    function initRatioSlider() {
        if (!editGeneticsLower || !editGeneticsUpper) return;
        const indica = parseInt(editIndicaInput?.value, 10);
        const sativa = parseInt(editSativaInput?.value, 10);
        const ruderalis = parseInt(editRuderalisInput?.value, 10);

        if (
            Number.isFinite(indica) && Number.isFinite(sativa) && Number.isFinite(ruderalis) &&
            indica >= 0 && sativa >= 0 && ruderalis >= 0 &&
            indica + sativa + ruderalis === 100
        ) {
            editGeneticsLower.value = String(indica);
            editGeneticsUpper.value = String(indica + sativa);
        } else {
            editGeneticsLower.value = "50";
            editGeneticsUpper.value = "100";
        }

        updateRatioPreview("upper");
    }

    if (editGeneticsLower) editGeneticsLower.addEventListener("input", () => updateRatioPreview("lower"));
    if (editGeneticsUpper) editGeneticsUpper.addEventListener("input", () => updateRatioPreview("upper"));
    initRatioSlider();

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
        // Client-side markdown rendering using a simple approach
        // We'll use the server's markdownify endpoint or do basic client-side rendering
        // For now, do a lightweight client-side render
        let html = md;

        // Escape HTML first
        html = html.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

        // Headers
        html = html.replace(/^######\s+(.+)$/gm, '<h6>$1</h6>');
        html = html.replace(/^#####\s+(.+)$/gm, '<h5>$1</h5>');
        html = html.replace(/^####\s+(.+)$/gm, '<h4>$1</h4>');
        html = html.replace(/^###\s+(.+)$/gm, '<h3>$1</h3>');
        html = html.replace(/^##\s+(.+)$/gm, '<h2>$1</h2>');
        html = html.replace(/^#\s+(.+)$/gm, '<h1>$1</h1>');

        // Bold and italic
        html = html.replace(/\*\*\*(.+?)\*\*\*/g, '<strong><em>$1</em></strong>');
        html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
        html = html.replace(/\*(.+?)\*/g, '<em>$1</em>');

        // Links
        html = html.replace(/\[([^\]]+)\]\(([^)]+)\)/g, '<a href="$2" target="_blank">$1</a>');

        // Images
        html = html.replace(/!\[([^\]]*)\]\(([^)]+)\)/g, '<img src="$2" alt="$1" style="max-width:100%">');

        // Inline code
        html = html.replace(/`([^`]+)`/g, '<code>$1</code>');

        // Unordered lists
        html = html.replace(/^[-*]\s+(.+)$/gm, '<li>$1</li>');
        html = html.replace(/(<li>.*<\/li>\n?)+/g, '<ul>$&</ul>');

        // Horizontal rules
        html = html.replace(/^---+$/gm, '<hr>');

        // Paragraphs (double newlines)
        html = html.replace(/\n\n/g, '</p><p>');
        html = '<p>' + html + '</p>';

        // Clean up empty paragraphs
        html = html.replace(/<p>\s*<\/p>/g, '');
        html = html.replace(/<p>\s*(<h[1-6]>)/g, '$1');
        html = html.replace(/(<\/h[1-6]>)\s*<\/p>/g, '$1');
        html = html.replace(/<p>\s*(<ul>)/g, '$1');
        html = html.replace(/(<\/ul>)\s*<\/p>/g, '$1');
        html = html.replace(/<p>\s*(<hr>)/g, '$1');
        html = html.replace(/(<hr>)\s*<\/p>/g, '$1');

        markdownPreview.innerHTML = html;
    }

    // --- Form submission ---
    if (editStrainForm) {
        editStrainForm.addEventListener("submit", (e) => {
            e.preventDefault();

            const strainId = document.getElementById("editStrainId").value;
            const payload = {
                name: document.getElementById("editStrainName").value,
                breeder_id: editBreederSelect.value === "new" ? null : parseInt(editBreederSelect.value, 10),
                new_breeder: editBreederSelect.value === "new" ? document.getElementById("editNewBreederName").value : null,
                indica: parseInt(editIndicaInput.value, 10) || 0,
                sativa: parseInt(editSativaInput.value, 10) || 0,
                ruderalis: parseInt(editRuderalisInput.value, 10) || 0,
                autoflower: document.getElementById("editAutoflower").value === "true",
                seed_count: parseInt(document.getElementById("editSeedCount").value, 10),
                description: descriptionTextarea.value,
                short_desc: document.getElementById("editStrainShortDescription").value,
                cycle_time: parseInt(document.getElementById("editCycleTime").value, 10),
                url: document.getElementById("editUrl").value
            };

            fetch(`/strains/${strainId}`, {
                method: "PUT",
                headers: { "Content-Type": "application/json" },
                body: JSON.stringify(payload),
            })
                .then(async response => {
                    if (!response.ok) {
                        let errorMessage = "Failed to update strain";
                        try {
                            const errorBody = await response.json();
                            if (errorBody && typeof errorBody.error === "string" && errorBody.error.trim()) {
                                errorMessage = errorBody.error;
                            }
                        } catch (_err) {
                            // Ignore JSON parsing errors and keep default message.
                        }
                        throw new Error(errorMessage);
                    }
                    // Save lineage if the editor is present
                    if (typeof window.saveLineage === "function") {
                        return window.saveLineage().then(() => strainId);
                    }
                    return strainId;
                })
                .then(id => {
                    // Redirect back to the strain detail page
                    if (id && /^\d+$/.test(String(id))) {
                        window.location.href = `/strain/${id}`;
                    }
                })
                .catch(error => {
                    console.error("Error updating strain:", error);
                    if (typeof uiMessages !== 'undefined') {
                        uiMessages.showToast(uiMessages.t('strain_update_fail') || 'Update failed', 'danger');
                    }
                });
        });
    }

    // --- Delete ---
    if (deleteStrainButton) {
        deleteStrainButton.addEventListener("click", () => {
            const strainId = document.getElementById("editStrainId").value;

            const doDelete = () => {
                fetch(`/strains/${strainId}`, { method: "DELETE" })
                    .then(response => {
                        if (!response.ok) throw new Error("Failed to delete strain");
                        window.location.href = "/strains";
                    })
                    .catch(error => {
                        console.error("Error deleting strain:", error);
                        if (typeof uiMessages !== 'undefined') {
                            uiMessages.showToast(uiMessages.t('delete_error') || 'Delete failed', 'danger');
                        }
                    });
            };

            if (typeof uiMessages !== 'undefined' && uiMessages.showConfirm) {
                uiMessages.showConfirm(uiMessages.t('confirm_delete_strain') || 'Are you sure you want to delete this strain?').then(confirmed => {
                    if (confirmed) doDelete();
                });
            } else if (confirm('Are you sure you want to delete this strain?')) {
                doDelete();
            }
        });
    }
});

// --- Markdown toolbar helpers ---
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

// Attach markdown toolbar buttons via event delegation (replaces inline onclick)
document.addEventListener("click", (e) => {
    const btn = e.target.closest(".md-btn");
    if (!btn) return;
    const action = btn.dataset.mdAction;
    if (action === "wrap") mdWrap(btn.dataset.mdBefore, btn.dataset.mdAfter);
    else if (action === "prefix") mdPrefix(btn.dataset.mdPrefix);
    else if (action === "link") mdLink();
});
