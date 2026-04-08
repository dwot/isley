/*
 * main.js — Isley Dashboard
 * Renders the zone-based dashboard with sensors, streams, and plants.
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

    /* ── Fetch data ────────────────────────────────────────────── */
    let plants = [], sensorData = {}, streamData = {};

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

    /* ── Build unified zone map ────────────────────────────────── */
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

    /* ── Nothing at all? Show empty state ─────────────────────── */
    const zoneNames = Object.keys(zoneMap);
    if (zoneNames.length === 0 && plants.length === 0) {
        document.getElementById("dashEmpty").style.display = "";
        return;
    }

    /* ── Summary bar ───────────────────────────────────────────── */
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

    /* Build summary cards — only show relevant stats */
    let summaryHTML = summaryCard(
        t("dash_active_plants", "Active Plants"), totalPlants,
        `${t("dash_across_zones", "across")} ${zoneNames.length} ${t("dash_zones", "zones")}`
    );

    if (inFlower.length > 0) {
        summaryHTML += summaryCard(t("dash_in_flower", "In Flower"), inFlower.length,
            `${t("dash_avg_days", "avg")} ${avgFlowerDays} ${t("dash_days", "days")}`);
    } else {
        /* Show dominant status instead of "In Flower: 0" */
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


    /* ── Render zones ──────────────────────────────────────────── */
    const zonesEl = document.getElementById("dashZones");

    for (const zoneName of zoneNames) {
        const z = zoneMap[zoneName];
        const section = el("div", "dash-zone");

        /* Flatten all sensors for this zone */
        const allSensors = [];
        for (const devSensors of Object.values(z.sensors)) {
            for (const sensor of devSensors) allSensors.push(sensor);
        }

        /* Separate linked sensors (for plant cards) from unlinked */
        const linkedByPlant = {};   // plant_name → sensor[]
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

        /* Zone header */
        section.appendChild(zoneHeader(zoneName, plantCount, sensorCount));

        /* Zone content */
        const content = el("div", "dash-zone-content");

        const hasPlants  = z.plants.length > 0;
        const hasStreams  = z.streams.length > 0;

        /* ── TOP SECTION: stream + plants in a unified grid ── */
        if (hasPlants || hasStreams) {
            const topDiv = el("div");
            /* Header: show "Plants" if we have plants, otherwise "Live" */
            if (hasPlants) {
                topDiv.appendChild(groupHeader("fa-cannabis",
                    t("title_plants", "Plants"),
                    "/plants", t("dash_view_all", "View all")));
            }

            const grid = el("div", "dash-top-grid");

            /* Stream cards flow into the same grid as plant cards */
            for (const stream of z.streams) {
                const wrapper = el("div", "dash-top-stream");
                wrapper.appendChild(streamCard(stream));
                grid.appendChild(wrapper);
            }

            /* Plant cards */
            for (const plant of z.plants) {
                const plantSensors = linkedByPlant[plant.name] || [];
                grid.appendChild(plantCard(plant, plantSensors));
            }

            topDiv.appendChild(grid);
            content.appendChild(topDiv);
        }

        /* ── SENSORS SECTION (sub-grouped, flowing) ── */
        if (unlinkedSensors.length > 0) {
            const sensorDiv = el("div");
            const allIds = unlinkedSensors.map(s => s.id).join(",");
            sensorDiv.appendChild(groupHeader("fa-microchip",
                t("title_sensors", "Sensors"),
                `/graph/${allIds}`, t("dash_view_graphs", "View graphs")));

            /* Bucket by type */
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
                /* Proportional flex-grow based on sensor count */
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
                return `<span class="dash-pc-sensor" title="${esc(s.name)}">
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
                    <span class="dash-pc-ind ${waterCls}"><i class="fa-solid fa-droplet"></i> ${waterDays}d</span>
                    <span class="dash-pc-ind ${feedCls}"><i class="fa-solid fa-flask"></i> ${feedDays}d</span>
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
