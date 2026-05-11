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

func TestDedupAndSortSpots_DifferentCommentStillCollapses(t *testing.T) {
	// POTA emits one spot per spotter, each with its own free-text comment,
	// for what is the same activation. They should collapse to one row.
	input := []DisplaySpot{
		mkSpot(10, "14044.0", "QRT soon"),
		mkSpot(11, "14044.0", "QRV"),
	}
	got := dedupAndSortSpots(input, 10)
	if len(got) != 1 {
		t.Fatalf("different comments at same freq should collapse, got %d entries", len(got))
	}
	if got[0].RawTime.Minute() != 11 {
		t.Errorf("expected timestamp bumped to latest (:11), got :%02d", got[0].RawTime.Minute())
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

func TestDedupAndSortSpots_GapOverOneHourDoesNotCollapse(t *testing.T) {
	// Same park/freq/mode, but spots are >1h apart → treated as a separate
	// activation rather than folded into one entry.
	morning := time.Date(2026, 4, 30, 9, 1, 0, 0, time.UTC)
	afternoon := time.Date(2026, 4, 30, 11, 5, 0, 0, time.UTC)
	input := []DisplaySpot{
		{Source: "POTA", RawTime: morning, Location: "US-0189", Frequency: "14036.0", Mode: "CW"},
		{Source: "POTA", RawTime: afternoon, Location: "US-0189", Frequency: "14036.0", Mode: "CW"},
	}
	got := dedupAndSortSpots(input, 10)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries for >1h gap, got %d", len(got))
	}
	if got[0].RawTime != afternoon || got[1].RawTime != morning {
		t.Errorf("expected newest-first ordering, got %v then %v", got[0].RawTime, got[1].RawTime)
	}
}

func TestDedupAndSortSpots_GapWithinOneHourCollapses(t *testing.T) {
	t0 := time.Date(2026, 4, 30, 9, 1, 0, 0, time.UTC)
	t1 := t0.Add(59 * time.Minute)
	input := []DisplaySpot{
		{Source: "POTA", RawTime: t0, Location: "US-0189", Frequency: "14036.0", Mode: "CW"},
		{Source: "POTA", RawTime: t1, Location: "US-0189", Frequency: "14036.0", Mode: "CW"},
	}
	got := dedupAndSortSpots(input, 10)
	if len(got) != 1 {
		t.Fatalf("expected 1 entry for <1h gap, got %d", len(got))
	}
	if got[0].RawTime != t1 {
		t.Errorf("expected timestamp bumped to latest, got %v", got[0].RawTime)
	}
}

func TestIsQRT(t *testing.T) {
	cases := map[string]bool{
		"":                       false,
		"QRV CW":                 false,
		"QRT":                    true,
		"qrt 73":                 true,
		"QRT in 5":               true,
		"55N SC":                 false,
		"RBN 8 dB 26 WPM":        false,
		"going QRT, thanks all!": true,
	}
	for in, want := range cases {
		if got := IsQRT(in); got != want {
			t.Errorf("IsQRT(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestDedupAndSortSpots_QRTTransitionIsNotCollapsed(t *testing.T) {
	// NORMAL → QRT → NORMAL on the same frequency must remain three entries
	// so the transition is visible in the recent-spots view.
	t0 := time.Date(2026, 4, 30, 9, 0, 0, 0, time.UTC)
	input := []DisplaySpot{
		{Source: "POTA", RawTime: t0, Location: "US-0189", Frequency: "14036.0", Mode: "CW"},
		{Source: "POTA", RawTime: t0.Add(5 * time.Minute), Location: "US-0189", Frequency: "14036.0", Mode: "CW", QRT: true},
		{Source: "POTA", RawTime: t0.Add(10 * time.Minute), Location: "US-0189", Frequency: "14036.0", Mode: "CW"},
	}
	got := dedupAndSortSpots(input, 10)
	if len(got) != 3 {
		t.Fatalf("expected 3 entries across NORMAL/QRT/NORMAL flips, got %d", len(got))
	}
	if got[0].QRT || !got[1].QRT || got[2].QRT {
		t.Errorf("expected newest-first NORMAL/QRT/NORMAL, got QRT flags %v/%v/%v",
			got[0].QRT, got[1].QRT, got[2].QRT)
	}
}

func TestStatusTracker_Transition(t *testing.T) {
	tr := NewStatusTracker()

	// First observation is never a transition.
	if tr.Transition("k", SpotStatus{QRT: false}) {
		t.Error("first observation should not be a transition")
	}
	// Same status repeated: no transition.
	if tr.Transition("k", SpotStatus{QRT: false}) {
		t.Error("unchanged status should not be a transition")
	}
	// NORMAL → QRT.
	if !tr.Transition("k", SpotStatus{QRT: true}) {
		t.Error("NORMAL→QRT should be a transition")
	}
	// QRT → NORMAL is latched: the tracker keeps the activation in QRT and
	// does not report a transition, so the alternating Qrt/RBN stream stops
	// bypassing the rate limiter.
	if tr.Transition("k", SpotStatus{QRT: false}) {
		t.Error("QRT→NORMAL must be latched, not reported as a transition")
	}
	// A later QRT spot is also not a transition because the tracker is
	// already in QRT — re-announcing it adds no value.
	if tr.Transition("k", SpotStatus{QRT: true}) {
		t.Error("QRT after latch should not be a fresh transition")
	}
	// SOTA Type change with QRT unchanged still counts as a transition.
	tr.Transition("sota", SpotStatus{QRT: false, Type: "NORMAL"})
	if !tr.Transition("sota", SpotStatus{QRT: false, Type: "TEST"}) {
		t.Error("Type change should be a transition even when QRT is unchanged")
	}
	// Type changes still fire even when QRT is latched.
	tr.Transition("sotaQRT", SpotStatus{QRT: true, Type: "NORMAL"})
	if !tr.Transition("sotaQRT", SpotStatus{QRT: false, Type: "TEST"}) {
		t.Error("Type change should fire even when QRT is latched")
	}
	// Different keys are independent.
	if tr.Transition("other", SpotStatus{QRT: true}) {
		t.Error("first observation under a new key should not be a transition")
	}
}

func TestDedupAndSortSpots_EmptyInput(t *testing.T) {
	if got := dedupAndSortSpots(nil, 10); len(got) != 0 {
		t.Errorf("expected empty result for nil input, got %d entries", len(got))
	}
}
