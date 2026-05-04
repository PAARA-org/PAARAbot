package bot

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/PAARA-org/PAARAbot/pota"
	"github.com/PAARA-org/PAARAbot/sota"
	"github.com/bwmarrin/discordgo"
)

const (
	maxCachedSpots  = 10
	dedupGapMaxTime = time.Hour
)

type DisplaySpot struct {
	ID        string
	Source    string
	RawTime   time.Time
	Location  string
	Frequency string
	Mode      string
	Comment   string
	QRT       bool
}

var (
	spotCache = make(map[string][]DisplaySpot)
	cacheMu   sync.RWMutex
)

// updateCache adds a spot to the cache for a callsign. Chronologically
// consecutive duplicates (same source, location, frequency, mode and comment)
// are collapsed into a single entry whose timestamp is bumped to the latest
// observation.
func updateCache(callsign string, spot DisplaySpot) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	callsign = strings.ToUpper(callsign)
	spots := append(spotCache[callsign], spot)
	spotCache[callsign] = dedupAndSortSpots(spots, maxCachedSpots)
}

// getCachedSpots returns a copy of cached spots for a callsign.
func getCachedSpots(callsign string) []DisplaySpot {
	cacheMu.RLock()
	defer cacheMu.RUnlock()

	if spots, ok := spotCache[callsign]; ok {
		// Return a copy
		result := make([]DisplaySpot, len(spots))
		copy(result, spots)
		return result
	}
	return nil
}

// spotsMatch reports whether two spots describe the same activation, so a
// later one is just a re-spot rather than a new entry worth recording.
// Comment is intentionally excluded: POTA emits a separate spot per spotter,
// each with their own free-text comment, for what is the same activation.
// QRT is part of the key so a NORMAL→QRT (or QRT→NORMAL) flip on the same
// frequency is preserved as a separate cache entry.
func spotsMatch(a, b DisplaySpot) bool {
	return a.Source == b.Source &&
		a.Location == b.Location &&
		a.Frequency == b.Frequency &&
		a.Mode == b.Mode &&
		a.QRT == b.QRT
}

// IsQRT reports whether a free-text spot comment indicates the activator is
// going off-air. POTA has no status field and SOTA's TEST/NORMAL types don't
// cover the QRT-via-comment path, so we look for the Q-code in the text.
func IsQRT(comment string) bool {
	return strings.Contains(strings.ToUpper(comment), "QRT")
}

// SpotStatus captures the bits of a spot we watch for transitions: QRT (from
// comment for POTA, from comment or Type for SOTA) and the SOTA Type field
// itself, so a NORMAL↔TEST flip is also caught.
type SpotStatus struct {
	QRT  bool
	Type string
}

// StatusTracker remembers the last-seen status per activation key and reports
// when it changes, so callers can fire an out-of-band Discord message that
// bypasses the normal rate limiter.
type StatusTracker struct {
	mu    sync.Mutex
	state map[string]SpotStatus
}

func NewStatusTracker() *StatusTracker {
	return &StatusTracker{state: make(map[string]SpotStatus)}
}

// Transition records the latest status for key and returns true if it differs
// from the previously stored one. The first observation is never a transition.
func (t *StatusTracker) Transition(key string, status SpotStatus) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	prev, exists := t.state[key]
	t.state[key] = status
	return exists && prev != status
}

// dedupAndSortSpots sorts spots chronologically, collapses runs of consecutive
// duplicates onto the latest timestamp, and returns the most recent entries
// first, capped at limit.
func dedupAndSortSpots(spots []DisplaySpot, limit int) []DisplaySpot {
	if len(spots) == 0 {
		return spots
	}

	sort.SliceStable(spots, func(i, j int) bool {
		return spots[i].RawTime.Before(spots[j].RawTime)
	})

	deduped := make([]DisplaySpot, 0, len(spots))
	for _, s := range spots {
		if n := len(deduped); n > 0 && spotsMatch(deduped[n-1], s) &&
			s.RawTime.Sub(deduped[n-1].RawTime) <= dedupGapMaxTime {
			if s.RawTime.After(deduped[n-1].RawTime) {
				deduped[n-1].RawTime = s.RawTime
				deduped[n-1].ID = s.ID
			}
			continue
		}
		deduped = append(deduped, s)
	}

	// Reverse to newest-first.
	for i, j := 0, len(deduped)-1; i < j; i, j = i+1, j-1 {
		deduped[i], deduped[j] = deduped[j], deduped[i]
	}

	if len(deduped) > limit {
		deduped = deduped[:limit]
	}
	return deduped
}

func messageHandler(s *discordgo.Session, m *discordgo.MessageCreate) {
	// Ignore all messages created by the bot itself
	if m.Author.ID == s.State.User.ID {
		return
	}

	// Check if the message is in the correct channels
	if m.ChannelID != PotaChannelID && m.ChannelID != SotaChannelID {
		return
	}

	// Check if the bot is mentioned
	isMentioned := false
	for _, user := range m.Mentions {
		if user.ID == s.State.User.ID {
			isMentioned = true
			break
		}
	}

	if !isMentioned {
		return
	}

	// Extract CallSign
	// Assumes format: "@Bot CallSign" or similar.
	// We split by spaces and look for the token after the mention or just the first non-mention word.
	content := strings.TrimSpace(m.Content)
	parts := strings.Fields(content)

	var callsign string
	for _, part := range parts {
		// specific mention format <@!ID> or <@ID>
		if !strings.HasPrefix(part, "<@") {
			callsign = strings.ToUpper(part)
			break
		}
	}

	if callsign == "" {
		return // No callsign found
	}

	// Check Cache
	spots := getCachedSpots(callsign)

	// If cache is empty, fetch fresh data
	if len(spots) == 0 {
		spots = fetchFreshSpots(callsign)
	}

	if len(spots) == 0 {
		if _, err := s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("No recent spots found for %s.", callsign)); err != nil {
			fmt.Println("Error sending message:", err)
		}
		return
	}

	// Format output
	var sb strings.Builder
	fmt.Fprintf(&sb, "Most recent %d spots for **%s**:\n", maxCachedSpots, callsign)
	for _, spot := range spots {
		suffix := ""
		if spot.QRT {
			suffix = " QRT"
		}
		fmt.Fprintf(&sb, "- **%s** [%s] %s %s %s%s\n", spot.Source, formatSpotTime(spot.RawTime), spot.Location, spot.Frequency, spot.Mode, suffix)
	}

	if _, err := s.ChannelMessageSend(m.ChannelID, sb.String()); err != nil {
		fmt.Println("Error sending message:", err)
	}
}

// parseRawTime parses a POTA/SOTA timestamp into a time.Time. Naive
// timestamps are assumed to be UTC. A zero time is returned on parse failure.
func parseRawTime(rawTime string) time.Time {
	formats := []string{
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		time.RFC3339,
	}

	for _, f := range formats {
		var (
			t   time.Time
			err error
		)
		if !strings.Contains(f, "Z") && !strings.Contains(f, "-07") {
			t, err = time.ParseInLocation(f, rawTime, time.UTC)
		} else {
			t, err = time.Parse(f, rawTime)
		}
		if err == nil {
			return t
		}
	}
	return time.Time{}
}

func formatSpotTime(t time.Time) string {
	if t.IsZero() {
		return "?"
	}
	return t.Local().Format("01/02 15:04")
}

func fetchFreshSpots(callsign string) []DisplaySpot {
	var results []DisplaySpot

	// Fetch POTA
	potaSpots, err := pota.ListSpots()
	if err == nil {
		for _, v := range potaSpots {
			if strings.EqualFold(v.Activator, callsign) {
				results = append(results, DisplaySpot{
					ID:        fmt.Sprintf("POTA-%d", v.SpotID),
					Source:    "POTA",
					RawTime:   parseRawTime(v.SpotTime),
					Location:  fmt.Sprintf("%s (%s %s)", v.Reference, v.Name, v.LocationDesc),
					Frequency: v.Frequency,
					Mode:      v.Mode,
					Comment:   v.Comments,
				})
			}
		}
	}

	// Fetch SOTA
	sotaSpots, err := sota.ListSpots()
	if err == nil {
		for _, v := range sotaSpots {
			if strings.EqualFold(v.ActivatorCallsign, callsign) {
				freq := fmt.Sprintf("%.3fMHz", v.Frequency)
				results = append(results, DisplaySpot{
					ID:        fmt.Sprintf("SOTA-%d", v.Id),
					Source:    "SOTA",
					RawTime:   parseRawTime(v.TimeStamp),
					Location:  fmt.Sprintf("%s (%s)", v.SummitCode, v.SummitName),
					Frequency: freq,
					Mode:      v.Mode,
					Comment:   v.Comments,
				})
			}
		}
	}

	return dedupAndSortSpots(results, maxCachedSpots)
}
