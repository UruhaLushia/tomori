package page

import (
	"fmt"

	"github.com/UruhaLushia/tomori/locales"
)

func playSummary(s HomeStatistics) (days [7]float64, launchDays [7]int, launches int) {
	if len(s.Packs) == 0 {
		return s.Days, s.DayLaunches, s.Launches
	}
	for _, pack := range s.Packs {
		launches += pack.Launches
		for i, value := range pack.Days {
			days[i] += value
			launchDays[i] += pack.LaunchesByDay[i]
		}
	}
	return days, launchDays, launches
}

func playDuration(seconds float64) string {
	minutes := max(0, int64(seconds/60))
	hours := minutes / 60
	if hours == 0 {
		return fmt.Sprintf(locales.DurationMinutes(), minutes)
	}
	return fmt.Sprintf(locales.DurationHoursMinutes(), hours, minutes%60)
}
