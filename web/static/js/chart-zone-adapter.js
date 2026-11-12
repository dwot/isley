// chart-zone-adapter.js — Chart.js date adapter in the instance time zone.
//
// Chart.js places time-axis ticks with its date adapter: "the start of each
// day", "add an hour". The date-fns adapter does that in the browser's zone,
// so day ticks sat at the browser's midnight, not the instance's (see
// isley-time.js). This adapter does the calendar math in IsleyTime.zone:
// it reads an instant's wall time there, works on that, and turns the wall
// time back into an instant. Load it after chart.umd and isley-time.js.
(function () {
    if (!window.Chart || !window.IsleyTime) return;

    const zone = IsleyTime.zone;
    const wallFormat = new Intl.DateTimeFormat("en-US", {
        timeZone: zone, hourCycle: "h23", weekday: "short",
        year: "numeric", month: "numeric", day: "numeric",
        hour: "numeric", minute: "numeric", second: "numeric",
    });
    const WEEKDAYS = { Sun: 0, Mon: 1, Tue: 2, Wed: 3, Thu: 4, Fri: 5, Sat: 6 };

    const MINUTE = 60 * 1000, HOUR = 60 * MINUTE, DAY = 24 * HOUR;
    // Units that are a fixed length whatever the zone (offsets are whole
    // minutes; an hour is not, in a :30 or :45 zone, so it isn't here).
    const FIXED = { millisecond: 1, second: 1000, minute: MINUTE };

    // wall is an instant's wall time in the zone; mo is 0-11.
    function wall(t) {
        const p = {};
        for (const part of wallFormat.formatToParts(new Date(t))) p[part.type] = part.value;
        return {
            y: +p.year, mo: +p.month - 1, d: +p.day, h: +p.hour % 24,
            mi: +p.minute, s: +p.second, ms: ((t % 1000) + 1000) % 1000,
            dow: WEEKDAYS[p.weekday],
        };
    }

    // asUTC reads a wall time as if it were UTC. Date.UTC carries overflow
    // (day 32, month -1) into the next field, which add relies on.
    function asUTC(w) {
        return Date.UTC(w.y, w.mo, w.d, w.h, w.mi, w.s, w.ms);
    }

    // fromWall is the instant with wall time w in the zone. Across a DST
    // gap it lands just after; in a repeated hour, on one of the two.
    function fromWall(w) {
        const target = asUTC(w);
        let t = target;
        for (let i = 0; i < 3; i++) {
            const next = target - (asUTC(wall(t)) - t);
            if (next === t) break;
            t = next;
        }
        return t;
    }

    function startOf(t, unit, weekday) {
        if (FIXED[unit]) return Math.floor(t / FIXED[unit]) * FIXED[unit];
        const w = wall(t);
        w.ms = 0; w.s = 0; w.mi = 0;
        if (unit === "hour") return fromWall(w);
        w.h = 0;
        if (unit === "week" || unit === "isoWeek") {
            const first = unit === "isoWeek" ? (weekday === true ? 1 : +weekday || 1) : 0;
            w.d -= (w.dow - first + 7) % 7;
        } else if (unit === "month") {
            w.d = 1;
        } else if (unit === "quarter") {
            w.d = 1; w.mo -= w.mo % 3;
        } else if (unit === "year") {
            w.d = 1; w.mo = 0;
        }
        return fromWall(w);
    }

    const MONTHS = { month: 1, quarter: 3, year: 12 };

    function add(t, amount, unit) {
        if (FIXED[unit]) return t + amount * FIXED[unit];
        if (unit === "hour") return t + amount * HOUR;
        const w = wall(t);
        if (unit === "day" || unit === "week" || unit === "isoWeek") {
            w.d += amount * (unit === "day" ? 1 : 7);
            return fromWall(w);
        }
        // Months keep the day of the month, clamped to the month's end
        // (Jan 31 + 1 month = Feb 28), as date-fns does.
        const first = new Date(Date.UTC(w.y, w.mo + amount * MONTHS[unit], 1));
        w.y = first.getUTCFullYear();
        w.mo = first.getUTCMonth();
        w.d = Math.min(w.d, new Date(Date.UTC(w.y, w.mo + 1, 0)).getUTCDate());
        return fromWall(w);
    }

    // diff counts whole units from min to max.
    function diff(max, min, unit) {
        if (FIXED[unit]) return Math.trunc((max - min) / FIXED[unit]);
        if (unit === "hour") return Math.trunc((max - min) / HOUR);
        const a = wall(max), b = wall(min);
        if (unit === "day" || unit === "week" || unit === "isoWeek") {
            return Math.trunc((asUTC(a) - asUTC(b)) / (unit === "day" ? DAY : 7 * DAY));
        }
        let months = (a.y - b.y) * 12 + (a.mo - b.mo);
        // A month isn't whole until the day and time come round again.
        const rest = (w) => asUTC(Object.assign({}, w, { y: 2000, mo: 0 }));
        if (months > 0 && rest(a) < rest(b)) months--;
        if (months < 0 && rest(a) > rest(b)) months++;
        return Math.trunc(months / MONTHS[unit]);
    }

    // Formats are unit names; format turns each into Intl options shown in
    // the zone. A format the page passes that isn't one of these (a date-fns
    // pattern) gets the full date and time.
    const OPTIONS = {
        datetime: { dateStyle: "medium", timeStyle: "short" },
        millisecond: { hour: "numeric", minute: "2-digit", second: "2-digit", fractionalSecondDigits: 3 },
        second: { hour: "numeric", minute: "2-digit", second: "2-digit" },
        minute: { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" },
        hour: { month: "short", day: "numeric", hour: "numeric" },
        day: { month: "short", day: "numeric" },
        week: { month: "short", day: "numeric" },
        month: { month: "short", year: "numeric" },
        quarter: { month: "short", year: "numeric" },
        year: { year: "numeric" },
    };

    Chart._adapters._date.override({
        formats() {
            const out = {};
            for (const unit of Object.keys(OPTIONS)) out[unit] = unit;
            return out;
        },
        parse(value) {
            if (value === null || value === undefined) return null;
            const t = typeof value === "number" ? value : new Date(value).getTime();
            return isNaN(t) ? null : t;
        },
        format(t, format) {
            return IsleyTime.formatDateTime(t, OPTIONS[format] || OPTIONS.datetime);
        },
        add,
        diff,
        startOf,
        endOf(t, unit) {
            return add(startOf(t, unit), 1, unit) - 1;
        },
    });

    // The calendar math, for graph.js (round minute ticks) and tests.
    window.IsleyChartZone = { wall, fromWall, startOf, add, diff };
})();
