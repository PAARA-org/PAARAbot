package bot

import (
	"testing"
	"time"
)

func mkSpot(min int, freq, comment string) DisplaySpot {
	return DisplaySpot{
		Source:    "POTA",
		RawTime:   time.Date(2026, 4, 29, 8, min, 0, 0, time.UTC),
		Location:  "US-0189 (Don Edwards SF Bay NWR US-CA)",
		Frequency: freq,
		Mode:      "CW",
		Comment:   comment,
	}
}

func TestDedupAndSortSpots_CollapsesConsecutiveDuplicates(t *testing.T) {
	// Mirrors the W6JY example: 14044.0 → 14044.1 → 14044.0 should collapse
	// to 3 entries. Input is in arrival order (chronological).
	input := []DisplaySpot{
		mkSpot(57, "14044.0", ""),
		mkSpot(57, "14044.0", ""),
		mkSpot(58, "14044.0", ""),
		mkSpot(59, "14044.0", ""),
		mkSpot(59, "14044.1", ""),
		mkSpot(59, "14044.0", ""),
		mkSpot(60, "14044.0", ""),
		mkSpot(61, "14044.0", ""),
		mkSpot(62, "14044.0", ""),
		mkSpot(62, "14044.0", ""),
	}

	got := dedupAndSortSpots(input, 10)

	if len(got) != 3 {
		t.Fatalf("expected 3 entries, got %d: %+v", len(got), got)
	}

	// Newest-first ordering, with timestamps bumped to the latest within each run.
	// time.Date normalizes minute 62 → 09:02, so .Minute() returns 2.
	want := []struct {
		freq string
		hour int
		min  int
	}{
		{"14044.0", 9, 2},  // second 14044.0 run, ends at 09:02
		{"14044.1", 8, 59}, // single 14044.1 spot
		{"14044.0", 8, 59}, // first 14044.0 run, ends at 08:59
	}
	for i, w := range want {
		gh, gm := got[i].RawTime.Hour(), got[i].RawTime.Minute()
		if got[i].Frequency != w.freq || gh != w.hour || gm != w.min {
			t.Errorf("entry %d: got %s @ %02d:%02d, want %s @ %02d:%02d",
				i, got[i].Frequency, gh, gm, w.freq, w.hour, w.min)
		}
	}
}

func TestDedupAndSortSpots_DifferentCommentIsNotDuplicate(t *testing.T) {
	input := []DisplaySpot{
		mkSpot(10, "14044.0", "QRT soon"),
		mkSpot(11, "14044.0", "QRV"),
	}
	got := dedupAndSortSpots(input, 10)
	if len(got) != 2 {
		t.Fatalf("different comments should not collapse, got %d entries", len(got))
	}
}

func TestDedupAndSortSpots_OutOfOrderInputIsSorted(t *testing.T) {
	input := []DisplaySpot{
		mkSpot(15, "14044.0", ""),
		mkSpot(10, "14044.0", ""),
		mkSpot(20, "14044.1", ""),
	}
	got := dedupAndSortSpots(input, 10)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries after collapsing the 14044.0 run, got %d", len(got))
	}
	// Newest first.
	if got[0].Frequency != "14044.1" || got[0].RawTime.Minute() != 20 {
		t.Errorf("entry 0: got %s @ :%02d, want 14044.1 @ :20", got[0].Frequency, got[0].RawTime.Minute())
	}
	if got[1].Frequency != "14044.0" || got[1].RawTime.Minute() != 15 {
		t.Errorf("entry 1: got %s @ :%02d (timestamp should be bumped to latest in run, :15)",
			got[1].Frequency, got[1].RawTime.Minute())
	}
}

func TestDedupAndSortSpots_RespectsLimit(t *testing.T) {
	input := make([]DisplaySpot, 0, 12)
	for i := 0; i < 12; i++ {
		// Each unique frequency so nothing collapses.
		input = append(input, DisplaySpot{
			Source:    "POTA",
			RawTime:   time.Date(2026, 4, 29, 8, i, 0, 0, time.UTC),
			Location:  "US-0189",
			Frequency: string(rune('A' + i)),
			Mode:      "CW",
		})
	}
	got := dedupAndSortSpots(input, 10)
	if len(got) != 10 {
		t.Fatalf("expected limit of 10, got %d", len(got))
	}
	// Newest entry (minute 11) should be first.
	if got[0].RawTime.Minute() != 11 {
		t.Errorf("expected newest-first ordering, got minute %d at index 0", got[0].RawTime.Minute())
	}
}

func TestDedupAndSortSpots_EmptyInput(t *testing.T) {
	if got := dedupAndSortSpots(nil, 10); len(got) != 0 {
		t.Errorf("expected empty result for nil input, got %d entries", len(got))
	}
}
