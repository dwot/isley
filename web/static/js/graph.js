document.addEventListener("DOMContentLoaded", () => {
    const ctx = document.getElementById('chart').getContext('2d');
    const sensorIds = document.getElementById('sensorID').value.split(',');
    const timePickerButton = document.getElementById('timePickerButton');
    const dropdownMenu = document.getElementById('timeRangeMenu');
    const timeRangeButtons = document.querySelectorAll('.time-range-btn');
    const startDateInput = document.getElementById('startDate');
    const endDateInput = document.getElementById('endDate');
    const applyDateRangeButton = document.getElementById('applyDateRange');
    const resetZoomButton = document.getElementById('resetZoom');
    let chart;
    let sensorNamesCache = {};

    // Series colours come from the theme's --chart-1..6, round again after
    // the sixth.
    const SERIES_TOKENS = 6;
    const seriesColor = (index) => IsleyTheme.color(`--chart-${(index % SERIES_TOKENS) + 1}`);

    // A change of theme or mode repaints the chart in place, keeping its
    // data and zoom.
    document.addEventListener('isley:themechange', () => {
        if (!chart) return;
        const text = IsleyTheme.color('--chart-text');
        const grid = IsleyTheme.color('--chart-grid');
        chart.data.datasets.forEach((dataset, index) => {
            dataset.borderColor = seriesColor(index);
            dataset.backgroundColor = seriesColor(index);
        });
        chart.options.plugins.legend.labels.color = text;
        ['x', 'y'].forEach((axis) => {
            chart.options.scales[axis].title.color = text;
            chart.options.scales[axis].ticks.color = text;
            chart.options.scales[axis].grid.color = grid;
        });
        chart.update('none');
    });

    // Retry fetch with exponential backoff
    const retryFetch = (url, retries = 3, delay = 500) => {
        return new Promise((resolve, reject) => {
            const attemptFetch = (attempt) => {
                fetch(url)
                    .then((response) => {
                        if (!response.ok) {
                            throw new Error(`HTTP error! Status: ${response.status}`);
                        }
                        resolve(response.json());
                    })
                    .catch((err) => {
                        if (attempt <= retries) {
                            console.warn(`Retrying ${url} (Attempt ${attempt} of ${retries})...`);
                            setTimeout(() => attemptFetch(attempt + 1), delay * attempt);
                        } else {
                            reject(err);
                        }
                    });
            };
            attemptFetch(1);
        });
    };

    timePickerButton.addEventListener('click', () => {
        dropdownMenu.parentElement.classList.toggle('open');
    });

    document.addEventListener('click', (e) => {
        if (!dropdownMenu.contains(e.target) && !timePickerButton.contains(e.target)) {
            dropdownMenu.parentElement.classList.remove('open');
        }
    });

    // Choose a sensible time unit based on requested minutes (or start/end range)
    const getTimeUnit = (params) => {
        // default to 'hour'
        let minutes = null;
        const m = params.match(/minutes=(\d+)/);
        if (m) minutes = parseInt(m[1], 10);

        const startMatch = params.match(/start=([^&]*)&end=([^&]*)/);
        if (!minutes && startMatch) {
            const s = new Date(startMatch[1]);
            const e = new Date(startMatch[2]);
            const diffMs = Math.abs(e - s);
            minutes = Math.round(diffMs / (1000 * 60));
        }

        if (minutes === null) return 'hour';
        if (minutes <= 3 * 60) return 'minute';
        if (minutes <= 24 * 60) return 'hour';
        return 'day';
    };

    // Minute ticks on round minutes of the instance zone's clock (:00, :10,
    // ...), spaced for about eight across what's visible, so zooming in
    // keeps them readable. Chart.js would otherwise tick every minute from
    // the range's start and thin them out, landing on :36, :46...
    const MINUTE_STEPS = [1, 2, 5, 10, 15, 30, 60];
    const roundMinuteTicks = (scale) => {
        const span = (scale.max - scale.min) / 60000;
        const step = MINUTE_STEPS.find(s => span / s <= 8) || 60;
        const ticks = [];
        // The hour starts on :00 in the instance zone, also in a :30 zone.
        for (let t = IsleyChartZone.startOf(scale.min, 'hour'); t <= scale.max; t += step * 60000) {
            if (t >= scale.min) ticks.push({ value: t });
        }
        scale.ticks = ticks;
    };

    const fetchAndRenderData = (queryParams) => {
        Promise.all(sensorIds.map(id =>
            retryFetch(`/sensorData?sensor=${id}&${queryParams}`)
                .then(data => {
                    // Cache sensor names from data
                    if (data.length > 0 && data[0].sensor_name) {
                        sensorNamesCache[id] = data[0].sensor_name;
                    }
                    return { id, data };
                })
        ))
        .then(results => {
            const datasets = results.map((result, index) => {
                const formattedData = result.data.map(item => ({
                    x: new Date(item.create_dt),
                    y: item.value
                }));

                return {
                    label: `Sensor: ${sensorNamesCache[result.id] || 'Unknown'}`,
                    data: formattedData,
                    borderColor: seriesColor(index),
                    backgroundColor: seriesColor(index),
                    borderWidth: 2,
                    pointRadius: 2,
                    pointHoverRadius: 5,
                    tension: 0.3,
                    spanGaps: true,
                };
            });

            if (chart) {
                chart.destroy();
            }

            const timeUnit = getTimeUnit(queryParams);

            chart = new Chart(ctx, {
                type: 'line',
                data: { datasets },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    interaction: { mode: 'nearest', intersect: false },
                    plugins: {
                        legend: {
                            position: 'top',
                            labels: { usePointStyle: true, color: IsleyTheme.color('--chart-text') }
                        },
                        tooltip: {
                            mode: 'index',
                            intersect: false,
                            callbacks: {
                                label: (context) => {
                                    const label = context.dataset.label || '';
                                    const value = context.parsed.y;
                                    return `${label}: ${value}`;
                                }
                            }
                        },
                        zoom: {
                            pan: { enabled: true, mode: 'x' },
                            zoom: {
                                wheel: { enabled: true },
                                pinch: { enabled: true },
                                mode: 'x',
                            }
                        }
                    },
                    scales: {
                        x: {
                            type: 'time',
                            // chart-zone-adapter.js places and labels the
                            // ticks (and the tooltip time) in the instance zone.
                            time: { unit: timeUnit },
                            afterBuildTicks: timeUnit === 'minute' ? roundMinuteTicks : undefined,
                            title: { display: true, text: 'Date / Time', color: IsleyTheme.color('--chart-text') },
                            ticks: { autoSkip: true, maxRotation: 0, autoSkipPadding: 10, color: IsleyTheme.color('--chart-text') },
                            grid: { color: IsleyTheme.color('--chart-grid') }
                        },
                        y: {
                            title: { display: true, text: 'Sensor Value', color: IsleyTheme.color('--chart-text') },
                            ticks: { color: IsleyTheme.color('--chart-text') },
                            grid: { color: IsleyTheme.color('--chart-grid') },
                        }
                    }
                }
            });
        })
        .catch(error => console.error('Error fetching data:', error));
    };

    timeRangeButtons.forEach(button => {
        button.addEventListener('click', (e) => {
            const minutes = e.target.dataset.value;
            fetchAndRenderData(`minutes=${minutes}`);
            dropdownMenu.parentElement.classList.remove('open');
        });
    });

    applyDateRangeButton.addEventListener('click', () => {
        const startDate = startDateInput.value;
        const endDate = endDateInput.value;

        if (startDate && endDate) {
            fetchAndRenderData(`start=${startDate}&end=${endDate}`);
            dropdownMenu.parentElement.classList.remove('open');
        }
    });

    resetZoomButton.addEventListener('click', () => {
        if (chart) {
            chart.resetZoom();
        }
    });

    fetchAndRenderData('minutes=1440');
});