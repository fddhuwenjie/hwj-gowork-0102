package domain

import "time"

func TimeOrNow(t time.Time, now time.Time) time.Time {
	if t.IsZero() {
		return now
	}
	return t
}

func BoolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
