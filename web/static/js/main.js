/*
Clickable rows for plant cards
 */
document.addEventListener("DOMContentLoaded", () => {
    // Add click event to rows
    document.querySelectorAll(".clickable-row").forEach(row => {
        row.addEventListener("click", () => {
            const plantId = row.getAttribute("data-id");
            if (plantId) {
                window.location.href = `/plant/${plantId}`;
            }
        });
    });
});

/*
Dynamic loading of sensor data and video streams
 */
document.addEventListener("DOMContentLoaded", async () => {
    const sensorsOverview = document.getElementById("sensorsOverview");

    // Define titles for each group
    const groupTitles = {
        Other:"Environment Sensors",
    ACIP: "AC Infinity Devices" ,
    Soil: "EcoWitt Soil Sensors",
};

    // Create spinner element
    const spinner = document.createElement("div");
    spinner.classList.add("spinner-border", "text-primary");
    spinner.setAttribute("role", "status");
    spinner.innerHTML = `<span class="visually-hidden">Loading ...</span>`;
    sensorsOverview.appendChild(spinner);


    try {
        // Fetch sensor and stream data concurrently
        const [sensorResponse, streamResponse] = await Promise.all([
            fetch("/sensors/grouped"),
            fetch("/streams")
        ]);

        const sensorData = await sensorResponse.json();
        const streamData = await streamResponse.json();
        sensorsOverview.innerHTML = "";
        sensorsOverview.classList.add("p-3");

        Object.keys(sensorData).forEach((zone) => {
            const zoneContainer = document.createElement("div");
            zoneContainer.classList.add("mb-5");

            const showZoneHeader = Object.keys(sensorData).length > 0;
            if (showZoneHeader) {
                zoneContainer.innerHTML = `
                    <h4 class="text-secondary mb-3">${zone}</h4>
                `;
            }

            // --- Add Video Feeds ---
            if (streamData[zone] && streamData[zone].length > 0) {
                const videoContainer = document.createElement("div");
                videoContainer.classList.add("row", "g-3");

                let streamCount = 0;
                streamData[zone].forEach((stream, index) => {
                    if (stream.visible === false) {
                        return;
                    } else {
                        streamCount++;
                    }
                });
                let classItem = "col-12 col-md-6 mb-3";
                if (streamCount === 1) {
                    classItem = "col-12 mb-3";
                }

                streamData[zone].forEach((stream, index) => {
                    if (stream.visible === false) {
                        return;
                    }
                    const videoId = `${zone.replace(/\s+/g, '-')}-video-${index}`;
                    const imageUrl = `/uploads/streams/stream_${stream.id}_latest.jpg`;

                    const streamHTML = `
    <div class="${classItem}">
        <div id="${videoId}-container">
            <img id="${videoId}-img" src="${imageUrl}" alt="Screengrab of ${stream.name}"
                 class="img-fluid rounded shadow-sm" style="cursor: pointer;" />
        </div>
    </div>
`;

                    videoContainer.insertAdjacentHTML("beforeend", streamHTML);

                    // Attach event listener AFTER ensuring the element is rendered
                    setTimeout(() => {
                        if (stream.url.endsWith('.m3u8')) {
                            const imageElement = document.getElementById(`${videoId}-img`);
                            if (imageElement) { // Check if the element exists
                                imageElement.addEventListener("click", () => {
                                    const container = document.getElementById(`${videoId}-container`);
                                    container.innerHTML = `
                    <video id="${videoId}-player" class="video-js vjs-default-skin" controls preload="auto" width="480" height="270">
                        <source src="${stream.url}" type="application/vnd.apple.mpegurl" />
                    </video>
                `;
                                    videojs(`${videoId}-player`, { fluid: true, liveui: true }).ready(function() {
                                        this.play();
                                    });
                                });
                            } else {
                                console.error(`Element ${videoId}-img not found.`);
                            }
                        }
                    }, 0); // Delay execution until next render cycle
                });

                zoneContainer.appendChild(videoContainer);
            }


            // --- Process Sensors ---
            const sensorGroups = { Other: [], ACIP: [], Soil: [] };
            Object.keys(sensorData[zone]).forEach((device) => {
                sensorData[zone][device].forEach((sensor) => {
                    if (sensor.type.startsWith("Soil")) {
                        sensorGroups.Soil.push(sensor);
                    } else if (sensor.type.startsWith("ACIP")) {
                        sensorGroups.ACIP.push(sensor);
                    } else {
                        sensorGroups.Other.push(sensor);
                    }
                });
            });

            const cardRow = document.createElement("div");
            cardRow.classList.add("row", "g-4");

            Object.keys(sensorGroups).forEach((group) => {
                if (sensorGroups[group].length > 0) {
                    const groupSensorIds = sensorGroups[group].map(sensor => sensor.id).join(",");
                    const groupHeaderLink = `<a href='/graph/${groupSensorIds}' class='text-light'>${groupTitles[group] || group}</a>`;

                    const card = `
                        <div class="col-12 col-md-4">
                            <div class="card h-100 bg-dark text-light">
                                <div class="card-header text-uppercase">
                                    ${groupHeaderLink}
                                </div>
                                <div class="card-body">
                                    ${sensorGroups[group]
                        .map(
                            (sensor) => `
                                        <div
                                            class="d-flex justify-content-between align-items-center sensor-row"
                                            data-id="${sensor.id}"
                                            style="cursor: pointer;"
                                        >
                                            <span>${sensor.name}</span>
                                            <div class="text-end">
                                                <strong>${Number(sensor.value).toFixed(2)} ${sensor.unit}</strong>
                                                <i class="fa ${
                                sensor.trend === "up"
                                    ? "fa-arrow-up text-success"
                                    : sensor.trend === "down"
                                        ? "fa-arrow-down text-danger"
                                        : "fa-minus text-muted"
                            }"></i>
                                            </div>
                                        </div>
                                    `
                        )
                        .join("")}
                                </div>
                            </div>
                        </div>
                    `;
                    cardRow.innerHTML += card;
                }
            });

            zoneContainer.appendChild(cardRow);
            sensorsOverview.appendChild(zoneContainer);
        });

        // Add click event to sensor rows
        console.time("Add Click Events");
        document.querySelectorAll(".sensor-row").forEach((row) => {
            row.addEventListener("click", () => {
                const sensorId = row.getAttribute("data-id");
                if (sensorId) {
                    window.location.href = `/graph/${sensorId}`;
                }
            });
        });

    } catch (error) {
        console.error("Error fetching data:", error);
    }
});