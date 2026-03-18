document.addEventListener("DOMContentLoaded", () => {
    const dropArea = document.getElementById("dropArea");
    const imageInput = document.getElementById("imageInput");
    const imagePreviewContainer = document.getElementById("imagePreviewContainer");
    const uploadImagesButton = document.getElementById("uploadImagesButton");

    const MAX_FILE_SIZE = 10 * 1024 * 1024; // 10 MB — matches server ParseMultipartForm limit

    let imagesToUpload = [];

    // Handle drag-and-drop events
    ["dragenter", "dragover"].forEach(eventType => {
        dropArea.addEventListener(eventType, (e) => {
            e.preventDefault();
            e.stopPropagation();
            dropArea.classList.add("border-success");
        });
    });

    ["dragleave", "drop"].forEach(eventType => {
        dropArea.addEventListener(eventType, (e) => {
            e.preventDefault();
            e.stopPropagation();
            dropArea.classList.remove("border-success");
        });
    });

    // Handle drop event
    dropArea.addEventListener("drop", (e) => {
        const files = Array.from(e.dataTransfer.files);
        handleFiles(files);
    });

    // Handle click to browse
    dropArea.addEventListener("click", () => {
        imageInput.click();
    });

    imageInput.addEventListener("change", () => {
        const files = Array.from(imageInput.files);
        handleFiles(files);
    });

    // Format bytes for display
    function formatBytes(bytes) {
        if (bytes < 1024) return bytes + ' B';
        if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB';
        return (bytes / (1024 * 1024)).toFixed(1) + ' MB';
    }

    // Process selected files
    function handleFiles(files) {
        files.forEach((file) => {
            if (!file.type.startsWith("image/")) {
                uiMessages.showToast(uiMessages.t('only_image_files') || 'Only image files are allowed', 'warning');
                return;
            }

            if (file.size > MAX_FILE_SIZE) {
                const msg = (uiMessages.t('file_too_large') || 'File too large') +
                    ': ' + file.name + ' (' + formatBytes(file.size) + '). ' +
                    (uiMessages.t('max_file_size') || 'Maximum') + ': ' + formatBytes(MAX_FILE_SIZE);
                uiMessages.showToast(msg, 'danger');
                return;
            }

            const reader = new FileReader();
            reader.onload = (e) => {
                const fileData = e.target.result;

                // Read EXIF metadata
                EXIF.getData(file, function () {
                    let imageDate = new Date().toISOString().split("T")[0]; // Default to current date

                    // Try to get the DateTimeOriginal tag from EXIF
                    const exifDate = EXIF.getTag(this, "DateTimeOriginal");
                    if (exifDate) {
                        const parsedDate = parseExifDate(exifDate);
                        if (parsedDate) imageDate = parsedDate;
                    }

                    addImagePreview(fileData, file, imageDate);
                });
            };

            reader.readAsDataURL(file);
            imagesToUpload.push(file);
        });
    }

// Parse EXIF DateTimeOriginal format "YYYY:MM:DD HH:MM:SS" into "YYYY-MM-DD"
    function parseExifDate(exifDate) {
        try {
            const parts = exifDate.split(" ")[0].split(":"); // Split on space, then ":"
            if (parts.length === 3) {
                return `${parts[0]}-${parts[1]}-${parts[2]}`;
            }
        } catch (error) {
            console.error("Error parsing EXIF date:", error);
        }
        return null;
    }

// Add image preview with default or EXIF date
    function addImagePreview(src, file, imageDate) {
        const col = document.createElement("div");
        col.className = "col-12 col-md-6 col-lg-4";

        const card = `
        <div class="card">
            <img src="${src}" class="card-img-top" alt="Preview">
            <div class="card-body">
                <div class="mb-3">
                    <label class="form-label">${uiMessages.t('description_txt') || 'Description'}</label>
                    <input type="text" class="form-control description" placeholder="${uiMessages.t('short_description_placeholder') || 'Enter description'}">
                </div>
                <div class="mb-3">
                    <label class="form-label">${uiMessages.t('title_date') || 'Date'}</label>
                    <input type="date" class="form-control image-date" value="${imageDate}">
                </div>
            </div>
        </div>
    `;

        col.innerHTML = card;
        imagePreviewContainer.appendChild(col);
    }

    // Create or get the upload progress bar container
    function getOrCreateProgressBar() {
        let container = document.getElementById("uploadProgressContainer");
        if (!container) {
            container = document.createElement("div");
            container.id = "uploadProgressContainer";
            container.className = "mt-3 d-none";
            container.innerHTML = `
                <div class="progress" style="height: 24px;">
                    <div id="uploadProgressBar" class="progress-bar progress-bar-striped progress-bar-animated"
                         role="progressbar" style="width: 0%;" aria-valuenow="0" aria-valuemin="0" aria-valuemax="100">
                        0%
                    </div>
                </div>
                <small id="uploadProgressText" class="text-muted mt-1 d-block"></small>
            `;
            uploadImagesButton.parentNode.insertBefore(container, uploadImagesButton.nextSibling);
        }
        return container;
    }

    function updateProgress(percent, text) {
        const container = getOrCreateProgressBar();
        container.classList.remove("d-none");
        const bar = document.getElementById("uploadProgressBar");
        const label = document.getElementById("uploadProgressText");
        bar.style.width = percent + "%";
        bar.setAttribute("aria-valuenow", percent);
        bar.textContent = Math.round(percent) + "%";
        if (label && text) label.textContent = text;
    }

    function hideProgress() {
        const container = document.getElementById("uploadProgressContainer");
        if (container) container.classList.add("d-none");
    }

    // Handle upload with XMLHttpRequest for progress tracking
    uploadImagesButton.addEventListener("click", () => {
        if (imagesToUpload.length === 0) {
            uiMessages.showToast(uiMessages.t('no_images_selected') || 'No images selected', 'warning');
            return;
        }

        const formData = new FormData();

        // Collect data for each image
        document.querySelectorAll("#imagePreviewContainer .card").forEach((card, index) => {
            const description = card.querySelector(".description").value;
            const date = card.querySelector(".image-date").value;

            // Append the file with a simple key
            formData.append("images[]", imagesToUpload[index]);
            formData.append(`descriptions[]`, description);
            formData.append(`dates[]`, date);
        });

        const plantId = document.getElementById("plantId").value;

        // Disable the upload button during upload
        uploadImagesButton.disabled = true;
        updateProgress(0, uiMessages.t('uploading') || 'Uploading...');

        const xhr = new XMLHttpRequest();
        xhr.open("POST", `/plant/${plantId}/images/upload`, true);

        // Track upload progress
        xhr.upload.addEventListener("progress", (e) => {
            if (e.lengthComputable) {
                const percent = (e.loaded / e.total) * 100;
                const uploaded = formatBytes(e.loaded);
                const total = formatBytes(e.total);
                updateProgress(percent, `${uploaded} / ${total}`);
            }
        });

        xhr.addEventListener("load", () => {
            if (xhr.status >= 200 && xhr.status < 300) {
                updateProgress(100, uiMessages.t('upload_complete') || 'Upload complete');
                setTimeout(() => location.reload(), 500);
            } else {
                hideProgress();
                uploadImagesButton.disabled = false;
                uiMessages.showToast(uiMessages.t('error_uploading_images') || 'An error occurred while uploading images.', 'danger');
            }
        });

        xhr.addEventListener("error", () => {
            hideProgress();
            uploadImagesButton.disabled = false;
            console.error("Upload error");
            uiMessages.showToast(uiMessages.t('error_uploading_images') || 'An error occurred while uploading images.', 'danger');
        });

        xhr.send(formData);
    });

});
