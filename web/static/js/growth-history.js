document.addEventListener("DOMContentLoaded", () => {
    const section = document.getElementById("growthHistorySection");
    if (!section) {
        return;
    }

    const historyDataEl = document.getElementById("growthHistoryData");
    const timelineEl = document.getElementById("growthTimeline");
    const stageListEl = document.getElementById("growthStageList");
    const emptyEl = document.getElementById("growthHistoryEmpty");
    const gridEl = document.getElementById("growthHistoryGrid");
    const currentStageEl = document.getElementById("growthCurrentStage");
    const currentDaysEl = document.getElementById("growthCurrentDays");
    const totalDaysEl = document.getElementById("growthTotalDays");

    const daysLabel = section.dataset.daysLabel || "Days";
    const emptyLabel = section.dataset.emptyLabel || "No growth history yet.";
    const currentStageFallback = section.dataset.currentStage || "Unknown";

    const msPerDay = 1000 * 60 * 60 * 24;

    const parseHistory = () => {
        if (!historyDataEl) {
            return [];
        }

        try {
            const parsed = JSON.parse(historyDataEl.textContent || "[]");
            const normalized = typeof parsed === "string" ? JSON.parse(parsed) : parsed;
            if (!Array.isArray(normalized)) {
                return [];
            }
            return normalized
                .filter(item => item && (item.status || item.Status) && (item.date || item.Date))
                .map(item => ({
                    status: item.status || item.Status,
                    date: new Date(item.date || item.Date),
                }))
                .filter(item => !Number.isNaN(item.date.getTime()))
                .sort((a, b) => a.date - b.date);
        } catch (error) {
            return [];
        }
    };

    const dedupeStages = stages => {
        const deduped = [];
        stages.forEach(stage => {
            const lastStage = deduped[deduped.length - 1];
            if (!lastStage || lastStage.status !== stage.status) {
                deduped.push(stage);
            }
        });
        return deduped;
    };

    const formatDays = days => {
        if (!Number.isFinite(days) || days <= 0) {
            return "0";
        }
        const rounded = days < 10 ? Math.round(days * 10) / 10 : Math.round(days);
        return Number.isInteger(rounded) ? `${rounded}` : rounded.toFixed(1);
    };

    const formatDateTime = date => new Intl.DateTimeFormat(undefined, {
        dateStyle: "medium",
        timeStyle: "short",
    }).format(date);

    const setEmptyState = () => {
        if (gridEl) {
            gridEl.classList.add("d-none");
        }
        if (emptyEl) {
            emptyEl.textContent = emptyLabel;
            emptyEl.classList.remove("d-none");
        }
        if (timelineEl) {
            timelineEl.innerHTML = "";
        }
        if (stageListEl) {
            stageListEl.innerHTML = "";
        }
    };

    const clearEmptyState = () => {
        if (gridEl) {
            gridEl.classList.remove("d-none");
        }
        if (emptyEl) {
            emptyEl.classList.add("d-none");
        }
    };

    const history = parseHistory();
    const stages = dedupeStages(history);

    if (stages.length === 0) {
        if (currentStageEl) {
            currentStageEl.textContent = currentStageFallback;
        }
        if (currentDaysEl) {
            currentDaysEl.textContent = "0";
        }
        if (totalDaysEl) {
            totalDaysEl.textContent = "0";
        }
        setEmptyState();
        return;
    }

    const now = new Date();
    const segments = stages.map((stage, index) => {
        const start = stage.date;
        const end = index + 1 < stages.length ? stages[index + 1].date : now;
        const duration = Math.max(0, (end - start) / msPerDay);
        return {
            status: stage.status,
            start,
            end,
            duration,
        };
    });

    const totalDays = segments.reduce((sum, segment) => sum + segment.duration, 0);
    const currentSegment = segments[segments.length - 1];

    if (currentStageEl) {
        currentStageEl.textContent = currentSegment.status || currentStageFallback;
    }
    if (currentDaysEl) {
        currentDaysEl.textContent = formatDays(currentSegment.duration);
    }
    if (totalDaysEl) {
        totalDaysEl.textContent = formatDays(totalDays);
    }

    clearEmptyState();

    if (stageListEl) {
        stageListEl.innerHTML = "";
        segments.forEach(segment => {
            const row = document.createElement("div");
            row.className = "growth-stage-row";
            if (segment === currentSegment) {
                row.classList.add("is-current");
            }

            const title = document.createElement("div");
            title.className = "growth-stage-title";
            title.textContent = segment.status;

            const pill = document.createElement("div");
            pill.className = "growth-stage-pill";
            pill.textContent = `${formatDays(segment.duration)} ${daysLabel}`;

            row.appendChild(title);
            row.appendChild(pill);
            stageListEl.appendChild(row);
        });
    }

    if (timelineEl) {
        timelineEl.innerHTML = "";
        const timelineSegments = [...segments].reverse();
        timelineSegments.forEach(segment => {
            const item = document.createElement("div");
            item.className = "growth-timeline-item";
            if (segment === currentSegment) {
                item.classList.add("is-current");
            }

            const stage = document.createElement("div");
            stage.className = "growth-timeline-stage";
            stage.textContent = segment.status;

            const meta = document.createElement("div");
            meta.className = "growth-timeline-meta";

            const range = document.createElement("div");
            range.className = "growth-timeline-range";
            range.textContent = `${formatDateTime(segment.start)} — ${formatDateTime(segment.end)}`;

            const duration = document.createElement("div");
            duration.className = "growth-timeline-duration";
            duration.textContent = `${formatDays(segment.duration)} ${daysLabel}`;

            meta.appendChild(range);
            meta.appendChild(duration);

            item.appendChild(stage);
            item.appendChild(meta);
            timelineEl.appendChild(item);
        });
    }
});
