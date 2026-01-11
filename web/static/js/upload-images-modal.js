document.addEventListener("DOMContentLoaded", () => {
    const dropArea = document.getElementById("dropArea");
    const imageInput = document.getElementById("imageInput");
    const imagePreviewContainer = document.getElementById("imagePreviewContainer");
    const uploadImagesButton = document.getElementById("uploadImagesButton");

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

    // Process selected files
    function handleFiles(files) {
        files.forEach((file) => {
            if (!file.type.startsWith("image/")) {
                uiMessages.showToast(uiMessages.t('only_image_files') || '{{ .lcl.only_image_files }}', 'warning');
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
                    <label class="form-label">{{ .lcl.title_description }}</label>
                    <input type="text" class="form-control description" placeholder="Enter description">
                </div>
                <div class="mb-3">
                    <label class="form-label">{{ .lcl.title_date }}</label>
                    <input type="date" class="form-control image-date" value="${imageDate}">
                </div>
            </div>
        </div>
    `;

        col.innerHTML = card;
        imagePreviewContainer.appendChild(col);
    }


    // Handle upload
    uploadImagesButton.addEventListener("click", () => {
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
        fetch(`/plant/${plantId}/images/upload`, {
            method: "POST",
            body: formData,
        })
            .then((response) => {
                if (!response.ok) {
                    throw new Error("Failed to upload images");
                }
                return response.json();
            })
            .then((data) => {
                //uiMessages.showToast(uiMessages.t('images_uploaded_successfully') || 'Images uploaded successfully!', 'success');
                location.reload();
            })
            .catch((error) => {
                console.error("Error uploading images:", error);
                uiMessages.showToast(uiMessages.t('error_uploading_images') || 'An error occurred while uploading images.', 'danger');
            });
    });

});