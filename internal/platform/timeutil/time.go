// Package timeutil implements the frontend millisecond timestamp contract.
package timeutil

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

type MillisTime struct{ time.Time }

func (t MillisTime) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}
	return strconv.AppendInt(nil, t.UnixMilli(), 10), nil
}

func (t *MillisTime) UnmarshalJSON(data []byte) error {
	if bytes.Equal(data, []byte("null")) {
		t.Time = time.Time{}
		return nil
	}
	var raw string
	if len(data) > 0 && data[0] == '"' {
		if err := json.Unmarshal(data, &raw); err != nil {
			return err
		}
	} else {
		raw = string(data)
	}
	value, err := Parse(raw)
	if err != nil {
		return err
	}
	t.Time = value
	return nil
}

// Parse accepts epoch milliseconds or a frontend wall-clock timestamp in Asia/Shanghai.
func Parse(raw string) (time.Time, error) {
	if ms, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return time.UnixMilli(ms).UTC(), nil
	}
	loc, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		return time.Time{}, fmt.Errorf("load application timezone: %w", err)
	}
	if value, err := time.ParseInLocation("2006-01-02 15:04:05", raw, loc); err == nil {
		return value.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("invalid timestamp %q", raw)
}
