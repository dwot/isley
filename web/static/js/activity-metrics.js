/**
 * Shared helpers for rendering metric input fields on activity modals.
 *
 * Fetches metric definitions from GET /metrics on first use.
 */
const activityMetrics = (() => {
    let _metricsCache = null;

    async function loadMetrics() {
        if (_metricsCache) return _metricsCache;
        try {
            const resp = await fetch('/metrics');
            if (resp.ok) {
                _metricsCache = await resp.json();
            } else {
                _metricsCache = [];
            }
        } catch (e) {
            _metricsCache = [];
        }
        return _metricsCache;
    }

    /**
     * Render metric input fields into a container.
     * @param {HTMLElement} container - The DOM element to render into.
     * @param {Array} metricLinks - Array of {metric_id, required} from the activity option's data-metrics.
     * @param {Object} [existingValues] - Optional map of metric_id -> value for pre-filling (edit mode).
     */
    async function renderInputs(container, metricLinks, existingValues) {
        container.innerHTML = '';
        if (!metricLinks || metricLinks.length === 0) return;

        const all = await loadMetrics();

        metricLinks.forEach(link => {
            const info = all.find(m => m.id === link.metric_id);
            if (!info) return;

            const div = document.createElement('div');
            div.className = 'mb-3';

            const label = document.createElement('label');
            label.className = 'form-label' + (link.required ? ' required' : '');
            label.textContent = info.name + (info.unit ? ' (' + info.unit + ')' : '');
            label.setAttribute('for', 'metric_' + info.id);

            const input = document.createElement('input');
            input.type = 'number';
            input.step = 'any';
            input.className = 'form-control';
            input.id = 'metric_' + info.id;
            input.name = 'metric_' + info.id;
            input.dataset.metricId = info.id;
            if (link.required) input.required = true;

            if (existingValues && existingValues[info.id] !== undefined) {
                input.value = existingValues[info.id];
            }

            div.appendChild(label);
            div.appendChild(input);
            container.appendChild(div);
        });
    }

    /**
     * Read metric links from the currently selected <option>'s data-metrics attribute.
     * @param {HTMLSelectElement} select
     * @returns {Array}
     */
    function getLinksFromSelect(select) {
        const opt = select.options[select.selectedIndex];
        if (!opt) return [];
        const raw = opt.getAttribute('data-metrics');
        if (!raw) return [];
        try { return JSON.parse(raw); } catch(e) { return []; }
    }

    /**
     * Collect metric values from rendered inputs inside a container.
     * @param {HTMLElement} container
     * @returns {Array} [{metric_id, value}]
     */
    function collectValues(container) {
        const values = [];
        container.querySelectorAll('input[data-metric-id]').forEach(input => {
            if (input.value === '') return;
            values.push({
                metric_id: parseInt(input.dataset.metricId, 10),
                value: parseFloat(input.value),
            });
        });
        return values;
    }

    return { renderInputs, getLinksFromSelect, collectValues };
})();
