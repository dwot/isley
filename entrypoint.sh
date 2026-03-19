#!/bin/sh
# Fix ownership on mounted volumes (may be root-owned from previous versions),
# then drop to the non-root isley user to run the application.
#
# DEPRECATION: This root entrypoint exists only to smooth the transition from
# older images that ran as root. It will be removed in a future release, after
# which the container will run as the isley user directly. If you see permission
# errors after that change, run once:
#   docker run --rm -v <your-volume>:/app/data alpine chown -R 100:101 /app/data

if [ "$(id -u)" = "0" ]; then
    chown -R isley:isley /app/data /app/uploads 2>/dev/null || true
    exec su-exec isley /app/isley "$@"
else
    # Already running as non-root (future default)
    exec /app/isley "$@"
fi
