package pagination

import (
	"encoding/json"
	"testing"
)

func TestPageContract(t *testing.T) {
	for _, tt := range []struct{ in, want PageQuery }{
		{PageQuery{}, PageQuery{1, 10}},
		{PageQuery{2, -1}, PageQuery{2, -1}},
		{PageQuery{-2, 999}, PageQuery{1, 200}},
		{PageQuery{3, 20}, PageQuery{3, 20}},
	} {
		if got := tt.in.Normalize(); got != tt.want {
			t.Fatalf("%+v normalized to %+v, want %+v", tt.in, got, tt.want)
		}
	}
	encoded, err := json.Marshal(PageResult[int]{})
	if err != nil || string(encoded) != `{"list":[],"total":0}` {
		t.Fatalf("empty page = %s, %v", encoded, err)
	}
	if offset := (PageQuery{PageNo: 3, PageSize: 20}).Offset(); offset != 40 {
		t.Fatal(offset)
	}
	if offset := (PageQuery{PageNo: 3, PageSize: -1}).Offset(); offset != 0 {
		t.Fatal(offset)
	}
}
