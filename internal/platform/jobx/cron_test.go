package jobx

import (
	"testing"
	"time"
)

func TestQuartzCalendarBoundaries(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	cron := NewCron(loc)
	after := time.Date(2026, time.November, 6, 12, 0, 0, 0, loc)
	cases := []struct {
		expression string
		want       time.Time
		invalid    bool
	}{
		{expression: "0 0 2 ? 2 6#5 2027"},
		{expression: "0 0 2 31 2 ?"},
		{expression: "0 0 2 31 * ?", want: time.Date(2026, time.December, 31, 2, 0, 0, 0, loc)},
		{expression: "0 0 2 29 2 ?", want: time.Date(2028, time.February, 29, 2, 0, 0, 0, loc)},
		{expression: "0 0 2 ? * L", want: time.Date(2026, time.November, 7, 2, 0, 0, 0, loc)},
		{expression: "0 0 2 LW * ?", want: time.Date(2026, time.November, 30, 2, 0, 0, 0, loc)},
		{expression: "0 0 22-2/2 * * ?", want: time.Date(2026, time.November, 6, 22, 0, 0, 0, loc)},
		{expression: "0 0 2 ? * FRI-MON", want: time.Date(2026, time.November, 7, 2, 0, 0, 0, loc)},
		{expression: "0 0 2 1 1 ? 2299", invalid: true},
		{expression: "1/9223372036854775807 0 2 * * ?", invalid: true},
		{expression: "0 0 2 ? * 6,8", invalid: true},
	}
	for _, tc := range cases {
		t.Run(tc.expression, func(t *testing.T) {
			next, err := cron.Next(tc.expression, after)
			if (err != nil) != tc.invalid || !next.Equal(tc.want) {
				t.Fatalf("next run = %v, err = %v, want %v, invalid = %t", next, err, tc.want, tc.invalid)
			}
		})
	}
}
