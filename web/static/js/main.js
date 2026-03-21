/*
Clickable rows for plant cards
 */
document.addEventListener("DOMContentLoaded", () => {
    // Add click event to rows
    document.querySelectorAll(".clickable-row").forEach(row => {
        row.addEventListener("click", () => {
            const plantId = row.getAttribute("data-id");
            if (plantId && /^\d+$/.test(plantId)) {
                window.location.href = `/plant/${plantId}`;
            }
        });
    });
});

/*
Dynamic loading of sensor data and video streams
 */
document.addEventListener("DOMContentLoaded", async () => {
    // Get the current Theme from storage
    const theme = localStorage.getItem("isley-theme") || "dark";
    const themeTextClass = theme === "dark" ? "text-light" : "text-dark";
    const themeBgClass = theme === "dark" ? "bg-dark" : "bg-light";

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
        while (sensorsOverview.firstChild) sensorsOverview.removeChild(sensorsOverview.firstChild);
        sensorsOverview.classList.add("p-3");

        Object.keys(sensorData).forEach((zone) => {
            const zoneContainer = document.createElement("div");
            zoneContainer.classList.add("mb-5");

            const showZoneHeader = Object.keys(sensorData).length > 0;
            if (showZoneHeader) {
                const zoneHeader = document.createElement('h4');
                zoneHeader.className = 'text-secondary mb-3';
                zoneHeader.textContent = zone;
                zoneContainer.appendChild(zoneHeader);
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

                    const divOuter = document.createElement('div');
                    divOuter.className = classItem;
                    const divInner = document.createElement('div');
                    divInner.id = `${videoId}-container`;
                    const img = document.createElement('img');
                    img.id = `${videoId}-img`;
                    img.src = imageUrl;
                    img.alt = `Screengrab of ${stream.name}`;
                    img.className = 'img-fluid rounded shadow-sm';
                    img.style.cursor = 'pointer';
                    divInner.appendChild(img);
                    divOuter.appendChild(divInner);
                    videoContainer.appendChild(divOuter);

                    // Attach event listener AFTER ensuring the element is rendered
                    setTimeout(() => {
                        if (stream.url.endsWith('.m3u8')) {
                            const imageElement = document.getElementById(`${videoId}-img`);
                            if (imageElement) { // Check if the element exists
                                imageElement.addEventListener("click", () => {
                                    const container = document.getElementById(`${videoId}-container`);
                                    const video = document.createElement('video');
                                    video.id = `${videoId}-player`;
                                    video.className = 'video-js vjs-default-skin';
                                    video.controls = true;
                                    video.preload = 'auto';
                                    video.width = 480;
                                    video.height = 270;
                                    const source = document.createElement('source');
                                    source.setAttribute('src', stream.url);
                                    source.setAttribute('type', 'application/vnd.apple.mpegurl');
                                    video.appendChild(source);
                                    container.innerHTML = '';
                                    container.appendChild(video);
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

                    const colDiv = document.createElement('div');
                    colDiv.className = 'col-12 col-md-4';

                    const cardDiv = document.createElement('div');
                    cardDiv.className = `card h-100 ${themeTextClass} ${themeBgClass}`;

                    const cardHeader = document.createElement('div');
                    cardHeader.className = 'card-header text-uppercase';
                    const headerLink = document.createElement('a');
                    headerLink.href = `/graph/${groupSensorIds}`;
                    headerLink.className = themeTextClass;
                    headerLink.textContent = groupTitles[group] || group;
                    cardHeader.appendChild(headerLink);

                    const cardBody = document.createElement('div');
                    cardBody.className = 'card-body';

                    sensorGroups[group].forEach((sensor) => {
                        const sensorRow = document.createElement('div');
                        sensorRow.className = 'd-flex justify-content-between align-items-center sensor-row';
                        sensorRow.dataset.id = sensor.id;
                        sensorRow.style.cursor = 'pointer';

                        const nameSpan = document.createElement('span');
                        // Show plant name prefix for sensors linked to active plants
                        if (sensor.plant_name) {
                            const plantBadge = document.createElement('small');
                            plantBadge.className = 'text-info me-1';
                            plantBadge.textContent = sensor.plant_name + ' \u2014';
                            nameSpan.appendChild(plantBadge);
                            nameSpan.appendChild(document.createTextNode(' ' + sensor.name));
                        } else {
                            nameSpan.textContent = sensor.name;
                        }

                        const valueDiv = document.createElement('div');
                        valueDiv.className = 'text-end';

                        const strong = document.createElement('strong');
                        strong.textContent = `${Number(sensor.value).toFixed(2)} `;
                        const unitText = document.createTextNode(sensor.unit);
                        strong.appendChild(unitText);

                        const icon = document.createElement('i');
                        icon.className = `fa ${
                            sensor.trend === "up" ? "fa-arrow-up text-success" :
                            sensor.trend === "down" ? "fa-arrow-down text-danger" :
                            "fa-minus text-muted"
                        }`;

                        valueDiv.appendChild(strong);
                        valueDiv.appendChild(icon);
                        sensorRow.appendChild(nameSpan);
                        sensorRow.appendChild(valueDiv);
                        cardBody.appendChild(sensorRow);
                    });

                    cardDiv.appendChild(cardHeader);
                    cardDiv.appendChild(cardBody);
                    colDiv.appendChild(cardDiv);
                    cardRow.appendChild(colDiv);
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
                if (sensorId && /^\d+$/.test(sensorId)) {
                    window.location.href = `/graph/${sensorId}`;
                }
            });
        });

    } catch (error) {
        console.error("Error fetching data:", error);
    }
});