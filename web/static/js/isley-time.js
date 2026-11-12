// isley-time.js — dates and times in the instance's time zone.
//
// Isley shows and reads every time in one zone, the instance zone from
// Settings, whatever zone the browser is in. The server sends instants as
// RFC 3339 with an offset, and reads a datetime-local value (no offset) in
// the instance zone. So the page must fill inputs and show times in that
// zone too, never the browser's. header.html puts the zone in
// <meta name="isley-time-zone">.
//
// Calendar days ("2026-11-11": a plant's start, an image's day) have no
// zone and are never shifted.
(function () {
    const meta = document.querySelector('meta[name="isley-time-zone"]');
    const browserZone = Intl.DateTimeFormat().resolvedOptions().timeZone;
    const zone = (meta && meta.content) || browserZone;

    const partsFormat = new Intl.DateTimeFormat("en-US", {
        timeZone: zone, hourCycle: "h23",
        year: "numeric", month: "2-digit", day: "2-digit",
        hour: "2-digit", minute: "2-digit", second: "2-digit",
    });
    const dayOnly = /^\d{4}-\d{2}-\d{2}$/;

    // toDate reads an instant: a Date, epoch milliseconds, or a string the
    // server sent. Returns null for anything unreadable or empty (Go's zero
    // time included).
    function toDate(value) {
        if (value === null || value === undefined || value === "") return null;
        const d = value instanceof Date ? value : new Date(value);
        if (isNaN(d.getTime()) || d.getUTCFullYear() <= 1) return null;
        return d;
    }

    function parts(d) {
        const out = {};
        for (const p of partsFormat.formatToParts(d)) out[p.type] = p.value;
        return out;
    }

    // toInput gives the value for a datetime-local input: the instant's
    // wall time in the instance zone, "YYYY-MM-DDTHH:MM:SS".
    function toInput(value) {
        const d = toDate(value);
        if (!d) return "";
        const p = parts(d);
        return `${p.year}-${p.month}-${p.day}T${p.hour}:${p.minute}:${p.second}`;
    }

    // nowInput is now, for a datetime-local input.
    function nowInput() { return toInput(new Date()); }

    // dayOf is the calendar day ("YYYY-MM-DD") an instant falls on in the
    // instance zone.
    function dayOf(value) {
        const d = toDate(value);
        if (!d) return "";
        const p = parts(d);
        return `${p.year}-${p.month}-${p.day}`;
    }

    // today is today's date in the instance zone, for a date input.
    function today() { return dayOf(new Date()); }

    // formatDateTime shows an instant in the instance zone, in the
    // browser's language. opts are Intl.DateTimeFormat options.
    function formatDateTime(value, opts) {
        const d = toDate(value);
        if (!d) return "";
        return d.toLocaleString(undefined, Object.assign({}, opts, { timeZone: zone }));
    }

    // formatTime shows the time of day of an instant.
    function formatTime(value, opts) {
        const d = toDate(value);
        if (!d) return "";
        return d.toLocaleTimeString(undefined, Object.assign({ hour: "2-digit", minute: "2-digit" }, opts, { timeZone: zone }));
    }

    // formatDate shows the day of an instant in the instance zone, or a
    // calendar day ("2026-11-11") as it is.
    function formatDate(value, opts) {
        if (typeof value === "string" && dayOnly.test(value)) {
            const [y, m, d] = value.split("-").map(Number);
            return new Date(Date.UTC(y, m - 1, d)).toLocaleDateString(undefined,
                Object.assign({}, opts, { timeZone: "UTC" }));
        }
        const d = toDate(value);
        if (!d) return "";
        return d.toLocaleDateString(undefined, Object.assign({}, opts, { timeZone: zone }));
    }

    // dayNumber counts days from a calendar day: an instant's day or a
    // "YYYY-MM-DD" string.
    function dayNumber(value) {
        const s = typeof value === "string" && dayOnly.test(value) ? value : dayOf(value);
        if (!s) return NaN;
        const [y, m, d] = s.split("-").map(Number);
        return Date.UTC(y, m - 1, d) / 86400000;
    }

    // ageOn is a plant's age on a day, the way the server counts it: the
    // start day is day 1, and week 1 is days 1-7.
    function ageOn(startDay, day) {
        const n = dayNumber(day) - dayNumber(startDay) + 1;
        if (isNaN(n) || n < 1) return { day: 0, week: 0 };
        return { day: n, week: Math.floor((n - 1) / 7) + 1 };
    }

    window.IsleyTime = { zone, toDate, toInput, nowInput, dayOf, today, formatDateTime, formatTime, formatDate, ageOn };
})();
