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
        if (minutes <= 30) return 'minute';
        if (minutes <= 24 * 60) return 'hour';
        if (minutes <= 7 * 24 * 60) return 'day';
        return 'day';
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

                const hue = (index * 60) % 360;

                return {
                    label: `Sensor: ${sensorNamesCache[result.id] || 'Unknown'}`,
                    data: formattedData,
                    borderColor: `hsl(${hue} 70% 50%)`,
                    backgroundColor: `hsl(${hue} 70% 90% / 0.6)`,
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
                            labels: { usePointStyle: true }
                        },
                        tooltip: {
                            mode: 'index',
                            intersect: false,
                            callbacks: {
                                title: (items) => {
                                    if (!items || items.length === 0) return '';
                                    const d = items[0].parsed.x;
                                    const date = (typeof d === 'number') ? new Date(d) : new Date(d);
                                    return new Intl.DateTimeFormat(undefined, { dateStyle: 'medium', timeStyle: 'short' }).format(date);
                                },
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
                            time: {
                                unit: timeUnit,
                                tooltipFormat: 'PP p', // adapter will format nicely (date-fns style)
                                displayFormats: {
                                    minute: 'MMM d, h:mm a',
                                    hour: 'MMM d, h a',
                                    day: 'MMM d',
                                    month: 'MMM yyyy'
                                }
                            },
                            title: { display: true, text: 'Date / Time' },
                            ticks: { autoSkip: true, maxRotation: 0, autoSkipPadding: 10 },
                            grid: { color: 'rgba(255,255,255,0.05)' }
                        },
                        y: {
                            title: { display: true, text: 'Sensor Value' },
                            grid: { color: 'rgba(255,255,255,0.03)' },
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