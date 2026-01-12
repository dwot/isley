document.addEventListener("DOMContentLoaded", () => {
    const imageModal = document.getElementById("imageModal");
    const modalImage = document.getElementById("modalImage");
    const originalImageSrc = document.getElementById("originalImageSrc");
    const modalDescription = document.getElementById("modalDescription");
    const modalDateInput = document.getElementById("modalDate");
    const imageIdInput = document.getElementById("imageId");
    const modalStartDate = document.getElementById("modalStartDate");
    const text1dropdown = document.getElementById("text1Dropdown");
    const text2dropdown = document.getElementById("text2Dropdown");
    const prevButton = document.getElementById("prevImage");
    const nextButton = document.getElementById("nextImage");

    let currentImageIndex = 0;
    const images = Array.from(document.querySelectorAll(".thumbnail-img"));

    function loadImage(index) {
        const img = images[index];
        modalImage.src = img.dataset.image;
        originalImageSrc.src = img.dataset.image;
        modalDateInput.value = img.dataset.date;
        imageIdInput.value = img.dataset.id;

        const plantStartDate = new Date(modalStartDate.value);
        const modalDate = new Date(img.dataset.date);
        const diffTime = Math.abs(modalDate - plantStartDate);
        const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));
        const diffWeeks = Math.floor(diffDays / 7);
        const strainName = document.getElementById("strainName").value;
        const breederName = document.getElementById("breederName").value;
        const plantName = document.getElementById("modalPlantName").value;

        // Populate dropdowns
        text1dropdown.innerHTML = "";
        text2dropdown.innerHTML = "";
        const options = [
            new Option(strainName, strainName),
            new Option(breederName, breederName),
            new Option(plantName, plantName),
            new Option("Day " + diffDays, "Day " + diffDays),
            new Option("Week " + diffWeeks, "Week " + diffWeeks),
            new Option("Custom", "Custom")
        ];
        options.forEach(opt => text1dropdown.add(opt.cloneNode(true)));
        options.forEach(opt => text2dropdown.add(opt.cloneNode(true)));

        text1dropdown.value = strainName;
        text2dropdown.value = "Day " + diffDays;

        // Update button state
        prevButton.disabled = index === 0;
        nextButton.disabled = index === images.length - 1;
    }

    images.forEach((img, index) => {
        img.addEventListener("click", () => {
            currentImageIndex = index;
            loadImage(currentImageIndex);
        });
    });
    // Navigate to the previous image
    function showPrevImage() {
        if (currentImageIndex > 0) {
            currentImageIndex--;
            loadImage(currentImageIndex);
        }
    }

    // Navigate to the next image
    function showNextImage() {
        if (currentImageIndex < images.length - 1) {
            currentImageIndex++;
            loadImage(currentImageIndex);
        }
    }
    // Add click listeners for buttons
    prevButton.addEventListener("click", showPrevImage);
    nextButton.addEventListener("click", showNextImage);

    // Add arrow key navigation
    document.addEventListener("keydown", (e) => {
        if (imageModal.classList.contains("show")) { // Only respond if modal is open
            if (e.key === "ArrowLeft") {
                showPrevImage(); // Left arrow key
            } else if (e.key === "ArrowRight") {
                showNextImage(); // Right arrow key
            }
        }
    });

});


document.addEventListener("DOMContentLoaded", () => {
    const fontDropdown = document.getElementById("fontDropdown");
    const fontPreview = document.getElementById("fontPreview");
    const logoDropdown = document.getElementById("logoDropdown");
    const logoPreview = document.getElementById("logoPreview");
    const decorateImageButton = document.getElementById("decorateImage");
    const modalImage = document.getElementById("modalImage");
    const originalImageSrc = document.getElementById("originalImageSrc");
    const text1Input = document.getElementById("text1");
    const text2Input = document.getElementById("text2");
    const text1Dropdown = document.getElementById("text1Dropdown");
    const text2Dropdown = document.getElementById("text2Dropdown");

    // Load fonts
    fetch("/listFonts")
        .then((response) => response.json())
        .then((data) => {
            if (data.success) {
                data.fonts.forEach((font) => {
                    const fontName = font.split("/").pop().replace(".ttf", "");
                    const option = document.createElement("option");
                    option.value = font;
                    option.textContent = fontName;
                    fontDropdown.appendChild(option);
                });

                const selectedFont = fontDropdown.value;
                if (selectedFont) {
                    const fontName = selectedFont.split("/").pop().replace(".ttf", "");
                    const fontUrl = `/${selectedFont}`;

                    // Add @font-face rule dynamically
                    const style = document.createElement("style");
                    style.innerHTML = `
                @font-face {
                    font-family: '${fontName}';
                    src: url('${fontUrl}');
                }
            `;
                    document.head.appendChild(style);

                    // Apply the new font to the preview
                    fontPreview.style.fontFamily = fontName;
                }

            }
        });

    // Load logos
    fetch("/listLogos")
        .then((response) => response.json())
        .then((data) => {
            if (data.success) {
                data.logos.forEach((logo) => {
                    const option = document.createElement("option");
                    option.value = logo;
                    option.textContent = logo.split("/").pop();
                    logoDropdown.appendChild(option);
                });
            }
        });

    // Font preview
    fontDropdown.addEventListener("change", () => {
        const selectedFont = fontDropdown.value;
        if (selectedFont) {
            const fontName = selectedFont.split("/").pop().replace(".ttf", "");
            const fontUrl = `/${selectedFont}`;

            // Add @font-face rule dynamically
            const style = document.createElement("style");
            style.innerHTML = `
                @font-face {
                    font-family: '${fontName}';
                    src: url('${fontUrl}');
                }
            `;
            document.head.appendChild(style);

            // Apply the new font to the preview
            fontPreview.style.fontFamily = fontName;
        }
    });

    // Logo preview
    logoDropdown.addEventListener("change", () => {
        if (logoDropdown.value) {
            logoPreview.src = `/${logoDropdown.value}`;
            logoPreview.style.display = "block";
        } else {
            logoPreview.style.display = "none";
        }
    });

    // Enable custom input fields based on dropdown selection
    [text1Dropdown, text2Dropdown].forEach((dropdown, index) => {
        dropdown.addEventListener("change", () => {
            const input = index === 0 ? text1Input : text2Input;
            input.disabled = dropdown.value !== "Custom";
        });
    });

    decorateImageButton.addEventListener("click", () => {
        const fullImagePath = originalImageSrc.src;
        const imagePath = new URL(fullImagePath).pathname.replace(/^\//, "");
        const text1Dropdown = document.getElementById("text1Dropdown");
        const text2Dropdown = document.getElementById("text2Dropdown");
        const text1Input = document.getElementById("text1");
        const text2Input = document.getElementById("text2");
        const text1 = text1Dropdown.value === "Custom" ? text1Input.value : text1Dropdown.value;
        const text2 = text2Dropdown.value === "Custom" ? text2Input.value : text2Dropdown.value;
        const logo = logoDropdown.value;
        const font = fontDropdown.value;
        const textColor = document.getElementById("textColor").value;
        const text1Corner = document.getElementById("text1Corner").value;
        const text2Corner = document.getElementById("text2Corner").value;

        // Add a spinner overlay
        const modal = document.getElementById("imageModal");
        const spinnerOverlay = document.createElement("div");
        spinnerOverlay.id = "spinnerOverlay";
        spinnerOverlay.innerHTML = `<div class="spinner-border text-primary" role="status"></div>`;
        modal.appendChild(spinnerOverlay);

        // Send POST request
        fetch("/decorateImage", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
                imagePath,
                text1,
                text2,
                text1Corner,
                text2Corner,
                logo,
                font,
                textColor,
            }),
        })
            .then((response) => response.json())
            .then((data) => {
                if (data.success) {
                    modalImage.src = "../" + data.outputPath + "?" + new Date().getTime();
                } else {
                    uiMessages.showToast(uiMessages.t('failed_to_decorate_image') || ('Failed to decorate image: ' + data.error), 'danger');
                }
            })
            .catch((error) => console.error("Error decorating image:", error))
            .finally(() => {
                document.getElementById("spinnerOverlay").remove();
            });
    });
});

function confirmDeleteImage() {
    const imageId = document.getElementById("imageId").value;
    uiMessages.showConfirm(uiMessages.t('confirm_delete_image') || 'Are you sure you want to delete this image?').then(confirmed => {
        if (!confirmed) return;
        fetch(`/plant/images/${imageId}/delete`, {
            method: "DELETE",
            headers: {
                "Content-Type": "application/json",
            },
        })
            .then((response) => {
                if (!response.ok) {
                    throw new Error("Failed to delete image");
                }
                return response.json();
            })
            .then((data) => {
                uiMessages.showToast(uiMessages.t('image_deleted_successfully') || 'Image deleted successfully!', 'success');
                // Reload the page to reflect changes
                location.reload();
            })
            .catch((error) => {
                console.error("Error deleting image:", error);
                uiMessages.showToast(uiMessages.t('error_deleting_image') || 'An error occurred while deleting the image.', 'danger');
            });
    });
}