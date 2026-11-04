package timeutil

import (
	"encoding/json"
	"testing"
	"time"
)

func TestMillisecondsAndShanghaiRange(t *testing.T) {
	want := time.Date(2026, 11, 5, 0, 0, 0, 0, time.UTC)
	for _, raw := range []string{"2026-11-05 08:00:00", "1793836800000"} {
		got, err := Parse(raw)
		if err != nil || !got.Equal(want) || got.Location() != time.UTC {
			t.Fatalf("Parse(%q) = %v, %v; want %v UTC", raw, got, err, want)
		}
	}
	value := MillisTime{Time: want}
	encoded, err := json.Marshal(value)
	if err != nil || string(encoded) != "1793836800000" {
		t.Fatalf("marshal = %s, %v", encoded, err)
	}
	var decoded MillisTime
	if err := json.Unmarshal(encoded, &decoded); err != nil || !decoded.Equal(want) {
		t.Fatalf("round trip: %v, %v", decoded, err)
	}
	if _, err := Parse("2026-13-01 00:00:00"); err == nil {
		t.Fatal("invalid timestamp accepted")
	}
}
