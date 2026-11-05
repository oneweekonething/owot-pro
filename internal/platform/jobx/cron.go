// Package jobx evaluates Quartz schedules for the job dispatcher.
package jobx

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/JiaBao-do/quartzx"
)

type Cron struct{ location *time.Location }

func NewCron(location *time.Location) Cron { return Cron{location: location} }

func (c Cron) Next(expression string, after time.Time) (time.Time, error) {
	expression, err := normalizeQuartz(expression)
	if err != nil {
		return time.Time{}, err
	}
	schedule, err := quartzx.ParseCron(expression)
	if err != nil {
		return time.Time{}, err
	}
	next, ok := schedule.In(c.location).Next(after)
	if !ok {
		return time.Time{}, nil
	}
	return next.UTC(), nil
}

type cronField struct {
	first, last int
	names       string
}

func (f cronField) value(raw string) (int, error) {
	for i, name := range strings.Split(f.names, " ") {
		if name != "" && strings.EqualFold(raw, name) {
			return f.first + i, nil
		}
	}
	return strconv.Atoi(raw)
}

func normalizeQuartz(expression string) (string, error) {
	fields := strings.Fields(expression)
	if len(fields) != 6 && len(fields) != 7 {
		if len(fields) == 1 && strings.HasPrefix(fields[0], "@") {
			return fields[0], nil
		}
		return "", fmt.Errorf("expected six or seven Quartz fields")
	}
	if strings.EqualFold(fields[5], "L") {
		fields[5] = "SAT"
	}
	bounds := [...]cronField{
		{0, 59, ""}, {0, 59, ""}, {0, 23, ""}, {1, 31, ""},
		{1, 12, "JAN FEB MAR APR MAY JUN JUL AUG SEP OCT NOV DEC"},
		{1, 7, "SUN MON TUE WED THU FRI SAT"},
	}
	for i, field := range fields {
		items := strings.Split(field, ",")
		for j, item := range items {
			value, rawStep, hasStep := strings.Cut(item, "/")
			step := 1
			if hasStep {
				n, err := strconv.Atoi(rawStep)
				// An increment greater than the year field's entire span is invalid.
				// Bound it before the parser adds it to a field value.
				if err != nil || n < 1 || n > 230 {
					return "", fmt.Errorf("invalid increment in Quartz field %q", item)
				}
				step = n
			}
			start, end, hasRange := strings.Cut(value, "-")
			if i == len(bounds) || !hasRange {
				continue
			}
			items[j] = expandWrappedRange(bounds[i], item, start, end, step)
		}
		fields[i] = strings.Join(items, ",")
	}
	return strings.Join(fields, " "), nil
}

func expandWrappedRange(field cronField, original, start, end string, step int) string {
	from, err := field.value(start)
	to, endErr := field.value(end)
	if err != nil || endErr != nil || from <= to || from > field.last || to < field.first {
		return original
	}
	width := field.last - field.first + 1
	values := make([]string, 0, width)
	for value := from; value <= to+width; value += step {
		values = append(values, strconv.Itoa(field.first+(value-field.first)%width))
	}
	return strings.Join(values, ",")
}
