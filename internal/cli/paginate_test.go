package cli

import "testing"

func rowsN(n int) []Row {
	rows := make([]Row, n)
	for i := range rows {
		rows[i] = Row{ID: string(rune('a' + i))}
	}
	return rows
}

func TestPaginate_InBounds(t *testing.T) {
	got := Paginate(rowsN(5), 1, 2)
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "c" {
		t.Fatalf("unexpected slice: %+v", got)
	}
}

func TestPaginate_OffsetPastEnd(t *testing.T) {
	got := Paginate(rowsN(3), 10, 5)
	if got == nil || len(got) != 0 {
		t.Fatalf("offset past end must yield empty (not nil-error), got %#v", got)
	}
}

func TestPaginate_LimitZeroIsUnlimited(t *testing.T) {
	got := Paginate(rowsN(4), 0, 0)
	if len(got) != 4 {
		t.Fatalf("limit 0 should be unlimited, got %d rows", len(got))
	}
}

func TestPaginate_LimitNegativeIsUnlimited(t *testing.T) {
	got := Paginate(rowsN(4), 1, -1)
	if len(got) != 3 {
		t.Fatalf("negative limit should be unlimited from offset, got %d rows", len(got))
	}
}

func TestPaginate_NegativeOffsetClampsToZero(t *testing.T) {
	got := Paginate(rowsN(3), -5, 1)
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("negative offset should clamp to 0, got %+v", got)
	}
}
