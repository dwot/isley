/**
 * CSRF Protection Helper
 * Automatically injects the X-CSRF-Token header into all state-changing fetch requests.
 */
(function () {
    const meta = document.querySelector('meta[name="csrf-token"]');
    if (!meta) return;

    const token = meta.getAttribute('content');
    if (!token) return;

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
})();
