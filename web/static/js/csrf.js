/**
 * CSRF Protection Helper
 * Automatically injects the X-CSRF-Token header into all state-changing
 * fetch and XMLHttpRequest requests.
 */
(function () {
    const meta = document.querySelector('meta[name="csrf-token"]');
    if (!meta) return;

    const token = meta.getAttribute('content');
    if (!token) return;

    // Intercept fetch requests
    const originalFetch = window.fetch;
    window.fetch = function (url, options) {
        options = options || {};
        const method = (options.method || 'GET').toUpperCase();

        if (method === 'POST' || method === 'PUT' || method === 'DELETE' || method === 'PATCH') {
            options.headers = options.headers || {};
            // Support both Headers object and plain object
            if (options.headers instanceof Headers) {
                if (!options.headers.has('X-CSRF-Token')) {
                    options.headers.set('X-CSRF-Token', token);
                }
            } else {
                if (!options.headers['X-CSRF-Token']) {
                    options.headers['X-CSRF-Token'] = token;
                }
            }
        }

        return originalFetch.call(this, url, options);
    };

    // Intercept XMLHttpRequest requests
    const originalOpen = XMLHttpRequest.prototype.open;
    const originalSend = XMLHttpRequest.prototype.send;

    XMLHttpRequest.prototype.open = function (method, url) {
        this._csrfMethod = (method || 'GET').toUpperCase();
        return originalOpen.apply(this, arguments);
    };

    XMLHttpRequest.prototype.send = function () {
        const method = this._csrfMethod;
        if (method === 'POST' || method === 'PUT' || method === 'DELETE' || method === 'PATCH') {
            this.setRequestHeader('X-CSRF-Token', token);
        }
        return originalSend.apply(this, arguments);
    };
})();
