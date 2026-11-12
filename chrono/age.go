package chrono

// PlantAge returns the day and week a plant started on start is on, on
// today. This is the one definition of plant age, used by every page, the
// API and MCP:
//
//   - The start day is Day 1; the count rolls over at midnight in the
//     instance zone (callers pass today from Zones.Today).
//   - Week 1 is days 1-7, week 2 days 8-14, and so on.
//
// A start day in the future gives day 0 or less and week 0.
func PlantAge(start, today Date) (day, week int) {
	day = today.DaysSince(start) + 1
	if day < 1 {
		return day, 0
	}
	return day, (day-1)/7 + 1
}
