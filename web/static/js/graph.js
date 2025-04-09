    document.addEventListener("DOMContentLoaded", () => {
    const ctx = document.getElementById('chart').getContext('2d');
    const sensorIds = document.getElementById('sensorID').value.split(',');
    const timePickerButton = document.getElementById('timePickerButton');
    const dropdownMenu = document.getElementById('timeRangeMenu');
    //const dropdownMenu = document.querySelector('.dropdown-menu');
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
    borderColor: `hsl(${index * 60}, 70%, 50%)`,
    backgroundColor: `hsl(${index * 60}, 70%, 80%)`,
    borderWidth: 1,
    tension: 0.4,
};
});

    if (chart) {
    chart.destroy();
}

    chart = new Chart(ctx, {
    type: 'line',
    data: { datasets },
    options: {
    responsive: true,
    maintainAspectRatio: false,
    scales: {
    x: {
    type: 'time',
    time: {
    tooltipFormat: 'MMM d, h:mm a',
    unit: 'hour',
},
    title: { display: true, text: 'Time' }
},
    y: { title: { display: true, text: 'Sensor Value' } }
},
    plugins: {
    zoom: {
    pan: { enabled: true, mode: 'x' },
    zoom: {
    pinch: { enabled: true },
    wheel: { enabled: true },
    mode: 'x',
}
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