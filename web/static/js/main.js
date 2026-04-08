/*
 * main.js — Isley Dashboard
 * Renders the zone-based dashboard with sensors, streams, and plants.
 * Uses polling to smoothly update sensor readings in-place.
 */
document.addEventListener("DOMContentLoaded", async () => {

    /* ── Translations ──────────────────────────────────────────── */
    let lcl = {};
    try {
        const resp = await fetch("/api/translations");
        if (resp.ok) lcl = await resp.json();
    } catch (_) { /* fallback to empty */ }

    const t = (key, fallback) => lcl[key] || fallback || key;

    /* Status label map (lowercase status → localised label) */
    const statusLabels = {
        germinating: t("germinating_label", "Germinating"),
        planted:     t("planted_label", "Planted"),
        seedling:    t("seedling_label", "Seedling"),
        veg:         t("veg_label", "Veg"),
        flower:      t("flower_label", "Flower"),
        drying:      t("drying_label", "Drying"),
        curing:      t("curing_label", "Curing"),
        success:     t("success_label", "Success"),
        dead:        t("dead_label", "Dead"),
    };

    /* ── Polling interval from server config ───────────────────── */
    const container = document.querySelector(".container[data-poll-interval]");
    const pollInterval = Math.max(15, parseInt(container?.dataset.pollInterval || "60", 10)) * 1000;

    /* ── Fetch data ────────────────────────────────────────────── */
    let plants = [], sensorData = {}, streamData = {};

    async function fetchAllData() {
        try {
            const [pResp, sResp, stResp] = await Promise.all([
                fetch("/plants/living"),
                fetch("/sensors/grouped"),
                fetch("/streams"),
            ]);
            if (pResp.ok)  plants = await pResp.json();
            if (sResp.ok)  sensorData = await sResp.json();
            if (stResp.ok) streamData = await stResp.json();
            if (!Array.isArray(plants)) plants = [];
        } catch (e) {
            console.error("Dashboard fetch error:", e);
        }
    }

    async function fetchSensorData() {
        try {
            const resp = await fetch("/sensors/grouped");
            if (resp.ok) sensorData = await resp.json();
        } catch (e) {
            console.error("Sensor poll error:", e);
        }
    }

    async function fetchPlantData() {
        try {
            const resp = await fetch("/plants/living");
            if (resp.ok) {
                const data = await resp.json();
                if (Array.isArray(data)) plants = data;
            }
        } catch (e) {
            console.error("Plant poll error:", e);
        }
    }

    /* ── Build unified zone map ────────────────────────────────── */
    function buildZoneMap() {
        const zoneMap = {};

        for (const [zone, devices] of Object.entries(sensorData)) {
            if (!zoneMap[zone]) zoneMap[zone] = { sensors: {}, streams: [], plants: [] };
            zoneMap[zone].sensors = devices;
        }

        for (const [zone, streams] of Object.entries(streamData)) {
            if (!zoneMap[zone]) zoneMap[zone] = { sensors: {}, streams: [], plants: [] };
            zoneMap[zone].streams = streams.filter(s => s.visible !== false);
        }

        for (const p of plants) {
            const zone = p.zone_name || "Unassigned";
            if (!zoneMap[zone]) zoneMap[zone] = { sensors: {}, streams: [], plants: [] };
            zoneMap[zone].plants.push(p);
        }

        return zoneMap;
    }

    /* ── Build a flat lookup of all sensors by ID ──────────────── */
    function buildSensorLookup() {
        const lookup = {};
        for (const devices of Object.values(sensorData)) {
            for (const sensors of Object.values(devices)) {
                for (const sensor of sensors) {
                    lookup[sensor.id] = sensor;
                }
            }
        }
        return lookup;
    }

    /* ── Initial full render ───────────────────────────────────── */
    await fetchAllData();
    const zoneMap = buildZoneMap();

    const zoneNames = Object.keys(zoneMap);
    if (zoneNames.length === 0 && plants.length === 0) {
        document.getElementById("dashEmpty").style.display = "";
        return;
    }

    renderSummary(zoneMap, zoneNames);
    renderZones(zoneMap, zoneNames);

    /* ── Polling loop ──────────────────────────────────────────── */
    setInterval(async () => {
        await Promise.all([fetchSensorData(), fetchPlantData()]);
        updateInPlace();
    }, pollInterval);


    /* ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
       In-place update — patches DOM without rebuilding
       ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ */

    function updateInPlace() {
        const sensorLookup = buildSensorLookup();

        /* Update standalone sensor chips */
        document.querySelectorAll(".dash-sensor-chip[data-id]").forEach(chip => {
            const sensor = sensorLookup[chip.dataset.id];
            if (!sensor) return;

            const val = Number(sensor.value).toFixed(2).replace(/\.?0+$/, '') || sensor.value;
            const readingEl = chip.querySelector(".dash-sensor-reading");
            const unitEl = chip.querySelector(".dash-sensor-unit");
            const trendEl = chip.querySelector(".dash-sensor-trend");

            if (readingEl && readingEl.textContent !== String(val)) {
                readingEl.textContent = val;
                chip.classList.add("dash-updated");
                setTimeout(() => chip.classList.remove("dash-updated"), 1200);
            }

            if (unitEl) unitEl.textContent = sensor.unit || '';

            if (trendEl) {
                const trendCls = sensor.trend === "up" ? "dash-trend-up"
                               : sensor.trend === "down" ? "dash-trend-down"
                               : "dash-trend-flat";
                const trendIcon = sensor.trend === "up" ? "fa-arrow-up"
                                : sensor.trend === "down" ? "fa-arrow-down"
                                : "fa-minus";
                trendEl.className = `dash-sensor-trend ${trendCls}`;
                trendEl.innerHTML = `<i class="fa-solid ${trendIcon}"></i>`;
            }
        });

        /* Update plant card linked-sensor badges */
        document.querySelectorAll(".dash-pc[data-plant-id]").forEach(card => {
            const plantId = parseInt(card.dataset.plantId, 10);
            const plant = plants.find(p => p.id === plantId);
            if (!plant) return;

            /* Update watering/feeding indicators */
            const waterDays = plant.days_since_last_watering ?? 0;
            const feedDays = plant.days_since_last_feeding ?? 0;
            const waterInd = card.querySelector(".dash-pc-ind-water");
            const feedInd = card.querySelector(".dash-pc-ind-feed");

            if (waterInd) {
                const waterCls = waterDays >= 4 ? "dash-ind-alert" : waterDays >= 3 ? "dash-ind-warn" : "dash-ind-ok";
                waterInd.className = `dash-pc-ind dash-pc-ind-water ${waterCls}`;
                waterInd.innerHTML = `<i class="fa-solid fa-droplet"></i> ${waterDays}d`;
            }
            if (feedInd) {
                const feedCls = feedDays >= 6 ? "dash-ind-alert" : feedDays >= 5 ? "dash-ind-warn" : "dash-ind-ok";
                feedInd.className = `dash-pc-ind dash-pc-ind-feed ${feedCls}`;
                feedInd.innerHTML = `<i class="fa-solid fa-flask"></i> ${feedDays}d`;
            }

            /* Update linked sensor badges */
            const badgeContainer = card.querySelector(".dash-pc-sensors");
            if (badgeContainer) {
                badgeContainer.querySelectorAll(".dash-pc-sensor[data-sensor-id]").forEach(badge => {
                    const sensor = sensorLookup[badge.dataset.sensorId];
                    if (!sensor) return;

                    const sv = Number(sensor.value).toFixed(0);
                    const unit = esc(sensor.unit || '');
                    const tCls = sensor.trend === "up" ? "dash-trend-up"
                               : sensor.trend === "down" ? "dash-trend-down"
                               : "dash-trend-flat";
                    const tIco = sensor.trend === "up" ? "fa-arrow-up"
                               : sensor.trend === "down" ? "fa-arrow-down"
                               : "fa-minus";

                    const newHTML = `<i class="fa-solid fa-droplet"></i> ${sv}${unit} <i class="fa-solid ${tIco} ${tCls}"></i>`;
                    if (badge.innerHTML !== newHTML) {
                        badge.innerHTML = newHTML;
                        badge.classList.add("dash-badge-updated");
                        setTimeout(() => badge.classList.remove("dash-badge-updated"), 1200);
                    }
                });
            }
        });

        /* Update summary bar */
        updateSummary();
    }

    function updateSummary() {
        const newZoneMap = buildZoneMap();
        const newZoneNames = Object.keys(newZoneMap);

        const totalPlants = plants.length;
        const inFlower = plants.filter(p => (p.status || "").toLowerCase() === "flower");
        const needWater = plants.filter(p => p.days_since_last_watering >= 3);
        let totalSensors = 0;
        for (const z of Object.values(newZoneMap)) {
            for (const devSensors of Object.values(z.sensors)) {
                totalSensors += devSensors.length;
            }
        }

        /* Update summary values by position */
        const summaryEl = document.getElementById("dashSummary");
        const statEls = summaryEl.querySelectorAll(".dash-summary-stat");

        statEls.forEach(statEl => {
            const labelEl = statEl.querySelector(".dash-summary-label");
            const valueEl = statEl.querySelector(".dash-summary-value");
            if (!labelEl || !valueEl) return;

            const label = labelEl.textContent;
            let newValue = null;

            if (label === t("dash_active_plants", "Active Plants")) {
                newValue = totalPlants;
            } else if (label === t("dash_in_flower", "In Flower")) {
                newValue = inFlower.length;
            } else if (label === t("dash_need_water", "Need Water")) {
                newValue = needWater.length;
            } else if (label === t("dash_active_sensors", "Active Sensors")) {
                newValue = totalSensors;
            }

            if (newValue !== null && valueEl.textContent !== String(newValue)) {
                valueEl.textContent = newValue;
                statEl.classList.add("dash-updated");
                setTimeout(() => statEl.classList.remove("dash-updated"), 1200);
            }
        });
    }


    /* ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
       Full render functions (initial load only)
       ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ */

    function renderSummary(zoneMap, zoneNames) {
        const summaryEl = document.getElementById("dashSummary");
        const totalPlants = plants.length;
        const inFlower = plants.filter(p => (p.status || "").toLowerCase() === "flower");
        const needWater = plants.filter(p => p.days_since_last_watering >= 3);
        let totalSensors = 0;
        for (const z of Object.values(zoneMap)) {
            for (const devSensors of Object.values(z.sensors)) {
                totalSensors += devSensors.length;
            }
        }
        const avgFlowerDays = inFlower.length > 0
            ? Math.round(inFlower.reduce((s, p) => s + (p.flowering_days || 0), 0) / inFlower.length)
            : 0;

        let summaryHTML = summaryCard(
            t("dash_active_plants", "Active Plants"), totalPlants,
            `${t("dash_across_zones", "across")} ${zoneNames.length} ${t("dash_zones", "zones")}`
        );

        if (inFlower.length > 0) {
            summaryHTML += summaryCard(t("dash_in_flower", "In Flower"), inFlower.length,
                `${t("dash_avg_days", "avg")} ${avgFlowerDays} ${t("dash_days", "days")}`);
        } else {
            const statusCounts = {};
            plants.forEach(p => {
                const s = (p.status || "unknown").toLowerCase();
                statusCounts[s] = (statusCounts[s] || 0) + 1;
            });
            const dominant = Object.entries(statusCounts).sort((a, b) => b[1] - a[1])[0];
            if (dominant) {
                const lbl = statusLabels[dominant[0]] || dominant[0];
                summaryHTML += summaryCard(lbl, dominant[1],
                    totalPlants > dominant[1] ? `of ${totalPlants} plants` : "");
            }
        }

        if (needWater.length > 0) {
            summaryHTML += summaryCard(t("dash_need_water", "Need Water"), needWater.length,
                t("dash_last_watered", "last watered 3+ days ago"), true);
        }

        summaryHTML += summaryCard(t("dash_active_sensors", "Active Sensors"), totalSensors, "");
        summaryEl.innerHTML = summaryHTML;
    }

    function renderZones(zoneMap, zoneNames) {
        const zonesEl = document.getElementById("dashZones");

        for (const zoneName of zoneNames) {
            const z = zoneMap[zoneName];
            const section = el("div", "dash-zone");

            const allSensors = [];
            for (const devSensors of Object.values(z.sensors)) {
                for (const sensor of devSensors) allSensors.push(sensor);
            }

            const linkedByPlant = {};
            const unlinkedSensors = [];
            for (const sensor of allSensors) {
                if (sensor.plant_name) {
                    if (!linkedByPlant[sensor.plant_name]) linkedByPlant[sensor.plant_name] = [];
                    linkedByPlant[sensor.plant_name].push(sensor);
                } else {
                    unlinkedSensors.push(sensor);
                }
            }

            const plantCount = z.plants.length;
            const sensorCount = allSensors.length;

            section.appendChild(zoneHeader(zoneName, plantCount, sensorCount));

            const content = el("div", "dash-zone-content");

            const hasPlants  = z.plants.length > 0;
            const hasStreams  = z.streams.length > 0;

            if (hasPlants || hasStreams) {
                const topDiv = el("div");
                if (hasPlants) {
                    topDiv.appendChild(groupHeader("fa-cannabis",
                        t("title_plants", "Plants"),
                        "/plants", t("dash_view_all", "View all")));
                }

                const grid = el("div", "dash-top-grid");

                for (const stream of z.streams) {
                    const wrapper = el("div", "dash-top-stream");
                    wrapper.appendChild(streamCard(stream));
                    grid.appendChild(wrapper);
                }

                for (const plant of z.plants) {
                    const plantSensors = linkedByPlant[plant.name] || [];
                    grid.appendChild(plantCard(plant, plantSensors));
                }

                topDiv.appendChild(grid);
                content.appendChild(topDiv);
            }

            if (unlinkedSensors.length > 0) {
                const sensorDiv = el("div");
                const allIds = unlinkedSensors.map(s => s.id).join(",");
                sensorDiv.appendChild(groupHeader("fa-microchip",
                    t("title_sensors", "Sensors"),
                    `/graph/${allIds}`, t("dash_view_graphs", "View graphs")));

                const buckets = { Other: [], ACIP: [], Soil: [] };
                for (const sensor of unlinkedSensors) {
                    if ((sensor.type || "").startsWith("Soil")) buckets.Soil.push(sensor);
                    else if ((sensor.type || "").startsWith("ACIP")) buckets.ACIP.push(sensor);
                    else buckets.Other.push(sensor);
                }

                const bucketMeta = {
                    Other: t("title_group_other", "Environment"),
                    ACIP:  t("title_group_acip", "AC Infinity"),
                    Soil:  t("title_group_soil", "Soil"),
                };

                const sensorWrap = el("div", "dash-sensor-groups");
                const activeBuckets = ["Other", "ACIP", "Soil"].filter(k => buckets[k].length > 0);
                const needLabels = activeBuckets.length > 1;

                for (const key of activeBuckets) {
                    const sensors = buckets[key];

                    const group = el("div", "dash-sensor-bucket");
                    group.style.flexGrow = sensors.length;

                    if (needLabels) {
                        const label = el("div", "dash-sensor-sublabel");
                        label.textContent = bucketMeta[key];
                        group.appendChild(label);
                    }

                    const strip = el("div", "dash-sensor-strip");
                    for (const sensor of sensors) {
                        strip.appendChild(sensorChip(sensor));
                    }
                    group.appendChild(strip);
                    sensorWrap.appendChild(group);
                }

                sensorDiv.appendChild(sensorWrap);
                content.appendChild(sensorDiv);
            }

            section.appendChild(content);
            zonesEl.appendChild(section);
        }
    }


    /* ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
       Helper functions
       ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━ */

    function el(tag, cls) {
        const e = document.createElement(tag);
        if (cls) e.className = cls;
        return e;
    }

    function summaryCard(label, value, sub, warn) {
        return `<div class="dash-summary-stat">
            <span class="dash-summary-label">${esc(label)}</span>
            <span class="dash-summary-value"${warn ? ' style="color:#f59e0b"' : ''}>${value}</span>
            ${sub ? `<span class="dash-summary-sub">${esc(sub)}</span>` : ''}
        </div>`;
    }

    function zoneHeader(name, plantCount, sensorCount) {
        const div = el("div", "dash-zone-header");
        div.innerHTML = `
            <div class="dash-zone-icon"><i class="fa-solid fa-tent-arrows-down"></i></div>
            <span class="dash-zone-name">${esc(name)}</span>
            <div class="dash-zone-meta">
                ${plantCount > 0 ? `<span class="dash-zone-meta-item"><i class="fa-solid fa-seedling"></i> ${plantCount} ${t("title_plants", "plants").toLowerCase()}</span>` : ''}
                ${sensorCount > 0 ? `<span class="dash-zone-meta-item"><i class="fa-solid fa-microchip"></i> ${sensorCount} ${t("title_sensors", "sensors").toLowerCase()}</span>` : ''}
            </div>
        `;
        return div;
    }

    function streamCard(stream) {
        const card = el("div", "dash-stream-card");
        const imageUrl = `/uploads/streams/stream_${stream.id}_latest.jpg`;
        const videoId = `stream-${stream.id}`;

        card.innerHTML = `
            <div class="dash-stream-thumb" id="${videoId}-container">
                <img id="${videoId}-img" src="${imageUrl}" alt="${esc(stream.name)}" loading="lazy"
                     onerror="this.style.display='none'">
                <div class="dash-stream-play"><i class="fa-solid fa-circle-play"></i></div>
            </div>
            <div class="dash-stream-label">
                <span class="dash-stream-name">${esc(stream.name)}</span>
                <div class="dash-stream-live"></div>
            </div>
        `;

        setTimeout(() => {
            const thumb = document.getElementById(`${videoId}-container`);
            if (!thumb) return;
            thumb.addEventListener("click", () => {
                if (stream.url && stream.url.endsWith('.m3u8')) {
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
                    thumb.innerHTML = '';
                    thumb.appendChild(video);
                    videojs(`${videoId}-player`, { fluid: true, liveui: true }).ready(function () {
                        this.play();
                    });
                }
            });
        }, 0);

        return card;
    }

    function groupHeader(icon, title, href, linkText) {
        const div = el("div", "dash-group-header");
        div.innerHTML = `
            <span class="dash-group-title"><i class="fa-solid ${icon}"></i> ${esc(title)}</span>
            <a href="${href}" class="dash-group-link">${esc(linkText)} <i class="fa-solid fa-arrow-right" style="font-size:0.65rem;margin-left:2px;"></i></a>
        `;
        return div;
    }

    function sensorChip(sensor) {
        const chip = el("div", "dash-sensor-chip");
        chip.style.cursor = "pointer";
        chip.dataset.id = sensor.id;

        const val = Number(sensor.value).toFixed(2).replace(/\.?0+$/, '') || sensor.value;
        const trendCls = sensor.trend === "up" ? "dash-trend-up"
                       : sensor.trend === "down" ? "dash-trend-down"
                       : "dash-trend-flat";
        const trendIcon = sensor.trend === "up" ? "fa-arrow-up"
                        : sensor.trend === "down" ? "fa-arrow-down"
                        : "fa-minus";

        /* Type indicator dot */
        let typeDot = "";
        if ((sensor.type || "").startsWith("ACIP")) {
            typeDot = `<span class="dash-sensor-type-dot dash-dot-acip" title="AC Infinity"></span>`;
        } else if ((sensor.type || "").startsWith("Soil")) {
            typeDot = `<span class="dash-sensor-type-dot dash-dot-soil" title="EcoWitt Soil"></span>`;
        }

        chip.innerHTML = `
            <div class="dash-sensor-label-row">
                ${typeDot}
                <span class="dash-sensor-label">${esc(sensor.name)}</span>
            </div>
            <div class="dash-sensor-value">
                <span class="dash-sensor-reading">${val}</span>
                <span class="dash-sensor-unit">${esc(sensor.unit || '')}</span>
            </div>
            <div class="dash-sensor-trend ${trendCls}"><i class="fa-solid ${trendIcon}"></i></div>
        `;

        chip.addEventListener("click", () => {
            window.location.href = `/graph/${sensor.id}`;
        });

        return chip;
    }

    function plantCard(plant, linkedSensors) {
        const link = el("a", "dash-pc");
        link.href = `/plant/${plant.id}`;
        link.dataset.plantId = plant.id;

        const status = (plant.status || "").toLowerCase();
        const statusLabel = statusLabels[status] || plant.status || "";
        const statusCls = `dash-status-${status || 'default'}`;

        const waterDays = plant.days_since_last_watering ?? 0;
        const feedDays = plant.days_since_last_feeding ?? 0;
        const waterCls = waterDays >= 4 ? "dash-ind-alert" : waterDays >= 3 ? "dash-ind-warn" : "dash-ind-ok";
        const feedCls = feedDays >= 6 ? "dash-ind-alert" : feedDays >= 5 ? "dash-ind-warn" : "dash-ind-ok";

        const weekDay = `${t("title_week", "Wk")} ${plant.current_week} / ${t("title_day", "Day")} ${plant.current_day}`;
        const breederSep = plant.breeder_name ? ` · ${esc(plant.breeder_name)}` : '';

        /* Build inline sensor badges for linked sensors */
        let sensorBadgesHTML = '';
        if (linkedSensors && linkedSensors.length > 0) {
            const badges = linkedSensors.map(s => {
                const sv = Number(s.value).toFixed(0);
                const unit = esc(s.unit || '');
                const tCls = s.trend === "up" ? "dash-trend-up"
                           : s.trend === "down" ? "dash-trend-down"
                           : "dash-trend-flat";
                const tIco = s.trend === "up" ? "fa-arrow-up"
                           : s.trend === "down" ? "fa-arrow-down"
                           : "fa-minus";
                return `<span class="dash-pc-sensor" data-sensor-id="${s.id}" title="${esc(s.name)}">
                    <i class="fa-solid fa-droplet"></i> ${sv}${unit}
                    <i class="fa-solid ${tIco} ${tCls}"></i>
                </span>`;
            }).join('');
            sensorBadgesHTML = `<div class="dash-pc-sensors">${badges}</div>`;
        }

        link.innerHTML = `
            <div class="dash-pc-top">
                <span class="dash-pc-name">${esc(plant.name)}</span>
                <span class="dash-pc-status ${statusCls}">${esc(statusLabel)}</span>
            </div>
            <span class="dash-pc-strain">${esc(plant.strain_name || '')}${breederSep}</span>
            ${sensorBadgesHTML}
            <div class="dash-pc-bottom">
                <span class="dash-pc-stat"><i class="fa-solid fa-calendar-day"></i> ${weekDay}</span>
                <div class="dash-pc-indicators">
                    <span class="dash-pc-ind dash-pc-ind-water ${waterCls}"><i class="fa-solid fa-droplet"></i> ${waterDays}d</span>
                    <span class="dash-pc-ind dash-pc-ind-feed ${feedCls}"><i class="fa-solid fa-flask"></i> ${feedDays}d</span>
                </div>
            </div>
        `;

        return link;
    }

    function esc(str) {
        if (!str) return '';
        const d = document.createElement('div');
        d.textContent = str;
        return d.innerHTML;
    }
});
